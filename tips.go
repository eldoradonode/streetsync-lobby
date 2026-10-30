package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"
)

// Tips are fixed amounts, each with a name. The same list is in ui/src/lib/tips.js, keep them together.
var tipOptions = []struct {
	Amount int
	Name   string
}{{1, "Suya"}, {5, "Burger"}, {10, "Pizza"}, {25, "Cake"}, {50, "Feast"}}

func tipName(amount int) (string, bool) {
	for _, o := range tipOptions {
		if o.Amount == amount {
			return o.Name, true
		}
	}
	return "", false
}

// A tip request carries a key made by the page. The same key can only ever move credits once.
var tipKeyRE = regexp.MustCompile(`^[A-Za-z0-9_-]{8,64}$`)

// ---- store ----

// transferCredits moves credits from one person to another in one transaction: either both
// balances change and both ledger entries exist, or nothing happens. The sender's entry uses
// ref and the recipient's uses a ref that includes the sender, so two senders can never collide.
// A ref that was already used moves nothing and reports applied=false.
// Both user rows are locked in id order first, so two people tipping each other at the same
// moment can not deadlock.
func (s *store) transferCredits(ctx context.Context, from, to, amount int64, ref, kindOut, kindIn, noteOut, noteIn string) (fromBal, toBal int64, applied bool, err error) {
	if from == to || amount <= 0 {
		return 0, 0, false, errors.New("bad transfer")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()

	lo, hi := pair(from, to)
	if _, err = tx.ExecContext(ctx, `SELECT id FROM users WHERE id IN ($1, $2) ORDER BY id FOR UPDATE`, lo, hi); err != nil {
		return
	}
	// Trusted facts, read after the rows are locked: a deleted account can not be paid,
	// and an ordinary account can not pay staff.
	ff, err := factsTx(ctx, tx, from)
	if err != nil {
		return
	}
	tf, err := factsTx(ctx, tx, to)
	if err != nil {
		return
	}
	if ff.Deleted || ff.Banned || tf.Deleted {
		return 0, 0, false, errGone
	}
	if staffShielded(ff.Role, tf.Role) {
		return 0, 0, false, errStaffTarget
	}
	if ff.Role == "admin" || tf.Role == "admin" {
		return 0, 0, false, errAdminWallet
	}
	var blocks int
	if err = tx.QueryRowContext(ctx,
		`SELECT count(*) FROM blocks WHERE (blocker = $1 AND blocked = $2) OR (blocker = $2 AND blocked = $1)`, from, to).Scan(&blocks); err != nil {
		return
	}
	if blocks > 0 {
		return 0, 0, false, errBlocked
	}
	fromBal, applied, err = applyCreditTx(ctx, tx, from, kindOut, -amount, ref, noteOut)
	if err != nil {
		return 0, 0, false, err
	}
	if !applied {
		return fromBal, 0, false, nil // the same transfer was already made
	}
	toBal, _, err = applyCreditTx(ctx, tx, to, kindIn, amount, fmt.Sprintf("%s:from:%d", ref, from), noteIn)
	if err != nil {
		return 0, 0, false, err
	}
	return fromBal, toBal, true, tx.Commit()
}

// ---- hub ----

// tip handles {"t":"tip","i":42,"amt":5,"c":"<key>"}: tip the author of message 42 in the room I am in.
func (h *hub) tip(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	name, ok := tipName(in.Amt)
	if !ok {
		cl.enqueue(errFrame("bad_amount"))
		return
	}
	if !tipKeyRE.MatchString(in.C) {
		cl.enqueue(errFrame("bad_request"))
		return
	}
	now := time.Now()
	h.mu.Lock()
	rm := cl.room
	if rm == nil {
		h.mu.Unlock()
		cl.enqueue(errFrame("no_room"))
		return
	}
	m := findMsg(rm, in.I)
	var author string
	if m != nil {
		author = m.Handle
	}
	tipCard := m != nil && m.TipAmt > 0
	self := m != nil && strings.EqualFold(author, ou.handle)
	allowed := m != nil && !tipCard && !self && takeToken(ou, now)
	h.mu.Unlock()
	switch {
	case m == nil:
		cl.enqueue(errFrame("msg_gone"))
		return
	case tipCard:
		cl.enqueue(errFrame("forbidden"))
		return
	case self:
		cl.enqueue(errFrame("self_tip"))
		return
	case !allowed:
		cl.enqueue(errFrame("slow_down"))
		return
	}

	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	target, ok := h.lookupUser(dctx, cl, author)
	if !ok {
		return
	}
	if target.ID == ou.id {
		cl.enqueue(errFrame("self_tip"))
		return
	}
	h.mu.Lock()
	myRole := ou.role
	h.mu.Unlock()
	if staffShielded(myRole, target.Role) {
		cl.enqueue(errFrame("staff_shield"))
		return
	}
	if myRole == "admin" || target.Role == "admin" {
		cl.enqueue(errFrame("admin_no_wallet"))
		return
	}
	fromBal, toBal, applied, err := h.st.transferCredits(dctx, ou.id, target.ID, int64(in.Amt), "tip:"+in.C, "tip_sent", "tip_received",
		fmt.Sprintf("%s for @%s", name, target.Handle), fmt.Sprintf("%s from @%s", name, ou.handle))
	switch {
	case errors.Is(err, errBlocked):
		cl.enqueue(errFrame("blocked"))
		return
	case errors.Is(err, errStaffTarget):
		cl.enqueue(errFrame("staff_shield"))
		return
	case errors.Is(err, errAdminWallet):
		cl.enqueue(errFrame("admin_no_wallet"))
		return
	case errors.Is(err, errGone):
		cl.enqueue(errFrame("no_such_user"))
		return
	case errors.Is(err, errNoFunds):
		cl.enqueue(errFrame("no_credits"))
		return
	case err != nil:
		log.Printf("tip failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if !applied {
		// A repeat of a tip that already went through (a retry or a double tap): nothing moves, nothing is shown twice.
		cl.enqueue(frame(map[string]any{"t": "tipok", "a": in.Amt, "n": name, "h": target.Handle, "dup": true}))
		return
	}

	h.mu.Lock()
	if h.rooms[rm.slug] == rm { // the room may have closed while the payment went through
		h.nextID++
		card := chatMsg{ID: h.nextID, Handle: ou.handle, Body: name, TS: now.UnixMilli(), Tier: ou.tier, TipTo: target.Handle, TipAmt: in.Amt, Av: ou.look()}
		if ou.role != "user" {
			card.Role = ou.role
		}
		if len(rm.buf) == bufferSize {
			copy(rm.buf, rm.buf[1:])
			rm.buf = rm.buf[:bufferSize-1]
		}
		rm.buf = append(rm.buf, card)
		rm.lastActive, rm.dirty = now, true
		b := frame(msgFrame{T: "m", chatMsg: card})
		for c := range rm.clients {
			c.enqueue(b)
		}
		select {
		case h.msgQ <- storedMsg{ID: card.ID, UserID: ou.id, Handle: card.Handle, Tier: card.Tier, Body: card.Body, TS: card.TS,
			Room: rm.slug, Role: card.Role, TipTo: card.TipTo, TipAmt: card.TipAmt, Avatar: card.Av}:
		default:
			h.dropped++
		}
	}
	h.mu.Unlock()

	h.setCredits(ou, fromBal)
	h.mu.Lock()
	tou := h.users[target.ID]
	h.mu.Unlock()
	if tou != nil {
		h.setCredits(tou, toBal)
	}
	h.sendToUser(target.ID, frame(map[string]any{"t": "tipgot", "f": ou.handle, "n": name, "a": in.Amt}))
	h.notify(noteIn{To: target.ID, ActorID: ou.id, Mode: "event", Ref: "tip:" + in.C,
		Note: note{Kind: "tip", Actor: ou.handle, Room: rm.slug, RoomName: rm.name, Body: name, Amount: in.Amt}})
	cl.enqueue(frame(map[string]any{"t": "tipok", "a": in.Amt, "n": name, "h": target.Handle}))
}
