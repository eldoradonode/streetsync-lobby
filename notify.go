package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Notifications and settings.
//
// A notification is written when something happens to a person that they may want to know about while they were away:
// a direct message, a friend request, a tag or a reply in a room, a gift. The same event never makes two rows
// (ref is unique per person), and chatty events are grouped so a busy friend is one row with a count.
// Everything is scoped to the signed in person on the server: there is no request that names someone else's rows.

const (
	noteListLimit = 50
	noteKeepDays  = 30
	maxTagsPerMsg = 3
)

type note struct {
	ID       int64
	Kind     string
	Actor    string
	Room     string
	RoomName string
	Body     string
	Amount   int
	N        int
	TS       int64
	Unread   bool
}

func (n note) json() map[string]any {
	m := map[string]any{"i": n.ID, "k": n.Kind, "a": n.Actor, "n": n.N, "ts": n.TS}
	if n.Room != "" {
		m["r"], m["rn"] = n.Room, n.RoomName
	}
	if n.Body != "" {
		m["b"] = n.Body
	}
	if n.Amount > 0 {
		m["m"] = n.Amount
	}
	if n.Unread {
		m["u"] = 1
	}
	return m
}

// ---- settings ----

type prefs struct {
	Dm, Tag, Tip, Friend    bool // which events make a notification
	FriendsOnly, HideOnline bool
}

func defaultPrefs() prefs { return prefs{Dm: true, Tag: true, Tip: true, Friend: true} }

func (p prefs) allows(kind string) bool {
	switch kind {
	case "dm":
		return p.Dm
	case "tag", "reply":
		return p.Tag
	case "tip":
		return p.Tip
	case "friend":
		return p.Friend
	}
	return false
}

func (p prefs) json() map[string]any {
	dp := "everyone"
	if p.FriendsOnly {
		dp = "friends"
	}
	return map[string]any{"t": "pf", "nd": p.Dm, "nk": p.Tag, "nt": p.Tip, "nf": p.Friend, "dp": dp, "so": !p.HideOnline}
}

// prefColumns are the only settings that can be changed, and the column each one writes.
var prefColumns = map[string]string{"nd": "n_dm", "nk": "n_tag", "nt": "n_tip", "nf": "n_friend", "dp": "dm_friends_only", "so": "hide_online"}

func (s *store) getPrefs(ctx context.Context, userID int64) (prefs, error) {
	p := defaultPrefs()
	err := s.db.QueryRowContext(ctx, `SELECT n_dm, n_tag, n_tip, n_friend, dm_friends_only, hide_online FROM user_prefs WHERE user_id = $1`, userID).
		Scan(&p.Dm, &p.Tag, &p.Tip, &p.Friend, &p.FriendsOnly, &p.HideOnline)
	if errors.Is(err, sql.ErrNoRows) {
		return defaultPrefs(), nil
	}
	return p, err
}

func (s *store) setPref(ctx context.Context, userID int64, col string, v bool) error {
	// col comes from prefColumns, never from the request.
	_, err := s.db.ExecContext(ctx, `INSERT INTO user_prefs (user_id, `+col+`) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET `+col+` = EXCLUDED.`+col, userID, v)
	return err
}

// ---- notification rows ----

// addNote writes a notification. mode "event" makes a row once and ignores repeats; "group" and "refresh" make or renew
// the row for this ref (a group counts up while unread). created is false when nothing new should be shown.
func (s *store) addNote(ctx context.Context, userID int64, n note, ref, mode string) (out note, created bool, err error) {
	q := `INSERT INTO notifications (user_id, kind, actor, ref, room, room_name, body, amount)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8)`
	if mode == "event" {
		q += ` ON CONFLICT (user_id, ref) DO NOTHING`
	} else {
		q += ` ON CONFLICT (user_id, ref) DO UPDATE SET body = EXCLUDED.body, room_name = EXCLUDED.room_name, amount = EXCLUDED.amount,
			n = CASE WHEN notifications.read_at IS NULL THEN notifications.n + 1 ELSE 1 END, read_at = NULL, created_at = now()`
	}
	q += ` RETURNING id, n, (extract(epoch from created_at) * 1000)::bigint`
	out = n
	err = s.db.QueryRowContext(ctx, q, userID, n.Kind, n.Actor, ref, n.Room, n.RoomName, n.Body, n.Amount).Scan(&out.ID, &out.N, &out.TS)
	if errors.Is(err, sql.ErrNoRows) {
		return out, false, nil
	}
	out.Unread = true
	return out, err == nil, err
}

