package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/lib/pq"
)

// Step 19: pinned room message, message reactions and room polls.
//
// Rules shared by all three:
//   - Everything is checked here on the server. The page only hides buttons.
//   - Nothing in this file calls ou.score, awards credits, touches tiers or stores a message: none of it earns anything.
//   - Every change is written to the database first and only then sent to the room, so what people see is what a
//     refresh or a reconnect will show.
//   - Changes in one room are serialized by rm.engMu (database call included), so two people acting at once can never
//     leave the room with counts or a pin that differ from the database. The hub lock is never held during a query.
//
// Who may manage (pin, unpin, create and close polls): the host of a community room, and moderators and admins
// everywhere. Official rooms therefore have staff only. The platform has no per-room moderator role.

const (
	pollQuestionMin = 3
	pollQuestionMax = 140
	pollOptionsMin  = 2
	pollOptionsMax  = 6
	pollOptionMax   = 60
	engageBurst     = 12 // reactions and votes one person may send in engageWindow
	engageWindow    = 5 * time.Second
	duplicateWindow = 10 * time.Second // an identical poll request this soon after the first is the same request
)

// reactionSet is the whole list of reactions. The server refuses anything else.
var reactionSet = []string{"👍", "❤️", "😂", "😮", "😢", "🙏"}

// canonReaction maps what a page sent (with or without the emoji variation selector) to the stored form.
func canonReaction(s string) (string, bool) {
	s = strings.ReplaceAll(s, "\uFE0F", "")
	for _, r := range reactionSet {
		if strings.ReplaceAll(r, "\uFE0F", "") == s {
			return r, true
		}
	}
	return "", false
}

type pinState struct {
	MsgID  int64
	Handle string
	Body   string
	TS     int64
	By     string
}

type pollState struct {
	ID     int64
	Q      string
	Opts   []string
	By     string
	ByID   int64
	Closed bool
	TS     int64
	Votes  map[int64]int // account -> option
	Counts []int
}

type engageState struct {
	engMu sync.Mutex // serializes changes to pin, reactions and poll in one room
	pin   *pinState
	poll  *pollState
}

func (p *pinState) json() map[string]any {
	if p == nil {
		return nil
	}
	return map[string]any{"i": p.MsgID, "h": p.Handle, "b": p.Body, "ts": p.TS, "by": p.By}
}

// pollJSON is the poll as one person sees it. y is their own vote, -1 when they have not voted.
func (p *pollState) json(uid int64) map[string]any {
	if p == nil {
		return nil
	}
	y := -1
	if v, ok := p.Votes[uid]; ok {
		y = v
	}
	total := 0
	for _, n := range p.Counts {
		total += n
	}
	return map[string]any{"id": p.ID, "q": p.Q, "o": p.Opts, "n": p.Counts, "t": total, "c": p.Closed, "by": p.By, "ts": p.TS, "y": y}
}

// canManageRoomLocked: the host of a community room, or staff. The caller holds the hub lock.
func (h *hub) canManageRoomLocked(ou *onlineUser, rm *room) bool {
	return roleRank(ou.role) >= 2 || (rm.community() && rm.ownerID == ou.id)
}

