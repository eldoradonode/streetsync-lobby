package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"strings"
	"testing"
	"time"
)

// Privacy guarantees of end-to-end encrypted direct messages, checked against a real database and the real
// WebSocket handler. Every message is synthetic and carries a unique marker so leaks can be searched for.

// dumpAll returns every row of every table as JSON text, so a marker can be searched for anywhere in storage.
func dumpAll(t *testing.T, st *store) string {
	t.Helper()
	rows, err := st.db.Query(`SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'`)
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var n string
		rows.Scan(&n)
		tables = append(tables, n)
	}
	rows.Close()
	var sb strings.Builder
	for _, n := range tables {
		r, err := st.db.Query(`SELECT row_to_json(x)::text FROM "` + n + `" x`)
		if err != nil {
			t.Fatal(err)
		}
		for r.Next() {
			var j string
			r.Scan(&j)
			sb.WriteString(j)
			sb.WriteByte('\n')
		}
		r.Close()
	}
	return sb.String()
}

func frameText(f map[string]any) string { b, _ := json.Marshal(f); return string(b) }

func TestE2EEKeyRegistrationAndAbuse(t *testing.T) {
	e := newTestEnv(t)
	e.account("t_ek_a", "user")
	e.account("t_ek_b", "user")
	a, b := e.login("t_ek_a"), e.login("t_ek_b")
	ca := newTestClient(t, "t_ek_a", mustID(t, e, "t_ek_a"))
	cb := newTestClient(t, "t_ek_b", mustID(t, e, "t_ek_b"))

	for name, pub := range map[string]string{
		"garbage":       "not base64!!",
		"short":         base64.StdEncoding.EncodeToString(ca.pub[:30]),
		"off curve":     base64.StdEncoding.EncodeToString(append([]byte{4}, bytes.Repeat([]byte{7}, 64)...)),
		"point at zero": base64.StdEncoding.EncodeToString(make([]byte, 65)),
	} {
		if got := a.ask(map[string]any{"t": "ekey", "pub": pub}, "ekeyok"); got != "err:bad_key" {
			t.Errorf("%s: %s", name, got)
		}
	}
	if got := a.ask(map[string]any{"t": "ekey", "pub": ca.pubB64()}, "ekeyok"); got != "ok:ekeyok" {
		t.Fatalf("first registration: %s", got)
	}
	if got := b.ask(map[string]any{"t": "ekey", "pub": cb.pubB64()}, "ekeyok"); got != "ok:ekeyok" {
		t.Fatalf("b registration: %s", got)
	}
	// Registering the same key again changes nothing.
	if got := a.ask(map[string]any{"t": "ekey", "pub": ca.pubB64()}, "ekeyok"); got != "ok:ekeyok" {
		t.Errorf("same key again: %s", got)
	}
	if n := e.count(`SELECT count(*) FROM e2e_key_log WHERE user_id = $1`, ca.id); n != 1 {
		t.Errorf("an unchanged key must not add history, got %d rows", n)
	}

	// The directory returns the owner's key, and a frame can not name another account to write.
	a.send(map[string]any{"t": "ekeyget", "h": "t_ek_b"})
	f := a.expect("ekeyis")
	if f["fp"] != cb.fp || f["pub"] != cb.pubB64() {
		t.Errorf("lookup returned %v", f)
	}
	// An attacker (a) tries to plant a key for b: every way of naming b is ignored, b's key stays.
	attacker := newTestClient(t, "t_ek_a", ca.id)
	if got := a.ask(map[string]any{"t": "ekey", "pub": attacker.pubB64(), "h": "t_ek_b", "id": cb.id, "user_id": cb.id}, "ekeyok"); got != "err:bad_login" {
		t.Errorf("planting a key for another account: %s", got)
	}
	var fpB string
	e.st.db.QueryRow(`SELECT fp FROM e2e_keys WHERE user_id = $1`, cb.id).Scan(&fpB)
	if fpB != cb.fp {
		t.Fatal("a user changed another user's key")
	}

	// Replacing your own key needs your password. Without it, and with a wrong one, nothing changes.
	var fpA string
	if got := a.ask(map[string]any{"t": "ekey", "pub": attacker.pubB64()}, "ekeyok"); got != "err:bad_login" {
		t.Errorf("replacement without a password: %s", got)
	}
	if got := a.ask(map[string]any{"t": "ekey", "pub": attacker.pubB64(), "o": "wrong"}, "ekeyok"); got != "err:bad_login" {
		t.Errorf("replacement with a wrong password: %s", got)
	}
	e.st.db.QueryRow(`SELECT fp FROM e2e_keys WHERE user_id = $1`, ca.id).Scan(&fpA)
	if fpA != ca.fp {
		t.Fatal("a key was replaced without the password")
	}
	if got := a.ask(map[string]any{"t": "ekey", "pub": attacker.pubB64(), "o": "password-t_ek_a"}, "ekeyok"); got != "ok:ekeyok" {
		t.Errorf("replacement with the password: %s", got)
	}
	e.st.db.QueryRow(`SELECT fp FROM e2e_keys WHERE user_id = $1`, ca.id).Scan(&fpA)
	if fpA != attacker.fp || e.count(`SELECT ver FROM e2e_keys WHERE user_id = $1`, ca.id) != 2 || e.count(`SELECT count(*) FROM e2e_key_log WHERE user_id = $1`, ca.id) != 2 {
		t.Error("a replacement must bump the version and leave a history row")
	}
	// Lookup rate limit.
	var last string
	for i := 0; i < keyLookupMax+3; i++ {
		last = a.ask(map[string]any{"t": "ekeyget", "h": "t_ek_b"}, "ekeyis")
	}
	if last != "err:slow_down" {
		t.Errorf("key lookups must be rate limited: %s", last)
	}
}

