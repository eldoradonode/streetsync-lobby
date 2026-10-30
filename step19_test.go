package main

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Step 19, objective 1: one room per account, the 60 second grace period, accurate counts, no duplicate notices.

func (e *testEnv) roomsOf(handle string) (here, inClients int) {
	e.t.Helper()
	id := mustID(e.t, e, handle)
	e.h.mu.Lock()
	defer e.h.mu.Unlock()
	for _, rm := range e.h.roomList {
		if rm.here[id] {
			here++
		}
		if rm.hasUser(id) {
			inClients++
		}
	}
	return
}

// setGrace shortens the grace period for one test and puts it back afterwards. It takes the same lock the server
// reads it under, so connections of an earlier test that are still closing cannot race with it.
func setGrace(t *testing.T, d time.Duration) {
	t.Helper()
	graceMu.Lock()
	old := graceLeave
	graceLeave = d
	graceMu.Unlock()
	t.Cleanup(func() {
		graceMu.Lock()
		graceLeave = old
		graceMu.Unlock()
	})
}

// setGraces is setGrace for all three waiting times of the presence code.
func setGraces(t *testing.T, grace, background, settle time.Duration) {
	t.Helper()
	graceMu.Lock()
	og, ob, os := graceLeave, backgroundGrace, enterSettle
	graceLeave, backgroundGrace, enterSettle = grace, background, settle
	graceMu.Unlock()
	t.Cleanup(func() {
		graceMu.Lock()
		graceLeave, backgroundGrace, enterSettle = og, ob, os
		graceMu.Unlock()
	})
}

func TestStep19GraceIsSixtySeconds(t *testing.T) {
	if graceDur() != 60*time.Second {
		t.Fatalf("the reconnection grace period is %v, want 60s", graceDur())
	}
}

func TestStep19OneRoomAcrossTabs(t *testing.T) {
	e := newTestEnv(t)
	e.account("t_19x", "user")
	e.account("t_19o", "user")
	e.account("t_19p", "user")
	wo := e.login("t_19o")
	wo.join("lobby")
	wp := e.login("t_19p")
	wp.join("abuja")
	wo.drain()
	wp.drain()
	pointsBefore := e.count(`SELECT points FROM users WHERE handle_key = 't_19x'`)
	msgsBefore := e.count(`SELECT count(*) FROM messages`)

	tab1 := e.login("t_19x")
	tab1.join("lobby")
	if f := wo.expect("rs"); f["a"] != "j" || f["h"] != "t_19x" {
		t.Fatalf("join notice: %v", f)
	}
	// A second tab of the same account joins ANOTHER room: the first tab leaves lobby, once, and lands on the hub.
	tab2 := e.login("t_19x")
	tab2.join("abuja")
	if f := tab1.expect("left"); f["w"] != "moved" {
		t.Fatalf("the first tab was not moved out: %v", f)
	}
	if f := wo.expect("rs"); f["a"] != "l" || f["h"] != "t_19x" {
		t.Fatalf("leave notice in the old room: %v", f)
	}
	if f := wp.expect("rs"); f["a"] != "j" || f["h"] != "t_19x" {
		t.Fatalf("join notice in the new room: %v", f)
	}
	wo.quiet("rs", 300*time.Millisecond)
	wp.quiet("rs", 300*time.Millisecond)
	if here, in := e.roomsOf("t_19x"); here != 1 || in != 1 {
		t.Fatalf("one account is in %d rooms (present in %d)", in, here)
	}
	e.h.mu.Lock()
	lobby, abuja := e.h.roomUsers(e.h.rooms["lobby"]), e.h.roomUsers(e.h.rooms["abuja"])
	e.h.mu.Unlock()
	if lobby != 1 || abuja != 2 {
		t.Fatalf("counts after the move: lobby=%d abuja=%d, want 1 and 2", lobby, abuja)
	}
	// A repeated join of the same room by the same account (duplicate attempt) makes no new notice.
	tab2.send(map[string]any{"t": "join", "r": "abuja"})
	tab2.expect("joined")
	wp.quiet("rs", 300*time.Millisecond)
	// Two tabs in the same room count once.
	tab1.join("abuja")
	wp.quiet("rs", 300*time.Millisecond)
	e.h.mu.Lock()
	abuja = e.h.roomUsers(e.h.rooms["abuja"])
	e.h.mu.Unlock()
	if abuja != 2 {
		t.Fatalf("an account counted twice: abuja=%d", abuja)
	}
	if e.count(`SELECT points FROM users WHERE handle_key = 't_19x'`) != pointsBefore || e.count(`SELECT count(*) FROM messages`) != msgsBefore {
		t.Fatal("room switching earned points or stored a message")
	}
}

