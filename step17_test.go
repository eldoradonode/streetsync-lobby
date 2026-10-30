package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// quiet fails when a frame of the type arrives within d.
func (w *wsc) quiet(typ string, d time.Duration) {
	w.t.Helper()
	tm := time.After(d)
	for {
		select {
		case f := <-w.frames:
			if f["t"] == typ {
				w.t.Fatalf("unexpected %q frame: %v", typ, f)
			}
		case <-tm:
			return
		}
	}
}

func (w *wsc) status(other string, id int64) int {
	w.t.Helper()
	w.drain()
	w.send(map[string]any{"t": "thread", "h": other, "nr": true})
	for _, x := range w.expect("thread")["m"].([]any) {
		m := x.(map[string]any)
		if int64(m["i"].(float64)) == id {
			if s, ok := m["st"].(float64); ok {
				return int(s)
			}
		}
	}
	w.t.Fatalf("message %d not in the thread", id)
	return 0
}

func (e *testEnv) dmPair(a, b string) (*wsc, *wsc) {
	e.t.Helper()
	ua, ub := e.account(a, "user"), e.account(b, "user")
	e.st.applyCredit(context.Background(), ua.ID, "grant", 50, "g-"+a, "")
	e.st.applyCredit(context.Background(), ub.ID, "grant", 50, "g-"+b, "")
	e.clientFor(a)
	e.clientFor(b)
	return e.login(a), e.login(b)
}

func TestDMReceiptsLifecycle(t *testing.T) {
	e := newTestEnv(t)
	wa, wb := e.dmPair("t_ra", "t_rb")
	idB := mustID(t, e, "t_rb")

	// 1. Accepted by the server: one tick, and the recipient's frame leaks nothing extra.
	wa.drain()
	wb.drain()
	wa.send(e.dmFrame("t_ra", "t_rb", "hello receipts"))
	sent := wa.expect("dm")
	id := int64(sent["i"].(float64))
	if sent["st"] != float64(1) {
		t.Fatalf("a new message must be at status 1: %v", sent)
	}
	wb.expect("dm")
	if got := wa.status("t_rb", id); got != 1 {
		t.Fatalf("not acknowledged yet, want 1 got %d", got)
	}

	// 2. The recipient's device confirms: delivered, and the sender hears about it live.
	wb.send(map[string]any{"t": "dack", "h": "t_ra", "ids": []int64{id}})
	r := wa.expect("rcpt")
	if r["s"] != float64(2) || r["h"] != "t_rb" || len(r["ids"].([]any)) != 1 {
		t.Fatalf("delivered receipt: %v", r)
	}
	// Receipts carry ids and a status only.
	for k := range r {
		if k != "t" && k != "h" && k != "s" && k != "ids" && k != "u" {
			t.Fatalf("receipt has an extra field %q", k)
		}
	}
	if got := wa.status("t_rb", id); got != 2 {
		t.Fatalf("want delivered, got %d", got)
	}

	// 3. A repeated acknowledgement changes nothing and sends nothing.
	wa.drain()
	wb.send(map[string]any{"t": "dack", "h": "t_ra", "ids": []int64{id, id}})
	wa.quiet("rcpt", 400*time.Millisecond)

	// 4. Opening the conversation visibly reads it, live for the sender.
	wb.send(map[string]any{"t": "thread", "h": "t_ra"})
	r = wa.expect("rcpt")
	if r["s"] != float64(3) || int64(r["u"].(float64)) != id {
		t.Fatalf("read receipt: %v", r)
	}
	if got := wa.status("t_rb", id); got != 3 {
		t.Fatalf("want read, got %d", got)
	}

	// 5. A late delivery receipt after read cannot move the status back, and sends nothing.
	wa.drain()
	wb.send(map[string]any{"t": "dack", "h": "t_ra", "ids": []int64{id}})
	wa.quiet("rcpt", 400*time.Millisecond)
	if got := wa.status("t_rb", id); got != 3 {
		t.Fatalf("a late receipt downgraded the status to %d", got)
	}
	// Reading again is a no-op too.
	wb.send(map[string]any{"t": "read", "h": "t_ra"})
	wa.quiet("rcpt", 400*time.Millisecond)

	// 6. A reconnect keeps the persisted status.
	wa2 := e.login("t_ra")
	if got := wa2.status("t_rb", id); got != 3 {
		t.Fatalf("after reconnect want 3 got %d", got)
	}
	_ = idB
}

