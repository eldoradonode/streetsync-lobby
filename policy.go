package main

import (
	"context"
	"database/sql"
	"errors"
)

// ---- role hierarchy and staff interaction policy ----
//
// Roles, highest first:  admin (3)  >  mod (2)  >  user (1).
//
// Moderation (mute, kick, delete someone's message, clear a bio, reports):
//   - nobody acts on themselves
//   - a moderator may act on ordinary users only
//   - an admin may act on moderators and ordinary users
//   - nobody acts on an admin (canAct in mod.go)
//   - promote, demote, grant credits, permanent ban and unban are admin only
//
// Staff shield (ordinary accounts reaching staff accounts):
//   - an ordinary user can not tip, start a direct message with, send a friend request to,
//     or file an ordinary report about a moderator or an admin
//   - they can reply to a staff member who wrote to them first, accept a request a staff member sent,
//     and use the staff complaint channel (scomp), which only admins can read
//   - the rules live in the server (here and in the store transactions), never only in the page
//
// The role is always read from the database or from the server's own record of the account.
// No frame carries a role, so a client can not claim one.

var (
	errStaffTarget = errors.New("staff accounts can not be reached this way")
	errGone        = errors.New("account is deleted")
)

func isStaff(role string) bool { return roleRank(role) >= 2 }

// staffShielded says whether an actor with actorRole may NOT reach targetRole through social features
// (tips, new direct messages, friend requests, ordinary reports).
func staffShielded(actorRole, targetRole string) bool {
	return !isStaff(actorRole) && isStaff(targetRole)
}

// accountFacts are the trusted facts about two accounts, read inside the transaction that acts on them.
type accountFacts struct {
	Role    string
	Deleted bool
	Banned  bool
}

// factsTx reads role, deleted and banned flags for two accounts and locks neither.
func factsTx(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, id int64) (accountFacts, error) {
	var f accountFacts
	err := q.QueryRowContext(ctx, `SELECT role, deleted_at IS NOT NULL, banned_at IS NOT NULL FROM users WHERE id = $1`, id).Scan(&f.Role, &f.Deleted, &f.Banned)
	if errors.Is(err, sql.ErrNoRows) {
		return f, errNotFound
	}
	return f, err
}

// staffStartedWith says whether a staff member has already written to this person, or is their friend.
// That is the only way an ordinary user may write to staff.
func staffStartedWith(ctx context.Context, q querier, user, staff int64) (bool, error) {
	var n int
	err := q.QueryRowContext(ctx,
		`SELECT (SELECT count(*) FROM dm_messages WHERE sender_id = $2 AND recipient_id = $1)
		      + (SELECT count(*) FROM friendships WHERE user_a = LEAST($1::bigint, $2::bigint) AND user_b = GREATEST($1::bigint, $2::bigint) AND status = 'accepted')`,
		user, staff).Scan(&n)
	return n > 0, err
}
