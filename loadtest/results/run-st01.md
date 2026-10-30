# Load test run st01

Started 2026-10-26T04:23:07Z. Target `ws://127.0.0.1:18080/ws`.

## Environment

- Generator: go1.22.2 on linux/amd64, 1 CPUs, GOMAXPROCS 1, file limit 20000
- cgroup cpu.max `n/a`, memory.max `n/a`, host memory 4091692 kB
- Server: pid 6543 (lobby)
- Database: PostgreSQL 16.15 (Ubuntu 16.15-0ubuntu0.24.04.1) on x86_64-pc-linux-gnu, compiled by gcc (Ubuntu 13.3.0-6ubuntu2~24.04.1) 13.3.0, 64-bit, max_connections 100, shared_buffers 128MB

## Configuration

Stages [100 500 1000], measured window 1m0s per stage, ramp 50 connections/s, message every ~30s, DM every ~30s, room change every ~45s, lookups every ~20s, 0.30 connection drops per bot per minute, rooms [lobby abuja lagos campus football music tech side-hustle creators].

Acceptance criteria: `max_busy_rate=0.005` `max_p95_ms=1000` `max_p99_ms=3000` `min_connect_success=0.99` `min_delivery=0.97` `min_echo_success=0.99` `min_reconnect_success=0.98` 

## Results

| Stage | Verdict | Live | Connect | Echo | Delivery* | p50 / p95 / p99 ms | Reconnect | Server CPU % | Server RSS MB | Gen CPU % | PG conns |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 100 | **PASS** | 98 | 1 | 1 | 1 | 1 / 2 / 4 | 1 | 1.05 | 20.93 | 0.77 | 3 |
| 500 | **PASS** | 498 | 1 | 1 | 1 | 2 / 5 / 9 | 1 | 7.1 | 53.63 | 4.47 | 4 |
| 1000 | **PASS** | 998 | 1 | 1 | 1 | 4 / 14 / 38 | 1 | 16.98 | 92.15 | 10.26 | 4 |

\* Delivery is approximate: the generator estimates how many bots were in a room when a message was sent. Latency is measured on the generator's clock from the send time in the message to the arrival time, so it is only meaningful when generator and server share a machine or have synchronised clocks.

**Highest stage that met the acceptance criteria in this environment: 1000 concurrent clients.** This says nothing about other hardware or a different network.

### Stage 100: PASS

- Reasons: []
- Bottleneck: none observed at this level
- Ramp: map[auth_failures:0 connect_attempts:99 connect_ok:99 connect_success:1 connection_errors:map[] duration:2.001s error_codes:map[]]
- Error codes: map[], connection errors: map[]
- Resources: map[generator_cpu_percent_of_one_core:0.77 generator_goroutines_max:305 generator_rss_mb_max:15.03 generator_rss_mb_start_end:[13.02 15.03] generator_scheduler_lag_max_ms:1.03 generator_scheduler_lag_p95_ms:1.02 host_cpu_busy_percent_all_cores:3.83 postgres_active_max:1 postgres_connections_max:3 postgres_deadlocks_in_window:0 postgres_lock_waiters_max:0 postgres_rollbacks_in_window:1 samples:29 server_cpu_percent_of_one_core:1.05 server_fds_max:110 server_process_alive_at_end:true server_rss_growth_mb_per_min:3.44 server_rss_mb_max:20.93 server_rss_mb_start_end:[17.73 20.93] server_threads_max:7]
- Metrics: map[auth_failures:0 connect_attempts:23 connect_ok:23 connect_success:1 deliveries_expected_approx:1587 deliveries_seen:1587 delivery_ratio_approx:1 dms_confirmed:144 dms_sent:145 drops_injected:25 echo_success:1 joins:88 joins_confirmed:110 leaves:87 messages_echoed:134 messages_sent:134 queries:248 queries_confirmed:248 reconnect_attempts:22 reconnect_ok:22 reconnect_success:1 server_busy:0 server_busy_rate:0 unexpected_disconnects:0]

  - connect: n=23 p50=1 p95=1 p99=1 max=3 ms
  - auth: n=23 p50=4 p95=5 p99=6 max=7 ms
  - message_delivery: n=1587 p50=1 p95=2 p99=4 max=4 ms
  - message_echo_roundtrip: n=134 p50=1 p95=2 p99=4 max=4 ms
  - reconnect_recovery: n=22 p50=350 p95=630 p99=695 max=697 ms