func TestE2EENoPlaintextAnywhere(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)

	e := newTestEnv(t)
	e.account("t_lk_a", "user")
	e.account("t_lk_b", "user")
	e.account("t_lk_mod", "mod")
	e.st.applyCredit(context.Background(), mustID(t, e, "t_lk_a"), "grant", 20, "t_lk_g", "")
	wa, wb := e.login("t_lk_a"), e.login("t_lk_b")
	ca, cb := e.clientFor("t_lk_a"), e.clientFor("t_lk_b")
	mk1, mk2, mk3 := "MARKER-PLAIN-ONE-7f3a9c", "MARKER-PLAIN-TWO-1b8d42", "MARKER-PLAIN-THREE-e05577"

	// 1. A plaintext frame is refused outright: nothing is stored or logged.
	if got := wa.ask(map[string]any{"t": "dm", "h": "t_lk_b", "b": mk1}, "dm"); got != "err:e2ee_required" {
		t.Fatalf("plaintext dm: %s", got)
	}
	// 2. A plaintext field smuggled next to an envelope is refused too.
	env, _ := ca.encrypt(t, cb, mk1)
	if got := wa.ask(map[string]any{"t": "dm", "h": "t_lk_b", "b": mk1, "e": env}, "dm"); got != "err:e2ee_required" {
		t.Fatalf("plaintext beside an envelope: %s", got)
	}
	if got := wa.ask(map[string]any{"t": "dm", "h": "t_lk_b", "e": map[string]any{"v": 1, "n": mk1, "c": mk1, "mid": mk1, "sf": mk1, "rf": mk1}}, "dm"); got != "err:bad_envelope" {
		t.Fatalf("malformed envelope: %s", got)
	}
	if n := e.count(`SELECT count(*) FROM dm_messages WHERE sender_id = $1`, ca.id); n != 0 {
		t.Fatalf("refused messages were stored: %d", n)
	}

	// 3. Real encrypted messages.
	var keys [][]byte
	sent := []string{mk1, mk2, mk3}
	for _, m := range sent {
		wb.drain()
		env, key := ca.encrypt(t, cb, m)
		keys = append(keys, key)
		if got := wa.ask(map[string]any{"t": "dm", "h": "t_lk_b", "e": env}, "dm"); got != "ok:dm" {
			t.Fatalf("encrypted dm: %s", got)
		}
	}
	// What B received over its WebSocket is ciphertext, and B's own key opens it.
	wb.drain()
	env4, _ := ca.encrypt(t, cb, "MARKER-LIVE-4c9d")
	wa.send(map[string]any{"t": "dm", "h": "t_lk_b", "e": env4})
	got := wb.expect("dm")
	txt := frameText(got)
	if strings.Contains(txt, "MARKER-LIVE") || got["b"] != "" {
		t.Errorf("the relayed frame carries readable text: %s", txt)
	}
	var recv envelope
	raw, _ := json.Marshal(got["e"])
	json.Unmarshal(raw, &recv)
	if pt, err := cb.decrypt(ca, &recv, "t_lk_a", "t_lk_b"); err != nil || pt != "MARKER-LIVE-4c9d" {
		t.Errorf("the recipient could not read it: %q %v", pt, err)
	}

	// 4. Thread, inbox, badge and notification frames carry no plaintext.
	wb.send(map[string]any{"t": "thread", "h": "t_lk_a"})
	thr := wb.expect("thread")
	wb.send(map[string]any{"t": "inbox"})
	inb := wb.expect("inbox")
	time.Sleep(500 * time.Millisecond) // notifications are written in the background
	wb.send(map[string]any{"t": "nlist"})
	notes := wb.expect("notes")
	for name, f := range map[string]map[string]any{"thread": thr, "inbox": inb, "notifications": notes} {
		s := frameText(f)
		for _, m := range append(sent, "MARKER-LIVE-4c9d") {
			if strings.Contains(s, m) {
				t.Errorf("%s frame leaks %s", name, m)
			}
		}
	}
	if !strings.Contains(frameText(notes), "encrypted") {
		t.Errorf("the notification should say it is encrypted: %s", frameText(notes))
	}

	// 5. Nothing in the database is readable, and no private key is stored anywhere.
	dump := dumpAll(t, e.st)
	for _, m := range append(sent, mk1, "MARKER-LIVE-4c9d") {
		if strings.Contains(dump, m) {
			t.Errorf("the database contains %s", m)
		}
	}
	var body string
	var enc int
	e.st.db.QueryRow(`SELECT body, enc FROM dm_messages WHERE id IN (SELECT max(id) FROM dm_messages WHERE sender_id = $1)`, ca.id).Scan(&body, &enc)
	if body != "" || enc != 1 {
		t.Errorf("stored row: body=%q enc=%d", body, enc)
	}
	for _, c := range []*testClient{ca, cb} {
		priv := c.priv.Bytes()
		for _, form := range []string{hex.EncodeToString(priv), base64.StdEncoding.EncodeToString(priv), base64.RawURLEncoding.EncodeToString(priv)} {
			if strings.Contains(dump, form) || strings.Contains(logs.String(), form) {
				t.Errorf("a private key of %s was found in storage or logs", c.handle)
			}
		}
	}
	// 6. Logs hold no message text, no key and no envelope.
	for _, m := range append(sent, "MARKER-LIVE-4c9d") {
		if strings.Contains(logs.String(), m) {
			t.Errorf("the server log contains %s", m)
		}
	}

	// 7. Everything the server holds (public keys, fingerprints, ciphertext, ids) does not open a message: a
	// key derived from public data alone, or by an attacker's own key pair, fails to decrypt it.
	var ct, nonce, mid, sfp, rfp string
	e.st.db.QueryRow(`SELECT ct, nonce, mid, sfp, rfp FROM dm_messages WHERE sender_id = $1 ORDER BY id LIMIT 1`, ca.id).Scan(&ct, &nonce, &mid, &sfp, &rfp)
	stored := dmRow{ID: 1, SenderHandle: "t_lk_a", RecipientHandle: "t_lk_b", Enc: 1, Ct: ct, Nonce: nonce, Sfp: sfp, Rfp: rfp, Mid: mid}
	server := newTestClient(t, "server", 0)
	if _, err := openDisclosed(stored, base64.StdEncoding.EncodeToString(server.msgKey(ca, mid))); err == nil {
		t.Error("a key from the server's own key pair opened a message")
	}
	if _, err := openDisclosed(stored, base64.StdEncoding.EncodeToString(hkdf([]byte(sfp+rfp), []byte("streetsync-dm-v1"), []byte(mid)))); err == nil {
		t.Error("a key derived from stored data opened a message")
	}
	if pt, err := openDisclosed(stored, base64.StdEncoding.EncodeToString(keys[0])); err != nil || pt != mk1 {
		t.Errorf("the real message key must open it: %q %v", pt, err)
	}

	// 8. Moving a ciphertext into another conversation or sender breaks decryption.
	moved := &envelope{V: 1, N: nonce, C: ct, Mid: mid, Sf: sfp, Rf: rfp}
	if _, err := cb.decrypt(ca, moved, "t_lk_c", "t_lk_b"); err == nil {
		t.Error("a message re-attributed to another sender still decrypted")
	}
	if _, err := cb.decrypt(ca, moved, "t_lk_a", "t_lk_c"); err == nil {
		t.Error("a message re-addressed to another recipient still decrypted")
	}
}

