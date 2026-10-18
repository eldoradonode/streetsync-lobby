package main

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestTipOptions(t *testing.T) {
	want := map[int]string{1: "Suya", 5: "Burger", 10: "Pizza", 25: "Cake", 50: "Feast"}
	if len(tipOptions) != len(want) {
		t.Fatalf("tip options: %v", tipOptions)
	}
	for amt, name := range want {
		if got, ok := tipName(amt); !ok || got != name {
			t.Errorf("tipName(%d) = %q, %v; want %q", amt, got, ok, name)
		}
	}
	for _, bad := range []int{-1, 0, 2, 7, 100, 1000} {
		if _, ok := tipName(bad); ok {
			t.Errorf("%d must not be a tip amount", bad)
		}
	}
	for _, k := range []string{"abcdefgh", "a1b2c3d4-e5f6-4789-abcd-0123456789ab", "KEY_with-dash_12"} {
		if !tipKeyRE.MatchString(k) {
			t.Errorf("key %q must be valid", k)
		}
	}
	for _, k := range []string{"", "short", "has space here", "semi;colon;key1", "<script>alert(1)</script>", "x"} {
		if tipKeyRE.MatchString(k) {
			t.Errorf("key %q must be refused", k)
		}
	}
}

func transfer(st *store, from, to userRow, amt int64, key string) (int64, int64, bool, error) {
	return st.transferCredits(context.Background(), from.ID, to.ID, amt, "tip:"+key, "tip_sent", "tip_received", "Burger for @"+to.Handle, "Burger from @"+from.Handle)
}

func TestTransferMovesBothSidesOnce(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_tpa", "t_tpb")
	a, b := u[0], u[1]
	st.applyCredit(ctx, a.ID, "grant", 20, "g", "")

	fb, tb, applied, err := transfer(st, a, b, 5, "key-00001")
	if err != nil || !applied || fb != 15 || tb != 5 {
		t.Fatalf("tip: from=%d to=%d applied=%v err=%v", fb, tb, applied, err)
	}
	if credits(t, st, a.ID) != 15 || credits(t, st, b.ID) != 5 {
		t.Errorf("balances %d / %d", credits(t, st, a.ID), credits(t, st, b.ID))
	}
	// The same key again (a retry or a double tap) moves nothing.
	fb, _, applied, err = transfer(st, a, b, 5, "key-00001")
	if err != nil || applied || fb != 15 {
		t.Fatalf("repeat: from=%d applied=%v err=%v", fb, applied, err)
	}
	if credits(t, st, a.ID) != 15 || credits(t, st, b.ID) != 5 {
		t.Errorf("a repeat changed balances to %d / %d", credits(t, st, a.ID), credits(t, st, b.ID))
	}
	// Both sides are recorded, with their own kinds and notes, never as room fees.
	ra, _ := st.ledgerFor(ctx, a.ID, 5)
	rb, _ := st.ledgerFor(ctx, b.ID, 5)
	if ra[0].Kind != "tip_sent" || ra[0].Amount != -5 || ra[0].Balance != 15 || ra[0].Note != "Burger for @"+b.Handle {
		t.Errorf("sender entry: %+v", ra[0])
	}
	if len(rb) != 1 || rb[0].Kind != "tip_received" || rb[0].Amount != 5 || rb[0].Balance != 5 || rb[0].Note != "Burger from @"+a.Handle {
		t.Errorf("recipient entry: %+v", rb)
	}
	var fees int
	st.db.QueryRow(`SELECT count(*) FROM credit_ledger WHERE kind IN ('room_entry','room_income','room_create') AND user_id IN ($1,$2)`, a.ID, b.ID).Scan(&fees)
	if fees != 0 {
		t.Error("a tip must never be recorded as a room fee")
	}
}

func TestTransferRefusals(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_tra", "t_trb")
	a, b := u[0], u[1]
	st.applyCredit(ctx, a.ID, "grant", 4, "g", "")

	// Not enough credits: nothing moves and nothing is recorded, and the key stays usable.
	if _, _, applied, err := transfer(st, a, b, 5, "key-poor01"); !errors.Is(err, errNoFunds) || applied {
		t.Fatalf("overspend: applied=%v err=%v", applied, err)
	}
	if credits(t, st, a.ID) != 4 || credits(t, st, b.ID) != 0 {
		t.Error("a refused tip changed balances")
	}
	var n int
	st.db.QueryRow(`SELECT count(*) FROM credit_ledger WHERE kind LIKE 'tip%' AND user_id IN ($1, $2)`, a.ID, b.ID).Scan(&n)
	if n != 0 {
		t.Errorf("a refused tip left %d ledger rows", n)
	}
	if _, _, applied, err := transfer(st, a, b, 1, "key-poor01"); err != nil || !applied {
		t.Fatalf("the key of a refused tip can be used again: applied=%v err=%v", applied, err)
	}
	// Tipping yourself or a zero amount is refused before any money moves.
	if _, _, _, err := transfer(st, a, a, 1, "key-self01"); err == nil {
		t.Error("a tip to yourself must be refused")
	}
	if _, _, _, err := transfer(st, a, b, 0, "key-zero01"); err == nil {
		t.Error("a zero tip must be refused")
	}
	// Blocked either way.
	st.blockUser(ctx, b.ID, a.ID)
	if _, _, applied, err := transfer(st, a, b, 1, "key-blk001"); !errors.Is(err, errBlocked) || applied {
		t.Errorf("tip to someone who blocked me: %v", err)
	}
	st.unblockUser(ctx, b.ID, a.ID)
	st.blockUser(ctx, a.ID, b.ID)
	if _, _, _, err := transfer(st, a, b, 1, "key-blk002"); !errors.Is(err, errBlocked) {
		t.Errorf("tip to someone I blocked: %v", err)
	}
}

