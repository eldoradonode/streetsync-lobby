package main

import (
	"context"
	"database/sql"
	"log"
	"sort"
	"strings"
	"time"
)

// Read only queries for the staff panels. Every one of them checks the role again on the server:
// the page hiding a button is a courtesy, not the lock.
//
//	astats   staff   platform numbers (moderators get the part about moderation)
//	ausers   staff   find people by handle, or the newest accounts when the search is empty
//	arooms   staff   every room, including private ones
//	astaff   admin   who has moderator or admin powers, and who promoted them
//	alog     admin   the moderation log

const (
	adminUserLimit = 20
	adminLogLimit  = 60
)

// ---- store ----

type platformStats struct {
	Users, WithPass, New24h, New7d, Active24h, Mods, Admins int
	Credits                                                 int64
	Messages24h                                             int
	Mutes, Kicks                                            int
	OpenReports                                             int
	MsgDays, UserDays                                       map[string]int
}

func (s *store) platformStats(ctx context.Context) (platformStats, error) {
	var p platformStats
	err := s.db.QueryRowContext(ctx, `SELECT count(*),
		count(*) FILTER (WHERE pass_hash IS NOT NULL),
		count(*) FILTER (WHERE created_at > now() - interval '24 hours'),
		count(*) FILTER (WHERE created_at > now() - interval '7 days'),
		count(*) FILTER (WHERE last_seen > now() - interval '24 hours'),
		count(*) FILTER (WHERE role = 'mod'),
		count(*) FILTER (WHERE role = 'admin'),
		COALESCE(sum(credits), 0) FROM users`).
		Scan(&p.Users, &p.WithPass, &p.New24h, &p.New7d, &p.Active24h, &p.Mods, &p.Admins, &p.Credits)
	if err != nil {
		return p, err
	}
	if err = s.db.QueryRowContext(ctx, `SELECT count(*) FROM messages WHERE created_at > now() - interval '24 hours'`).Scan(&p.Messages24h); err != nil {
		return p, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT kind, count(DISTINCT user_id) FROM sanctions WHERE until > now() GROUP BY kind`)
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var kind string
		var n int
		if err = rows.Scan(&kind, &n); err != nil {
			rows.Close()
			return p, err
		}
		if kind == "mute" {
			p.Mutes = n
		} else {
			p.Kicks = n
		}
	}
	rows.Close()
	if p.OpenReports, err = s.openReportCount(ctx); err != nil {
		return p, err
	}
	if p.MsgDays, err = s.perDay(ctx, `SELECT to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD'), count(*) FROM messages WHERE created_at > now() - interval '8 days' GROUP BY 1`); err != nil {
		return p, err
	}
	p.UserDays, err = s.perDay(ctx, `SELECT to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD'), count(*) FROM users WHERE created_at > now() - interval '8 days' GROUP BY 1`)
	return p, err
}

func (s *store) perDay(ctx context.Context, q string) (map[string]int, error) {
	out := map[string]int{}
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var d string
		var n int
		if err := rows.Scan(&d, &n); err != nil {
			return out, err
		}
		out[d] = n
	}
	return out, rows.Err()
}

type adminUserRow struct {
	ID               int64
	Handle, Role     string
	Points, Credits  int64
	Joined, LastSeen time.Time
	HasPass          bool
	Mute, Kick       sql.NullTime
	Banned           bool
}

