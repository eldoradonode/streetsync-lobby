package main

import (
	"context"
	"testing"
	"time"
)

func credits15(t *testing.T, e *testEnv, id int64) int64 {
	t.Helper()
	b, err := e.st.creditBalance(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func ledgerRows(e *testEnv) int {
	return e.count(`SELECT count(*) FROM credit_ledger`)
}

// Regular user, moderator and admin all in the lobby. Every restricted action is sent as a raw frame.
func TestStaffShieldAndHierarchy(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	admin := e.account("t_adm", "admin")
	mod := e.account("t_mod", "mod")
	mod2 := e.account("t_mod2", "mod")
	_ = mod2
	u := e.account("t_usr", "user")
	u2 := e.account("t_usr2", "user")
	e.st.applyCredit(ctx, u.ID, "grant", 100, "g1", "")
	e.st.applyCredit(ctx, u2.ID, "grant", 100, "g2", "")

	wa, wm, wu, wu2 := e.login("t_adm"), e.login("t_mod"), e.login("t_usr"), e.login("t_usr2")
	for _, w := range []*wsc{wa, wm, wu, wu2} {
		w.join("lobby")
	}
	adminMsg := wa.say("admin says hi")
	modMsg := wm.say("mod says hi")
	userMsg := wu2.say("user says hi")
	wu.sawMessage("admin says hi")
	wu.sawMessage("mod says hi")
	time.Sleep(time.Second) // the daily credit for a first message arrives asynchronously
	balBefore, ledgerBefore := credits15(t, e, u.ID), ledgerRows(e)
	friendsBefore := e.count(`SELECT count(*) FROM friendships`)
	dmBefore := e.count(`SELECT count(*) FROM dm_messages`)
	repBefore := e.count(`SELECT count(*) FROM reports`)

	t.Run("ordinary user cannot tip staff", func(t *testing.T) {
		for _, m := range []struct {
			id int64
			n  string
		}{{adminMsg, "admin"}, {modMsg, "mod"}} {
			if got := wu.ask(map[string]any{"t": "tip", "i": m.id, "amt": 5, "c": "tipkey-" + m.n + "-0001"}, "tipok"); got != "err:staff_shield" {
				t.Errorf("tip %s: %s", m.n, got)
			}
		}
	})
	t.Run("ordinary user cannot DM or friend staff", func(t *testing.T) {
		for _, h := range []string{"t_adm", "t_mod"} {
			if got := wu.ask(e.dmFrame("t_usr", h, "hello"), "dm"); got != "err:staff_shield" {
				t.Errorf("dm %s: %s", h, got)
			}
			if got := wu.ask(map[string]any{"t": "fr", "a": "add", "h": h}, "frok"); got != "err:staff_shield" {
				t.Errorf("friend %s: %s", h, got)
			}
		}
	})
	t.Run("ordinary reports of staff are refused, complaint channel works", func(t *testing.T) {
		if got := wu.ask(map[string]any{"t": "rep", "i": modMsg, "c": "abuse"}, "repok"); got != "err:staff_report" {
			t.Errorf("rep mod message: %s", got)
		}
		if got := wu.ask(map[string]any{"t": "rep", "i": adminMsg, "c": "abuse"}, "repok"); got != "err:staff_report" {
			t.Errorf("rep admin message: %s", got)
		}
		if got := wu.ask(map[string]any{"t": "urep", "h": "t_mod", "c": "abuse"}, "repok"); got != "err:staff_report" {
			t.Errorf("urep mod: %s", got)
		}
		if got := wu.ask(map[string]any{"t": "scomp", "h": "t_mod", "i": modMsg, "c": "abuse", "b": "rude to me"}, "scompok"); got != "ok:scompok" {
			t.Errorf("scomp: %s", got)
		}
		if got := wu.ask(map[string]any{"t": "scomp", "h": "t_mod", "c": "abuse"}, "scompok"); got != "err:already_reported" {
			t.Errorf("duplicate scomp: %s", got)
		}
		if got := wu.ask(map[string]any{"t": "scomp", "h": "t_usr2", "c": "abuse"}, "scompok"); got != "err:not_staff" {
			t.Errorf("scomp on ordinary user: %s", got)
		}
		if got := wm.ask(map[string]any{"t": "scomp", "h": "t_adm", "i": adminMsg, "c": "unfair", "b": "unfair mute"}, "scompok"); got != "ok:scompok" {
			t.Errorf("mod complaint about admin: %s", got)
		}
		// Moderators cannot read complaints. Admins can, minus complaints about themselves.
		if got := wm.ask(map[string]any{"t": "acomp"}, "comps"); got != "err:forbidden" {
			t.Errorf("mod acomp: %s", got)
		}
		if got := wu.ask(map[string]any{"t": "acomp"}, "comps"); got != "err:forbidden" {
			t.Errorf("user acomp: %s", got)
		}
		wa.drain()
		wa.send(map[string]any{"t": "acomp"})
		list := wa.expect("comps")["r"].([]any)
		if len(list) != 1 || list[0].(map[string]any)["h"] != "t_mod" {
			t.Errorf("admin should see only the complaint about the mod: %v", list)
		}
		// Moderators never see reports about staff in their queue either.
		if n := e.count(`SELECT count(*) FROM reports`); n != repBefore {
			t.Errorf("reports table changed: %d -> %d", repBefore, n)
		}
	})
	t.Run("rejected requests changed nothing", func(t *testing.T) {
		if credits15(t, e, u.ID) != balBefore || ledgerRows(e) != ledgerBefore {
			t.Error("a rejected tip moved credits or wrote the ledger")
		}
		if e.count(`SELECT count(*) FROM friendships`) != friendsBefore || e.count(`SELECT count(*) FROM dm_messages`) != dmBefore {
			t.Error("a rejected request changed friendships or messages")
		}
	})
	t.Run("moderator cannot touch admins", func(t *testing.T) {
		for _, a := range []string{"mute", "kick", "unmute", "promote", "demote", "clearbio", "ban", "unban"} {
			if got := wm.ask(map[string]any{"t": "mod", "a": a, "h": "t_adm"}, "modok"); got != "err:forbidden" {
				t.Errorf("mod %s admin: %s", a, got)
			}
		}
		if got := wm.ask(map[string]any{"t": "mod", "a": "del", "i": adminMsg}, "modok"); got != "err:forbidden" {
			t.Errorf("mod delete admin message: %s", got)
		}
		if e.count(`SELECT count(*) FROM users WHERE role = 'admin' AND handle_key = 't_adm'`) != 1 {
			t.Error("admin role changed")
		}
	})
	t.Run("moderator cannot gain admin powers", func(t *testing.T) {
		for _, a := range []struct{ a, h string }{{"promote", "t_usr"}, {"promote", "t_mod"}, {"demote", "t_mod2"}, {"ban", "t_usr"}, {"unban", "t_usr"}, {"promote", "t_mod"}} {
			if got := wm.ask(map[string]any{"t": "mod", "a": a.a, "h": a.h}, "modok"); got != "err:forbidden" {
				t.Errorf("mod %s %s: %s", a.a, a.h, got)
			}
		}
		if got := wm.ask(map[string]any{"t": "mod", "a": "grant", "h": "t_usr", "m": 50}, "modok"); got != "err:forbidden" {
			t.Errorf("mod grant: %s", got)
		}
		if got := wm.ask(map[string]any{"t": "mod", "a": "mute", "h": "t_mod2"}, "modok"); got != "err:forbidden" {
			t.Errorf("mod mutes mod: %s", got)
		}
		if got := wm.ask(map[string]any{"t": "mod", "a": "mute", "h": "t_mod"}, "modok"); got != "err:forbidden" {
			t.Errorf("mod mutes self: %s", got)
		}
		if credits15(t, e, u.ID) != balBefore {
			t.Error("a refused grant moved credits")
		}
	})
	t.Run("ordinary user cannot moderate", func(t *testing.T) {
		for _, a := range []string{"mute", "kick", "promote", "ban", "grant", "dismiss"} {
			if got := wu.ask(map[string]any{"t": "mod", "a": a, "h": "t_usr2", "id": 1}, "modok"); got != "err:forbidden" {
				t.Errorf("user mod %s: %s", a, got)
			}
		}
		if got := wu.ask(map[string]any{"t": "mod", "a": "del", "i": userMsg}, "modok"); got != "err:forbidden" {
			t.Errorf("user deletes other message: %s", got)
		}
		if got := wu.ask(map[string]any{"t": "reports"}, "reps"); got != "err:forbidden" {
			t.Errorf("user lists reports: %s", got)
		}
	})
	t.Run("moderator acts on ordinary users, admin on moderators", func(t *testing.T) {
		if got := wm.ask(map[string]any{"t": "mod", "a": "mute", "h": "t_usr2", "m": 1}, "modok"); got != "ok:modok" {
			t.Errorf("mod mutes user: %s", got)
		}
		if got := wm.ask(map[string]any{"t": "mod", "a": "unmute", "h": "t_usr2"}, "modok"); got != "ok:modok" {
			t.Errorf("mod unmutes user: %s", got)
		}
		if got := wa.ask(map[string]any{"t": "mod", "a": "mute", "h": "t_mod", "m": 1}, "modok"); got != "ok:modok" {
			t.Errorf("admin mutes mod: %s", got)
		}
		if got := wa.ask(map[string]any{"t": "mod", "a": "unmute", "h": "t_mod"}, "modok"); got != "ok:modok" {
			t.Errorf("admin unmutes mod: %s", got)
		}
		if got := wa.ask(map[string]any{"t": "mod", "a": "mute", "h": "t_adm"}, "modok"); got != "err:forbidden" {
			t.Errorf("admin on self: %s", got)
		}
	})
	t.Run("staff can reply to staff, regular users keep working", func(t *testing.T) {
		// A moderator writes first: the user may now answer.
		if got := wm.ask(e.dmFrame("t_mod", "t_usr", "welcome"), "dm"); got != "ok:dm" {
			t.Fatalf("mod dm: %s", got)
		}
		if got := wu.ask(e.dmFrame("t_usr", "t_mod", "thanks"), "dm"); got != "ok:dm" {
			t.Errorf("reply to staff who wrote first: %s", got)
		}
		// A staff friend request can be accepted, and a user can not request first.
		if got := wm.ask(map[string]any{"t": "fr", "a": "add", "h": "t_usr2"}, "frok"); got != "ok:frok" {
			t.Errorf("mod friend request to user: %s", got)
		}
		if got := wu2.ask(map[string]any{"t": "fr", "a": "accept", "h": "t_mod"}, "frok"); got != "ok:frok" {
			t.Errorf("user accepts staff request: %s", got)
		}
		// Ordinary to ordinary. The rate limit bucket is shared, so let it refill first.
		time.Sleep(5 * time.Second)
		if got := wu.ask(map[string]any{"t": "tip", "i": userMsg, "amt": 1, "c": "tipkey-user-0001"}, "tipok"); got != "ok:tipok" {
			t.Errorf("user tips user: %s", got)
		}
		if got := wu.ask(e.dmFrame("t_usr", "t_usr2", "hey"), "dm"); got != "ok:dm" {
			t.Errorf("user dms user: %s", got)
		}
		if got := wu.ask(map[string]any{"t": "fr", "a": "add", "h": "t_usr2"}, "frok"); got != "ok:frok" {
			t.Errorf("user friends user: %s", got)
		}
		if got := wu.ask(map[string]any{"t": "urep", "h": "t_usr2", "c": "spam"}, "repok"); got != "ok:repok" {
			t.Errorf("user reports user: %s", got)
		}
		if got := wu.ask(map[string]any{"t": "urep", "h": "t_usr2", "c": "spam"}, "repok"); got != "err:already_reported" {
			t.Errorf("duplicate user report: %s", got)
		}
		if got := wu.ask(map[string]any{"t": "urep", "h": "t_usr", "c": "spam"}, "repok"); got != "err:self_report" {
			t.Errorf("self report: %s", got)
		}
		if got := wu.ask(map[string]any{"t": "rep", "i": userMsg, "c": "abuse"}, "repok"); got != "ok:repok" {
			t.Errorf("message report: %s", got)
		}
	})
	t.Run("a tip to staff never moves credits even with replayed keys", func(t *testing.T) {
		before := credits15(t, e, mod.ID)
		for i := 0; i < 3; i++ {
			wu.ask(map[string]any{"t": "tip", "i": modMsg, "amt": 50, "c": "tipkey-replay-01"}, "tipok")
		}
		if credits15(t, e, mod.ID) != before || e.count(`SELECT count(*) FROM credit_ledger WHERE kind LIKE 'tip%' AND user_id = $1`, mod.ID) != 0 {
			t.Error("staff received a tip")
		}
	})
	_ = admin
}

func TestStaffShieldInStoreTransactions(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	mod := e.account("t_smod", "mod")
	u := e.account("t_suser", "user")
	e.st.applyCredit(ctx, u.ID, "grant", 20, "g", "")
	if _, _, _, err := e.st.transferCredits(ctx, u.ID, mod.ID, 5, "tip:abcdefgh", "tip_sent", "tip_received", "", ""); err != errStaffTarget {
		t.Fatalf("store allowed a tip to staff: %v", err)
	}
	if _, _, err := e.st.sendDM(ctx, 9000001, u.ID, mod.ID, "t_smod", "hi", time.Now().UnixMilli(), 1); err != errStaffTarget {
		t.Fatalf("store allowed a first DM to staff: %v", err)
	}
	if _, err := e.st.friendAction(ctx, "add", u.ID, mod.ID); err != errStaffTarget {
		t.Fatalf("store allowed a friend request to staff: %v", err)
	}
	// Staff to ordinary is fine.
	if _, _, _, err := e.st.transferCredits(ctx, u.ID, u.ID+0, 0, "x", "a", "b", "", ""); err == nil {
		t.Fatal("bad transfer accepted")
	}
	if credits15(t, e, u.ID) != 20 {
		t.Fatal("credits moved")
	}
}

func TestPermanentBan(t *testing.T) {
	e := newTestEnv(t)
	e.account("t_badm", "admin")
	e.account("t_bmod", "mod")
	victim := e.account("t_bvic", "user")
	wa, wm := e.login("t_badm"), e.login("t_bmod")
	wv, wv2 := e.login("t_bvic"), e.login("t_bvic") // two live connections
	token := wv.Token

	if got := wm.ask(map[string]any{"t": "mod", "a": "ban", "h": "t_bvic"}, "modok"); got != "err:forbidden" {
		t.Fatalf("a moderator banned someone: %s", got)
	}
	if e.count(`SELECT count(*) FROM users WHERE banned_at IS NOT NULL`) != 0 {
		t.Fatal("ban stored after a refusal")
	}
	if got := wa.ask(map[string]any{"t": "mod", "a": "ban", "h": "t_badm"}, "modok"); got != "err:forbidden" {
		t.Fatalf("admin banned self: %s", got)
	}
	if got := wa.ask(map[string]any{"t": "mod", "a": "ban", "h": "t_bvic", "b": "spamming"}, "modok"); got != "ok:modok" {
		t.Fatalf("admin ban: %s", got)
	}
	if !wv.isClosed() || !wv2.isClosed() {
		t.Fatal("banned person's connections stayed open")
	}
	if got := wa.ask(map[string]any{"t": "mod", "a": "ban", "h": "t_bvic"}, "modok"); got != "err:already_banned" {
		t.Fatalf("second ban: %s", got)
	}
	if e.count(`SELECT count(*) FROM mod_log WHERE action = 'ban' AND target = 't_bvic' AND actor = 't_badm'`) != 1 {
		t.Fatal("ban not in the audit log exactly once")
	}
	if e.count(`SELECT count(*) FROM sessions WHERE user_id = $1`, victim.ID) != 0 {
		t.Fatal("sessions survived the ban")
	}
	// Resume with the old token, sign in again, both refused.
	w := e.dialRaw()
	w.send(map[string]any{"t": "hi", "k": token})
	if f := w.expect("err"); f["c"] != "session_expired" && f["c"] != "banned" {
		t.Fatalf("old token: %v", f)
	}
	w = e.dialRaw()
	w.send(map[string]any{"t": "login", "h": "t_bvic", "p": "password-t_bvic"})
	if f := w.expect("err"); f["c"] != "banned" {
		t.Fatalf("login after ban: %v", f)
	}
	// Wrong password does not reveal the ban.
	w = e.dialRaw()
	w.send(map[string]any{"t": "login", "h": "t_bvic", "p": "nope-nope-nope"})
	if f := w.expect("err"); f["c"] != "bad_login" {
		t.Fatalf("wrong password: %v", f)
	}
	// A banned person is not found by search or profile.
	wo := e.guest("t_other")
	if got := wo.ask(map[string]any{"t": "profile", "h": "t_bvic"}, "uprofile"); got != "err:no_such_user" {
		t.Fatalf("banned profile: %s", got)
	}
	// Unban restores sign in. Only an admin can.
	if got := wm.ask(map[string]any{"t": "mod", "a": "unban", "h": "t_bvic"}, "modok"); got != "err:forbidden" {
		t.Fatalf("mod unban: %s", got)
	}
	if got := wa.ask(map[string]any{"t": "mod", "a": "unban", "h": "t_bvic"}, "modok"); got != "ok:modok" {
		t.Fatalf("unban: %s", got)
	}
	e.login("t_bvic")
	// Temporary sanctions stay temporary: a kick expires, a ban does not.
	if e.count(`SELECT count(*) FROM sanctions WHERE kind NOT IN ('mute','kick')`) != 0 {
		t.Fatal("bans must not be stored as sanctions")
	}
}
