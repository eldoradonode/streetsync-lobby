package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAccountDeletion(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	e.account("t_dadm", "admin")
	del := e.account("t_del", "user")
	buddy := e.account("t_buddy", "user")
	e.st.applyCredit(ctx, del.ID, "grant", 40, "g-del", "")
	e.st.applyCredit(ctx, buddy.ID, "grant", 40, "g-buddy", "")
	// Data that must be handled: a friendship, direct messages, a block, a room with a member, a report, a tip.
	e.st.friendAction(ctx, "add", del.ID, buddy.ID)
	e.st.friendAction(ctx, "accept", buddy.ID, del.ID)
	if _, _, err := e.st.sendDM(ctx, 8100001, del.ID, buddy.ID, "t_buddy", "bye", time.Now().UnixMilli(), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.createRoom(ctx, roomRow{Slug: "t_delroom-aaaa", Name: "t del room", Category: "General", Sort: 1000, Kind: "public", OwnerID: del.ID, Invite: "ABCDEFGH"}, 0, 5); err != nil {
		t.Fatal(err)
	}
	e.st.joinMember(ctx, "t_delroom-aaaa", buddy.ID)
	e.st.addReport(ctx, reportRow{ReporterID: del.ID, ReporterHandle: "t_del", MsgID: 8200001, TargetHandle: "t_buddy", Body: "x", Reason: "spam"})
	if _, _, _, err := e.st.transferCredits(ctx, buddy.ID, del.ID, 5, "tip:abcdefg1", "tip_sent", "tip_received", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := e.h.load(ctx); err != nil { // so the room exists in memory
		t.Fatal(err)
	}
	ledgerSumBefore := e.count(`SELECT count(*) FROM credit_ledger`)

	w1, w2, wb := e.login("t_del"), e.login("t_del"), e.login("t_buddy")
	wb.join("lobby")
	w1.join("lobby")
	w1.say("a message that must disappear")
	wb.sawMessage("a message that must disappear")
	time.Sleep(1200 * time.Millisecond)

	// Wrong password, missing token, wrong confirm word: nothing changes.
	if got := w1.ask(map[string]any{"t": "delreq", "p": "wrong-password"}, "delready"); got != "err:bad_password" {
		t.Fatalf("wrong password: %s", got)
	}
	if got := w1.ask(map[string]any{"t": "delgo", "k": "forged", "c": "DELETE"}, "delok"); got != "err:delete_expired" {
		t.Fatalf("forged token: %s", got)
	}
	// Another user can not use someone else's token.
	w1.send(map[string]any{"t": "delreq", "p": "password-t_del"})
	tok := w1.expect("delready")["k"].(string)
	if got := wb.ask(map[string]any{"t": "delgo", "k": tok, "c": "DELETE"}, "delok"); got != "err:delete_expired" {
		t.Fatalf("another account used the token: %s", got)
	}
	if got := w1.ask(map[string]any{"t": "delgo", "k": tok, "c": "delete"}, "delok"); got != "err:bad_confirm" {
		t.Fatalf("wrong confirm word: %s", got)
	}
	if e.count(`SELECT count(*) FROM users WHERE id = $1 AND deleted_at IS NULL`, del.ID) != 1 {
		t.Fatal("account changed before the second stage")
	}
	// An admin can not delete the admin account this way.
	wa := e.login("t_dadm")
	wa.send(map[string]any{"t": "delreq", "p": "password-t_dadm"})
	if f := wa.expect("err"); f["c"] != "admin_cannot_delete" {
		t.Fatalf("admin delete: %v", f)
	}

	// The real thing.
	w1.send(map[string]any{"t": "delreq", "p": "password-t_del"})
	tok = w1.expect("delready")["k"].(string)
	w1.send(map[string]any{"t": "delgo", "k": tok, "c": "DELETE"})
	ok := w1.expect("delok")
	if ok["n"].(float64) != 1 {
		t.Errorf("expected 1 closed room, got %v", ok["n"])
	}
	testTombs = append(testTombs, del.ID)
	if !w1.isClosed() || !w2.isClosed() {
		t.Fatal("connections of the deleted account stayed open")
	}
	// Replaying the same token does nothing (and the connection is gone anyway).
	if got := w1.ask(map[string]any{"t": "delgo", "k": tok, "c": "DELETE"}, "delok"); got == "ok:delok" {
		t.Fatal("token worked twice")
	}

	// Tombstone, anonymised.
	var handle, key string
	var hasPass bool
	var credits int64
	var deletedAt *time.Time
	if err := e.st.db.QueryRow(`SELECT handle, handle_key, pass_hash IS NOT NULL, credits, deleted_at FROM users WHERE id = $1`, del.ID).Scan(&handle, &key, &hasPass, &credits, &deletedAt); err != nil {
		t.Fatal(err)
	}
	if handle == "t_del" || key == "t_del" || hasPass || credits != 0 || deletedAt == nil {
		t.Fatalf("not anonymised: %q %q pass=%v credits=%d", handle, key, hasPass, credits)
	}
	// Personal content gone.
	for q, want := range map[string]int{
		`SELECT count(*) FROM messages WHERE user_id = $1`:                                  0,
		`SELECT count(*) FROM dm_messages WHERE sender_id = $1 OR recipient_id = $1`:        0,
		`SELECT count(*) FROM friendships WHERE user_a = $1 OR user_b = $1`:                 0,
		`SELECT count(*) FROM sessions WHERE user_id = $1`:                                  0,
		`SELECT count(*) FROM room_members WHERE user_id = $1`:                              0,
		`SELECT count(*) FROM rooms WHERE owner_id = $1`:                                    0,
		`SELECT count(*) FROM reports WHERE reporter_id = $1 AND reporter_handle = 't_del'`: 0,
	} {
		if got := e.count(q, del.ID); got != want {
			t.Errorf("%s = %d, want %d", q, got, want)
		}
	}
	if e.count(`SELECT count(*) FROM rooms WHERE slug = 't_delroom-aaaa'`) != 0 || e.count(`SELECT count(*) FROM room_members WHERE room = 't_delroom-aaaa'`) != 0 {
		t.Error("the owned room or its members remain")
	}
	if e.count(`SELECT count(*) FROM reports WHERE reporter_id = $1`, del.ID) != 1 {
		t.Error("the report should be kept, anonymised")
	}
	// Ledger preserved and still adds up.
	if e.count(`SELECT count(*) FROM credit_ledger WHERE user_id = $1`, del.ID) < 4 {
		t.Error("ledger rows were lost")
	}
	if n := e.count(`SELECT coalesce(sum(amount), 0) FROM credit_ledger WHERE user_id = $1`, del.ID); n != 0 {
		t.Errorf("ledger no longer adds up to the closed balance: %d", n)
	}
	if e.count(`SELECT count(*) FROM credit_ledger WHERE user_id = $1 AND kind = 'account_closed'`, del.ID) != 1 {
		t.Error("closing balance row missing or duplicated")
	}
	if e.count(`SELECT count(*) FROM credit_ledger`) < ledgerSumBefore {
		t.Error("total ledger shrank")
	}
	// A user row with ledger rows can not be hard deleted, by design.
	if _, err := e.st.db.Exec(`DELETE FROM users WHERE id = $1`, del.ID); err == nil {
		t.Fatal("the database allowed deleting a user that has ledger rows")
	}
	// Foreign key integrity.
	for _, q := range []string{
		`SELECT count(*) FROM credit_ledger l LEFT JOIN users u ON u.id = l.user_id WHERE u.id IS NULL`,
		`SELECT count(*) FROM dm_messages d LEFT JOIN users u ON u.id = d.sender_id WHERE u.id IS NULL`,
		`SELECT count(*) FROM rooms r WHERE r.kind <> 'official' AND r.owner_id IS NULL`,
		`SELECT count(*) FROM room_members m LEFT JOIN rooms r ON r.slug = m.room WHERE r.slug IS NULL`,
	} {
		if n := e.count(q); n != 0 {
			t.Errorf("orphans: %s -> %d", q, n)
		}
	}
	// The buddy was told the room closed, and nothing of the deleted user is left in memory.
	wb.expect("rclosed")
	e.h.mu.Lock()
	_, still := e.h.users[del.ID]
	e.h.mu.Unlock()
	if still {
		t.Error("deleted user still in memory")
	}
	// Dead tokens and logins.
	w := e.dialRaw()
	w.send(map[string]any{"t": "hi", "k": w1.Token})
	if f := w.expect("err"); f["c"] != "session_expired" {
		t.Errorf("old token: %v", f)
	}
	w = e.dialRaw()
	w.send(map[string]any{"t": "login", "h": "t_del", "p": "password-t_del"})
	if f := w.expect("err"); f["c"] != "bad_login" {
		t.Errorf("login: %v", f)
	}
	// The old handle is free again, and the tombstone name can not be chosen.
	e.guest("t_del")
	n := e.dialRaw()
	n.send(map[string]any{"t": "hi", "h": handle})
	if f := n.expect("err"); f["c"] != "bad_handle" {
		t.Errorf("tombstone handle: %v", f)
	}
	// Tips to the tombstone do nothing.
	if _, _, _, err := e.st.transferCredits(ctx, buddy.ID, del.ID, 5, "tip:abcdefg2", "tip_sent", "tip_received", "", ""); err == nil {
		t.Error("a deleted account can still be paid")
	}
}

func TestAccountDeletionRetriesAndConcurrency(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	u := e.account("t_dret", "user")
	e.st.applyCredit(ctx, u.ID, "grant", 10, "g", "")
	testTombs = append(testTombs, u.ID)
	var wg sync.WaitGroup
	results := make(chan deleteResult, 6)
	errs := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := e.st.deleteAccount(ctx, u.ID)
			results <- r
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("a retry failed: %v", err)
		}
	}
	fresh := 0
	for r := range results {
		if !r.AlreadyDone {
			fresh++
		}
	}
	if fresh != 1 {
		t.Fatalf("%d runs did the work, want exactly 1", fresh)
	}
	if e.count(`SELECT count(*) FROM credit_ledger WHERE user_id = $1 AND kind = 'account_closed'`, u.ID) != 1 {
		t.Fatal("closing row duplicated by concurrent retries")
	}
	if _, err := e.st.deleteAccount(ctx, 999999999); err != errNotFound {
		t.Fatalf("unknown account: %v", err)
	}
}

