package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCleanAvatar(t *testing.T) {
	for _, ok := range []string{"m0", "m7", "f3", "x5", "x0"} {
		if cleanAvatar(ok) != ok {
			t.Errorf("%q must be a valid look", ok)
		}
	}
	for _, bad := range []string{"", "m8", "f9", "z1", "m", "f33", "F3", "m-1", "<b>", "m3 ", " m3", "mm3", "f3\n"} {
		if cleanAvatar(bad) != "" {
			t.Errorf("%q must be refused", bad)
		}
	}
}

func TestAvatarsAreStoredAndFound(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	a, created, err := st.createGuestWithAvatar(ctx, "t_ava", "t_ava", "f3")
	if err != nil || !created || a.Avatar != "f3" {
		t.Fatalf("create with avatar: %+v %v %v", a, created, err)
	}
	b, _, _ := st.createGuest(ctx, "t_avb", "t_avb")
	if b.Avatar != "" {
		t.Errorf("a guest without a pick has no avatar: %q", b.Avatar)
	}
	if err := st.setAvatar(ctx, b.ID, "m1"); err != nil {
		t.Fatal(err)
	}
	m, err := st.avatarsFor(ctx, []string{"T_AVA", "t_avb", "t_nobody", "bad handle!"})
	if err != nil || len(m) != 2 || m["t_ava"] != "f3" || m["t_avb"] != "m1" {
		t.Fatalf("avatarsFor: %v %v", m, err)
	}
	if m, _ := st.avatarsFor(ctx, nil); len(m) != 0 {
		t.Errorf("no handles, no looks: %v", m)
	}
	// People who never picked are left out, so the page makes their look from the handle.
	st.setAvatar(ctx, b.ID, "")
	if m, _ := st.avatarsFor(ctx, []string{"t_avb"}); len(m) != 0 {
		t.Errorf("an empty look must be left out: %v", m)
	}
	// Messages keep the look of the sender.
	err = st.insertMessages(ctx, []storedMsg{
		{ID: 990001, Handle: "t_ava", Body: "hi", TS: time.Now().UnixMilli(), Room: "testroom", Avatar: "f3"},
		{ID: 990002, Handle: "t_avb", Body: "yo", TS: time.Now().UnixMilli(), Room: "testroom"},
	})
	if err != nil {
		t.Fatal(err)
	}
	msgs, _ := st.recentMessages(ctx, "testroom", 10)
	if len(msgs) != 2 || msgs[0].Av != "f3" || msgs[1].Av != "" {
		t.Errorf("message looks: %+v", msgs)
	}
}

func TestProfileOf(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u, _, _ := st.createGuestWithAvatar(ctx, "t_prf", "t_prf", "x2")
	st.db.Exec(`UPDATE users SET points = 450 WHERE id = $1`, u.ID)
	p, id, err := st.profileOf(ctx, "t_prf")
	if err != nil || id != u.ID || p.Handle != "t_prf" || p.Avatar != "x2" || p.Points != 450 || p.Role != "user" || time.Since(p.Joined) > time.Minute {
		t.Fatalf("profileOf: %+v %d %v", p, id, err)
	}
	if _, _, err := st.profileOf(ctx, "t_missing_person"); !errors.Is(err, errNotFound) {
		t.Errorf("missing person: %v", err)
	}
}
