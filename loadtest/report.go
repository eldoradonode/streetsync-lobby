package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ---- sampler: server, generator, host and database readings every two seconds ----

type sample struct {
	At                  time.Time
	Srv, Self           procSample
	SrvOK               bool
	Lag                 time.Duration // how late the sampler's own timer fired: a busy generator fires late
	Goroutines          int
	HostBusy, HostTotal float64
	PGConns, PGActive   int
	PGWait              int
	Rollbacks, Deadlock int64
	HasPG               bool
}

type sampler struct {
	mu      sync.Mutex
	samples []sample
}

func hostJiffies() (busy, total float64) {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return
	}
	f := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
	if len(f) < 8 {
		return
	}
	var v [8]float64
	for i := 0; i < 8; i++ {
		v[i], _ = strconv.ParseFloat(f[i+1], 64)
		total += v[i]
	}
	busy = total - v[3] - v[4] // not idle, not waiting for disk
	return
}

func startSampler(ctx context.Context, db *sql.DB, serverPID string) *sampler {
	s := &sampler{}
	go func() {
		const every = 2 * time.Second
		next := time.Now().Add(every)
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Until(next)):
			}
			lag := time.Since(next)
			next = next.Add(every)
			sm := sample{At: time.Now(), Lag: lag, Goroutines: runtime.NumGoroutine()}
			sm.Self, _ = readProc("self")
			if serverPID != "" {
				var err error
				if sm.Srv, err = readProc(serverPID); err == nil {
					sm.SrvOK = true
				}
			}
			sm.HostBusy, sm.HostTotal = hostJiffies()
			if db != nil {
				qctx, cancel := context.WithTimeout(ctx, 3*time.Second)
				err := db.QueryRowContext(qctx,
					`SELECT count(*), count(*) FILTER (WHERE state = 'active'), count(*) FILTER (WHERE wait_event_type = 'Lock')
					 FROM pg_stat_activity WHERE datname = current_database()`).Scan(&sm.PGConns, &sm.PGActive, &sm.PGWait)
				if err == nil {
					err = db.QueryRowContext(qctx, `SELECT xact_rollback, deadlocks FROM pg_stat_database WHERE datname = current_database()`).Scan(&sm.Rollbacks, &sm.Deadlock)
				}
				cancel()
				sm.HasPG = err == nil
			}
			s.mu.Lock()
			s.samples = append(s.samples, sm)
			s.mu.Unlock()
		}
	}()
	return s
}

func (s *sampler) window(a, b time.Time) []sample {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []sample
	for _, x := range s.samples {
		if !x.At.Before(a) && !x.At.After(b) {
			out = append(out, x)
		}
	}
	return out
}

// ---- stage summary ----

func ratio(a, b int64) float64 {
	if b == 0 {
		return 1
	}
	return float64(a) / float64(b)
}

func r2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

