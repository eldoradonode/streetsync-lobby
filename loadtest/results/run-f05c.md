# Load test run f05c

Started 2026-10-26T04:09:26Z. Target `ws://127.0.0.1:18080/ws`.

## Environment

- Generator: go1.22.2 on linux/amd64, 1 CPUs, GOMAXPROCS 1, file limit 20000
- cgroup cpu.max `n/a`, memory.max `n/a`, host memory 4091692 kB
- Server: pid 160 (lobby)
- Database: PostgreSQL 16.15 (Ubuntu 16.15-0ubuntu0.24.04.1) on x86_64-pc-linux-gnu, compiled by gcc (Ubuntu 13.3.0-6ubuntu2~24.04.1) 13.3.0, 64-bit, max_connections 100, shared_buffers 128MB

## Configuration

Stages [100], measured window 20s per stage, ramp 50 connections/s, message every ~10s, DM every ~30s, room change every ~45s, lookups every ~20s, 0.30 connection drops per bot per minute, rooms [lobby abuja lagos campus football music tech side-hustle creators].

Acceptance criteria: `max_busy_rate=0.005` `max_p95_ms=1000` `max_p99_ms=3000` `min_connect_success=0.99` `min_delivery=0.97` `min_echo_success=0.99` `min_reconnect_success=0.98` 

## Results

| Stage | Verdict | Live | Connect | Echo | Delivery* | p50 / p95 / p99 ms | Reconnect | Server CPU % | Server RSS MB | Gen CPU % | PG conns |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 100 | **PASS** | 100 | 1 | 1 | 1 | 1 / 1 / 2 | 1 | 1 | 19.63 | 0.75 | 3 |

\* Delivery is approximate: the generator estimates how many bots were in a room when a message was sent. Latency is measured on the generator's clock from the send time in the message to the arrival time, so it is only meaningful when generator and server share a machine or have synchronised clocks.

**Highest stage that met the acceptance criteria in this environment: 100 concurrent clients.** This says nothing about other hardware or a different network.

### Stage 100: PASS

- Reasons: []
- Bottleneck: none observed at this level
- Ramp: map[auth_failures:0 connect_attempts:99 connect_ok:99 connect_success:1 connection_errors:map[] duration:2.001s error_codes:map[]]
- Error codes: map[], connection errors: map[]
- Resources: map[generator_cpu_percent_of_one_core:0.75 generator_goroutines_max:305 generator_rss_mb_max:14.59 generator_rss_mb_start_end:[13.35 14.59] generator_scheduler_lag_max_ms:1.18 generator_scheduler_lag_p95_ms:1.02 host_cpu_busy_percent_all_cores:3.64 postgres_active_max:1 postgres_connections_max:3 postgres_deadlocks_in_window:0 postgres_lock_waiters_max:0 postgres_rollbacks_in_window:0 samples:9 server_cpu_percent_of_one_core:1 server_fds_max:109 server_process_alive_at_end:true server_rss_growth_mb_per_min:6.17 server_rss_mb_max:19.63 server_rss_mb_start_end:[17.98 19.63] server_threads_max:7]
- Metrics: map[auth_failures:0 connect_attempts:8 connect_ok:8 connect_success:1 deliveries_expected_approx:1725 deliveries_seen:1725 delivery_ratio_approx:1 dms_confirmed:23 dms_sent:23 drops_injected:7 echo_success:1 joins:1 joins_confirmed:8 leaves:0 messages_echoed:160 messages_sent:160 queries:43 queries_confirmed:43 reconnect_attempts:7 reconnect_ok:7 reconnect_success:1 server_busy:0 server_busy_rate:0 unexpected_disconnects:0]

  - connect: n=8 p50=1 p95=1 p99=1 max=3 ms
  - auth: n=8 p50=4 p95=5 p99=5 max=6 ms
  - message_delivery: n=1725 p50=1 p95=1 p99=2 max=11 ms
  - message_echo_roundtrip: n=160 p50=1 p95=1 p99=2 max=11 ms
  - reconnect_recovery: n=7 p50=511 p95=570 p99=570 max=614 ms

