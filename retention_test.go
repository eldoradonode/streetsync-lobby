package main

import (
	"context"
	"testing"
	"time"
)

// Retention policy: direct messages and room history are kept seven days. These tests run against a real Postgres.

func insertDMAged(t *testing.T, st *store, id, from, to int64, body string, age time.Duration) {
	t.Helper()
	if _, err := st.db.Exec(`INSERT INTO dm_messages (id, sender_id, recipient_id, body, created_at) VALUES ($1,$2,$3,$4, now() - ($5 * interval '1 second'))`,
		id, from, to, body, int64(age.Seconds())); err != nil {
		t.Fatal(err)
	}
}

func insertRoomMsgAged(t *testing.T, st *store, id int64, room, body string, age time.Duration) {
	t.Helper()
	if _, err := st.db.Exec(`INSERT INTO messages (id, room, handle, body, created_at) VALUES ($1,$2,'t_x',$3, now() - ($4 * interval '1 second'))`,
		id, room, body, int64(age.Seconds())); err != nil {
		t.Fatal(err)
	}
}

func countWhere(t *testing.T, st *store, q string, args ...any) int {
	t.Helper()
	var n int
	if err := st.db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRetentionDefaultsAreSevenDays(t *testing.T) {
	if defaultDMDays != 7 || defaultHistoryDays != 7 {
		t.Fatalf("defaults must be 7 and 7, got %d and %d", defaultDMDays, defaultHistoryDays)
	}
	h := newHub(nil, newAuthSvc(1000), nil, time.Second, defaultHistoryDays, 14)
	if h.dmDays != 7 {
		t.Errorf("a new hub must keep DMs 7 days, got %d", h.dmDays)
	}
	t.Setenv("DM_DAYS", "")
	if got := envInt("DM_DAYS", defaultDMDays); got != 7 {
		t.Errorf("unset DM_DAYS: %d", got)
	}
	t.Setenv("DM_DAYS", "3")
	if got := envInt("DM_DAYS", defaultDMDays); got != 3 {
		t.Errorf("DM_DAYS=3: %d", got)
	}
	t.Setenv("DM_DAYS", "banana")
	if got := envInt("DM_DAYS", defaultDMDays); got != 7 {
		t.Errorf("invalid DM_DAYS must fall back to 7, got %d", got)
	}
	t.Setenv("HISTORY_DAYS", "2")
	if got := envInt("HISTORY_DAYS", defaultHistoryDays); got != 2 {
		t.Errorf("HISTORY_DAYS=2: %d", got)
	}
}

func TestRetentionKeepsInsideWindowDeletesOutside(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_ret_a", "t_ret_b")
	a, b := u[0], u[1]
	day := 24 * time.Hour
	insertDMAged(t, st, 9200001, a.ID, b.ID, "t_old_8d", 8*day)
	insertDMAged(t, st, 9200002, b.ID, a.ID, "t_old_30d", 30*day)
	insertDMAged(t, st, 9200003, a.ID, b.ID, "t_edge_6d23h", 7*day-time.Hour)
	insertDMAged(t, st, 9200004, a.ID, b.ID, "t_new_1d", day)
	insertDMAged(t, st, 9200005, b.ID, a.ID, "t_new_now", 0)
	insertRoomMsgAged(t, st, 9200101, "t_retroom", "t_room_8d", 8*day)
	insertRoomMsgAged(t, st, 9200102, "t_retroom", "t_room_6d", 6*day)
	insertRoomMsgAged(t, st, 9200103, "t_retroom", "t_room_now", 0)

	// Rows that must never be touched by cleanup.
	st.applyCredit(ctx, a.ID, "grant", 40, "t_ret_grant", "")
	if _, err := st.db.Exec(`UPDATE credit_ledger SET created_at = now() - interval '400 days' WHERE user_id = $1`, a.ID); err != nil {
		t.Fatal(err)
	}
	ledgerBefore := countWhere(t, st, `SELECT count(*) FROM credit_ledger WHERE user_id = $1`, a.ID)
	st.logMod(ctx, "t_ret_a", "test", "x", "old")
	if _, err := st.db.Exec(`UPDATE mod_log SET created_at = now() - interval '400 days' WHERE actor = 't_ret_a'`); err != nil {
		t.Fatal(err)
	}

	d, err := st.pruneDMs(ctx, defaultDMDays)
	if err != nil || d != 2 {
		t.Fatalf("pruneDMs removed %d (err %v), want 2", d, err)
	}
	m, err := st.pruneRoomHistory(ctx, defaultHistoryDays)
	if err != nil || m < 1 {
		t.Fatalf("pruneRoomHistory removed %d (err %v)", m, err)
	}
	if n := countWhere(t, st, `SELECT count(*) FROM dm_messages WHERE id IN (9200001, 9200002)`); n != 0 {
		t.Errorf("expired DMs remain: %d", n)
	}
	if n := countWhere(t, st, `SELECT count(*) FROM dm_messages WHERE id IN (9200003, 9200004, 9200005)`); n != 3 {
		t.Errorf("DMs inside the 7 day window must remain, got %d of 3", n)
	}
	if n := countWhere(t, st, `SELECT count(*) FROM messages WHERE id = 9200101`); n != 0 {
		t.Error("expired room message remains")
	}
	if n := countWhere(t, st, `SELECT count(*) FROM messages WHERE id IN (9200102, 9200103)`); n != 2 {
		t.Errorf("room messages inside the window must remain, got %d of 2", n)
	}
	if n := countWhere(t, st, `SELECT count(*) FROM credit_ledger WHERE user_id = $1`, a.ID); n != ledgerBefore {
		t.Errorf("cleanup must not touch the ledger: %d -> %d", ledgerBefore, n)
	}
	if n := countWhere(t, st, `SELECT count(*) FROM mod_log WHERE actor = 't_ret_a'`); n != 1 {
		t.Error("cleanup must not touch the moderation log")
	}
	// Repeat is a no-op.
	if d2, err := st.pruneDMs(ctx, defaultDMDays); err != nil || d2 != 0 {
		t.Errorf("second run must remove nothing: %d %v", d2, err)
	}
}

func TestRetentionBatchesAndRetries(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	u := mkUsers(t, st, "t_bat_a", "t_bat_b")
	st.pruneBatch = 10
	for i := int64(0); i < 95; i++ {
		insertDMAged(t, st, 9300000+i, u[0].ID, u[1].ID, "t_batch", 9*24*time.Hour)
	}
	insertDMAged(t, st, 9300500, u[0].ID, u[1].ID, "t_batch_keep", time.Hour)
	// A cancelled run stops cleanly and leaves the rest for the next run.
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := st.pruneDMs(cctx, 7); err == nil {
		t.Error("a cancelled run should report the cancellation")
	}
	n, err := st.pruneDMs(ctx, 7)
	if err != nil || n != 95 {
		t.Fatalf("batched prune removed %d (err %v), want 95", n, err)
	}
	if left := countWhere(t, st, `SELECT count(*) FROM dm_messages WHERE sender_id = $1`, u[0].ID); left != 1 {
		t.Errorf("exactly the recent message should remain, %d rows left", left)
	}
	if n, _ := st.pruneDMs(ctx, 7); n != 0 {
		t.Errorf("retry removed %d", n)
	}
}

func TestRetentionRunsAutomaticallyAndTrimsBuffers(t *testing.T) {
	e := newTestEnv(t)
	st := e.st
	u := mkUsers(t, st, "t_auto_a", "t_auto_b")
	insertDMAged(t, st, 9400001, u[0].ID, u[1].ID, "t_auto_old", 10*24*time.Hour)
	insertDMAged(t, st, 9400002, u[0].ID, u[1].ID, "t_auto_new", time.Hour)
	// A room buffer holding an expired message and a fresh one.
	e.h.mu.Lock()
	rm := e.h.roomList[0]
	rm.buf = append(rm.buf, chatMsg{ID: 9400101, Handle: "t_x", Body: "t_buf_old", TS: time.Now().Add(-8 * 24 * time.Hour).UnixMilli()},
		chatMsg{ID: 9400102, Handle: "t_x", Body: "t_buf_new", TS: time.Now().UnixMilli()})
	e.h.mu.Unlock()
	e.h.dmDays = 7
	e.h.firstCleanup = 50 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go e.h.run(ctx)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if countWhere(t, st, `SELECT count(*) FROM dm_messages WHERE id = 9400001`) == 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if countWhere(t, st, `SELECT count(*) FROM dm_messages WHERE id = 9400001`) != 0 {
		t.Fatal("the scheduled cleanup did not delete the expired DM on its own")
	}
	if countWhere(t, st, `SELECT count(*) FROM dm_messages WHERE id = 9400002`) != 1 {
		t.Error("the scheduled cleanup deleted a DM inside the window")
	}
	e.h.mu.Lock()
	var oldLeft, newLeft bool
	for _, m := range e.h.roomList[0].buf {
		oldLeft = oldLeft || m.ID == 9400101
		newLeft = newLeft || m.ID == 9400102
	}
	e.h.mu.Unlock()
	if oldLeft || !newLeft {
		t.Errorf("buffer trim: expired kept=%v fresh kept=%v", oldLeft, newLeft)
	}
}

func TestRetentionOverrideShortensWindow(t *testing.T) {
	e := newTestEnv(t)
	u := mkUsers(t, e.st, "t_ovr_a", "t_ovr_b")
	insertDMAged(t, e.st, 9500001, u[0].ID, u[1].ID, "t_ovr_4d", 4*24*time.Hour)
	e.h.dmDays = 7
	e.h.cleanupOnce(context.Background())
	if countWhere(t, e.st, `SELECT count(*) FROM dm_messages WHERE id = 9500001`) != 1 {
		t.Fatal("a 4 day old DM must survive the default 7 day window")
	}
	e.h.dmDays = 3 // what DM_DAYS=3 does
	e.h.cleanupOnce(context.Background())
	if countWhere(t, e.st, `SELECT count(*) FROM dm_messages WHERE id = 9500001`) != 0 {
		t.Fatal("DM_DAYS=3 must delete a 4 day old DM")
	}
}

func TestPoolConfig(t *testing.T) {
	t.Setenv("DB_MAX_OPEN_CONNS", "")
	c := poolFromEnv()
	if c != defaultPool() {
		t.Errorf("defaults changed: %+v", c)
	}
	t.Setenv("DB_MAX_OPEN_CONNS", "8")
	t.Setenv("DB_MAX_IDLE_CONNS", "20") // more than open: clamped
	t.Setenv("DB_CONN_MAX_LIFETIME_MIN", "15")
	t.Setenv("DB_CONN_MAX_IDLE_MIN", "0")
	c = poolFromEnv()
	if c.MaxOpen != 8 || c.MaxIdle != 8 || c.MaxLifetime != 15*time.Minute || c.MaxIdleTime != 0 {
		t.Errorf("env values: %+v", c)
	}
	t.Setenv("DB_MAX_OPEN_CONNS", "0")
	t.Setenv("DB_MAX_IDLE_CONNS", "-3")
	t.Setenv("DB_CONN_MAX_LIFETIME_MIN", "abc")
	c = poolFromEnv()
	d := defaultPool()
	if c.MaxOpen != d.MaxOpen || c.MaxIdle != d.MaxIdle || c.MaxLifetime != d.MaxLifetime {
		t.Errorf("invalid values must fall back to defaults: %+v", c)
	}
	t.Setenv("DB_MAX_OPEN_CONNS", "100000")
	if poolFromEnv().MaxOpen != d.MaxOpen {
		t.Error("an absurd open limit must be refused")
	}
}
