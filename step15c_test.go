package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func roomEntry(w *wsc, slug string) map[string]any {
	w.t.Helper()
	w.drain()
	w.send(map[string]any{"t": "dir"})
	for _, r := range w.expect("dir")["r"].([]any) {
		if m := r.(map[string]any); m["s"] == slug {
			return m
		}
	}
	return nil
}

func TestRoomDescriptionsMembershipAndManagement(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	host := e.account("t_host", "user")
	mem := e.account("t_mem", "user")
	out := e.account("t_out", "user")
	e.account("t_rmod", "mod")
	e.account("t_radm", "admin")
	e.st.db.Exec(`UPDATE users SET points = 500 WHERE id = $1`, host.ID) // Regular tier: may create a room
	e.st.applyCredit(ctx, host.ID, "grant", 100, "g1", "")
	e.st.applyCredit(ctx, mem.ID, "grant", 100, "g2", "")
	wh, wm, wo, wmod, wadm := e.login("t_host"), e.login("t_mem"), e.login("t_out"), e.login("t_rmod"), e.login("t_radm")

	// Description is validated and bounded.
	long := strings.Repeat("x", maxRoomDescRunes+1)
	if got := wh.ask(map[string]any{"t": "croom", "n": "T Desc Room", "k": "public", "d": long}, "croomok"); got != "err:bad_description" {
		t.Fatalf("long description: %s", got)
	}
	if e.count(`SELECT count(*) FROM rooms WHERE lower(name) = 't desc room'`) != 0 || credits15(t, e, host.ID) != 100 {
		t.Fatal("a refused description created a room or charged")
	}
	wh.send(map[string]any{"t": "croom", "n": "T Desc Room", "k": "public", "d": "  A room   for\ttesting  "})
	slug := wh.expect("croomok")["s"].(string)
	var desc string
	e.st.db.QueryRow(`SELECT description FROM rooms WHERE slug = $1`, slug).Scan(&desc)
	if desc != "A room for testing" {
		t.Fatalf("stored description %q", desc)
	}
	if got := roomEntry(wo, slug); got == nil || got["d"] != "A room for testing" {
		t.Fatalf("description missing from the directory: %v", got)
	}
	wo.drain()
	wo.send(map[string]any{"t": "find", "n": "testing"}) // matches the description
	if len(wo.expect("found")["r"].([]any)) != 1 {
		t.Fatal("search should match the description")
	}
	// Private and password rooms never appear in discovery.
	h2 := e.account("t_host2", "user")
	e.st.db.Exec(`UPDATE users SET points = 500 WHERE id = $1`, h2.ID)
	e.st.applyCredit(ctx, h2.ID, "grant", 100, "g1b", "")
	wh2 := e.login("t_host2")
	wh2.send(map[string]any{"t": "croom", "n": "T Hidden", "k": "private", "d": "secret"})
	wh2.expect("croomok")
	wo.drain()
	wo.send(map[string]any{"t": "find", "n": ""})
	for _, r := range wo.expect("found")["r"].([]any) {
		if r.(map[string]any)["n"] == "T Hidden" {
			t.Fatal("private room leaked into search")
		}
	}

	// Membership: join, see the state, leave, rejoin without anything changing twice.
	wm.send(map[string]any{"t": "join", "r": slug})
	wm.expect("joined")
	time.Sleep(300 * time.Millisecond)
	if e.count(`SELECT count(*) FROM room_members WHERE room = $1 AND user_id = $2 AND left_at IS NULL`, slug, mem.ID) != 1 {
		t.Fatal("join not recorded")
	}
	wm.send(map[string]any{"t": "join", "r": slug}) // joining twice
	wm.expect("joined")
	if e.count(`SELECT count(*) FROM room_members WHERE room = $1 AND user_id = $2`, slug, mem.ID) != 1 {
		t.Fatal("duplicate membership")
	}
	if got := wh.ask(map[string]any{"t": "rleave", "r": slug}, "rleft"); got != "err:owner_cannot_leave" {
		t.Fatalf("host leaving: %s", got)
	}
	if got := wo.ask(map[string]any{"t": "rleave", "r": slug}, "rleft"); got != "err:not_member" {
		t.Fatalf("leaving without joining: %s", got)
	}
	if got := wm.ask(map[string]any{"t": "rleave", "r": "lobby"}, "rleft"); got != "err:no_such_room" {
		t.Fatalf("leaving an official room: %s", got)
	}
	if got := wm.ask(map[string]any{"t": "rleave", "r": slug}, "rleft"); got != "ok:rleft" {
		t.Fatalf("leave: %s", got)
	}
	if e.count(`SELECT count(*) FROM room_members WHERE room = $1 AND user_id = $2 AND left_at IS NOT NULL`, slug, mem.ID) != 1 {
		t.Fatal("leave not recorded")
	}

	// A fee room is charged once, however often the person leaves and returns, and under concurrent joins.
	wh.send(map[string]any{"t": "rset", "r": slug, "f": 10})
	wh.expect("rsetok")
	before := credits15(t, e, mem.ID)
	fee := 10
	for i := 0; i < 3; i++ {
		wm.send(map[string]any{"t": "join", "r": slug, "f": fee})
		wm.expect("joined")
		time.Sleep(200 * time.Millisecond)
		if got := wm.ask(map[string]any{"t": "rleave", "r": slug}, "rleft"); got != "ok:rleft" {
			t.Fatalf("leave %d: %s", i, got)
		}
	}
	// The earlier free membership keeps free entry (nothing charged), which is the safe default.
	if credits15(t, e, mem.ID) != before {
		t.Fatalf("a returning member was charged: %d -> %d", before, credits15(t, e, mem.ID))
	}
	// A brand new person pays exactly once even with parallel joins.
	e.st.applyCredit(ctx, out.ID, "grant", 100, "g3", "")
	for i := 0; i < 4; i++ {
		go wo.send(map[string]any{"t": "join", "r": slug, "f": fee})
	}
	time.Sleep(1500 * time.Millisecond)
	if n := e.count(`SELECT count(*) FROM credit_ledger WHERE user_id = $1 AND kind = 'room_entry'`, out.ID); n != 1 {
		t.Fatalf("entry fee charged %d times", n)
	}
	if n := e.count(`SELECT count(*) FROM room_members WHERE room = $1 AND user_id = $2`, slug, out.ID); n != 1 {
		t.Fatalf("%d membership rows", n)
	}
	wo.drain()
	wo.send(map[string]any{"t": "rleave", "r": slug})
	wo.expect("rleft")
	for i := 0; i < 2; i++ {
		wo.send(map[string]any{"t": "join", "r": slug, "f": fee})
		wo.expect("joined")
		wo.send(map[string]any{"t": "rleave", "r": slug})
		wo.expect("rleft")
	}
	if n := e.count(`SELECT count(*) FROM credit_ledger WHERE user_id = $1 AND kind = 'room_entry'`, out.ID); n != 1 {
		t.Fatalf("rejoining charged again: %d entries", n)
	}

	// Management page: only the host (and staff), outside the live chat.
	if got := wm.ask(map[string]any{"t": "rmgr", "r": slug}, "rmgrinfo"); got != "err:not_host" {
		t.Fatalf("member opened management: %s", got)
	}
	wh.drain()
	wh.send(map[string]any{"t": "rmgr", "r": slug})
	info := wh.expect("rmgrinfo")
	if len(info["m"].([]any)) < 2 || info["d"] != "A room for testing" {
		t.Fatalf("management data: %v", info)
	}
	if got := wh.ask(map[string]any{"t": "hmod", "a": "mute", "r": slug, "h": "t_mem", "m": 5}, "modok"); got != "ok:modok" {
		t.Fatalf("host mutes outside the chat: %s", got)
	}
	if got := wm.ask(map[string]any{"t": "hmod", "a": "mute", "r": slug, "h": "t_out"}, "modok"); got != "err:not_host" {
		t.Fatalf("non-host moderates: %s", got)
	}
	for _, target := range []string{"t_rmod", "t_radm", "t_host"} {
		if got := wh.ask(map[string]any{"t": "hmod", "a": "kick", "r": slug, "h": target}, "modok"); got != "err:forbidden" {
			t.Errorf("host kicks %s: %s", target, got)
		}
		if got := wh.ask(map[string]any{"t": "hmod", "a": "remove", "r": slug, "h": target}, "modok"); got != "err:forbidden" {
			t.Errorf("host removes %s: %s", target, got)
		}
	}
	if got := wh.ask(map[string]any{"t": "hmod", "a": "remove", "r": slug, "h": "t_mem"}, "modok"); got != "ok:modok" {
		t.Fatalf("host removes member: %s", got)
	}
	if e.count(`SELECT count(*) FROM room_members WHERE room = $1 AND user_id = $2`, slug, mem.ID) != 0 {
		t.Fatal("member not removed")
	}
	if got := wh.ask(map[string]any{"t": "rset", "r": slug, "d": long}, "rsetok"); got != "err:bad_description" {
		t.Fatalf("rset long description: %s", got)
	}
	if got := wh.ask(map[string]any{"t": "rset", "r": slug, "d": "New words"}, "rsetok"); got != "ok:rsetok" {
		t.Fatalf("rset description: %s", got)
	}
	if got := wo.ask(map[string]any{"t": "rset", "r": slug, "d": "hijack"}, "rsetok"); got != "err:not_host" {
		t.Fatalf("non-host edits room: %s", got)
	}
	if got := wo.ask(map[string]any{"t": "rclose", "r": slug}, "modok"); got != "err:not_host" {
		t.Fatalf("non-host closes room: %s", got)
	}

	// Reports about the room.
	if got := wo.ask(map[string]any{"t": "rrep", "r": slug, "c": "spam", "b": "too many ads"}, "repok"); got != "ok:repok" {
		t.Fatalf("room report: %s", got)
	}
	if got := wo.ask(map[string]any{"t": "rrep", "r": slug, "c": "spam"}, "repok"); got != "err:already_reported" {
		t.Fatalf("duplicate room report: %s", got)
	}
	if got := wh.ask(map[string]any{"t": "rrep", "r": slug, "c": "spam"}, "repok"); got != "err:self_report" {
		t.Fatalf("host reports own room: %s", got)
	}
	if got := wo.ask(map[string]any{"t": "rrep", "r": slug, "c": "nonsense"}, "repok"); got != "err:bad_report" {
		t.Fatalf("bad reason: %s", got)
	}
	var hidden string
	e.st.db.QueryRow(`SELECT slug FROM rooms WHERE name = 'T Hidden'`).Scan(&hidden)
	if got := wm.ask(map[string]any{"t": "rrep", "r": hidden, "c": "abuse"}, "repok"); got != "err:no_such_room" {
		t.Fatalf("reporting a room I can not see: %s", got)
	}
	wmod.drain()
	wmod.send(map[string]any{"t": "reports"})
	found := false
	for _, r := range wmod.expect("reps")["r"].([]any) {
		if m := r.(map[string]any); m["k"] == "room" && m["rm"] == slug {
			found = true
		}
	}
	if !found {
		t.Fatal("the room report is not in the staff queue")
	}
	wh.drain()
	wh.send(map[string]any{"t": "rmgr", "r": slug})
	info = wh.expect("rmgrinfo")
	reps := info["r"].([]any)
	if len(reps) != 1 {
		t.Fatalf("host should see the report, got %v", reps)
	}
	for k := range reps[0].(map[string]any) {
		if k == "by" || k == "h" {
			t.Fatal("reporter identity shown to the host")
		}
	}
	// Staff close the room through the same frame.
	if got := wadm.ask(map[string]any{"t": "rclose", "r": slug}, "modok"); got != "ok:modok" {
		t.Fatalf("admin closes room: %s", got)
	}
}

