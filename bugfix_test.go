package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"
)

func testDatabaseURL() string { return os.Getenv("TEST_DATABASE_URL") }

// ---- bug 1: starting a poll answered "The server is busy" ----

// The root cause: the poll options were sent to Postgres as a Go []byte. With binary_parameters=yes in the connection
// string lib/pq sends raw bytes, and Postgres reads them as binary jsonb and refuses them.
func TestBugfixPollInsertWorksInBothConnectionModes(t *testing.T) {
	e := newTestEnv(t)
	u := e.account("t_bfpoll", "user")
	for name, suffix := range map[string]string{"default": "", "binary_parameters": "&binary_parameters=yes"} {
		db, err := sql.Open("postgres", testDatabaseURL()+suffix)
		if err != nil {
			t.Fatal(err)
		}
		st := &store{db: db}
		id, created, err := st.createPoll(context.Background(), "lobby", "Best city?", []string{"Abuja", "Lagos"}, u.ID, "t_bfpoll")
		if err != nil || id == 0 || created.IsZero() {
			t.Fatalf("%s: creating a poll failed: %v", name, err)
		}
		var opts string
		if err := db.QueryRow(`SELECT options::text FROM polls WHERE id = $1`, id).Scan(&opts); err != nil || !strings.Contains(opts, "Abuja") {
			t.Fatalf("%s: stored options %q (%v)", name, opts, err)
		}
		db.Exec(`DELETE FROM polls WHERE id = $1`, id)
		db.Close()
	}
}

// The negative control: the old way of sending the options really fails in that mode, with the error behind the
// "server is busy" message. If this ever stops failing, the test above no longer proves the fix.
func TestBugfixOldByteParameterFailsWithBinaryParameters(t *testing.T) {
	e := newTestEnv(t)
	u := e.account("t_bfold", "user")
	db, err := sql.Open("postgres", testDatabaseURL()+"&binary_parameters=yes")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`INSERT INTO polls (room, question, options, created_by, created_by_handle) VALUES ('lobby','q',$1,$2,'x')`, []byte(`["a","b"]`), u.ID)
	if err == nil || !strings.Contains(err.Error(), "unsupported jsonb version") {
		t.Fatalf("expected the jsonb version error, got %v", err)
	}
}

func TestBugfixPollFailuresAreTold(t *testing.T) {
	for _, c := range []struct {
		err  error
		want string
	}{
		{&pq.Error{Code: "23505"}, "poll_open"},   // another poll is open
		{&pq.Error{Code: "42P01"}, "poll_failed"}, // table missing: the migration did not run
		{&pq.Error{Code: "42501"}, "poll_failed"}, // permission refused
		{&pq.Error{Code: "22023"}, "poll_failed"}, // bad value
		{&pq.Error{Code: "23503"}, "poll_failed"}, // the room is not in the database
		{&pq.Error{Code: "08006"}, "server_busy"}, // connection lost
		{&pq.Error{Code: "53300"}, "server_busy"}, // too many connections
		{&pq.Error{Code: "40001"}, "server_busy"}, // serialization failure
		{context.DeadlineExceeded, "server_busy"}, // too slow
		{sql.ErrConnDone, "server_busy"},
		{errors.New("something unexpected"), "poll_failed"},
	} {
		if got, detail := pollFailCode(c.err); got != c.want || detail == "" {
			t.Errorf("%v -> %q (detail %q), want %q", c.err, got, detail, c.want)
		}
	}
}

