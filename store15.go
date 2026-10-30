package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

var (
	errAlreadyBanned = errors.New("already banned")
	errNotBanned     = errors.New("not banned")
	errAdminTarget   = errors.New("admins can not be banned or deleted this way")
)

func logModTx(ctx context.Context, tx *sql.Tx, actor, action, target, detail string) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO mod_log (actor, action, target, detail) VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''))`,
		actor, action, target, detail)
	return err
}

// ---- permanent bans ----

// banUser bans an account for good. The ban, the end of every session and the audit entry are one transaction,
// so a ban is never half applied. Admins can not be banned.
func (s *store) banUser(ctx context.Context, targetID int64, by, reason string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var handle, role string
	var banned bool
	err = tx.QueryRowContext(ctx, `SELECT handle, role, banned_at IS NOT NULL FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, targetID).Scan(&handle, &role, &banned)
	if errors.Is(err, sql.ErrNoRows) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	if role == "admin" {
		return errAdminTarget
	}
	if banned {
		return errAlreadyBanned
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET banned_at = now(), banned_by = $2, ban_reason = NULLIF($3, '') WHERE id = $1`, targetID, by, reason); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1`, targetID); err != nil {
		return err
	}
	if err = logModTx(ctx, tx, by, "ban", handle, reason); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) unbanUser(ctx context.Context, targetID int64, by string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var handle string
	var banned bool
	err = tx.QueryRowContext(ctx, `SELECT handle, banned_at IS NOT NULL FROM users WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, targetID).Scan(&handle, &banned)
	if errors.Is(err, sql.ErrNoRows) {
		return errNotFound
	}
	if err != nil {
		return err
	}
	if !banned {
		return errNotBanned
	}
	if _, err = tx.ExecContext(ctx, `UPDATE users SET banned_at = NULL, banned_by = NULL, ban_reason = NULL WHERE id = $1`, targetID); err != nil {
		return err
	}
	if err = logModTx(ctx, tx, by, "unban", handle, ""); err != nil {
		return err
	}
	return tx.Commit()
}

// accountState is the live truth about an account, used to close races between sign in and a ban or deletion.
func (s *store) accountState(ctx context.Context, id int64) (banned, deleted bool, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT banned_at IS NOT NULL, deleted_at IS NOT NULL FROM users WHERE id = $1`, id).Scan(&banned, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return false, true, nil
	}
	return
}

func (s *store) roleOf(ctx context.Context, id int64) (string, error) {
	var role string
	err := s.db.QueryRowContext(ctx, `SELECT role FROM users WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return "", errNotFound
	}
	return role, err
}

// ---- reports against people and rooms, and complaints about staff ----

type reportInput struct {
	ReporterID     int64
	ReporterHandle string
	Kind           string // user or room
	TargetUserID   int64
	TargetHandle   string
	TargetRoom     string
	Reason         string
	Details        string
}

// addTargetReport files a report about a person or a room. dup is true when this reporter already has an open one.
func (s *store) addTargetReport(ctx context.Context, r reportInput) (id int64, dup bool, err error) {
	var q string
	switch r.Kind {
	case "user":
		q = `INSERT INTO reports (reporter_id, reporter_handle, kind, target_user_id, target_handle, body, reason, details)
		     VALUES ($1, $2, 'user', $3, $4, '', $5, $6)
		     ON CONFLICT (reporter_id, target_user_id) WHERE kind = 'user' AND status = 'open' DO NOTHING RETURNING id`
		err = s.db.QueryRowContext(ctx, q, r.ReporterID, r.ReporterHandle, r.TargetUserID, r.TargetHandle, r.Reason, r.Details).Scan(&id)
	case "room":
		q = `INSERT INTO reports (reporter_id, reporter_handle, kind, target_room, target_handle, body, reason, details)
		     VALUES ($1, $2, 'room', $3, $4, '', $5, $6)
		     ON CONFLICT (reporter_id, target_room) WHERE kind = 'room' AND status = 'open' DO NOTHING RETURNING id`
		err = s.db.QueryRowContext(ctx, q, r.ReporterID, r.ReporterHandle, r.TargetRoom, r.TargetHandle, r.Reason, r.Details).Scan(&id)
	default:
		return 0, false, errors.New("bad report kind")
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, true, nil
	}
	return id, false, err
}

type complaintRow struct {
	ID             int64
	ReporterID     int64
	ReporterHandle string
	TargetID       int64
	TargetHandle   string
	TargetRole     string
	MsgID          int64
	Body           string
	Category       string
	Details        string
	Status         string
	Note           string
	TS             int64
}

func (s *store) addComplaint(ctx context.Context, c complaintRow) (id int64, dup bool, err error) {
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO staff_complaints (reporter_id, reporter_handle, target_id, target_handle, target_role, msg_id, body, category, details)
		 VALUES ($1, $2, $3, $4, $5, NULLIF($6, 0), $7, $8, $9)
		 ON CONFLICT (reporter_id, target_id) WHERE status = 'open' DO NOTHING RETURNING id`,
		c.ReporterID, c.ReporterHandle, c.TargetID, c.TargetHandle, c.TargetRole, c.MsgID, c.Body, c.Category, c.Details).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, true, nil
	}
	return
}