func TestE2EEKeyGuardsOnSend(t *testing.T) {
	e := newTestEnv(t)
	e.account("t_kg_a", "user")
	e.account("t_kg_b", "user")
	e.st.applyCredit(context.Background(), mustID(t, e, "t_kg_a"), "grant", 20, "t_kg_g", "")
	wa := e.login("t_kg_a")
	a := newTestClient(t, "t_kg_a", mustID(t, e, "t_kg_a"))
	b := newTestClient(t, "t_kg_b", mustID(t, e, "t_kg_b"))
	env, _ := a.encrypt(t, b, "synthetic")
	e.refill()
	if got := wa.ask(map[string]any{"t": "dm", "h": "t_kg_b", "e": env}, "dm"); got != "err:no_own_key" {
		t.Errorf("sender without a key: %s", got)
	}
	a.register(t, e.st)
	e.refill()
	if got := wa.ask(map[string]any{"t": "dm", "h": "t_kg_b", "e": env}, "dm"); got != "err:peer_no_key" {
		t.Errorf("recipient without a key: %s", got)
	}
	b.register(t, e.st)
	e.refill()
	if got := wa.ask(map[string]any{"t": "dm", "h": "t_kg_b", "e": env}, "dm"); got != "ok:dm" {
		t.Fatalf("with both keys: %s", got)
	}
	// Replay: the same envelope (same message id) is refused, and no credit is charged twice.
	bal := credits(t, e.st, a.id)
	e.refill()
	if got := wa.ask(map[string]any{"t": "dm", "h": "t_kg_b", "e": env}, "dm"); got != "err:replay" {
		t.Errorf("replayed envelope: %s", got)
	}
	if credits(t, e.st, a.id) != bal {
		t.Error("a replayed message was charged")
	}
	// The recipient rotates their key: a message made for the old key is refused until the sender re-checks.
	b2 := newTestClient(t, "t_kg_b", b.id)
	b2.register(t, e.st)
	e.refill()
	stale, _ := a.encrypt(t, b, "made for the old key")
	if got := wa.ask(map[string]any{"t": "dm", "h": "t_kg_b", "e": stale}, "dm"); got != "err:key_changed" {
		t.Errorf("stale recipient key: %s", got)
	}
	// A key claimed in the envelope that is not the sender's registered key is refused.
	forged, _ := b2.encrypt(t, b, "claims to be from b2's key")
	forged.Sf = b2.fp
	e.refill()
	if got := wa.ask(map[string]any{"t": "dm", "h": "t_kg_b", "e": forged}, "dm"); got != "err:key_changed" {
		t.Errorf("forged sender key: %s", got)
	}
}

