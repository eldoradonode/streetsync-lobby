package main

import (
	"context"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/lib/pq"
)

// Step 20: favourite rooms, recently visited rooms, directory sorting and the room member list.
//
// Rules for all of it:
//   - The server decides. The page only shows what it is sent.
//   - Nothing here is an access path. A favourite or a recent entry is only a shortcut: opening it goes through the
//     normal join, so passwords, fees, invites, tiers, bans and membership are checked exactly as before.
//   - Rooms the person cannot see (private rooms they have no access to, password rooms they never entered) are never
//     listed, favourited or described.
//   - Nothing here earns points or credits, and nothing here changes who is in a room.

const (
	memberPage = 40
	listGap    = 300 * time.Millisecond // least time between two member list requests of one person
)

// How many favourites and recent rooms a person keeps. Variables only so the tests can use small numbers.
var (
	favLimit    = 50
	recentLimit = 15
)

// ---- store ----

func (s *store) loadSavedRooms(ctx context.Context, userID int64) (favs, visits []string, err error) {
	rows, err := s.db.QueryContext(ctx, `SELECT room FROM room_favourites WHERE user_id = $1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, nil, err
	}
	for rows.Next() {
		var r string
		if err = rows.Scan(&r); err != nil {
			rows.Close()
			return nil, nil, err
		}
		favs = append(favs, r)
	}
	rows.Close()
	vrows, err := s.db.QueryContext(ctx, `SELECT room FROM room_visits WHERE user_id = $1 ORDER BY visited_at DESC LIMIT $2`, userID, recentLimit)
	if err != nil {
		return nil, nil, err
	}
	defer vrows.Close()
	for vrows.Next() {
		var r string
		if err = vrows.Scan(&r); err != nil {
			return nil, nil, err
		}
		visits = append(visits, r)
	}
	return favs, visits, vrows.Err()
}

// setFavourite adds or removes one favourite. full is true when the person already has favLimit of them.
func (s *store) setFavourite(ctx context.Context, userID int64, room string, on bool) (full bool, err error) {
	if !on {
		_, err = s.db.ExecContext(ctx, `DELETE FROM room_favourites WHERE user_id = $1 AND room = $2`, userID, room)
		return false, err
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO room_favourites (user_id, room) SELECT $1, $2 WHERE (SELECT count(*) FROM room_favourites WHERE user_id = $1) < $3
		 ON CONFLICT (user_id, room) DO NOTHING`, userID, room, favLimit)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return false, nil
	}
	var exists bool
	err = s.db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM room_favourites WHERE user_id = $1 AND room = $2)`, userID, room).Scan(&exists)
	return !exists, err
}

func (s *store) recordVisit(ctx context.Context, userID int64, room string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO room_visits (user_id, room) VALUES ($1, $2) ON CONFLICT (user_id, room) DO UPDATE SET visited_at = now()`, userID, room); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM room_visits WHERE user_id = $1 AND room NOT IN (SELECT room FROM room_visits WHERE user_id = $1 ORDER BY visited_at DESC LIMIT $2)`, userID, recentLimit)
	return err
}

// ---- who may see a room ----

// canSeeRoomLocked: official rooms and listed community rooms are for everyone. A private room or one with a password
// is only for people who own it, were let in (the standing right), or are staff.
func (h *hub) canSeeRoomLocked(ou *onlineUser, rm *room) bool {
	if !rm.community() || rm.listed() {
		return true
	}
	_, mine := ou.rooms[rm.slug]
	return mine || rm.ownerID == ou.id || roleRank(ou.role) >= 2
}

// ---- favourites and recent rooms ----

func (h *hub) loadSavedRoomsFor(ou *onlineUser) {
	ctx, cancel := context.WithTimeout(context.Background(), authWait)
	defer cancel()
	ou.favMu.Lock() // a favourite change that is under way finishes first, so what is read here includes it
	defer ou.favMu.Unlock()
	favs, visits, err := h.st.loadSavedRooms(ctx, ou.id)
	if err != nil {
		log.Printf("loading favourite rooms failed: %v", err)
		return
	}
	h.mu.Lock()
	// A room entered while this was loading (a page that reconnects straight into its room) is newer than the
	// database copy, which may not have it yet: keep those first.
	merged := make([]string, 0, recentLimit)
	seen := map[string]bool{}
	for _, list := range [][]string{ou.visits, visits} {
		for _, s := range list {
			if !seen[s] && len(merged) < recentLimit {
				seen[s] = true
				merged = append(merged, s)
			}
		}
	}
	ou.favs, ou.visits = favs, merged
	b := h.myRoomsFrameLocked(ou)
	for c := range ou.clients {
		c.enqueue(b)
	}
	h.mu.Unlock()
}

// myRoomsFrameLocked is the person's favourites and recent rooms. Entries for rooms that no longer exist, or that
// the person may no longer see, are dropped from memory and left out. (The database rows of a deleted room go with it.)
func (h *hub) myRoomsFrameLocked(ou *onlineUser) []byte {
	now := time.Now()
	build := func(slugs []string) ([]string, []roomJSON) {
		keep := make([]string, 0, len(slugs))
		list := make([]roomJSON, 0, len(slugs))
		for _, s := range slugs {
			rm := h.rooms[s]
			if rm == nil || !h.canSeeRoomLocked(ou, rm) {
				continue
			}
			keep = append(keep, s)
			list = append(list, h.roomJSONLocked(rm, ou, h.roomUsers(rm), now))
		}
		return keep, list
	}
	var fl, vl []roomJSON
	ou.favs, fl = build(ou.favs)
	ou.visits, vl = build(ou.visits)
	slugs := make([]string, 0, len(ou.favs))
	slugs = append(slugs, ou.favs...)
	return frame(map[string]any{"t": "mr", "fs": slugs, "f": fl, "v": vl})
}

func (h *hub) sendMyRooms(cl *client) {
	h.mu.Lock()
	b := h.myRoomsFrameLocked(cl.user)
	h.mu.Unlock()
	cl.enqueue(b)
}

// favouriteRoom handles {"t":"fav","r":"slug","fav":true}. It is saved before it is confirmed.
func (h *hub) favouriteRoom(ctx context.Context, cl *client, in inbound) {
	if in.Fav == nil {
		cl.enqueue(errFrame("bad_request"))
		return
	}
	ou := cl.user
	ou.favMu.Lock() // two tabs toggling at once keep the database and the memory in step
	defer ou.favMu.Unlock()
	h.mu.Lock()
	rm := h.rooms[in.R]
	if rm == nil || (*in.Fav && !h.canSeeRoomLocked(ou, rm)) {
		h.mu.Unlock()
		cl.enqueue(errFrame("no_such_room")) // the same answer for a room that does not exist and one you may not see
		return
	}
	slug := rm.slug
	h.mu.Unlock()
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	full, err := h.st.setFavourite(dctx, ou.id, slug, *in.Fav)
	if err != nil {
		log.Printf("saving a favourite room failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if full {
		cl.enqueue(errFrame("fav_full"))
		return
	}
	h.mu.Lock()
	rest := make([]string, 0, len(ou.favs)+1)
	for _, s := range ou.favs {
		if s != slug {
			rest = append(rest, s)
		}
	}
	if *in.Fav {
		rest = append([]string{slug}, rest...)
	}
	ou.favs = rest
	b := h.myRoomsFrameLocked(ou)
	for c := range ou.clients { // every tab and device of the person
		c.enqueue(b)
	}
	h.mu.Unlock()
}

// noteVisit remembers that the person entered a room. It is called only after a successful entry, so a refused
// password or an unaffordable fee leaves nothing behind.
func (h *hub) noteVisit(ou *onlineUser, slug string) {
	h.mu.Lock()
	rest := make([]string, 0, recentLimit)
	rest = append(rest, slug)
	for _, s := range ou.visits {
		if s != slug && len(rest) < recentLimit {
			rest = append(rest, s)
		}
	}
	ou.visits = rest
	h.mu.Unlock()
	safeGo(func() {
		ctx, cancel := context.WithTimeout(context.Background(), authWait)
		defer cancel()
		if err := h.st.recordVisit(ctx, ou.id, slug); err != nil {
			log.Printf("saving a visited room failed: %v", err)
		}
	})
}

// ---- directory sorting ----

type rankedRoom struct {
	rm *room
	n  int
}

// sortRooms orders the discoverable rooms. The numbers come from the server: people in the room right now, the time of
// the last message, the time the room was created. Unknown names fall back to the default.
func sortRooms(hits []rankedRoom, mode string) string {
	switch mode {
	case "recent":
		sort.SliceStable(hits, func(i, j int) bool {
			if !hits[i].rm.lastActive.Equal(hits[j].rm.lastActive) {
				return hits[i].rm.lastActive.After(hits[j].rm.lastActive)
			}
			return hits[i].n > hits[j].n
		})
	case "new":
		sort.SliceStable(hits, func(i, j int) bool {
			if !hits[i].rm.created.Equal(hits[j].rm.created) {
				return hits[i].rm.created.After(hits[j].rm.created)
			}
			return hits[i].n > hits[j].n
		})
	default:
		mode = "active"
		sort.SliceStable(hits, func(i, j int) bool {
			if hits[i].n != hits[j].n {
				return hits[i].n > hits[j].n
			}
			return hits[i].rm.lastActive.After(hits[j].rm.lastActive)
		})
	}
	return mode
}

// ---- the member list ----

type memberEntry struct {
	H string `json:"h"`           // handle
	A string `json:"a,omitempty"` // avatar look
	S string `json:"s"`           // on, off or hid (the person keeps their online state private)
	O string `json:"o,omitempty"` // mod or admin
	X bool   `json:"x,omitempty"` // the host
}

// roomVisibleLocked: may other people see this person's online state? Until their settings are known they count as
// hidden, the same rule the conversation header uses.
func roomVisibleLocked(ou *onlineUser) bool { return ou.prefsLoaded && !ou.prefs.HideOnline }

func (h *hub) isMemberLocked(rm *room, ou *onlineUser) bool {
	if !rm.community() {
		return false
	}
	_, j := ou.joined[rm.slug]
	return j || rm.ownerID == ou.id
}

func memberRole(role string) string {
	if role == "mod" || role == "admin" {
		return role
	}
	return ""
}

func (h *hub) memberEntryLocked(rm *room, ou *onlineUser, status string) memberEntry {
	return memberEntry{H: ou.handle, A: ou.look(), S: status, O: memberRole(ou.role), X: rm.ownerID == ou.id}
}

// roomMembersList handles {"t":"members","off":0}. Only a person who is inside the room may ask, and only about
// that room, so the list is never a way to look into a room one could not enter. The people who are here now come
// from memory, the ones who are not from the database, a page at a time.
func (h *hub) roomMembersList(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	now := time.Now()
	h.mu.Lock()
	rm := cl.room
	if rm == nil {
		h.mu.Unlock()
		cl.enqueue(errFrame("no_room"))
		return
	}
	if now.Sub(ou.lastList) < listGap {
		h.mu.Unlock()
		cl.enqueue(errFrame("slow_down"))
		return
	}
	ou.lastList = now
	off := in.Off
	if off < 0 {
		off = 0
	}
	cl.members = rm // from now on this connection hears about changes
	var here []memberEntry
	var hereIDs []int64
	extra := 0
	for id, w := range rm.hereWho {
		vis := roomVisibleLocked(w)
		member := h.isMemberLocked(rm, w)
		switch {
		case vis:
			here = append(here, h.memberEntryLocked(rm, w, "on"))
			if !member {
				extra++
			}
		case member:
			here = append(here, h.memberEntryLocked(rm, w, "hid"))
		default:
			continue // a hidden visitor is not revealed by listing them
		}
		hereIDs = append(hereIDs, id)
	}
	online := 0
	for _, e := range here {
		if e.S == "on" {
			online++
		}
	}
	sort.Slice(here, func(i, j int) bool {
		if here[i].S != here[j].S {
			return here[i].S == "on"
		}
		if here[i].X != here[j].X {
			return here[i].X
		}
		return strings.ToLower(here[i].H) < strings.ToLower(here[j].H)
	})
	community, slug, owner := rm.community(), rm.slug, rm.ownerID
	// ids of everyone present, hidden or not, so the database page does not list them a second time
	for id := range rm.hereWho {
		found := false
		for _, x := range hereIDs {
			if x == id {
				found = true
			}
		}
		if !found {
			hereIDs = append(hereIDs, id)
		}
	}
	h.mu.Unlock()

	page := make([]memberEntry, 0, memberPage)
	if off < len(here) {
		end := off + memberPage
		if end > len(here) {
			end = len(here)
		}
		page = append(page, here[off:end]...)
	}
	total, more := len(here), false
	if community {
		dctx, cancel := context.WithTimeout(ctx, authWait)
		defer cancel()
		dbOff := off - len(here)
		if dbOff < 0 {
			dbOff = 0
		}
		rows, n, err := h.st.offlineMembers(dctx, slug, owner, hereIDs, memberPage-len(page)+1, dbOff, off == 0)
		if err != nil {
			log.Printf("loading the member list failed: %v", err)
			cl.enqueue(errFrame("server_busy"))
			return
		}
		room := memberPage - len(page)
		if len(rows) > room {
			rows, more = rows[:room], true
		}
		for _, r := range rows {
			s := "off"
			if r.Hidden {
				s = "hid"
			}
			page = append(page, memberEntry{H: r.Handle, A: r.Avatar, S: s, O: memberRole(r.Role), X: r.ID == owner})
		}
		if off == 0 {
			total = len(here) + n
		}
	}
	if off+len(page) < len(here) {
		more = true
	}
	frameOut := map[string]any{"t": "ml", "r": slug, "m": page, "off": off, "more": more, "on": online}
	if off == 0 {
		frameOut["tot"] = total
	}
	cl.enqueue(frame(frameOut))
}

type offlineRow struct {
	ID                   int64
	Handle, Avatar, Role string
	Hidden               bool
}

// offlineMembers is one page of the people who belong to a room but are not in it now: the members who have not left,
// and the host, without deleted accounts. count is only asked for on the first page.
func (s *store) offlineMembers(ctx context.Context, slug string, owner int64, present []int64, limit, offset int, count bool) ([]offlineRow, int, error) {
	base := `FROM (SELECT user_id FROM room_members WHERE room = $1 AND left_at IS NULL UNION SELECT $2::bigint) s
		 JOIN users u ON u.id = s.user_id LEFT JOIN user_prefs p ON p.user_id = u.id
		 WHERE u.deleted_at IS NULL AND NOT (u.id = ANY($3))`
	n := 0
	if count {
		if err := s.db.QueryRowContext(ctx, `SELECT count(*) `+base, slug, owner, pq.Array(present)).Scan(&n); err != nil {
			return nil, 0, err
		}
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT u.id, u.handle, u.avatar || `+photoLookSQL+`, coalesce(u.role, 'user'), coalesce(p.hide_online, false) `+base+`
		 ORDER BY lower(u.handle) LIMIT $4 OFFSET $5`, slug, owner, pq.Array(present), limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []offlineRow
	for rows.Next() {
		var r offlineRow
		if err := rows.Scan(&r.ID, &r.Handle, &r.Avatar, &r.Role, &r.Hidden); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, n, rows.Err()
}