// openComplaints lists open complaints for an admin. Complaints about the viewer are never shown to them.
func (s *store) openComplaints(ctx context.Context, viewerID int64, limit int) ([]complaintRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, reporter_id, reporter_handle, target_id, target_handle, target_role, coalesce(msg_id, 0), body, category, details,
		        (extract(epoch from created_at) * 1000)::bigint
		 FROM staff_complaints WHERE status = 'open' AND target_id <> $1 ORDER BY id DESC LIMIT $2`, viewerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []complaintRow
	for rows.Next() {
		var c complaintRow
		if err := rows.Scan(&c.ID, &c.ReporterID, &c.ReporterHandle, &c.TargetID, &c.TargetHandle, &c.TargetRole, &c.MsgID, &c.Body, &c.Category, &c.Details, &c.TS); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *store) openComplaintCount(ctx context.Context, viewerID int64) (n int, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM staff_complaints WHERE status = 'open' AND target_id <> $1`, viewerID).Scan(&n)
	return
}

// closeComplaint settles a complaint. An admin can not settle one about themselves.
func (s *store) closeComplaint(ctx context.Context, id int64, status, note, by string, viewerID int64) (bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var target string
	err = tx.QueryRowContext(ctx,
		`UPDATE staff_complaints SET status = $2, note = $3, handled_by = $4, handled_at = now()
		 WHERE id = $1 AND status = 'open' AND target_id <> $5 RETURNING target_handle`, id, status, note, by, viewerID).Scan(&target)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = logModTx(ctx, tx, by, "complaint_"+status, target, fmt.Sprintf("complaint %d", id)); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// ---- public search ----

type publicUser struct {
	Handle string
	Avatar string
	Role   string
	Points int64
}

// searchPublic finds people by handle for any signed in user. It returns public fields only.
// It leaves out the searcher, deleted and banned accounts, and anyone who blocked the searcher.
func (s *store) searchPublic(ctx context.Context, me int64, q string, limit int) ([]publicUser, error) {
	key := strings.ToLower(q)
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.handle, u.avatar || `+photoLookSQL+`, u.role, u.points FROM users u
		 WHERE u.handle_key LIKE $1 ESCAPE '\' AND u.id <> $2 AND u.deleted_at IS NULL AND u.banned_at IS NULL
		   AND NOT EXISTS (SELECT 1 FROM blocks b WHERE b.blocker = u.id AND b.blocked = $2)
		 ORDER BY (u.handle_key = $3) DESC, (u.handle_key LIKE $4 ESCAPE '\') DESC, u.handle_key LIMIT $5`,
		"%"+likeEscape(key)+"%", me, key, likeEscape(key)+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []publicUser
	for rows.Next() {
		var u publicUser
		if err := rows.Scan(&u.Handle, &u.Avatar, &u.Role, &u.Points); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ---- account deletion ----

// deleteResult says what a deletion did, so the hub can clean up memory and tell people.
type deleteResult struct {
	AlreadyDone bool
	Handle      string
	ClosedRooms []closedRoom
	Forfeited   int64
}

type closedRoom struct {
	Slug, Name string
	Members    []int64
}

// deleteAccount anonymizes an account in ONE transaction. It is safe to repeat: a second call finds the tombstone and
// does nothing. The users row is kept as an anonymous tombstone ("deleted_<id>") so credit_ledger keeps a valid
// pseudonymous reference. See README, "Account deletion and data retention", for what is deleted, anonymized and kept.
func (s *store) deleteAccount(ctx context.Context, id int64) (res deleteResult, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()

	var role string
	var deletedAt sql.NullTime
	var credits int64
	err = tx.QueryRowContext(ctx, `SELECT handle, role, deleted_at, credits FROM users WHERE id = $1 FOR UPDATE`, id).Scan(&res.Handle, &role, &deletedAt, &credits)
	if errors.Is(err, sql.ErrNoRows) {
		err = errNotFound
		return
	}
	if err != nil {
		return
	}
	if deletedAt.Valid {
		res.AlreadyDone = true
		err = tx.Commit()
		return
	}
	if role == "admin" {
		err = errAdminTarget // an admin must be demoted by another admin first
		return
	}
	tomb := fmt.Sprintf("deleted_%d", id)
	exec := func(q string, args ...any) {
		if err == nil {
			_, err = tx.ExecContext(ctx, q, args...)
		}
	}

	// Rooms this person owns are closed, with what was said in them. Members are returned so they can be told.
	rows, qerr := tx.QueryContext(ctx, `SELECT slug, name FROM rooms WHERE owner_id = $1 AND kind <> 'official' FOR UPDATE`, id)
	if qerr != nil {
		err = qerr
		return
	}
	for rows.Next() {
		var r closedRoom
		if err = rows.Scan(&r.Slug, &r.Name); err != nil {
			rows.Close()
			return
		}
		res.ClosedRooms = append(res.ClosedRooms, r)
	}
	rows.Close()
	for i := range res.ClosedRooms {
		r := &res.ClosedRooms[i]
		mrows, merr := tx.QueryContext(ctx, `SELECT user_id FROM room_members WHERE room = $1 AND user_id <> $2`, r.Slug, id)
		if merr != nil {
			err = merr
			return
		}
		for mrows.Next() {
			var m int64
			if err = mrows.Scan(&m); err != nil {
				mrows.Close()
				return
			}
			r.Members = append(r.Members, m)
		}
		mrows.Close()
		exec(`DELETE FROM messages WHERE room = $1`, r.Slug)
		exec(`DELETE FROM rooms WHERE slug = $1`, r.Slug) // room_members rows follow (ON DELETE CASCADE)
	}

	// Personal content.
	exec(`DELETE FROM message_reactions WHERE msg_id IN (SELECT id FROM messages WHERE user_id = $1)`, id)
	exec(`DELETE FROM room_pins WHERE msg_id IN (SELECT id FROM messages WHERE user_id = $1)`, id)
	exec(`DELETE FROM message_reactions WHERE user_id = $1`, id)
	exec(`DELETE FROM poll_votes WHERE user_id = $1`, id)
	exec(`DELETE FROM messages WHERE user_id = $1`, id)
	exec(`UPDATE messages SET tip_to = $2 WHERE lower(tip_to) = lower($1)`, res.Handle, tomb)
	exec(`UPDATE messages SET reply_handle = $2, reply_snip = '' WHERE lower(reply_handle) = lower($1)`, res.Handle, tomb)
	exec(`DELETE FROM dm_messages WHERE sender_id = $1 OR recipient_id = $1`, id)
	exec(`DELETE FROM friendships WHERE user_a = $1 OR user_b = $1`, id)
	exec(`DELETE FROM blocks WHERE blocker = $1 OR blocked = $1`, id)
	exec(`DELETE FROM notifications WHERE user_id = $1`, id)
	exec(`UPDATE notifications SET actor = $2, body = NULL WHERE lower(actor) = lower($1)`, res.Handle, tomb)
	exec(`DELETE FROM user_prefs WHERE user_id = $1`, id)
	exec(`DELETE FROM room_members WHERE user_id = $1`, id)
	exec(`DELETE FROM sessions WHERE user_id = $1`, id)

	// Records other people or the platform rely on keep their rows, but name only the tombstone.
	exec(`UPDATE reports SET reporter_handle = $2 WHERE reporter_id = $1`, id, tomb)
	exec(`UPDATE reports SET target_handle = $2 WHERE target_user_id = $1 OR lower(target_handle) = lower($3)`, id, tomb, res.Handle)
	exec(`UPDATE staff_complaints SET reporter_handle = $2 WHERE reporter_id = $1`, id, tomb)
	exec(`UPDATE staff_complaints SET target_handle = $2 WHERE target_id = $1`, id, tomb)
	exec(`UPDATE mod_log SET actor = $2 WHERE lower(actor) = lower($1)`, res.Handle, tomb)
	exec(`UPDATE mod_log SET target = $2 WHERE lower(target) = lower($1)`, res.Handle, tomb)
	exec(`UPDATE sanctions SET by_handle = $2 WHERE lower(by_handle) = lower($1)`, res.Handle, tomb)

	// Money: any balance is forfeited through a ledger row, so the ledger still adds up. Nothing is erased.
	if err == nil && credits > 0 {
		if _, _, err = applyCreditTx(ctx, tx, id, "account_closed", -credits, fmt.Sprintf("close:%d", id), "Account deleted, balance forfeited"); err == nil {
			res.Forfeited = credits
		}
	}
	// The tombstone.
	exec(`UPDATE users SET handle = $2, handle_key = $2, pass_hash = NULL, points = 0, streak_days = 0, streak_day = 0,
	        bio = '', avatar = '', photo_key = NULL, photo_ver = photo_ver + 1, role = 'user', deleted_at = now(), welcomed = true WHERE id = $1`, id, tomb)
	if err == nil {
		err = logModTx(ctx, tx, "system", "account_deleted", tomb, "")
	}
	if err != nil {
		return deleteResult{}, err
	}
	err = tx.Commit()
	return
}

// anonymousIDs lists accounts for the retention tests and maintenance.
func (s *store) tombstoneIDs(ctx context.Context, ids []int64) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM users WHERE id = ANY($1) AND deleted_at IS NOT NULL`, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// dmIsReported says whether a direct message appears in a report or a staff complaint.
func (s *store) dmIsReported(ctx context.Context, id int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT (SELECT count(*) FROM reports WHERE msg_id = $1 AND body LIKE '[direct message]%')
		      + (SELECT count(*) FROM staff_complaints WHERE msg_id = $1 AND body LIKE '[direct message]%')`, id).Scan(&n)
	return n > 0, err
}

// hiddenFrom says whether the target blocked the viewer, and whether the target is banned.
func (s *store) hiddenFrom(ctx context.Context, viewer, target int64) (blockedMe, banned bool, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM blocks WHERE blocker = $2 AND blocked = $1), banned_at IS NOT NULL FROM users WHERE id = $2`,
		viewer, target).Scan(&blockedMe, &banned)
	return
}
