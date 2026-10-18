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
	Role       string // user, mod or admin
	Credits    int64
	Welcomed   bool
	Avatar     string // the look picked at sign up, empty when none
	Bio        string // a short line about the person, empty when none
}

type storedMsg struct {
	ID          int64
	UserID      int64
	Handle      string
	Tier        int
	Body        string
	TS          int64 // unix milliseconds
	ReplyTo     int64 // 0 when the message is not a reply
	ReplyHandle string
	ReplySnip   string
	Room        string
	Role        string // mod or admin, empty for normal users
	TipTo       string // set on tip cards: who was tipped
	TipAmt      int    // set on tip cards: how many credits
	Avatar      string // the sender's look
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

const userCols = `id, handle, pass_hash, points, streak_days, streak_day, role, credits, welcomed, avatar, bio`

func scanUser(r interface{ Scan(...any) error }) (userRow, error) {
	var u userRow
	err := r.Scan(&u.ID, &u.Handle, &u.PassHash, &u.Points, &u.StreakDays, &u.StreakDay, &u.Role, &u.Credits, &u.Welcomed, &u.Avatar, &u.Bio)
	if errors.Is(err, sql.ErrNoRows) {
		return u, errNotFound
	}
	return u, err
}

// createGuest reserves a handle. created is false when the handle already exists.
func (s *store) createGuest(ctx context.Context, handle, key string) (userRow, bool, error) {
	return s.createGuestWithAvatar(ctx, handle, key, "")
}

// createGuestWithAvatar is createGuest with the look the person picked.
func (s *store) createGuestWithAvatar(ctx context.Context, handle, key, avatar string) (userRow, bool, error) {
	return s.createAccount(ctx, handle, key, avatar, "")
}

// createAccount reserves a handle and, when hash is not empty, stores the password hash in the same insert,
// so a handle never exists without its password. created is false when the handle already exists.
func (s *store) createAccount(ctx context.Context, handle, key, avatar, hash string) (userRow, bool, error) {
	row := s.db.QueryRowContext(ctx,
		`INSERT INTO users (handle, handle_key, avatar, pass_hash) VALUES ($1, $2, $3, $4)
		 ON CONFLICT (handle_key) DO NOTHING
		 RETURNING `+userCols, handle, key, avatar, sql.NullString{String: hash, Valid: hash != ""})
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
		`SELECT u.id, u.handle, u.pass_hash, u.points, u.streak_days, u.streak_day, u.role, u.credits, u.welcomed, u.avatar, u.bio
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

// deleteSession ends one login. Other logins of the same person stay.
func (s *store) deleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash)
	return err
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
		`SELECT id, handle, tier, body, (extract(epoch from created_at) * 1000)::bigint,
		        COALESCE(reply_to, 0), COALESCE(reply_handle, ''), COALESCE(reply_snip, ''), COALESCE(role, ''),
		        COALESCE(tip_to, ''), COALESCE(tip_amount, 0), COALESCE(avatar, '')
		 FROM messages WHERE room = $1 ORDER BY id DESC LIMIT $2`, room, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []chatMsg
	for rows.Next() {
		var m chatMsg
		if err := rows.Scan(&m.ID, &m.Handle, &m.Tier, &m.Body, &m.TS, &m.ReplyTo, &m.ReplyHandle, &m.ReplySnip, &m.Role, &m.TipTo, &m.TipAmt, &m.Av); err != nil {
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
	err := s.db.QueryRowContext(ctx, `SELECT GREATEST((SELECT COALESCE(MAX(id), 0) FROM messages), (SELECT COALESCE(MAX(id), 0) FROM dm_messages))`).Scan(&id)
	return id, err
}

func (s *store) insertMessages(ctx context.Context, msgs []storedMsg) error {
	n := len(msgs)
	ids, uids, tiers, ts, rto := make([]int64, n), make([]int64, n), make([]int64, n), make([]int64, n), make([]int64, n)
	handles, bodies, rh, rs := make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	rooms, roles, tipTo := make([]string, n), make([]string, n), make([]string, n)
	tipAmt := make([]int64, n)
	avs := make([]string, n)
	for i, m := range msgs {
		ids[i], uids[i], tiers[i], ts[i], rto[i] = m.ID, m.UserID, int64(m.Tier), m.TS, m.ReplyTo
		handles[i], bodies[i], rh[i], rs[i] = m.Handle, m.Body, m.ReplyHandle, m.ReplySnip
		rooms[i], roles[i] = m.Room, m.Role
		tipTo[i], tipAmt[i] = m.TipTo, int64(m.TipAmt)
		avs[i] = m.Avatar
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO messages (id, room, user_id, handle, tier, body, created_at, reply_to, reply_handle, reply_snip, role, tip_to, tip_amount, avatar)
		 SELECT t.id, t.room, t.uid, t.handle, t.tier, t.body, to_timestamp(t.ms / 1000.0),
		        NULLIF(t.rto, 0), NULLIF(t.rh, ''), NULLIF(t.rs, ''), NULLIF(t.role, ''), NULLIF(t.tipto, ''), NULLIF(t.tipamt, 0), NULLIF(t.av, '')
		 FROM unnest($1::bigint[], $2::bigint[], $3::text[], $4::smallint[], $5::text[], $6::bigint[],
		             $7::bigint[], $8::text[], $9::text[], $10::text[], $11::text[], $12::text[], $13::bigint[], $14::text[])
		      AS t(id, uid, handle, tier, body, ms, rto, rh, rs, room, role, tipto, tipamt, av)
		 ON CONFLICT (id) DO NOTHING`,
		pq.Array(ids), pq.Array(uids), pq.Array(handles), pq.Array(tiers), pq.Array(bodies), pq.Array(ts),
		pq.Array(rto), pq.Array(rh), pq.Array(rs), pq.Array(rooms), pq.Array(roles), pq.Array(tipTo), pq.Array(tipAmt), pq.Array(avs))
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

// ---- moderation ----

type reportRow struct {
	ID             int64
	ReporterID     int64
	ReporterHandle string
	MsgID          int64
	TargetHandle   string
	Body           string
	Reason         string
	TS             int64 // unix milliseconds
}

// activeSanctions returns when the user's current mute and kick end. Zero means none.
func (s *store) activeSanctions(ctx context.Context, userID int64) (mute, kick time.Time, err error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT kind, max(until) FROM sanctions WHERE user_id = $1 AND until > now() GROUP BY kind`, userID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var kind string
		var until time.Time
		if err = rows.Scan(&kind, &until); err != nil {
			return
		}
		if kind == "mute" {
			mute = until
		} else {
			kick = until
		}
	}
	err = rows.Err()
	return
}

func (s *store) addSanction(ctx context.Context, userID int64, kind, by, reason string, minutes int) (time.Time, error) {
	var until time.Time
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO sanctions (user_id, kind, until, by_handle, reason)
		 VALUES ($1, $2, now() + make_interval(mins => $3::int), $4, NULLIF($5, ''))
		 RETURNING until`, userID, kind, minutes, by, reason).Scan(&until)
	return until, err
}