func (s *store) listNotes(ctx context.Context, userID int64) ([]note, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, actor, COALESCE(room, ''), COALESCE(room_name, ''), COALESCE(body, ''), amount, n,
		(extract(epoch from created_at) * 1000)::bigint, read_at IS NULL
		FROM notifications WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2`, userID, noteListLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []note
	for rows.Next() {
		var n note
		if err := rows.Scan(&n.ID, &n.Kind, &n.Actor, &n.Room, &n.RoomName, &n.Body, &n.Amount, &n.N, &n.TS, &n.Unread); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s *store) unreadNotes(ctx context.Context, userID int64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

// readNotes marks one notification (id > 0) or all of them (id = 0) as read, for this person only.
func (s *store) readNotes(ctx context.Context, userID, id int64) (int64, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL AND ($2 = 0 OR id = $2)`, userID, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *store) readNoteRef(ctx context.Context, userID int64, ref string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE notifications SET read_at = now() WHERE user_id = $1 AND ref = $2 AND read_at IS NULL`, userID, ref)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *store) pruneNotes(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM notifications WHERE created_at < now() - make_interval(days => $1::int)`, noteKeepDays)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *store) usersByKeys(ctx context.Context, keys []string) (map[string]int64, error) {
	out := map[string]int64{}
	if len(keys) == 0 {
		return out, nil
	}
	args := make([]any, len(keys))
	marks := make([]string, len(keys))
	for i, k := range keys {
		args[i], marks[i] = k, "$"+strconv.Itoa(i+1)
	}
	rows, err := s.db.QueryContext(ctx, `SELECT handle_key, id FROM users WHERE handle_key IN (`+strings.Join(marks, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var id int64
		if err := rows.Scan(&k, &id); err != nil {
			return nil, err
		}
		out[k] = id
	}
	return out, rows.Err()
}

// ---- hub side ----

// prefsOf reads a person's settings. The copy kept for people who are online is used when there is one.
func (h *hub) prefsOf(ctx context.Context, id int64) prefs {
	h.mu.Lock()
	if ou := h.users[id]; ou != nil && ou.prefsLoaded {
		p := ou.prefs
		h.mu.Unlock()
		return p
	}
	h.mu.Unlock()
	p, err := h.st.getPrefs(ctx, id)
	if err != nil {
		log.Printf("loading settings failed: %v", err)
		return defaultPrefs()
	}
	return p
}

type noteIn struct {
	To      int64
	ActorID int64
	Note    note
	Ref     string
	Mode    string
	Room    *room // the room it happened in: people already looking at it are not told again
}

// notify records an event for someone, in the background so nothing waits on it, and tells their open tabs.
// It does nothing when the person switched that kind off, blocked the other person, or is the other person.
func (h *hub) notify(in noteIn) {
	if in.To == in.ActorID || in.To == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), authWait)
		defer cancel()
		if !h.prefsOf(ctx, in.To).allows(in.Note.Kind) {
			return
		}
		if blocked, err := h.st.iBlocked(ctx, in.To, in.ActorID); err != nil || blocked {
			return
		}
		if in.Room != nil {
			h.mu.Lock()
			watching := false
			if ou := h.users[in.To]; ou != nil {
				for c := range ou.clients {
					if c.room == in.Room {
						watching = true
					}
				}
			}
			h.mu.Unlock()
			if watching {
				return
			}
		}
		n, created, err := h.st.addNote(ctx, in.To, in.Note, in.Ref, in.Mode)
		if err != nil {
			log.Printf("saving a notification failed: %v", err)
			return
		}
		if !created {
			return
		}
		count, err := h.st.unreadNotes(ctx, in.To)
		if err != nil {
			return
		}
		h.sendToUser(in.To, frame(map[string]any{"t": "nn", "n": n.json(), "c": count}))
	}()
}

