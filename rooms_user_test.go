package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRoomNamesTagsSlugs(t *testing.T) {
	good := map[string]string{"Abuja Night Owls": "Abuja Night Owls", "  spaced   out  ": "spaced out", "Café Gist": "Café Gist", "R&B, soul!": "R&B, soul!"}
	for in, want := range good {
		if got, ok := cleanRoomName(in); !ok || got != want {
			t.Errorf("cleanRoomName(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "ab", strings.Repeat("x", 31), "<script>", "!!!", "---", "emoji \U0001F600 room", "tab\there<b>"} {
		if got, ok := cleanRoomName(bad); ok {
			t.Errorf("cleanRoomName(%q) = %q, want refused", bad, got)
		}
	}
	if got := roomTags(" Football, banter ,abuja,extra,more"); got != "Football,banter,abuja" {
		t.Errorf("roomTags = %q", got)
	}
	if got := roomTags(strings.Repeat("x", 40)); len([]rune(got)) != roomTagMax {
		t.Errorf("long tag kept %d characters", len([]rune(got)))
	}
	if got := roomTags(" , ,"); got != "" {
		t.Errorf("empty tags = %q", got)
	}
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		s := roomSlug("Abuja Night Owls!!")
		if !strings.HasPrefix(s, "abuja-night-owls-") || len(s) != len("abuja-night-owls-")+4 {
			t.Fatalf("slug %q", s)
		}
		seen[s] = true
	}
	if len(seen) < 45 {
		t.Errorf("slugs repeat too much: %d different of 50", len(seen))
	}
	if s := roomSlug("日本語 room"); !strings.HasPrefix(s, "room-") {
		t.Errorf("non-latin name slug %q", s)
	}
	if s := roomSlug("!!!"); !strings.HasPrefix(s, "room-") {
		t.Errorf("symbols only slug %q", s)
	}
	if roomLimitFor(0) != 1 || roomLimitFor(3) != 1 || roomLimitFor(4) != 2 {
		t.Error("room limits: one room, two for Legends")
	}
	c := randCode(8, inviteAlphabet)
	if len(c) != 8 || strings.ContainsAny(c, "01OIl") {
		t.Errorf("invite code %q", c)
	}
}

func TestCreateRoomChargesOnce(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_rma", "t_rmb")
	a, b := u[0], u[1]
	st.applyCredit(ctx, a.ID, "grant", 60, "g", "")
	// The official room names must exist, or the test passes only on a database that earlier tests already seeded.
	if err := st.seedRooms(ctx, defaultRooms); err != nil {
		t.Fatal(err)
	}
	mk := func(slug, name string, owner int64) roomRow {
		return roomRow{Slug: slug, Name: name, Category: "Community", Kind: "public", OwnerID: owner, Invite: randCode(8, inviteAlphabet), Sort: 1000}
	}
	bal, err := st.createRoom(ctx, mk("t_one-aaaa", "T One", a.ID), 25, 1)
	if err != nil || bal != 35 {
		t.Fatalf("first room: bal=%d err=%v", bal, err)
	}
	// One room per person: refused, not charged, nothing stored.
	if _, err := st.createRoom(ctx, mk("t_two-aaaa", "T Two", a.ID), 25, 1); !errors.Is(err, errRoomLimit) {
		t.Fatalf("second room with a limit of 1: %v", err)
	}
	if credits(t, st, a.ID) != 35 {
		t.Errorf("a refused room must not charge: %d", credits(t, st, a.ID))
	}
	// A limit of 2 allows it.
	if _, err := st.createRoom(ctx, mk("t_two-aaaa", "T Two", a.ID), 25, 2); err != nil {
		t.Fatalf("second room with a limit of 2: %v", err)
	}
	// Not enough credits: refused and nothing created.
	if _, err := st.createRoom(ctx, mk("t_bro-aaaa", "T Broke", b.ID), 25, 1); !errors.Is(err, errNoFunds) {
		t.Fatalf("no credits: %v", err)
	}
	rooms, _ := st.listRooms(ctx)
	for _, r := range rooms {
		if r.Slug == "t_bro-aaaa" {
			t.Error("a room that could not be paid for exists")
		}
	}
	// Names are unique whatever the capital letters.
	st.applyCredit(ctx, b.ID, "grant", 30, "g", "")
	if _, err := st.createRoom(ctx, mk("t_dup-aaaa", "t one", b.ID), 25, 1); !errors.Is(err, errNameTaken) {
		t.Fatalf("duplicate name: %v", err)
	}
	if credits(t, st, b.ID) != 30 {
		t.Errorf("a refused name must not charge: %d", credits(t, st, b.ID))
	}
	// Official room names are taken as well.
	if _, err := st.createRoom(ctx, mk("t_off-aaaa", "general chat", b.ID), 25, 1); !errors.Is(err, errNameTaken) {
		t.Fatalf("official name: %v", err)
	}
	// Read back with the owner's handle.
	rooms, _ = st.listRooms(ctx)
	found := false
	for _, r := range rooms {
		if r.Slug == "t_one-aaaa" {
			found = true
			if r.Kind != "public" || r.OwnerID != a.ID || r.OwnerHandle != a.Handle || r.Invite == "" || r.PassHash != "" || r.Fee != 0 {
				t.Errorf("stored room: %+v", r)
			}
		}
	}
	if !found {
		t.Error("created room is not listed")
	}
	// Closing a room removes it and its messages, and never an official room.
	st.db.Exec(`INSERT INTO messages (id, room, handle, body, created_at) VALUES (777001, 't_one-aaaa', 'x', 'hi', now())`)
	if err := st.deleteRoom(ctx, "t_one-aaaa"); err != nil {
		t.Fatal(err)
	}
	var n int
	st.db.QueryRow(`SELECT count(*) FROM messages WHERE room='t_one-aaaa'`).Scan(&n)
	if n != 0 {
		t.Error("messages of a closed room remain")
	}
	st.db.Exec(`INSERT INTO messages (id, room, handle, body, created_at) VALUES (777002, 'lobby', 'x', 'keep', now())`)
	st.deleteRoom(ctx, "lobby")
	st.db.QueryRow(`SELECT count(*) FROM rooms WHERE slug='lobby'`).Scan(&n)
	if n != 1 {
		t.Error("deleteRoom must never remove an official room")
	}
	// The creation fee is in the ledger.
	var kinds string
	st.db.QueryRow(`SELECT string_agg(kind, ',' ORDER BY id) FROM credit_ledger WHERE user_id=$1`, a.ID).Scan(&kinds)
	if kinds != "grant,room_create,room_create" {
		t.Errorf("ledger kinds: %s", kinds)
	}
	st.db.Exec(`DELETE FROM messages WHERE id IN (777001, 777002)`)
}

func TestEnterRoomFeeAndHostShare(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_rea", "t_reb", "t_rec")
	host, guest, broke := u[0], u[1], u[2]
	st.applyCredit(ctx, host.ID, "grant", 30, "g", "")
	st.applyCredit(ctx, guest.ID, "grant", 30, "g", "")
	r := roomRow{Slug: "t_fee-aaaa", Name: "T Fee Room", Category: "Community", Kind: "private", OwnerID: host.ID, Fee: 10, Invite: randCode(8, inviteAlphabet), Sort: 1000}
	if _, err := st.createRoom(ctx, r, 25, 1); err != nil {
		t.Fatal(err)
	}

	// The visitor must name the price they saw.
	if _, _, _, _, err := st.enterRoom(ctx, "t_fee-aaaa", guest.ID, 5); !errors.Is(err, errFeeChanged) {
		t.Fatalf("a changed price: %v", err)
	}
	if member, _ := st.isRoomMember(ctx, "t_fee-aaaa", guest.ID); member {
		t.Error("a refused entry made them a member")
	}
	if credits(t, st, guest.ID) != 30 {
		t.Error("a refused entry must not charge")
	}
	bal, charged, hostID, got, err := st.enterRoom(ctx, "t_fee-aaaa", guest.ID, 10)
	if err != nil || !charged || bal != 20 || hostID != host.ID || got != 8 {
		t.Fatalf("paid entry: bal=%d charged=%v host=%d got=%d err=%v", bal, charged, hostID, got, err)
	}
	if credits(t, st, host.ID) != 5+8 {
		t.Errorf("host balance %d, want 13 (30 - 25 + 8)", credits(t, st, host.ID))
	}
	if member, _ := st.isRoomMember(ctx, "t_fee-aaaa", guest.ID); !member {
		t.Error("a paid entry must make them a member")
	}
	// Coming back is free, even if the price went up meanwhile.
	st.db.Exec(`UPDATE rooms SET entry_fee = 40 WHERE slug = 't_fee-aaaa'`)
	bal, charged, _, _, err = st.enterRoom(ctx, "t_fee-aaaa", guest.ID, 10)
	if err != nil || charged {
		t.Fatalf("second entry: charged=%v err=%v", charged, err)
	}
	if credits(t, st, guest.ID) != 20 || credits(t, st, host.ID) != 13 {
		t.Error("a member must not pay twice, and the host must not be paid twice")
	}
	// No credits, no entry, no membership.
	if _, _, _, _, err := st.enterRoom(ctx, "t_fee-aaaa", broke.ID, 40); !errors.Is(err, errNoFunds) {
		t.Fatalf("no credits: %v", err)
	}
	if member, _ := st.isRoomMember(ctx, "t_fee-aaaa", broke.ID); member {
		t.Error("a failed payment made them a member")
	}
	// Missing room.
	if _, _, _, _, err := st.enterRoom(ctx, "t_nope", guest.ID, 0); !errors.Is(err, errNotFound) {
		t.Errorf("missing room: %v", err)
	}
	// The books balance.
	for _, id := range []int64{host.ID, guest.ID, broke.ID} {
		var sum int64
		st.db.QueryRow(`SELECT coalesce(sum(amount),0) FROM credit_ledger WHERE user_id=$1`, id).Scan(&sum)
		if sum != credits(t, st, id) {
			t.Errorf("user %d: ledger %d, balance %d", id, sum, credits(t, st, id))
		}
	}
	// What the person owns or entered comes back as their rooms.
	mine, _ := st.memberRooms(ctx, guest.ID)
	owned, _ := st.memberRooms(ctx, host.ID)
	if len(mine) != 1 || mine[0] != "t_fee-aaaa" || len(owned) != 1 || owned[0] != "t_fee-aaaa" {
		t.Errorf("memberRooms: %v / %v", mine, owned)
	}
}

func TestRoomSettingsAndActivity(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_rsa")
	st.applyCredit(ctx, u[0].ID, "grant", 30, "g", "")
	r := roomRow{Slug: "t_set-aaaa", Name: "T Settings", Category: "Community", Kind: "public", OwnerID: u[0].ID, Invite: randCode(8, inviteAlphabet), Sort: 1000}
	if _, err := st.createRoom(ctx, r, 25, 1); err != nil {
		t.Fatal(err)
	}
	fee, hash, kind := 12, "somehash", "private"
	if err := st.updateRoomSettings(ctx, "t_set-aaaa", &fee, &hash, &kind); err != nil {
		t.Fatal(err)
	}
	rooms, _ := st.listRooms(ctx)
	for _, x := range rooms {
		if x.Slug == "t_set-aaaa" && (x.Fee != 12 || x.PassHash != "somehash" || x.Kind != "private") {
			t.Errorf("after update: %+v", x)
		}
	}
	empty := ""
	st.updateRoomSettings(ctx, "t_set-aaaa", nil, &empty, nil) // an empty hash removes the password, the rest stays
	rooms, _ = st.listRooms(ctx)
	for _, x := range rooms {
		if x.Slug == "t_set-aaaa" && (x.PassHash != "" || x.Fee != 12 || x.Kind != "private") {
			t.Errorf("after removing the password: %+v", x)
		}
	}
	// Settings can never change an official room.
	k := "private"
	st.updateRoomSettings(ctx, "lobby", nil, nil, &k)
	rooms, _ = st.listRooms(ctx)
	for _, x := range rooms {
		if x.Slug == "lobby" && x.Kind != "official" {
			t.Error("an official room was changed")
		}
	}
	// Activity is remembered.
	when := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	if err := st.touchRooms(ctx, map[string]time.Time{"t_set-aaaa": when}); err != nil {
		t.Fatal(err)
	}
	rooms, _ = st.listRooms(ctx)
	for _, x := range rooms {
		if x.Slug == "t_set-aaaa" && !x.LastActive.Equal(when) {
			t.Errorf("last active %v, want %v", x.LastActive, when)
		}
	}
}
