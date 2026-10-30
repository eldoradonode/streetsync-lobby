package main

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"
)

// Direct message rules. Starting values, easy to tune.
const (
	dmFee         = 1   // credits the sender pays per message to someone who is not a friend
	friendCap     = 100 // friends per person
	pendingInCap  = 50  // open requests a person can have waiting
	pendingOutCap = 30  // open requests a person can have out
	threadLimit   = 50  // messages sent when a conversation is opened
)

type dmFrame struct {
	T string `json:"t"`
	dmMsg
	Fa string `json:"fa,omitempty"` // the sender's avatar look
}

type threadFrame struct {
	T  string            `json:"t"`
	H  string            `json:"h"` // the other person
	S  string            `json:"s"` // none, friend, sent, got or blocked (I blocked them)
	C  int64             `json:"c"` // credits one message from me costs right now
	M  []dmMsg           `json:"m"`
	Av map[string]string `json:"av,omitempty"` // avatar looks of the two people
	Cl bool              `json:"cl,omitempty"` // they only take messages from friends, and I am not one
	Ek *keyRow           `json:"ek,omitempty"` // the other person's registered public key
	Mk *keyRow           `json:"mk,omitempty"` // my own registered public key (the page checks it holds the matching private key)
}

type inboxFrame struct {
	T string    `json:"t"`
	C []chatRow `json:"c"`
	friendLists
	Av map[string]string `json:"av,omitempty"` // avatar looks of everyone listed
}

