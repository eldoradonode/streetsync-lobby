package main

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ---- helpers ----

func strs(v any) []string {
	var out []string
	if a, ok := v.([]any); ok {
		for _, x := range a {
			out = append(out, x.(string))
		}
	}
	return out
}

func slugsOf(v any) []string {
	var out []string
	if a, ok := v.([]any); ok {
		for _, x := range a {
			out = append(out, x.(map[string]any)["s"].(string))
		}
	}
	return out
}

// myRooms asks for the favourites and the recent rooms and returns them.
func myRooms(w *wsc) (fs, fav, recent []string) {
	quietly(w)
	w.send(map[string]any{"t": "myrooms"})
	f := w.expect("mr")
	return strs(f["fs"]), slugsOf(f["f"]), slugsOf(f["v"])
}

// members asks for a page of the member list and returns the frame. It waits out the small gap between two requests.
func members(w *wsc, off int) map[string]any {
	time.Sleep(listGap + 60*time.Millisecond)
	quietly(w)
	w.send(map[string]any{"t": "members", "off": off})
	return w.expect("ml")
}

func statusOf(f map[string]any) map[string]string {
	out := map[string]string{}
	for _, x := range f["m"].([]any) {
		m := x.(map[string]any)
		out[m["h"].(string)] = m["s"].(string)
	}
	return out
}

func mpFor(w *wsc, who string, wait time.Duration) []string {
	var out []string
	end := time.Now().Add(wait)
	for time.Now().Before(end) {
		f, ok := w.tryRead(20 * time.Millisecond)
		if !ok || f["t"] != "mp" || f["h"] != who {
			continue
		}
		if f["d"] != nil {
			out = append(out, "drop")
		} else {
			out = append(out, f["s"].(string))
		}
	}
	return out
}

func (e *testEnv) setPrivacy(w *wsc, hide bool) {
	e.t.Helper()
	b := "1" // "show when I am online"
	if hide {
		b = "0"
	}
	w.send(map[string]any{"t": "set", "k": "so", "b": b})
	time.Sleep(250 * time.Millisecond)
}

// ---- favourites ----

func TestStep20Favourites(t *testing.T) {
	old := favLimit
	favLimit = 2
	defer func() { favLimit = old }()
	e := newTestEnv(t)
	e.account("t_20fa", "user")
	e.account("t_20fb", "user")
	spec := e.communityRoom("t_20fhost")
	slug := e.slugOf(spec, "t_20fhost")
	a, a2 := e.login("t_20fa"), e.login("t_20fa")
	before := e.rewards("t_20fa")

	if got := a.ask(map[string]any{"t": "fav", "r": "lobby", "fav": true}, "mr"); got != "ok:mr" {
		t.Fatalf("favouriting: %s", got)
	}
	fs, fav, _ := myRooms(a)
	if len(fs) != 1 || fs[0] != "lobby" || len(fav) != 1 {
		t.Fatalf("favourites: %v %v", fs, fav)
	}
	a2.expect("mr") // the other tab hears about it too
	// Twice is still once. A missing flag, an unknown room: refused.
	a.ask(map[string]any{"t": "fav", "r": "lobby", "fav": true}, "mr")
	if e.count(`SELECT count(*) FROM room_favourites WHERE user_id = (SELECT id FROM users WHERE handle_key = 't_20fa')`) != 1 {
		t.Fatal("a duplicate favourite row")
	}
	if got := a.ask(map[string]any{"t": "fav", "r": "lobby"}, "mr"); got != "err:bad_request" {
		t.Fatalf("no flag: %s", got)
	}
	if got := a.ask(map[string]any{"t": "fav", "r": "no-such-room", "fav": true}, "mr"); got != "err:no_such_room" {
		t.Fatalf("unknown room: %s", got)
	}
	// The limit.
	a.ask(map[string]any{"t": "fav", "r": "abuja", "fav": true}, "mr")
	if got := a.ask(map[string]any{"t": "fav", "r": slug, "fav": true}, "mr"); got != "err:fav_full" {
		t.Fatalf("over the limit: %s", got)
	}
	// It persists: a new connection sees it. Removing it works.
	b := e.login("t_20fa")
	if fs, _, _ := myRooms(b); len(fs) != 2 || fs[0] != "abuja" {
		t.Fatalf("after logging in again: %v", fs)
	}
	a.ask(map[string]any{"t": "fav", "r": "abuja", "fav": false}, "mr")
	if fs, _, _ := myRooms(a); len(fs) != 1 || fs[0] != "lobby" {
		t.Fatalf("after removing: %v", fs)
	}
	// Two tabs toggling at once leave one answer, the same in memory and in the database.
	for i := 0; i < 10; i++ {
		a.send(map[string]any{"t": "fav", "r": slug, "fav": i%2 == 0})
		b.send(map[string]any{"t": "fav", "r": slug, "fav": i%2 == 1})
	}
	time.Sleep(600 * time.Millisecond)
	fsMem, _, _ := myRooms(a)
	inDB := e.count(`SELECT count(*) FROM room_favourites WHERE user_id = (SELECT id FROM users WHERE handle_key = 't_20fa') AND room = $1`, slug)
	inMem := 0
	for _, s := range fsMem {
		if s == slug {
			inMem = 1
		}
	}
	if inDB != inMem {
		t.Fatalf("memory says %d, the database %d", inMem, inDB)
	}
	if e.rewards("t_20fa") != before {
		t.Fatal("favourites changed points, credits or messages")
	}
}

