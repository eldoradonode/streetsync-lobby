// Command loadtest drives the real STREETSYNC server over the real WebSocket protocol with many simulated people.
//
// It is built for a dedicated local or staging environment with a throwaway database. It refuses any target that is
// not explicitly isolated (see safety.go). Run it with -h for every setting. See README.md ("Load testing").
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
)

var live atomic.Int64 // bots with a working connection right now

type stageResult struct {
	Stage      int                `json:"stage"`
	Verdict    string             `json:"verdict"` // PASS, FAIL or INVALID
	Reasons    []string           `json:"reasons"`
	Window     string             `json:"window"`
	LiveAtEnd  int64              `json:"live_clients_at_end"`
	Ramp       map[string]any     `json:"ramp"`
	Metrics    map[string]any     `json:"metrics"`
	Resources  map[string]any     `json:"resources"`
	Bottleneck string             `json:"bottleneck"`
	ErrCodes   map[string]int64   `json:"error_codes"`
	ConnErrs   map[string]int64   `json:"connection_errors"`
	Latency    map[string]pct     `json:"latency"`
	Thresholds map[string]float64 `json:"thresholds"`
}

type config struct {
	URL, Out, DBURL, ServerPID, Confirm string
	Stages                              []int
	StageDur, Ramp                      time.Duration
	ConnectRate                         int
	Behaviour                           behaviour
	StopOnFail                          bool
	Th                                  map[string]float64
}

func main() {
	var (
		urlF     = flag.String("url", "", "WebSocket address of the server under test, for example ws://127.0.0.1:8080/ws (required)")
		stages   = flag.String("stages", "100,500,1000,3000", "comma separated client counts, run in order")
		smoke    = flag.Bool("smoke", false, "short smoke test: one stage of 100 clients for 20 seconds")
		dur      = flag.Duration("stage-duration", 60*time.Second, "measured time per stage, after the ramp")
		rate     = flag.Int("connect-rate", 50, "new connections per second while ramping up")
		msgEvery = flag.Duration("msg-interval", 10*time.Second, "mean time between messages per bot")
		dmEvery  = flag.Duration("dm-interval", 30*time.Second, "mean time between direct messages per bot")
		churn    = flag.Duration("churn-interval", 45*time.Second, "mean time between leaving a room and joining another")
		qEvery   = flag.Duration("query-interval", 20*time.Second, "mean time between people search, profile, wallet and inbox requests")
		drop     = flag.Float64("drop-per-min", 0.3, "chance per minute that a bot's connection is cut, then restored with its saved login")
		rooms    = flag.String("rooms", "lobby,abuja,lagos,campus,football,music,tech,side-hustle,creators", "rooms the bots use")
		out      = flag.String("out", "loadtest/results", "folder for the report files")
		dbURL    = flag.String("db-url", os.Getenv("LOADTEST_DATABASE_URL"), "PostgreSQL address of the test database, to record connection usage (and for -cleanup)")
		pid      = flag.String("server-pid", "", "process id of a local server, to record its CPU, memory and file descriptors")
		confirm  = flag.String("confirm-host", "", "confirm an allow-listed non-local host (see safety.go)")
		cleanup  = flag.Bool("cleanup", false, "delete the bot accounts left by earlier runs from the test database, then exit")
		runID    = flag.String("run-id", "", "limit -cleanup to one run, or name this run (4 lowercase letters or digits)")
		stop     = flag.Bool("stop-on-fail", true, "stop at the first stage that does not pass")
		seed     = flag.Int64("seed", 0, "random seed, for repeatable timing (0 = time)")
		thConn   = flag.Float64("min-connect-success", 0.99, "acceptance: share of connection attempts that succeed")
		thEcho   = flag.Float64("min-echo-success", 0.99, "acceptance: share of messages echoed back to their sender")
		thDeliv  = flag.Float64("min-delivery", 0.97, "acceptance: share of expected deliveries seen (approximate, see report)")
		thP95    = flag.Float64("max-p95-ms", 1000, "acceptance: message delivery p95 in milliseconds")
		thP99    = flag.Float64("max-p99-ms", 3000, "acceptance: message delivery p99 in milliseconds")
		thRec    = flag.Float64("min-reconnect-success", 0.98, "acceptance: share of reconnections that succeed")
		thBusy   = flag.Float64("max-busy-rate", 0.005, "acceptance: share of requests answered server_busy")
	)
	flag.Parse()
	if *seed != 0 {
		rand.Seed(*seed)
	}
	if *cleanup {
		runCleanup(*dbURL, *confirm, *runID)
		return
	}
	if *urlF == "" {
		log.Fatal("-url is required: say explicitly which environment to test. Nothing runs against a default.")
	}
	if err := checkTarget(*urlF, *confirm, "target"); err != nil {
		log.Fatalf("refused: %v", err)
	}
	if *dbURL != "" {
		if err := checkTarget(*dbURL, *confirm, "database"); err != nil {
			log.Fatalf("refused: %v", err)
		}
	}
	var st []int
	for _, s := range strings.Split(*stages, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || n <= 0 {
			log.Fatalf("bad stage %q", s)
		}
		st = append(st, n)
	}
	d := *dur
	if *smoke {
		st, d = []int{100}, 20*time.Second
	}
	id := *runID
	if id == "" {
		id = fmt.Sprintf("%04x", rand.Intn(1<<16))
	}
	cfg := &config{URL: *urlF, Out: *out, DBURL: *dbURL, ServerPID: *pid, Stages: st, StageDur: d, ConnectRate: *rate, StopOnFail: *stop,
		Th: map[string]float64{"min_connect_success": *thConn, "min_echo_success": *thEcho, "min_delivery": *thDeliv,
			"max_p95_ms": *thP95, "max_p99_ms": *thP99, "min_reconnect_success": *thRec, "max_busy_rate": *thBusy}}
	cfg.Behaviour = behaviour{URL: *urlF, Rooms: strings.Split(*rooms, ","), MsgEvery: *msgEvery, DMEvery: *dmEvery, ChurnEvery: *churn,
		QueryEvery: *qEvery, DropPerMin: *drop, RunID: id}
	nofile := raiseNoFile()
	maxStage := st[len(st)-1]
	if uint64(maxStage)+200 > nofile {
		log.Fatalf("the file descriptor limit is %d, too low for %d connections. Raise it (ulimit -n) and run again.", nofile, maxStage)
	}
	ctx, stopSig := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stopSig()
	runStages(ctx, cfg, id)
}

