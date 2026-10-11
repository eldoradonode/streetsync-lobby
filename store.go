package main

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"
)

//go:embed schema.sql
var schemaSQL string

var errNotFound = errors.New("not found")

type userRow struct {
	ID         int64
	Handle     string
	PassHash   sql.NullString
	Points     int64
	StreakDays int
	StreakDay  int
}

type storedMsg struct {
	ID     int64
	UserID int64
	Handle string
	Tier   int
	Body   string
	TS     int64 // unix milliseconds
}

type userSave struct {
	ID         int64
	Points     int64
	StreakDays int
	StreakDay  int
}

type store struct {
	db          *sql.DB
	sessionDays int
}

func openStore(ctx context.Context, url string, sessionDays int) (*store, error) {
	db, err := sql.Open("postgres", url)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)

	var lastErr error
	for i := 0; i < 5; i++ {
		pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		lastErr = db.PingContext(pctx)
		cancel()
		if lastErr == nil {
			break
		}
		log.Printf("database not reachable yet (%v), retrying", lastErr)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(1<<i) * time.Second):
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	if _, err := db.ExecContext(ctx, schemaSQL); err != nil {
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	return &store{db: db, sessionDays: sessionDays}, nil
}

func (s *store) close() { _ = s.db.Close() }

const userCols = `id, handle, pass_hash, points, streak_days, streak_day`

func scanUser(r interface{ Scan(...any) error }) (userRow, error) {
	var u userRow
	err := r.Scan(&u.ID, &u.Handle, &u.PassHash, &u.Points, &u.StreakDays, &u.StreakDay)
	if errors.Is(err, sql.ErrNoRows) {
		return u, errNotFound
	}
	return u, err
}

// createGuest reserves a handle. created is false when the handle already exists.
func (s *store) createGuest(ctx context.Context, handle, key string) (userRow, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`INSERT INTO users (handle, handle_key) VALUES ($1, $2)
		 ON CONFLICT (handle_key) DO NOTHING
		 RETURNING `+userCols, handle, key)
	u, err := scanUser(row)
	if errors.Is(err, errNotFound) {
		return userRow{}, false, nil
	}
	if err != nil {
		return userRow{}, false, err
	}
	return u, true, nil
}

func (s *store) userByKey(ctx context.Context, key string) (userRow, error) {
	return scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE handle_key = $1`, key))
}

func (s *store) newSession(ctx context.Context, userID int64) (string, error) {
	token := newToken()
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (token_hash, user_id) VALUES ($1, $2)`, tokenHash(token), userID)
	if err != nil {
		return "", err
	}
	return token, nil
}

// userBySession finds the user for a token. Sessions expire after sessionDays without use.
func (s *store) userBySession(ctx context.Context, token string) (userRow, error) {
	th := tokenHash(token)
	row := s.db.QueryRowContext(ctx,
		`SELECT u.id, u.handle, u.pass_hash, u.points, u.streak_days, u.streak_day
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.last_used > now() - make_interval(days => $2::int)`, th, s.sessionDays)
	u, err := scanUser(row)
	if err != nil {
		return u, err
	}
	// Keep the session and the handle alive. Failures here are not worth failing a login over.
	_, _ = s.db.ExecContext(ctx, `UPDATE sessions SET last_used = now() WHERE token_hash = $1 AND last_used < now() - interval '1 hour'`, th)
	_, _ = s.db.ExecContext(ctx, `UPDATE users SET last_seen = now() WHERE id = $1`, u.ID)
	return u, nil
}

func (s *store) touchUser(ctx context.Context, id int64) {
	_, _ = s.db.ExecContext(ctx, `UPDATE users SET last_seen = now() WHERE id = $1`, id)
}

// setPassword only works while the account has no password yet.
func (s *store) setPassword(ctx context.Context, id int64, hash string) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET pass_hash = $2 WHERE id = $1 AND pass_hash IS NULL`, id, hash)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

func (s *store) recentMessages(ctx context.Context, room string, n int) ([]chatMsg, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, handle, tier, body, (extract(epoch from created_at) * 1000)::bigint
		 FROM messages WHERE room = $1 ORDER BY id DESC LIMIT $2`, room, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []chatMsg
	for rows.Next() {
		var m chatMsg
		if err := rows.Scan(&m.ID, &m.Handle, &m.Tier, &m.Body, &m.TS); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (s *store) maxMessageID(ctx context.Context) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM messages`).Scan(&id)
	return id, err
}

func (s *store) insertMessages(ctx context.Context, room string, msgs []storedMsg) error {
	n := len(msgs)
	ids, uids, tiers, ts := make([]int64, n), make([]int64, n), make([]int64, n), make([]int64, n)
	handles, bodies := make([]string, n), make([]string, n)
	for i, m := range msgs {
		ids[i], uids[i], tiers[i], ts[i] = m.ID, m.UserID, int64(m.Tier), m.TS
		handles[i], bodies[i] = m.Handle, m.Body
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO messages (id, room, user_id, handle, tier, body, created_at)
		 SELECT t.id, $1::text, t.uid, t.handle, t.tier, t.body, to_timestamp(t.ms / 1000.0)
		 FROM unnest($2::bigint[], $3::bigint[], $4::text[], $5::smallint[], $6::text[], $7::bigint[])
		      AS t(id, uid, handle, tier, body, ms)
		 ON CONFLICT (id) DO NOTHING`,
		room, pq.Array(ids), pq.Array(uids), pq.Array(handles), pq.Array(tiers), pq.Array(bodies), pq.Array(ts))
	return err
}

func (s *store) saveUsers(ctx context.Context, batch []userSave) error {
	n := len(batch)
	ids, points := make([]int64, n), make([]int64, n)
	sd, sday := make([]int64, n), make([]int64, n)
	for i, u := range batch {
		ids[i], points[i], sd[i], sday[i] = u.ID, u.Points, int64(u.StreakDays), int64(u.StreakDay)
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE users u SET points = d.points, streak_days = d.sd, streak_day = d.sday, last_seen = now()
		 FROM unnest($1::bigint[], $2::bigint[], $3::int[], $4::int[]) AS d(id, points, sd, sday)
		 WHERE u.id = d.id`,
		pq.Array(ids), pq.Array(points), pq.Array(sd), pq.Array(sday))
	return err
}

// prune keeps the free database small: old messages, idle guests and old sessions go.
func (s *store) prune(ctx context.Context, historyDays, guestDays int) (msgs, guests, sessions int64, err error) {
	run := func(q string, days int) (int64, error) {
		res, err := s.db.ExecContext(ctx, q, days)
		if err != nil {
			return 0, err
		}
		return res.RowsAffected()
	}
	if msgs, err = run(`DELETE FROM messages WHERE created_at < now() - make_interval(days => $1::int)`, historyDays); err != nil {
		return
	}
	if guests, err = run(`DELETE FROM users WHERE pass_hash IS NULL AND last_seen < now() - make_interval(days => $1::int)`, guestDays); err != nil {
		return
	}
	sessions, err = run(`DELETE FROM sessions WHERE last_used < now() - make_interval(days => $1::int)`, s.sessionDays)
	return
}