func (h *hub) membersClose(cl *client) {
	h.mu.Lock()
	cl.members = nil
	h.mu.Unlock()
}

// sendMemberLocked tells the connections that have the member list open about one change.
func (h *hub) sendMemberLocked(rm *room, v map[string]any) {
	var b []byte
	for c := range rm.clients {
		if c.members == rm {
			if b == nil {
				b = frame(v)
			}
			c.enqueue(b)
		}
	}
}

// memberPresenceLocked is called whenever a person starts or stops counting as here. Their status in the list follows
// the same facts as the join and leave notices, so it obeys the grace periods and several tabs: it changes only when
// the notice would. A person who keeps their online state private gets no frames at all, in either direction: a frame
// at the moment they arrive or leave would give them away.
func (h *hub) memberPresenceLocked(rm *room, ou *onlineUser, here bool) {
	vis, member := roomVisibleLocked(ou), h.isMemberLocked(rm, ou)
	if vis {
		ou.roomVis = 1
	} else {
		ou.roomVis = 2
		return
	}
	e := h.memberEntryLocked(rm, ou, "")
	v := map[string]any{"t": "mp", "h": e.H, "a": e.A, "o": e.O, "x": e.X}
	switch {
	case here:
		v["s"] = "on"
	case member:
		v["s"] = "off"
	default:
		v["d"] = 1 // someone who only visited is no longer in the list
	}
	h.sendMemberLocked(rm, v)
}

// memberRemovedLocked: the person stopped being a member (Leave room, or joined another room).
func (h *hub) memberRemovedLocked(rm *room, handle string) {
	h.sendMemberLocked(rm, map[string]any{"t": "mp", "h": handle, "d": 1})
}

// memberVisibilityLocked is called when a person's online-visibility setting becomes known or changes. Becoming
// visible shows them as here in the rooms they are in. Becoming hidden takes the "online" off them: members go back to
// "status hidden" and visitors leave the list, as if they had gone.
func (h *hub) memberVisibilityLocked(ou *onlineUser) {
	was := ou.roomVis
	vis := roomVisibleLocked(ou)
	switch {
	case vis && was != 1:
		for _, rm := range h.roomList {
			if rm.hereWho[ou.id] != nil {
				h.memberPresenceLocked(rm, ou, true)
			}
		}
	case !vis && was == 1:
		ou.roomVis = 2
		for _, rm := range h.roomList {
			if rm.hereWho[ou.id] != nil {
				v := map[string]any{"t": "mp", "h": ou.handle}
				if h.isMemberLocked(rm, ou) {
					v["s"] = "hid"
				} else {
					v["d"] = 1
				}
				h.sendMemberLocked(rm, v)
			}
		}
	}
}