func TestPeopleSearchAndProfiles(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	e.account("t_sea", "user")
	b := e.account("t_seb", "mod")
	_ = b
	c := e.account("t_sec", "user")
	d := e.account("t_sed", "user")
	e.st.setBio(ctx, c.ID, "hello from c")
	e.st.db.Exec(`UPDATE users SET avatar = 'f3' WHERE id = $1`, c.ID)
	e.st.blockUser(ctx, d.ID, mustID(t, e, "t_sea")) // d blocked a
	adm := e.account("t_sead", "admin")
	e.st.db.Exec(`UPDATE users SET banned_at = now() WHERE handle_key = 't_sead'`)
	_ = adm
	wa := e.login("t_sea")
	list := func(q string) []map[string]any {
		wa.drain()
		wa.send(map[string]any{"t": "usearch", "n": q})
		var out []map[string]any
		for _, r := range wa.expect("ufound")["r"].([]any) {
			out = append(out, r.(map[string]any))
		}
		return out
	}
	res := list("t_se")
	names := map[string]bool{}
	for _, r := range res {
		names[r["h"].(string)] = true
		for k := range r {
			switch k {
			case "h", "v", "o", "r":
			default:
				t.Fatalf("search leaks field %q", k)
			}
		}
	}
	if !names["t_seb"] || !names["t_sec"] {
		t.Errorf("expected b and c: %v", names)
	}
	if names["t_sea"] || names["t_sed"] || names["t_sead"] {
		t.Errorf("self, blocked-me and banned must be hidden: %v", names)
	}
	time.Sleep(300 * time.Millisecond)
	if got := wa.ask(map[string]any{"t": "usearch", "n": "x"}, "ufound"); got != "err:bad_query" {
		t.Errorf("too short: %s", got)
	}
	time.Sleep(300 * time.Millisecond)
	if got := wa.ask(map[string]any{"t": "usearch", "n": "a%_b"}, "ufound"); got != "err:bad_query" {
		t.Errorf("wildcards: %s", got)
	}
	// Profile: public bio and avatar, never private data; staff shield flags; blocked-me hidden.
	wa.drain()
	wa.send(map[string]any{"t": "profile", "h": "t_sec"})
	p := wa.expect("uprofile")
	if p["bio"] != "hello from c" || p["v"] != "f3" {
		t.Errorf("profile: %v", p)
	}
	for k := range p {
		switch k {
		case "t", "h", "j", "on", "v", "bio", "s", "r", "p":
		default:
			t.Errorf("profile leaks %q", k)
		}
	}
	wa.drain()
	wa.send(map[string]any{"t": "profile", "h": "t_seb"})
	if p := wa.expect("uprofile"); p["sh"] != true || p["o"] != "mod" {
		t.Errorf("staff profile flags: %v", p)
	}
	// Existing decision: a person who was blocked is not told. The profile still opens, with relationship "none".
	wa.drain()
	wa.send(map[string]any{"t": "profile", "h": "t_sed"})
	if p := wa.expect("uprofile"); p["s"] != "none" {
		t.Errorf("profile of someone who blocked me: %v", p)
	}
	if strings.Contains(strings.ToLower(mustJSON(p)), "pass") {
		t.Error("profile mentions a password field")
	}
	// Guests can search too.
	g := e.guest("t_seg")
	g.send(map[string]any{"t": "usearch", "n": "t_sec"})
	g.expect("ufound")
}