// A permanent database fault must not be reported as "busy, try again": here the polls table is missing.
func TestBugfixPollMissingTableIsNotServerBusy(t *testing.T) {
	e := newTestEnv(t)
	e.account("t_bfmod", "mod")
	w := e.login("t_bfmod")
	w.join("lobby")
	w.drain()
	if _, err := e.st.db.Exec(`ALTER TABLE polls RENAME TO polls_hidden_by_test`); err != nil {
		t.Fatal(err)
	}
	restore := func() { e.st.db.Exec(`ALTER TABLE IF EXISTS polls_hidden_by_test RENAME TO polls`) }
	defer restore()
	got := w.ask(map[string]any{"t": "pcreate", "b": "Best city?", "os": []string{"Abuja", "Lagos"}}, "poll")
	restore()
	if got != "err:poll_failed" {
		t.Fatalf("a missing table answered %q, want err:poll_failed", got)
	}
	// And once the fault is gone the same request works again: nothing stayed stuck.
	if got := w.ask(map[string]any{"t": "pcreate", "b": "Best city?", "os": []string{"Abuja", "Lagos"}}, "poll"); got != "ok:poll" {
		t.Fatalf("after the fault: %q", got)
	}
}

func TestBugfixPollCreationPermissions(t *testing.T) {
	e := newTestEnv(t)
	spec := e.communityRoom("t_bfhost")
	slug := e.slugOf(spec, "t_bfhost")
	e.account("t_bfmem", "user")
	e.account("t_bfmod2", "mod")
	host, mem, mod := e.login("t_bfhost"), e.login("t_bfmem"), e.login("t_bfmod2")
	host.join(slug)
	mem.join(slug)
	mod.join(slug)
	args := map[string]any{"t": "pcreate", "b": "Best city?", "os": []string{"Abuja", "Lagos"}}
	if got := mem.ask(args, "poll"); got != "err:forbidden" {
		t.Fatalf("member: %s", got)
	}
	out := e.login("t_bfmem")
	if got := out.ask(args, "poll"); got != "err:no_room" {
		t.Fatalf("a person in no room: %s", got)
	}
	out.join("lobby")
	if got := out.ask(args, "poll"); got != "err:forbidden" {
		t.Fatalf("a member in an official room: %s", got)
	}
	if e.count(`SELECT count(*) FROM polls`) != 0 {
		t.Fatal("a refused request stored a poll")
	}
	if got := mod.ask(args, "poll"); got != "ok:poll" {
		t.Fatalf("moderator in a community room: %s", got)
	}
	if got := host.ask(map[string]any{"t": "pclose", "pid": 1}, "poll"); got == "err:forbidden" {
		t.Fatalf("the host may close polls: %s", got)
	}
}

// A double tap is one poll. A different poll while one is open is still refused.
func TestBugfixPollDuplicateRequests(t *testing.T) {
	e := newTestEnv(t)
	spec := e.communityRoom("t_bfdup")
	slug := e.slugOf(spec, "t_bfdup")
	host := e.login("t_bfdup")
	host.join(slug)
	host.drain()
	w2 := e.login("t_bfdup")
	w2.join(slug)
	w2.drain()
	req := map[string]any{"t": "pcreate", "b": "Best city?", "os": []string{"Abuja", "Lagos"}}
	host.send(req)
	host.send(req)
	w2.send(req) // the same person from another tab
	first := pollOf(host.expect("poll"))
	second := pollOf(host.expect("poll"))
	third := pollOf(w2.expect("poll"))
	if first["id"] != second["id"] || first["id"] != third["id"] {
		t.Fatalf("different polls came back: %v %v %v", first["id"], second["id"], third["id"])
	}
	if n := e.count(`SELECT count(*) FROM polls WHERE room = $1`, slug); n != 1 {
		t.Fatalf("%d polls for one request", n)
	}
	quietly(host, w2)
	if got := host.ask(map[string]any{"t": "pcreate", "b": "Something else?", "os": []string{"x", "y"}}, "poll"); got != "err:poll_open" {
		t.Fatalf("a different poll while one is open: %s", got)
	}
	// Voting and closing still work on it.
	host.send(map[string]any{"t": "pvote", "pid": int64(first["id"].(float64)), "opt": 1})
	if p := pollMine(host, 1); counts(p)[1] != 1 {
		t.Fatalf("vote: %v", p)
	}
	host.send(map[string]any{"t": "pclose", "pid": int64(first["id"].(float64))})
	for i := 0; i < 5; i++ {
		if p := pollOf(host.expect("poll")); p["c"] == true {
			return
		}
	}
	t.Fatal("close did not arrive")
}

