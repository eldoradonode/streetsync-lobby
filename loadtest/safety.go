package main

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
)

// The load test creates thousands of throwaway guest accounts and can delete them again.
// It must never be pointed at production, so every target is checked before anything is sent.
//
// Allowed without further ceremony: loopback, private network addresses, localhost and *.local / *.internal names.
// Anything else (a real staging server on a public name) must be listed in LOADTEST_ALLOWED_HOSTS and confirmed
// with -confirm-host <host>. Hosts in LOADTEST_FORBIDDEN_HOSTS are always refused. LOADTEST_ENV=production refuses everything.

func hostOf(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", fmt.Errorf("not a usable address: %q", raw)
	}
	h := u.Hostname()
	return strings.ToLower(h), nil
}

func isLocalHost(h string) bool {
	if h == "localhost" || strings.HasSuffix(h, ".local") || strings.HasSuffix(h, ".internal") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

func inList(env, h string) bool {
	for _, x := range strings.Split(os.Getenv(env), ",") {
		if x = strings.ToLower(strings.TrimSpace(x)); x != "" && x == h {
			return true
		}
	}
	return false
}

// checkTarget refuses anything that is not an explicitly isolated environment. what is "target" or "database".
func checkTarget(raw, confirm, what string) error {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("LOADTEST_ENV")), "production") {
		return errors.New("LOADTEST_ENV=production: refusing to run")
	}
	h, err := hostOf(raw)
	if err != nil {
		return err
	}
	if inList("LOADTEST_FORBIDDEN_HOSTS", h) {
		return fmt.Errorf("%s host %s is in LOADTEST_FORBIDDEN_HOSTS", what, h)
	}
	if isLocalHost(h) {
		return nil
	}
	if !inList("LOADTEST_ALLOWED_HOSTS", h) {
		return fmt.Errorf("%s host %s is not local. To use an authorized staging host, set LOADTEST_ALLOWED_HOSTS=%s and pass -confirm-host %s", what, h, h, h)
	}
	if strings.ToLower(confirm) != h {
		return fmt.Errorf("%s host %s is allowed but not confirmed: pass -confirm-host %s", what, h, h)
	}
	return nil
}