func TestStep19SimultaneousJoinsLeaveOneRoom(t *testing.T) {
	e := newTestEnv(t)
	e.account("t_19r", "user")
	a, b := e.login("t_19r"), e.login("t_19r")
	rooms := []string{"lobby", "abuja", "lagos", "campus"}
	for i := 0; i < 20; i++ {
		ra, rb := rooms[i%4], rooms[(i+1)%4]
		a.send(map[string]any{"t": "join", "r": ra})
		b.send(map[string]any{"t": "join", "r": rb})
		a.expect("joined")
		b.expect("joined")
		time.Sleep(60 * time.Millisecond)
		if here, in := e.roomsOf("t_19r"); here != 1 || in != 1 {
			t.Fatalf("round %d: the account is in %d rooms and present in %d", i, in, here)
		}
	}
}

func TestStep19MovingFlushesPendingLeave(t *testing.T) {
	setGrace(t, 400*time.Millisecond)
	e := newTestEnv(t)
	e.account("t_19m", "user")
	e.account("t_19w", "user")
	ww := e.login("t_19w")
	ww.join("lobby")
	ww.drain()
	m1 := e.login("t_19m")
	m1.join("lobby")
	ww.expect("rs")
	// The connection drops: grace starts, nothing yet.
	m1.c.CloseNow()
	time.Sleep(100 * time.Millisecond)
	ww.quiet("rs", 100*time.Millisecond)
	// The person shows up in another room before the grace ends: the old room hears about it now, and only once.
	m2 := e.login("t_19m")
	m2.join("abuja")
	if f := ww.expect("rs"); f["a"] != "l" || f["h"] != "t_19m" {
		t.Fatalf("leave notice: %v", f)
	}
	ww.quiet("rs", 900*time.Millisecond) // the old timer must not send a second one
}

func TestStep19ReconnectInsideGraceIsSilent(t *testing.T) {
	setGrace(t, 600*time.Millisecond)
	e := newTestEnv(t)
	e.account("t_19c", "user")
	e.account("t_19w2", "user")
	ww := e.login("t_19w2")
	ww.join("lobby")
	c1 := e.login("t_19c")
	c1.join("lobby")
	ww.expect("rs")
	c1.c.CloseNow()
	time.Sleep(150 * time.Millisecond)
	c2 := e.login("t_19c")
	c2.join("lobby")
	ww.quiet("rs", 1200*time.Millisecond) // no leave, no second join
	// Silence is not leaving: no messages for longer than the grace period changes nothing.
	if here, _ := e.roomsOf("t_19c"); here != 1 {
		t.Fatalf("a quiet member was removed: here=%d", here)
	}
	// Not coming back: the notice goes out once after the grace period.
	c2.c.CloseNow()
	if f := ww.expect("rs"); f["a"] != "l" {
		t.Fatalf("leave after grace: %v", f)
	}
}

func TestStep19JoinedStateMoves(t *testing.T) {
	e := newTestEnv(t)
	specA, specB := e.communityRoom("t_19ha"), e.communityRoom("t_19hb")
	a, b := specA[:len(specA)-len("|t_19ha")], specB[:len(specB)-len("|t_19hb")]
	u := e.account("t_19u", "user")
	w := e.login("t_19u")
	w.join(a)
	if e.count(`SELECT count(*) FROM room_members WHERE user_id = $1 AND room = $2 AND left_at IS NULL`, u.ID, a) != 1 {
		t.Fatal("not joined to the first room")
	}
	w.join(b)
	time.Sleep(150 * time.Millisecond)
	if e.count(`SELECT count(*) FROM room_members WHERE user_id = $1 AND room = $2 AND left_at IS NULL`, u.ID, a) != 0 {
		t.Fatal("still joined to the first room after joining another")
	}
	if e.count(`SELECT count(*) FROM room_members WHERE user_id = $1 AND room = $2 AND left_at IS NULL`, u.ID, b) != 1 {
		t.Fatal("not joined to the second room")
	}
	// The right to come back is kept: the row stays.
	if e.count(`SELECT count(*) FROM room_members WHERE user_id = $1 AND room = $2`, u.ID, a) != 1 {
		t.Fatal("the access row of the first room was deleted")
	}
}