// ---- bug 2: people announced as gone while they are still there ----

func (e *testEnv) serverClients(handle string) []*client {
	id := mustID(e.t, e, handle)
	e.h.mu.Lock()
	defer e.h.mu.Unlock()
	var out []*client
	if ou := e.h.users[id]; ou != nil {
		for c := range ou.clients {
			out = append(out, c)
		}
	}
	return out
}

func rsFor(w *wsc, who string, wait time.Duration) []string {
	var out []string
	end := time.Now().Add(wait)
	for time.Now().Before(end) {
		if f, ok := w.tryRead(20 * time.Millisecond); ok && f["t"] == "rs" && f["h"] == who {
			out = append(out, f["a"].(string))
		}
	}
	return out
}

// A person who only reads stays in the room for as long as their connection is open, however long that is.
func TestBugfixQuietUserStaysInTheRoom(t *testing.T) {
	setGraces(t, 200*time.Millisecond, 400*time.Millisecond, 200*time.Millisecond)
	e := newTestEnv(t)
	e.account("t_bfq", "user")
	e.account("t_bfqo", "user")
	obs := e.login("t_bfqo")
	obs.join("lobby")
	q := e.login("t_bfq")
	q.join("lobby")
	obs.expect("rs")
	points := e.count(`SELECT points FROM users WHERE handle_key = 't_bfq'`)
	if got := rsFor(obs, "t_bfq", 1500*time.Millisecond); len(got) != 0 { // many times the grace period, no message sent
		t.Fatalf("a quiet person was announced: %v", got)
	}
	if here, in := e.roomsOf("t_bfq"); here != 1 || in != 1 {
		t.Fatalf("a quiet person is in %d rooms, present in %d", in, here)
	}
	e.h.mu.Lock()
	n := e.h.roomUsers(e.h.rooms["lobby"])
	e.h.mu.Unlock()
	if n != 2 {
		t.Fatalf("room count %d, want 2", n)
	}
	if e.count(`SELECT points FROM users WHERE handle_key = 't_bfq'`) != points {
		t.Fatal("presence changed points")
	}
}

// A lost connection is a lost connection: nothing is said at once, coming back in time is silent, and staying away
// is announced once, after the grace period and not before.
func TestBugfixNetworkLossGraceAndReconnect(t *testing.T) {
	setGraces(t, 500*time.Millisecond, 3*time.Second, 200*time.Millisecond)
	e := newTestEnv(t)
	e.account("t_bfn", "user")
	e.account("t_bfno", "user")
	obs := e.login("t_bfno")
	obs.join("lobby")
	n1 := e.login("t_bfn")
	n1.join("lobby")
	obs.expect("rs")
	n1.c.CloseNow()
	if got := rsFor(obs, "t_bfn", 250*time.Millisecond); len(got) != 0 {
		t.Fatalf("announced at once: %v", got)
	}
	n2 := e.login("t_bfn") // back inside the grace period
	n2.join("lobby")
	if got := rsFor(obs, "t_bfn", 900*time.Millisecond); len(got) != 0 {
		t.Fatalf("a reconnect made notices: %v", got)
	}
	start := time.Now()
	n2.c.CloseNow() // gone for good
	if got := rsFor(obs, "t_bfn", 1200*time.Millisecond); len(got) != 1 || got[0] != "l" {
		t.Fatalf("expected one leave after the grace period, got %v", got)
	}
	_ = start
	if here, in := e.roomsOf("t_bfn"); here != 0 || in != 0 {
		t.Fatalf("still counted after leaving: %d %d", here, in)
	}
}

