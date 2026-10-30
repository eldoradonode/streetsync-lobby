package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/lib/pq"
)

// These tests need a real Postgres. Set TEST_DATABASE_URL to run them, for example:
//
//	TEST_DATABASE_URL="postgres://user:pass@localhost/db?sslmode=disable" go test ./...
//
// They create and delete rows with handles that start with "t_".
// testTombs are ids of test accounts that were deleted (renamed to deleted_<id>), so cleanup can still find them.
var testTombs []int64

func testStore(t *testing.T) *store {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	st, err := openStore(context.Background(), url, 30)
	if err != nil {
		t.Fatal(err)
	}
	clean := func() {
		// The ledger is never removed by deleting a user (the foreign key refuses), so tests clear their own rows first.
		_, _ = st.db.Exec(`DELETE FROM rooms WHERE kind <> 'official' AND (owner_id IN (SELECT id FROM users WHERE handle_key LIKE 't\_%') OR owner_id IS NULL OR name LIKE 'T %')`)
		_, _ = st.db.Exec(`DELETE FROM credit_ledger WHERE user_id IN (SELECT id FROM users WHERE handle_key LIKE 't\_%')`)
		_, _ = st.db.Exec(`DELETE FROM credit_ledger WHERE user_id = ANY($1)`, pq.Array(testTombs))
		_, _ = st.db.Exec(`DELETE FROM users WHERE id = ANY($1)`, pq.Array(testTombs))
		testTombs = nil
		_, _ = st.db.Exec(`DELETE FROM staff_complaints WHERE reporter_handle LIKE 't\_%' OR target_handle LIKE 't\_%'`)
		_, _ = st.db.Exec(`DELETE FROM users WHERE handle_key LIKE 't\_%'`)
		_, _ = st.db.Exec(`DELETE FROM messages WHERE room = 'testroom'`)
		_, _ = st.db.Exec(`DELETE FROM room_pins`) // step 19: pins, reactions and polls of official rooms
		_, _ = st.db.Exec(`DELETE FROM message_reactions`)
		_, _ = st.db.Exec(`DELETE FROM polls`)
		_, _ = st.db.Exec(`DELETE FROM reports WHERE reporter_handle LIKE 't\_%'`)
		_, _ = st.db.Exec(`DELETE FROM mod_log WHERE actor LIKE 't\_%'`)
		_, _ = st.db.Exec(`DELETE FROM messages WHERE room LIKE 't\_%'`)
		_, _ = st.db.Exec(`DELETE FROM rooms WHERE slug LIKE 't\_%'`)
	}
	clean()
	t.Cleanup(func() { clean(); st.close() })
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
		{ID: 9000001, Room: "testroom", UserID: 1, Handle: "t_a", Tier: 0, Body: "first, with \"quotes\" and a comma", TS: base},
		{ID: 9000002, Room: "testroom", UserID: 2, Handle: "t_b", Role: "admin", Tier: 2, Body: "second {braces} and \\ backslash and emoji \U0001F600", TS: base + 5},
		{ID: 9000003, Room: "testroom", UserID: 1, Handle: "t_a", Tier: 1, Body: "third", TS: base + 9,
			ReplyTo: 9000002, ReplyHandle: "t_b", ReplySnip: "second {braces} and \\ backslash"},
	}
	if err := st.insertMessages(ctx, msgs); err != nil {
		t.Fatal(err)
	}
	if err := st.insertMessages(ctx, msgs[:1]); err != nil {
		t.Fatalf("duplicate insert should be ignored: %v", err)
	}
	got, err := st.recentMessages(ctx, "testroom", 2)
	if err != nil || len(got) != 2 {
		t.Fatalf("recentMessages: %v %v", got, err)
	}
	if got[0].ID != 9000002 || got[1].ID != 9000003 || got[0].Body != msgs[1].Body || got[0].Tier != 2 || got[1].Handle != "t_a" {
		t.Errorf("unexpected messages or order: %+v", got)
	}
	if got[1].ReplyTo != 9000002 || got[1].ReplyHandle != "t_b" || got[1].ReplySnip != "second {braces} and \\ backslash" {
		t.Errorf("reply fields did not round trip: %+v", got[1])
	}
	if got[0].Role != "admin" || got[1].Role != "" {
		t.Errorf("role should be kept for staff and empty for members: %q %q", got[0].Role, got[1].Role)
	}
	if got[0].ReplyTo != 0 || got[0].ReplyHandle != "" || got[0].ReplySnip != "" {
		t.Errorf("a normal message should have no reply fields: %+v", got[0])
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
	st.insertMessages(ctx, []storedMsg{
		{ID: 9100001, Room: "testroom", Handle: "t_a", Body: "old", TS: time.Now().Add(-10 * 24 * time.Hour).UnixMilli()},
		{ID: 9100002, Room: "testroom", Handle: "t_a", Body: "new", TS: time.Now().UnixMilli()},
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

func TestStoreModeration(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	bob, _, _ := st.createGuest(ctx, "t_bob", "t_bob")
	if bob.Role != "user" {
		t.Fatalf("new users start as user, got %q", bob.Role)
	}

	// mute and kick
	mute, kick, err := st.activeSanctions(ctx, bob.ID)
	if err != nil || !mute.IsZero() || !kick.IsZero() {
		t.Fatalf("no sanctions expected: %v %v %v", mute, kick, err)
	}
	until, err := st.addSanction(ctx, bob.ID, "mute", "t_boss", "spam", 30)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(until); d < 29*time.Minute || d > 31*time.Minute {
		t.Errorf("mute should last about 30 minutes, got %v", d)
	}
	if _, err := st.addSanction(ctx, bob.ID, "kick", "t_boss", "", 5); err != nil {
		t.Fatal(err)
	}
	mute, kick, _ = st.activeSanctions(ctx, bob.ID)
	if mute.IsZero() || kick.IsZero() || !mute.After(kick) {
		t.Errorf("both should be active, mute later than kick: %v %v", mute, kick)
	}
	if _, err := st.addSanction(ctx, bob.ID, "ban", "t_boss", "", 5); err == nil {
		t.Error("unknown sanction kinds must be refused by the database")
	}
	if had, err := st.clearMute(ctx, bob.ID); err != nil || !had {
		t.Errorf("clearMute: %v %v", had, err)
	}
	mute, kick, _ = st.activeSanctions(ctx, bob.ID)
	if !mute.IsZero() || kick.IsZero() {
		t.Errorf("mute should be gone and kick kept: %v %v", mute, kick)
	}
	if had, _ := st.clearMute(ctx, bob.ID); had {
		t.Error("clearing twice should report nothing to clear")
	}
	// a finished sanction no longer counts
	st.db.Exec(`UPDATE sanctions SET until = now() - interval '1 minute' WHERE user_id = $1`, bob.ID)
	if m, k, _ := st.activeSanctions(ctx, bob.ID); !m.IsZero() || !k.IsZero() {
		t.Error("expired sanctions must not count")
	}

	// roles
	if err := st.setRole(ctx, bob.ID, "mod"); err != nil {
		t.Fatal(err)
	}
	if got, _ := st.userByKey(ctx, "t_bob"); got.Role != "mod" {
		t.Errorf("role not saved: %q", got.Role)
	}

	// reports
	rep := reportRow{ReporterID: 41, ReporterHandle: "t_ann", MsgID: 777, TargetHandle: "t_bob", Body: "bad words", Reason: "abuse"}
	id, dup, err := st.addReport(ctx, rep)
	if err != nil || dup || id == 0 {
		t.Fatalf("addReport: %d %v %v", id, dup, err)
	}
	if _, dup, _ := st.addReport(ctx, rep); !dup {
		t.Error("same reporter and message twice must be a duplicate")
	}
	rep2 := rep
	rep2.ReporterID, rep2.ReporterHandle = 42, "t_cleo"
	if _, dup, _ := st.addReport(ctx, rep2); dup {
		t.Error("another reporter of the same message is allowed")
	}
	list, err := st.openReports(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	mine := 0
	for _, r := range list {
		if r.TargetHandle == "t_bob" && r.Body == "bad words" && r.MsgID == 777 && r.TS > 0 {
			mine++
		}
	}
	if mine != 2 {
		t.Errorf("expected my 2 reports in the open list, got %d of %d", mine, len(list))
	}
	before, _ := st.openReportCount(ctx)
	if ok, err := st.closeReport(ctx, id, "dismissed", "t_boss"); err != nil || !ok {
		t.Fatalf("closeReport: %v %v", ok, err)
	}
	if ok, _ := st.closeReport(ctx, id, "dismissed", "t_boss"); ok {
		t.Error("a closed report cannot be closed again")
	}
	if after, _ := st.openReportCount(ctx); after != before-1 {
		t.Errorf("open count %d -> %d", before, after)
	}

	// log and delete
	st.logMod(ctx, "t_boss", "mute", "t_bob", "30 minutes")
	var n int
	st.db.QueryRow(`SELECT count(*) FROM mod_log WHERE actor = 't_boss' AND action = 'mute' AND target = 't_bob'`).Scan(&n)
	if n != 1 {
		t.Errorf("moderation log entry missing: %d", n)
	}
	st.insertMessages(ctx, []storedMsg{{ID: 9200001, Room: "testroom", Handle: "t_bob", Body: "remove me", TS: time.Now().UnixMilli()}})
	if err := st.deleteMessage(ctx, 9200001); err != nil {
		t.Fatal(err)
	}
	if left, _ := st.recentMessages(ctx, "testroom", 5); len(left) != 0 {
		t.Errorf("message should be deleted: %+v", left)
	}
}

func TestStoreEnsureAdmin(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	hash := func() (string, error) { return "pbkdf2-sha256$1$AA$AA", nil }
	fail := func() (string, error) { return "", errors.New("no password configured") }

	// missing account is created
	created, adopted, err := st.ensureAdmin(ctx, "T_Boss", "t_boss", hash)
	if err != nil || !created || adopted {
		t.Fatalf("create: %v %v %v", created, adopted, err)
	}
	u, _ := st.userByKey(ctx, "t_boss")
	if u.Role != "admin" || !u.PassHash.Valid || u.Handle != "T_Boss" {
		t.Errorf("admin not created properly: %+v", u)
	}
	// existing account with a password keeps it, and the hash function is not even needed
	st.db.Exec(`UPDATE users SET pass_hash = 'keepme' WHERE id = $1`, u.ID)
	created, adopted, err = st.ensureAdmin(ctx, "T_Boss", "t_boss", fail)
	if err != nil || created || adopted {
		t.Fatalf("existing: %v %v %v", created, adopted, err)
	}
	if u, _ = st.userByKey(ctx, "t_boss"); u.PassHash.String != "keepme" || u.Role != "admin" {
		t.Errorf("existing password must stay: %+v", u)
	}

	// a guest account is adopted and loses its sessions
	g, _, _ := st.createGuest(ctx, "t_guestboss", "t_guestboss")
	tok, _ := st.newSession(ctx, g.ID)
	created, adopted, err = st.ensureAdmin(ctx, "t_guestboss", "t_guestboss", hash)
	if err != nil || created || !adopted {
		t.Fatalf("adopt: %v %v %v", created, adopted, err)
	}
	if _, err := st.userBySession(ctx, tok); err != errNotFound {
		t.Error("old sessions of an adopted account must be ended")
	}
	if u, _ = st.userByKey(ctx, "t_guestboss"); u.Role != "admin" || !u.PassHash.Valid {
		t.Errorf("adopted account: %+v", u)
	}

	// no password configured and one is needed
	if _, _, err := st.ensureAdmin(ctx, "t_other", "t_other", fail); err == nil {
		t.Error("creating an admin without a password must fail")
	}
	if _, err := st.userByKey(ctx, "t_other"); err != errNotFound {
		t.Error("a failed creation must leave nothing behind")
	}
}

func TestStoreRooms(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()

	if err := st.seedRooms(ctx, defaultRooms); err != nil {
		t.Fatal(err)
	}
	extra := []roomRow{
		{Slug: "t_b", Name: "Second", Category: "Test", Tags: "x,y", MinTier: 2, Sort: 2},
		{Slug: "t_a", Name: "First", Category: "Test", Tags: "z", MinTier: 0, Sort: 1},
	}
	if err := st.seedRooms(ctx, extra); err != nil {
		t.Fatal(err)
	}
	list, err := st.listRooms(ctx)
	if err != nil {
		t.Fatal(err)
	}
	slugs := map[string]roomRow{}
	pos := map[string]int{}
	for i, r := range list {
		slugs[r.Slug] = r
		pos[r.Slug] = i
	}
	for _, want := range defaultRooms {
		if got, ok := slugs[want.Slug]; !ok || got.Name == "" {
			t.Errorf("default room %q missing", want.Slug)
		}
	}
	if slugs["elite-lounge"].MinTier != 3 {
		t.Errorf("elite lounge should need tier 3, got %d", slugs["elite-lounge"].MinTier)
	}
	if slugs["t_b"].MinTier != 2 || slugs["t_b"].Tags != "x,y" || pos["t_a"] > pos["t_b"] {
		t.Errorf("room fields or order wrong: %+v pos a=%d b=%d", slugs["t_b"], pos["t_a"], pos["t_b"])
	}

	// An edit made in the database must survive the next startup.
	if _, err := st.db.Exec(`UPDATE rooms SET name = 'Renamed' WHERE slug = 't_a'`); err != nil {
		t.Fatal(err)
	}
	if err := st.seedRooms(ctx, extra); err != nil {
		t.Fatal(err)
	}
	list, _ = st.listRooms(ctx)
	for _, r := range list {
		if r.Slug == "t_a" && r.Name != "Renamed" {
			t.Errorf("seeding must not overwrite edits, got %q", r.Name)
		}
	}
	var n int
	st.db.QueryRow(`SELECT count(*) FROM rooms WHERE slug = 't_a'`).Scan(&n)
	if n != 1 {
		t.Errorf("seeding twice must not duplicate: %d", n)
	}

	// Messages stay in their own room.
	base := time.Now().UnixMilli()
	if err := st.insertMessages(ctx, []storedMsg{
		{ID: 9300001, Room: "t_a", Handle: "t_x", Body: "in a", TS: base},
		{ID: 9300002, Room: "t_b", Handle: "t_x", Body: "in b", TS: base + 1},
		{ID: 9300003, Room: "t_a", Handle: "t_x", Body: "in a again", TS: base + 2},
	}); err != nil {
		t.Fatal(err)
	}
	a, _ := st.recentMessages(ctx, "t_a", 10)
	b, _ := st.recentMessages(ctx, "t_b", 10)
	if len(a) != 2 || a[0].Body != "in a" || a[1].Body != "in a again" || len(b) != 1 || b[0].Body != "in b" {
		t.Errorf("messages leaked between rooms: a=%+v b=%+v", a, b)
	}
}

func TestStoreDeleteSession(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u, _, _ := st.createGuest(ctx, "t_leaver", "t_leaver")
	one, _ := st.newSession(ctx, u.ID)
	two, _ := st.newSession(ctx, u.ID)
	if err := st.deleteSession(ctx, tokenHash(one)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.userBySession(ctx, one); err != errNotFound {
		t.Error("the ended login must stop working")
	}
	if got, err := st.userBySession(ctx, two); err != nil || got.ID != u.ID {
		t.Errorf("the other login must keep working: %v %v", got, err)
	}
	if err := st.deleteSession(ctx, tokenHash("never existed")); err != nil {
		t.Errorf("ending an unknown login is not an error: %v", err)
	}
}