// clearMute ends any active mute. It reports whether there was one.
func (s *store) clearMute(ctx context.Context, userID int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE sanctions SET until = now() WHERE user_id = $1 AND kind = 'mute' AND until > now()`, userID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *store) setRole(ctx context.Context, userID int64, role string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET role = $2 WHERE id = $1`, userID, role)
	return err
}

func (s *store) logMod(ctx context.Context, actor, action, target, detail string) {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO mod_log (actor, action, target, detail) VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''))`,
		actor, action, target, detail)
	if err != nil {
		log.Printf("writing the moderation log failed: %v", err)
	}
}

func (s *store) deleteMessage(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM messages WHERE id = $1`, id)
	return err
}

// addReport files a report. dup is true when this user already reported this message.
func (s *store) addReport(ctx context.Context, r reportRow) (id int64, dup bool, err error) {
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO reports (reporter_id, reporter_handle, msg_id, target_handle, body, reason)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (reporter_id, msg_id) DO NOTHING
		 RETURNING id`, r.ReporterID, r.ReporterHandle, r.MsgID, r.TargetHandle, r.Body, r.Reason).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, true, nil
	}
	return id, false, err
}

func (s *store) openReports(ctx context.Context, limit int) ([]reportRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, reporter_id, reporter_handle, msg_id, target_handle, body, reason,
		        (extract(epoch from created_at) * 1000)::bigint
		 FROM reports WHERE status = 'open' ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []reportRow
	for rows.Next() {
		var r reportRow
		if err := rows.Scan(&r.ID, &r.ReporterID, &r.ReporterHandle, &r.MsgID, &r.TargetHandle, &r.Body, &r.Reason, &r.TS); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *store) openReportCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM reports WHERE status = 'open'`).Scan(&n)
	return n, err
}

