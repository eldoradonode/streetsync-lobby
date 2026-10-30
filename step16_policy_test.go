package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Admin and moderator wallet policy, and staff permission regressions, over the real handlers and database.

func ledgerCount(t *testing.T, e *testEnv, id int64) int {
	return e.count(`SELECT count(*) FROM credit_ledger WHERE user_id = $1`, id)
}

func TestAdminHasNoWallet(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	adm := e.account("t_aw_adm", "admin")
	mod := e.account("t_aw_mod", "mod")
	usr := e.account("t_aw_usr", "user")
	// An admin that already holds a balance (older data): it must stay exactly as it is.
	if _, err := e.st.db.Exec(`UPDATE users SET credits = 77 WHERE id = $1`, adm.ID); err != nil {
		t.Fatal(err)
	}
	e.st.db.Exec(`INSERT INTO credit_ledger (user_id, kind, amount, balance_after, ref, note) VALUES ($1,'grant',77,77,'t_aw_old','old')`, adm.ID)
	e.st.applyCredit(ctx, usr.ID, "grant", 40, "t_aw_u", "")
	e.st.applyCredit(ctx, mod.ID, "grant", 40, "t_aw_m", "")
	ledgerBefore := ledgerCount(t, e, adm.ID)

	// The single choke point refuses every kind of movement for an admin, in both directions.
	for _, amt := range []int64{5, -5} {
		if _, _, err := e.st.applyCredit(ctx, adm.ID, "grant", amt, "t_aw_x"+time.Now().String(), ""); !errors.Is(err, errAdminWallet) {
			t.Errorf("applyCredit(%d) on an admin: %v", amt, err)
		}
	}
	if credits(t, e.st, adm.ID) != 77 || ledgerCount(t, e, adm.ID) != ledgerBefore {
		t.Fatal("an admin's old balance and ledger must be left exactly as they were")
	}
	// Welcome gift is skipped quietly and not asked for again.
	e.st.db.Exec(`UPDATE users SET welcomed = false WHERE id = $1`, adm.ID)
	if _, got, err := e.st.grantWelcome(ctx, adm.ID); err != nil || got {
		t.Errorf("welcome gift to an admin: got=%v err=%v", got, err)
	}
	if e.count(`SELECT count(*) FROM users WHERE id = $1 AND welcomed`, adm.ID) != 1 {
		t.Error("the welcome check should be settled for an admin")
	}
	// Transfers in both directions.
	if _, _, _, err := e.st.transferCredits(ctx, usr.ID, adm.ID, 5, "tip:t_aw_1", "tip_sent", "tip_received", "x", "y"); err == nil {
		t.Error("a user paid an admin")
	}
	if _, _, _, err := e.st.transferCredits(ctx, adm.ID, usr.ID, 5, "tip:t_aw_2", "tip_sent", "tip_received", "x", "y"); !errors.Is(err, errAdminWallet) {
		t.Errorf("an admin tipped a user: %v", err)
	}
	if _, _, _, err := e.st.transferCredits(ctx, mod.ID, adm.ID, 5, "tip:t_aw_3", "tip_sent", "tip_received", "x", "y"); err == nil {
		t.Error("a moderator paid an admin")
	}
	if credits(t, e.st, usr.ID) != 40 || credits(t, e.st, mod.ID) != 40 {
		t.Error("a refused transfer moved credits")
	}

	// Room fees: an admin creates rooms and enters paid rooms without touching credits.
	mk := func(slug, name string, owner int64, fee int) roomRow {
		return roomRow{Slug: slug, Name: name, Category: "Community", Kind: "paid", OwnerID: owner, Invite: randCode(8, inviteAlphabet), Sort: 1000, Fee: fee}
	}
	if _, err := e.st.createRoom(ctx, mk("t_aw-room-aaaa", "T AW Room", adm.ID, 0), 25, 1); err != nil {
		t.Fatalf("an admin creating a room: %v", err)
	}
	if credits(t, e.st, adm.ID) != 77 {
		t.Error("room creation changed an admin balance")
	}
	e.st.createRoom(ctx, mk("t_aw-paid-aaaa", "T AW Paid", usr.ID, 10), 25, 1)
	if _, charged, _, _, err := e.st.enterRoom(ctx, "t_aw-paid-aaaa", adm.ID, 10); err != nil || charged {
		t.Errorf("an admin entering a paid room: charged=%v err=%v", charged, err)
	}
	if credits(t, e.st, adm.ID) != 77 || credits(t, e.st, usr.ID) != 40-25 {
		t.Errorf("balances after the admin entered: admin=%d user=%d", credits(t, e.st, adm.ID), credits(t, e.st, usr.ID))
	}

	// Over the WebSocket: wallet, grant to an admin, tips.
	wa := e.login("t_aw_adm")
	if got := wa.ask(map[string]any{"t": "wallet"}, "wallet"); got != "err:admin_no_wallet" {
		t.Errorf("wallet for an admin: %s", got)
	}
	wa2 := wa
	if got := wa2.ask(map[string]any{"t": "mod", "a": "grant", "h": "t_aw_adm", "m": 10}, "modok"); got != "err:admin_no_wallet" && got != "err:forbidden" {
		t.Errorf("grant to an admin: %s", got)
	}
	if got := wa.ask(map[string]any{"t": "mod", "a": "grant", "h": "t_aw_usr", "m": 10}, "modok"); got != "ok:modok" {
		t.Errorf("an admin may still grant to an ordinary user: %s", got)
	}
	if credits(t, e.st, usr.ID) != 15+10 || credits(t, e.st, adm.ID) != 77 {
		t.Errorf("grant bookkeeping: user=%d admin=%d", credits(t, e.st, usr.ID), credits(t, e.st, adm.ID))
	}
	wm, wu := e.login("t_aw_mod"), e.login("t_aw_usr")
	for _, w := range []*wsc{wa, wm, wu} {
		w.join("lobby")
	}
	admMsg, usrMsg := wa.say("admin message"), wu.say("user message")
	time.Sleep(200 * time.Millisecond)
	if got := wa.ask(map[string]any{"t": "tip", "i": usrMsg, "amt": 5, "c": "tipkey-t_aw-0001"}, "tipok"); got != "err:admin_no_wallet" {
		t.Errorf("an admin tipping: %s", got)
	}
	if got := wm.ask(map[string]any{"t": "tip", "i": admMsg, "amt": 5, "c": "tipkey-t_aw-0002"}, "tipok"); got != "err:admin_no_wallet" && got != "err:staff_shield" {
		t.Errorf("a moderator tipping an admin: %s", got)
	}
	if credits(t, e.st, adm.ID) != 77 {
		t.Error("the admin balance moved")
	}
}

