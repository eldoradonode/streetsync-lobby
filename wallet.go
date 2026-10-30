package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
)

// Credit rules. Starting values, easy to tune.
// Credits are earned only by real chatting, never by idle time, same as points.
const (
	welcomeCredits = 20     // every account gets this once
	dailyCredits   = 2      // first real message of each UTC day
	maxGrant       = 100000 // largest single grant an admin can make
	ledgerPage     = 30     // entries sent with the wallet
)

// streakMilestones pay extra on the day a streak reaches exactly this many days.
var streakMilestones = map[int]int64{3: 5, 7: 15, 14: 30, 30: 60}

var errNoFunds = errors.New("not enough credits")

// ---- store ----

type ledgerRow struct {
	ID      int64  `json:"i"`
	Kind    string `json:"k"`
	Amount  int64  `json:"a"`
	Balance int64  `json:"b"`
	Note    string `json:"n"`
	TS      int64  `json:"ts"`
}

// applyCreditTx moves credits inside a transaction the caller owns.
// It returns the new balance and whether it was applied. A ref that was already used
// is not applied again (applied is false, no error). Spending more than the balance
// returns errNoFunds and changes nothing.
var errAdminWallet = errors.New("admin accounts do not use credits")

func applyCreditTx(ctx context.Context, tx *sql.Tx, userID int64, kind string, amount int64, ref, note string) (int64, bool, error) {
	if amount == 0 || ref == "" || kind == "" {
		return 0, false, errors.New("bad credit entry")
	}
	// Admin accounts are not part of the credit economy. This is the only place credits move, so the rule holds
	// for tips, direct messages, room fees, grants and anything added later. An old balance is left untouched
	// (never deleted), it just can not change: see docs/WALLET_POLICY.md.
	var role string
	if err := tx.QueryRowContext(ctx, `SELECT role FROM users WHERE id = $1`, userID).Scan(&role); err != nil {
		return 0, false, err
	}
	if role == "admin" {
		return 0, false, errAdminWallet
	}
	var id int64
	err := tx.QueryRowContext(ctx,
		`INSERT INTO credit_ledger (user_id, kind, amount, balance_after, ref, note) VALUES ($1, $2, $3, 0, $4, $5)
		 ON CONFLICT (user_id, ref) DO NOTHING RETURNING id`, userID, kind, amount, ref, note).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		var bal int64
		if err := tx.QueryRowContext(ctx, `SELECT credits FROM users WHERE id = $1`, userID).Scan(&bal); err != nil {
			return 0, false, err
		}
		return bal, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	var bal int64
	err = tx.QueryRowContext(ctx,
		`UPDATE users SET credits = credits + $2 WHERE id = $1 AND credits + $2 >= 0 RETURNING credits`, userID, amount).Scan(&bal)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, errNoFunds
	}
	if err != nil {
		return 0, false, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE credit_ledger SET balance_after = $2 WHERE id = $1`, id, bal); err != nil {
		return 0, false, err
	}
	return bal, true, nil
}

// applyCredit is one credit movement in its own transaction.
func (s *store) applyCredit(ctx context.Context, userID int64, kind string, amount int64, ref, note string) (int64, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	bal, applied, err := applyCreditTx(ctx, tx, userID, kind, amount, ref, note)
	if err != nil || !applied {
		return bal, false, err
	}
	return bal, true, tx.Commit()
}

// grantWelcome gives the one time welcome credits. Safe to call on every connection:
// only the first call for an account does anything.
func (s *store) grantWelcome(ctx context.Context, userID int64) (int64, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE users SET welcomed = true WHERE id = $1 AND NOT welcomed`, userID)
	if err != nil {
		return 0, false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, false, nil
	}
	var role string
	if err := tx.QueryRowContext(ctx, `SELECT role FROM users WHERE id = $1`, userID).Scan(&role); err != nil {
		return 0, false, err
	}
	if role == "admin" { // admins are outside the credit economy: no gift, and it is not asked for again
		return 0, false, tx.Commit()
	}
	bal, applied, err := applyCreditTx(ctx, tx, userID, "welcome", welcomeCredits, "welcome", "Welcome gift")
	if err != nil || !applied {
		return bal, false, err
	}
	return bal, true, tx.Commit()
}

func (s *store) creditBalance(ctx context.Context, userID int64) (int64, error) {
	var bal int64
	err := s.db.QueryRowContext(ctx, `SELECT credits FROM users WHERE id = $1`, userID).Scan(&bal)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, errNotFound
	}
	return bal, err
}

