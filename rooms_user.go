package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"log"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// Rules for rooms people create. Starting values, easy to tune.
const (
	roomCreateCost  = 25  // credits to create a room
	roomMinTier     = 1   // Regular
	roomMaxFee      = 50  // highest entry fee a host can set
	maxCommunity    = 200 // rooms people created, all together, to keep the free database small
	roomNameMin     = 3
	roomNameMax     = 30
	maxRoomTags     = 3
	roomTagMax      = 16
	roomPassMin     = 4
	roomPassMax     = 32
	maxRoomPwFails  = 5 // wrong room passwords in roomPwWindow
	roomPwWindow    = 10 * time.Minute
	hostMuteMax     = 1440 // longest a host can mute or kick, in minutes
	findLimit       = 10
	defaultRoomIdle = 3 * 24 * time.Hour
)

// roomLimitFor is how many rooms a person may own at once. Legends get two.
func roomLimitFor(tier int) int {
	if tier >= 4 {
		return 2
	}
	return 1
}

func randCode(n int, alphabet string) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b)
}

const (
	inviteAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0, O, 1 or I
	slugAlphabet   = "abcdefghijklmnopqrstuvwxyz0123456789"
)

// cleanRoomName tidies a room name and says whether it is allowed.
func cleanRoomName(raw string) (string, bool) {
	name := strings.Join(strings.Fields(clean(raw)), " ")
	n := utf8.RuneCountInString(name)
	if n < roomNameMin || n > roomNameMax {
		return "", false
	}
	hasWord := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			hasWord = true
		case strings.ContainsRune(" -_'.&!?,", r):
		default:
			return "", false
		}
	}
	return name, hasWord
}

// roomTags turns "football, banter, abuja, extra" into at most three short tags.
func roomTags(raw string) string {
	var out []string
	for _, t := range strings.Split(raw, ",") {
		t = strings.Join(strings.Fields(clean(t)), " ")
		if r := []rune(t); len(r) > roomTagMax {
			t = string(r[:roomTagMax])
		}
		if t != "" {
			out = append(out, t)
		}
		if len(out) == maxRoomTags {
			break
		}
	}
	return strings.Join(out, ",")
}

// roomSlug makes the address of a room from its name, with a random ending so it is never guessable or taken.
func roomSlug(name string) string {
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	s := strings.Trim(b.String(), "-")
	if len(s) > 24 {
		s = strings.Trim(s[:24], "-")
	}
	if s == "" {
		s = "room"
	}
	return s + "-" + randCode(4, slugAlphabet)
}

func (ou *onlineUser) addRoom(slug string) {
	if ou.rooms == nil {
		ou.rooms = make(map[string]struct{})
	}
	ou.rooms[slug] = struct{}{}
}

// pushRooms sends a fresh room list to every tab of a person that is on the hub.
func (h *hub) pushRooms(ou *onlineUser) {
	h.mu.Lock()
	defer h.mu.Unlock()
	b := h.roomsFrameForLocked(ou)
	for c := range ou.clients {
		if c.room == nil {
			c.enqueue(b)
		}
	}
}

// loadMemberships learns which rooms a person owns or was let into, so the hub can show them.
func (h *hub) loadMemberships(ou *onlineUser) {
	ctx, cancel := context.WithTimeout(context.Background(), authWait)
	defer cancel()
	slugs, err := h.st.memberRooms(ctx, ou.id)
	if err != nil {
		log.Printf("loading rooms failed: %v", err)
		return
	}
	if len(slugs) == 0 {
		return
	}
	h.mu.Lock()
	for _, s := range slugs {
		ou.addRoom(s)
	}
	h.mu.Unlock()
	h.pushRooms(ou)
}

// refreshCredits tells a person who is online their balance after someone paid them.
func (h *hub) refreshCredits(ctx context.Context, id int64) {
	h.mu.Lock()
	ou := h.users[id]
	h.mu.Unlock()
	if ou == nil {
		return
	}
	if bal, err := h.st.creditBalance(ctx, id); err == nil {
		h.setCredits(ou, bal)
	}
}