### Stage 500: PASS

- Reasons: []
- Bottleneck: none observed at this level
- Ramp: map[auth_failures:0 connect_attempts:402 connect_ok:402 connect_success:1 connection_errors:map[] duration:8s error_codes:map[]]
- Error codes: map[], connection errors: map[]
- Resources: map[generator_cpu_percent_of_one_core:4.47 generator_goroutines_max:1505 generator_rss_mb_max:37.23 generator_rss_mb_start_end:[29.1 37.23] generator_scheduler_lag_max_ms:2.04 generator_scheduler_lag_p95_ms:1 host_cpu_busy_percent_all_cores:27.06 postgres_active_max:2 postgres_connections_max:4 postgres_deadlocks_in_window:0 postgres_lock_waiters_max:0 postgres_rollbacks_in_window:0 samples:30 server_cpu_percent_of_one_core:7.1 server_fds_max:509 server_process_alive_at_end:true server_rss_growth_mb_per_min:11.04 server_rss_mb_max:53.63 server_rss_mb_start_end:[42.96 53.63] server_threads_max:8]
- Metrics: map[auth_failures:0 connect_attempts:173 connect_ok:173 connect_success:1 deliveries_expected_approx:44267 deliveries_seen:44268 delivery_ratio_approx:1 dms_confirmed:798 dms_sent:798 drops_injected:174 echo_success:1 joins:466 joins_confirmed:638 leaves:467 messages_echoed:805 messages_sent:805 queries:1323 queries_confirmed:1323 reconnect_attempts:172 reconnect_ok:172 reconnect_success:1 server_busy:0 server_busy_rate:0 unexpected_disconnects:0]

  - connect: n=173 p50=1 p95=3 p99=6 max=10 ms
  - auth: n=173 p50=5 p95=13 p99=19 max=85 ms
  - message_delivery: n=44268 p50=2 p95=5 p99=9 max=24 ms
  - message_echo_roundtrip: n=805 p50=2 p95=5 p99=8 max=23 ms
  - reconnect_recovery: n=172 p50=522 p95=734 p99=746 max=755 ms

### Stage 1000: PASS

- Reasons: []
- Bottleneck: none observed at this level
- Ramp: map[auth_failures:0 connect_attempts:523 connect_ok:523 connect_success:1 connection_errors:map[] duration:10.004s error_codes:map[]]
- Error codes: map[], connection errors: map[]
- Resources: map[generator_cpu_percent_of_one_core:10.26 generator_goroutines_max:3003 generator_rss_mb_max:64.75 generator_rss_mb_start_end:[50.25 64.75] generator_scheduler_lag_max_ms:1.04 generator_scheduler_lag_p95_ms:0.92 host_cpu_busy_percent_all_cores:51.03 postgres_active_max:2 postgres_connections_max:4 postgres_deadlocks_in_window:0 postgres_lock_waiters_max:0 postgres_rollbacks_in_window:0 samples:30 server_cpu_percent_of_one_core:16.98 server_fds_max:1009 server_process_alive_at_end:true server_rss_growth_mb_per_min:18.18 server_rss_mb_max:92.15 server_rss_mb_start_end:[74.58 92.15] server_threads_max:10]
- Metrics: map[auth_failures:0 connect_attempts:300 connect_ok:300 connect_success:1 deliveries_expected_approx:190036 deliveries_seen:189942 delivery_ratio_approx:1 dms_confirmed:1728 dms_sent:1729 drops_injected:301 echo_success:1 joins:1051 joins_confirmed:1349 leaves:1049 messages_echoed:1716 messages_sent:1717 queries:2749 queries_confirmed:2749 reconnect_attempts:299 reconnect_ok:299 reconnect_success:1 server_busy:0 server_busy_rate:0 unexpected_disconnects:0]

  - connect: n=300 p50=1 p95=7 p99=21 max=64 ms
  - auth: n=300 p50=6 p95=34 p99=55 max=80 ms
  - message_delivery: n=189942 p50=4 p95=14 p99=38 max=78 ms
  - message_echo_roundtrip: n=1716 p50=3 p95=14 p99=37 max=69 ms
  - reconnect_recovery: n=299 p50=499 p95=743 p99=755 max=759 ms

