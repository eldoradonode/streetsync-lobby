package main

import (
	"context"
	"os"
	"testing"
	"time"
)

// These tests need a real Postgres. Set TEST_DATABASE_URL to run them, for example:
//
//	TEST_DATABASE_URL="postgres://user:pass@localhost/db?sslmode=disable" go test ./...
//
// They create and delete rows with handles that start with "t_".
func testStore(t *testing.T) *store {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	st, err := openStore(context.Background(), url, 30)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = st.db.Exec(`DELETE FROM users WHERE handle_key LIKE 't\_%'`)
	_, _ = st.db.Exec(`DELETE FROM messages WHERE room = 'testroom'`)
	t.Cleanup(func() {
		_, _ = st.db.Exec(`DELETE FROM users WHERE handle_key LIKE 't\_%'`)
		_, _ = st.db.Exec(`DELETE FROM messages WHERE room = 'testroom'`)
		st.close()
	})
	return st
}

func TestStoreUsersAndSessions(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	u, created, err := st.createGuest(ctx, "T_Ann", "t_ann")
	if err != nil || !created || u.ID == 0 || u.Handle != "T_Ann" || u.PassHash.Valid {
		t.Fatalf("createGuest: %+v created=%v err=%v", u, created, err)
	}
	if _, created, _ := st.createGuest(ctx, "t_ANN", "t_ann"); created {
		t.Error("second claim of the same handle must fail")
	}

	tok, err := st.newSession(ctx, u.ID)
	if err != nil || len(tok) != 64 {
		t.Fatalf("newSession: %q %v", tok, err)
	}
	var stored string
	if err := st.db.QueryRow(`SELECT token_hash FROM sessions WHERE user_id=$1`, u.ID).Scan(&stored); err != nil || stored == tok || stored != tokenHash(tok) {
		t.Errorf("token must be stored hashed, got %q (err %v)", stored, err)
	}
	got, err := st.userBySession(ctx, tok)
	if err != nil || got.ID != u.ID {
		t.Fatalf("userBySession: %+v %v", got, err)
	}
	if _, err := st.userBySession(ctx, "nope"); err != errNotFound {
		t.Errorf("unknown token: %v", err)
	}
	// An unused session expires.
	if _, err := st.db.Exec(`UPDATE sessions SET last_used = now() - interval '40 days' WHERE user_id=$1`, u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.userBySession(ctx, tok); err != errNotFound {
		t.Errorf("expired session should be gone, got %v", err)
	}

	ok, err := st.setPassword(ctx, u.ID, "hash1")
	if err != nil || !ok {
		t.Fatalf("setPassword: %v %v", ok, err)
	}
	if ok, _ := st.setPassword(ctx, u.ID, "hash2"); ok {
		t.Error("password must not be replaceable through setPassword")
	}
	byKey, err := st.userByKey(ctx, "t_ann")
	if err != nil || !byKey.PassHash.Valid || byKey.PassHash.String != "hash1" {
		t.Errorf("userByKey: %+v %v", byKey, err)
	}
	if _, err := st.userByKey(ctx, "t_missing"); err != errNotFound {
		t.Errorf("missing user: %v", err)
	}

	if err := st.saveUsers(ctx, []userSave{{ID: u.ID, Points: 321, StreakDays: 3, StreakDay: 20000}}); err != nil {
		t.Fatal(err)
	}
	again, _ := st.userByKey(ctx, "t_ann")
	if again.Points != 321 || again.StreakDays != 3 || again.StreakDay != 20000 {
		t.Errorf("saveUsers did not stick: %+v", again)
	}
}

func TestStoreMessages(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	base := time.Now().UnixMilli()
	msgs := []storedMsg{
		{ID: 9000001, UserID: 1, Handle: "t_a", Tier: 0, Body: "first, with \"quotes\" and a comma", TS: base},
		{ID: 9000002, UserID: 2, Handle: "t_b", Tier: 2, Body: "second {braces} and \\ backslash and emoji \U0001F600", TS: base + 5},
		{ID: 9000003, UserID: 1, Handle: "t_a", Tier: 1, Body: "third", TS: base + 9},
	}
	if err := st.insertMessages(ctx, "testroom", msgs); err != nil {
		t.Fatal(err)
	}
	if err := st.insertMessages(ctx, "testroom", msgs[:1]); err != nil {
		t.Fatalf("duplicate insert should be ignored: %v", err)
	}
	got, err := st.recentMessages(ctx, "testroom", 2)
	if err != nil || len(got) != 2 {
		t.Fatalf("recentMessages: %v %v", got, err)
	}
	if got[0].ID != 9000002 || got[1].ID != 9000003 || got[0].Body != msgs[1].Body || got[0].Tier != 2 || got[1].Handle != "t_a" {
		t.Errorf("unexpected messages or order: %+v", got)
	}
	if d := got[1].TS - (base + 9); d < -2 || d > 2 {
		t.Errorf("timestamp drifted by %d ms", d)
	}
	max, err := st.maxMessageID(ctx)
	if err != nil || max < 9000003 {
		t.Errorf("maxMessageID = %d %v", max, err)
	}
}

func TestStorePrune(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	guest, _, _ := st.createGuest(ctx, "t_idle_guest", "t_idle_guest")
	member, _, _ := st.createGuest(ctx, "t_idle_member", "t_idle_member")
	fresh, _, _ := st.createGuest(ctx, "t_fresh_guest", "t_fresh_guest")
	st.setPassword(ctx, member.ID, "h")
	st.newSession(ctx, guest.ID)
	st.db.Exec(`UPDATE users SET last_seen = now() - interval '30 days' WHERE id IN ($1, $2)`, guest.ID, member.ID)
	st.insertMessages(ctx, "testroom", []storedMsg{
		{ID: 9100001, Handle: "t_a", Body: "old", TS: time.Now().Add(-10 * 24 * time.Hour).UnixMilli()},
		{ID: 9100002, Handle: "t_a", Body: "new", TS: time.Now().UnixMilli()},
	})

	m, g, _, err := st.prune(ctx, 7, 14)
	if err != nil {
		t.Fatal(err)
	}
	if m < 1 || g < 1 {
		t.Errorf("prune removed messages=%d guests=%d", m, g)
	}
	if _, err := st.userByKey(ctx, "t_idle_guest"); err != errNotFound {
		t.Error("idle guest should be gone")
	}
	if _, err := st.userByKey(ctx, "t_idle_member"); err != nil {
		t.Error("idle account with a password must stay")
	}
	if _, err := st.userByKey(ctx, "t_fresh_guest"); err != nil || fresh.ID == 0 {
		t.Error("fresh guest must stay")
	}
	left, _ := st.recentMessages(ctx, "testroom", 10)
	if len(left) != 1 || left[0].Body != "new" {
		t.Errorf("old message should be pruned, left: %+v", left)
	}
	var n int
	st.db.QueryRow(`SELECT count(*) FROM sessions WHERE user_id=$1`, guest.ID).Scan(&n)
	if n != 0 {
		t.Error("sessions of a deleted guest must go with it")
	}
}