func TestModeratorKeepsWalletThroughPromotionAndDemotion(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	adm := e.account("t_pd_adm", "admin")
	u := e.account("t_pd_usr", "user")
	other := e.account("t_pd_oth", "user")
	e.st.applyCredit(ctx, u.ID, "grant", 60, "t_pd_g", "")
	before := ledgerCount(t, e, u.ID)
	wa := e.login("t_pd_adm")
	_ = adm
	if got := wa.ask(map[string]any{"t": "mod", "a": "promote", "h": "t_pd_usr"}, "modok"); got != "ok:modok" {
		t.Fatalf("promote: %s", got)
	}
	if credits(t, e.st, u.ID) != 60 || ledgerCount(t, e, u.ID) != before {
		t.Fatal("promotion changed a balance or ledger")
	}
	// A moderator keeps the wallet and can tip an ordinary user.
	wm, wo := e.login("t_pd_usr"), e.login("t_pd_oth")
	wm.join("lobby")
	wo.join("lobby")
	om := wo.say("hello from the other user")
	wm.sawMessage("hello from the other user")
	if got := wm.ask(map[string]any{"t": "tip", "i": om, "amt": 5, "c": "tipkey-t_pd-0001"}, "tipok"); got != "ok:tipok" {
		t.Errorf("a moderator tipping a user: %s", got)
	}
	if credits(t, e.st, u.ID) != 55 || credits(t, e.st, other.ID) < 5 {
		t.Errorf("after the tip: mod=%d other=%d", credits(t, e.st, u.ID), credits(t, e.st, other.ID))
	}
	// Staff do not earn entry fees.
	if _, err := e.st.createRoom(ctx, roomRow{Slug: "t_pd-room-aaaa", Name: "T PD Room", Category: "Community", Kind: "paid", OwnerID: u.ID, Invite: randCode(8, inviteAlphabet), Sort: 1000, Fee: 20}, 25, 1); err != nil {
		t.Fatal(err)
	}
	e.st.applyCredit(ctx, other.ID, "grant", 30, "t_pd_g2", "")
	otherBefore := credits(t, e.st, other.ID)
	if _, charged, _, got, err := e.st.enterRoom(ctx, "t_pd-room-aaaa", other.ID, 0); err != nil || charged || got != 0 {
		t.Errorf("staff-owned room entry: charged=%v host got=%d err=%v", charged, got, err)
	}
	if credits(t, e.st, other.ID) != otherBefore {
		t.Error("a staff-owned room charged an entry fee")
	}
	after := credits(t, e.st, u.ID)
	ledAfter := ledgerCount(t, e, u.ID)
	if got := wa.ask(map[string]any{"t": "mod", "a": "demote", "h": "t_pd_usr"}, "modok"); got != "ok:modok" {
		t.Fatalf("demote: %s", got)
	}
	if credits(t, e.st, u.ID) != after || ledgerCount(t, e, u.ID) != ledAfter {
		t.Error("demotion changed a balance or ledger")
	}
}

