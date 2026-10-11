package main

import (
	"context"
	"encoding/json"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	roomName      = "lobby"
	bufferSize    = 50   // messages kept in memory for the room
	historyOnJoin = 30   // messages sent to a brand new joiner
	maxMsgRunes   = 300  // longest message, in characters
	sendQueueSize = 64   // frames waiting to go out per connection
	burstMessages = 5.0  // rate limit: bucket size
	refillPerSec  = 1.0  // rate limit: messages regained per second
	idGap         = 1000 // after a restart, ids skip ahead so a crash can never repeat an id a client saw
	msgQueueSize  = 4096 // messages waiting to be saved to the database
)

var handleRe = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)

// ---- wire format (short keys on purpose, to save data) ----

type chatMsg struct {
	ID     int64  `json:"i"`
	Handle string `json:"h"`
	Body   string `json:"b"`
	TS     int64  `json:"ts"`
	Tier   int    `json:"r"`
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

// meFields is what a user is told about their own progress.
type meFields struct {
	Points int64 `json:"p"`
	Tier   int   `json:"r"`
	Next   int64 `json:"x"` // points for the next tier, 0 at the top
	Streak int   `json:"s"`
	HasPw  bool  `json:"g"` // true once the handle is protected by a password
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
//	{"t":"hi","h":"ada"}              first frame, new guest
//	{"t":"hi","k":"<token>","l":42}   first frame, returning user (l = last message id seen)
//	{"t":"login","h":"ada","p":"..."} first frame, user with a password
//	{"t":"m","b":"hello"}             chat message
//	{"t":"pw","p":"..."}              protect this handle with a password
//	{"t":"p"}                         ping
type inbound struct {
	T string `json:"t"`
	H string `json:"h"`
	K string `json:"k"`
	L int64  `json:"l"`
	B string `json:"b"`
	P string `json:"p"`
}

func frame(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func errFrame(code string) []byte { return frame(map[string]string{"t": "err", "c": code}) }

var pongFrame = frame(map[string]string{"t": "pong"})

// ---- state ----

type onlineUser struct {
	id         int64
	handle     string
	hasPass    bool
	points     int64
	tier       int
	streakDays int
	streakDay  int
	dirty      bool // points changed and are not saved yet
	clients    map[*client]struct{}

	tokens     float64 // rate limit bucket, shared by all of the user's tabs
	lastRefill time.Time
	hourStart  time.Time
	hourPoints int64
	lastBody   string
	lastBodyAt time.Time
}

func (u *onlineUser) me() meFields {
	return meFields{Points: u.points, Tier: u.tier, Next: nextTierAt(u.tier), Streak: u.streakDays, HasPw: u.hasPass}
}

type client struct {
	send   chan []byte
	cancel context.CancelFunc
	user   *onlineUser
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
	mu          sync.Mutex
	users       map[int64]*onlineUser // users connected now, or with points not saved yet
	clients     map[*client]struct{}
	onlineCount int // users with at least one connection
	buf         []chatMsg
	nextID      int64
	total       int64
	dropped     int64

	st      *store
	auth    *authSvc
	limit   *loginLimiter
	origins []string
	msgQ    chan storedMsg

	flushEvery  time.Duration
	historyDays int
	guestDays   int
}

func newHub(st *store, auth *authSvc, origins []string, flushEvery time.Duration, historyDays, guestDays int) *hub {
	return &hub{
		users:       make(map[int64]*onlineUser),
		clients:     make(map[*client]struct{}),
		buf:         make([]chatMsg, 0, bufferSize),
		st:          st,
		auth:        auth,
		limit:       newLoginLimiter(),
		origins:     origins,
		msgQ:        make(chan storedMsg, msgQueueSize),
		flushEvery:  flushEvery,
		historyDays: historyDays,
		guestDays:   guestDays,
	}
}

// load restores the recent history and the message counter after a start.
func (h *hub) load(ctx context.Context) error {
	msgs, err := h.st.recentMessages(ctx, roomName, bufferSize)
	if err != nil {
		return err
	}
	maxID, err := h.st.maxMessageID(ctx)
	if err != nil {
		return err
	}
	h.buf = append(h.buf, msgs...)
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
func (h *hub) acquire(row userRow) *onlineUser {
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
		tokens:  burstMessages, lastRefill: time.Now(),
	}
	h.users[row.ID] = ou
	return ou
}

// history returns what a client should see on connect. Caller holds the lock.
func (h *hub) history(last int64) []chatMsg {
	out := []chatMsg{}
	if last > 0 {
		for _, m := range h.buf {
			if m.ID > last {
				out = append(out, m)
			}
		}
		return out
	}
	start := len(h.buf) - historyOnJoin
	if start < 0 {
		start = 0
	}
	return append(out, h.buf[start:]...)
}

// attach connects a client. The history snapshot and the first live message are
// queued under one lock, so nothing is missed or doubled.
func (h *hub) attach(ou *onlineUser, cl *client, last int64, token string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	cl.user = ou
	if len(ou.clients) == 0 {
		h.onlineCount++
	}
	ou.clients[cl] = struct{}{}
	h.clients[cl] = struct{}{}
	cl.enqueue(frame(okFrame{T: "ok", H: ou.handle, K: token, meFields: ou.me()}))
	cl.enqueue(frame(histFrame{T: "hist", M: h.history(last)}))
	cl.enqueue(frame(countFrame{T: "n", C: h.onlineCount}))
}

func (h *hub) detach(cl *client) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, cl)
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

// post validates a message, scores it, stores it and sends it to the room.
// It returns an error code, or "" on success.
func (h *hub) post(ou *onlineUser, raw string) string {
	body := clean(raw)
	if body == "" {
		return "empty"
	}
	if utf8.RuneCountInString(body) > maxMsgRunes {
		return "too_long"
	}
	h.mu.Lock()
	defer h.mu.Unlock()

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
	oldTier := ou.tier
	ou.tier = tierFor(ou.points)

	h.nextID++
	h.total++
	m := chatMsg{ID: h.nextID, Handle: ou.handle, Body: body, TS: now.UnixMilli(), Tier: ou.tier}
	if len(h.buf) == bufferSize {
		copy(h.buf, h.buf[1:])
		h.buf = h.buf[:bufferSize-1]
	}
	h.buf = append(h.buf, m)

	b := frame(msgFrame{T: "m", chatMsg: m})
	for cl := range h.clients {
		cl.enqueue(b)
	}
	if ou.tier > oldTier {
		up := frame(upFrame{T: "up", H: ou.handle, R: ou.tier})
		for cl := range h.clients {
			cl.enqueue(up)
		}
	}
	if gain+bonus > 0 {
		me := frame(meFrame{T: "me", meFields: ou.me()})
		for cl := range ou.clients {
			cl.enqueue(me)
		}
	}

	select {
	case h.msgQ <- storedMsg{ID: m.ID, UserID: ou.id, Handle: m.Handle, Tier: m.Tier, Body: m.Body, TS: m.TS}:
	default:
		h.dropped++ // the database is far behind: the message is still delivered, just not kept
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
		fctx, cancel := context.WithTimeout(c, 10*time.Second)
		defer cancel()
		if err := h.st.insertMessages(fctx, roomName, batch); err != nil {
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
	firstPrune := time.After(time.Minute)
	defer count.Stop()
	defer flush.Stop()
	defer stats.Stop()
	defer prune.Stop()
	last := -1
	doPrune := func() {
		pctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		m, g, s, err := h.st.prune(pctx, h.historyDays, h.guestDays)
		if err != nil {
			log.Printf("cleanup failed: %v", err)
			return
		}
		if m+g+s > 0 {
			log.Printf("cleanup removed messages=%d idle_guests=%d old_sessions=%d", m, g, s)
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-count.C:
			h.mu.Lock()
			if n := h.onlineCount; n != last {
				last = n
				b := frame(countFrame{T: "n", C: n})
				for cl := range h.clients {
					cl.enqueue(b)
				}
			}
			h.mu.Unlock()
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
