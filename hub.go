package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	bufferSize    = 200             // messages kept in memory for the room, also the limit for replying to older messages
	snippetRunes  = 80              // how much of the original message a reply carries
	historyOnJoin = 30              // messages sent to a brand new joiner
	maxMsgRunes   = 300             // longest message, in characters
	sendQueueSize = 64              // frames waiting to go out per connection
	burstMessages = 5.0             // rate limit: bucket size
	refillPerSec  = 1.0             // rate limit: messages regained per second
	idGap         = 1000            // after a restart, ids skip ahead so a crash can never repeat an id a client saw
	msgQueueSize  = 4096            // messages waiting to be saved to the database
	heatWindow    = 5 * time.Minute // heat counts messages sent in this long
)

var handleRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)

// ---- wire format (short keys on purpose, to save data) ----

type chatMsg struct {
	ID     int64  `json:"i"`
	Handle string `json:"h"`
	Body   string `json:"b"`
	TS     int64  `json:"ts"`
	Tier   int    `json:"r"`
	Role   string `json:"o,omitempty"` // mod or admin, so everyone can tell staff from members
	// Set only on replies: the id, sender and a short snippet of the message replied to.
	ReplyTo     int64  `json:"q,omitempty"`
	ReplyHandle string `json:"qh,omitempty"`
	ReplySnip   string `json:"qb,omitempty"`
	// Set only on tip cards: who was tipped and how many credits. The body is the name of the treat.
	TipTo  string `json:"tt,omitempty"`
	TipAmt int    `json:"ta,omitempty"`
	Av     string `json:"av,omitempty"` // the sender's avatar look, like "f3"
}

type msgFrame struct {
	T string `json:"t"`
	chatMsg
}

type histFrame struct {
	T string    `json:"t"`
	M []chatMsg `json:"m"`
}

type countFrame struct {
	T string `json:"t"`
	C int    `json:"c"`
}

type upFrame struct {
	T string `json:"t"`
	H string `json:"h"`
	R int    `json:"r"`
}

type roomJSON struct {
	S string   `json:"s"`           // slug
	N string   `json:"n"`           // name
	C string   `json:"c"`           // category
	G []string `json:"g"`           // tags
	U int      `json:"u"`           // people in the room now
	H int      `json:"h"`           // heat, 0 to 5
	T int      `json:"t,omitempty"` // tier needed to enter, left out when anyone can

	// rooms people create (all left out for official rooms)
	K string `json:"k,omitempty"` // kind: public or private
	F int    `json:"f,omitempty"` // entry fee in credits
	W bool   `json:"w,omitempty"` // has a password
	O string `json:"o,omitempty"` // host handle
	Y bool   `json:"y,omitempty"` // I am the host or already a member: no code, password or fee needed
}

type roomsFrame struct {
	T string     `json:"t"`
	R []roomJSON `json:"r"`
}

type hubRow struct {
	S string `json:"s"`
	U int    `json:"u"`
	H int    `json:"h"`
}

type hubFrame struct {
	T string   `json:"t"`
	R []hubRow `json:"r"`
}

type joinedFrame struct {
	T string `json:"t"`
	R string `json:"r"`
	N string `json:"n"`
	K string `json:"k,omitempty"` // kind of a community room
	F int    `json:"f,omitempty"` // entry fee
	W bool   `json:"w,omitempty"` // has a password
	O string `json:"o,omitempty"` // host handle
	Y bool   `json:"y,omitempty"` // I am the host
	I string `json:"i,omitempty"` // invite code, host only
}

// meFields is what a user is told about their own progress.
type meFields struct {
	Points  int64  `json:"p"`
	Tier    int    `json:"r"`
	Next    int64  `json:"x"` // points for the next tier, 0 at the top
	Streak  int    `json:"s"`
	HasPw   bool   `json:"g"`             // true once the handle is protected by a password
	Role    string `json:"o,omitempty"`   // mod or admin, left out for normal users
	Credits int64  `json:"c"`             // wallet balance
	Avatar  string `json:"v,omitempty"`   // the look I picked, left out when none
	Bio     string `json:"bio,omitempty"` // my bio, left out when empty
}

type meFrame struct {
	T string `json:"t"`
	meFields
}

type okFrame struct {
	T string `json:"t"`
	H string `json:"h"`
	K string `json:"k"`
	meFields
}

