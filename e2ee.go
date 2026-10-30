package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"
)

// End-to-end encrypted direct messages.
//
// The browser holds each person's private key. The server stores and relays ciphertext, public keys and the
// minimum metadata it needs for delivery (who, when, fingerprints, a random message id). It cannot decrypt.
// Nothing in this file ever sees a private key or a conversation secret. See docs/E2EE.md.
//
// Scheme (version 1, written in ui/src/lib/e2ee.js):
//
//	identity key   ECDH P-256, generated in the browser, private half not extractable
//	shared secret  ECDH(my private, their public), 32 bytes, never sent anywhere
//	message key    HKDF-SHA256(secret, salt "streetsync-dm-v1", info "msgkey|<fpLow>|<fpHigh>|<mid>")
//	cipher         AES-256-GCM, random 96 bit nonce, AAD binds mid, both account handles and both key fingerprints
//
// A reporter can reveal ONE message key. The server checks it by decrypting the stored ciphertext itself, so a
// report cannot be forged, and the message key opens nothing else (HKDF is one way).
const (
	e2eeVersion     = 1
	maxCipherBytes  = 4*maxMsgRunes + 64 // UTF-8 worst case plus the 16 byte tag, with room to spare
	maxReportItems  = 5
	keyLookupWindow = time.Minute
	keyLookupMax    = 60
)

var (
	errBadEnvelope = errors.New("bad envelope")
	errNoOwnKey    = errors.New("sender has no key")
	errPeerNoKey   = errors.New("recipient has no key")
	errKeyChanged  = errors.New("key changed")
	errReplay      = errors.New("message id already used")
)

// envelope is what the page sends and receives for one encrypted message. All fields are base64 or hex text.
type envelope struct {
	V   int    `json:"v"`
	N   string `json:"n"`   // nonce, base64, 12 bytes
	C   string `json:"c"`   // ciphertext and tag, base64
	Mid string `json:"mid"` // random 16 byte message id, base64url
	Sf  string `json:"sf"`  // sender key fingerprint, hex
	Rf  string `json:"rf"`  // recipient key fingerprint, hex
}

