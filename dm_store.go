package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	errBlocked   = errors.New("blocked")
	errAlready   = errors.New("already friends or already asked")
	errNoRequest = errors.New("no such request")
	errFriendCap = errors.New("friend list is full")
	errTooMany   = errors.New("too many requests")
)

// pair puts two user ids in the order the friendships table keeps them.
func pair(a, b int64) (lo, hi int64) {
	if a < b {
		return a, b
	}
	return b, a
}

// ---- direct messages ----

type dmMsg struct {
	ID   int64  `json:"i"`
	From string `json:"f"`
	To   string `json:"w"`
	Body string `json:"b"`
	TS   int64  `json:"ts"`
}

// sendDM stores one direct message. When dmPays says it costs credits (and fee is not 0)
// the sender is charged fee credits in the same transaction, so a message is never
// sent without paying and never paid for without being sent. The credit entry uses
// the message id as its key, so a retry can not charge twice.
// charged is true when credits were taken, and bal is then the sender's new balance.
func (s *store) sendDM(ctx context.Context, id, from, to int64, toHandle, body string, ms, fee int64) (bal int64, charged bool, err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()

	var blocks int
	if err = tx.QueryRowContext(ctx,
		`SELECT count(*) FROM blocks WHERE (blocker = $1 AND blocked = $2) OR (blocker = $2 AND blocked = $1)`, from, to).Scan(&blocks); err != nil {
		return
	}
	if blocks > 0 {
		return 0, false, errBlocked
	}

	if fee > 0 {
		var pays bool
		if pays, err = dmPays(ctx, tx, from, to); err != nil {
			return
		}
		if pays {
			var applied bool
			bal, applied, err = applyCreditTx(ctx, tx, from, "dm", -fee, fmt.Sprintf("dm:%d", id), "Message to @"+toHandle)
			if err != nil {
				return 0, false, err
			}
			charged = applied
		}
	}

	if _, err = tx.ExecContext(ctx,
		`INSERT INTO dm_messages (id, sender_id, recipient_id, body, created_at) VALUES ($1, $2, $3, $4, to_timestamp($5::double precision / 1000))`,
		id, from, to, body, ms); err != nil {
		return 0, false, err
	}
	return bal, charged, tx.Commit()
}

// querier is what both a transaction and the database connection can do.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// dmPays says whether a message from "from" to "to" costs credits. It is free between friends.
// Between strangers the person who sent the first message in the conversation pays for every
// message, and the person who was written to replies for free. A pair with no messages yet
// means the sender is starting the conversation, so they pay.
func dmPays(ctx context.Context, q querier, from, to int64) (bool, error) {
	lo, hi := pair(from, to)
	var status string
	err := q.QueryRowContext(ctx, `SELECT status FROM friendships WHERE user_a = $1 AND user_b = $2`, lo, hi).Scan(&status)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if status == "accepted" {
		return false, nil
	}
	var first int64
	err = q.QueryRowContext(ctx,
		`SELECT sender_id FROM dm_messages WHERE LEAST(sender_id, recipient_id) = $1 AND GREATEST(sender_id, recipient_id) = $2 ORDER BY id ASC LIMIT 1`, lo, hi).Scan(&first)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return first == from, nil
}

// threadMessages returns the latest messages between two people, oldest first.
func (s *store) threadMessages(ctx context.Context, me, other int64, limit int) ([]dmMsg, error) {
	lo, hi := pair(me, other)
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, s.handle, r.handle, m.body, (extract(epoch from m.created_at) * 1000)::bigint
		 FROM dm_messages m JOIN users s ON s.id = m.sender_id JOIN users r ON r.id = m.recipient_id
		 WHERE LEAST(m.sender_id, m.recipient_id) = $1 AND GREATEST(m.sender_id, m.recipient_id) = $2
		 ORDER BY m.id DESC LIMIT $3`, lo, hi, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []dmMsg{}
	for rows.Next() {
		var m dmMsg
		if err := rows.Scan(&m.ID, &m.From, &m.To, &m.Body, &m.TS); err != nil {
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

// dmCost is what one message from me to other costs me right now.
func (s *store) dmCost(ctx context.Context, me, other, fee int64) (int64, error) {
	pays, err := dmPays(ctx, s.db, me, other)
	if err != nil || !pays {
		return 0, err
	}
	return fee, nil
}

// markRead marks everything the other person sent me as read.
func (s *store) markRead(ctx context.Context, me, other int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE dm_messages SET read_at = now() WHERE recipient_id = $1 AND sender_id = $2 AND read_at IS NULL`, me, other)
	return err
}

type chatRow struct {
	Handle string `json:"h"`
	LastID int64  `json:"i"`
	Mine   bool   `json:"m"` // the last message is mine
	Body   string `json:"b"`
	TS     int64  `json:"ts"`
	Unread int    `json:"u"`
}