func runStages(ctx context.Context, cfg *config, id string) {
	var db *sql.DB
	if cfg.DBURL != "" {
		var err error
		if db, err = sql.Open("postgres", cfg.DBURL); err != nil {
			log.Fatal(err)
		}
		db.SetMaxOpenConns(1)
		defer db.Close()
		if err := db.Ping(); err != nil {
			log.Fatalf("database: %v", err)
		}
	}
	env := collectEnv(db, cfg.ServerPID)
	smp := startSampler(ctx, db, cfg.ServerPID)

	beh := &cfg.Behaviour
	beh.Stats = &atomic.Pointer[counters]{}
	beh.Stats.Store(newCounters())
	beh.Occupancy = map[string]*atomic.Int64{}
	for _, r := range beh.Rooms {
		beh.Occupancy[r] = &atomic.Int64{}
	}
	beh.TotalBots = &atomic.Int64{}
	handle := func(i int) string { return fmt.Sprintf("lt%s_%d", id, i) }
	beh.HandlePeers = func(self int) string {
		n := int(beh.TotalBots.Load())
		if n < 2 {
			return ""
		}
		return handle((self + 1 + rand.Intn(n-1)) % n)
	}
	started := time.Now()
	log.Printf("load test %s against %s, stages %v, %s measured per stage", id, cfg.URL, cfg.Stages, cfg.StageDur)
	var botsWG sync.WaitGroup
	botCtx, stopBots := context.WithCancel(ctx)
	launched := 0
	var results []stageResult
	for _, target := range cfg.Stages {
		if ctx.Err() != nil {
			break
		}
		ramp := newCounters()
		beh.Stats.Store(ramp)
		rampStart := time.Now()
		tick := time.NewTicker(time.Second / time.Duration(maxInt(cfg.ConnectRate, 1)))
		for launched < target && ctx.Err() == nil {
			<-tick.C
			b := &bot{id: launched, handle: handle(launched), b: beh}
			beh.TotalBots.Add(1)
			launched++
			botsWG.Add(1)
			go func() { defer botsWG.Done(); b.run(botCtx) }()
		}
		tick.Stop()
		// Let the last connections finish signing in before measuring.
		for i := 0; i < 60 && live.Load() < int64(float64(target)*0.97); i++ {
			time.Sleep(time.Second)
		}
		rampDur := time.Since(rampStart)
		win := newCounters()
		beh.Stats.Store(win)
		w0 := time.Now()
		log.Printf("stage %d: %d clients live after a %s ramp, measuring for %s", target, live.Load(), rampDur.Round(time.Second), cfg.StageDur)
		select {
		case <-time.After(cfg.StageDur):
		case <-ctx.Done():
		}
		w1 := time.Now()
		res := summarise(cfg, target, ramp, win, rampDur, w0, w1, smp)
		results = append(results, res)
		log.Printf("stage %d: %s %v", target, res.Verdict, res.Reasons)
		if cfg.StopOnFail && res.Verdict != "PASS" {
			break
		}
	}
	stopBots()
	done := make(chan struct{})
	go func() { botsWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
	}
	writeReport(cfg, id, env, results, started)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func runCleanup(dbURL, confirm, runID string) {
	if dbURL == "" {
		log.Fatal("-cleanup needs -db-url (or LOADTEST_DATABASE_URL) for the TEST database")
	}
	if err := checkTarget(dbURL, confirm, "database"); err != nil {
		log.Fatalf("refused: %v", err)
	}
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	pattern := `^lt[0-9a-f]{4}_[0-9]+$`
	if runID != "" {
		pattern = `^lt` + runID + `_[0-9]+$`
	}
	// Only never-passworded guests with the bot naming pattern are touched. The ledger rows go first (the key refuses otherwise).
	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}
	defer tx.Rollback()
	const ids = `SELECT id FROM users WHERE pass_hash IS NULL AND role = 'user' AND handle_key ~ $1`
	for _, q := range []string{
		`DELETE FROM credit_ledger WHERE user_id IN (` + ids + `)`,
		`DELETE FROM messages WHERE user_id IN (` + ids + `)`,
		`DELETE FROM users WHERE id IN (` + ids + `)`,
	} {
		res, err := tx.Exec(q, pattern)
		if err != nil {
			log.Fatalf("cleanup: %v", err)
		}
		n, _ := res.RowsAffected()
		fmt.Printf("%d rows: %s\n", n, strings.Fields(q)[2])
	}
	if err := tx.Commit(); err != nil {
		log.Fatal(err)
	}
}