var tagRE = regexp.MustCompile(`(?:^|[^A-Za-z0-9_])@([A-Za-z0-9_]{3,16})`)

// notifyTags tells the people a room message tagged with @handle, and the author of the message it replied to.
// It runs after the message was delivered, so chat never waits for it.
func (h *hub) notifyTags(rm *room, from *onlineUser, msgID int64, body, replyHandle string) {
	keys := map[string]bool{}
	var order []string
	for _, m := range tagRE.FindAllStringSubmatch(body, -1) {
		k := strings.ToLower(m[1])
		if !keys[k] && len(order) < maxTagsPerMsg {
			keys[k] = true
			order = append(order, k)
		}
	}
	rk := ""
	if replyHandle != "" {
		rk = strings.ToLower(replyHandle)
		if !keys[rk] {
			order = append(order, rk)
			keys[rk] = true
		}
	}
	if len(order) == 0 {
		return
	}
	snip := snippet(body)
	name, slug, actor, actorID := rm.name, rm.slug, from.handle, from.id
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), authWait)
		defer cancel()
		ids, err := h.st.usersByKeys(ctx, order)
		if err != nil {
			log.Printf("finding tagged people failed: %v", err)
			return
		}
		for _, k := range order {
			id, ok := ids[k]
			if !ok {
				continue
			}
			kind := "tag"
			if k == rk {
				kind = "reply"
			}
			h.notify(noteIn{To: id, ActorID: actorID, Room: rm, Mode: "group", Ref: kind + ":" + slug + ":" + strconv.FormatInt(actorID, 10),
				Note: note{Kind: kind, Actor: actor, Room: slug, RoomName: name, Body: snip}})
		}
	}()
	_ = msgID
}

// ---- requests from the page ----

// notesList handles {"t":"nlist"}: my latest notifications and how many are unread.
func (h *hub) notesList(ctx context.Context, cl *client) {
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	list, err := h.st.listNotes(dctx, cl.user.id)
	var count int
	if err == nil {
		count, err = h.st.unreadNotes(dctx, cl.user.id)
	}
	if err != nil {
		log.Printf("loading notifications failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, n := range list {
		out = append(out, n.json())
	}
	cl.enqueue(frame(map[string]any{"t": "notes", "l": out, "c": count}))
}

// notesRead handles {"t":"nread","i":12} for one notification or {"t":"nread","all":true} for all of mine.
func (h *hub) notesRead(ctx context.Context, cl *client, in inbound) {
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	id := int64(0)
	if !in.All {
		if in.I <= 0 {
			cl.enqueue(errFrame("bad_request"))
			return
		}
		id = in.I
	}
	if _, err := h.st.readNotes(dctx, cl.user.id, id); err != nil {
		log.Printf("marking notifications read failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	h.pushNoteCount(cl.user.id)
}

// readRef marks the notification for one ref as read, for example when the conversation it is about is opened.
func (h *hub) readRef(userID int64, ref string) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), authWait)
		defer cancel()
		if n, err := h.st.readNoteRef(ctx, userID, ref); err == nil && n > 0 {
			h.pushNoteCount(userID)
		}
	}()
}

func (h *hub) pushNoteCount(userID int64) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), authWait)
		defer cancel()
		c, err := h.st.unreadNotes(ctx, userID)
		if err != nil {
			return
		}
		h.sendToUser(userID, frame(map[string]any{"t": "nc", "c": c}))
	}()
}