func TestModeratorCannotUseAdminOnlyActions(t *testing.T) {
	e := newTestEnv(t)
	adm := e.account("t_pm_adm", "admin")
	e.account("t_pm_mod", "mod")
	e.account("t_pm_mod2", "mod")
	e.account("t_pm_usr", "user")
	wm := e.login("t_pm_mod")
	for _, a := range []string{"promote", "demote", "ban", "unban"} {
		for _, target := range []string{"t_pm_usr", "t_pm_mod2", "t_pm_adm"} {
			if got := wm.ask(map[string]any{"t": "mod", "a": a, "h": target}, "modok"); got != "err:forbidden" {
				t.Errorf("moderator %s %s: %s", a, target, got)
			}
		}
	}
	if got := wm.ask(map[string]any{"t": "mod", "a": "grant", "h": "t_pm_usr", "m": 5}, "modok"); got != "err:forbidden" {
		t.Errorf("moderator grant: %s", got)
	}
	// Moderation actions on an admin or another moderator are refused.
	for _, a := range []string{"mute", "kick", "clearbio"} {
		for _, target := range []string{"t_pm_adm", "t_pm_mod2"} {
			got := wm.ask(map[string]any{"t": "mod", "a": a, "h": target, "m": 5}, "modok")
			if got == "ok:modok" {
				t.Errorf("moderator %s on %s was allowed", a, target)
			}
		}
	}
	if e.count(`SELECT count(*) FROM users WHERE id = $1 AND banned_at IS NULL AND role = 'admin'`, adm.ID) != 1 {
		t.Error("the admin was changed")
	}
	// Admin-only panels: the staff list and the staff action log. (Stats, users and rooms are shared with moderators.)
	for _, f := range []string{"astaff", "alog"} {
		if got := wm.ask(map[string]any{"t": f}, "x"); got != "err:forbidden" {
			t.Errorf("moderator frame %s: %s", f, got)
		}
	}
}