func writeReport(cfg *config, id string, env envInfo, results []stageResult, started time.Time) {
	_ = os.MkdirAll(cfg.Out, 0o755)
	full := map[string]any{"run_id": id, "started": started.UTC().Format(time.RFC3339), "finished": time.Now().UTC().Format(time.RFC3339),
		"target": cfg.URL, "environment": env, "config": map[string]any{"stages": cfg.Stages, "stage_duration": cfg.StageDur.String(),
			"connect_rate_per_s": cfg.ConnectRate, "msg_interval": cfg.Behaviour.MsgEvery.String(), "dm_interval": cfg.Behaviour.DMEvery.String(),
			"churn_interval": cfg.Behaviour.ChurnEvery.String(), "query_interval": cfg.Behaviour.QueryEvery.String(),
			"drop_per_min": cfg.Behaviour.DropPerMin, "rooms": cfg.Behaviour.Rooms, "go_version": runtime.Version()},
		"thresholds": cfg.Th, "stages": results}
	b, _ := json.MarshalIndent(full, "", "  ")
	jp := filepath.Join(cfg.Out, "run-"+id+".json")
	_ = os.WriteFile(jp, b, 0o644)
	mp := filepath.Join(cfg.Out, "run-"+id+".md")
	_ = os.WriteFile(mp, []byte(markdown(cfg, id, env, results, started)), 0o644)
	fmt.Printf("\nreport written: %s and %s\n", mp, jp)
	fmt.Print(shortSummary(results))
}
