package main

import (
	"strings"
	"testing"
	"time"
)

func TestSnippet(t *testing.T) {
	if got := snippet("short"); got != "short" {
		t.Errorf("short text changed: %q", got)
	}
	exact := strings.Repeat("a", snippetRunes)
	if snippet(exact) != exact {
		t.Error("text of exactly the limit must stay whole")
	}
	long := strings.Repeat("é", snippetRunes+20)
	got := snippet(long)
	if want := strings.Repeat("é", snippetRunes) + "…"; got != want {
		t.Errorf("long text not cut at %d characters: %q", snippetRunes, got)
	}
}

func TestCanAct(t *testing.T) {
	cases := []struct {
		actor, target string
		want          bool
	}{
		{"user", "user", false}, {"user", "mod", false}, {"user", "admin", false},
		{"mod", "user", true}, {"mod", "mod", false}, {"mod", "admin", false},
		{"admin", "user", true}, {"admin", "mod", true}, {"admin", "admin", false},
		{"", "user", false}, {"nonsense", "user", false},
	}
	for _, c := range cases {
		if got := canAct(c.actor, c.target); got != c.want {
			t.Errorf("canAct(%q, %q) = %v, want %v", c.actor, c.target, got, c.want)
		}
	}
}

func TestMinutesLeft(t *testing.T) {
	if minutesLeft(time.Now().Add(-time.Minute)) != 0 {
		t.Error("past times have no minutes left")
	}
	if got := minutesLeft(time.Now().Add(9*time.Minute + 30*time.Second)); got != 10 {
		t.Errorf("9.5 minutes should show as 10, got %d", got)
	}
	if got := minutesLeft(time.Now().Add(time.Second)); got != 1 {
		t.Errorf("one second left should show as 1 minute, got %d", got)
	}
}

func TestHeatLevel(t *testing.T) {
	cases := map[int]int{-3: 0, 0: 0, 1: 1, 4: 1, 5: 2, 14: 2, 15: 3, 39: 3, 40: 4, 99: 4, 100: 5, 5000: 5}
	for n, want := range cases {
		if got := heatLevel(n); got != want {
			t.Errorf("heatLevel(%d) = %d, want %d", n, got, want)
		}
	}
}

func TestRoomHeatForgetsOldMessages(t *testing.T) {
	rm := &room{}
	now := time.Now()
	for i := 0; i < 20; i++ {
		rm.times = append(rm.times, now.Add(-10*time.Minute)) // older than the window
	}
	for i := 0; i < 5; i++ {
		rm.times = append(rm.times, now.Add(-time.Minute))
	}
	if got := rm.heat(now); got != 2 {
		t.Errorf("only the 5 recent messages should count, heat = %d", got)
	}
	if len(rm.times) != 5 {
		t.Errorf("old entries should be dropped, %d left", len(rm.times))
	}
}
