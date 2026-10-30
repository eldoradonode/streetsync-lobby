# Load test run 5de0

Started 2026-10-28T09:20:13Z. Target `ws://127.0.0.1:8110/ws`.

## Environment

- Generator: go1.22.2 on linux/amd64, 1 CPUs, GOMAXPROCS 1, file limit 20000
- cgroup cpu.max `n/a`, memory.max `n/a`, host memory 4091696 kB
- Server: pid 1859 (lobby-test)
- Database: PostgreSQL 16.15 (Ubuntu 16.15-0ubuntu0.24.04.1) on x86_64-pc-linux-gnu, compiled by gcc (Ubuntu 13.3.0-6ubuntu2~24.04.1) 13.3.0, 64-bit, max_connections 100, shared_buffers 128MB

## Configuration

Stages [100 300], measured window 30s per stage, ramp 50 connections/s, message every ~3s, DM every ~8s, room change every ~8s, lookups every ~6s, 0.30 connection drops per bot per minute, rooms [lobby abuja lagos campus football music tech side-hustle creators].

Acceptance criteria: `max_busy_rate=0.005` `max_p95_ms=1000` `max_p99_ms=3000` `min_connect_success=0.99` `min_delivery=0.97` `min_echo_success=0.99` `min_reconnect_success=0.98` 

## Results

| Stage | Verdict | Live | Connect | Echo | Delivery* | p50 / p95 / p99 ms | Reconnect | Server CPU % | Server RSS MB | Gen CPU % | PG conns |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 100 | **PASS** | 99 | 1 | 1 | 1 | 1 / 1 / 3 | 1 | 3.39 | 100.64 | 3.43 | 3 |
| 300 | **PASS** | 299 | 1 | 1 | 1 | 1 / 5 / 14 | 1 | 14.64 | 44.32 | 12.54 | 3 |

\* Delivery is approximate: the generator estimates how many bots were in a room when a message was sent. Latency is measured on the generator's clock from the send time in the message to the arrival time, so it is only meaningful when generator and server share a machine or have synchronised clocks.

**Highest stage that met the acceptance criteria in this environment: 300 concurrent clients.** This says nothing about other hardware or a different network.

### Stage 100: PASS

- Reasons: []
- Bottleneck: none observed at this level
- Ramp: map[auth_failures:0 connect_attempts:99 connect_ok:99 connect_success:1 connection_errors:map[] duration:2.005s error_codes:map[]]
- Error codes: map[], connection errors: map[]
- Resources: map[generator_cpu_percent_of_one_core:3.43 generator_goroutines_max:305 generator_rss_mb_max:15.59 generator_rss_mb_start_end:[13.59 15.59] generator_scheduler_lag_max_ms:1.04 generator_scheduler_lag_p95_ms:1.01 host_cpu_busy_percent_all_cores:11.06 postgres_active_max:1 postgres_connections_max:3 postgres_deadlocks_in_window:0 postgres_lock_waiters_max:0 postgres_rollbacks_in_window:0 samples:15 server_cpu_percent_of_one_core:3.39 server_fds_max:109 server_process_alive_at_end:true server_rss_growth_mb_per_min:-19.2 server_rss_mb_max:100.64 server_rss_mb_start_end:[92.2 83.23] server_threads_max:8]
- Metrics: map[auth_failures:0 connect_attempts:13 connect_ok:13 connect_success:1 deliveries_expected_approx:10388 deliveries_seen:10388 delivery_ratio_approx:1 dms_confirmed:3 dms_sent:3 drops_injected:13 echo_success:1 joins:327 joins_confirmed:339 leaves:331 messages_echoed:986 messages_sent:986 queries:463 queries_confirmed:463 reconnect_attempts:12 reconnect_ok:12 reconnect_success:1 server_busy:0 server_busy_rate:0 unexpected_disconnects:0]

  - connect: n=13 p50=1 p95=3 p99=3 max=6 ms
  - auth: n=13 p50=6 p95=10 p99=10 max=18 ms
  - message_delivery: n=10388 p50=1 p95=1 p99=3 max=5 ms
  - message_echo_roundtrip: n=986 p50=1 p95=1 p99=3 max=3 ms
  - reconnect_recovery: n=12 p50=565 p95=710 p99=710 max=728 ms

### Stage 300: PASS

- Reasons: []
- Bottleneck: none observed at this level
- Ramp: map[auth_failures:0 connect_attempts:200 connect_ok:200 connect_success:1 connection_errors:map[] duration:4s error_codes:map[]]
- Error codes: map[], connection errors: map[]
- Resources: map[generator_cpu_percent_of_one_core:12.54 generator_goroutines_max:905 generator_rss_mb_max:29.46 generator_rss_mb_start_end:[24.27 29.46] generator_scheduler_lag_max_ms:1.72 generator_scheduler_lag_p95_ms:1.61 host_cpu_busy_percent_all_cores:41.53 postgres_active_max:1 postgres_connections_max:3 postgres_deadlocks_in_window:0 postgres_lock_waiters_max:0 postgres_rollbacks_in_window:1 samples:15 server_cpu_percent_of_one_core:14.64 server_fds_max:309 server_process_alive_at_end:true server_rss_growth_mb_per_min:5.39 server_rss_mb_max:44.32 server_rss_mb_start_end:[41.65 44.17] server_threads_max:9]
- Metrics: map[auth_failures:0 connect_attempts:52 connect_ok:52 connect_success:1 deliveries_expected_approx:96052 deliveries_seen:96011 delivery_ratio_approx:1 dms_confirmed:12 dms_sent:12 drops_injected:52 echo_success:1 joins:1047 joins_confirmed:1098 leaves:1049 messages_echoed:2959 messages_sent:2960 queries:1443 queries_confirmed:1443 reconnect_attempts:51 reconnect_ok:51 reconnect_success:1 server_busy:0 server_busy_rate:0 unexpected_disconnects:0]

  - connect: n=52 p50=1 p95=3 p99=9 max=20 ms
  - auth: n=52 p50=7 p95=23 p99=44 max=60 ms
  - message_delivery: n=96011 p50=1 p95=5 p99=14 max=126 ms
  - message_echo_roundtrip: n=2959 p50=1 p95=5 p99=13 max=125 ms
  - reconnect_recovery: n=51 p50=567 p95=726 p99=755 max=759 ms