// sendNoteStateTo gives a connection that just opened its unread count and its settings.
func (h *hub) sendNoteStateTo(cl *client) {
	ctx, cancel := context.WithTimeout(context.Background(), authWait)
	defer cancel()
	p, err := h.st.getPrefs(ctx, cl.user.id)
	if err != nil {
		log.Printf("loading settings failed: %v", err)
		p = defaultPrefs()
	}
	h.mu.Lock()
	cl.user.prefs, cl.user.prefsLoaded = p, true
	h.mu.Unlock()
	cl.enqueue(frame(p.json()))
	if c, err := h.st.unreadNotes(ctx, cl.user.id); err == nil {
		cl.enqueue(frame(map[string]any{"t": "nc", "c": c}))
	}
}

// setPrefFrame handles {"t":"set","k":"nd","b":"0"}. Only the settings in prefColumns exist.
func (h *hub) setPrefFrame(ctx context.Context, cl *client, in inbound) {
	col, ok := prefColumns[in.K]
	if !ok {
		cl.enqueue(errFrame("bad_request"))
		return
	}
	// The value is in "b": "1" or "0" for on and off, "friends" or "everyone" for who may message me.
	var v bool
	switch {
	case in.K == "dp":
		if in.B != "everyone" && in.B != "friends" {
			cl.enqueue(errFrame("bad_request"))
			return
		}
		v = in.B == "friends"
	case in.B == "1" || in.B == "0":
		v = in.B == "1"
		if in.K == "so" { // the setting is "show when I am online", stored as its opposite
			v = !v
		}
	default:
		cl.enqueue(errFrame("bad_request"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	if err := h.st.setPref(dctx, cl.user.id, col, v); err != nil {
		log.Printf("saving a setting failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	ou := cl.user
	h.mu.Lock()
	switch in.K {
	case "nd":
		ou.prefs.Dm = v
	case "nk":
		ou.prefs.Tag = v
	case "nt":
		ou.prefs.Tip = v
	case "nf":
		ou.prefs.Friend = v
	case "dp":
		ou.prefs.FriendsOnly = v
	case "so":
		ou.prefs.HideOnline = v
	}
	ou.prefsLoaded = true
	f := frame(ou.prefs.json())
	for c := range ou.clients {
		c.enqueue(f)
	}
	h.mu.Unlock()
}

// ---- change password ----

// passwordChange handles {"t":"pwchange","o":"old","p":"new"}. It uses the same attempt limit as logging in,
// and ends every other login of the account, so a stolen login does not survive a change.
func (h *hub) passwordChange(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	if len(in.P) < minPassBytes || len(in.P) > maxPassBytes {
		cl.enqueue(errFrame("weak_password"))
		return
	}
	h.mu.Lock()
	has, key := ou.hasPass, strings.ToLower(ou.handle)
	h.mu.Unlock()
	if !has {
		cl.enqueue(errFrame("needs_password"))
		return
	}
	now := time.Now()
	if !h.limit.allowed(key, now) {
		cl.enqueue(errFrame("too_many_attempts"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, 2*authWait)
	defer cancel()
	row, err := h.st.userByKey(dctx, key)
	if err != nil || !row.PassHash.Valid {
		cl.enqueue(errFrame("server_busy"))
		return
	}
	match, err := h.auth.verify(dctx, row.PassHash.String, in.O)
	if err != nil {
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if !match {
		h.limit.fail(key, now)
		cl.enqueue(errFrame("bad_login"))
		return
	}
	h.limit.reset(key)
	hash, err := h.auth.hash(dctx, in.P)
	if err != nil {
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if err := h.st.replacePassword(dctx, ou.id, hash, cl.tokenH); err != nil {
		log.Printf("changing a password failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	cl.enqueue(frame(map[string]string{"t": "pwchanged"}))
}

func (s *store) replacePassword(ctx context.Context, userID int64, hash, keepToken string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET pass_hash = $2 WHERE id = $1`, userID, hash); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = $1 AND token_hash <> $2`, userID, keepToken); err != nil {
		return err
	}
	return tx.Commit()
}