const maxInboxChats = 50

// conversations lists my chats, newest first, leaving out people I blocked.
func (s *store) conversations(ctx context.Context, me int64) ([]chatRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`WITH mine AS (
		   SELECT id, sender_id, body, created_at,
		          CASE WHEN sender_id = $1 THEN recipient_id ELSE sender_id END AS other
		   FROM dm_messages WHERE sender_id = $1 OR recipient_id = $1
		 ), last AS (
		   SELECT DISTINCT ON (other) other, id, sender_id, body, (extract(epoch from created_at) * 1000)::bigint AS ms
		   FROM mine ORDER BY other, id DESC
		 ), unread AS (
		   SELECT sender_id AS other, count(*) AS n FROM dm_messages WHERE recipient_id = $1 AND read_at IS NULL GROUP BY sender_id
		 )
		 SELECT u.handle, l.id, l.sender_id = $1, l.body, l.ms, coalesce(un.n, 0)
		 FROM last l JOIN users u ON u.id = l.other LEFT JOIN unread un ON un.other = l.other
		 WHERE NOT EXISTS (SELECT 1 FROM blocks b WHERE b.blocker = $1 AND b.blocked = l.other)
		 ORDER BY l.id DESC LIMIT $2`, me, maxInboxChats)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []chatRow{}
	for rows.Next() {
		var c chatRow
		if err := rows.Scan(&c.Handle, &c.LastID, &c.Mine, &c.Body, &c.TS, &c.Unread); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// unreadAndRequests are the two numbers shown as a badge on the Inbox tab.
func (s *store) unreadAndRequests(ctx context.Context, me int64) (unread, requests int, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT
		   (SELECT count(*) FROM dm_messages m WHERE m.recipient_id = $1 AND m.read_at IS NULL
		      AND NOT EXISTS (SELECT 1 FROM blocks b WHERE b.blocker = $1 AND b.blocked = m.sender_id)),
		   (SELECT count(*) FROM friendships WHERE status = 'pending' AND requester <> $1 AND (user_a = $1 OR user_b = $1))`, me).Scan(&unread, &requests)
	return
}

// dmByID is used when someone reports a direct message.
func (s *store) dmByID(ctx context.Context, id int64) (senderHandle string, senderID, recipientID int64, body string, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT u.handle, m.sender_id, m.recipient_id, m.body FROM dm_messages m JOIN users u ON u.id = m.sender_id WHERE m.id = $1`, id).
		Scan(&senderHandle, &senderID, &recipientID, &body)
	if errors.Is(err, sql.ErrNoRows) {
		err = errNotFound
	}
	return
}

// deleteDM removes a direct message. It reports the sender, or "" when there was nothing to delete.
func (s *store) deleteDM(ctx context.Context, id int64) (string, error) {
	var handle string
	err := s.db.QueryRowContext(ctx,
		`WITH d AS (DELETE FROM dm_messages WHERE id = $1 RETURNING sender_id)
		 SELECT u.handle FROM d JOIN users u ON u.id = d.sender_id`, id).Scan(&handle)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return handle, err
}

func (s *store) pruneDMs(ctx context.Context, days int) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM dm_messages WHERE created_at < now() - make_interval(days => $1::int)`, days)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ---- friends ----

func (s *store) relation(ctx context.Context, me, other int64) (string, error) {
	lo, hi := pair(me, other)
	var status string
	var requester int64
	err := s.db.QueryRowContext(ctx, `SELECT status, requester FROM friendships WHERE user_a = $1 AND user_b = $2`, lo, hi).Scan(&status, &requester)
	if errors.Is(err, sql.ErrNoRows) {
		return "none", nil
	}
	if err != nil {
		return "", err
	}
	switch {
	case status == "accepted":
		return "friend", nil
	case requester == me:
		return "sent", nil
	}
	return "got", nil
}

func friendCount(ctx context.Context, tx *sql.Tx, id int64) (int, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM friendships WHERE status = 'accepted' AND (user_a = $1 OR user_b = $1)`, id).Scan(&n)
	return n, err
}

