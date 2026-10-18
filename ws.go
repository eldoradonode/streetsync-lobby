package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"
)

const (
	readLimitBytes = 2048
	handshakeWait  = 10 * time.Second // time allowed to send the first frame
	authWait       = 12 * time.Second // time allowed for the database and password work at sign in
	writeWait      = 10 * time.Second
	pingEvery      = 25 * time.Second
)

func (h *hub) reject(ctx context.Context, conn *websocket.Conn, code string) {
	h.rejectN(ctx, conn, code, 0)
}

// rejectN is reject with a number, for example the minutes left on a kick.
func (h *hub) rejectN(ctx context.Context, conn *websocket.Conn, code string, n int) {
	frameBytes := errFrame(code)
	if n > 0 {
		frameBytes = errFrameN(code, n)
	}
	wctx, cancel := context.WithTimeout(ctx, writeWait)
	defer cancel()
	_ = conn.Write(wctx, websocket.MessageText, frameBytes)
	_ = conn.Close(websocket.StatusPolicyViolation, code)
}

// authenticate turns the first frame into a user and a session token.
// On failure it returns an error code for the client.
func (h *hub) authenticate(ctx context.Context, in inbound) (userRow, string, string) {
	switch in.T {
	case "hi":
		if in.K != "" {
			row, err := h.st.userBySession(ctx, in.K)
			if errors.Is(err, errNotFound) {
				return userRow{}, "", "session_expired"
			}
			if err != nil {
				log.Printf("session lookup failed: %v", err)
				return userRow{}, "", "server_busy"
			}
			return row, in.K, ""
		}
		handle, key, ok := normHandle(in.H)
		if !ok {
			return userRow{}, "", "bad_handle"
		}
		hash := ""
		if h.requirePass {
			// A new account needs a password. Check the handle first so a taken handle costs no hashing.
			if len(in.P) < minPassBytes || len(in.P) > maxPassBytes {
				return userRow{}, "", "weak_password"
			}
			if _, err := h.st.userByKey(ctx, key); err == nil {
				return userRow{}, "", "handle_taken"
			} else if !errors.Is(err, errNotFound) {
				log.Printf("handle lookup failed: %v", err)
				return userRow{}, "", "server_busy"
			}
			hs, err := h.auth.hash(ctx, in.P)
			if err != nil {
				return userRow{}, "", "server_busy"
			}
			hash = hs
		}
		row, created, err := h.st.createAccount(ctx, handle, key, cleanAvatar(in.V), hash)
		if err != nil {
			log.Printf("creating guest failed: %v", err)
			return userRow{}, "", "server_busy"
		}
		if !created {
			return userRow{}, "", "handle_taken"
		}
		token, err := h.st.newSession(ctx, row.ID)
		if err != nil {
			log.Printf("creating session failed: %v", err)
			return userRow{}, "", "server_busy"
		}
		return row, token, ""

	case "login":
		_, key, ok := normHandle(in.H)
		if !ok {
			return userRow{}, "", "bad_login"
		}
		now := time.Now()
		if !h.limit.allowed(key, now) {
			return userRow{}, "", "too_many_attempts"
		}
		row, err := h.st.userByKey(ctx, key)
		if err != nil && !errors.Is(err, errNotFound) {
			log.Printf("login lookup failed: %v", err)
			return userRow{}, "", "server_busy"
		}
		// Always do the same password work, so response time does not reveal which handles exist.
		stored := h.auth.dummy
		if err == nil && row.PassHash.Valid {
			stored = row.PassHash.String
		}
		match, aerr := h.auth.verify(ctx, stored, in.P)
		if aerr != nil {
			return userRow{}, "", "server_busy"
		}
		if err != nil || !row.PassHash.Valid || !match {
			h.limit.fail(key, now)
			return userRow{}, "", "bad_login"
		}
		h.limit.reset(key)
		token, err := h.st.newSession(ctx, row.ID)
		if err != nil {
			log.Printf("creating session failed: %v", err)
			return userRow{}, "", "server_busy"
		}
		h.st.touchUser(ctx, row.ID)
		return row, token, ""
	}
	return userRow{}, "", "bad_hello"
}

func (h *hub) serveWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.origins})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(readLimitBytes)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// First frame must say who is connecting.
	hctx, hcancel := context.WithTimeout(ctx, handshakeWait)
	_, data, err := conn.Read(hctx)
	hcancel()
	if err != nil {
		return
	}
	var in inbound
	if json.Unmarshal(data, &in) != nil {
		h.reject(ctx, conn, "bad_hello")
		return
	}

	actx, acancel := context.WithTimeout(ctx, authWait)
	row, token, code := h.authenticate(actx, in)
	var muteUntil time.Time
	kickMinutes := 0
	if code == "" {
		mute, kick, err := h.st.activeSanctions(actx, row.ID)
		switch {
		case err != nil:
			log.Printf("sanction lookup failed: %v", err)
			code = "server_busy"
		case kick.After(time.Now()):
			code, kickMinutes = "kicked", minutesLeft(kick)
		default:
			muteUntil = mute
		}
	}
	acancel()
	if code != "" {
		h.rejectN(ctx, conn, code, kickMinutes)
		return
	}

	ou := h.acquire(row, muteUntil)
	cl := &client{send: make(chan []byte, sendQueueSize), cancel: cancel}
	h.attach(ou, cl, token)
	defer h.detach(cl)
	if !row.Welcomed {
		safeGo(func() { h.welcome(ou) }) // welcome credits, once per account
	}
	safeGo(func() { h.sendBadgeTo(cl) })     // unread messages and waiting friend requests
	safeGo(func() { h.sendNoteStateTo(cl) }) // unread notifications and my settings
	safeGo(func() { h.loadMemberships(ou) }) // rooms I own or was let into
	if in.R != "" {
		h.enter(ctx, cl, in.R, in.L, "", "", nil) // a returning client names its room, so it lands back in it
	}
	h.mu.Lock()
	isMod := roleRank(ou.role) >= 2
	h.mu.Unlock()
	if isMod {
		safeGo(func() { h.sendReportCountTo(cl) })
	}

	go h.writeLoop(ctx, cancel, conn, cl)
	h.readLoop(ctx, conn, cl)
	_ = conn.Close(websocket.StatusNormalClosure, "")
}