func TestE2EEReportDisclosesOnlySelectedMessages(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	defer log.SetOutput(os.Stderr)

	e := newTestEnv(t)
	e.account("t_rp_bad", "user")
	e.account("t_rp_me", "user")
	e.account("t_rp_other", "user")
	e.account("t_rp_mod", "mod")
	e.account("t_rp_adm", "admin")
	e.st.applyCredit(context.Background(), mustID(t, e, "t_rp_bad"), "grant", 50, "t_rp_g", "")
	bad, me, other := e.login("t_rp_bad"), e.login("t_rp_me"), e.login("t_rp_other")
	cbad, cme := e.clientFor("t_rp_bad"), e.clientFor("t_rp_me")
	e.clientFor("t_rp_other")

	msgs := []string{"REPORT-BEFORE-aa11-harmless", "REPORT-ABUSIVE-bb22-selected", "REPORT-AFTER-cc33-harmless"}
	var ids []int64
	var keys [][]byte
	for _, m := range msgs {
		me.drain()
		env, key := cbad.encrypt(t, cme, m)
		if got := bad.ask(map[string]any{"t": "dm", "h": "t_rp_me", "e": env}, "dm"); got != "ok:dm" {
			t.Fatalf("send: %s", got)
		}
		f := me.expect("dm")
		ids = append(ids, int64(f["i"].(float64)))
		keys = append(keys, key)
	}
	b64 := base64.StdEncoding.EncodeToString

	// Without a key the server has nothing to read: it refuses and files nothing.
	if got := me.ask(map[string]any{"t": "drep", "i": ids[1], "c": "abuse"}, "repok"); got != "err:needs_disclosure" {
		t.Errorf("report without disclosure: %s", got)
	}
	// A key for a different message does not open this one.
	if got := me.ask(map[string]any{"t": "drep", "c": "abuse", "it": []map[string]any{{"i": ids[1], "k": b64(keys[0])}}}, "repok"); got != "err:bad_disclosure" {
		t.Errorf("wrong message key: %s", got)
	}
	if got := me.ask(map[string]any{"t": "drep", "c": "abuse", "it": []map[string]any{{"i": ids[1], "k": b64(make([]byte, 32))}}}, "repok"); got != "err:bad_disclosure" {
		t.Errorf("zero key: %s", got)
	}
	// The sender and a stranger can not report it.
	if got := bad.ask(map[string]any{"t": "drep", "c": "abuse", "it": []map[string]any{{"i": ids[1], "k": b64(keys[1])}}}, "repok"); got != "err:report_gone" && got != "err:self_report" {
		t.Errorf("sender reporting own message: %s", got)
	}
	if got := other.ask(map[string]any{"t": "drep", "c": "abuse", "it": []map[string]any{{"i": ids[1], "k": b64(keys[1])}}}, "repok"); got != "err:report_gone" {
		t.Errorf("a stranger holding the key: %s", got)
	}
	if n := e.count(`SELECT count(*) FROM reports WHERE reporter_handle LIKE 't\_rp\_%'`); n != 0 {
		t.Fatalf("refused reports left %d rows", n)
	}

	// The reporter selects ONLY the second message.
	if got := me.ask(map[string]any{"t": "drep", "c": "abuse", "it": []map[string]any{{"i": ids[1], "k": b64(keys[1])}}}, "repok"); got != "ok:repok" {
		t.Fatalf("selected report: %s", got)
	}
	if n := e.count(`SELECT count(*) FROM reports WHERE reporter_handle = 't_rp_me'`); n != 1 {
		t.Fatalf("expected exactly one report row, got %d", n)
	}
	var body string
	e.st.db.QueryRow(`SELECT body FROM reports WHERE reporter_handle = 't_rp_me'`).Scan(&body)
	if !strings.Contains(body, msgs[1]) {
		t.Errorf("the selected message must be in the report: %q", body)
	}
	dump := dumpAll(t, e.st)
	for _, m := range []string{msgs[0], msgs[2]} {
		if strings.Contains(dump, m) {
			t.Errorf("an unselected message is readable in storage: %s", m)
		}
	}
	// A moderator sees the report with the selected message and nothing around it.
	mod := e.login("t_rp_mod")
	mod.send(map[string]any{"t": "reports"})
	list := frameText(mod.expect("reps"))
	if !strings.Contains(list, msgs[1]) || strings.Contains(list, msgs[0]) || strings.Contains(list, msgs[2]) {
		t.Errorf("the staff report list: %s", list)
	}
	// The same message reported twice is a duplicate.
	if got := me.ask(map[string]any{"t": "drep", "c": "abuse", "it": []map[string]any{{"i": ids[1], "k": b64(keys[1])}}}, "repok"); got != "err:already_reported" {
		t.Errorf("duplicate: %s", got)
	}
	// Too many items in one report are refused.
	many := []map[string]any{}
	for i := 0; i < maxReportItems+1; i++ {
		many = append(many, map[string]any{"i": ids[0], "k": b64(keys[0])})
	}
	if got := me.ask(map[string]any{"t": "drep", "c": "abuse", "it": many}, "repok"); got != "err:bad_report" {
		t.Errorf("too many items: %s", got)
	}
	// Reporting two chosen messages at once files exactly those two.
	if got := me.ask(map[string]any{"t": "drep", "c": "spam", "it": []map[string]any{{"i": ids[0], "k": b64(keys[0])}, {"i": ids[2], "k": b64(keys[2])}}}, "repok"); got != "ok:repok" {
		t.Errorf("two selected: %s", got)
	}
	if n := e.count(`SELECT count(*) FROM reports WHERE reporter_handle = 't_rp_me'`); n != 3 {
		t.Errorf("rows after selecting all three: %d", n)
	}
	if strings.Contains(logs.String(), "REPORT-") {
		t.Error("report text appeared in the server log")
	}
}

