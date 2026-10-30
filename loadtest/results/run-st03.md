# Load test run st03

Started 2026-10-26T09:23:32Z. Target `ws://127.0.0.1:18080/ws`.

## Environment

- Generator: go1.22.2 on linux/amd64, 1 CPUs, GOMAXPROCS 1, file limit 20000
- cgroup cpu.max `n/a`, memory.max `n/a`, host memory 4091692 kB
- Server: pid 2879 (lobby)
- Database: PostgreSQL 16.15 (Ubuntu 16.15-0ubuntu0.24.04.1) on x86_64-pc-linux-gnu, compiled by gcc (Ubuntu 13.3.0-6ubuntu2~24.04.1) 13.3.0, 64-bit, max_connections 100, shared_buffers 128MB

## Configuration

Stages [1500 2000], measured window 1m0s per stage, ramp 50 connections/s, message every ~30s, DM every ~30s, room change every ~45s, lookups every ~20s, 0.30 connection drops per bot per minute, rooms [lobby abuja lagos campus football music tech side-hustle creators].

Acceptance criteria: `max_busy_rate=0.005` `max_p95_ms=1000` `max_p99_ms=3000` `min_connect_success=0.99` `min_delivery=0.97` `min_echo_success=0.99` `min_reconnect_success=0.98` 

## Results

| Stage | Verdict | Live | Connect | Echo | Delivery* | p50 / p95 / p99 ms | Reconnect | Server CPU % | Server RSS MB | Gen CPU % | PG conns |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 1500 | **PASS** | 1495 | 1 | 1 | 1 | 10 / 68 / 127 | 1 | 30.72 | 132.14 | 17.95 | 5 |
| 2000 | **FAIL** | 1983 | 0.98 | 1 | 1 | 60 / 345 / 1366 | 0.98 | 42.45 | 170.19 | 23.67 | 5 |

\* Delivery is approximate: the generator estimates how many bots were in a room when a message was sent. Latency is measured on the generator's clock from the send time in the message to the arrival time, so it is only meaningful when generator and server share a machine or have synchronised clocks.

**Highest stage that met the acceptance criteria in this environment: 1500 concurrent clients.** This says nothing about other hardware or a different network.

### Stage 1500: PASS

- Reasons: []
- Bottleneck: none observed at this level
- Ramp: map[auth_failures:0 connect_attempts:1581 connect_ok:1581 connect_success:1 connection_errors:map[] duration:30.14s error_codes:map[]]
- Error codes: map[], connection errors: map[]
- Resources: map[generator_cpu_percent_of_one_core:17.95 generator_goroutines_max:4505 generator_rss_mb_max:90.11 generator_rss_mb_start_end:[71.54 90.11] generator_scheduler_lag_max_ms:45.18 generator_scheduler_lag_p95_ms:3.37 host_cpu_busy_percent_all_cores:89.9 postgres_active_max:3 postgres_connections_max:5 postgres_deadlocks_in_window:0 postgres_lock_waiters_max:0 postgres_rollbacks_in_window:1 samples:30 server_cpu_percent_of_one_core:30.72 server_fds_max:1511 server_process_alive_at_end:true server_rss_growth_mb_per_min:26.06 server_rss_mb_max:132.14 server_rss_mb_start_end:[106.95 132.14] server_threads_max:12]
- Metrics: map[auth_failures:0 connect_attempts:445 connect_ok:444 connect_success:1 deliveries_expected_approx:442995 deliveries_seen:442461 delivery_ratio_approx:1 dms_confirmed:2678 dms_sent:2681 drops_injected:448 echo_success:1 joins:1616 joins_confirmed:2055 leaves:1612 messages_echoed:2677 messages_sent:2680 queries:4215 queries_confirmed:4209 reconnect_attempts:444 reconnect_ok:443 reconnect_success:1 server_busy:0 server_busy_rate:0 unexpected_disconnects:0]

  - connect: n=444 p50=6 p95=96 p99=182 max=270 ms
  - auth: n=444 p50=28 p95=141 p99=232 max=485 ms
  - message_delivery: n=441809 p50=10 p95=68 p99=127 max=246 ms
  - message_echo_roundtrip: n=2677 p50=11 p95=68 p99=126 max=198 ms
  - reconnect_recovery: n=444 p50=571 p95=811 p99=952 max=1470 ms

### Stage 2000: FAIL

- Reasons: [connect success 0.975 < 0.99 reconnect success 0.975 < 0.98]
- Bottleneck: shared host CPU is saturated (server 42%, generator 24% of one core): on one machine the two can not be separated cleanly; run the generator on another machine to confirm
- Ramp: map[auth_failures:0 connect_attempts:585 connect_ok:585 connect_success:1 connection_errors:map[] duration:12.479s error_codes:map[no_such_user:4]]
- Error codes: map[], connection errors: map[]
- Resources: map[generator_cpu_percent_of_one_core:23.67 generator_goroutines_max:5999 generator_rss_mb_max:114.98 generator_rss_mb_start_end:[104.81 114.98] generator_scheduler_lag_max_ms:85.3 generator_scheduler_lag_p95_ms:15.94 host_cpu_busy_percent_all_cores:99.65 postgres_active_max:3 postgres_connections_max:5 postgres_deadlocks_in_window:0 postgres_lock_waiters_max:0 postgres_rollbacks_in_window:0 samples:30 server_cpu_percent_of_one_core:42.45 server_fds_max:2009 server_process_alive_at_end:true server_rss_growth_mb_per_min:10.69 server_rss_mb_max:170.19 server_rss_mb_start_end:[159.86 170.19] server_threads_max:12]
- Metrics: map[auth_failures:0 connect_attempts:606 connect_ok:596 connect_success:0.98 deliveries_expected_approx:794303 deliveries_seen:793046 delivery_ratio_approx:1 dms_confirmed:3595 dms_sent:3632 drops_injected:608 echo_success:1 joins:2302 joins_confirmed:2895 leaves:2297 messages_echoed:3616 messages_sent:3625 queries:5591 queries_confirmed:5544 reconnect_attempts:606 reconnect_ok:596 reconnect_success:0.98 server_busy:0 server_busy_rate:0 unexpected_disconnects:0]

  - connect: n=605 p50=100 p95=567 p99=685 max=835 ms
  - auth: n=596 p50=614 p95=3592 p99=7012 max=9168 ms
  - message_delivery: n=788293 p50=60 p95=345 p99=1366 max=7142 ms
  - message_echo_roundtrip: n=3616 p50=66 p95=361 p99=1168 max=6927 ms
  - reconnect_recovery: n=605 p50=1332 p95=4215 p99=7835 max=9965 ms