// closeReport marks an open report as handled. It reports whether one was open.
func (s *store) closeReport(ctx context.Context, id int64, status, by string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE reports SET status = $2, handled_by = $3, handled_at = now() WHERE id = $1 AND status = 'open'`, id, status, by)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// ensureAdmin makes sure the admin account exists. It is called at every start.
//   - Missing account: created with the password from hashFn.
//   - Existing guest account: adopted. It gets the password, and every old session is ended,
//     so only someone who knows the password can use it.
//   - Existing account with a password: only the role is set. The password is left alone.
func (s *store) ensureAdmin(ctx context.Context, handle, key string, hashFn func() (string, error)) (created, adopted bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()

	var id int64
	var pass sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id, pass_hash FROM users WHERE handle_key = $1 FOR UPDATE`, key).Scan(&id, &pass)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		var hash string
		if hash, err = hashFn(); err != nil {
			return
		}
		if _, err = tx.ExecContext(ctx,
			`INSERT INTO users (handle, handle_key, pass_hash, role) VALUES ($1, $2, $3, 'admin')`, handle, key, hash); err != nil {
			return
		}
		created = true
	case err != nil:
		return
	case !pass.Valid:
		var hash string
		if hash, err = hashFn(); err != nil {
			return
		}
		if _, err = tx.ExecContext(ctx, `UPDATE users SET pass_hash = $2, role = 'admin' WHERE id = $1`, id, hash); err != nil {
			return
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, id); err != nil {
			return
		}
		adopted = true
	default:
		if _, err = tx.ExecContext(ctx, `UPDATE users SET role = 'admin' WHERE id = $1`, id); err != nil {
			return
		}
	}
	err = tx.Commit()
	return
}

// ---- rooms ----

type roomRow struct {
	Slug        string
	Name        string
	Category    string
	Tags        string // comma separated
	MinTier     int
	Sort        int
	Kind        string // official, public or private
	OwnerID     int64  // 0 for official rooms
	OwnerHandle string
	PassHash    string // empty when there is no password
	Fee         int
	Invite      string
	LastActive  time.Time
}

// seedRooms creates the rooms that are missing. Rooms you already have, edited or not, are left alone.
func (s *store) seedRooms(ctx context.Context, rooms []roomRow) error {
	for _, r := range rooms {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO rooms (slug, name, category, tags, min_tier, sort) VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT (slug) DO NOTHING`, r.Slug, r.Name, r.Category, r.Tags, r.MinTier, r.Sort)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *store) listRooms(ctx context.Context) ([]roomRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT r.slug, r.name, r.category, r.tags, r.min_tier, r.sort, r.kind, coalesce(r.owner_id, 0), coalesce(u.handle, ''),
		        coalesce(r.pass_hash, ''), r.entry_fee, coalesce(r.invite, ''), r.last_active
		 FROM rooms r LEFT JOIN users u ON u.id = r.owner_id ORDER BY r.sort, r.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []roomRow
	for rows.Next() {
		var r roomRow
		if err := rows.Scan(&r.Slug, &r.Name, &r.Category, &r.Tags, &r.MinTier, &r.Sort, &r.Kind, &r.OwnerID, &r.OwnerHandle,
			&r.PassHash, &r.Fee, &r.Invite, &r.LastActive); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