func TestDMReceiptsOfflineAndHiddenAndUnauthorized(t *testing.T) {
	e := newTestEnv(t)
	wa, wb := e.dmPair("t_oa", "t_ob")
	wc := e.login(e.account("t_oc", "user").Handle)
	e.clientFor("t_oc")

	// Recipient offline: nothing is delivered or read.
	wb.send(map[string]any{"t": "bye"})
	time.Sleep(200 * time.Millisecond)
	wa.drain()
	wa.send(e.dmFrame("t_oa", "t_ob", "while you were away"))
	id := int64(wa.expect("dm")["i"].(float64))
	var d, rd *time.Time
	e.st.db.QueryRow(`SELECT delivered_at, read_at FROM dm_messages WHERE id = $1`, id).Scan(&d, &rd)
	if d != nil || rd != nil {
		t.Fatal("an offline recipient must not cause delivered or read")
	}
	if got := wa.status("t_ob", id); got != 1 {
		t.Fatalf("want 1 got %d", got)
	}

	// The recipient returns and opens the conversation while the page is hidden: nothing is read.
	wb2 := e.login("t_ob")
	wb2.send(map[string]any{"t": "thread", "h": "t_oa", "nr": true})
	th := wb2.expect("thread")
	if st := th["m"].([]any)[0].(map[string]any)["st"]; st != float64(1) {
		t.Fatalf("hidden open must not read: %v", st)
	}
	if e.count(`SELECT count(*) FROM dm_messages WHERE id = $1 AND read_at IS NOT NULL`, id) != 0 {
		t.Fatal("a hidden page marked the message read")
	}
	// It confirms delivery, still not read.
	wa.drain()
	wb2.send(map[string]any{"t": "dack", "h": "t_oa", "ids": []int64{id}})
	if r := wa.expect("rcpt"); r["s"] != float64(2) {
		t.Fatalf("want delivered: %v", r)
	}
	if got := wa.status("t_ob", id); got != 2 {
		t.Fatalf("want 2 got %d", got)
	}

	// Unauthorized: a third person, the sender, and unknown ids cannot advance anything.
	id2 := func() int64 {
		wa.send(e.dmFrame("t_oa", "t_ob", "second"))
		return int64(wa.expect("dm")["i"].(float64))
	}()
	wc.send(map[string]any{"t": "dack", "h": "t_oa", "ids": []int64{id2}})
	wc.send(map[string]any{"t": "dack", "h": "t_ob", "ids": []int64{id2}})
	wc.send(map[string]any{"t": "read", "h": "t_oa"})
	wa.send(map[string]any{"t": "dack", "h": "t_ob", "ids": []int64{id2}}) // the sender acking their own message
	wa.send(map[string]any{"t": "dack", "h": "t_ob", "ids": []int64{999999999, -5, 0}})
	time.Sleep(400 * time.Millisecond)
	if e.count(`SELECT count(*) FROM dm_messages WHERE id = $1 AND (delivered_at IS NOT NULL OR read_at IS NOT NULL)`, id2) != 0 {
		t.Fatal("someone other than the recipient advanced a message")
	}
	// A bad frame never breaks the connection.
	wa.send(map[string]any{"t": "dack", "h": "nobody_here_x", "ids": []int64{1}})
	if got := wa.status("t_ob", id2); got != 1 {
		t.Fatalf("want 1 got %d", got)
	}
}

func TestDMReceiptsNotSentToBlockedSender(t *testing.T) {
	e := newTestEnv(t)
	wa, wb := e.dmPair("t_ba", "t_bb")
	wa.send(e.dmFrame("t_ba", "t_bb", "x"))
	id := int64(wa.expect("dm")["i"].(float64))
	wb.expect("dm")
	if got := wb.ask(map[string]any{"t": "blk", "a": "add", "h": "t_ba"}, "blkok"); got != "ok:blkok" {
		t.Fatal(got)
	}
	wa.drain()
	wb.send(map[string]any{"t": "dack", "h": "t_ba", "ids": []int64{id}})
	wa.quiet("rcpt", 400*time.Millisecond)
}

