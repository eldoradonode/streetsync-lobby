# Load test run 6e57

Started 2026-10-27T12:03:27Z. Target `ws://127.0.0.1:18081/ws`.

## Environment

- Generator: go1.22.2 on linux/amd64, 1 CPUs, GOMAXPROCS 1, file limit 20000
- cgroup cpu.max `n/a`, memory.max `n/a`, host memory 4091700 kB
- Server: pid 815 (s16)
- Database: PostgreSQL 16.15 (Ubuntu 16.15-0ubuntu0.24.04.1) on x86_64-pc-linux-gnu, compiled by gcc (Ubuntu 13.3.0-6ubuntu2~24.04.1) 13.3.0, 64-bit, max_connections 100, shared_buffers 128MB

## Configuration

Stages [1000], measured window 25s per stage, ramp 100 connections/s, message every ~10s, DM every ~6s, room change every ~45s, lookups every ~20s, 0.30 connection drops per bot per minute, rooms [lobby abuja lagos campus football music tech side-hustle creators].

Acceptance criteria: `max_busy_rate=0.005` `max_p95_ms=1000` `max_p99_ms=3000` `min_connect_success=0.99` `min_delivery=0.97` `min_echo_success=0.99` `min_reconnect_success=0.98` 

## Results

| Stage | Verdict | Live | Connect | Echo | Delivery* | p50 / p95 / p99 ms | Reconnect | Server CPU % | Server RSS MB | Gen CPU % | PG conns |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 1000 | **PASS** | 998 | 1 | 1 | 1 | 8 / 49 / 78 | 1 | 34.77 | 92.8 | 25.09 | 5 |

\* Delivery is approximate: the generator estimates how many bots were in a room when a message was sent. Latency is measured on the generator's clock from the send time in the message to the arrival time, so it is only meaningful when generator and server share a machine or have synchronised clocks.

**Highest stage that met the acceptance criteria in this environment: 1000 concurrent clients.** This says nothing about other hardware or a different network.

### Stage 1000: PASS

- Reasons: []
- Bottleneck: none observed at this level
- Ramp: map[auth_failures:0 connect_attempts:999 connect_ok:999 connect_success:1 connection_errors:map[] duration:10.011s error_codes:map[]]
- Error codes: map[], connection errors: map[]
- Resources: map[generator_cpu_percent_of_one_core:25.09 generator_goroutines_max:3004 generator_rss_mb_max:69.88 generator_rss_mb_start_end:[52.73 69.88] generator_scheduler_lag_max_ms:8.38 generator_scheduler_lag_p95_ms:3.66 host_cpu_busy_percent_all_cores:80.06 postgres_active_max:2 postgres_connections_max:5 postgres_deadlocks_in_window:0 postgres_lock_waiters_max:0 postgres_rollbacks_in_window:0 samples:12 server_cpu_percent_of_one_core:34.77 server_fds_max:1011 server_process_alive_at_end:true server_rss_growth_mb_per_min:33.93 server_rss_mb_max:92.8 server_rss_mb_start_end:[80.36 92.8] server_threads_max:11]
- Metrics: map[auth_failures:0 connect_attempts:110 connect_ok:110 connect_success:1 deliveries_expected_approx:257369 deliveries_seen:256971 delivery_ratio_approx:1 dms_confirmed:4 dms_sent:3 drops_injected:111 echo_success:1 joins:181 joins_confirmed:304 leaves:158 messages_echoed:2328 messages_sent:2331 queries:1015 queries_confirmed:1014 reconnect_attempts:109 reconnect_ok:109 reconnect_success:1 server_busy:0 server_busy_rate:0 unexpected_disconnects:0]

  - connect: n=110 p50=5 p95=47 p99=84 max=110 ms
  - auth: n=110 p50=13 p95=126 p99=295 max=652 ms
  - message_delivery: n=256869 p50=8 p95=49 p99=78 max=386 ms
  - message_echo_roundtrip: n=2328 p50=8 p95=52 p99=80 max=368 ms
  - reconnect_recovery: n=109 p50=526 p95=813 p99=872 max=1098 ms

