package main

import (
	"math/rand"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// reservoir keeps a bounded random sample of durations, so memory stays flat at any run length.
type reservoir struct {
	mu   sync.Mutex
	n    int64
	max  int
	vals []float64 // milliseconds
}

func newReservoir(max int) *reservoir { return &reservoir{max: max} }

func (r *reservoir) add(d time.Duration) {
	ms := float64(d) / float64(time.Millisecond)
	r.mu.Lock()
	r.n++
	if len(r.vals) < r.max {
		r.vals = append(r.vals, ms)
	} else if j := rand.Int63n(r.n); j < int64(r.max) {
		r.vals[j] = ms
	}
	r.mu.Unlock()
}

type pct struct {
	N                  int64   `json:"samples"`
	P50, P95, P99, Max float64 `json:"-"`
	P50ms              float64 `json:"p50_ms"`
	P95ms              float64 `json:"p95_ms"`
	P99ms              float64 `json:"p99_ms"`
	Maxms              float64 `json:"max_ms"`
}

func (r *reservoir) summary() pct {
	r.mu.Lock()
	v := append([]float64(nil), r.vals...)
	n := r.n
	r.mu.Unlock()
	p := pct{N: n}
	if len(v) == 0 {
		return p
	}
	sort.Float64s(v)
	at := func(q float64) float64 { return v[int(q*float64(len(v)-1))] }
	p.P50ms, p.P95ms, p.P99ms, p.Maxms = at(0.50), at(0.95), at(0.99), v[len(v)-1]
	return p
}

// counters is everything counted during one stage's measured window.
type counters struct {
	connAttempts, connOK     atomic.Int64
	authFail                 atomic.Int64
	reconnAttempts, reconnOK atomic.Int64
	dropsInjected            atomic.Int64
	unexpectedClose          atomic.Int64
	msgSent, msgEchoed       atomic.Int64
	delivered, expected      atomic.Int64
	dmSent, dmOK             atomic.Int64
	queries, queriesOK       atomic.Int64
	joins, joinsOK, leaves   atomic.Int64
	serverBusy               atomic.Int64

	mu       sync.Mutex
	errCodes map[string]int64
	connErrs map[string]int64

	connectLat, authLat, deliverLat, echoLat, recoverLat *reservoir
}

func newCounters() *counters {
	return &counters{errCodes: map[string]int64{}, connErrs: map[string]int64{},
		connectLat: newReservoir(50000), authLat: newReservoir(50000), deliverLat: newReservoir(200000),
		echoLat: newReservoir(50000), recoverLat: newReservoir(20000)}
}

func (c *counters) errCode(code string) {
	if code == "server_busy" {
		c.serverBusy.Add(1)
	}
	c.mu.Lock()
	c.errCodes[code]++
	c.mu.Unlock()
}

func (c *counters) connErr(kind string) {
	c.mu.Lock()
	c.connErrs[kind]++
	c.mu.Unlock()
}
