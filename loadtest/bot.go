package main

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	mrand "math/rand"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// A bot is one simulated person. It speaks the real wire protocol to the real server: it signs in as a guest,
// joins rooms, chats at human pace, leaves and rejoins, sends direct messages, looks people up, checks its wallet,
// and sometimes loses its connection and has to come back with its saved login.

type behaviour struct {
	URL         string
	Rooms       []string
	MsgEvery    time.Duration // mean time between room messages
	DMEvery     time.Duration
	ChurnEvery  time.Duration // leave and join another room
	QueryEvery  time.Duration // search, profile, wallet, inbox
	DropPerMin  float64       // chance per minute that the connection is cut
	RunID       string
	Stats       *atomic.Pointer[counters] // swapped when a stage's measured window starts
	Occupancy   map[string]*atomic.Int64  // bots in each room right now, to estimate expected deliveries
	TotalBots   *atomic.Int64
	HandlePeers func(self int) string
}

type bot struct {
	id     int
	handle string
	token  string
	room   string
	b      *behaviour
	conn   *websocket.Conn
	seq    int

	// End-to-end encryption: DMs now carry an envelope. The server can not tell a real ciphertext from random bytes of
	// the right shape, so the bots send real-shaped envelopes under a real registered public key. That exercises key
	// registration, lookup, validation and storage, which is what costs the server something.
	fp     string
	pubB64 string
	peers  sync.Map // handle (lower case) -> fingerprint
}

func (b *bot) ensureKey(ctx context.Context) {
	if b.fp == "" {
		k, err := ecdh.P256().GenerateKey(rand.Reader)
		if err != nil {
			return
		}
		raw := k.PublicKey().Bytes()
		sum := sha256.Sum256(raw)
		b.fp, b.pubB64 = hex.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(raw)
	}
	_ = b.send(ctx, map[string]any{"t": "ekey", "pub": b.pubB64})
}

func (b *bot) envelopeFor(peerFP string) map[string]any {
	n, c, m := make([]byte, 12), make([]byte, 48), make([]byte, 16)
	_, _ = rand.Read(n)
	_, _ = rand.Read(c)
	_, _ = rand.Read(m)
	return map[string]any{"v": 1, "n": base64.StdEncoding.EncodeToString(n), "c": base64.StdEncoding.EncodeToString(c),
		"mid": base64.RawURLEncoding.EncodeToString(m), "sf": b.fp, "rf": peerFP}
}

func jitter(d time.Duration) time.Duration {
	return time.Duration(float64(d) * (0.5 + mrand.Float64()))
}

func (b *bot) st() *counters { return b.b.Stats.Load() }

// connect dials and signs in (first time as a new guest, afterwards with the saved login). It returns after the "ok" frame.
func (b *bot) connect(ctx context.Context, reconnect bool) error {
	st := b.st()
	st.connAttempts.Add(1)
	if reconnect {
		st.reconnAttempts.Add(1)
	}
	t0 := time.Now()
	dctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	c, _, err := websocket.Dial(dctx, b.b.URL, nil)
	if err != nil {
		st.connErr("dial")
		return err
	}
	c.SetReadLimit(1 << 20)
	st.connectLat.add(time.Since(t0))
	hi := map[string]any{"t": "hi"}
	if b.token != "" {
		hi["k"] = b.token
		if b.room != "" {
			hi["r"] = b.room
		}
	} else {
		hi["h"] = b.handle
	}
	data, _ := json.Marshal(hi)
	a0 := time.Now()
	if err := c.Write(dctx, websocket.MessageText, data); err != nil {
		st.connErr("write_hi")
		_ = c.CloseNow()
		return err
	}
	for {
		_, msg, err := c.Read(dctx)
		if err != nil {
			st.connErr("auth_read")
			st.authFail.Add(1)
			_ = c.CloseNow()
			return err
		}
		switch {
		case bytes.Contains(msg, []byte(`"t":"err"`)):
			var e struct {
				C string `json:"c"`
			}
			_ = json.Unmarshal(msg, &e)
			st.errCode(e.C)
			st.authFail.Add(1)
			_ = c.CloseNow()
			return fmt.Errorf("auth refused: %s", e.C)
		case bytes.Contains(msg, []byte(`"t":"ok"`)):
			var ok struct {
				K string `json:"k"`
			}
			_ = json.Unmarshal(msg, &ok)
			b.token = ok.K
			st.authLat.add(time.Since(a0))
			st.connOK.Add(1)
			if reconnect {
				st.reconnOK.Add(1)
			}
			b.conn = c
			return nil
		}
	}
}