// ---- create ----

// roomCreate handles {"t":"croom","n":"Name","k":"public|private","g":"tag, tag","p":"password","f":10}
func (h *hub) roomCreate(ctx context.Context, cl *client, in inbound) {
	ou := cl.user
	name, ok := cleanRoomName(in.N)
	if !ok {
		cl.enqueue(errFrame("bad_room_name"))
		return
	}
	if in.K != "public" && in.K != "private" {
		cl.enqueue(errFrame("bad_room_kind"))
		return
	}
	category, ok := cleanRoomCategory(in.C)
	if !ok {
		cl.enqueue(errFrame("bad_category"))
		return
	}
	fee := 0
	if in.F != nil {
		fee = *in.F
	}
	if fee < 0 || fee > roomMaxFee {
		cl.enqueue(errFrame("bad_fee"))
		return
	}
	if in.P != "" {
		if n := utf8.RuneCountInString(in.P); n < roomPassMin || n > roomPassMax {
			cl.enqueue(errFrame("bad_room_password"))
			return
		}
	}

	h.mu.Lock()
	switch {
	case roleRank(ou.role) < 2 && ou.tier < roomMinTier:
		h.mu.Unlock()
		cl.enqueue(errFrameN("tier_locked", roomMinTier))
		return
	case ou.creating:
		h.mu.Unlock()
		cl.enqueue(errFrame("slow_down"))
		return
	}
	owned, community := 0, 0
	for _, rm := range h.roomList {
		if rm.community() {
			community++
			if rm.ownerID == ou.id {
				owned++
			}
		}
		if strings.EqualFold(rm.name, name) {
			h.mu.Unlock()
			cl.enqueue(errFrame("room_name_taken"))
			return
		}
	}
	limit := roomLimitFor(ou.tier)
	if owned >= limit {
		h.mu.Unlock()
		cl.enqueue(errFrameN("room_limit", limit))
		return
	}
	if community >= maxCommunity {
		h.mu.Unlock()
		cl.enqueue(errFrame("rooms_full"))
		return
	}
	var slug string
	for {
		if slug = roomSlug(name); h.rooms[slug] == nil {
			break
		}
	}
	ou.creating = true
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		ou.creating = false
		h.mu.Unlock()
	}()

	dctx, cancel := context.WithTimeout(ctx, 2*authWait)
	defer cancel()
	passHash := ""
	if in.P != "" {
		var err error
		if passHash, err = h.auth.hash(dctx, in.P); err != nil {
			cl.enqueue(errFrame("server_busy"))
			return
		}
	}
	row := roomRow{Slug: slug, Name: name, Category: category, Tags: roomTags(in.G), Sort: 1000, Kind: in.K,
		OwnerID: ou.id, OwnerHandle: ou.handle, PassHash: passHash, Fee: fee, Invite: randCode(8, inviteAlphabet), LastActive: time.Now()}
	bal, err := h.st.createRoom(dctx, row, roomCreateCost, limit)
	switch {
	case errors.Is(err, errNoFunds):
		cl.enqueue(errFrameN("no_credits", roomCreateCost))
		return
	case errors.Is(err, errRoomLimit):
		cl.enqueue(errFrameN("room_limit", limit))
		return
	case errors.Is(err, errNameTaken):
		cl.enqueue(errFrame("room_name_taken"))
		return
	case err != nil:
		log.Printf("creating a room failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}

	h.mu.Lock()
	rm := newRoom(row)
	h.rooms[slug] = rm
	h.roomList = append(h.roomList, rm)
	ou.addRoom(slug)
	cl.enqueue(frame(map[string]string{"t": "croomok", "s": slug, "n": name, "i": row.Invite}))
	h.mu.Unlock()
	h.setCredits(ou, bal)
	h.pushRooms(ou)
}

// roomCategories are the topics a room's host can choose from. An empty choice means General.
var roomCategories = []string{"General", "City", "Campus", "Sport", "Music", "Tech", "Business", "Creators", "Entertainment", "Lifestyle"}

func cleanRoomCategory(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "General", true
	}
	for _, c := range roomCategories {
		if strings.EqualFold(c, raw) {
			return c, true
		}
	}
	return "", false
}