// A page that said it was in the background is kept longer when its connection dies, because a phone that locks its
// screen cuts the connection without the person leaving. A page that did not say so gets the normal grace period.
func TestBugfixBackgroundedPageIsKeptLonger(t *testing.T) {
	setGraces(t, 300*time.Millisecond, 1500*time.Millisecond, 200*time.Millisecond)
	e := newTestEnv(t)
	e.account("t_bfbg", "user")
	e.account("t_bfbo", "user")
	obs := e.login("t_bfbo")
	obs.join("lobby")
	p := e.login("t_bfbg")
	p.join("lobby")
	obs.expect("rs")
	p.send(map[string]any{"t": "bg"})
	time.Sleep(50 * time.Millisecond)
	p.c.CloseNow()
	if got := rsFor(obs, "t_bfbg", 800*time.Millisecond); len(got) != 0 { // well past the normal grace period
		t.Fatalf("a backgrounded page was announced after the normal grace: %v", got)
	}
	back := e.login("t_bfbg") // the screen is unlocked again
	back.join("lobby")
	if got := rsFor(obs, "t_bfbg", 1200*time.Millisecond); len(got) != 0 {
		t.Fatalf("coming back made notices: %v", got)
	}
	// Foreground again: the next loss is an ordinary one.
	back.send(map[string]any{"t": "bg"})
	back.send(map[string]any{"t": "fg"})
	time.Sleep(50 * time.Millisecond)
	back.c.CloseNow()
	if got := rsFor(obs, "t_bfbg", 900*time.Millisecond); len(got) != 1 || got[0] != "l" {
		t.Fatalf("a foreground loss: %v", got)
	}
	// A backgrounded page that never comes back is announced after the longer period.
	e.account("t_bfbg2", "user")
	g := e.login("t_bfbg2")
	g.join("lobby")
	obs.expect("rs")
	g.send(map[string]any{"t": "bg"})
	time.Sleep(50 * time.Millisecond)
	g.c.CloseNow()
	if got := rsFor(obs, "t_bfbg2", 1000*time.Millisecond); len(got) != 0 {
		t.Fatalf("too early: %v", got)
	}
	if got := rsFor(obs, "t_bfbg2", 1200*time.Millisecond); len(got) != 1 || got[0] != "l" {
		t.Fatalf("a backgrounded page that never came back: %v", got)
	}
}

// Several tabs and devices: closing one, or the back arrow in one, says nothing while another is in the room.
func TestBugfixSeveralTabsNoFalseDeparture(t *testing.T) {
	setGraces(t, 300*time.Millisecond, 600*time.Millisecond, 200*time.Millisecond)
	e := newTestEnv(t)
	e.account("t_bft", "user")
	e.account("t_bfto", "user")
	obs := e.login("t_bfto")
	obs.join("lobby")
	a, b, c := e.login("t_bft"), e.login("t_bft"), e.login("t_bft")
	a.join("lobby")
	obs.expect("rs")
	b.join("lobby")
	c.join("lobby")
	if got := rsFor(obs, "t_bft", 300*time.Millisecond); len(got) != 0 {
		t.Fatalf("extra tabs made notices: %v", got)
	}
	a.c.CloseNow()                       // close a tab
	b.send(map[string]any{"t": "leave"}) // back arrow in another
	b.expect("left")
	c.send(map[string]any{"t": "bg"}) // the one that remains is only in the background
	if got := rsFor(obs, "t_bft", 1200*time.Millisecond); len(got) != 0 {
		t.Fatalf("a remaining tab did not keep the person in the room: %v", got)
	}
	e.h.mu.Lock()
	n := e.h.roomUsers(e.h.rooms["lobby"])
	e.h.mu.Unlock()
	if n != 2 {
		t.Fatalf("count %d, want 2 (the observer and the person once)", n)
	}
}