func summarise(cfg *config, target int, ramp, win *counters, rampDur time.Duration, w0, w1 time.Time, smp *sampler) stageResult {
	th := cfg.Th
	res := stageResult{Stage: target, Thresholds: th, Window: w1.Sub(w0).Round(time.Second).String(), LiveAtEnd: live.Load()}
	win.mu.Lock()
	res.ErrCodes, res.ConnErrs = copyMap(win.errCodes), copyMap(win.connErrs)
	win.mu.Unlock()
	ramp.mu.Lock()
	rampErrs := copyMap(ramp.connErrs)
	rampCodes := copyMap(ramp.errCodes)
	ramp.mu.Unlock()
	res.Ramp = map[string]any{"duration": rampDur.Round(time.Millisecond).String(), "connect_attempts": ramp.connAttempts.Load(), "connect_ok": ramp.connOK.Load(),
		"connect_success": r2(ratio(ramp.connOK.Load(), ramp.connAttempts.Load())), "auth_failures": ramp.authFail.Load(), "connection_errors": rampErrs, "error_codes": rampCodes}

	reqs := win.msgSent.Load() + win.dmSent.Load() + win.queries.Load() + win.joins.Load()
	connS := ratio(win.connOK.Load(), win.connAttempts.Load())
	echoS := ratio(win.msgEchoed.Load(), win.msgSent.Load())
	delivS := ratio(win.delivered.Load(), win.expected.Load())
	recS := ratio(win.reconnOK.Load(), win.reconnAttempts.Load())
	busy := ratio(win.serverBusy.Load(), reqs)
	res.Latency = map[string]pct{"message_delivery": win.deliverLat.summary(), "message_echo_roundtrip": win.echoLat.summary(),
		"connect": win.connectLat.summary(), "auth": win.authLat.summary(), "reconnect_recovery": win.recoverLat.summary()}
	res.Metrics = map[string]any{
		"connect_attempts": win.connAttempts.Load(), "connect_ok": win.connOK.Load(), "connect_success": r2(connS), "auth_failures": win.authFail.Load(),
		"messages_sent": win.msgSent.Load(), "messages_echoed": win.msgEchoed.Load(), "echo_success": r2(echoS),
		"deliveries_expected_approx": win.expected.Load(), "deliveries_seen": win.delivered.Load(), "delivery_ratio_approx": r2(delivS),
		"dms_sent": win.dmSent.Load(), "dms_confirmed": win.dmOK.Load(),
		"joins": win.joins.Load(), "joins_confirmed": win.joinsOK.Load(), "leaves": win.leaves.Load(),
		"queries": win.queries.Load(), "queries_confirmed": win.queriesOK.Load(),
		"drops_injected": win.dropsInjected.Load(), "reconnect_attempts": win.reconnAttempts.Load(), "reconnect_ok": win.reconnOK.Load(), "reconnect_success": r2(recS),
		"unexpected_disconnects": win.unexpectedClose.Load(), "server_busy": win.serverBusy.Load(), "server_busy_rate": r2(busy*1000) / 1000,
	}

	// Resources over the measured window.
	sm := smp.window(w0, w1)
	rs := map[string]any{"samples": len(sm)}
	var lagMS []float64
	var srvCPU, genCPU, hostPct float64
	alive := true
	if len(sm) >= 2 {
		first, last := sm[0], sm[len(sm)-1]
		wall := last.At.Sub(first.At).Seconds()
		genCPU = (last.Self.CPUSecs - first.Self.CPUSecs) / wall * 100
		if d := last.HostTotal - first.HostTotal; d > 0 {
			hostPct = (last.HostBusy - first.HostBusy) / d * 100
		}
		rs["generator_cpu_percent_of_one_core"] = r2(genCPU)
		rs["host_cpu_busy_percent_all_cores"] = r2(hostPct)
		rs["generator_rss_mb_start_end"] = []float64{r2(first.Self.RSSMB), r2(last.Self.RSSMB)}
		if first.SrvOK && last.SrvOK {
			srvCPU = (last.Srv.CPUSecs - first.Srv.CPUSecs) / wall * 100
			rs["server_cpu_percent_of_one_core"] = r2(srvCPU)
			rs["server_rss_mb_start_end"] = []float64{r2(first.Srv.RSSMB), r2(last.Srv.RSSMB)}
			rs["server_rss_growth_mb_per_min"] = r2((last.Srv.RSSMB - first.Srv.RSSMB) / wall * 60)
		}
		alive = last.SrvOK || cfg.ServerPID == ""
		var fdS, thS, gor, pgc, pga, pgw int
		var rssMaxS, rssMaxG float64
		for _, x := range sm {
			lagMS = append(lagMS, float64(x.Lag)/float64(time.Millisecond))
			fdS, thS, gor = maxI(fdS, x.Srv.FDs), maxI(thS, x.Srv.Threads), maxI(gor, x.Goroutines)
			rssMaxS, rssMaxG = maxF(rssMaxS, x.Srv.RSSMB), maxF(rssMaxG, x.Self.RSSMB)
			pgc, pga, pgw = maxI(pgc, x.PGConns), maxI(pga, x.PGActive), maxI(pgw, x.PGWait)
		}
		rs["server_rss_mb_max"], rs["generator_rss_mb_max"] = r2(rssMaxS), r2(rssMaxG)
		rs["server_fds_max"], rs["server_threads_max"], rs["generator_goroutines_max"] = fdS, thS, gor
		if first.HasPG && last.HasPG {
			rs["postgres_connections_max"], rs["postgres_active_max"], rs["postgres_lock_waiters_max"] = pgc, pga, pgw
			rs["postgres_rollbacks_in_window"], rs["postgres_deadlocks_in_window"] = last.Rollbacks-first.Rollbacks, last.Deadlock-first.Deadlock
		}
	}
	sort.Float64s(lagMS)
	lagP95 := 0.0
	if len(lagMS) > 0 {
		lagP95 = lagMS[int(0.95*float64(len(lagMS)-1))]
		rs["generator_scheduler_lag_p95_ms"], rs["generator_scheduler_lag_max_ms"] = r2(lagP95), r2(lagMS[len(lagMS)-1])
	}
	rs["server_process_alive_at_end"] = alive
	res.Resources = rs

	// Where did the time go? On one shared machine the two programs compete, so say so.
	switch {
	case lagP95 > 500:
		res.Bottleneck = fmt.Sprintf("load generator: its own timers fired %.0f ms late (p95), so latencies include generator delay", lagP95)
	case hostPct >= 90 && srvCPU > 0 && genCPU > srvCPU:
		res.Bottleneck = fmt.Sprintf("shared host CPU is saturated and the generator used more of it (%.0f%%) than the server (%.0f%%): treat as generator limited", genCPU, srvCPU)
	case hostPct >= 90:
		res.Bottleneck = fmt.Sprintf("shared host CPU is saturated (server %.0f%%, generator %.0f%% of one core): on one machine the two can not be separated cleanly; run the generator on another machine to confirm", srvCPU, genCPU)
	case win.serverBusy.Load() > 0:
		res.Bottleneck = "database: the server answered server_busy, which means its small connection pool or the database was too slow"
	default:
		res.Bottleneck = "none observed at this level"
	}

	// Verdict.
	var why []string
	fail := func(f string, a ...any) { why = append(why, fmt.Sprintf(f, a...)) }
	if connS < th["min_connect_success"] {
		fail("connect success %.3f < %.2f", connS, th["min_connect_success"])
	}
	if echoS < th["min_echo_success"] {
		fail("echo success %.3f < %.2f", echoS, th["min_echo_success"])
	}
	if delivS < th["min_delivery"] {
		fail("delivery %.3f < %.2f (approximate)", delivS, th["min_delivery"])
	}
	if p := res.Latency["message_delivery"]; p.N > 0 && p.P95ms > th["max_p95_ms"] {
		fail("delivery p95 %.0f ms > %.0f", p.P95ms, th["max_p95_ms"])
	} else if p.N > 0 && p.P99ms > th["max_p99_ms"] {
		fail("delivery p99 %.0f ms > %.0f", p.P99ms, th["max_p99_ms"])
	}
	if win.reconnAttempts.Load() >= 5 && recS < th["min_reconnect_success"] {
		fail("reconnect success %.3f < %.2f", recS, th["min_reconnect_success"])
	}
	if busy > th["max_busy_rate"] {
		fail("server_busy rate %.4f > %.4f", busy, th["max_busy_rate"])
	}
	if !alive {
		fail("the server process is gone")
	}
	if live.Load() < int64(float64(target)*0.95) {
		fail("only %d of %d clients are connected", live.Load(), target)
	}
	if rc := ramp.connOK.Load(); ratio(rc, ramp.connAttempts.Load()) < th["min_connect_success"] && ramp.connAttempts.Load() > 0 {
		fail("ramp connect success %.3f < %.2f", ratio(rc, ramp.connAttempts.Load()), th["min_connect_success"])
	}
	res.Verdict, res.Reasons = "PASS", []string{}
	if len(why) > 0 {
		res.Verdict, res.Reasons = "FAIL", why
	}
	if lagP95 > 500 {
		res.Verdict = "INVALID"
		res.Reasons = append([]string{"the load generator was saturated, so this stage does not measure the server"}, res.Reasons...)
	}
	return res
}