// roomDir handles {"t":"dir"}: every public room without a password, even empty ones, busiest first.
// The hub list only shows rooms with people in them, so this is how nobody's room gets lost.
func (h *hub) roomDir(cl *client) {
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	type hit struct {
		rm *room
		n  int
	}
	var hits []hit
	for _, rm := range h.roomList {
		if rm.community() && rm.listed() {
			hits = append(hits, hit{rm, h.roomUsers(rm)})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].n != hits[j].n {
			return hits[i].n > hits[j].n
		}
		return hits[i].rm.lastActive.After(hits[j].rm.lastActive)
	})
	list := make([]roomJSON, 0, len(hits))
	for _, x := range hits {
		list = append(list, h.roomJSONLocked(x.rm, cl.user, x.n, now))
	}
	cl.enqueue(frame(roomsFrame{T: "dir", R: list}))
}

// ---- find, look up, join ----

// roomFind handles {"t":"find","n":"foot"}: public rooms without a password, matching the words, even empty ones.
func (h *hub) roomFind(cl *client, in inbound) {
	q := strings.ToLower(strings.TrimSpace(clean(in.N)))
	if utf8.RuneCountInString(q) > 30 {
		cl.enqueue(errFrame("bad_query"))
		return
	}
	now := time.Now()
	h.mu.Lock()
	defer h.mu.Unlock()
	type hit struct {
		rm *room
		n  int
	}
	var hits []hit
	for _, rm := range h.roomList {
		if !rm.community() || !rm.listed() {
			continue
		}
		if q != "" {
			match := strings.Contains(strings.ToLower(rm.name), q)
			for _, t := range rm.tags {
				if strings.Contains(strings.ToLower(t), q) {
					match = true
				}
			}
			if !match {
				continue
			}
		}
		hits = append(hits, hit{rm, h.roomUsers(rm)})
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].n != hits[j].n {
			return hits[i].n > hits[j].n
		}
		return hits[i].rm.lastActive.After(hits[j].rm.lastActive)
	})
	if len(hits) > findLimit {
		hits = hits[:findLimit]
	}
	list := make([]roomJSON, 0, len(hits))
	for _, x := range hits {
		list = append(list, h.roomJSONLocked(x.rm, cl.user, x.n, now))
	}
	cl.enqueue(frame(roomsFrame{T: "found", R: list}))
}