// inbound is anything a client can send:
//
//	{"t":"hi","h":"ada","p":"pw","v":"f3"} first frame, new account (the password is required unless REQUIRE_PASSWORD=0)
//	{"t":"hi","k":"<token>"}          first frame, returning user
//	{"t":"login","h":"ada","p":"..."} first frame, user with a password
//	  hi and login may add "r":"football","l":42 to enter that room straight away (l = last message id seen there)
//	{"t":"join","r":"football","l":42} enter a room, leaving the one you are in
//	{"t":"leave"}                     back to the hub
//	{"t":"m","b":"hello"}             chat message in your room
//	{"t":"m","b":"yes","q":42}        reply to message 42
//	{"t":"pw","p":"..."}              protect this handle with a password
//	{"t":"p"}                         ping
//	{"t":"bye"}                       log out: ends this login
//	{"t":"rep","i":42,"c":"spam"}     report message 42 (spam, abuse or other)
//	{"t":"del","i":42}                delete your own message 42
//	{"t":"wallet"}                    your credit balance and latest ledger entries
//	{"t":"dm","h":"bob","b":"hi"}     direct message (1 credit unless you are friends)
//	{"t":"thread","h":"bob"}          open a conversation, marks it read
//	{"t":"read","h":"bob"}            mark a conversation read
//	{"t":"inbox"}                     chats, friends, requests and blocked people
//	{"t":"fr","a":"add","h":"bob"}    friend request: add, accept, decline, cancel, remove
//	{"t":"blk","a":"add","h":"bob"}   block or unblock (a = add, del)
//	{"t":"drep","i":42,"c":"abuse"}   report a direct message you received
//	{"t":"reports"}                   moderators: list open reports
//	{"t":"mod","a":"mute","h":"bob","m":10,"rid":7}   moderators: mute, kick, unmute, del, dismiss, promote, demote
type inbound struct {
	T string `json:"t"`
	H string `json:"h"`
	K string `json:"k"`
	L int64  `json:"l"`
	B string `json:"b"`
	P string `json:"p"`
	Q int64  `json:"q"`
	R string `json:"r"` // room

	V string `json:"v"` // avatar look, like "f3"

	// rooms people create
	Amt int     `json:"amt"` // tip amount in credits
	N   string  `json:"n"`   // room name, or words to search for
	G   string  `json:"g"`   // tags
	F   *int    `json:"f"`   // entry fee, a pointer so that 0 and "not given" differ
	PW  *string `json:"pw"`  // new room password: "" removes it, left out keeps it

	// moderation
	A   string `json:"a"`   // action
	M   int    `json:"m"`   // minutes for a mute or kick
	I   int64  `json:"i"`   // message id to report or delete
	C   string `json:"c"`   // report category
	RID int64  `json:"rid"` // report this action settles
	ID  int64  `json:"id"`  // report id to dismiss

	// notifications, settings, password change
	All bool   `json:"all"` // mark every notification read
	O   string `json:"o"`   // current password, for a password change
}