// engageGateLocked is the common door for reactions and votes: be in the room, be a member of it, not muted, not flooding.
func (h *hub) engageGateLocked(cl *client) (*room, string) {
	ou, rm := cl.user, cl.room
	if rm == nil {
		return nil, "no_room"
	}
	now := time.Now()
	if now.Before(ou.mutedUntil) {
		return nil, "muted"
	}
	if until, ok := rm.muted[ou.id]; ok && now.Before(until) {
		return nil, "room_muted"
	}
	if rm.community() {
		if _, joined := ou.joined[rm.slug]; !joined && rm.ownerID != ou.id && roleRank(ou.role) < 2 {
			return nil, "not_member"
		}
	}
	cut := now.Add(-engageWindow)
	kept := ou.engTimes[:0]
	for _, t := range ou.engTimes {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	ou.engTimes = kept
	if len(kept) >= engageBurst {
		return nil, "slow_down"
	}
	ou.engTimes = append(ou.engTimes, now)
	return rm, ""
}

func (h *hub) engageSendLocked(rm *room, v any) {
	b := frame(v)
	for c := range rm.clients {
		c.enqueue(b)
	}
}

// ---- store ----

// createPoll stores a new poll. The options go in as TEXT with an explicit cast to jsonb. They used to go in as a Go
// []byte, which lib/pq sends as raw bytes when the connection string contains binary_parameters=yes (a common
// setting behind a connection pooler). Postgres then reads those bytes as binary jsonb and refuses them with
// "unsupported jsonb version number 91", so every poll creation failed. Text works in both modes.
func (s *store) createPoll(ctx context.Context, room, question string, options []string, userID int64, handle string) (id int64, created time.Time, err error) {
	raw, err := json.Marshal(options)
	if err != nil {
		return 0, time.Time{}, err
	}
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO polls (room, question, options, created_by, created_by_handle) VALUES ($1,$2,$3::jsonb,$4,$5) RETURNING id, created_at`,
		room, question, string(raw), userID, handle).Scan(&id, &created)
	return id, created, err
}

// pollFailCode turns a database failure into the code the person sees, and says what happened in the log. Only a
// failure that may pass by itself (connection trouble, timeouts, overload) is "server_busy". A duplicate open poll
// is "poll_open". Everything else (a missing table, a refused permission, a bad value) is a fault that retrying will
// not fix, so it is "poll_failed" and the log line names the exact database error.
func pollFailCode(err error) (code, detail string) {
	var pe *pq.Error
	switch {
	case errors.As(err, &pe):
		detail = "sqlstate=" + string(pe.Code) + " " + pe.Message
		switch {
		case pe.Code == "23505":
			return "poll_open", detail
		case pe.Code.Class() == "08" || pe.Code.Class() == "53" || pe.Code.Class() == "57" || pe.Code.Class() == "40":
			return "server_busy", detail
		}
		return "poll_failed", detail
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled), errors.Is(err, sql.ErrConnDone), errors.Is(err, driver.ErrBadConn):
		return "server_busy", err.Error()
	}
	return "poll_failed", err.Error()
}

func (s *store) setPin(ctx context.Context, room string, m chatMsg, by string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO room_pins (room, msg_id, handle, body, msg_ts, pinned_by) VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (room) DO UPDATE SET msg_id = EXCLUDED.msg_id, handle = EXCLUDED.handle, body = EXCLUDED.body,
		   msg_ts = EXCLUDED.msg_ts, pinned_by = EXCLUDED.pinned_by, pinned_at = now()`,
		room, m.ID, m.Handle, m.Body, m.TS, by)
	return err
}

func (s *store) clearPin(ctx context.Context, room string, msgID int64) error {
	if msgID == 0 {
		_, err := s.db.ExecContext(ctx, `DELETE FROM room_pins WHERE room = $1`, room)
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM room_pins WHERE room = $1 AND msg_id = $2`, room, msgID)
	return err
}

// setReaction adds, changes or (emoji "") removes one person's reaction. changed is false when nothing differed.
func (s *store) setReaction(ctx context.Context, room string, msgID, userID int64, emoji string) (bool, error) {
	var res sql.Result
	var err error
	if emoji == "" {
		res, err = s.db.ExecContext(ctx, `DELETE FROM message_reactions WHERE msg_id = $1 AND user_id = $2`, msgID, userID)
	} else {
		res, err = s.db.ExecContext(ctx,
			`INSERT INTO message_reactions (msg_id, room, user_id, emoji) VALUES ($1,$2,$3,$4)
			 ON CONFLICT (msg_id, user_id) DO UPDATE SET emoji = EXCLUDED.emoji, created_at = now()
			 WHERE message_reactions.emoji <> EXCLUDED.emoji`, msgID, room, userID, emoji)
	}
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *store) reactionCounts(ctx context.Context, msgID int64) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT emoji, count(*) FROM message_reactions WHERE msg_id = $1 GROUP BY emoji`, msgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var e string
		var n int
		if err := rows.Scan(&e, &n); err != nil {
			return nil, err
		}
		out[e] = n
	}
	return out, rows.Err()
}

type rxInfo struct {
	C map[string]int `json:"c"`
	Y string         `json:"y,omitempty"`
}