// roomInfo handles {"t":"rinfo","c":"CODE"}: what a person needs to know before joining with an invite code.
func (h *hub) roomInfo(ctx context.Context, cl *client, in inbound) {
	code := strings.ToUpper(strings.TrimSpace(in.C))
	if len(code) != 8 {
		cl.enqueue(errFrame("no_such_room"))
		return
	}
	h.mu.Lock()
	var rm *room
	for _, r := range h.roomList {
		if r.community() && subtle.ConstantTimeCompare([]byte(r.invite), []byte(code)) == 1 {
			rm = r
		}
	}
	if rm == nil {
		h.mu.Unlock()
		cl.enqueue(errFrame("no_such_room"))
		return
	}
	j := h.roomJSONLocked(rm, cl.user, h.roomUsers(rm), time.Now())
	slug, owner := rm.slug, rm.ownerID == cl.user.id
	h.mu.Unlock()
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	member, err := h.st.isRoomMember(dctx, slug, cl.user.id)
	if err != nil {
		log.Printf("room lookup failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	j.Y = j.Y || member || owner
	cl.enqueue(frame(map[string]any{"t": "rinfo", "s": j.S, "n": j.N, "o": j.O, "f": j.F, "w": j.W, "k": j.K, "y": j.Y, "c": code}))
}

func (h *hub) roomPwAllowed(ou *onlineUser, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	keep := ou.roomFails[:0]
	for _, t := range ou.roomFails {
		if t.After(now.Add(-roomPwWindow)) {
			keep = append(keep, t)
		}
	}
	ou.roomFails = keep
	return len(keep) < maxRoomPwFails
}

func (h *hub) roomPwFailed(ou *onlineUser, now time.Time) {
	h.mu.Lock()
	ou.roomFails = append(ou.roomFails, now)
	h.mu.Unlock()
}

// enter lets a client into a room. Official rooms only need the tier. A community room needs, in this order:
// not being kicked, then being the host, staff or an earlier member, or else the invite code (private rooms and
// rooms with a password), the password, and the entry fee the person agreed to. Anyone who gets in on a code,
// password or fee comes back later without them.
func (h *hub) enter(ctx context.Context, cl *client, slug string, last int64, code, pass string, agreed *int) {
	ou := cl.user
	h.mu.Lock()
	rm := h.rooms[slug]
	if rm == nil {
		h.mu.Unlock()
		cl.enqueue(errFrame("no_such_room"))
		return
	}
	if !rm.community() {
		h.mu.Unlock()
		h.joinRoom(cl, slug, last)
		return
	}
	kind, ownerID, passHash, fee, invite := rm.kind, rm.ownerID, rm.passHash, rm.fee, rm.invite
	staff := roleRank(ou.role) >= 2
	h.mu.Unlock()

	if staff || ownerID == ou.id {
		h.joinRoom(cl, slug, last)
		return
	}
	dctx, cancel := context.WithTimeout(ctx, 2*authWait)
	defer cancel()
	member, err := h.st.isRoomMember(dctx, slug, ou.id)
	if err != nil {
		log.Printf("room membership failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	if member {
		h.joinRoom(cl, slug, last)
		return
	}

	unlisted := kind == "private" || passHash != ""
	if unlisted && subtle.ConstantTimeCompare([]byte(strings.ToUpper(strings.TrimSpace(code))), []byte(invite)) != 1 {
		cl.enqueue(errFrame("needs_invite"))
		return
	}
	if passHash != "" {
		now := time.Now()
		if !h.roomPwAllowed(ou, now) {
			cl.enqueue(errFrame("too_many_attempts"))
			return
		}
		okPw, err := h.auth.verify(dctx, passHash, pass)
		if err != nil {
			cl.enqueue(errFrame("server_busy"))
			return
		}
		if !okPw {
			h.roomPwFailed(ou, now)
			cl.enqueue(errFrame("bad_room_password"))
			return
		}
	}
	if unlisted || fee > 0 {
		agreedFee := 0
		if agreed != nil {
			agreedFee = *agreed
		}
		bal, charged, hostID, _, err := h.st.enterRoom(dctx, slug, ou.id, agreedFee)
		switch {
		case errors.Is(err, errFeeChanged):
			cl.enqueue(errFrameN("fee_changed", fee))
			return
		case errors.Is(err, errNoFunds):
			cl.enqueue(errFrameN("no_credits", fee))
			return
		case errors.Is(err, errNotFound):
			cl.enqueue(errFrame("no_such_room"))
			return
		case err != nil:
			log.Printf("entering a room failed: %v", err)
			cl.enqueue(errFrame("server_busy"))
			return
		}
		h.mu.Lock()
		ou.addRoom(slug)
		h.mu.Unlock()
		if charged {
			h.setCredits(ou, bal)
		}
		if hostID != 0 {
			h.refreshCredits(dctx, hostID)
		}
	}
	h.joinRoom(cl, slug, last)
}

// ---- host tools ----

// hostRoom returns the room the client is in when they are its host (or staff) and it is a community room.
// The caller holds the lock.
func (h *hub) hostRoomLocked(cl *client) *room {
	rm := cl.room
	if rm == nil || !rm.community() {
		return nil
	}
	if rm.ownerID == cl.user.id || roleRank(cl.user.role) >= 2 {
		return rm
	}
	return nil
}

// roomSet handles {"t":"rset","f":10,"pw":"secret","k":"private"}. Fields that are left out stay as they are.
// An empty "pw" removes the password.
func (h *hub) roomSet(ctx context.Context, cl *client, in inbound) {
	if in.F != nil && (*in.F < 0 || *in.F > roomMaxFee) {
		cl.enqueue(errFrame("bad_fee"))
		return
	}
	if in.K != "" && in.K != "public" && in.K != "private" {
		cl.enqueue(errFrame("bad_room_kind"))
		return
	}
	if in.PW != nil && *in.PW != "" {
		if n := utf8.RuneCountInString(*in.PW); n < roomPassMin || n > roomPassMax {
			cl.enqueue(errFrame("bad_room_password"))
			return
		}
	}
	h.mu.Lock()
	rm := h.hostRoomLocked(cl)
	h.mu.Unlock()
	if rm == nil {
		cl.enqueue(errFrame("not_host"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, 2*authWait)
	defer cancel()
	var newHash *string
	if in.PW != nil {
		hash := ""
		if *in.PW != "" {
			var err error
			if hash, err = h.auth.hash(dctx, *in.PW); err != nil {
				cl.enqueue(errFrame("server_busy"))
				return
			}
		}
		newHash = &hash
	}
	var kind *string
	if in.K != "" {
		kind = &in.K
	}
	if err := h.st.updateRoomSettings(dctx, rm.slug, in.F, newHash, kind); err != nil {
		log.Printf("saving room settings failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	h.mu.Lock()
	if in.F != nil {
		rm.fee = *in.F
	}
	if newHash != nil {
		rm.passHash = *newHash
	}
	if kind != nil {
		rm.kind = *kind
	}
	out := map[string]any{"t": "rsetok", "f": rm.fee, "w": rm.passHash != "", "k": rm.kind}
	h.mu.Unlock()
	cl.enqueue(frame(out))
}

// hostMod handles {"t":"hmod","a":"mute|unmute|kick","h":"bob","m":10} for the host of the room the client is in.
// A mute or kick only applies in this room, lasts at most a day, and never touches staff or the host.
func (h *hub) hostMod(ctx context.Context, cl *client, in inbound) {
	if in.A != "mute" && in.A != "unmute" && in.A != "kick" {
		cl.enqueue(errFrame("bad_action"))
		return
	}
	mins := in.M
	if mins <= 0 {
		mins = 10
	}
	if mins > hostMuteMax {
		mins = hostMuteMax
	}
	h.mu.Lock()
	rm := h.hostRoomLocked(cl)
	h.mu.Unlock()
	if rm == nil {
		cl.enqueue(errFrame("not_host"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, authWait)
	defer cancel()
	target, ok := h.lookupUser(dctx, cl, in.H)
	if !ok {
		return
	}
	if target.ID == cl.user.id || target.ID == rm.ownerID || roleRank(target.Role) >= 2 {
		cl.enqueue(errFrame("forbidden"))
		return
	}
	until := time.Now().Add(time.Duration(mins) * time.Minute)
	h.mu.Lock()
	defer h.mu.Unlock()
	notice := frame(map[string]any{"t": "mod", "a": in.A, "h": target.Handle, "m": mins})
	switch in.A {
	case "mute":
		rm.muted[target.ID] = until
		if ou := h.users[target.ID]; ou != nil {
			f := frame(map[string]any{"t": "rmuted", "m": mins})
			for c := range ou.clients {
				c.enqueue(f)
			}
		}
	case "unmute":
		delete(rm.muted, target.ID)
	case "kick":
		rm.banned[target.ID] = until
		if ou := h.users[target.ID]; ou != nil {
			for c := range ou.clients {
				if c.room == rm {
					delete(rm.clients, c)
					c.room = nil
					c.enqueue(frame(map[string]any{"t": "rkicked", "m": mins, "n": rm.name}))
					c.enqueue(h.roomsFrameForLocked(ou))
				}
			}
		}
	}
	for c := range rm.clients {
		c.enqueue(notice)
	}
	cl.enqueue(frame(map[string]any{"t": "modok", "a": in.A}))
}

// dropRoom removes a community room from memory and moves everyone in it to the hub.
// The caller has already deleted it from the database.
func (h *hub) dropRoomLocked(rm *room, why string) {
	for c := range rm.clients {
		delete(rm.clients, c)
		c.room = nil
		c.enqueue(frame(map[string]string{"t": why, "s": rm.slug, "n": rm.name}))
		c.enqueue(h.roomsFrameForLocked(c.user))
	}
	delete(h.rooms, rm.slug)
	delete(h.lastCount, rm.slug)
	for i, r := range h.roomList {
		if r == rm {
			h.roomList = append(h.roomList[:i], h.roomList[i+1:]...)
			break
		}
	}
	for c := range h.clients {
		if c.room == nil {
			c.enqueue(h.roomsFrameForLocked(c.user))
		}
	}
}

// roomClose handles {"t":"rclose"} for the host of the room the client is in. Staff can close any community room.
func (h *hub) roomClose(ctx context.Context, cl *client, in inbound) {
	h.mu.Lock()
	rm := h.hostRoomLocked(cl)
	if in.R != "" && roleRank(cl.user.role) >= 2 {
		if r := h.rooms[in.R]; r != nil && r.community() {
			rm = r
		}
	}
	h.mu.Unlock()
	if rm == nil {
		cl.enqueue(errFrame("not_host"))
		return
	}
	dctx, cancel := context.WithTimeout(ctx, 2*authWait)
	defer cancel()
	if err := h.st.deleteRoom(dctx, rm.slug); err != nil {
		log.Printf("closing a room failed: %v", err)
		cl.enqueue(errFrame("server_busy"))
		return
	}
	h.mu.Lock()
	h.dropRoomLocked(rm, "rclosed")
	h.mu.Unlock()
	if roleRank(cl.user.role) >= 2 && rm.ownerID != cl.user.id {
		h.st.logMod(ctx, cl.user.handle, "close_room", rm.ownerHandle, rm.name)
	}
	cl.enqueue(frame(map[string]any{"t": "modok", "a": "close"}))
}

// maintainRooms saves which rooms were active and closes community rooms that nobody has used for a few days.
func (h *hub) maintainRooms(ctx context.Context) {
	now := time.Now()
	active := map[string]time.Time{}
	var idle []string
	h.mu.Lock()
	for _, rm := range h.roomList {
		if !rm.community() {
			continue
		}
		if rm.dirty {
			active[rm.slug] = rm.lastActive
			rm.dirty = false
		}
		if len(rm.clients) == 0 && now.Sub(rm.lastActive) > h.roomIdle {
			idle = append(idle, rm.slug)
		}
	}
	h.mu.Unlock()
	if err := h.st.touchRooms(ctx, active); err != nil {
		log.Printf("saving room activity failed: %v", err)
	}
	for _, slug := range idle {
		if err := h.st.deleteRoom(ctx, slug); err != nil {
			log.Printf("closing an idle room failed: %v", err)
			continue
		}
		h.mu.Lock()
		if rm := h.rooms[slug]; rm != nil && len(rm.clients) == 0 {
			h.dropRoomLocked(rm, "rclosed")
		}
		h.mu.Unlock()
		log.Printf("closed idle room %s", slug)
	}
}