func TestE2EELegacyPlaintextStaysUntilRetention(t *testing.T) {
	e := newTestEnv(t)
	u := e.account("t_lg_a", "user")
	v := e.account("t_lg_b", "user")
	ctx := context.Background()
	e.st.applyCredit(ctx, u.ID, "grant", 20, "t_lg_g", "")
	// Seed legacy rows the way older versions stored them.
	if _, _, err := e.st.sendDM(ctx, 9600001, u.ID, v.ID, "t_lg_b", "legacy readable text", time.Now().Add(-8*24*time.Hour).UnixMilli(), 0); err != nil {
		t.Fatal(err)
	}
	if _, _, err := e.st.sendDM(ctx, 9600002, u.ID, v.ID, "t_lg_b", "legacy recent text", time.Now().UnixMilli(), 0); err != nil {
		t.Fatal(err)
	}
	e.st.db.Exec(`UPDATE dm_messages SET created_at = now() - interval '8 days' WHERE id = 9600001`)
	ca, cb := e.clientFor("t_lg_a"), e.clientFor("t_lg_b")
	env, _ := ca.encrypt(t, cb, "new encrypted")
	wa := e.login("t_lg_a")
	if got := wa.ask(map[string]any{"t": "dm", "h": "t_lg_b", "e": env}, "dm"); got != "ok:dm" {
		t.Fatalf("new encrypted message in an old conversation: %s", got)
	}
	// Legacy rows are served as they were, marked by having a body and no envelope.
	wb := e.login("t_lg_b")
	wb.send(map[string]any{"t": "thread", "h": "t_lg_a"})
	thr := wb.expect("thread")
	txt := frameText(thr)
	if !strings.Contains(txt, "legacy recent text") || strings.Contains(txt, "new encrypted") {
		t.Errorf("thread: %s", txt)
	}
	// Retention removes the expired legacy row, then the recent one stays; encrypted rows age out the same way.
	e.h.cleanupOnce(ctx)
	if e.count(`SELECT count(*) FROM dm_messages WHERE id = 9600001`) != 0 || e.count(`SELECT count(*) FROM dm_messages WHERE id = 9600002`) != 1 {
		t.Error("legacy rows must follow the same 7 day window")
	}
	e.st.db.Exec(`UPDATE dm_messages SET created_at = now() - interval '9 days' WHERE enc = 1 AND sender_id = $1`, u.ID)
	e.h.cleanupOnce(ctx)
	if e.count(`SELECT count(*) FROM dm_messages WHERE enc = 1 AND sender_id = $1`, u.ID) != 0 {
		t.Error("expired encrypted rows must be deleted too")
	}
}