func (s *store) ledgerFor(ctx context.Context, userID int64, limit int) ([]ledgerRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, kind, amount, balance_after, note, (extract(epoch from created_at) * 1000)::bigint
		 FROM credit_ledger WHERE user_id = $1 ORDER BY id DESC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ledgerRow{}
	for rows.Next() {
		var r ledgerRow
		if err := rows.Scan(&r.ID, &r.Kind, &r.Amount, &r.Balance, &r.Note, &r.TS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- rewards ----

type reward struct {
	Kind   string
	Amount int64
	Ref    string
	Note   string
}

// streakRewards is what a new streak day pays: the daily credits, plus a milestone bonus
// when the streak has just reached a milestone. day is days since 1970-01-01 UTC.
func streakRewards(day, streak int) []reward {
	out := []reward{{"daily", dailyCredits, fmt.Sprintf("daily:%d", day), "Daily chat bonus"}}
	if amt, ok := streakMilestones[streak]; ok {
		out = append(out, reward{"streak", amt, fmt.Sprintf("streak%d:%d", streak, day), fmt.Sprintf("%d day streak", streak)})
	}
	return out
}

// setCredits records a new balance and tells every open tab of that user.
func (h *hub) setCredits(ou *onlineUser, bal int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ou.credits = bal
	f := frame(meFrame{T: "me", meFields: ou.me()})
	for c := range ou.clients {
		c.enqueue(f)
	}
}

// awardStreak pays the rewards of a new streak day. It runs in the background so chat never waits for it.
func (h *hub) awardStreak(ou *onlineUser, day, streak int) {
	h.mu.Lock()
	isAdmin := ou.role == "admin"
	h.mu.Unlock()
	if isAdmin {
		return // admins do not earn credits
	}
	ctx, cancel := context.WithTimeout(context.Background(), authWait)
	defer cancel()
	var bal int64
	got := false
	for _, r := range streakRewards(day, streak) {
		b, applied, err := h.st.applyCredit(ctx, ou.id, r.Kind, r.Amount, r.Ref, r.Note)
		if err != nil {
			log.Printf("paying %s credits failed: %v", r.Kind, err)
			continue
		}
		if applied {
			bal, got = b, true
		}
	}
	if got {
		h.setCredits(ou, bal)
	}
}

// welcome gives the welcome credits to an account that has not had them yet.
func (h *hub) welcome(ou *onlineUser) {
	ctx, cancel := context.WithTimeout(context.Background(), authWait)
	defer cancel()
	bal, applied, err := h.st.grantWelcome(ctx, ou.id)
	if err != nil {
		log.Printf("welcome credits failed: %v", err)
		return
	}
	if applied {
		h.setCredits(ou, bal)
	}
}

// ---- wire ----

type walletFrame struct {
	T string      `json:"t"`
	C int64       `json:"c"`
	L []ledgerRow `json:"l"`
}

// sendWallet sends the balance and the latest ledger entries, read fresh from the database.
func (h *hub) sendWallet(ctx context.Context, cl *client) {
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	ou := cl.user
	h.mu.Lock()
	isAdmin := ou.role == "admin"
	h.mu.Unlock()
	if isAdmin {
		cl.enqueue(errFrame("admin_no_wallet"))
		return
	}
	bal, err := h.st.creditBalance(dctx, ou.id)
	if err == nil {
		var rows []ledgerRow
		if rows, err = h.st.ledgerFor(dctx, ou.id, ledgerPage); err == nil {
			cl.enqueue(frame(walletFrame{T: "wallet", C: bal, L: rows}))
			h.mu.Lock()
			changed := ou.credits != bal
			h.mu.Unlock()
			if changed {
				h.setCredits(ou, bal)
			}
			return
		}
	}
	log.Printf("wallet read failed: %v", err)
	cl.enqueue(errFrame("server_busy"))
}

// adminGrant lets the admin hand out credits, for rewards, support and testing.
// {"t":"mod","a":"grant","h":"bob","m":50}   m is the number of credits
func (h *hub) adminGrant(ctx context.Context, cl *client, in inbound) {
	actor := cl.user
	h.mu.Lock()
	role := actor.role
	h.mu.Unlock()
	if role != "admin" {
		cl.enqueue(errFrame("forbidden"))
		return
	}
	if in.M <= 0 || in.M > maxGrant {
		cl.enqueue(errFrame("bad_amount"))
		return
	}
	_, key, ok := normHandle(in.H)
	if !ok {
		cl.enqueue(errFrame("no_such_user"))
		return
	}
	target, err := h.st.userByKey(ctx, key)
	if errors.Is(err, errNotFound) {
		cl.enqueue(errFrame("no_such_user"))
		return
	}
	if err != nil {
		log.Printf("grant lookup failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if target.Role == "admin" {
		cl.enqueue(errFrame("admin_no_wallet"))
		return
	}
	ref := "grant:" + newToken()[:16]
	bal, _, err := h.st.applyCredit(ctx, target.ID, "grant", int64(in.M), ref, "Gift from the team")
	if err != nil {
		log.Printf("grant failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	h.st.logMod(ctx, actor.handle, "grant", target.Handle, fmt.Sprintf("%d credits", in.M))
	h.mu.Lock()
	ou := h.users[target.ID]
	h.mu.Unlock()
	if ou != nil {
		h.setCredits(ou, bal)
	}
	cl.enqueue(frame(map[string]any{"t": "modok", "a": "grant", "h": target.Handle, "m": in.M}))
}