// reactionsFor loads the reactions of many messages with one query. mine is the asking person's own reaction.
func (s *store) reactionsFor(ctx context.Context, room string, ids []int64, userID int64) (map[string]*rxInfo, error) {
	out := map[string]*rxInfo{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT msg_id, emoji, count(*), bool_or(user_id = $3) FROM message_reactions
		 WHERE room = $1 AND msg_id = ANY($2) GROUP BY msg_id, emoji`, room, pq.Array(ids), userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var e string
		var n int
		var mine bool
		if err := rows.Scan(&id, &e, &n, &mine); err != nil {
			return nil, err
		}
		k := strconv.FormatInt(id, 10)
		if out[k] == nil {
			out[k] = &rxInfo{C: map[string]int{}}
		}
		out[k].C[e] = n
		if mine {
			out[k].Y = e
		}
	}
	return out, rows.Err()
}

func (s *store) deleteMessageEngagement(ctx context.Context, msgID int64) {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM message_reactions WHERE msg_id = $1`, msgID); err != nil {
		log.Printf("removing reactions of a deleted message failed: %v", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM room_pins WHERE msg_id = $1`, msgID); err != nil {
		log.Printf("removing the pin of a deleted message failed: %v", err)
	}
}

// loadEngagement reads pins and the latest poll of every room into memory. It runs at start, and again after an
// account deletion removed votes.
func (h *hub) loadEngagement(ctx context.Context) error {
	pins := map[string]*pinState{}
	rows, err := h.st.db.QueryContext(ctx, `SELECT room, msg_id, handle, body, msg_ts, pinned_by FROM room_pins`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var room string
		p := &pinState{}
		if err := rows.Scan(&room, &p.MsgID, &p.Handle, &p.Body, &p.TS, &p.By); err != nil {
			rows.Close()
			return err
		}
		pins[room] = p
	}
	rows.Close()

	polls := map[string]*pollState{}
	byID := map[int64]*pollState{}
	var ids []int64
	prow, err := h.st.db.QueryContext(ctx,
		`SELECT DISTINCT ON (room) room, id, question, options, created_by, created_by_handle, (extract(epoch from created_at) * 1000)::bigint, closed_at IS NOT NULL
		 FROM polls ORDER BY room, (closed_at IS NULL) DESC, id DESC`)
	if err != nil {
		return err
	}
	for prow.Next() {
		var room string
		var raw []byte
		p := &pollState{Votes: map[int64]int{}}
		if err := prow.Scan(&room, &p.ID, &p.Q, &raw, &p.ByID, &p.By, &p.TS, &p.Closed); err != nil {
			prow.Close()
			return err
		}
		if err := json.Unmarshal(raw, &p.Opts); err != nil {
			prow.Close()
			return err
		}
		p.Counts = make([]int, len(p.Opts))
		polls[room], byID[p.ID] = p, p
		ids = append(ids, p.ID)
	}
	prow.Close()
	if len(ids) > 0 {
		vrows, err := h.st.db.QueryContext(ctx, `SELECT poll_id, user_id, opt FROM poll_votes WHERE poll_id = ANY($1)`, pq.Array(ids))
		if err != nil {
			return err
		}
		for vrows.Next() {
			var pid, uid int64
			var opt int
			if err := vrows.Scan(&pid, &uid, &opt); err != nil {
				vrows.Close()
				return err
			}
			if p := byID[pid]; p != nil && opt < len(p.Counts) {
				p.Votes[uid] = opt
				p.Counts[opt]++
			}
		}
		vrows.Close()
	}
	h.mu.Lock()
	for _, rm := range h.roomList {
		rm.pin, rm.poll = pins[rm.slug], polls[rm.slug]
	}
	h.mu.Unlock()
	return nil
}

// ---- joining a room: one small frame with the pin, the poll and the reactions of the messages just sent ----

// sendEngagement queues {"t":"x19"} for a client that just entered a room. Pin and poll come from memory, the
// reactions of the messages in the history snapshot from one query.
func (h *hub) sendEngagement(ctx context.Context, cl *client, last int64) {
	h.mu.Lock()
	rm := cl.room
	if rm == nil {
		h.mu.Unlock()
		return
	}
	uid := cl.user.id
	pin, poll := rm.pin.json(), rm.poll.json(uid)
	hist := h.history(rm, last)
	ids := make([]int64, 0, len(hist))
	for _, m := range hist {
		ids = append(ids, m.ID)
	}
	slug := rm.slug
	h.mu.Unlock()
	rx, err := h.st.reactionsFor(ctx, slug, ids, uid)
	if err != nil {
		log.Printf("loading reactions failed: %v", err)
		rx = map[string]*rxInfo{}
	}
	cl.enqueue(frame(map[string]any{"t": "x19", "r": slug, "pin": pin, "poll": poll, "rx": rx}))
}

// ---- pins ----

func (h *hub) pinMsg(ctx context.Context, cl *client, in inbound) {
	h.mu.Lock()
	rm := cl.room
	if rm == nil {
		h.mu.Unlock()
		cl.enqueue(errFrame("no_room"))
		return
	}
	if !h.canManageRoomLocked(cl.user, rm) {
		h.mu.Unlock()
		cl.enqueue(errFrame("forbidden"))
		return
	}
	m := findMsg(rm, in.I)
	h.mu.Unlock()
	if m == nil {
		cl.enqueue(errFrame("msg_gone"))
		return
	}
	if m.TipAmt > 0 {
		cl.enqueue(errFrame("forbidden")) // a tip card is the record of a payment, not an announcement
		return
	}
	rm.engMu.Lock()
	defer rm.engMu.Unlock()
	h.mu.Lock()
	_, gone := h.deleted[m.ID]
	same := rm.pin != nil && rm.pin.MsgID == m.ID
	h.mu.Unlock()
	if gone {
		cl.enqueue(errFrame("msg_gone"))
		return
	}
	if same {
		return // already pinned: no second pin, no repeated notice
	}
	if err := h.st.setPin(ctx, rm.slug, *m, cl.user.handle); err != nil {
		log.Printf("pinning failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	h.mu.Lock()
	if _, gone := h.deleted[m.ID]; gone { // deleted while the pin was being saved
		h.mu.Unlock()
		_ = h.st.clearPin(ctx, rm.slug, m.ID)
		cl.enqueue(errFrame("msg_gone"))
		return
	}
	rm.pin = &pinState{MsgID: m.ID, Handle: m.Handle, Body: m.Body, TS: m.TS, By: cl.user.handle}
	h.engageSendLocked(rm, map[string]any{"t": "pin", "p": rm.pin.json()})
	h.mu.Unlock()
}

func (h *hub) unpinMsg(ctx context.Context, cl *client, in inbound) {
	h.mu.Lock()
	rm := cl.room
	if rm == nil {
		h.mu.Unlock()
		cl.enqueue(errFrame("no_room"))
		return
	}
	if !h.canManageRoomLocked(cl.user, rm) {
		h.mu.Unlock()
		cl.enqueue(errFrame("forbidden"))
		return
	}
	h.mu.Unlock()
	rm.engMu.Lock()
	defer rm.engMu.Unlock()
	h.mu.Lock()
	had := rm.pin != nil
	h.mu.Unlock()
	if !had {
		return // nothing pinned: nothing to do
	}
	if err := h.st.clearPin(ctx, rm.slug, 0); err != nil {
		log.Printf("unpinning failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	h.mu.Lock()
	rm.pin = nil
	h.engageSendLocked(rm, map[string]any{"t": "pin", "p": nil})
	h.mu.Unlock()
}

// unpinDeletedLocked is called, with the hub lock held, when a message was removed from a room. It forgets the pin
// when that message was the pinned one and tells the room. The database row goes in deleteMessageEngagement.
func (h *hub) unpinDeletedLocked(rm *room, msgID int64) {
	if rm != nil && rm.pin != nil && rm.pin.MsgID == msgID {
		rm.pin = nil
		h.engageSendLocked(rm, map[string]any{"t": "pin", "p": nil})
	}
}

// ---- reactions ----

func (h *hub) react(ctx context.Context, cl *client, in inbound) {
	if in.Em == nil {
		cl.enqueue(errFrame("bad_emoji"))
		return
	}
	emoji := ""
	if *in.Em != "" {
		var ok bool
		if emoji, ok = canonReaction(*in.Em); !ok {
			cl.enqueue(errFrame("bad_emoji"))
			return
		}
	}
	h.mu.Lock()
	rm, code := h.engageGateLocked(cl)
	if code != "" {
		h.mu.Unlock()
		cl.enqueue(errFrame(code))
		return
	}
	m := findMsg(rm, in.I)
	h.mu.Unlock()
	if m == nil {
		cl.enqueue(errFrame("msg_gone")) // too old, deleted, or never in this room
		return
	}
	rm.engMu.Lock()
	defer rm.engMu.Unlock()
	h.mu.Lock()
	_, gone := h.deleted[m.ID]
	h.mu.Unlock()
	if gone {
		cl.enqueue(errFrame("msg_gone"))
		return
	}
	changed, err := h.st.setReaction(ctx, rm.slug, m.ID, cl.user.id, emoji)
	if err == nil {
		var counts map[string]int
		if counts, err = h.st.reactionCounts(ctx, m.ID); err == nil {
			h.mu.Lock()
			if _, gone := h.deleted[m.ID]; gone {
				h.mu.Unlock()
				h.st.deleteMessageEngagement(ctx, m.ID)
				cl.enqueue(errFrame("msg_gone"))
				return
			}
			f := map[string]any{"t": "rx", "i": m.ID, "c": counts, "h": cl.user.handle, "y": emoji}
			if changed {
				h.engageSendLocked(rm, f)
			} else {
				cl.enqueue(frame(f)) // nothing changed: only the asker needs the current numbers
			}
			h.mu.Unlock()
			return
		}
	}
	log.Printf("saving a reaction failed: %v", err)
	cl.enqueue(errFrame("server_busy"))
}

// ---- polls ----

// cleanPoll checks a question and its options. The page checks the same limits for a quicker answer.
func cleanPoll(q string, opts []string) (string, []string, bool) {
	q = strings.Join(strings.Fields(clean(q)), " ")
	if n := utf8.RuneCountInString(q); n < pollQuestionMin || n > pollQuestionMax {
		return "", nil, false
	}
	if len(opts) < pollOptionsMin || len(opts) > pollOptionsMax {
		return "", nil, false
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(opts))
	for _, o := range opts {
		o = strings.Join(strings.Fields(clean(o)), " ")
		n := utf8.RuneCountInString(o)
		if n < 1 || n > pollOptionMax || seen[strings.ToLower(o)] {
			return "", nil, false
		}
		seen[strings.ToLower(o)] = true
		out = append(out, o)
	}
	return q, out, true
}

func (h *hub) pollCreate(ctx context.Context, cl *client, in inbound) {
	q, opts, ok := cleanPoll(in.B, in.Os)
	if !ok {
		cl.enqueue(errFrame("bad_poll"))
		return
	}
	h.mu.Lock()
	rm := cl.room
	if rm == nil {
		h.mu.Unlock()
		cl.enqueue(errFrame("no_room"))
		return
	}
	if !h.canManageRoomLocked(cl.user, rm) {
		h.mu.Unlock()
		cl.enqueue(errFrame("forbidden"))
		return
	}
	h.mu.Unlock()
	rm.engMu.Lock()
	defer rm.engMu.Unlock()
	h.mu.Lock()
	cur := rm.poll
	open := cur != nil && !cur.Closed
	// The same request twice (a double tap, a retry after a slow answer) is one poll, not an error and not two polls:
	// the same person asking for the same question and options within a few seconds just gets the poll again.
	if open && cur.ByID == cl.user.id && cur.Q == q && sameStrings(cur.Opts, opts) && time.Since(time.UnixMilli(cur.TS)) < duplicateWindow {
		cl.enqueue(frame(map[string]any{"t": "poll", "p": cur.json(cl.user.id)}))
		h.mu.Unlock()
		return
	}
	h.mu.Unlock()
	if open {
		cl.enqueue(errFrame("poll_open")) // close the running poll first
		return
	}
	p := &pollState{Q: q, Opts: opts, By: cl.user.handle, ByID: cl.user.id, Votes: map[int64]int{}, Counts: make([]int, len(opts))}
	id, created, err := h.st.createPoll(ctx, rm.slug, q, opts, cl.user.id, cl.user.handle)
	if err != nil {
		code, detail := pollFailCode(err)
		log.Printf("creating a poll in %s by @%s failed (%s): %s", rm.slug, cl.user.handle, code, detail)
		cl.enqueue(errFrame(code))
		return
	}
	p.ID, p.TS = id, created.UnixMilli()
	h.mu.Lock()
	rm.poll = p
	for c := range rm.clients {
		c.enqueue(frame(map[string]any{"t": "poll", "p": p.json(c.user.id)}))
	}
	h.mu.Unlock()
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (h *hub) pollVote(ctx context.Context, cl *client, in inbound) {
	if in.Opt == nil || in.PID <= 0 {
		cl.enqueue(errFrame("bad_poll"))
		return
	}
	h.mu.Lock()
	rm, code := h.engageGateLocked(cl)
	h.mu.Unlock()
	if code != "" {
		cl.enqueue(errFrame(code))
		return
	}
	rm.engMu.Lock()
	defer rm.engMu.Unlock()
	h.mu.Lock()
	p := rm.poll
	switch {
	case p == nil || p.ID != in.PID:
		h.mu.Unlock()
		cl.enqueue(errFrame("no_poll")) // another room's poll, an old one, or a made up id
		return
	case p.Closed:
		h.mu.Unlock()
		cl.enqueue(errFrame("poll_closed"))
		return
	case *in.Opt < 0 || *in.Opt >= len(p.Opts):
		h.mu.Unlock()
		cl.enqueue(errFrame("bad_poll"))
		return
	}
	pid := p.ID
	h.mu.Unlock()
	// The vote only lands while the poll is still open in the database, and one row per account makes a second
	// vote a change of choice, never a second vote.
	res, err := h.st.db.ExecContext(ctx,
		`INSERT INTO poll_votes (poll_id, user_id, opt)
		 SELECT $1, $2, $3 WHERE EXISTS (SELECT 1 FROM polls WHERE id = $1 AND room = $4 AND closed_at IS NULL)
		 ON CONFLICT (poll_id, user_id) DO UPDATE SET opt = EXCLUDED.opt, voted_at = now()`,
		pid, cl.user.id, *in.Opt, rm.slug)
	if err != nil {
		log.Printf("saving a vote failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		cl.enqueue(errFrame("poll_closed"))
		return
	}
	h.mu.Lock()
	if rm.poll == p {
		if old, had := p.Votes[cl.user.id]; had && old < len(p.Counts) {
			p.Counts[old]--
		}
		p.Votes[cl.user.id] = *in.Opt
		p.Counts[*in.Opt]++
		for c := range rm.clients {
			c.enqueue(frame(map[string]any{"t": "poll", "p": p.json(c.user.id)})) // each person gets their own choice back
		}
	}
	h.mu.Unlock()
}

func (h *hub) pollClose(ctx context.Context, cl *client, in inbound) {
	h.mu.Lock()
	rm := cl.room
	if rm == nil {
		h.mu.Unlock()
		cl.enqueue(errFrame("no_room"))
		return
	}
	if !h.canManageRoomLocked(cl.user, rm) {
		h.mu.Unlock()
		cl.enqueue(errFrame("forbidden"))
		return
	}
	h.mu.Unlock()
	rm.engMu.Lock()
	defer rm.engMu.Unlock()
	h.mu.Lock()
	p := rm.poll
	h.mu.Unlock()
	if p == nil || p.ID != in.PID {
		cl.enqueue(errFrame("no_poll"))
		return
	}
	if p.Closed {
		return // closing twice is harmless
	}
	if _, err := h.st.db.ExecContext(ctx, `UPDATE polls SET closed_at = now(), closed_by = $3 WHERE id = $1 AND room = $2 AND closed_at IS NULL`, p.ID, rm.slug, cl.user.handle); err != nil {
		log.Printf("closing a poll failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	h.mu.Lock()
	p.Closed = true
	for c := range rm.clients {
		c.enqueue(frame(map[string]any{"t": "poll", "p": p.json(c.user.id)}))
	}
	h.mu.Unlock()
}
