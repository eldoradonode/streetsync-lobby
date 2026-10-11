package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	minPassBytes     = 8
	maxPassBytes     = 128
	maxLoginFails    = 8                // failed logins per handle inside the window
	loginFailWindow  = 15 * time.Minute // window for the count above
	maxParallelHash  = 2                // password hashes running at once, so chat never starves
	hashWaitLimit    = 5 * time.Second  // how long a login may wait for a free hashing slot
	defaultHashIters = 600000           // PBKDF2-HMAC-SHA256 rounds, the current OWASP guidance
)

var errBusy = errors.New("busy")

// pbkdf2SHA256 is PBKDF2 (RFC 8018) with HMAC-SHA256 and one 32 byte output block.
func pbkdf2SHA256(password, salt []byte, iter int) []byte {
	prf := hmac.New(sha256.New, password)
	prf.Write(salt)
	prf.Write([]byte{0, 0, 0, 1})
	u := prf.Sum(nil)
	t := make([]byte, len(u))
	copy(t, u)
	for i := 1; i < iter; i++ {
		prf.Reset()
		prf.Write(u)
		u = prf.Sum(u[:0])
		for j := range t {
			t[j] ^= u[j]
		}
	}
	return t
}

func hashPassword(pw string, iter int) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	dk := pbkdf2SHA256([]byte(pw), salt, iter)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("pbkdf2-sha256$%d$%s$%s", iter, enc.EncodeToString(salt), enc.EncodeToString(dk)), nil
}

func checkPassword(stored, pw string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" {
		return false
	}
	iter, err := strconv.Atoi(parts[1])
	if err != nil || iter < 1 || iter > 5000000 {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(parts[2])
	want, err2 := enc.DecodeString(parts[3])
	if err1 != nil || err2 != nil {
		return false
	}
	got := pbkdf2SHA256([]byte(pw), salt, iter)
	return subtle.ConstantTimeCompare(got, want) == 1
}

// newToken makes a session token. Only its hash is stored in the database.
func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// authSvc runs the slow password work with a limit on how much runs at once.
type authSvc struct {
	iter  int
	sem   chan struct{}
	dummy string // checked when a handle does not exist, so timing does not reveal it
}

func newAuthSvc(iter int) *authSvc {
	if iter < 1 {
		iter = defaultHashIters
	}
	dummy, _ := hashPassword("not-a-real-password", iter)
	return &authSvc{iter: iter, sem: make(chan struct{}, maxParallelHash), dummy: dummy}
}

func (a *authSvc) acquire(ctx context.Context) error {
	t := time.NewTimer(hashWaitLimit)
	defer t.Stop()
	select {
	case a.sem <- struct{}{}:
		return nil
	case <-t.C:
		return errBusy
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *authSvc) release() { <-a.sem }

func (a *authSvc) hash(ctx context.Context, pw string) (string, error) {
	if err := a.acquire(ctx); err != nil {
		return "", err
	}
	defer a.release()
	return hashPassword(pw, a.iter)
}

func (a *authSvc) verify(ctx context.Context, stored, pw string) (bool, error) {
	if err := a.acquire(ctx); err != nil {
		return false, err
	}
	defer a.release()
	return checkPassword(stored, pw), nil
}

// loginLimiter counts failed logins per handle.
type loginLimiter struct {
	mu    sync.Mutex
	fails map[string][]time.Time
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{fails: make(map[string][]time.Time)} }

func (l *loginLimiter) recent(key string, now time.Time) []time.Time {
	list := l.fails[key]
	cut := now.Add(-loginFailWindow)
	i := 0
	for i < len(list) && list[i].Before(cut) {
		i++
	}
	list = list[i:]
	if len(list) == 0 {
		delete(l.fails, key)
	} else {
		l.fails[key] = list
	}
	return list
}

func (l *loginLimiter) allowed(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(key, now)) < maxLoginFails
}

func (l *loginLimiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fails[key] = append(l.recent(key, now), now)
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, key)
}