// friendAction changes the relationship between me and other.
// add, accept, decline, cancel and remove. It returns the new relationship from my point of view:
// none, friend or sent. A request that crosses an incoming one is accepted straight away.
func (s *store) friendAction(ctx context.Context, action string, me, other int64) (string, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	lo, hi := pair(me, other)
	var status string
	var requester int64
	err = tx.QueryRowContext(ctx, `SELECT status, requester FROM friendships WHERE user_a = $1 AND user_b = $2 FOR UPDATE`, lo, hi).Scan(&status, &requester)
	exists := true
	if errors.Is(err, sql.ErrNoRows) {
		exists, err = false, nil
	}
	if err != nil {
		return "", err
	}

	accept := func() (string, error) {
		for _, id := range []int64{me, other} {
			n, err := friendCount(ctx, tx, id)
			if err != nil {
				return "", err
			}
			if n >= friendCap {
				return "", errFriendCap
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE friendships SET status = 'accepted', accepted_at = now() WHERE user_a = $1 AND user_b = $2`, lo, hi); err != nil {
			return "", err
		}
		return "friend", tx.Commit()
	}
	remove := func(result string) (string, error) {
		if _, err := tx.ExecContext(ctx, `DELETE FROM friendships WHERE user_a = $1 AND user_b = $2`, lo, hi); err != nil {
			return "", err
		}
		return result, tx.Commit()
	}

	switch action {
	case "add":
		var blocks int
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM blocks WHERE (blocker = $1 AND blocked = $2) OR (blocker = $2 AND blocked = $1)`, me, other).Scan(&blocks); err != nil {
			return "", err
		}
		if blocks > 0 {
			return "", errBlocked
		}
		if exists {
			if status == "accepted" || requester == me {
				return "", errAlready
			}
			return accept() // they already asked me: both want it
		}
		var out, in int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM friendships WHERE status = 'pending' AND requester = $1`, me).Scan(&out); err != nil {
			return "", err
		}
		if err := tx.QueryRowContext(ctx,
			`SELECT count(*) FROM friendships WHERE status = 'pending' AND requester <> $1 AND (user_a = $1 OR user_b = $1)`, other).Scan(&in); err != nil {
			return "", err
		}
		if out >= pendingOutCap || in >= pendingInCap {
			return "", errTooMany
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO friendships (user_a, user_b, requester, status) VALUES ($1, $2, $3, 'pending')`, lo, hi, me); err != nil {
			return "", err
		}
		return "sent", tx.Commit()
	case "accept":
		if !exists || status != "pending" || requester != other {
			return "", errNoRequest
		}
		return accept()
	case "decline":
		if !exists || status != "pending" || requester != other {
			return "", errNoRequest
		}
		return remove("none")
	case "cancel":
		if !exists || status != "pending" || requester != me {
			return "", errNoRequest
		}
		return remove("none")
	case "remove":
		if !exists || status != "accepted" {
			return "", errNoRequest
		}
		return remove("none")
	}
	return "", errors.New("unknown friend action")
}

type friendLists struct {
	Friends  []string `json:"f"`
	Incoming []string `json:"q"` // asked me
	Outgoing []string `json:"p"` // I asked
	Blocked  []string `json:"x"`
}

func (s *store) friendLists(ctx context.Context, me int64) (friendLists, error) {
	out := friendLists{Friends: []string{}, Incoming: []string{}, Outgoing: []string{}, Blocked: []string{}}
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.handle, f.status, f.requester = $1
		 FROM friendships f JOIN users u ON u.id = CASE WHEN f.user_a = $1 THEN f.user_b ELSE f.user_a END
		 WHERE f.user_a = $1 OR f.user_b = $1 ORDER BY lower(u.handle)`, me)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var handle, status string
		var mine bool
		if err := rows.Scan(&handle, &status, &mine); err != nil {
			rows.Close()
			return out, err
		}
		switch {
		case status == "accepted":
			out.Friends = append(out.Friends, handle)
		case mine:
			out.Outgoing = append(out.Outgoing, handle)
		default:
			out.Incoming = append(out.Incoming, handle)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	brows, err := s.db.QueryContext(ctx, `SELECT u.handle FROM blocks b JOIN users u ON u.id = b.blocked WHERE b.blocker = $1 ORDER BY lower(u.handle)`, me)
	if err != nil {
		return out, err
	}
	defer brows.Close()
	for brows.Next() {
		var handle string
		if err := brows.Scan(&handle); err != nil {
			return out, err
		}
		out.Blocked = append(out.Blocked, handle)
	}
	return out, brows.Err()
}

// ---- blocks ----

// blockUser stops all messages and requests both ways, and ends any friendship or request between the two.
func (s *store) blockUser(ctx context.Context, me, other int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO blocks (blocker, blocked) VALUES ($1, $2) ON CONFLICT DO NOTHING`, me, other); err != nil {
		return err
	}
	lo, hi := pair(me, other)
	if _, err := tx.ExecContext(ctx, `DELETE FROM friendships WHERE user_a = $1 AND user_b = $2`, lo, hi); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *store) unblockUser(ctx context.Context, me, other int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM blocks WHERE blocker = $1 AND blocked = $2`, me, other)
	return err
}

func (s *store) iBlocked(ctx context.Context, me, other int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM blocks WHERE blocker = $1 AND blocked = $2`, me, other).Scan(&n)
	return n > 0, err
}
