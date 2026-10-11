package main

import (
	"strings"
	"testing"
	"time"
)

func TestTierFor(t *testing.T) {
	cases := map[int64]int{0: 0, 1: 0, 299: 0, 300: 1, 1999: 1, 2000: 2, 9999: 2, 10000: 3, 39999: 3, 40000: 4, 1 << 40: 4}
	for pts, want := range cases {
		if got := tierFor(pts); got != want {
			t.Errorf("tierFor(%d) = %d, want %d", pts, got, want)
		}
	}
	if nextTierAt(0) != 300 || nextTierAt(3) != 40000 || nextTierAt(4) != 0 {
		t.Error("nextTierAt wrong")
	}
}

func newUser() *onlineUser { return &onlineUser{clients: map[*client]struct{}{}} }

func TestScoreBasics(t *testing.T) {
	u := newUser()
	t0 := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

	if g, b := u.score("hi", t0); g != 0 || b != 0 {
		t.Errorf("short message scored %d+%d", g, b)
	}
	g, b := u.score("hello there", t0.Add(time.Second))
	if g != 1 || b != 10 || u.points != 11 || u.streakDays != 1 {
		t.Errorf("first message: gain=%d bonus=%d points=%d streak=%d", g, b, u.points, u.streakDays)
	}
	if g, b := u.score("Hello There", t0.Add(2*time.Second)); g != 0 || b != 0 {
		t.Errorf("repeat within window scored %d+%d", g, b)
	}
	if g, _ := u.score("Hello There", t0.Add(40*time.Second)); g != 1 {
		t.Errorf("repeat after the window should score, got %d", g)
	}
	if g, b := u.score("another one", t0.Add(41*time.Second)); g != 1 || b != 0 {
		t.Errorf("same-day message gave %d+%d, no second bonus expected", g, b)
	}
	if !u.dirty {
		t.Error("user should be marked dirty")
	}
}

func TestScoreHourlyCap(t *testing.T) {
	u := newUser()
	t0 := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	earned := int64(0)
	for i := 0; i < 100; i++ {
		g, _ := u.score(strings.Repeat("x", 5)+string(rune('a'+i%26))+string(rune('a'+i/26)), t0.Add(time.Duration(i)*time.Second))
		earned += g
	}
	if earned != hourlyPointCap {
		t.Errorf("earned %d message points in one hour, cap is %d", earned, hourlyPointCap)
	}
	if g, _ := u.score("after the hour", t0.Add(61*time.Minute)); g != 1 {
		t.Errorf("cap should reset after an hour, got %d", g)
	}
}

func TestScoreStreak(t *testing.T) {
	u := newUser()
	day := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	want := []int64{10, 20, 30, 40, 50, 60, 70, 70}
	for i, w := range want {
		_, b := u.score("good morning all", day.AddDate(0, 0, i))
		if b != w {
			t.Errorf("day %d bonus = %d, want %d", i+1, b, w)
		}
	}
	if u.streakDays != 8 {
		t.Errorf("streak = %d, want 8", u.streakDays)
	}
	// A missed day resets the streak to 1.
	_, b := u.score("back again", day.AddDate(0, 0, 10))
	if u.streakDays != 1 || b != 10 {
		t.Errorf("after a gap: streak=%d bonus=%d", u.streakDays, b)
	}
}

func TestNormHandle(t *testing.T) {
	good := map[string]string{"ada": "ada", "@Ada_1": "Ada_1", "  bob  ": "bob"}
	for in, want := range good {
		h, k, ok := normHandle(in)
		if !ok || h != want || k != strings.ToLower(want) {
			t.Errorf("normHandle(%q) = %q %q %v", in, h, k, ok)
		}
	}
	for _, in := range []string{"", "ab", "has space", "x!y", "waytoolonghandle_here", "@@ada"} {
		if _, _, ok := normHandle(in); ok {
			t.Errorf("normHandle(%q) should fail", in)
		}
	}
}
