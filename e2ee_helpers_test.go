package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"sort"
	"testing"
	"time"
)

// sendDM seeds a LEGACY plaintext direct message, the kind older versions wrote. Production code has no such
// path any more: the server only accepts encrypted envelopes (sendDMRow with Env).
func (s *store) sendDM(ctx context.Context, id, from, to int64, toHandle, body string, ms, fee int64) (int64, bool, error) {
	return s.sendDMRow(ctx, id, from, to, toHandle, dmInsert{Body: body}, ms, fee)
}

// hkdf is HKDF-SHA256 (RFC 5869) for one 32 byte output block, matching WebCrypto's HKDF in the page.
func hkdf(secret, salt, info []byte) []byte {
	ext := hmac.New(sha256.New, salt)
	ext.Write(secret)
	prk := ext.Sum(nil)
	exp := hmac.New(sha256.New, prk)
	exp.Write(info)
	exp.Write([]byte{1})
	return exp.Sum(nil)
}

// testClient plays the browser: it owns a private key the server never sees.
type testClient struct {
	handle string
	id     int64
	priv   *ecdh.PrivateKey
	pub    []byte
	fp     string
}

func newTestClient(t *testing.T, handle string, id int64) *testClient {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PublicKey().Bytes()
	return &testClient{handle: handle, id: id, priv: priv, pub: pub, fp: fingerprint(pub)}
}

func (c *testClient) pubB64() string { return base64.StdEncoding.EncodeToString(c.pub) }

// register stores the public key directly in the database (the page does it with an "ekey" frame).
func (c *testClient) register(t *testing.T, st *store) {
	t.Helper()
	if _, _, err := st.registerKey(context.Background(), c.id, c.pubB64(), c.fp); err != nil {
		t.Fatal(err)
	}
}

func (c *testClient) msgKey(peer *testClient, mid string) []byte {
	secret, err := c.priv.ECDH(mustPub(peer.pub))
	if err != nil {
		panic(err)
	}
	lo, hi := c.fp, peer.fp
	if hi < lo {
		lo, hi = hi, lo
	}
	return hkdf(secret, []byte("streetsync-dm-v1"), []byte(fmt.Sprintf("msgkey|%s|%s|%s", lo, hi, mid)))
}

func mustPub(raw []byte) *ecdh.PublicKey {
	k, err := ecdh.P256().NewPublicKey(raw)
	if err != nil {
		panic(err)
	}
	return k
}

// encrypt builds the envelope the page would send from c to peer, and returns the message key too.
func (c *testClient) encrypt(t *testing.T, peer *testClient, plaintext string) (*envelope, []byte) {
	t.Helper()
	midRaw := make([]byte, 16)
	nonce := make([]byte, 12)
	rand.Read(midRaw)
	rand.Read(nonce)
	mid := base64.RawURLEncoding.EncodeToString(midRaw)
	key := c.msgKey(peer, mid)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	ct := gcm.Seal(nil, nonce, []byte(plaintext), dmAAD(mid, c.handle, peer.handle, c.fp, peer.fp))
	return &envelope{V: e2eeVersion, N: base64.StdEncoding.EncodeToString(nonce), C: base64.StdEncoding.EncodeToString(ct), Mid: mid, Sf: c.fp, Rf: peer.fp}, key
}

// decrypt is what the receiving page does.
func (c *testClient) decrypt(peer *testClient, e *envelope, sender, recipient string) (string, error) {
	key := c.msgKey(peer, e.Mid)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	n, _ := base64.StdEncoding.DecodeString(e.N)
	ct, _ := base64.StdEncoding.DecodeString(e.C)
	pt, err := gcm.Open(nil, n, ct, dmAAD(e.Mid, sender, recipient, e.Sf, e.Rf))
	return string(pt), err
}

// clientFor makes (once per handle) a client with a registered key for an existing account.
func (e *testEnv) clientFor(handle string) *testClient {
	e.t.Helper()
	if e.clients == nil {
		e.clients = map[string]*testClient{}
	}
	if c := e.clients[handle]; c != nil {
		return c
	}
	c := newTestClient(e.t, handle, mustID(e.t, e, handle))
	c.register(e.t, e.st)
	e.clients[handle] = c
	return c
}

// dmFrame is a valid encrypted "dm" frame from one account to another, with a synthetic message.
func (e *testEnv) dmFrame(from, to, plaintext string) map[string]any {
	e.t.Helper()
	a, b := e.clientFor(from), e.clientFor(to)
	env, _ := a.encrypt(e.t, b, plaintext)
	return map[string]any{"t": "dm", "h": to, "e": env}
}

func sortedPair(a, b string) (string, string) {
	s := []string{a, b}
	sort.Strings(s)
	return s[0], s[1]
}

// refill tops up every online person's message bucket, so tests that send many frames do not trip the rate limit.
func (e *testEnv) refill() {
	e.h.mu.Lock()
	defer e.h.mu.Unlock()
	for _, ou := range e.h.users {
		ou.tokens = burstMessages
		ou.lastRefill = time.Now()
	}
}