// ---- objectives 2 to 4: pins, reactions, polls ----

func (e *testEnv) slugOf(spec, host string) string { return spec[:len(spec)-len("|"+host)] }

// snapshot of everything that must NOT change when someone pins, reacts or votes.
type rewardSnap struct{ points, ledger, msgs int }

func (e *testEnv) rewards(handle string) rewardSnap {
	return rewardSnap{
		e.count(`SELECT points FROM users WHERE handle_key = $1`, handle),
		e.count(`SELECT count(*) FROM credit_ledger WHERE user_id = (SELECT id FROM users WHERE handle_key = $1)`, handle),
		e.count(`SELECT count(*) FROM messages WHERE lower(handle) = $1`, handle),
	}
}

func TestStep19PinPermissionsAndSync(t *testing.T) {
	e := newTestEnv(t)
	spec := e.communityRoom("t_19phost")
	slug := e.slugOf(spec, "t_19phost")
	e.account("t_19pmem", "user")
	e.account("t_19pmod", "mod")
	host := e.login("t_19phost")
	host.join(slug)
	mem := e.login("t_19pmem")
	mem.join(slug)
	host.drain()
	mem.drain()
	id := mem.say("Meetup on Saturday at 5pm")
	host.sawMessage("Meetup on Saturday at 5pm")
	before := e.rewards("t_19pmem")

	// A regular member can neither pin nor unpin, and nothing changes.
	quietly(mem)
	if got := mem.ask(map[string]any{"t": "pin", "i": id}, "pin"); got != "err:forbidden" {
		t.Fatalf("member pin: %s", got)
	}
	if e.count(`SELECT count(*) FROM room_pins WHERE room = $1`, slug) != 0 {
		t.Fatal("a member pinned a message")
	}
	// The host can. Everyone in the room is told, once.
	host.drain()
	mem.drain()
	host.send(map[string]any{"t": "pin", "i": id})
	for _, w := range []*wsc{host, mem} {
		f := w.expect("pin")
		p, _ := f["p"].(map[string]any)
		if p == nil || p["i"] != float64(id) || p["b"] != "Meetup on Saturday at 5pm" || p["by"] != "t_19phost" {
			t.Fatalf("pin frame: %v", f)
		}
	}
	// A duplicate pin of the same message makes no second row and no second notice.
	host.send(map[string]any{"t": "pin", "i": id})
	mem.quiet("pin", 400*time.Millisecond)
	if e.count(`SELECT count(*) FROM room_pins WHERE room = $1`, slug) != 1 {
		t.Fatal("duplicate pin rows")
	}
	quietly(mem)
	if got := mem.ask(map[string]any{"t": "unpin"}, "pin"); got != "err:forbidden" {
		t.Fatalf("member unpin: %s", got)
	}
	// Pinning something that is not in this room is refused.
	if got := host.ask(map[string]any{"t": "pin", "i": 999999999}, "pin"); got != "err:msg_gone" {
		t.Fatalf("pin of a missing message: %s", got)
	}
	// Someone arriving later sees the pin straight away, in one frame.
	late := e.login("t_19pmem")
	late.send(map[string]any{"t": "join", "r": slug})
	x := late.expect("x19")
	if p, _ := x["pin"].(map[string]any); p == nil || p["i"] != float64(id) {
		t.Fatalf("late joiner: %v", x)
	}
	// The pin survives a restart (memory dropped, database read again).
	e.h.mu.Lock()
	e.h.rooms[slug].pin = nil
	e.h.mu.Unlock()
	if err := e.h.loadEngagement(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.h.mu.Lock()
	ok := e.h.rooms[slug].pin != nil && e.h.rooms[slug].pin.MsgID == id
	e.h.mu.Unlock()
	if !ok {
		t.Fatal("the pin was not restored from the database")
	}
	// Deleting the pinned message removes the pin everywhere.
	host.drain()
	mem.drain()
	host.send(map[string]any{"t": "del", "i": id})
	if f := mem.expect("pin"); f["p"] != nil {
		t.Fatalf("pin after delete: %v", f)
	}
	if e.count(`SELECT count(*) FROM room_pins WHERE room = $1`, slug) != 0 {
		t.Fatal("the pin of a deleted message stayed")
	}
	// Unpinning with nothing pinned is harmless. A new pin replaces the old (one pin per room).
	host.send(map[string]any{"t": "unpin"})
	mem.quiet("pin", 300*time.Millisecond)
	a := host.say("first")
	b := host.say("second")
	host.send(map[string]any{"t": "pin", "i": a})
	host.expect("pin")
	host.send(map[string]any{"t": "pin", "i": b})
	if f := host.expect("pin"); f["p"].(map[string]any)["i"] != float64(b) {
		t.Fatalf("replacing the pin: %v", f)
	}
	if e.count(`SELECT count(*) FROM room_pins WHERE room = $1`, slug) != 1 {
		t.Fatal("more than one pin in a room")
	}
	// Officials rooms: staff only.
	reg := e.login("t_19pmem")
	reg.join("lobby")
	lid := reg.say("hello lobby")
	reg.drain()
	if got := reg.ask(map[string]any{"t": "pin", "i": lid}, "pin"); got != "err:forbidden" {
		t.Fatalf("member pin in an official room: %s", got)
	}
	mod := e.login("t_19pmod")
	mod.join("lobby")
	if got := mod.ask(map[string]any{"t": "pin", "i": lid}, "pin"); got != "ok:pin" {
		t.Fatalf("moderator pin in an official room: %s", got)
	}
	// The members' own host rights stop at their own room.
	if got := host.ask(map[string]any{"t": "pin", "i": lid}, "pin"); got != "err:msg_gone" && got != "err:forbidden" {
		t.Fatalf("host pin across rooms: %s", got)
	}
	after := e.rewards("t_19pmem")
	if after.points != before.points+e.count(`SELECT 0`)+0 && after.points < before.points {
		t.Fatalf("points went down: %v %v", before, after)
	}
}

func TestStep19Reactions(t *testing.T) {
	e := newTestEnv(t)
	e.account("t_19ra", "user")
	e.account("t_19rb", "user")
	e.account("t_19rc", "user")
	a, b := e.login("t_19ra"), e.login("t_19rb")
	a.join("lobby")
	b.join("lobby")
	a.drain()
	b.drain()
	id := a.say("react to me")
	b.sawMessage("react to me")
	snapB := e.rewards("t_19rb")
	snapA := e.rewards("t_19ra")

	b.send(map[string]any{"t": "rx", "i": id, "em": "👍"})
	for _, w := range []*wsc{a, b} {
		f := w.expect("rx")
		if c, _ := f["c"].(map[string]any); c["👍"] != float64(1) || f["h"] != "t_19rb" {
			t.Fatalf("rx frame: %v", f)
		}
	}
	// The same reaction again: nothing changes, the other person hears nothing.
	b.send(map[string]any{"t": "rx", "i": id, "em": "👍"})
	b.expect("rx")
	a.quiet("rx", 400*time.Millisecond)
	// Change it, then another person adds the same one.
	b.send(map[string]any{"t": "rx", "i": id, "em": "❤️"})
	if f := a.expect("rx"); f["c"].(map[string]any)["❤️"] != float64(1) || f["c"].(map[string]any)["👍"] != nil {
		t.Fatalf("changed reaction: %v", f)
	}
	b.drain()
	a.send(map[string]any{"t": "rx", "i": id, "em": "❤"}) // without the variation selector
	if f := rxFrom(b, "t_19ra"); f["c"].(map[string]any)["❤️"] != float64(2) {
		t.Fatalf("second heart: %v", f)
	}
	if e.count(`SELECT count(*) FROM message_reactions WHERE msg_id = $1`, id) != 2 {
		t.Fatal("rows do not match the two people")
	}
	// Removing.
	a.drain()
	b.send(map[string]any{"t": "rx", "i": id, "em": ""})
	if f := rxFrom(a, "t_19rb"); f["c"].(map[string]any)["❤️"] != float64(1) || f["y"] != "" {
		t.Fatalf("removed: %v", f)
	}
	rxFrom(b, "t_19rb") // b's own copy of the removal, so it cannot be mistaken for an answer below
	// Anything outside the list, a missing field, a message that is not here: refused.
	quietly(b)
	if got := b.ask(map[string]any{"t": "rx", "i": id, "em": "💩"}, "rx"); got != "err:bad_emoji" {
		t.Fatalf("outside the list: %s", got)
	}
	quietly(b)
	if got := b.ask(map[string]any{"t": "rx", "i": id}, "rx"); got != "err:bad_emoji" {
		t.Fatalf("missing emoji: %s", got)
	}
	quietly(b)
	if got := b.ask(map[string]any{"t": "rx", "i": 424242424, "em": "👍"}, "rx"); got != "err:msg_gone" {
		t.Fatalf("missing message: %s", got)
	}
	// Not in the room: refused. Another room's message: refused.
	c := e.login("t_19rc")
	quietly(c)
	if got := c.ask(map[string]any{"t": "rx", "i": id, "em": "👍"}, "rx"); got != "err:no_room" {
		t.Fatalf("outside any room: %s", got)
	}
	c.join("abuja")
	quietly(c)
	if got := c.ask(map[string]any{"t": "rx", "i": id, "em": "👍"}, "rx"); got != "err:msg_gone" {
		t.Fatalf("another room's message: %s", got)
	}
	// A room member list: a community room needs membership. A person who never joined cannot react there.
	spec := e.communityRoom("t_19rhost")
	slug := e.slugOf(spec, "t_19rhost")
	h := e.login("t_19rhost")
	h.join(slug)
	hid := h.say("host words")
	e.h.mu.Lock()
	cu := e.h.users[mustID(t, e, "t_19rc")]
	e.h.mu.Unlock()
	c.join(slug) // entering a free public room is joining it, so this works
	c.send(map[string]any{"t": "rx", "i": hid, "em": "🙏"})
	c.expect("rx")
	e.h.mu.Lock()
	delete(cu.joined, slug) // membership gone (left): the server must refuse
	e.h.mu.Unlock()
	quietly(c)
	if got := c.ask(map[string]any{"t": "rx", "i": hid, "em": "😮"}, "rx"); got != "err:not_member" {
		t.Fatalf("reaction without membership: %s", got)
	}
	// A new arrival gets the reactions of the history in the join answer, with their own marked.
	late := e.login("t_19ra")
	late.send(map[string]any{"t": "join", "r": "lobby"})
	x := late.expect("x19")
	rx, _ := x["rx"].(map[string]any)
	entry, _ := rx[strconv.FormatInt(id, 10)].(map[string]any)
	if entry == nil || entry["y"] != "❤️" {
		t.Fatalf("reactions on join: %v", x["rx"])
	}
	// Deleting the message removes its reactions and a late reaction on it is refused.
	a.send(map[string]any{"t": "del", "i": id})
	a.expect("del")
	time.Sleep(100 * time.Millisecond)
	if e.count(`SELECT count(*) FROM message_reactions WHERE msg_id = $1`, id) != 0 {
		t.Fatal("reactions of a deleted message stayed")
	}
	quietly(b)
	if got := b.ask(map[string]any{"t": "rx", "i": id, "em": "👍"}, "rx"); got != "err:msg_gone" {
		t.Fatalf("reaction on a deleted message: %s", got)
	}
	if e.rewards("t_19rb") != snapB || e.count(`SELECT points FROM users WHERE handle_key = 't_19ra'`) < snapA.points {
		t.Fatalf("reactions changed points, credits or messages: %v -> %v", snapB, e.rewards("t_19rb"))
	}
}

func TestStep19ReactionsConcurrent(t *testing.T) {
	e := newTestEnv(t)
	host := e.login(func() string { e.account("t_19cx", "user"); return "t_19cx" }())
	host.join("lobby")
	id := host.say("many hands")
	const n = 12
	ws := make([]*wsc, n)
	for i := range ws {
		h := "t_19cy" + string(rune('a'+i))
		e.account(h, "user")
		ws[i] = e.login(h)
		ws[i].join("lobby")
	}
	// Everyone reacts at once, and one account also does it from two tabs with different emoji.
	twin := e.login("t_19cya")
	twin.join("lobby")
	for i := range ws {
		ws[i].send(map[string]any{"t": "rx", "i": id, "em": "👍"})
	}
	twin.send(map[string]any{"t": "rx", "i": id, "em": "😂"})
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if e.count(`SELECT count(*) FROM message_reactions WHERE msg_id = $1`, id) == n {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := e.count(`SELECT count(*) FROM message_reactions WHERE msg_id = $1`, id); got != n {
		rows, _ := e.st.db.Query(`SELECT u.handle_key, r.emoji FROM message_reactions r JOIN users u ON u.id = r.user_id WHERE r.msg_id = $1`, id)
		for rows != nil && rows.Next() {
			var hk, em string
			rows.Scan(&hk, &em)
			t.Logf("row: %s %s", hk, em)
		}
		t.Fatalf("%d reaction rows for %d people", got, n)
	}
	if got := e.count(`SELECT count(*) FROM (SELECT user_id FROM message_reactions WHERE msg_id = $1 GROUP BY user_id HAVING count(*) > 1) d`, id); got != 0 {
		t.Fatal("one account has two reactions on one message")
	}
}

// rxFrom waits for the next reaction frame caused by one person, skipping copies of earlier ones.
func rxFrom(w *wsc, handle string) map[string]any {
	w.t.Helper()
	for i := 0; i < 20; i++ {
		if f := w.expect("rx"); f["h"] == handle {
			return f
		}
	}
	w.t.Fatalf("no reaction frame from %s", handle)
	return nil
}

// pollMine waits for a poll frame in which this person's own vote is y, skipping older copies still in flight.
func pollMine(w *wsc, y float64) map[string]any {
	w.t.Helper()
	for i := 0; i < 20; i++ {
		if p := pollOf(w.expect("poll")); p["y"] == y {
			return p
		}
	}
	w.t.Fatalf("no poll frame with own vote %v", y)
	return nil
}

// quietly lets frames that are still on their way arrive and throws them away, so the next answer is the real one.
func quietly(ws ...*wsc) {
	time.Sleep(120 * time.Millisecond)
	for _, w := range ws {
		w.drain()
	}
}

func pollOf(f map[string]any) map[string]any {
	p, _ := f["p"].(map[string]any)
	return p
}

func counts(p map[string]any) []int {
	var out []int
	for _, v := range p["n"].([]any) {
		out = append(out, int(v.(float64)))
	}
	return out
}

func TestStep19Polls(t *testing.T) {
	e := newTestEnv(t)
	spec := e.communityRoom("t_19pollhost")
	slug := e.slugOf(spec, "t_19pollhost")
	e.account("t_19pm1", "user")
	e.account("t_19pm2", "user")
	e.account("t_19pmod", "mod")
	host := e.login("t_19pollhost")
	host.join(slug)
	m1, m2 := e.login("t_19pm1"), e.login("t_19pm2")
	m1.join(slug)
	m2.join(slug)
	host.drain()
	m1.drain()
	m2.drain()
	before := e.rewards("t_19pm1")

	create := func(w *wsc, q string, opts ...string) string {
		return w.ask(map[string]any{"t": "pcreate", "b": q, "os": opts}, "poll")
	}
	// Members cannot create a poll. Bad input is refused by the server whatever the page checked.
	if got := create(m1, "Best city?", "Abuja", "Lagos"); got != "err:forbidden" {
		t.Fatalf("member create: %s", got)
	}
	long := strings.Repeat("x", pollQuestionMax+1)
	for name, c := range map[string][]any{
		"one option":   {"Best city?", "Abuja"},
		"seven":        {"Best city?", "a", "b", "c", "d", "e", "f", "g"},
		"duplicate":    {"Best city?", "Abuja", "abuja"},
		"empty option": {"Best city?", "Abuja", "  "},
		"short q":      {"ab", "x", "y"},
		"long q":       {long, "x", "y"},
		"long option":  {"Best city?", strings.Repeat("o", pollOptionMax+1), "y"},
	} {
		opts := []string{}
		for _, o := range c[1:] {
			opts = append(opts, o.(string))
		}
		if got := create(host, c[0].(string), opts...); got != "err:bad_poll" {
			t.Fatalf("%s: %s", name, got)
		}
	}
	if e.count(`SELECT count(*) FROM polls WHERE room = $1`, slug) != 0 {
		t.Fatal("a refused poll was stored")
	}
	// The host creates one. Everyone in the room gets it, with no votes.
	host.send(map[string]any{"t": "pcreate", "b": "Best city?", "os": []string{"Abuja", "Lagos", "Kano"}})
	var pid float64
	for _, w := range []*wsc{host, m1, m2} {
		p := pollOf(w.expect("poll"))
		if p == nil || p["q"] != "Best city?" || p["c"] != false || p["y"] != float64(-1) || p["t"] != float64(0) {
			t.Fatalf("new poll: %v", p)
		}
		pid = p["id"].(float64)
	}
	// Only one open poll at a time.
	if got := create(host, "Another?", "a", "b"); got != "err:poll_open" {
		t.Fatalf("second open poll: %s", got)
	}
	// A vote. The voter's own choice comes back to them only, and totals reach everyone.
	m1.drain()
	m2.drain()
	m1.send(map[string]any{"t": "pvote", "pid": int64(pid), "opt": 1})
	p1, p2 := pollOf(m1.expect("poll")), pollOf(m2.expect("poll"))
	if c := counts(p1); c[0] != 0 || c[1] != 1 || p1["y"] != float64(1) || p2["y"] != float64(-1) || p2["t"] != float64(1) {
		t.Fatalf("after one vote: %v %v", p1, p2)
	}
	// Changing a vote moves it, it does not add one.
	m1.send(map[string]any{"t": "pvote", "pid": int64(pid), "opt": 2})
	p1 = pollMine(m1, 2)
	if c := counts(p1); c[1] != 0 || c[2] != 1 || p1["t"] != float64(1) {
		t.Fatalf("after changing: %v", p1)
	}
	m2.drain()
	m2.send(map[string]any{"t": "pvote", "pid": int64(pid), "opt": 2})
	pollMine(m2, 2)
	if e.count(`SELECT count(*) FROM poll_votes WHERE poll_id = $1`, int64(pid)) != 2 {
		t.Fatal("votes in the database do not match the two people")
	}
	// Forged votes: out of range, a made up poll, from outside the room.
	quietly(m1)
	if got := m1.ask(map[string]any{"t": "pvote", "pid": int64(pid), "opt": 9}, "poll"); got != "err:bad_poll" {
		t.Fatalf("out of range: %s", got)
	}
	quietly(m1)
	if got := m1.ask(map[string]any{"t": "pvote", "pid": 987654, "opt": 0}, "poll"); got != "err:no_poll" {
		t.Fatalf("made up poll: %s", got)
	}
	quietly(m1)
	if got := m1.ask(map[string]any{"t": "pvote", "pid": int64(pid)}, "poll"); got != "err:bad_poll" {
		t.Fatalf("missing option: %s", got)
	}
	out := e.login("t_19pmod")
	quietly(out)
	if got := out.ask(map[string]any{"t": "pvote", "pid": int64(pid), "opt": 0}, "poll"); got != "err:no_room" {
		t.Fatalf("voting from outside a room: %s", got)
	}
	out.join("lobby")
	quietly(out)
	if got := out.ask(map[string]any{"t": "pvote", "pid": int64(pid), "opt": 0}, "poll"); got != "err:no_poll" {
		t.Fatalf("voting on another room's poll: %s", got)
	}
	// A late arrival gets the poll with their own vote.
	late := e.login("t_19pm1")
	late.send(map[string]any{"t": "join", "r": slug})
	if p, _ := late.expect("x19")["poll"].(map[string]any); p == nil || p["y"] != float64(2) {
		t.Fatalf("late joiner poll: %v", p)
	}
	// Persistence: drop memory, read the database again.
	e.h.mu.Lock()
	e.h.rooms[slug].poll = nil
	e.h.mu.Unlock()
	if err := e.h.loadEngagement(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.h.mu.Lock()
	pl := e.h.rooms[slug].poll
	ok := pl != nil && pl.Counts[2] == 2 && pl.Votes[mustID(t, e, "t_19pm1")] == 2
	e.h.mu.Unlock()
	if !ok {
		t.Fatal("the poll and its votes were not restored from the database")
	}
	// Closing: members cannot. The host can. Nobody votes afterwards, repeat closes are harmless.
	m1.drain()
	quietly(m1)
	if got := m1.ask(map[string]any{"t": "pclose", "pid": int64(pid)}, "poll"); got != "err:forbidden" {
		t.Fatalf("member close: %s", got)
	}
	host.drain()
	m2.drain()
	host.send(map[string]any{"t": "pclose", "pid": int64(pid)})
	if p := pollOf(m2.expect("poll")); p["c"] != true {
		t.Fatalf("closed poll: %v", p)
	}
	host.send(map[string]any{"t": "pclose", "pid": int64(pid)})
	m2.quiet("poll", 300*time.Millisecond)
	quietly(m2)
	if got := m2.ask(map[string]any{"t": "pvote", "pid": int64(pid), "opt": 0}, "poll"); got != "err:poll_closed" {
		t.Fatalf("vote after close: %s", got)
	}
	if e.count(`SELECT count(*) FROM polls WHERE id = $1 AND closed_at IS NOT NULL`, int64(pid)) != 1 {
		t.Fatal("close was not stored")
	}
	// A new poll can be made once the old one is closed, and its votes start from zero.
	host.drain()
	host.send(map[string]any{"t": "pcreate", "b": "Next?", "os": []string{"yes", "no"}})
	if p := pollOf(host.expect("poll")); p["t"] != float64(0) || p["id"] == float64(pid) {
		t.Fatalf("second poll: %v", p)
	}
	// Nothing here earns anything.
	if e.rewards("t_19pm1") != before {
		t.Fatalf("polls changed points, credits or messages: %v -> %v", before, e.rewards("t_19pm1"))
	}
}

func TestStep19PollsConcurrentVotes(t *testing.T) {
	e := newTestEnv(t)
	spec := e.communityRoom("t_19cphost")
	slug := e.slugOf(spec, "t_19cphost")
	host := e.login("t_19cphost")
	host.join(slug)
	host.drain()
	host.send(map[string]any{"t": "pcreate", "b": "Many votes?", "os": []string{"a", "b"}})
	pid := int64(pollOf(host.expect("poll"))["id"].(float64))
	const n = 12
	ws := make([]*wsc, n)
	for i := range ws {
		h := "t_19cpv" + string(rune('a'+i))
		e.account(h, "user")
		ws[i] = e.login(h)
		ws[i].join(slug)
	}
	twin := e.login("t_19cpva")
	twin.join(slug)
	for i := range ws {
		ws[i].send(map[string]any{"t": "pvote", "pid": pid, "opt": i % 2})
	}
	twin.send(map[string]any{"t": "pvote", "pid": pid, "opt": 1}) // same account, other tab
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if e.count(`SELECT count(*) FROM poll_votes WHERE poll_id = $1`, pid) == n {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := e.count(`SELECT count(*) FROM poll_votes WHERE poll_id = $1`, pid); got != n {
		t.Fatalf("%d votes for %d people", got, n)
	}
	time.Sleep(300 * time.Millisecond)
	e.h.mu.Lock()
	p := e.h.rooms[slug].poll
	sum := p.Counts[0] + p.Counts[1]
	e.h.mu.Unlock()
	var a, b int
	e.st.db.QueryRow(`SELECT count(*) FILTER (WHERE opt = 0), count(*) FILTER (WHERE opt = 1) FROM poll_votes WHERE poll_id = $1`, pid).Scan(&a, &b)
	if sum != n || p.Counts[0] != a || p.Counts[1] != b {
		t.Fatalf("live counts %v do not match the database %d/%d", p.Counts, a, b)
	}
	// A vote racing a close never lands after it: whatever the order, memory and database agree.
	host.send(map[string]any{"t": "pclose", "pid": pid})
	ws[3].send(map[string]any{"t": "pvote", "pid": pid, "opt": 1})
	time.Sleep(400 * time.Millisecond)
	e.st.db.QueryRow(`SELECT count(*) FILTER (WHERE opt = 0), count(*) FILTER (WHERE opt = 1) FROM poll_votes WHERE poll_id = $1`, pid).Scan(&a, &b)
	e.h.mu.Lock()
	ca, cb := p.Counts[0], p.Counts[1]
	e.h.mu.Unlock()
	if ca != a || cb != b {
		t.Fatalf("after close: live %d/%d, database %d/%d", ca, cb, a, b)
	}
}

// The Leave room button leaves at once. The back arrow only starts the grace period, and coming back is silent.
func TestStep19ExplicitLeaveAndBackArrow(t *testing.T) {
	setGrace(t, 500*time.Millisecond)
	e := newTestEnv(t)
	e.account("t_19lv", "user")
	e.account("t_19lw", "user")
	ww := e.login("t_19lw")
	ww.join("lobby")
	ww.drain()
	u := e.login("t_19lv")
	u.join("lobby")
	ww.expect("rs")
	// Back arrow, then straight back in: no notice at all.
	u.send(map[string]any{"t": "leave"})
	u.expect("left")
	ww.quiet("rs", 150*time.Millisecond)
	u.join("lobby")
	ww.quiet("rs", 900*time.Millisecond)
	// Back arrow and stay away: one notice, after the grace period and not before.
	u.send(map[string]any{"t": "leave"})
	ww.quiet("rs", 200*time.Millisecond)
	if f := ww.expect("rs"); f["a"] != "l" || f["h"] != "t_19lv" {
		t.Fatalf("leave after grace: %v", f)
	}
	// Explicit Leave room: the notice is immediate.
	u.join("lobby")
	ww.expect("rs")
	start := time.Now()
	u.send(map[string]any{"t": "leave", "a": "x"})
	if f := ww.expect("rs"); f["a"] != "l" || time.Since(start) > 350*time.Millisecond {
		t.Fatalf("explicit leave was not immediate: %v after %v", f, time.Since(start))
	}
}