// A room that is deleted takes its favourites and recent entries with it, and nothing stale is shown.
func TestStep20ClosedRoomLeavesNoEntry(t *testing.T) {
	e := newTestEnv(t)
	spec := e.communityRoom("t_20chost")
	slug := e.slugOf(spec, "t_20chost")
	host := e.login("t_20chost")
	host.join(slug)
	host.ask(map[string]any{"t": "fav", "r": slug, "fav": true}, "mr")
	time.Sleep(200 * time.Millisecond)
	if fs, _, rc := myRooms(host); len(fs) != 1 || len(rc) == 0 {
		t.Fatalf("before closing: %v %v", fs, rc)
	}
	if got := host.ask(map[string]any{"t": "rclose", "r": slug}, "modok"); got != "ok:modok" {
		t.Fatalf("closing: %s", got)
	}
	if fs, fav, rc := myRooms(host); len(fs) != 0 || len(fav) != 0 || len(rc) != 0 {
		t.Fatalf("a closed room is still listed: %v %v %v", fs, fav, rc)
	}
	if e.count(`SELECT count(*) FROM room_favourites WHERE room = $1`, slug)+e.count(`SELECT count(*) FROM room_visits WHERE room = $1`, slug) != 0 {
		t.Fatal("rows of a closed room stayed")
	}
}

// ---- recent rooms ----