// Leave room is the person leaving: every tab leaves, once, at once, and the other tabs are told.
func TestBugfixExplicitLeaveIsImmediateAndPerPerson(t *testing.T) {
	e := newTestEnv(t) // the normal 60 second grace period: an explicit leave must not wait for it
	e.account("t_bfl", "user")
	e.account("t_bflo", "user")
	obs := e.login("t_bflo")
	obs.join("lobby")
	a, b := e.login("t_bfl"), e.login("t_bfl")
	a.join("lobby")
	b.join("lobby")
	obs.expect("rs")
	obs.drain()
	points := e.count(`SELECT points FROM users WHERE handle_key = 't_bfl'`)
	start := time.Now()
	a.send(map[string]any{"t": "leave", "a": "x"})
	got := rsFor(obs, "t_bfl", 600*time.Millisecond)
	if len(got) != 1 || got[0] != "l" || time.Since(start) > 2*time.Second {
		t.Fatalf("explicit leave: %v", got)
	}
	b.expect("left") // the other tab is taken out too
	if here, in := e.roomsOf("t_bfl"); here != 0 || in != 0 {
		t.Fatalf("still in the room after leaving: %d %d", here, in)
	}
	if e.count(`SELECT points FROM users WHERE handle_key = 't_bfl'`) != points {
		t.Fatal("notices changed points")
	}
}

// Ending a login on purpose, or a ban, is not a network loss: the person is not coming back, so no 60 second wait.
func TestBugfixLogoutLeavesAtOnce(t *testing.T) {
	e := newTestEnv(t)
	e.account("t_bfo", "user")
	e.account("t_bfoo", "user")
	obs := e.login("t_bfoo")
	obs.join("lobby")
	w := e.login("t_bfo")
	w.join("lobby")
	obs.expect("rs")
	w.send(map[string]any{"t": "bye"})
	if got := rsFor(obs, "t_bfo", 3*time.Second); len(got) != 1 || got[0] != "l" {
		t.Fatalf("logging out: %v", got)
	}
}

// A connection that is still being put back into the room (after a reconnect) counts as the person being there.
func TestBugfixConnectionOnItsWayInIsNotAnnouncedGone(t *testing.T) {
	setGraces(t, 250*time.Millisecond, time.Second, 500*time.Millisecond)
	e := newTestEnv(t)
	e.account("t_bfe", "user")
	e.account("t_bfeo", "user")
	obs := e.login("t_bfeo")
	obs.join("lobby")
	old := e.login("t_bfe")
	old.join("lobby")
	obs.expect("rs")
	fresh := e.login("t_bfe") // the reconnect: connected, not in the room yet
	var sc *client
	for _, c := range e.serverClients("t_bfe") {
		e.h.mu.Lock()
		if c.room == nil {
			sc = c
			c.entering = "lobby"
		}
		e.h.mu.Unlock()
	}
	if sc == nil {
		t.Fatal("no connection on its way in")
	}
	old.c.CloseNow()
	if got := rsFor(obs, "t_bfe", 650*time.Millisecond); len(got) != 0 { // past the grace period, inside the extra wait
		t.Fatalf("announced while the new connection was on its way in: %v", got)
	}
	fresh.join("lobby")
	e.h.mu.Lock()
	sc.entering = ""
	e.h.mu.Unlock()
	if got := rsFor(obs, "t_bfe", 900*time.Millisecond); len(got) != 0 {
		t.Fatalf("arriving made notices: %v", got)
	}
	// If it never lands, the person is announced once, after the extra wait, not forever deferred.
	e.account("t_bfe2", "user")
	o2 := e.login("t_bfe2")
	o2.join("lobby")
	obs.expect("rs")
	f2 := e.login("t_bfe2")
	_ = f2
	for _, c := range e.serverClients("t_bfe2") {
		e.h.mu.Lock()
		if c.room == nil {
			c.entering = "lobby"
		}
		e.h.mu.Unlock()
	}
	o2.c.CloseNow()
	if got := rsFor(obs, "t_bfe2", 1500*time.Millisecond); len(got) != 1 || got[0] != "l" {
		t.Fatalf("a connection that never arrived: %v", got)
	}
}