func TestRoomPresenceNotices(t *testing.T) {
	old := graceLeave
	graceLeave = 300 * time.Millisecond
	defer func() { graceLeave = old }()
	e := newTestEnv(t)
	e.account("t_pa", "user")
	e.account("t_pb", "user")
	wa := e.login("t_pa")
	wa.join("lobby")
	wa.drain()
	pointsBefore := e.count(`SELECT points FROM users WHERE handle_key = 't_pa'`)
	msgsBefore := e.count(`SELECT count(*) FROM messages`)

	wb := e.login("t_pb")
	wb.join("lobby")
	if f := wa.expect("rs"); f["a"] != "j" || f["h"] != "t_pb" {
		t.Fatalf("join notice: %v", f)
	}
	// A second tab of the same person makes no second notice and no wrong count.
	wb2 := e.login("t_pb")
	wb2.join("lobby")
	wa.quiet("rs", 300*time.Millisecond)
	e.h.tick()
	if n := wa.expect("n"); n["c"] != float64(2) {
		t.Fatalf("count with two people on three connections: %v", n)
	}
	// One of the two tabs goes away: still here, no notice.
	wb2.send(map[string]any{"t": "bye"})
	wa.quiet("rs", 500*time.Millisecond)

	// The last tab drops: the person leaves the count at once, the notice comes only after the grace period.
	wb.c.CloseNow()
	time.Sleep(100 * time.Millisecond)
	if got := func() int { e.h.mu.Lock(); defer e.h.mu.Unlock(); return e.h.roomUsers(e.h.rooms["lobby"]) }(); got != 1 {
		t.Fatalf("a disconnected client stayed in the room registry: %d", got)
	}
	wa.quiet("rs", 100*time.Millisecond)
	if f := wa.expect("rs"); f["a"] != "l" || f["h"] != "t_pb" {
		t.Fatalf("leave notice: %v", f)
	}

	// Coming back inside the grace period makes no notice at all.
	wb3 := e.login("t_pb")
	wb3.join("lobby")
	wa.expect("rs") // joined again (their notice had already gone out)
	wb3.c.CloseNow()
	time.Sleep(100 * time.Millisecond)
	wb4 := e.login("t_pb")
	wb4.join("lobby")
	wa.quiet("rs", 700*time.Millisecond)

	// The Leave room button is immediate and a repeat sends nothing more.
	wb4.send(map[string]any{"t": "leave", "a": "x"})
	if f := wa.expect("rs"); f["a"] != "l" {
		t.Fatalf("explicit leave: %v", f)
	}
	wb4.send(map[string]any{"t": "leave", "a": "x"})
	wa.quiet("rs", 400*time.Millisecond)

	// Notices are not messages and earn nothing.
	if e.count(`SELECT count(*) FROM messages`) != msgsBefore {
		t.Fatal("a notice was stored as a message")
	}
	if e.count(`SELECT points FROM users WHERE handle_key = 't_pa'`) != pointsBefore {
		t.Fatal("a notice earned points")
	}
	e.h.mu.Lock()
	for _, m := range e.h.rooms["lobby"].buf {
		if strings.Contains(m.Body, "joined") || strings.Contains(m.Body, "left") {
			t.Error("a notice is in the room history")
		}
	}
	e.h.mu.Unlock()
}

func TestRoomLeaveMembershipAndFind(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	host := e.account("t_lh", "user")
	e.account("t_lm", "user")
	e.st.db.Exec(`UPDATE users SET points = 500 WHERE id = $1`, host.ID)
	e.st.applyCredit(ctx, host.ID, "grant", 100, "g-lh", "")
	wh, wm := e.login("t_lh"), e.login("t_lm")
	wh.send(map[string]any{"t": "croom", "n": "Leave Test Room", "k": "public"})
	slug := wh.expect("croomok")["s"].(string)

	// Search echoes the words, finds partial and mixed case, and never lists unlisted rooms.
	wm.drain()
	wm.send(map[string]any{"t": "find", "n": "LEAVE test"})
	f := wm.expect("found")
	if f["q"] != "leave test" || len(f["r"].([]any)) != 1 {
		t.Fatalf("find: %v", f)
	}
	wm.send(map[string]any{"t": "find", "n": "zzzz-none"})
	if f := wm.expect("found"); f["q"] != "zzzz-none" || len(f["r"].([]any)) != 0 {
		t.Fatalf("empty find: %v", f)
	}

	// Join, then leave: membership follows the server, a second leave is refused, a non-member cannot leave.
	wm.send(map[string]any{"t": "join", "r": slug})
	wm.expect("joined")
	if e.count(`SELECT count(*) FROM room_members WHERE room = $1 AND user_id = $2`, slug, mustID(t, e, "t_lm")) != 1 {
		t.Fatal("join was not recorded")
	}
	if got := wm.ask(map[string]any{"t": "rleave", "r": slug}, "rleft"); got != "ok:rleft" {
		t.Fatalf("leave: %s", got)
	}
	if got := wm.ask(map[string]any{"t": "rleave", "r": slug}, "rleft"); got != "err:not_member" {
		t.Fatalf("second leave: %s", got)
	}
	if e.count(`SELECT count(*) FROM room_members WHERE room = $1 AND user_id = $2 AND left_at IS NULL`, slug, mustID(t, e, "t_lm")) != 0 {
		t.Fatal("leave was not recorded")
	}
	if got := wh.ask(map[string]any{"t": "rleave", "r": slug}, "rleft"); got != "err:owner_cannot_leave" {
		t.Fatalf("owner leave: %s", got)
	}
}

func TestUserSearchCaseInsensitivePartial(t *testing.T) {
	e := newTestEnv(t)
	e.account("t_sx_Maria", "user")
	e.account("t_sx_other", "user")
	w := e.login("t_sx_other")
	for _, q := range []string{"MARI", "Maria", "@mar", "aria"} {
		w.drain()
		w.send(map[string]any{"t": "usearch", "n": q})
		f := w.expect("ufound")
		b, _ := json.Marshal(f)
		if f["q"] != strings.ToLower(strings.TrimPrefix(q, "@")) || !strings.Contains(strings.ToLower(string(b)), "t_sx_maria") {
			t.Fatalf("query %q: %s", q, b)
		}
		time.Sleep(300 * time.Millisecond) // the server's own cooldown between searches
	}
}