func (h *hub) readLoop(ctx context.Context, conn *websocket.Conn, cl *client) {
	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		var in inbound
		if json.Unmarshal(data, &in) != nil {
			continue
		}
		switch in.T {
		case "m":
			if code := h.post(cl, in.B, in.Q); code == "muted" {
				cl.enqueue(errFrameN("muted", h.mutedMinutes(cl.user)))
			} else if code != "" {
				cl.enqueue(errFrame(code))
			}
		case "bye":
			h.logout(ctx, cl)
		case "join":
			h.enter(ctx, cl, in.R, in.L, in.C, in.P, in.F)
		case "av":
			h.setAvatar(ctx, cl, in)
		case "profile":
			h.profile(ctx, cl, in)
		case "bio":
			h.setBio(ctx, cl, in)
		case "dir":
			h.roomDir(cl)
		case "astats", "ausers", "astaff", "alog", "arooms":
			h.adminQuery(ctx, cl, in)
		case "tip":
			h.tip(ctx, cl, in)
		case "croom":
			h.roomCreate(ctx, cl, in)
		case "find":
			h.roomFind(cl, in)
		case "rinfo":
			h.roomInfo(ctx, cl, in)
		case "rset":
			h.roomSet(ctx, cl, in)
		case "hmod":
			h.hostMod(ctx, cl, in)
		case "rclose":
			h.roomClose(ctx, cl, in)
		case "leave":
			h.leaveRoom(cl)
		case "del":
			h.userDelete(ctx, cl, in)
		case "rep":
			h.report(ctx, cl, in)
		case "reports":
			h.listReports(ctx, cl)
		case "wallet":
			h.sendWallet(ctx, cl)
		case "dm":
			h.dmSend(ctx, cl, in)
		case "thread":
			h.dmThread(ctx, cl, in)
		case "read":
			h.dmRead(ctx, cl, in)
		case "inbox":
			h.dmInbox(ctx, cl)
		case "fr":
			h.friendAct(ctx, cl, in)
		case "blk":
			h.blockAct(ctx, cl, in)
		case "drep":
			h.dmReport(ctx, cl, in)
		case "mod":
			h.modAction(ctx, cl, in)
		case "p":
			cl.enqueue(pongFrame)
		case "pw":
			h.changePassword(ctx, cl, in.P)
		case "pwchange":
			h.passwordChange(ctx, cl, in)
		case "nlist":
			h.notesList(ctx, cl)
		case "nread":
			h.notesRead(ctx, cl, in)
		case "set":
			h.setPrefFrame(ctx, cl, in)
		}
	}
}

// changePassword protects a guest handle with a password.
func (h *hub) changePassword(ctx context.Context, cl *client, pw string) {
	ou := cl.user
	if len(pw) < minPassBytes || len(pw) > maxPassBytes {
		cl.enqueue(errFrame("weak_password"))
		return
	}
	h.mu.Lock()
	has := ou.hasPass
	h.mu.Unlock()
	if has {
		cl.enqueue(errFrame("has_password"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	hash, err := h.auth.hash(dctx, pw)
	if err != nil {
		cl.enqueue(errFrame("server_busy"))
		return
	}
	ok, err := h.st.setPassword(dctx, ou.id, hash)
	if err != nil {
		log.Printf("saving password failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if !ok {
		cl.enqueue(errFrame("has_password"))
		return
	}
	h.mu.Lock()
	ou.hasPass = true
	me := frame(meFrame{T: "me", meFields: ou.me()})
	for c := range ou.clients {
		c.enqueue(me)
	}
	h.mu.Unlock()
	cl.enqueue(frame(map[string]string{"t": "pwok"}))
}

func (h *hub) writeLoop(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, cl *client) {
	defer cancel()
	tick := time.NewTicker(pingEvery)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-cl.send:
			wctx, wcancel := context.WithTimeout(ctx, writeWait)
			err := conn.Write(wctx, websocket.MessageText, b)
			wcancel()
			if err != nil {
				return
			}
		case <-tick.C:
			pctx, pcancel := context.WithTimeout(ctx, writeWait)
			err := conn.Ping(pctx)
			pcancel()
			if err != nil {
				return
			}
		}
	}
}

// logout ends the login this connection used and closes the connection.
// The person's other logins, on other devices, keep working.
func (h *hub) logout(ctx context.Context, cl *client) {
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	if err := h.st.deleteSession(dctx, cl.tokenH); err != nil {
		log.Printf("ending a login failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	cl.enqueue(frame(map[string]string{"t": "byeok"}))
	time.AfterFunc(kickFlushDelay, cl.cancel)
}