func (b *bot) send(ctx context.Context, m map[string]any) error {
	data, _ := json.Marshal(m)
	wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return b.conn.Write(wctx, websocket.MessageText, data)
}

// readLoop classifies frames cheaply (byte search) and only parses the rare ones it needs.
func (b *bot) readLoop(ctx context.Context, c *websocket.Conn, closed chan<- error) {
	selfDM := []byte(`"f":"` + b.handle + `"`)
	for {
		_, msg, err := c.Read(ctx)
		if err != nil {
			closed <- err
			return
		}
		st := b.st()
		switch {
		case bytes.Contains(msg, []byte(`"t":"m"`)):
			if i := bytes.Index(msg, []byte("LT~")); i >= 0 {
				rest := msg[i+3:]
				j := bytes.IndexByte(rest, '~')
				if j > 0 {
					// body is LT~<botid>~<unix micro>
					k := j + 1
					e := k
					for e < len(rest) && rest[e] >= '0' && rest[e] <= '9' {
						e++
					}
					sender, _ := strconv.Atoi(string(rest[:j]))
					us, _ := strconv.ParseInt(string(rest[k:e]), 10, 64)
					d := time.Since(time.UnixMicro(us))
					if sender == b.id {
						st.msgEchoed.Add(1)
						st.echoLat.add(d)
					} else {
						st.delivered.Add(1)
						st.deliverLat.add(d)
					}
				}
			}
		case bytes.Contains(msg, []byte(`"t":"err"`)):
			var e struct {
				C string `json:"c"`
			}
			_ = json.Unmarshal(msg, &e)
			st.errCode(e.C)
		case bytes.Contains(msg, []byte(`"t":"dm"`)):
			if bytes.Contains(msg, selfDM) {
				st.dmOK.Add(1)
			}
		case bytes.Contains(msg, []byte(`"t":"ekeyis"`)):
			var k struct {
				H  string `json:"h"`
				Fp string `json:"fp"`
			}
			if json.Unmarshal(msg, &k) == nil && k.Fp != "" {
				b.peers.Store(strings.ToLower(k.H), k.Fp)
			}
		case bytes.Contains(msg, []byte(`"t":"joined"`)):
			st.joinsOK.Add(1)
		case bytes.Contains(msg, []byte(`"t":"ufound"`)), bytes.Contains(msg, []byte(`"t":"uprofile"`)),
			bytes.Contains(msg, []byte(`"t":"wallet"`)), bytes.Contains(msg, []byte(`"t":"inbox"`)):
			st.queriesOK.Add(1)
		}
	}
}

func (b *bot) joinRoom(ctx context.Context, slug string) {
	occ := b.b.Occupancy
	if b.room != "" {
		if o := occ[b.room]; o != nil {
			o.Add(-1)
		}
	}
	b.st().joins.Add(1)
	b.room = slug
	if o := occ[slug]; o != nil {
		o.Add(1)
	}
	_ = b.send(ctx, map[string]any{"t": "join", "r": slug})
}

func (b *bot) leaveRoom(ctx context.Context) {
	if b.room == "" {
		return
	}
	if o := b.b.Occupancy[b.room]; o != nil {
		o.Add(-1)
	}
	b.room = ""
	b.st().leaves.Add(1)
	_ = b.send(ctx, map[string]any{"t": "leave"})
}