func TestStep20RecentRooms(t *testing.T) {
	old := recentLimit
	recentLimit = 3
	defer func() { recentLimit = old }()
	e := newTestEnv(t)
	e.account("t_20r", "user")
	w := e.login("t_20r")
	before := e.rewards("t_20r")
	w.join("lobby")
	w.join("abuja")
	w.join("lobby")
	_, _, rc := myRooms(w)
	if len(rc) != 2 || rc[0] != "lobby" || rc[1] != "abuja" {
		t.Fatalf("recent rooms: %v", rc)
	}
	time.Sleep(300 * time.Millisecond)
	// They persist and follow the person to another connection.
	w2 := e.login("t_20r")
	if _, _, rc := myRooms(w2); len(rc) != 2 || rc[0] != "lobby" {
		t.Fatalf("after logging in again: %v", rc)
	}
	// Only the newest few are kept.
	w.join("lagos")
	w.join("campus")
	if _, _, rc := myRooms(w); len(rc) != 3 || rc[0] != "campus" {
		t.Fatalf("kept %v", rc)
	}
	// A refused entry leaves no entry.
	e.h.mu.Lock()
	e.h.rooms["lobby"].minTier = 4
	e.h.mu.Unlock()
	defer func() { e.h.mu.Lock(); e.h.rooms["lobby"].minTier = 0; e.h.mu.Unlock() }()
	e.account("t_20r2", "user")
	x := e.login("t_20r2")
	if got := x.ask(map[string]any{"t": "join", "r": "lobby"}, "joined"); got != "err:tier_locked" {
		t.Fatalf("a locked room: %s", got)
	}
	if _, _, rc := myRooms(x); len(rc) != 0 {
		t.Fatalf("a refused entry was remembered: %v", rc)
	}
	// Opening an old entry is the normal entry: the room's rules apply today.
	e.h.mu.Lock()
	e.h.rooms["abuja"].minTier = 4
	e.h.mu.Unlock()
	defer func() { e.h.mu.Lock(); e.h.rooms["abuja"].minTier = 0; e.h.mu.Unlock() }()
	w3 := e.login("t_20r")
	if got := w3.ask(map[string]any{"t": "join", "r": "lobby"}, "joined"); got != "err:tier_locked" {
		t.Fatalf("reopening a room that is locked now: %s", got)
	}
	if e.rewards("t_20r") != before {
		t.Fatal("recent rooms changed points, credits or messages")
	}
}

// A shortcut never works as a key. A recent entry is only a way to ask for the room: the room's own rules decide.
// Someone who was taken out of a room (the host removed them) and finds it with a password now must give it.
func TestStep20ShortcutsAreNotKeys(t *testing.T) {
	e := newTestEnv(t)
	spec := e.communityRoom("t_20khost")
	slug := e.slugOf(spec, "t_20khost")
	e.account("t_20k", "user")
	w := e.login("t_20k")
	w.join(slug)
	time.Sleep(250 * time.Millisecond)
	if _, _, rc := myRooms(w); len(rc) != 1 || rc[0] != slug {
		t.Fatalf("recent: %v", rc)
	}
	w.send(map[string]any{"t": "leave", "a": "x"})
	w.expect("left")
	// The host takes the standing right away and protects the room with a password.
	uid := mustID(t, e, "t_20k")
	if _, err := e.st.removeMember(ctxBackground(), slug, uid); err != nil {
		t.Fatal(err)
	}
	hash, _ := hashPassword("secret", 1000)
	e.h.mu.Lock()
	e.h.rooms[slug].passHash = hash
	e.h.mu.Unlock()
	if got := w.ask(map[string]any{"t": "join", "r": slug}, "joined"); got != "err:needs_invite" {
		t.Fatalf("a recent entry got past a protected room: %s", got)
	}
	// The room is protected and the person has no standing right: it is not shown to them any more.
	if _, _, rc := myRooms(w); len(rc) != 0 {
		t.Fatalf("a protected room shows in the recent rooms of someone without access: %v", rc)
	}
	if got := w.ask(map[string]any{"t": "fav", "r": slug, "fav": true}, "mr"); got != "err:no_such_room" {
		t.Fatalf("favouriting a room one may not see: %s", got)
	}
	e.h.mu.Lock()
	e.h.rooms[slug].passHash, e.h.rooms[slug].kind = "", "private"
	e.h.mu.Unlock()
	if got := w.ask(map[string]any{"t": "fav", "r": slug, "fav": true}, "mr"); got != "err:no_such_room" {
		t.Fatalf("favouriting a private room: %s", got)
	}
	host := e.login("t_20khost")
	if got := host.ask(map[string]any{"t": "fav", "r": slug, "fav": true}, "mr"); got != "ok:mr" {
		t.Fatalf("host favouriting: %s", got)
	}
	if fs, fav, _ := myRooms(host); len(fs) != 1 || len(fav) != 1 {
		t.Fatalf("host favourites: %v %v", fs, fav)
	}
}

// ---- discovery and sorting ----