func isHexFP(s string) bool {
	if len(s) != 64 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// validate checks the shape of an envelope. It can not and does not look inside the ciphertext.
func (e *envelope) validate() error {
	if e == nil || e.V != e2eeVersion || !isHexFP(e.Sf) || !isHexFP(e.Rf) {
		return errBadEnvelope
	}
	if n, err := base64.StdEncoding.DecodeString(e.N); err != nil || len(n) != 12 {
		return errBadEnvelope
	}
	if c, err := base64.StdEncoding.DecodeString(e.C); err != nil || len(c) < 17 || len(c) > maxCipherBytes {
		return errBadEnvelope
	}
	if m, err := base64.RawURLEncoding.DecodeString(e.Mid); err != nil || len(m) != 16 {
		return errBadEnvelope
	}
	return nil
}

// dmAAD is the additional authenticated data for a message. It is rebuilt from database columns when a report is
// checked, so a message moved to another conversation, sender or key pair no longer decrypts.
// Accounts are named by their lower case handle, which is what the page knows for both sides.
func dmAAD(mid, senderHandle, recipientHandle, sfp, rfp string) []byte {
	return []byte(fmt.Sprintf("streetsync-dm-v1|%s|%s|%s|%s|%s", mid, strings.ToLower(senderHandle), strings.ToLower(recipientHandle), sfp, rfp))
}

// ---- keys ----

type keyRow struct {
	Pub string `json:"pub"`
	Fp  string `json:"fp"`
	Ver int    `json:"ver"`
}

// validPublicKey accepts only a real P-256 point. crypto/ecdh rejects wrong lengths, the point at infinity and
// points that are not on the curve.
func validPublicKey(b64 string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != 65 {
		return nil, errBadEnvelope
	}
	if _, err := ecdh.P256().NewPublicKey(raw); err != nil {
		return nil, errBadEnvelope
	}
	return raw, nil
}

func fingerprint(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (s *store) keyOf(ctx context.Context, userID int64) (keyRow, bool, error) {
	var k keyRow
	err := s.db.QueryRowContext(ctx, `SELECT pub, fp, ver FROM e2e_keys WHERE user_id = $1`, userID).Scan(&k.Pub, &k.Fp, &k.Ver)
	if errors.Is(err, sql.ErrNoRows) {
		return keyRow{}, false, nil
	}
	return k, err == nil, err
}

// registerKey stores the person's own key. The first key is accepted as is. A different key replaces the old one
// (version goes up, history row added). The caller has already decided whether a replacement is allowed.
func (s *store) registerKey(ctx context.Context, userID int64, pub, fp string) (keyRow, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return keyRow{}, false, err
	}
	defer tx.Rollback()
	var cur keyRow
	err = tx.QueryRowContext(ctx, `SELECT pub, fp, ver FROM e2e_keys WHERE user_id = $1 FOR UPDATE`, userID).Scan(&cur.Pub, &cur.Fp, &cur.Ver)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		cur = keyRow{Pub: pub, Fp: fp, Ver: 1}
		if _, err = tx.ExecContext(ctx, `INSERT INTO e2e_keys (user_id, pub, fp, ver) VALUES ($1,$2,$3,1)`, userID, pub, fp); err != nil {
			return keyRow{}, false, err
		}
	case err != nil:
		return keyRow{}, false, err
	case cur.Fp == fp:
		return cur, false, tx.Commit() // same key again: nothing changes
	default:
		cur = keyRow{Pub: pub, Fp: fp, Ver: cur.Ver + 1}
		if _, err = tx.ExecContext(ctx, `UPDATE e2e_keys SET pub = $2, fp = $3, ver = $4, updated_at = now() WHERE user_id = $1`, userID, pub, fp, cur.Ver); err != nil {
			return keyRow{}, false, err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO e2e_key_log (user_id, fp, ver) VALUES ($1,$2,$3)`, userID, fp, cur.Ver); err != nil {
		return keyRow{}, false, err
	}
	return cur, true, tx.Commit()
}

// ---- frames ----

// keyRegister handles {"t":"ekey","pub":"<base64>","o":"current password"}. The key always belongs to the
// authenticated connection's own account: there is no way to name another account. Replacing an existing key
// needs the account password (accounts that have one), because a replaced key is how an attacker would take over
// future conversations. Contacts are warned by their own device when a key changes.
func (h *hub) keyRegister(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	raw, err := validPublicKey(in.Pub)
	if err != nil {
		cl.enqueue(errFrame("bad_key"))
		return
	}
	fp := fingerprint(raw)
	dctx, cancel := context.WithTimeout(ctx, 2*authWait)
	defer cancel()
	cur, has, err := h.st.keyOf(dctx, ou.id)
	if err != nil {
		log.Printf("loading a key failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if has && cur.Fp != fp {
		h.mu.Lock()
		hasPass, lkey := ou.hasPass, strings.ToLower(ou.handle)
		h.mu.Unlock()
		if hasPass {
			now := time.Now()
			if !h.limit.allowed(lkey, now) {
				cl.enqueue(errFrame("too_many_attempts"))
				return
			}
			row, uerr := h.st.userByKey(dctx, lkey)
			if uerr != nil || row.ID != ou.id || !row.PassHash.Valid || len(in.O) == 0 || len(in.O) > maxPassBytes {
				h.limit.fail(lkey, now)
				cl.enqueue(errFrame("bad_login"))
				return
			}
			ok, verr := h.auth.verify(dctx, row.PassHash.String, in.O)
			if verr != nil {
				cl.enqueue(errFrame("server_busy"))
				return
			}
			if !ok {
				h.limit.fail(lkey, now)
				cl.enqueue(errFrame("bad_login"))
				return
			}
			h.limit.reset(lkey)
		}
	}
	k, changed, err := h.st.registerKey(dctx, ou.id, in.Pub, fp)
	if err != nil {
		log.Printf("saving a key failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	cl.enqueue(frame(map[string]any{"t": "ekeyok", "fp": k.Fp, "ver": k.Ver, "new": changed}))
}

// keyLookupOK is a small per-person limit on key lookups, so the directory can not be scraped.
func (h *hub) keyLookupOK(ou *onlineUser, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	keep := ou.keyTimes[:0]
	for _, t := range ou.keyTimes {
		if now.Sub(t) < keyLookupWindow {
			keep = append(keep, t)
		}
	}
	ou.keyTimes = keep
	if len(keep) >= keyLookupMax {
		return false
	}
	ou.keyTimes = append(ou.keyTimes, now)
	return true
}

// keyGet handles {"t":"ekeyget","h":"bob"}: the public key and fingerprint of an account, or none.
func (h *hub) keyGet(ctx context.Context, cl *client, in inbound) {
	if !h.keyLookupOK(cl.user, time.Now()) {
		cl.enqueue(errFrame("slow_down"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	other, ok := h.lookupUser(dctx, cl, in.H)
	if !ok {
		return
	}
	k, has, err := h.st.keyOf(dctx, other.ID)
	if err != nil {
		cl.enqueue(errFrame("server_busy"))
		return
	}
	out := map[string]any{"t": "ekeyis", "h": other.Handle}
	if has {
		out["pub"], out["fp"], out["ver"] = k.Pub, k.Fp, k.Ver
	}
	cl.enqueue(frame(out))
}

// ---- stored messages ----

// dmRow is one stored message with what is needed to check a disclosed key.
type dmRow struct {
	ID, SenderID, RecipientID     int64
	SenderHandle, RecipientHandle string
	Enc                           int
	Body, Ct, Nonce               string
	Sfp, Rfp, Mid                 string
}

func (s *store) dmFull(ctx context.Context, id int64) (dmRow, error) {
	var r dmRow
	var ct, nonce, sfp, rfp, mid sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT m.id, m.sender_id, m.recipient_id, u.handle, v.handle, m.enc, m.body, m.ct, m.nonce, m.sfp, m.rfp, m.mid
		 FROM dm_messages m JOIN users u ON u.id = m.sender_id JOIN users v ON v.id = m.recipient_id WHERE m.id = $1`, id).
		Scan(&r.ID, &r.SenderID, &r.RecipientID, &r.SenderHandle, &r.RecipientHandle, &r.Enc, &r.Body, &ct, &nonce, &sfp, &rfp, &mid)
	if errors.Is(err, sql.ErrNoRows) {
		return r, errNotFound
	}
	r.Ct, r.Nonce, r.Sfp, r.Rfp, r.Mid = ct.String, nonce.String, sfp.String, rfp.String, mid.String
	return r, err
}

// openDisclosed decrypts one stored message with the message key the reporter chose to reveal. It returns an error
// if the key does not open exactly this stored message, so a reporter can not attach invented text.
func openDisclosed(r dmRow, keyB64 string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(keyB64)
	if err != nil || len(key) != 32 || r.Enc != 1 {
		return "", errBadEnvelope
	}
	nonce, err1 := base64.StdEncoding.DecodeString(r.Nonce)
	ct, err2 := base64.StdEncoding.DecodeString(r.Ct)
	if err1 != nil || err2 != nil {
		return "", errBadEnvelope
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", errBadEnvelope
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", errBadEnvelope
	}
	pt, err := gcm.Open(nil, nonce, ct, dmAAD(r.Mid, r.SenderHandle, r.RecipientHandle, r.Sfp, r.Rfp))
	if err != nil || !utf8.Valid(pt) || utf8.RuneCount(pt) > maxMsgRunes {
		return "", errBadEnvelope
	}
	return clean(string(pt)), nil
}