func copyMap(m map[string]int64) map[string]int64 {
	o := make(map[string]int64, len(m))
	for k, v := range m {
		o[k] = v
	}
	return o
}
func maxI(a, b int) int {
	if b > a {
		return b
	}
	return a
}
func maxF(a, b float64) float64 {
	if b > a {
		return b
	}
	return a
}

// ---- text output ----

func shortSummary(rs []stageResult) string {
	var b strings.Builder
	b.WriteString("\nstage  verdict  live   connect  echo   p50/p95/p99 ms        reconnect\n")
	best := 0
	for _, r := range rs {
		d := r.Latency["message_delivery"]
		fmt.Fprintf(&b, "%-6d %-8s %-6d %-8v %-6v %.0f/%.0f/%.0f%s%v\n", r.Stage, r.Verdict, r.LiveAtEnd, r.Metrics["connect_success"], r.Metrics["echo_success"],
			d.P50ms, d.P95ms, d.P99ms, strings.Repeat(" ", 6), r.Metrics["reconnect_success"])
		if r.Verdict == "PASS" {
			best = r.Stage
		}
	}
	if best == 0 {
		b.WriteString("\nNo stage met the acceptance criteria.\n")
	} else {
		fmt.Fprintf(&b, "\nHighest stage that met the acceptance criteria in this environment: %d concurrent clients.\n", best)
	}
	return b.String()
}