func TestStep20DirectorySorting(t *testing.T) {
	e := newTestEnv(t)
	var slugs [3]string
	for i, h := range []string{"t_20s1", "t_20s2", "t_20s3"} {
		slugs[i] = e.slugOf(e.communityRoom(h), h)
	}
	// people in the room now: room 1 has three, room 3 two, room 2 one
	n := map[int]int{0: 3, 1: 1, 2: 2}
	for i := 0; i < 3; i++ {
		for j := 0; j < n[i]; j++ {
			h := "t_20sp" + strconv.Itoa(i) + strconv.Itoa(j)
			e.account(h, "user")
			e.login(h).join(slugs[i])
		}
	}
	now := time.Now()
	e.h.mu.Lock()
	// last message: room 2 newest, room 1, room 3 oldest. created: room 3 newest, room 1, room 2 oldest.
	for i, d := range []time.Duration{-10 * time.Minute, -1 * time.Minute, -30 * time.Minute} {
		e.h.rooms[slugs[i]].lastActive = now.Add(d)
	}
	for i, d := range []time.Duration{-5 * time.Hour, -9 * time.Hour, -1 * time.Hour} {
		e.h.rooms[slugs[i]].created = now.Add(d)
	}
	e.h.mu.Unlock()
	e.account("t_20sv", "user")
	v := e.login("t_20sv")
	order := func(mode string) ([]string, string) {
		quietly(v)
		v.send(map[string]any{"t": "dir", "sort": mode})
		f := v.expect("dir")
		var mine []string
		for _, s := range slugsOf(f["r"]) {
			for i, x := range slugs {
				if s == x {
					mine = append(mine, strconv.Itoa(i+1))
				}
			}
		}
		o, _ := f["o"].(string)
		return mine, o
	}
	for mode, want := range map[string]string{"active": "1 3 2", "recent": "2 1 3", "new": "3 1 2", "": "1 3 2", "bogus": "1 3 2"} {
		got, echo := order(mode)
		if strings.Join(got, " ") != want {
			t.Errorf("sort %q: rooms came in the order %v, want %s", mode, got, want)
		}
		if mode == "bogus" || mode == "" {
			if echo != "active" {
				t.Errorf("sort %q answered with %q", mode, echo)
			}
		} else if echo != mode {
			t.Errorf("sort %q answered with %q", mode, echo)
		}
	}
	// Search follows the same order.
	quietly(v)
	v.send(map[string]any{"t": "find", "n": "t_20s", "sort": "new"})
	f := v.expect("found")
	var got []string
	for _, s := range slugsOf(f["r"]) {
		for i, x := range slugs {
			if s == x {
				got = append(got, strconv.Itoa(i+1))
			}
		}
	}
	if strings.Join(got, " ") != "3 1 2" {
		t.Errorf("search in newest order: %v", got)
	}
}

func TestStep20PrivateAndPasswordRoomsStayOutOfDiscovery(t *testing.T) {
	e := newTestEnv(t)
	pub := e.slugOf(e.communityRoom("t_20dpub"), "t_20dpub")
	priv := e.slugOf(e.communityRoom("t_20dpriv"), "t_20dpriv")
	pw := e.slugOf(e.communityRoom("t_20dpw"), "t_20dpw")
	hash, _ := hashPassword("secret", 1000)
	e.h.mu.Lock()
	e.h.rooms[priv].kind = "private"
	e.h.rooms[pw].passHash = hash
	nPriv, nPw := e.h.rooms[priv].name, e.h.rooms[pw].name
	e.h.mu.Unlock()
	e.account("t_20dv", "user")
	v := e.login("t_20dv")
	for _, mode := range []string{"active", "recent", "new"} {
		quietly(v)
		v.send(map[string]any{"t": "dir", "sort": mode})
		got := slugsOf(v.expect("dir")["r"])
		has := map[string]bool{}
		for _, s := range got {
			has[s] = true
		}
		if !has[pub] || has[priv] || has[pw] {
			t.Fatalf("directory (%s) lists %v", mode, got)
		}
		quietly(v)
		v.send(map[string]any{"t": "find", "n": "t_20d", "sort": mode})
		found := slugsOf(v.expect("found")["r"])
		for _, s := range found {
			if s == priv || s == pw {
				t.Fatalf("search (%s) found a private or password room: %v", mode, found)
			}
		}
		for _, name := range []string{nPriv, nPw} {
			quietly(v)
			v.send(map[string]any{"t": "find", "n": strings.ToLower(name), "sort": mode})
			for _, s := range slugsOf(v.expect("found")["r"]) {
				if s == priv || s == pw {
					t.Fatalf("searching a private room's name found it")
				}
			}
		}
	}
}