// lookupUser finds a person by handle for a DM or friend action. It sends the error itself.
func (h *hub) lookupUser(ctx context.Context, cl *client, handle string) (userRow, bool) {
	_, key, ok := normHandle(handle)
	if !ok {
		cl.enqueue(errFrame("no_such_user"))
		return userRow{}, false
	}
	row, err := h.st.userByKey(ctx, key)
	if errors.Is(err, errNotFound) {
		cl.enqueue(errFrame("no_such_user"))
		return userRow{}, false
	}
	if err != nil {
		log.Printf("user lookup failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return userRow{}, false
	}
	return row, true
}

// takeToken uses one message from the person's rate limit bucket, shared with room chat.
// The caller holds the lock.
func takeToken(ou *onlineUser, now time.Time) bool {
	ou.tokens += now.Sub(ou.lastRefill).Seconds() * refillPerSec
	if ou.tokens > burstMessages {
		ou.tokens = burstMessages
	}
	ou.lastRefill = now
	if ou.tokens < 1 {
		return false
	}
	ou.tokens--
	return true
}

// sendToUser queues a frame for every open tab of a person who is online.
func (h *hub) sendToUser(id int64, b []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ou := h.users[id]; ou != nil {
		for c := range ou.clients {
			c.enqueue(b)
		}
	}
}

// dmSend sends one encrypted direct message: {"t":"dm","h":"bob","e":{...envelope...}}
// Plaintext is never accepted: a frame with a text body is refused and never stored, relayed or logged.
func (h *hub) dmSend(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	if in.B != "" || in.E == nil {
		cl.enqueue(errFrame("e2ee_required"))
		return
	}
	if err := in.E.validate(); err != nil {
		cl.enqueue(errFrame("bad_envelope"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	target, ok := h.lookupUser(dctx, cl, in.H)
	if !ok {
		return
	}
	if target.ID == ou.id {
		cl.enqueue(errFrame("self_dm"))
		return
	}
	if h.dmClosedTo(dctx, ou, target.ID) {
		cl.enqueue(errFrame("dm_closed"))
		return
	}

	now := time.Now()
	h.mu.Lock()
	staff := roleRank(ou.role) >= 2
	muted := now.Before(ou.mutedUntil)
	mins := minutesLeft(ou.mutedUntil)
	allowed := !muted && takeToken(ou, now)
	senderLook := ou.look()
	var id int64
	if allowed {
		h.nextID++
		id = h.nextID
	}
	h.mu.Unlock()
	if muted {
		cl.enqueue(errFrameN("muted", mins))
		return
	}
	if !allowed {
		cl.enqueue(errFrame("slow_down"))
		return
	}

	fee := int64(dmFee)
	if staff {
		fee = 0 // moderators and admins need to be able to reach people
	}
	m := dmMsg{ID: id, From: ou.handle, To: target.Handle, E: in.E, TS: now.UnixMilli(), St: 1}
	bal, charged, err := h.st.sendDMRow(dctx, id, ou.id, target.ID, target.Handle, dmInsert{Env: in.E}, m.TS, fee)
	switch {
	case errors.Is(err, errNoOwnKey):
		cl.enqueue(errFrame("no_own_key"))
		return
	case errors.Is(err, errPeerNoKey):
		cl.enqueue(errFrame("peer_no_key"))
		return
	case errors.Is(err, errReplay):
		cl.enqueue(errFrame("replay"))
		return
	case errors.Is(err, errKeyChanged):
		cl.enqueue(errFrame("key_changed"))
		return
	case errors.Is(err, errBlocked):
		cl.enqueue(errFrame("blocked"))
		return
	case errors.Is(err, errStaffTarget):
		cl.enqueue(errFrame("staff_shield"))
		return
	case errors.Is(err, errGone):
		cl.enqueue(errFrame("no_such_user"))
		return
	case errors.Is(err, errNoFunds):
		cl.enqueue(errFrame("no_credits"))
		return
	case err != nil:
		log.Printf("sending a direct message failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}

	b := frame(dmFrame{T: "dm", dmMsg: m, Fa: senderLook})
	h.mu.Lock()
	for c := range ou.clients {
		c.enqueue(b)
	}
	if tou := h.users[target.ID]; tou != nil {
		for c := range tou.clients {
			c.enqueue(b)
		}
	}
	h.mu.Unlock()
	if charged {
		h.setCredits(ou, bal)
	}
	h.notify(noteIn{To: target.ID, ActorID: ou.id, Mode: "group", Ref: "dm:" + strconv.FormatInt(ou.id, 10),
		Note: note{Kind: "dm", Actor: ou.handle, Body: "Sent you an encrypted message"}})
}

// dmClosedTo says whether this person only takes messages from friends and I am not one. Staff can always reach people.
func (h *hub) dmClosedTo(ctx context.Context, me *onlineUser, otherID int64) bool {
	h.mu.Lock()
	staff := roleRank(me.role) >= 2
	h.mu.Unlock()
	if staff || !h.prefsOf(ctx, otherID).FriendsOnly {
		return false
	}
	rel, err := h.st.relation(ctx, me.id, otherID)
	return err == nil && rel != "friend"
}

// relationFor says how I stand with someone: none, friend, sent, got or blocked.
func (h *hub) relationFor(ctx context.Context, me, other int64) (string, error) {
	if blocked, err := h.st.iBlocked(ctx, me, other); err != nil || blocked {
		return "blocked", err
	}
	return h.st.relation(ctx, me, other)
}

// dmThread opens a conversation: {"t":"thread","h":"bob"}. It marks their messages to me as read.
func (h *hub) dmThread(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	other, ok := h.lookupUser(dctx, cl, in.H)
	if !ok {
		return
	}
	if other.ID == ou.id {
		cl.enqueue(errFrame("self_dm"))
		return
	}
	rel, err := h.relationFor(dctx, ou.id, other.ID)
	var msgs []dmMsg
	if err == nil {
		msgs, err = h.st.threadMessages(dctx, ou.id, other.ID, threadLimit)
	}
	var readTop int64
	if err == nil && !in.NR {
		readTop, err = h.st.markRead(dctx, ou.id, other.ID)
	}
	var looks map[string]string
	if err == nil {
		looks, err = h.st.avatarsFor(dctx, []string{ou.handle, other.Handle})
	}
	if err != nil {
		log.Printf("opening a conversation failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	cost := int64(0)
	h.mu.Lock()
	staff := roleRank(ou.role) >= 2
	h.mu.Unlock()
	if !staff && rel != "blocked" {
		var cerr error
		if cost, cerr = h.st.dmCost(dctx, ou.id, other.ID, dmFee); cerr != nil {
			log.Printf("pricing a conversation failed: %v", cerr)
			cl.enqueue(errFrame("server_busy"))
			return
		}
	}
	// Presence is shown only when neither side blocked the other, and never for a banned account.
	if bm, banned, herr := h.st.hiddenFrom(dctx, ou.id, other.ID); rel != "blocked" && herr == nil && !bm && !banned {
		h.presenceOpen(cl, other.ID, other.Handle)
	} else {
		h.presenceClose(cl)
	}
	tf := threadFrame{T: "thread", H: other.Handle, S: rel, C: cost, M: msgs, Av: looks, Cl: rel != "blocked" && h.dmClosedTo(dctx, ou, other.ID)}
	if k, has, kerr := h.st.keyOf(dctx, other.ID); kerr == nil && has {
		tf.Ek = &k
	}
	if k, has, kerr := h.st.keyOf(dctx, ou.id); kerr == nil && has {
		tf.Mk = &k
	}
	cl.enqueue(frame(tf))
	if !in.NR {
		h.sendReceipt(dctx, ou, other.ID, 3, nil, readTop)
		h.pushBadge(ou)
		h.readRef(ou.id, "dm:"+strconv.FormatInt(other.ID, 10))
	}
}

// dmRead marks a conversation as read without sending it again: {"t":"read","h":"bob"}.
// The page uses it when a message arrives in a conversation that is already open.
func (h *hub) dmRead(ctx context.Context, cl *client, in inbound) {
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	other, ok := h.lookupUser(dctx, cl, in.H)
	if !ok {
		return
	}
	top, err := h.st.markRead(dctx, cl.user.id, other.ID)
	if err != nil {
		log.Printf("marking read failed: %v", err)
		return
	}
	h.sendReceipt(dctx, cl.user, other.ID, 3, nil, top)
	h.pushBadge(cl.user)
	h.readRef(cl.user.id, "dm:"+strconv.FormatInt(other.ID, 10))
}

// dmInbox sends my chats, friends, requests and blocked people: {"t":"inbox"}.
func (h *hub) dmInbox(ctx context.Context, cl *client) {
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	chats, err := h.st.conversations(dctx, cl.user.id)
	var lists friendLists
	if err == nil {
		lists, err = h.st.friendLists(dctx, cl.user.id)
	}
	if err != nil {
		log.Printf("loading the inbox failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	var handles []string
	for _, c := range chats {
		handles = append(handles, c.Handle)
	}
	for _, l := range [][]string{lists.Friends, lists.Incoming, lists.Outgoing, lists.Blocked} {
		handles = append(handles, l...)
	}
	looks, err := h.st.avatarsFor(dctx, handles)
	if err != nil {
		log.Printf("loading avatars failed: %v", err)
		looks = nil
	}
	cl.enqueue(frame(inboxFrame{T: "inbox", C: chats, friendLists: lists, Av: looks}))
}

// badgeFrame is the unread count and the number of waiting friend requests.
func (h *hub) badgeFrame(ctx context.Context, id int64) ([]byte, error) {
	u, q, err := h.st.unreadAndRequests(ctx, id)
	if err != nil {
		return nil, err
	}
	return frame(map[string]any{"t": "ib", "u": u, "q": q}), nil
}

// pushBadge sends a person's current badge to all of their tabs, without making the caller wait.
func (h *hub) pushBadge(ou *onlineUser) {
	safeGo(func() {
		ctx, cancel := context.WithTimeout(context.Background(), authWait)
		defer cancel()
		b, err := h.badgeFrame(ctx, ou.id)
		if err != nil {
			log.Printf("badge failed: %v", err)
			return
		}
		h.sendToUser(ou.id, b)
	})
}

// pushBadgeFor does the same for a person who may not be online. Nothing happens if they are not.
func (h *hub) pushBadgeFor(id int64) {
	h.mu.Lock()
	ou := h.users[id]
	h.mu.Unlock()
	if ou != nil {
		h.pushBadge(ou)
	}
}

// sendBadgeTo gives a connection that just opened its badge.
func (h *hub) sendBadgeTo(cl *client) {
	ctx, cancel := context.WithTimeout(context.Background(), authWait)
	defer cancel()
	if b, err := h.badgeFrame(ctx, cl.user.id); err == nil {
		cl.enqueue(b)
	}
}

var friendErrors = map[error]string{
	errBlocked:     "blocked",
	errStaffTarget: "staff_shield",
	errGone:        "no_such_user",
	errAlready:     "already_friends",
	errNoRequest:   "no_request",
	errFriendCap:   "friend_cap",
	errTooMany:     "too_many_requests",
}

// friendAct handles {"t":"fr","a":"add|accept|decline|cancel|remove","h":"bob"}.
func (h *hub) friendAct(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	switch in.A {
	case "add", "accept", "decline", "cancel", "remove":
	default:
		cl.enqueue(errFrame("bad_action"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	other, ok := h.lookupUser(dctx, cl, in.H)
	if !ok {
		return
	}
	if other.ID == ou.id {
		cl.enqueue(errFrame("self_dm"))
		return
	}
	rel, err := h.st.friendAction(dctx, in.A, ou.id, other.ID)
	if err != nil {
		if code, known := friendErrors[err]; known {
			cl.enqueue(errFrame(code))
			return
		}
		log.Printf("friend action %s failed: %v", in.A, err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	cl.enqueue(frame(map[string]string{"t": "frok", "a": in.A, "h": other.Handle, "s": rel}))
	h.pushBadge(ou)

	// Tell the other person, when it is something they should see.
	event := ""
	switch {
	case in.A == "add" && rel == "sent":
		event = "request"
	case rel == "friend":
		event = "friends" // accepted, or both asked at once
	case in.A == "remove":
		event = "removed"
	}
	if event != "" {
		h.sendToUser(other.ID, frame(map[string]string{"t": "fr", "a": event, "h": ou.handle}))
		h.pushBadgeFor(other.ID)
	}
	if event == "request" || event == "friends" {
		kind := map[string]string{"request": "request", "friends": "accepted"}[event]
		h.notify(noteIn{To: other.ID, ActorID: ou.id, Mode: "refresh", Ref: "fr:" + strconv.FormatInt(ou.id, 10) + ":" + kind,
			Note: note{Kind: "friend", Actor: ou.handle, Body: kind}})
	}
}

// blockAct handles {"t":"blk","a":"add|del","h":"bob"}.
func (h *hub) blockAct(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	if in.A != "add" && in.A != "del" {
		cl.enqueue(errFrame("bad_action"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	other, ok := h.lookupUser(dctx, cl, in.H)
	if !ok {
		return
	}
	if other.ID == ou.id {
		cl.enqueue(errFrame("self_dm"))
		return
	}
	var err error
	if in.A == "add" {
		err = h.st.blockUser(dctx, ou.id, other.ID)
	} else {
		err = h.st.unblockUser(dctx, ou.id, other.ID)
	}
	if err != nil {
		log.Printf("block action failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	cl.enqueue(frame(map[string]string{"t": "blkok", "a": in.A, "h": other.Handle}))
	h.pushBadge(ou)
	if in.A == "add" {
		h.pushBadgeFor(other.ID) // a request from me may have just disappeared from their count
	}
}

// dmReport lets someone report a direct message they received: {"t":"drep","i":42,"c":"abuse"}.
func (h *hub) dmReport(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	if !validReportReason(in.C) {
		cl.enqueue(errFrame("bad_report"))
		return
	}
	now := time.Now()
	h.mu.Lock()
	keep := ou.reportTimes[:0]
	for _, t := range ou.reportTimes {
		if t.After(now.Add(-time.Hour)) {
			keep = append(keep, t)
		}
	}
	ou.reportTimes = keep
	tooMany := len(keep) >= maxReportsPerHour
	h.mu.Unlock()
	if tooMany {
		cl.enqueue(errFrame("slow_down"))
		return
	}

	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	// Legacy plaintext messages (sent before encryption) can still be reported by id. An encrypted message can
	// only be reported by revealing its own message key, so the server never reads a conversation by itself.
	items := in.It
	if len(items) == 0 && in.I != 0 {
		items = []dmReveal{{I: in.I}}
	}
	if len(items) == 0 || len(items) > maxReportItems {
		cl.enqueue(errFrame("bad_report"))
		return
	}
	filed := 0
	var firstErr string
	for _, it := range items {
		code := h.reportOneDM(dctx, cl, it, in.C)
		if code == "" {
			filed++
		} else if firstErr == "" {
			firstErr = code
		}
	}
	if filed == 0 {
		cl.enqueue(errFrame(firstErr))
		return
	}
	h.mu.Lock()
	ou.reportTimes = append(ou.reportTimes, now)
	h.mu.Unlock()
	cl.enqueue(frame(map[string]any{"t": "repok", "n": filed}))
	h.sendReportCount(dctx)
}

// reportOneDM files one selected message. It returns an error code, or "" on success.
func (h *hub) reportOneDM(ctx context.Context, cl *client, it dmReveal, reason string) string {
	ou := cl.user
	row, err := h.st.dmFull(ctx, it.I)
	switch {
	case errors.Is(err, errNotFound) || (err == nil && row.RecipientID != ou.id):
		return "report_gone" // gone, or not a message sent to me
	case err != nil:
		log.Printf("loading a message to report failed: %v", err)
		return "server_busy"
	case row.SenderID == ou.id:
		return "self_report"
	}
	body := row.Body
	if row.Enc == 1 {
		if it.K == "" {
			return "needs_disclosure"
		}
		pt, oerr := openDisclosed(row, it.K)
		if oerr != nil {
			return "bad_disclosure"
		}
		body = pt
	}
	// A message from a staff member goes to the staff complaint channel, which moderators can not read.
	if srole, rerr := h.st.roleOf(ctx, row.SenderID); rerr == nil && isStaff(srole) {
		h.fileComplaint(ctx, cl, complaintRow{TargetID: row.SenderID, TargetHandle: row.SenderHandle, TargetRole: srole, MsgID: it.I,
			Body: "[direct message] " + body, Category: reason, Details: ""})
		return ""
	}
	_, dup, err := h.st.addReport(ctx, reportRow{
		ReporterID: ou.id, ReporterHandle: ou.handle, MsgID: it.I,
		TargetHandle: row.SenderHandle, Body: "[direct message] " + body, Reason: reason,
	})
	if err != nil {
		log.Printf("saving report failed: %v", err)
		return "server_busy"
	}
	if dup {
		return "already_reported"
	}
	return ""
}

// deleteReportedDM lets a moderator remove a direct message that was reported.
// It returns false when the id is not a direct message.
func (h *hub) deleteReportedDM(ctx context.Context, cl *client, in inbound) bool {
	_, senderID, _, _, err := h.st.dmByID(ctx, in.I)
	if errors.Is(err, errNotFound) {
		return false
	}
	if err != nil {
		log.Printf("loading a direct message failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return true
	}
	// Staff may only remove a direct message somebody reported. Private messages are never browsable by id.
	reported, err := h.st.dmIsReported(ctx, in.I)
	if err != nil {
		cl.enqueue(errFrame("server_busy"))
		return true
	}
	h.mu.Lock()
	actorRole := cl.user.role
	h.mu.Unlock()
	srole, rerr := h.st.roleOf(ctx, senderID)
	if !reported || (rerr == nil && !canAct(actorRole, srole)) {
		cl.enqueue(errFrame("forbidden"))
		return true
	}
	handle, err := h.st.deleteDM(ctx, in.I)
	if err != nil {
		log.Printf("deleting a direct message failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return true
	}
	if handle == "" {
		return false
	}
	h.st.logMod(ctx, cl.user.handle, "del", handle, "direct message")
	if in.RID > 0 {
		if _, err := h.st.closeReport(ctx, in.RID, "actioned:del", cl.user.handle); err == nil {
			h.sendReportCount(ctx)
		}
	}
	cl.enqueue(frame(map[string]any{"t": "modok", "a": "del", "i": in.I}))
	return true
}

// rcptFrame tells the sender how far one of their messages got. It carries ids and a status number, nothing else.
type rcptFrame struct {
	T   string  `json:"t"`
	H   string  `json:"h"`             // the recipient
	S   int     `json:"s"`             // 2 delivered, 3 read
	Ids []int64 `json:"ids,omitempty"` // delivered: these messages
	U   int64   `json:"u,omitempty"`   // read: every message of mine up to this id
}

// sendReceipt tells every open tab of the sender. Nothing is sent when nothing changed, or when the reader blocked the sender.
func (h *hub) sendReceipt(ctx context.Context, reader *onlineUser, senderID int64, status int, ids []int64, upto int64) {
	if len(ids) == 0 && upto == 0 {
		return
	}
	if blocked, err := h.st.iBlocked(ctx, reader.id, senderID); err != nil || blocked {
		return
	}
	h.sendToUser(senderID, frame(rcptFrame{T: "rcpt", H: reader.handle, S: status, Ids: ids, U: upto}))
}

// dmAck handles {"t":"dack","h":"bob","ids":[12,13]}: this device received those messages from bob.
// Only the recipient of a message can advance it, so nobody can touch another person's conversation.
func (h *hub) dmAck(ctx context.Context, cl *client, in inbound) {
	ids := make([]int64, 0, len(in.Ids))
	seen := map[int64]struct{}{}
	for _, id := range in.Ids {
		if _, dup := seen[id]; id > 0 && !dup && len(ids) < 100 {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	other, ok := h.lookupUser(dctx, cl, in.H)
	if !ok {
		return
	}
	changed, err := h.st.markDelivered(dctx, cl.user.id, other.ID, ids)
	if err != nil {
		log.Printf("marking delivered failed: %v", err)
		return
	}
	h.sendReceipt(dctx, cl.user, other.ID, 2, changed, 0)
}
