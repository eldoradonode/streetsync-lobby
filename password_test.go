package main

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

// Password change over the real WebSocket handler, against a real database.

func TestPasswordChangeFlow(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)

	e := newTestEnv(t)
	e.account("t_pw_ann", "user")
	e.account("t_pw_bob", "user")
	g := e.guest("t_pw_guest")
	annHash := func() string {
		var h string
		if err := e.st.db.QueryRow(`SELECT pass_hash FROM users WHERE handle_key = 't_pw_ann'`).Scan(&h); err != nil {
			t.Fatal(err)
		}
		return h
	}
	before := annHash()

	ann := e.login("t_pw_ann")
	annTab := e.dialRaw() // a second tab of the same login (same saved token)
	annTab.send(map[string]any{"t": "hi", "k": ann.Token})
	annTab.expect("ok")
	phone := e.login("t_pw_ann") // another device: its own login

	if got := ann.ask(map[string]any{"t": "pwchange", "o": "not the password", "p": "brand new pw 1"}, "pwchanged"); got != "err:bad_login" {
		t.Errorf("wrong current password: %s", got)
	}
	if got := ann.ask(map[string]any{"t": "pwchange", "o": "", "p": "brand new pw 1"}, "pwchanged"); got != "err:bad_login" {
		t.Errorf("empty current password: %s", got)
	}
	if got := ann.ask(map[string]any{"t": "pwchange", "o": "password-t_pw_ann", "p": "short"}, "pwchanged"); got != "err:weak_password" {
		t.Errorf("weak new password: %s", got)
	}
	if got := ann.ask(map[string]any{"t": "pwchange", "o": "password-t_pw_ann", "p": strings.Repeat("x", maxPassBytes+1)}, "pwchanged"); got != "err:weak_password" {
		t.Errorf("too long new password: %s", got)
	}
	if got := ann.ask(map[string]any{"t": "pwchange", "o": "password-t_pw_ann", "p": "password-t_pw_ann"}, "pwchanged"); got != "err:same_password" {
		t.Errorf("same password: %s", got)
	}
	// Someone else's password is not the current password of this account.
	if got := ann.ask(map[string]any{"t": "pwchange", "o": "password-t_pw_bob", "p": "brand new pw 1"}, "pwchanged"); got != "err:bad_login" {
		t.Errorf("another account's password: %s", got)
	}
	if annHash() != before {
		t.Fatal("a refused change altered the stored password")
	}

	if got := ann.ask(map[string]any{"t": "pwchange", "o": "password-t_pw_ann", "p": "brand new pw 1"}, "pwchanged"); got != "ok:pwchanged" {
		t.Fatalf("a correct change: %s", got)
	}
	after := annHash()
	if after == before || !strings.HasPrefix(after, "pbkdf2-sha256$") || strings.Contains(after, "brand new pw 1") {
		t.Errorf("the new password must be stored hashed: %q", after)
	}
	if !checkPassword(after, "brand new pw 1") || checkPassword(after, "password-t_pw_ann") {
		t.Error("stored hash must match only the new password")
	}

	// New password signs in, old does not.
	w := e.dialRaw()
	w.send(map[string]any{"t": "login", "h": "t_pw_ann", "p": "brand new pw 1"})
	w.expect("ok")
	w2 := e.dialRaw()
	w2.send(map[string]any{"t": "login", "h": "t_pw_ann", "p": "password-t_pw_ann"})
	if f := w2.expect("err"); f["c"] != "bad_login" {
		t.Errorf("old password after change: %v", f)
	}

	// The login that made the change, and its other tab, keep working.
	if got := ann.ask(map[string]any{"t": "inbox"}, "inbox"); got != "ok:inbox" {
		t.Errorf("the changing login must keep working: %s", got)
	}
	if got := annTab.ask(map[string]any{"t": "inbox"}, "inbox"); got != "ok:inbox" {
		t.Errorf("another tab of the same login must keep working: %s", got)
	}
	// Other logins are ended: live connection closed, saved token dead.
	phone.expect("err")
	if !phone.isClosed() {
		t.Error("the other device's live connection must be closed after a password change")
	}
	rs := e.dialRaw()
	rs.send(map[string]any{"t": "hi", "k": phone.Token})
	if f := rs.expect("err"); f["c"] != "session_expired" {
		t.Errorf("the other device's token after change: %v", f)
	}

	// A guest cannot use it.
	if got := g.ask(map[string]any{"t": "pwchange", "o": "x", "p": "brand new pw 1"}, "pwchanged"); got != "err:needs_password" {
		t.Errorf("guest: %s", got)
	}

	// Guessing the current password is rate limited like logging in.
	bob := e.login("t_pw_bob")
	var last string
	for i := 0; i < maxLoginFails+2; i++ {
		last = bob.ask(map[string]any{"t": "pwchange", "o": "guess", "p": "brand new pw 2"}, "pwchanged")
	}
	if last != "err:too_many_attempts" {
		t.Errorf("rate limit: %s", last)
	}

	for _, secret := range []string{"brand new pw 1", "brand new pw 2", "password-t_pw_ann", "password-t_pw_bob", "not the password"} {
		if strings.Contains(logs.String(), secret) {
			t.Errorf("a password appeared in the server log: %q", secret)
		}
	}
}