func ctxBackground() context.Context { return context.Background() }

// ---- the member list ----

func TestStep20MemberList(t *testing.T) {
	setGraces(t, 200*time.Millisecond, 400*time.Millisecond, 200*time.Millisecond)
	e := newTestEnv(t)
	spec := e.communityRoom("t_20mhost")
	slug := e.slugOf(spec, "t_20mhost")
	for _, h := range []string{"t_20ma", "t_20mb", "t_20mc"} {
		e.account(h, "user")
	}
	host, a := e.login("t_20mhost"), e.login("t_20ma")
	host.join(slug)
	a.join(slug)
	b := e.login("t_20mb")
	b.join(slug)
	b.c.CloseNow() // member B is gone for good
	time.Sleep(700 * time.Millisecond)
	before := e.rewards("t_20ma")

	f := members(a, 0)
	st := statusOf(f)
	if st["t_20mhost"] != "on" || st["t_20ma"] != "on" || st["t_20mb"] != "off" || len(st) != 3 {
		t.Fatalf("statuses: %v", st)
	}
	if f["tot"] != float64(3) || f["on"] != float64(2) || f["more"] != false || f["r"] != slug {
		t.Fatalf("counts: %v", f)
	}
	for _, x := range f["m"].([]any) { // only what the list needs: no ids, no email, no tier, no points
		for k := range x.(map[string]any) {
			if k != "h" && k != "a" && k != "s" && k != "o" && k != "x" {
				t.Fatalf("the member list leaks %q", k)
			}
		}
	}
	if raw, _ := json.Marshal(f); strings.Contains(string(raw), "@") && strings.Contains(string(raw), ".com") {
		t.Fatalf("something that looks like an email: %s", raw)
	}
	for _, x := range f["m"].([]any) {
		if m := x.(map[string]any); m["h"] == "t_20mhost" && m["x"] != true {
			t.Fatalf("the host is not marked: %v", m)
		}
	}
	// Too many requests too fast.
	a.send(map[string]any{"t": "members", "off": 0})
	a.send(map[string]any{"t": "members", "off": 0})
	a.drain()
	// Only the room you are in: someone in another room gets that room's list, someone in none gets nothing.
	e.account("t_20mo", "user")
	o := e.login("t_20mo")
	if got := o.ask(map[string]any{"t": "members", "off": 0}, "ml"); got != "err:no_room" {
		t.Fatalf("in no room: %s", got)
	}
	o.join("lobby")
	lf := members(o, 0)
	if lf["r"] != "lobby" {
		t.Fatalf("someone in the lobby was shown %v", lf["r"])
	}
	for h := range statusOf(lf) {
		if strings.HasPrefix(h, "t_20m") && h != "t_20mo" {
			t.Fatalf("the lobby list shows a member of the private room: %s", h)
		}
	}
	// Official rooms have no members, only people who are there now.
	if st := statusOf(lf); len(st) != 1 || st["t_20mo"] != "on" || lf["tot"] != float64(1) {
		t.Fatalf("lobby list: %v", st)
	}
	// Someone who leaves the room is no longer a member and is not listed. The others are told.
	c := e.login("t_20mc")
	c.join(slug)
	members(a, 0) // a opens the list
	a.drain()
	c.send(map[string]any{"t": "rleave", "r": slug})
	if got := mpFor(a, "t_20mc", 900*time.Millisecond); len(got) == 0 || got[len(got)-1] != "drop" {
		t.Fatalf("others were not told that C left the room: %v", got)
	}
	if st := statusOf(members(a, 0)); st["t_20mc"] != "" {
		t.Fatalf("a member who left is still listed: %v", st)
	}
	if e.rewards("t_20ma") != before {
		t.Fatal("opening the member list changed points, credits or messages")
	}
}

