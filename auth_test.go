package main

import (
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

// Reference values come from Python's hashlib.pbkdf2_hmac.
func TestPBKDF2Vectors(t *testing.T) {
	cases := []struct {
		pw, salt string
		iter     int
		want     string
	}{
		{"password", "salt", 1, "120fb6cffcf8b32c43e7225256c4f837a86548c92ccc35480805987cb70be17b"},
		{"password", "salt", 2, "ae4d0c95af6b46d32d0adff928f06dd02a303f8ef3c251dfd6e2d85a95474c43"},
		{"password", "salt", 4096, "c5e478d59288c841aa530db6845c4c8d962893a001ce4e11a4963873aa98134a"},
		{"passwd", "salt", 1, "55ac046e56e3089fec1691c22544b605f94185216dde0465e68b9d57c20dacbc"},
		{"correct horse battery staple", "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f", 1000,
			"a69b179e3add3c1e0aaf227a0eb3aa2aa8645ab86fecf6ca00c17512697c719e"},
	}
	for _, c := range cases {
		got := hex.EncodeToString(pbkdf2SHA256([]byte(c.pw), []byte(c.salt), c.iter))
		if got != c.want {
			t.Errorf("pbkdf2(%q, %d) = %s, want %s", c.pw, c.iter, got, c.want)
		}
	}
}

func TestHashAndCheck(t *testing.T) {
	h, err := hashPassword("s3cret-pass", 1000)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "pbkdf2-sha256$1000$") {
		t.Fatalf("unexpected format %q", h)
	}
	if !checkPassword(h, "s3cret-pass") {
		t.Error("correct password rejected")
	}
	if checkPassword(h, "s3cret-pasS") || checkPassword(h, "") {
		t.Error("wrong password accepted")
	}
	h2, _ := hashPassword("s3cret-pass", 1000)
	if h == h2 {
		t.Error("two hashes of the same password should differ (random salt)")
	}
	for _, bad := range []string{"", "x", "pbkdf2-sha256$abc$AA$AA", "md5$1$AA$AA", "pbkdf2-sha256$1000$!!$!!", "pbkdf2-sha256$99999999$AA$AA"} {
		if checkPassword(bad, "x") {
			t.Errorf("malformed hash %q accepted", bad)
		}
	}
}

func TestTokenHash(t *testing.T) {
	a, b := newToken(), newToken()
	if len(a) != 64 || a == b {
		t.Fatalf("bad tokens %q %q", a, b)
	}
	if tokenHash(a) == a || len(tokenHash(a)) != 64 || tokenHash(a) != tokenHash(a) {
		t.Error("token hash should be stable, 64 chars and different from the token")
	}
}

func TestLoginLimiter(t *testing.T) {
	l := newLoginLimiter()
	now := time.Now()
	for i := 0; i < maxLoginFails; i++ {
		if !l.allowed("ada", now) {
			t.Fatalf("blocked too early at %d", i)
		}
		l.fail("ada", now)
	}
	if l.allowed("ada", now) {
		t.Error("should be blocked after max failures")
	}
	if !l.allowed("bob", now) {
		t.Error("other handles must not be affected")
	}
	if !l.allowed("ada", now.Add(loginFailWindow+time.Second)) {
		t.Error("block should end after the window")
	}
	l.fail("bob", now)
	l.reset("bob")
	if len(l.fails) != 1 && len(l.fails) != 0 {
		t.Errorf("reset left entries: %v", l.fails)
	}
}
