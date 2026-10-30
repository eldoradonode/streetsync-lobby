package main

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Retention policy (agreed): direct messages and room history are kept for seven days.
// Both values can be changed with DM_DAYS and HISTORY_DAYS. Nothing else is deleted here:
// the credit ledger, reports, staff complaints and the moderation log are never touched by cleanup.
const (
	defaultDMDays      = 7
	defaultHistoryDays = 7
	defaultPruneBatch  = 1000 // rows per DELETE statement
	maxPruneBatches    = 200  // statements per table per run; the rest waits for the next hourly run
)

// deleteInBatches deletes the rows matching where, at most s.pruneBatch per statement, so a large backlog
// never becomes one long lock. It is safe to repeat and safe to run on two instances at once: each statement
// only takes rows nobody else holds (SKIP LOCKED) and a second run finds nothing left to delete.
// table, key and where are constants written in this file, never user input. $1... in where are args.
func (s *store) deleteInBatches(ctx context.Context, table, key, where string, args ...any) (int64, error) {
	batch := s.pruneBatch
	if batch < 1 {
		batch = defaultPruneBatch
	}
	q := fmt.Sprintf(`DELETE FROM %[1]s WHERE %[2]s IN (SELECT %[2]s FROM %[1]s WHERE %[3]s ORDER BY %[2]s LIMIT $%[4]d FOR UPDATE SKIP LOCKED)`,
		table, key, where, len(args)+1)
	all := append(append([]any{}, args...), batch)
	var total int64
	for i := 0; i < maxPruneBatches; i++ {
		if err := ctx.Err(); err != nil {
			return total, err
		}
		res, err := s.db.ExecContext(ctx, q, all...)
		if err != nil {
			return total, err
		}
		n, _ := res.RowsAffected()
		total += n
		if n < int64(batch) {
			break
		}
	}
	return total, nil
}

// pruneRoomHistory deletes room messages older than days.
func (s *store) pruneRoomHistory(ctx context.Context, days int) (int64, error) {
	return s.deleteInBatches(ctx, "messages", "id", `created_at < now() - make_interval(days => $1::int)`, days)
}

// pruneOldSessions deletes sessions that have not been used for sessionDays.
func (s *store) pruneOldSessions(ctx context.Context) (int64, error) {
	return s.deleteInBatches(ctx, "sessions", "token_hash", `last_used < now() - make_interval(days => $1::int)`, s.sessionDays)
}

// trimBuffers drops messages older than the retention window from the in-memory room buffers, so a room
// never serves history that the database has already deleted. It returns how many it dropped.
func (h *hub) trimBuffers(cutoff time.Time) int {
	ms := cutoff.UnixMilli()
	dropped := 0
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, rm := range h.roomList {
		i := 0
		for i < len(rm.buf) && rm.buf[i].TS < ms {
			i++
		}
		if i > 0 {
			rm.buf = append(rm.buf[:0:0], rm.buf[i:]...)
			dropped += i
		}
	}
	return dropped
}

// cleanupOnce runs every retention job. A failure in one job never stops the others, and every job is
// idempotent, so a retry after a crash or a timeout is harmless. Only counts are logged, never message text.
func (h *hub) cleanupOnce(ctx context.Context) {
	if n, err := h.st.pruneNotes(ctx); err != nil {
		log.Printf("notification cleanup failed: %v", err)
	} else if n > 0 {
		log.Printf("removed %d old notifications", n)
	}
	m, merr := h.st.pruneRoomHistory(ctx, h.historyDays)
	if merr != nil {
		log.Printf("room history cleanup failed: %v", merr)
	}
	b := h.trimBuffers(time.Now().Add(-time.Duration(h.historyDays) * 24 * time.Hour))
	d, derr := h.st.pruneDMs(ctx, h.dmDays)
	if derr != nil {
		log.Printf("direct message cleanup failed: %v", derr)
	}
	g, gerr := h.st.pruneGuests(ctx, h.guestDays)
	if gerr != nil {
		log.Printf("guest cleanup failed: %v", gerr)
	}
	s, serr := h.st.pruneOldSessions(ctx)
	if serr != nil {
		log.Printf("session cleanup failed: %v", serr)
	}
	h.pruneEngagement(ctx)
	h.maintainRooms(ctx)
	if m+int64(b)+d+g+s > 0 {
		log.Printf("cleanup removed messages=%d buffered=%d direct_messages=%d idle_guests=%d old_sessions=%d", m, b, d, g, s)
	}
}

// pruneEngagement removes reactions whose message is gone (older than an hour, so messages still waiting to be
// saved are safe) and polls that were closed more than historyDays ago. Pins are kept: they hold their own text.
func (h *hub) pruneEngagement(ctx context.Context) {
	if _, err := h.st.db.ExecContext(ctx, `DELETE FROM message_reactions r WHERE r.created_at < now() - interval '1 hour' AND NOT EXISTS (SELECT 1 FROM messages m WHERE m.id = r.msg_id)`); err != nil {
		log.Printf("reaction cleanup failed: %v", err)
	}
	if _, err := h.st.db.ExecContext(ctx, `DELETE FROM polls WHERE closed_at < now() - make_interval(days => $1::int)`, h.historyDays); err != nil {
		log.Printf("poll cleanup failed: %v", err)
	}
}