// likeEscape makes a search word safe to put inside a LIKE pattern.
func likeEscape(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func (s *store) searchUsers(ctx context.Context, key string, limit int) ([]adminUserRow, error) {
	q := `SELECT u.id, u.handle, u.role, u.points, u.credits, u.created_at, u.last_seen, u.pass_hash IS NOT NULL,
		(SELECT max(until) FROM sanctions x WHERE x.user_id = u.id AND x.kind = 'mute' AND x.until > now()),
		(SELECT max(until) FROM sanctions x WHERE x.user_id = u.id AND x.kind = 'kick' AND x.until > now()),
		u.banned_at IS NOT NULL
		FROM users u `
	var rows *sql.Rows
	var err error
	if key == "" {
		rows, err = s.db.QueryContext(ctx, q+`WHERE u.deleted_at IS NULL ORDER BY u.id DESC LIMIT $1`, limit)
	} else {
		rows, err = s.db.QueryContext(ctx, q+`WHERE u.deleted_at IS NULL AND u.handle_key LIKE $1 ESCAPE '\' ORDER BY (u.handle_key = $2) DESC, (u.handle_key LIKE $3 ESCAPE '\') DESC, u.handle_key LIMIT $4`,
			"%"+likeEscape(key)+"%", key, likeEscape(key)+"%", limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []adminUserRow
	for rows.Next() {
		var u adminUserRow
		if err := rows.Scan(&u.ID, &u.Handle, &u.Role, &u.Points, &u.Credits, &u.Joined, &u.LastSeen, &u.HasPass, &u.Mute, &u.Kick, &u.Banned); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

type staffRow struct {
	Handle, Role     string
	Joined, LastSeen time.Time
	By               sql.NullString
	At               sql.NullTime
}

func (s *store) staffList(ctx context.Context) ([]staffRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT u.handle, u.role, u.created_at, u.last_seen, l.actor, l.created_at
		FROM users u
		LEFT JOIN LATERAL (SELECT actor, created_at FROM mod_log WHERE action = 'promote' AND lower(target) = u.handle_key ORDER BY id DESC LIMIT 1) l ON true
		WHERE u.role IN ('mod', 'admin') ORDER BY (u.role = 'admin') DESC, u.handle_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []staffRow
	for rows.Next() {
		var r staffRow
		if err := rows.Scan(&r.Handle, &r.Role, &r.Joined, &r.LastSeen, &r.By, &r.At); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type logRow struct {
	Actor, Action string
	Target, Info  sql.NullString
	At            time.Time
}

func (s *store) modLog(ctx context.Context, limit int) ([]logRow, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT actor, action, target, detail, created_at FROM mod_log ORDER BY id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []logRow
	for rows.Next() {
		var r logRow
		if err := rows.Scan(&r.Actor, &r.Action, &r.Target, &r.Info, &r.At); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ---- handler ----

func (h *hub) adminQuery(ctx context.Context, cl *client, in inbound) {
	h.mu.Lock()
	role := cl.user.role
	h.mu.Unlock()
	rank := roleRank(role)
	adminOnly := in.T == "astaff" || in.T == "alog"
	if rank < 2 || (adminOnly && rank < 3) {
		cl.enqueue(errFrame("forbidden"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, 2*authWait)
	defer cancel()
	busy := func(what string, err error) {
		log.Printf("admin %s failed: %v", what, err)
		cl.enqueue(errFrame("server_busy"))
	}

	switch in.T {
	case "astats":
		p, err := h.st.platformStats(dctx)
		if err != nil {
			busy("stats", err)
			return
		}
		h.mu.Lock()
		online, official, community, inRooms := 0, 0, 0, 0
		for _, ou := range h.users {
			if len(ou.clients) > 0 {
				online++
			}
		}
		for _, rm := range h.roomList {
			if rm.community() {
				community++
			} else {
				official++
			}
			inRooms += len(rm.clients)
		}
		h.mu.Unlock()
		out := map[string]any{"t": "astats", "on": online, "rep": p.OpenReports, "mu": p.Mutes, "ki": p.Kicks,
			"ro": official, "rc": community, "ir": inRooms}
		if rank >= 3 {
			out["us"], out["pw"], out["n1"], out["n7"], out["a1"] = p.Users, p.WithPass, p.New24h, p.New7d, p.Active24h
			out["mo"], out["ad"], out["cr"], out["m1"] = p.Mods, p.Admins, p.Credits, p.Messages24h
			out["md"], out["ud"] = lastDays(p.MsgDays, 7), lastDays(p.UserDays, 7)
		}
		cl.enqueue(frame(out))

	case "ausers":
		key := onlyHandleChars(strings.ToLower(clean(in.H))) // letters, numbers and underscore only; '@' is dropped
		if len(key) > 17 {
			key = key[:17]
		}
		rows, err := h.st.searchUsers(dctx, key, adminUserLimit)
		if err != nil {
			busy("users", err)
			return
		}
		h.mu.Lock()
		list := make([]map[string]any, 0, len(rows))
		for _, u := range rows {
			ou := h.users[u.ID]
			e := map[string]any{"h": u.Handle, "o": u.Role, "p": u.Points, "r": tierFor(u.Points), "j": u.Joined.UnixMilli(), "g": u.HasPass,
				"on": ou != nil && len(ou.clients) > 0}
			if u.Mute.Valid {
				e["mu"] = minutesLeft(u.Mute.Time)
			}
			if u.Banned {
				e["bn"] = true
			}
			if u.Kick.Valid {
				e["ki"] = minutesLeft(u.Kick.Time)
			}
			if rank >= 3 {
				e["c"], e["ls"] = u.Credits, u.LastSeen.UnixMilli()
			}
			list = append(list, e)
		}
		h.mu.Unlock()
		cl.enqueue(frame(map[string]any{"t": "ausers", "q": key, "u": list}))

	case "astaff":
		rows, err := h.st.staffList(dctx)
		if err != nil {
			busy("staff", err)
			return
		}
		h.mu.Lock()
		list := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			on := false
			for _, ou := range h.users {
				if strings.EqualFold(ou.handle, r.Handle) && len(ou.clients) > 0 {
					on = true
					break
				}
			}
			e := map[string]any{"h": r.Handle, "o": r.Role, "j": r.Joined.UnixMilli(), "ls": r.LastSeen.UnixMilli(), "on": on}
			if r.By.Valid {
				e["by"] = r.By.String
			}
			if r.At.Valid {
				e["at"] = r.At.Time.UnixMilli()
			}
			list = append(list, e)
		}
		h.mu.Unlock()
		cl.enqueue(frame(map[string]any{"t": "astaff", "s": list}))

	case "alog":
		rows, err := h.st.modLog(dctx, adminLogLimit)
		if err != nil {
			busy("log", err)
			return
		}
		list := make([]map[string]any, 0, len(rows))
		for _, r := range rows {
			e := map[string]any{"a": r.Actor, "x": r.Action, "ts": r.At.UnixMilli()}
			if r.Target.Valid {
				e["h"] = r.Target.String
			}
			if r.Info.Valid {
				e["d"] = r.Info.String
			}
			list = append(list, e)
		}
		cl.enqueue(frame(map[string]any{"t": "alog", "l": list}))

	case "arooms":
		now := time.Now()
		h.mu.Lock()
		type item struct {
			e map[string]any
			n int
		}
		items := make([]item, 0, len(h.roomList))
		for _, rm := range h.roomList {
			n := h.roomUsers(rm)
			e := map[string]any{"s": rm.slug, "n": rm.name, "c": rm.category, "u": n, "k": rm.kind, "h": rm.heat(now)}
			if rm.community() {
				e["o"], e["f"], e["w"], e["la"] = rm.ownerHandle, rm.fee, rm.passHash != "", rm.lastActive.UnixMilli()
			}
			items = append(items, item{e, n})
		}
		h.mu.Unlock()
		sort.SliceStable(items, func(i, j int) bool { return items[i].n > items[j].n })
		list := make([]map[string]any, 0, len(items))
		for _, it := range items {
			list = append(list, it.e)
		}
		cl.enqueue(frame(map[string]any{"t": "arooms", "r": list}))
	}
}

// lastDays returns the counts of the last n days in order, oldest first, with zeros for quiet days.
func lastDays(m map[string]int, n int) []map[string]any {
	out := make([]map[string]any, 0, n)
	today := time.Now().UTC()
	for i := n - 1; i >= 0; i-- {
		d := today.AddDate(0, 0, -i).Format("2006-01-02")
		out = append(out, map[string]any{"d": d, "n": m[d]})
	}
	return out
}

func onlyHandleChars(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' {
			return r
		}
		return -1
	}, s)
}
