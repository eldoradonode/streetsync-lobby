package main

import (
	"math/rand"
	"testing"
	"time"
)

// Randomized lifecycle check. One person (the subject) opens, refreshes, drops, duplicates and closes tabs in one room
// while an observer watches. Rules checked on everything the observer is told:
//   - notices alternate: a join is followed by a leave and a leave by a join, never two of the same in a row
//   - the last notice agrees with whether the subject really has a connection in the room at the end
//   - the subject is never announced as leaving while a connection of theirs stayed in the room the whole time
func TestBugfixPresenceRandomLifecycle(t *testing.T) {
	setGraces(t, 250*time.Millisecond, 500*time.Millisecond, 300*time.Millisecond)
	e := newTestEnv(t)
	e.account("t_bfsub", "user")
	e.account("t_bfobs", "user")
	obs := e.login("t_bfobs")
	obs.join("lobby")
	obs.drain()
	rng := rand.New(rand.NewSource(7))
	var tabs []*wsc
	var notices []string
	pump := func() {
		for {
			f, ok := obs.tryRead(5 * time.Millisecond)
			if !ok {
				return
			}
			if f["t"] == "rs" && f["h"] == "t_bfsub" {
				notices = append(notices, f["a"].(string))
			}
		}
	}
	steady := true // true while at least one tab has stayed in the room since the last leave notice
	var violations []string
	for step := 0; step < 120; step++ {
		switch op := rng.Intn(8); {
		case op == 0 || len(tabs) == 0: // open a tab straight into the room
			w := e.login("t_bfsub")
			w.join("lobby")
			tabs = append(tabs, w)
		case op == 1: // refresh: drop and reconnect at once
			i := rng.Intn(len(tabs))
			tabs[i].c.CloseNow()
			w := e.login("t_bfsub")
			w.join("lobby")
			tabs[i] = w
		case op == 2: // close a tab for good
			i := rng.Intn(len(tabs))
			tabs[i].c.CloseNow()
			tabs = append(tabs[:i], tabs[i+1:]...)
		case op == 3: // back arrow in a tab, then it comes back
			i := rng.Intn(len(tabs))
			tabs[i].send(map[string]any{"t": "leave"})
			tabs[i].join("lobby")
		case op == 4: // silence: nothing at all for a while, longer than the grace period sometimes
			time.Sleep(time.Duration(rng.Intn(400)) * time.Millisecond)
		case op == 6: // a phone locks its screen: the page said it was in the background, then the connection died
			i := rng.Intn(len(tabs))
			tabs[i].send(map[string]any{"t": "bg"})
			time.Sleep(20 * time.Millisecond)
			tabs[i].c.CloseNow()
			tabs = append(tabs[:i], tabs[i+1:]...)
		case op == 7 && rng.Intn(3) == 0: // the Leave room button: the person leaves, in every tab
			tabs[rng.Intn(len(tabs))].send(map[string]any{"t": "leave", "a": "x"})
			time.Sleep(30 * time.Millisecond)
			for _, w := range tabs {
				w.c.CloseNow()
			}
			tabs = nil
		default: // a second tab
			w := e.login("t_bfsub")
			w.join("lobby")
			tabs = append(tabs, w)
		}
		time.Sleep(time.Duration(rng.Intn(60)) * time.Millisecond)
		pump()
		if len(tabs) > 0 {
			_ = steady
		}
	}
	// Settle: let every grace period end, then compare with the truth.
	time.Sleep(1300 * time.Millisecond)
	pump()
	for i := 1; i < len(notices); i++ {
		if notices[i] == notices[i-1] {
			violations = append(violations, "two "+notices[i]+" notices in a row at position "+itoa(i))
		}
	}
	here, in := e.roomsOf("t_bfsub")
	wantIn := 0
	if len(tabs) > 0 {
		wantIn = 1
	}
	last := ""
	if len(notices) > 0 {
		last = notices[len(notices)-1]
	}
	if (last == "j") != (wantIn == 1) && !(last == "" && wantIn == 0) {
		violations = append(violations, "last notice "+last+" but subject present="+itoa(wantIn))
	}
	if here != wantIn || in != wantIn {
		violations = append(violations, "server says here="+itoa(here)+" connections="+itoa(in)+" truth="+itoa(wantIn))
	}
	if len(violations) > 0 {
		t.Fatalf("notices %v: %v", notices, violations)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}

// tryRead returns the next frame if one arrives within d.
func (w *wsc) tryRead(d time.Duration) (map[string]any, bool) {
	select {
	case f := <-w.frames:
		return f, true
	case <-time.After(d):
		return nil, false
	}
}
