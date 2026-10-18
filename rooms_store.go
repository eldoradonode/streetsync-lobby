package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

func isUnique(err error) bool {
	var pe *pq.Error
	return errors.As(err, &pe) && pe.Code == "23505"
}

var (
	errRoomLimit = errors.New("room limit reached")
	errNameTaken = errors.New("room name taken")
)

// createRoom makes a room and takes the creation fee in one transaction, so a failed
// creation is never charged and a charge always comes with a room.
// limit is how many rooms this person may own. It returns the new balance.
func (s *store) createRoom(ctx context.Context, r roomRow, cost int64, limit int) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Serialise creations by the same person, so two taps can not get around the limit.
	if _, err := tx.ExecContext(ctx, `SELECT 1 FROM users WHERE id = $1 FOR UPDATE`, r.OwnerID); err != nil {
		return 0, err
	}
	var owned int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM rooms WHERE owner_id = $1`, r.OwnerID).Scan(&owned); err != nil {
		return 0, err
	}
	if owned >= limit {
		return 0, errRoomLimit
	}
	var bal int64
	if cost > 0 {
		var applied bool
		bal, applied, err = applyCreditTx(ctx, tx, r.OwnerID, "room_create", -cost, "room:"+r.Slug, "Created the room "+r.Name)
		if err != nil {
			return 0, err
		}
		if !applied {
			return 0, errors.New("room fee already taken")
		}
	}
	var pass any
	if r.PassHash != "" {
		pass = r.PassHash
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO rooms (slug, name, category, tags, min_tier, sort, kind, owner_id, pass_hash, entry_fee, invite)
		 VALUES ($1, $2, $3, $4, 0, $5, $6, $7, $8, $9, $10)`,
		r.Slug, r.Name, r.Category, r.Tags, r.Sort, r.Kind, r.OwnerID, pass, r.Fee, r.Invite)
	if err != nil {
		if isUnique(err) {
			return 0, errNameTaken
		}
		return 0, err
	}
	return bal, tx.Commit()
}

// deleteRoom removes a room and what was said in it.
func (s *store) deleteRoom(ctx context.Context, slug string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM messages WHERE room = $1`, slug); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM rooms WHERE slug = $1 AND kind <> 'official'`, slug); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) isRoomMember(ctx context.Context, slug string, userID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM room_members WHERE room = $1 AND user_id = $2`, slug, userID).Scan(&n)
	return n > 0, err
}

// hostShare is what the host of a room keeps from each entry fee, in percent. The rest is the platform's.
const hostShare = 80

// enterRoom lets someone in for good. If there is an entry fee they pay it, and the host is paid
// their share, all in one transaction. The room's price is checked here, against the database,
// so a host who raised it a moment ago can not be paid less than the visitor agreed to.
// A person who is already a member pays nothing. It returns the visitor's new balance when they paid.
func (s *store) enterRoom(ctx context.Context, slug string, userID int64, agreedFee int) (bal int64, charged bool, hostID int64, hostGot int64, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()

	var fee int
	var owner sql.NullInt64
	var name string
	if err = tx.QueryRowContext(ctx, `SELECT entry_fee, owner_id, name FROM rooms WHERE slug = $1 FOR SHARE`, slug).Scan(&fee, &owner, &name); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			err = errNotFound
		}
		return
	}
	var inserted bool
	var x int
	err = tx.QueryRowContext(ctx, `INSERT INTO room_members (room, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING RETURNING 1`, slug, userID).Scan(&x)
	if err == nil {
		inserted = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return
	}
	err = nil
	if !inserted {
		return 0, false, 0, 0, tx.Commit() // already a member
	}
	if fee != agreedFee {
		return 0, false, 0, int64(fee), errFeeChanged
	}
	if fee > 0 {
		var applied bool
		bal, applied, err = applyCreditTx(ctx, tx, userID, "room_entry", -int64(fee), "roomin:"+slug, "Entered "+name)
		if err != nil {
			return 0, false, 0, 0, err
		}
		charged = applied
		if owner.Valid && owner.Int64 != userID {
			hostID = owner.Int64
			hostGot = int64(fee) * hostShare / 100
			if hostGot > 0 {
				if _, _, err = applyCreditTx(ctx, tx, hostID, "room_income", hostGot, fmt.Sprintf("roomin:%s:%d", slug, userID), "Entry to "+name); err != nil {
					return 0, false, 0, 0, err
				}
			}
		}
	}
	err = tx.Commit()
	return
}

var errFeeChanged = errors.New("the entry fee changed")

// updateRoomSettings saves what the host changed. A nil pointer means leave it alone.
func (s *store) updateRoomSettings(ctx context.Context, slug string, fee *int, passHash *string, kind *string) error {
	if fee != nil {
		if _, err := s.db.ExecContext(ctx, `UPDATE rooms SET entry_fee = $2 WHERE slug = $1 AND kind <> 'official'`, slug, *fee); err != nil {
			return err
		}
	}
	if passHash != nil {
		var v any
		if *passHash != "" {
			v = *passHash
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE rooms SET pass_hash = $2 WHERE slug = $1 AND kind <> 'official'`, slug, v); err != nil {
			return err
		}
	}
	if kind != nil {
		if _, err := s.db.ExecContext(ctx, `UPDATE rooms SET kind = $2 WHERE slug = $1 AND kind <> 'official'`, slug, *kind); err != nil {
			return err
		}
	}
	return nil
}

// touchRooms remembers when rooms last had a message, so the idle cleanup survives a restart.
func (s *store) touchRooms(ctx context.Context, active map[string]time.Time) error {
	for slug, at := range active {
		if _, err := s.db.ExecContext(ctx, `UPDATE rooms SET last_active = $2 WHERE slug = $1`, slug, at); err != nil {
			return err
		}
	}
	return nil
}

// memberRooms lists the private, password and fee rooms a person has been let into, plus the rooms they own.
func (s *store) memberRooms(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT room FROM room_members WHERE user_id = $1 UNION SELECT slug FROM rooms WHERE owner_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
