package main

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
)

// Go and the browser code must agree byte for byte. tests/fixtures/e2ee-vectors.json holds synthetic test keys and a
// message made here; tests/e2ee_interop.test.js decrypts it in the browser code and writes e2ee-js-vector.json, which
// this test then opens on the server side (the same check a report disclosure goes through).
// Regenerate the first file with WRITE_VECTORS=1 go test -run Interop.

type vectors struct {
	AliceD   string    `json:"aliceD"`
	AlicePub string    `json:"alicePub"`
	BobD     string    `json:"bobD"`
	BobPub   string    `json:"bobPub"`
	Plain    string    `json:"plain"`
	Env      *envelope `json:"env"` // alice to bob, made in Go
}

func clientFromD(t *testing.T, handle, dB64 string) *testClient {
	t.Helper()
	d, _ := base64.StdEncoding.DecodeString(dB64)
	priv, err := ecdh.P256().NewPrivateKey(d)
	if err != nil {
		t.Fatal(err)
	}
	pub := priv.PublicKey().Bytes()
	return &testClient{handle: handle, priv: priv, pub: pub, fp: fingerprint(pub)}
}

func TestE2EEInteropWithBrowser(t *testing.T) {
	const path = "tests/fixtures/e2ee-vectors.json"
	if os.Getenv("WRITE_VECTORS") == "1" {
		a, b := newTestClient(t, "alice", 0), newTestClient(t, "bob", 0)
		env, _ := a.encrypt(t, b, "Interop vector: héllo ✓ 123")
		v := vectors{AliceD: base64.StdEncoding.EncodeToString(a.priv.Bytes()), AlicePub: a.pubB64(), BobD: base64.StdEncoding.EncodeToString(b.priv.Bytes()), BobPub: b.pubB64(), Plain: "Interop vector: héllo ✓ 123", Env: env}
		out, _ := json.MarshalIndent(v, "", "  ")
		if err := os.WriteFile(path, out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Skip("no vectors file")
	}
	var v vectors
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	alice, bob := clientFromD(t, "alice", v.AliceD), clientFromD(t, "bob", v.BobD)
	if pt, err := bob.decrypt(alice, v.Env, "alice", "bob"); err != nil || pt != v.Plain {
		t.Fatalf("the Go vector does not decrypt in Go: %q %v", pt, err)
	}
	// A message made by the browser code, with its revealed message key.
	jraw, err := os.ReadFile("tests/fixtures/e2ee-js-vector.json")
	if err != nil {
		t.Skip("run tests/e2ee_interop.test.js first")
	}
	var j struct {
		Plain string    `json:"plain"`
		Env   *envelope `json:"env"`
		Key   string    `json:"key"`
	}
	if err := json.Unmarshal(jraw, &j); err != nil {
		t.Fatal(err)
	}
	if err := j.Env.validate(); err != nil {
		t.Fatalf("the browser's envelope fails the server's validation: %v", err)
	}
	if pt, err := bob.decrypt(alice, j.Env, "alice", "bob"); err != nil || pt != j.Plain {
		t.Fatalf("Go could not decrypt what the browser encrypted: %q %v", pt, err)
	}
	row := dmRow{Enc: 1, Ct: j.Env.C, Nonce: j.Env.N, Sfp: j.Env.Sf, Rfp: j.Env.Rf, Mid: j.Env.Mid, SenderHandle: "alice", RecipientHandle: "bob"}
	if pt, err := openDisclosed(row, j.Key); err != nil || pt != j.Plain {
		t.Fatalf("the server could not verify a disclosure made by the browser: %q %v", pt, err)
	}
}
