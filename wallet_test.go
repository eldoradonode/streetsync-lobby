package main

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestStreakRewards(t *testing.T) {
	r := streakRewards(20000, 1)
	if len(r) != 1 || r[0].Kind != "daily" || r[0].Amount != dailyCredits || r[0].Ref != "daily:20000" {
		t.Fatalf("day 1: %+v", r)
	}
	r = streakRewards(20002, 3)
	if len(r) != 2 || r[1].Kind != "streak" || r[1].Amount != 5 || r[1].Ref != "streak3:20002" {
		t.Fatalf("day 3: %+v", r)
	}
	for _, n := range []int{2, 4, 5, 6, 8, 29, 31} {
		if got := streakRewards(1, n); len(got) != 1 {
			t.Errorf("streak %d must pay only the daily bonus, got %+v", n, got)
		}
	}
	for n, amt := range streakMilestones {
		got := streakRewards(1, n)
		if len(got) != 2 || got[1].Amount != amt {
			t.Errorf("streak %d: %+v", n, got)
		}
	}
}

func TestLedgerBasics(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u, _, err := st.createGuest(ctx, "t_wal1", "t_wal1")
	if err != nil {
		t.Fatal(err)
	}

	bal, applied, err := st.applyCredit(ctx, u.ID, "grant", 50, "r1", "test")
	if err != nil || !applied || bal != 50 {
		t.Fatalf("first credit: bal=%d applied=%v err=%v", bal, applied, err)
	}
	// The same ref again does nothing and reports the current balance.
	bal, applied, err = st.applyCredit(ctx, u.ID, "grant", 50, "r1", "test")
	if err != nil || applied || bal != 50 {
		t.Fatalf("repeat: bal=%d applied=%v err=%v", bal, applied, err)
	}
	// Spending.
	bal, applied, err = st.applyCredit(ctx, u.ID, "spend", -20, "s1", "spend")
	if err != nil || !applied || bal != 30 {
		t.Fatalf("spend: bal=%d applied=%v err=%v", bal, applied, err)
	}
	// Overspending fails, changes nothing, and leaves no ledger row behind.
	_, applied, err = st.applyCredit(ctx, u.ID, "spend", -31, "s2", "too much")
	if !errors.Is(err, errNoFunds) || applied {
		t.Fatalf("overspend: applied=%v err=%v", applied, err)
	}
	if got, _ := st.creditBalance(ctx, u.ID); got != 30 {
		t.Errorf("balance after failed spend = %d, want 30", got)
	}
	var n int
	_ = st.db.QueryRow(`SELECT count(*) FROM credit_ledger WHERE user_id=$1 AND ref='s2'`, u.ID).Scan(&n)
	if n != 0 {
		t.Error("a failed spend must not leave a ledger row")
	}
	// Spending exactly everything is fine.
	if bal, _, err := st.applyCredit(ctx, u.ID, "spend", -30, "s3", "all"); err != nil || bal != 0 {
		t.Fatalf("spend all: bal=%d err=%v", bal, err)
	}
	// Bad entries are refused.
	if _, _, err := st.applyCredit(ctx, u.ID, "grant", 0, "z", ""); err == nil {
		t.Error("zero amount must be refused")
	}
	if _, _, err := st.applyCredit(ctx, u.ID, "grant", 5, "", ""); err == nil {
		t.Error("empty ref must be refused")
	}

	rows, err := st.ledgerFor(ctx, u.ID, 10)
	if err != nil || len(rows) != 3 {
		t.Fatalf("ledger rows: %+v err=%v", rows, err)
	}
	if rows[0].Kind != "spend" || rows[0].Amount != -30 || rows[0].Balance != 0 {
		t.Errorf("newest row: %+v", rows[0])
	}
	if rows[2].Kind != "grant" || rows[2].Amount != 50 || rows[2].Balance != 50 {
		t.Errorf("oldest row: %+v", rows[2])
	}
	// The balance always equals the sum of the ledger.
	var sum int64
	_ = st.db.QueryRow(`SELECT coalesce(sum(amount),0) FROM credit_ledger WHERE user_id=$1`, u.ID).Scan(&sum)
	if got, _ := st.creditBalance(ctx, u.ID); got != sum {
		t.Errorf("balance %d does not match ledger sum %d", got, sum)
	}
}

func TestWelcomeOnlyOnce(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u, _, _ := st.createGuest(ctx, "t_wal2", "t_wal2")

	var wg sync.WaitGroup
	var mu sync.Mutex
	paid := 0
	for i := 0; i < 8; i++ { // eight tabs connect at the same moment
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, applied, err := st.grantWelcome(ctx, u.ID)
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
	if paid != 1 {
		t.Fatalf("welcome paid %d times, want 1", paid)
	}
	if got, _ := st.creditBalance(ctx, u.ID); got != welcomeCredits {
		t.Errorf("balance = %d, want %d", got, welcomeCredits)
	}
	again, _ := st.userByKey(ctx, "t_wal2")
	if !again.Welcomed || again.Credits != welcomeCredits {
		t.Errorf("user row after welcome: %+v", again)
	}
}

func TestConcurrentSpendNeverGoesNegative(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u, _, _ := st.createGuest(ctx, "t_wal3", "t_wal3")
	if _, _, err := st.applyCredit(ctx, u.ID, "grant", 10, "g", ""); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	ok := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, applied, err := st.applyCredit(ctx, u.ID, "spend", -1, "c"+string(rune('a'+i)), "")
			if err == nil && applied {
				mu.Lock()
				ok++
				mu.Unlock()
			} else if err != nil && !errors.Is(err, errNoFunds) {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	if ok != 10 {
		t.Errorf("%d spends succeeded, want exactly 10", ok)
	}
	if got, _ := st.creditBalance(ctx, u.ID); got != 0 {
		t.Errorf("balance = %d, want 0", got)
	}
}