func frame(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func errFrame(code string) []byte { return frame(map[string]string{"t": "err", "c": code}) }

// errFrameN is an error that carries a number, for example the minutes left on a mute.
func errFrameN(code string, n int) []byte {
	return frame(map[string]any{"t": "err", "c": code, "m": n})
}

var pongFrame = frame(map[string]string{"t": "pong"})

// ---- state ----

type onlineUser struct {
	id          int64
	handle      string
	hasPass     bool
	points      int64
	tier        int
	streakDays  int
	streakDay   int
	dirty       bool // points changed and are not saved yet
	clients     map[*client]struct{}
	role        string    // user, mod or admin
	mutedUntil  time.Time // zero when not muted
	credits     int64     // wallet balance, the database is the source of truth
	avatar      string    // the look picked at sign up
	bio         string    // a short line about me
	prefs       prefs     // my settings
	prefsLoaded bool
	rooms       map[string]struct{} // rooms I own or have been let into
	creating    bool                // a room creation is in progress, so two taps make one room
	roomFails   []time.Time         // wrong room passwords lately

	reportTimes []time.Time // when this user filed reports in the last hour
	tokens      float64     // rate limit bucket, shared by all of the user's tabs
	lastRefill  time.Time
	hourStart   time.Time
	hourPoints  int64
	lastBody    string
	lastBodyAt  time.Time
}

func (u *onlineUser) me() meFields {
	role := ""
	if u.role != "user" {
		role = u.role
	}
	return meFields{Points: u.points, Tier: u.tier, Next: nextTierAt(u.tier), Streak: u.streakDays, HasPw: u.hasPass, Role: role, Credits: u.credits, Avatar: u.avatar, Bio: u.bio}
}

type client struct {
	send   chan []byte
	cancel context.CancelFunc
	user   *onlineUser
	room   *room  // nil while the person is on the hub
	tokenH string // hash of the login this connection used, so it can be ended on log out
}

// room is one chat room. Its state is guarded by the hub lock.
type room struct {
	slug, name, category string
	tags                 []string
	minTier              int
	buf                  []chatMsg   // recent messages, oldest first
	times                []time.Time // when recent messages were sent, for the heat level
	clients              map[*client]struct{}

	// rooms people create
	kind        string // official, public or private
	ownerID     int64
	ownerHandle string
	passHash    string
	fee         int
	invite      string
	lastActive  time.Time
	dirty       bool                // lastActive changed and is not saved yet
	muted       map[int64]time.Time // muted by the host, until
	banned      map[int64]time.Time // kicked by the host, until
}

// official rooms are yours. Everything else was created by a person.
func (rm *room) community() bool { return rm.kind != "official" }

// listed rooms show on the hub while people are in them. Private rooms and rooms with a password never do.
func (rm *room) listed() bool { return rm.kind == "public" && rm.passHash == "" }

// heatLevel turns the number of messages in the last few minutes into 0 to 5 flames.
func heatLevel(n int) int {
	switch {
	case n <= 0:
		return 0
	case n < 5:
		return 1
	case n < 15:
		return 2
	case n < 40:
		return 3
	case n < 100:
		return 4
	}
	return 5
}

func (rm *room) heat(now time.Time) int {
	cut := now.Add(-heatWindow)
	i := 0
	for i < len(rm.times) && rm.times[i].Before(cut) {
		i++
	}
	rm.times = rm.times[i:]
	return heatLevel(len(rm.times))
}

// enqueue never blocks. A connection that cannot keep up is dropped,
// and it comes back through the normal resume path.
func (c *client) enqueue(b []byte) {
	select {
	case c.send <- b:
	default:
		c.cancel()
	}
}

type hub struct {
	requirePass  bool // new accounts must set a password at sign up (REQUIRE_PASSWORD, default on)
	mu           sync.Mutex
	users        map[int64]*onlineUser // users connected now, or with points not saved yet
	clients      map[*client]struct{}
	onlineCount  int // users with at least one connection
	rooms        map[string]*room
	roomList     []*room       // in the order shown on the hub
	lastHub      []byte        // the last hub frame sent, so unchanged ones are skipped
	lastSig      string        // which community rooms were on the hub last time
	roomIdle     time.Duration // community rooms with no messages for this long are closed
	firstCleanup time.Duration // how long after start the first cleanup runs
	lastCount    map[string]int
	nextID       int64
	total        int64
	dropped      int64
	deleted      map[int64]struct{} // messages removed by a moderator, so they are never saved late

	st      *store
	auth    *authSvc
	limit   *loginLimiter
	origins []string
	msgQ    chan storedMsg

	flushEvery  time.Duration
	historyDays int
	guestDays   int
	dmDays      int // direct messages older than this are deleted
}

func newHub(st *store, auth *authSvc, origins []string, flushEvery time.Duration, historyDays, guestDays int) *hub {
	return &hub{
		users:        make(map[int64]*onlineUser),
		clients:      make(map[*client]struct{}),
		deleted:      make(map[int64]struct{}),
		rooms:        make(map[string]*room),
		lastCount:    make(map[string]int),
		st:           st,
		auth:         auth,
		limit:        newLoginLimiter(),
		origins:      origins,
		msgQ:         make(chan storedMsg, msgQueueSize),
		flushEvery:   flushEvery,
		historyDays:  historyDays,
		guestDays:    guestDays,
		dmDays:       30,
		roomIdle:     defaultRoomIdle,
		firstCleanup: time.Minute,
	}
}

// load creates the official rooms, restores each room's recent history and the message counter.
// newRoom builds the in-memory room for a row.
func newRoom(r roomRow) *room {
	rm := &room{slug: r.Slug, name: r.Name, category: r.Category, minTier: r.MinTier,
		buf: make([]chatMsg, 0, bufferSize), clients: make(map[*client]struct{}),
		kind: r.Kind, ownerID: r.OwnerID, ownerHandle: r.OwnerHandle, passHash: r.PassHash, fee: r.Fee, invite: r.Invite,
		lastActive: r.LastActive, muted: make(map[int64]time.Time), banned: make(map[int64]time.Time)}
	if rm.kind == "" {
		rm.kind = "official"
	}
	for _, t := range strings.Split(r.Tags, ",") {
		if t = strings.TrimSpace(t); t != "" {
			rm.tags = append(rm.tags, t)
		}
	}
	return rm
}

func (h *hub) load(ctx context.Context) error {
	if err := h.st.seedRooms(ctx, defaultRooms); err != nil {
		return err
	}
	rows, err := h.st.listRooms(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		rm := newRoom(r)
		msgs, err := h.st.recentMessages(ctx, r.Slug, bufferSize)
		if err != nil {
			return err
		}
		rm.buf = append(rm.buf, msgs...)
		h.rooms[rm.slug] = rm
		h.roomList = append(h.roomList, rm)
	}
	maxID, err := h.st.maxMessageID(ctx)
	if err != nil {
		return err
	}
	if maxID > 0 {
		h.nextID = maxID + idGap
	}
	return nil
}

func normHandle(raw string) (handle, key string, ok bool) {
	handle = strings.TrimPrefix(strings.TrimSpace(raw), "@")
	if !handleRe.MatchString(handle) {
		return "", "", false
	}
	return handle, strings.ToLower(handle), true
}

// acquire returns the in-memory record for a user, creating it on first use.
// mute is when the user's current mute ends, zero if they are not muted.
func (h *hub) acquire(row userRow, mute time.Time) *onlineUser {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ou, ok := h.users[row.ID]; ok {
		if row.PassHash.Valid {
			ou.hasPass = true
		}
		return ou
	}
	ou := &onlineUser{
		id: row.ID, handle: row.Handle, hasPass: row.PassHash.Valid,
		points: row.Points, tier: tierFor(row.Points),
		streakDays: row.StreakDays, streakDay: row.StreakDay,
		clients: make(map[*client]struct{}),
		role:    row.Role, mutedUntil: mute, credits: row.Credits, avatar: row.Avatar, bio: row.Bio,
		tokens: burstMessages, lastRefill: time.Now(),
	}
	h.users[row.ID] = ou
	return ou
}

// history returns what a client should see on entering a room. Caller holds the lock.
func (h *hub) history(rm *room, last int64) []chatMsg {
	out := []chatMsg{}
	if last > 0 {
		for _, m := range rm.buf {
			if m.ID > last {
				out = append(out, m)
			}
		}
		return out
	}
	start := len(rm.buf) - historyOnJoin
	if start < 0 {
		start = 0
	}
	return append(out, rm.buf[start:]...)
}

// roomUsers counts the different people in a room. Caller holds the lock.
func (h *hub) roomUsers(rm *room) int {
	seen := make(map[*onlineUser]struct{}, len(rm.clients))
	for c := range rm.clients {
		seen[c.user] = struct{}{}
	}
	return len(seen)
}

// roomsFrameForLocked is the room list for one person. Official rooms are always in it. Community rooms are in it
// while people are inside (public ones without a password only), and always for the person who owns them or was let in.
// Caller holds the lock.
func (h *hub) roomsFrameForLocked(ou *onlineUser) []byte {
	now := time.Now()
	list := make([]roomJSON, 0, len(h.roomList))
	for _, rm := range h.roomList {
		n := h.roomUsers(rm)
		_, mine := ou.rooms[rm.slug]
		if rm.community() && !(rm.listed() && n > 0) && !mine {
			continue
		}
		list = append(list, h.roomJSONLocked(rm, ou, n, now))
	}
	return frame(roomsFrame{T: "rooms", R: list})
}

// roomJSONLocked describes one room for one person.
func (h *hub) roomJSONLocked(rm *room, ou *onlineUser, n int, now time.Time) roomJSON {
	j := roomJSON{S: rm.slug, N: rm.name, C: rm.category, G: rm.tags, U: n, H: rm.heat(now), T: rm.minTier}
	if rm.community() {
		_, mine := ou.rooms[rm.slug]
		j.K, j.F, j.W, j.O = rm.kind, rm.fee, rm.passHash != "", rm.ownerHandle
		j.Y = mine || rm.ownerID == ou.id || roleRank(ou.role) >= 2
	}
	return j
}

// communitySigLocked changes whenever the set of community rooms on the hub, or what shows about them, changes.
func (h *hub) communitySigLocked() string {
	var b strings.Builder
	for _, rm := range h.roomList {
		if rm.community() && rm.listed() && h.roomUsers(rm) > 0 {
			fmt.Fprintf(&b, "%s/%s/%d;", rm.slug, rm.name, rm.fee)
		}
	}
	return b.String()
}

// joinRoom puts a client in a room, leaving the one it was in.
// The history snapshot and the first live message are queued under one lock,
// so nothing is missed or doubled.
func (h *hub) joinRoom(cl *client, slug string, last int64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	rm := h.rooms[slug]
	if rm == nil {
		cl.enqueue(errFrame("no_such_room"))
		return
	}
	// Moderators and admins can enter every room. Members need the room's tier.
	if roleRank(cl.user.role) < 2 && cl.user.tier < rm.minTier {
		cl.enqueue(errFrameN("tier_locked", rm.minTier))
		return
	}
	if until, ok := rm.banned[cl.user.id]; ok {
		if time.Now().Before(until) {
			cl.enqueue(errFrameN("room_banned", minutesLeft(until)))
			return
		}
		delete(rm.banned, cl.user.id)
	}
	if cl.room != nil {
		delete(cl.room.clients, cl)
	}
	cl.room = rm
	rm.clients[cl] = struct{}{}
	jf := joinedFrame{T: "joined", R: rm.slug, N: rm.name}
	if rm.community() {
		jf.K, jf.F, jf.W, jf.O = rm.kind, rm.fee, rm.passHash != "", rm.ownerHandle
		if rm.ownerID == cl.user.id {
			jf.Y, jf.I = true, rm.invite // only the host gets the invite code
		}
	}
	cl.enqueue(frame(jf))
	cl.enqueue(frame(histFrame{T: "hist", M: h.history(rm, last)}))
	cl.enqueue(frame(countFrame{T: "n", C: h.roomUsers(rm)}))
}

// leaveRoom takes a client back to the hub.
func (h *hub) leaveRoom(cl *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cl.room != nil {
		delete(cl.room.clients, cl)
		cl.room = nil
	}
	cl.enqueue(frame(map[string]string{"t": "left"}))
	cl.enqueue(h.roomsFrameForLocked(cl.user))
}

// attach connects a client. It starts on the hub, unless the first frame named a room.
func (h *hub) attach(ou *onlineUser, cl *client, token string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	cl.user = ou
	cl.tokenH = tokenHash(token)
	if len(ou.clients) == 0 {
		h.onlineCount++
	}
	ou.clients[cl] = struct{}{}
	h.clients[cl] = struct{}{}
	cl.enqueue(frame(okFrame{T: "ok", H: ou.handle, K: token, meFields: ou.me()}))
	cl.enqueue(h.roomsFrameForLocked(ou))
}

func (h *hub) detach(cl *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, cl)
	if cl.room != nil {
		delete(cl.room.clients, cl)
		cl.room = nil
	}
	if ou := cl.user; ou != nil {
		if _, ok := ou.clients[cl]; ok {
			delete(ou.clients, cl)
			if len(ou.clients) == 0 {
				h.onlineCount--
			}
		}
	}
}