// run lives until the context ends. Every connection loss, injected or not, is followed by a reconnect.
func (b *bot) run(ctx context.Context) {
	reconnect := false
	var lostAt time.Time
	for ctx.Err() == nil {
		if err := b.connect(ctx, reconnect); err != nil {
			if ctx.Err() != nil {
				return
			}
			time.Sleep(jitter(2 * time.Second))
			continue
		}
		if reconnect {
			b.st().recoverLat.add(time.Since(lostAt))
		}
		reconnect = true
		live.Add(1)
		cctx, cancel := context.WithCancel(ctx)
		closed := make(chan error, 1)
		go b.readLoop(cctx, b.conn, closed)
		b.ensureKey(cctx) // same public key on every reconnect, so it is a no-op for the server after the first time
		if b.room == "" {
			b.joinRoom(cctx, b.b.Rooms[mrand.Intn(len(b.b.Rooms))])
		} else {
			// the hi frame carried the room, so only the bookkeeping continues
			if o := b.b.Occupancy[b.room]; o != nil {
				o.Add(1)
			}
		}
		intentional := b.session(cctx, closed)
		live.Add(-1)
		cancel()
		if b.room != "" {
			if o := b.b.Occupancy[b.room]; o != nil {
				o.Add(-1)
			}
		}
		if b.conn != nil {
			_ = b.conn.CloseNow()
		}
		lostAt = time.Now()
		if !intentional && ctx.Err() == nil {
			b.st().unexpectedClose.Add(1)
		}
		if ctx.Err() == nil {
			time.Sleep(jitter(500 * time.Millisecond))
		}
	}
}

// session is the bot's life on one connection. It returns true when the bot cut the connection on purpose.
func (b *bot) session(ctx context.Context, closed <-chan error) (intentional bool) {
	msgT := time.NewTimer(jitter(b.b.MsgEvery))
	dmT := time.NewTimer(jitter(b.b.DMEvery))
	churnT := time.NewTimer(jitter(b.b.ChurnEvery))
	qT := time.NewTimer(jitter(b.b.QueryEvery))
	dropT := time.NewTicker(10 * time.Second)
	defer msgT.Stop()
	defer dmT.Stop()
	defer churnT.Stop()
	defer qT.Stop()
	defer dropT.Stop()
	for {
		select {
		case <-ctx.Done():
			return true
		case <-closed:
			return false
		case <-dropT.C:
			if mrand.Float64() < b.b.DropPerMin/6 {
				b.st().dropsInjected.Add(1)
				_ = b.conn.CloseNow() // a dead network, not a polite goodbye
				return true
			}
		case <-msgT.C:
			if b.room != "" {
				b.seq++
				st := b.st()
				st.msgSent.Add(1)
				if o := b.b.Occupancy[b.room]; o != nil {
					if n := o.Load() - 1; n > 0 {
						st.expected.Add(n)
					}
				}
				body := fmt.Sprintf("LT~%d~%d", b.id, time.Now().UnixMicro())
				if b.send(ctx, map[string]any{"t": "m", "b": body}) != nil {
					return false
				}
			}
			msgT.Reset(jitter(b.b.MsgEvery))
		case <-dmT.C:
			if peer := b.b.HandlePeers(b.id); peer != "" {
				if fp, ok := b.peers.Load(strings.ToLower(peer)); !ok {
					if b.send(ctx, map[string]any{"t": "ekeyget", "h": peer}) != nil { // learn their key first
						return false
					}
				} else {
					b.st().dmSent.Add(1)
					if b.send(ctx, map[string]any{"t": "dm", "h": peer, "e": b.envelopeFor(fp.(string))}) != nil {
						return false
					}
				}
			}
			dmT.Reset(jitter(b.b.DMEvery))
		case <-churnT.C:
			b.leaveRoom(ctx)
			time.Sleep(time.Duration(mrand.Intn(300)) * time.Millisecond)
			b.joinRoom(ctx, b.b.Rooms[mrand.Intn(len(b.b.Rooms))])
			churnT.Reset(jitter(b.b.ChurnEvery))
		case <-qT.C:
			st := b.st()
			st.queries.Add(1)
			var m map[string]any
			switch mrand.Intn(4) {
			case 0:
				m = map[string]any{"t": "usearch", "n": "lt" + b.b.RunID}
			case 1:
				if p := b.b.HandlePeers(b.id); p != "" {
					m = map[string]any{"t": "profile", "h": p}
				} else {
					m = map[string]any{"t": "wallet"}
				}
			case 2:
				m = map[string]any{"t": "wallet"}
			default:
				m = map[string]any{"t": "inbox"}
			}
			if b.send(ctx, m) != nil {
				return false
			}
			qT.Reset(jitter(b.b.QueryEvery))
		}
	}
}