func TestGuestDeletionAndPrune(t *testing.T) {
	e := newTestEnv(t)
	g := e.guest("t_gst")
	g.send(map[string]any{"t": "delreq", "h": "someone_else"})
	if f := g.expect("err"); f["c"] != "bad_confirm" {
		t.Fatalf("guest without typing the handle: %v", f)
	}
	g.send(map[string]any{"t": "delreq", "h": "T_Gst"})
	tok := g.expect("delready")["k"].(string)
	g.send(map[string]any{"t": "delgo", "k": tok, "c": "DELETE"})
	g.expect("delok")
	var id int64
	e.st.db.QueryRow(`SELECT id FROM users WHERE handle LIKE 'deleted\_%' AND deleted_at IS NOT NULL ORDER BY id DESC LIMIT 1`).Scan(&id)
	testTombs = append(testTombs, id)
	// Pruning idle guests must neither fail on ledger rows nor touch tombstones or paid history.
	ctx := context.Background()
	idle := e.account("t_idle", "user")
	e.st.db.Exec(`UPDATE users SET pass_hash = NULL, last_seen = now() - interval '40 days' WHERE id = $1`, idle.ID)
	e.st.applyCredit(ctx, idle.ID, "welcome", 20, "welcome", "")
	paid := e.account("t_paid", "user")
	e.st.db.Exec(`UPDATE users SET pass_hash = NULL, last_seen = now() - interval '40 days' WHERE id = $1`, paid.ID)
	e.st.applyCredit(ctx, paid.ID, "tip_received", 5, "tipx", "")
	if _, _, _, err := e.st.prune(ctx, 7, 14); err != nil {
		t.Fatalf("prune failed: %v", err)
	}
	if e.count(`SELECT count(*) FROM users WHERE id = $1`, idle.ID) != 0 {
		t.Error("idle guest with only free credits should be pruned")
	}
	if e.count(`SELECT count(*) FROM users WHERE id = $1`, paid.ID) != 1 || e.count(`SELECT count(*) FROM credit_ledger WHERE user_id = $1`, paid.ID) != 1 {
		t.Error("a guest with paid ledger history must be kept")
	}
	if e.count(`SELECT count(*) FROM users WHERE id = $1`, id) != 1 {
		t.Error("prune removed a tombstone")
	}
}