func markdown(cfg *config, id string, env envInfo, rs []stageResult, started time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Load test run %s\n\nStarted %s. Target `%s`.\n\n", id, started.UTC().Format(time.RFC3339), cfg.URL)
	b.WriteString("## Environment\n\n")
	fmt.Fprintf(&b, "- Generator: %s on %s, %d CPUs, GOMAXPROCS %d, file limit %d\n- cgroup cpu.max `%s`, memory.max `%s`, host memory %s\n- Server: %s\n- Database: %s, max_connections %s, shared_buffers %s\n\n",
		env.Go, env.OSArch, env.CPUs, env.GoMaxProc, env.NoFile, env.CgroupCPU, env.CgroupMem, env.MemTotal, env.Server, env.DBVersion, env.DBMaxConn, env.DBShared)
	b.WriteString("## Configuration\n\n")
	fmt.Fprintf(&b, "Stages %v, measured window %s per stage, ramp %d connections/s, message every ~%s, DM every ~%s, room change every ~%s, lookups every ~%s, %.2f connection drops per bot per minute, rooms %v.\n\n",
		cfg.Stages, cfg.StageDur, cfg.ConnectRate, cfg.Behaviour.MsgEvery, cfg.Behaviour.DMEvery, cfg.Behaviour.ChurnEvery, cfg.Behaviour.QueryEvery, cfg.Behaviour.DropPerMin, cfg.Behaviour.Rooms)
	b.WriteString("Acceptance criteria: ")
	keys := make([]string, 0, len(cfg.Th))
	for k := range cfg.Th {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(&b, "`%s=%v` ", k, cfg.Th[k])
	}
	b.WriteString("\n\n## Results\n\n| Stage | Verdict | Live | Connect | Echo | Delivery* | p50 / p95 / p99 ms | Reconnect | Server CPU % | Server RSS MB | Gen CPU % | PG conns |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	best := 0
	for _, r := range rs {
		d := r.Latency["message_delivery"]
		rss := "n/a"
		if v, ok := r.Resources["server_rss_mb_max"]; ok {
			rss = fmt.Sprint(v)
		}
		fmt.Fprintf(&b, "| %d | **%s** | %d | %v | %v | %v | %.0f / %.0f / %.0f | %v | %v | %s | %v | %v |\n", r.Stage, r.Verdict, r.LiveAtEnd, r.Metrics["connect_success"], r.Metrics["echo_success"],
			r.Metrics["delivery_ratio_approx"], d.P50ms, d.P95ms, d.P99ms, r.Metrics["reconnect_success"], r.Resources["server_cpu_percent_of_one_core"], rss,
			r.Resources["generator_cpu_percent_of_one_core"], r.Resources["postgres_connections_max"])
		if r.Verdict == "PASS" {
			best = r.Stage
		}
	}
	b.WriteString("\n\\* Delivery is approximate: the generator estimates how many bots were in a room when a message was sent. Latency is measured on the generator's clock from the send time in the message to the arrival time, so it is only meaningful when generator and server share a machine or have synchronised clocks.\n\n")
	if best == 0 {
		b.WriteString("**No stage met the acceptance criteria.**\n\n")
	} else {
		fmt.Fprintf(&b, "**Highest stage that met the acceptance criteria in this environment: %d concurrent clients.** This says nothing about other hardware or a different network.\n\n", best)
	}
	for _, r := range rs {
		fmt.Fprintf(&b, "### Stage %d: %s\n\n- Reasons: %v\n- Bottleneck: %s\n- Ramp: %v\n- Error codes: %v, connection errors: %v\n- Resources: %v\n- Metrics: %v\n\n", r.Stage, r.Verdict, r.Reasons, r.Bottleneck, r.Ramp, r.ErrCodes, r.ConnErrs, r.Resources, r.Metrics)
		for _, k := range []string{"connect", "auth", "message_delivery", "message_echo_roundtrip", "reconnect_recovery"} {
			p := r.Latency[k]
			fmt.Fprintf(&b, "  - %s: n=%d p50=%.0f p95=%.0f p99=%.0f max=%.0f ms\n", k, p.N, p.P50ms, p.P95ms, p.P99ms, p.Maxms)
		}
		b.WriteString("\n")
	}
	return b.String()
}