func TestStep20MemberStatusFollowsPresenceRules(t *testing.T) {
	setGraces(t, 300*time.Millisecond, 600*time.Millisecond, 200*time.Millisecond)
	e := newTestEnv(t)
	spec := e.communityRoom("t_20phost")
	slug := e.slugOf(spec, "t_20phost")
	e.account("t_20pa", "user")
	e.account("t_20pb", "user")
	a := e.login("t_20pa")
	a.join(slug)
	members(a, 0) // a keeps the list open
	b1 := e.login("t_20pb")
	b1.join(slug)
	if got := mpFor(a, "t_20pb", 800*time.Millisecond); len(got) != 1 || got[0] != "on" {
		t.Fatalf("B arrives: %v", got)
	}
	// A second tab and closing one tab change nothing.
	b2 := e.login("t_20pb")
	b2.join(slug)
	b1.c.CloseNow()
	if got := mpFor(a, "t_20pb", 900*time.Millisecond); len(got) != 0 {
		t.Fatalf("closing one of two tabs changed the list: %v", got)
	}
	// Lost connection: nothing at once, nothing when it comes back in time.
	b2.c.CloseNow()
	if got := mpFor(a, "t_20pb", 150*time.Millisecond); len(got) != 0 {
		t.Fatalf("offline at once: %v", got)
	}
	b3 := e.login("t_20pb")
	b3.join(slug)
	if got := mpFor(a, "t_20pb", 1000*time.Millisecond); len(got) != 0 {
		t.Fatalf("a reconnect inside the grace period changed the list: %v", got)
	}
	// Gone for good: offline once, after the grace period.
	b3.c.CloseNow()
	got := mpFor(a, "t_20pb", 1200*time.Millisecond)
	if len(got) != 1 || got[0] != "off" {
		t.Fatalf("after the grace period: %v", got)
	}
	// Silence is not leaving: a member who never writes stays online for as long as the connection is open.
	e.account("t_20pq", "user")
	q := e.login("t_20pq")
	q.join(slug)
	time.Sleep(1500 * time.Millisecond) // five times the grace period, no message
	if st := statusOf(members(a, 0)); st["t_20pq"] != "on" || st["t_20pa"] != "on" {
		t.Fatalf("quiet members: %v", st)
	}
	if got := mpFor(a, "t_20pq", 200*time.Millisecond); len(got) != 0 {
		t.Fatalf("a quiet member was marked offline: %v", got)
	}
	// Moving to another room: a person is in one room at a time, so they leave this one (Step 19) and drop out of its
	// list. The saved right to come back stays in the database.
	q.join("lobby")
	if got := mpFor(a, "t_20pq", 800*time.Millisecond); len(got) == 0 || got[len(got)-1] != "drop" {
		t.Fatalf("after moving to another room: %v", got)
	}
	if st := statusOf(members(a, 0)); st["t_20pq"] != "" {
		t.Fatalf("someone who moved to another room is still listed: %v", st)
	}
	if e.count(`SELECT count(*) FROM room_members WHERE room = $1 AND user_id = (SELECT id FROM users WHERE handle_key = 't_20pq')`, slug) != 1 {
		t.Fatal("switching rooms erased the standing right")
	}
	q.send(map[string]any{"t": "leave"})
	q.expect("left")
	if got := q.ask(map[string]any{"t": "join", "r": slug}, "joined"); got != "ok:joined" {
		t.Fatalf("coming back to the room: %s", got)
	}
}

