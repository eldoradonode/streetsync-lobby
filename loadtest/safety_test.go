package main

import "testing"

func TestCheckTarget(t *testing.T) {
	t.Setenv("LOADTEST_ENV", "")
	t.Setenv("LOADTEST_ALLOWED_HOSTS", "")
	t.Setenv("LOADTEST_FORBIDDEN_HOSTS", "")
	for _, ok := range []string{"ws://127.0.0.1:8080/ws", "ws://localhost/ws", "ws://10.0.0.5/ws", "ws://192.168.1.2:9/ws", "ws://box.local/ws"} {
		if err := checkTarget(ok, "", "target"); err != nil {
			t.Errorf("%s should be allowed: %v", ok, err)
		}
	}
	for _, bad := range []string{"wss://streetsync.example.com/ws", "wss://8.8.8.8/ws", "postgres://u:p@db.example.com/x", "", "not a url"} {
		if err := checkTarget(bad, "", "target"); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}
	// A public staging host needs the allow list AND a matching confirmation.
	t.Setenv("LOADTEST_ALLOWED_HOSTS", "staging.example.com")
	if err := checkTarget("wss://staging.example.com/ws", "", "target"); err == nil {
		t.Error("allowed but unconfirmed must be refused")
	}
	if err := checkTarget("wss://staging.example.com/ws", "other.example.com", "target"); err == nil {
		t.Error("wrong confirmation must be refused")
	}
	if err := checkTarget("wss://staging.example.com/ws", "staging.example.com", "target"); err != nil {
		t.Errorf("confirmed staging host: %v", err)
	}
	t.Setenv("LOADTEST_FORBIDDEN_HOSTS", "staging.example.com")
	if err := checkTarget("wss://staging.example.com/ws", "staging.example.com", "target"); err == nil {
		t.Error("a forbidden host must always be refused")
	}
	t.Setenv("LOADTEST_FORBIDDEN_HOSTS", "")
	t.Setenv("LOADTEST_ENV", "production")
	if err := checkTarget("ws://127.0.0.1/ws", "", "target"); err == nil {
		t.Error("LOADTEST_ENV=production must refuse everything")
	}
}