func clean(raw string) string {
	s := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, raw)
	return strings.TrimSpace(s)
}

// snippet shortens the original message for the quote shown in a reply.
func snippet(s string) string {
	r := []rune(s)
	if len(r) <= snippetRunes {
		return s
	}
	return string(r[:snippetRunes]) + "…"
}

// post validates a message, scores it, stores it and sends it to the room.
// replyTo is the id of the message being answered, or 0.
// It returns an error code, or "" on success.
func (h *hub) post(cl *client, raw string, replyTo int64) string {
	ou := cl.user
	body := clean(raw)
	if body == "" {
		return "empty"
	}
	if utf8.RuneCountInString(body) > maxMsgRunes {
		return "too_long"
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	rm := cl.room
	if rm == nil {
		return "no_room"
	}
	if time.Now().Before(ou.mutedUntil) {
		return "muted"
	}
	if until, ok := rm.muted[ou.id]; ok && time.Now().Before(until) {
		return "room_muted" // muted by the host of this room
	}

	var quoted *chatMsg
	if replyTo > 0 {
		quoted = findMsg(rm, replyTo)
		if quoted == nil || quoted.TipAmt > 0 { // tip cards can not be replied to
			return "reply_gone" // too old, or never existed
		}
	}

	now := time.Now()
	ou.tokens += now.Sub(ou.lastRefill).Seconds() * refillPerSec
	if ou.tokens > burstMessages {
		ou.tokens = burstMessages
	}
	ou.lastRefill = now
	if ou.tokens < 1 {
		return "slow_down"
	}
	ou.tokens--

	gain, bonus := ou.score(body, now)
	if bonus > 0 {
		// A bonus only happens on the first scoring message of a UTC day: credits follow it.
		go h.awardStreak(ou, int(now.UTC().Unix()/86400), ou.streakDays)
	}
	oldTier := ou.tier
	ou.tier = tierFor(ou.points)

	h.nextID++
	h.total++
	m := chatMsg{ID: h.nextID, Handle: ou.handle, Body: body, TS: now.UnixMilli(), Tier: ou.tier, Av: ou.avatar}
	if ou.role != "user" {
		m.Role = ou.role
	}
	if quoted != nil {
		m.ReplyTo, m.ReplyHandle, m.ReplySnip = quoted.ID, quoted.Handle, snippet(quoted.Body)
	}
	if len(rm.buf) == bufferSize {
		copy(rm.buf, rm.buf[1:])
		rm.buf = rm.buf[:bufferSize-1]
	}
	rm.buf = append(rm.buf, m)
	rm.lastActive, rm.dirty = now, true
	rm.heat(now) // drops old entries so the list stays short
	rm.times = append(rm.times, now)

	b := frame(msgFrame{T: "m", chatMsg: m})
	for c := range rm.clients {
		c.enqueue(b)
	}
	if ou.tier > oldTier {
		up := frame(upFrame{T: "up", H: ou.handle, R: ou.tier})
		for c := range rm.clients {
			c.enqueue(up)
		}
	}
	if gain+bonus > 0 {
		me := frame(meFrame{T: "me", meFields: ou.me()})
		for c := range ou.clients {
			c.enqueue(me)
		}
	}

	select {
	case h.msgQ <- storedMsg{ID: m.ID, UserID: ou.id, Handle: m.Handle, Tier: m.Tier, Body: m.Body, TS: m.TS,
		ReplyTo: m.ReplyTo, ReplyHandle: m.ReplyHandle, ReplySnip: m.ReplySnip, Room: rm.slug, Role: m.Role, Avatar: m.Av}:
	default:
		h.dropped++ // the database is far behind: the message is still delivered, just not kept
	}
	if strings.Contains(body, "@") || m.ReplyHandle != "" {
		h.notifyTags(rm, ou, m.ID, body, m.ReplyHandle)
	}
	return ""
}

// ---- background work ----

// persistLoop saves messages in batches so chat never waits for the database.
func (h *hub) persistLoop(ctx context.Context, done chan<- struct{}) {
	defer close(done)
	var batch []storedMsg
	flush := func(c context.Context) {
		if len(batch) == 0 {
			return
		}
		batch = h.withoutDeleted(batch)
		if len(batch) == 0 {
			return
		}
		fctx, cancel := context.WithTimeout(c, 10*time.Second)
		defer cancel()
		if err := h.st.insertMessages(fctx, batch); err != nil {
			log.Printf("saving %d messages failed, will retry: %v", len(batch), err)
			if len(batch) > 5000 {
				batch = batch[len(batch)-5000:]
			}
			return
		}
		batch = batch[:0]
	}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
loop:
	for {
		select {
		case m := <-h.msgQ:
			batch = append(batch, m)
			if len(batch) >= 200 {
				flush(ctx)
			}
		case <-tick.C:
			flush(ctx)
		case <-ctx.Done():
			break loop
		}
	}
	for {
		select {
		case m := <-h.msgQ:
			batch = append(batch, m)
			continue
		default:
		}
		break
	}
	flush(context.Background())
}

// flushUsers saves changed points and forgets users who left and are fully saved.
func (h *hub) flushUsers(ctx context.Context) {
	h.mu.Lock()
	var batch []userSave
	for _, ou := range h.users {
		if ou.dirty {
			batch = append(batch, userSave{ID: ou.id, Points: ou.points, StreakDays: ou.streakDays, StreakDay: ou.streakDay})
			ou.dirty = false
		}
	}
	h.mu.Unlock()

	if len(batch) > 0 {
		fctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := h.st.saveUsers(fctx, batch)
		cancel()
		if err != nil {
			log.Printf("saving %d users failed, will retry: %v", len(batch), err)
			h.mu.Lock()
			for _, b := range batch {
				if ou := h.users[b.ID]; ou != nil {
					ou.dirty = true
				}
			}
			h.mu.Unlock()
			return
		}
	}
	h.mu.Lock()
	for id, ou := range h.users {
		if len(ou.clients) == 0 && !ou.dirty {
			delete(h.users, id)
		}
	}
	h.mu.Unlock()
}

// run sends the online count when it changed, saves points, cleans up and logs stats.
func (h *hub) run(ctx context.Context) {
	count := time.NewTicker(2 * time.Second)
	flush := time.NewTicker(h.flushEvery)
	stats := time.NewTicker(time.Minute)
	prune := time.NewTicker(time.Hour)
	firstPrune := time.After(h.firstCleanup)
	defer count.Stop()
	defer flush.Stop()
	defer stats.Stop()
	defer prune.Stop()
	doPrune := func() {
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		if n, err := h.st.pruneNotes(pctx); err != nil {
			log.Printf("notification cleanup failed: %v", err)
		} else if n > 0 {
			log.Printf("removed %d old notifications", n)
		}
		m, g, s, err := h.st.prune(pctx, h.historyDays, h.guestDays)
		if err != nil {
			log.Printf("cleanup failed: %v", err)
			return
		}
		h.maintainRooms(pctx)
		d, derr := h.st.pruneDMs(pctx, h.dmDays)
		if derr != nil {
			log.Printf("direct message cleanup failed: %v", derr)
		}
		if m+g+s+d > 0 {
			log.Printf("cleanup removed messages=%d direct_messages=%d idle_guests=%d old_sessions=%d", m, d, g, s)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-count.C:
			h.tick()
		case <-flush.C:
			h.flushUsers(ctx)
		case <-stats.C:
			h.mu.Lock()
			log.Printf("stats online=%d connections=%d messages=%d not_saved=%d", h.onlineCount, len(h.clients), h.total, h.dropped)
			h.mu.Unlock()
		case <-firstPrune:
			doPrune()
		case <-prune.C:
			doPrune()
		}
	}
}

// findMsg looks for a message in a room's recent history. Caller holds the lock.
func findMsg(rm *room, id int64) *chatMsg {
	for i := len(rm.buf) - 1; i >= 0; i-- {
		if rm.buf[i].ID == id {
			m := rm.buf[i]
			return &m
		}
	}
	return nil
}

// tick tells each room how many people are in it, and tells people on the hub
// how busy every room is. Both are skipped when nothing changed.
func (h *hub) tick() {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	rows := make([]hubRow, 0, len(h.roomList))
	for _, rm := range h.roomList {
		n := h.roomUsers(rm)
		if n != h.lastCount[rm.slug] {
			h.lastCount[rm.slug] = n
			b := frame(countFrame{T: "n", C: n})
			for c := range rm.clients {
				c.enqueue(b)
			}
		}
		if !rm.community() || (rm.listed() && n > 0) {
			rows = append(rows, hubRow{S: rm.slug, U: n, H: rm.heat(now)})
		}
	}
	// When a community room appears on the hub or disappears from it, people on the hub get a fresh list.
	if sig := h.communitySigLocked(); sig != h.lastSig {
		h.lastSig = sig
		for c := range h.clients {
			if c.room == nil {
				c.enqueue(h.roomsFrameForLocked(c.user))
			}
		}
	}
	b := frame(hubFrame{T: "hub", R: rows})
	if bytes.Equal(b, h.lastHub) {
		return
	}
	h.lastHub = b
	for c := range h.clients {
		if c.room == nil {
			c.enqueue(b)
		}
	}
}