func TestTwoSendersSameKeyNeverLoseMoney(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_tsa", "t_tsb", "t_tsc")
	for _, x := range u[:2] {
		st.applyCredit(ctx, x.ID, "grant", 10, "g", "")
	}
	// Two different people choose the same key for the same recipient: both tips must go through.
	if _, _, applied, err := transfer(st, u[0], u[2], 5, "same-key-1"); err != nil || !applied {
		t.Fatal(err, applied)
	}
	if _, _, applied, err := transfer(st, u[1], u[2], 5, "same-key-1"); err != nil || !applied {
		t.Fatalf("the second sender was charged without the recipient being paid? applied=%v err=%v", applied, err)
	}
	if credits(t, st, u[2].ID) != 10 || credits(t, st, u[0].ID) != 5 || credits(t, st, u[1].ID) != 5 {
		t.Errorf("balances %d / %d / %d", credits(t, st, u[0].ID), credits(t, st, u[1].ID), credits(t, st, u[2].ID))
	}
}

func TestTipsAtTheSameMomentAreSafe(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_tca", "t_tcb")
	a, b := u[0], u[1]
	st.applyCredit(ctx, a.ID, "grant", 100, "g", "")
	st.applyCredit(ctx, b.ID, "grant", 100, "g", "")

	// Forty double taps of one tip: exactly one goes through.
	var wg sync.WaitGroup
	var mu sync.Mutex
	paid := 0
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, applied, err := transfer(st, a, b, 10, "double-tap-1")
			if err != nil {
				t.Error(err)
			}
			if applied {
				mu.Lock()
				paid++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if paid != 1 || credits(t, st, a.ID) != 90 || credits(t, st, b.ID) != 110 {
		t.Fatalf("double taps: paid=%d, balances %d / %d", paid, credits(t, st, a.ID), credits(t, st, b.ID))
	}

	// People tipping each other at the same time: no deadlock, and credits are neither made nor lost.
	for i := 0; i < 30; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			if _, _, _, err := transfer(st, a, b, 1, "ab-key-"+string(rune('a'+i%26))+string(rune('a'+i/26))+"xx"); err != nil && !errors.Is(err, errNoFunds) {
				t.Error(err)
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			if _, _, _, err := transfer(st, b, a, 1, "ba-key-"+string(rune('a'+i%26))+string(rune('a'+i/26))+"xx"); err != nil && !errors.Is(err, errNoFunds) {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if credits(t, st, a.ID)+credits(t, st, b.ID) != 200 {
		t.Errorf("credits were made or lost: %d + %d", credits(t, st, a.ID), credits(t, st, b.ID))
	}
	for _, x := range []userRow{a, b} {
		var sum int64
		st.db.QueryRow(`SELECT coalesce(sum(amount),0) FROM credit_ledger WHERE user_id=$1`, x.ID).Scan(&sum)
		if sum != credits(t, st, x.ID) {
			t.Errorf("%s: ledger %d, balance %d", x.Handle, sum, credits(t, st, x.ID))
		}
	}
}

func TestTipCardsAreStoredAndLoaded(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	err := st.insertMessages(ctx, []storedMsg{
		{ID: 880001, UserID: 0, Handle: "t_card", Body: "Burger", TS: 1_700_000_000_000, Room: "testroom", TipTo: "t_bob", TipAmt: 5},
		{ID: 880002, UserID: 0, Handle: "t_card", Body: "plain", TS: 1_700_000_000_001, Room: "testroom"},
	})
	if err != nil {
		t.Fatal(err)
	}
	msgs, err := st.recentMessages(ctx, "testroom", 10)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("messages: %+v %v", msgs, err)
	}
	if msgs[0].TipTo != "t_bob" || msgs[0].TipAmt != 5 || msgs[0].Body != "Burger" {
		t.Errorf("tip card: %+v", msgs[0])
	}
	if msgs[1].TipTo != "" || msgs[1].TipAmt != 0 {
		t.Errorf("a plain message must carry no tip: %+v", msgs[1])
	}
}