func TestStep20MemberListRespectsOnlinePrivacy(t *testing.T) {
	setGraces(t, 300*time.Millisecond, 600*time.Millisecond, 200*time.Millisecond)
	e := newTestEnv(t)
	spec := e.communityRoom("t_20vhost")
	slug := e.slugOf(spec, "t_20vhost")
	e.account("t_20va", "user")
	e.account("t_20vd", "user")
	a := e.login("t_20va")
	a.join(slug)
	e.account("t_20vl", "user")
	o := e.login("t_20vl") // someone watching the lobby
	o.join("lobby")
	d := e.login("t_20vd")
	e.setPrivacy(d, true)
	members(a, 0)
	d.join(slug)
	if got := mpFor(a, "t_20vd", 900*time.Millisecond); len(got) != 0 {
		t.Fatalf("a person who hides their online state was announced: %v", got)
	}
	if st := statusOf(members(a, 0)); st["t_20vd"] != "hid" {
		t.Fatalf("hidden member shown as %q", st["t_20vd"])
	}
	// Away and in the room look the same: nothing tells the two apart.
	d.c.CloseNow()
	if got := mpFor(a, "t_20vd", 900*time.Millisecond); len(got) != 0 {
		t.Fatalf("a hidden person leaving was announced: %v", got)
	}
	if st := statusOf(members(a, 0)); st["t_20vd"] != "hid" {
		t.Fatalf("hidden offline member shown as %q", st["t_20vd"])
	}
	// Showing and hiding while in the room is followed live.
	d2 := e.login("t_20vd")
	d2.join(slug)
	members(a, 0)
	e.setPrivacy(d2, false)
	if got := mpFor(a, "t_20vd", 700*time.Millisecond); len(got) == 0 || got[len(got)-1] != "on" {
		t.Fatalf("showing the online state: %v", got)
	}
	e.setPrivacy(d2, true)
	if got := mpFor(a, "t_20vd", 700*time.Millisecond); len(got) == 0 || got[len(got)-1] != "hid" {
		t.Fatalf("hiding it again: %v", got)
	}
	// In an official room a hidden visitor is not listed at all.
	d2.join("lobby")
	if st := statusOf(members(o, 0)); st["t_20vd"] != "" {
		t.Fatalf("a hidden visitor is listed in the lobby: %v", st)
	}
	f := members(a, 0)
	for h, s := range statusOf(f) {
		if h == "t_20vd" && s == "on" {
			t.Fatal("hidden member counted online")
		}
	}
}

func TestStep20MemberListPaging(t *testing.T) {
	e := newTestEnv(t)
	spec := e.communityRoom("t_20gh")
	slug := e.slugOf(spec, "t_20gh")
	for i := 0; i < 45; i++ {
		e.account("t_20gm"+strconv.Itoa(1000+i), "user")
	}
	if _, err := e.st.db.Exec(`INSERT INTO room_members (room, user_id) SELECT $1, id FROM users WHERE handle_key LIKE 't\_20gm%'`, slug); err != nil {
		t.Fatal(err)
	}
	e.account("t_20gv", "user")
	v := e.login("t_20gv")
	v.join(slug)
	p1 := members(v, 0)
	n1 := len(p1["m"].([]any))
	if n1 != memberPage || p1["more"] != true || p1["tot"] != float64(47) {
		t.Fatalf("first page: %d entries, more=%v, total=%v", n1, p1["more"], p1["tot"])
	}
	p2 := members(v, n1)
	seen := map[string]int{}
	for _, f := range []map[string]any{p1, p2} {
		for h := range statusOf(f) {
			seen[h]++
		}
	}
	for h, n := range seen {
		if n > 1 {
			t.Fatalf("%s listed twice", h)
		}
	}
	if len(seen) != 47 || p2["more"] != false {
		t.Fatalf("%d people over two pages, more=%v", len(seen), p2["more"])
	}
	if st := statusOf(p1); st["t_20gv"] != "on" {
		t.Fatalf("the viewer is not online in their own list: %v", st["t_20gv"])
	}
}
