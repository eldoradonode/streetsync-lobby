# Load test run st02

Started 2026-10-26T04:19:02Z. Target `ws://127.0.0.1:18080/ws`.

## Environment

- Generator: go1.22.2 on linux/amd64, 1 CPUs, GOMAXPROCS 1, file limit 20000
- cgroup cpu.max `n/a`, memory.max `n/a`, host memory 4091692 kB
- Server: pid 3716 (lobby)
- Database: PostgreSQL 16.15 (Ubuntu 16.15-0ubuntu0.24.04.1) on x86_64-pc-linux-gnu, compiled by gcc (Ubuntu 13.3.0-6ubuntu2~24.04.1) 13.3.0, 64-bit, max_connections 100, shared_buffers 128MB

## Configuration

Stages [3000], measured window 1m0s per stage, ramp 50 connections/s, message every ~30s, DM every ~30s, room change every ~45s, lookups every ~20s, 0.30 connection drops per bot per minute, rooms [lobby abuja lagos campus football music tech side-hustle creators].

Acceptance criteria: `max_busy_rate=0.005` `max_p95_ms=1000` `max_p99_ms=3000` `min_connect_success=0.99` `min_delivery=0.97` `min_echo_success=0.99` `min_reconnect_success=0.98` 

## Results

| Stage | Verdict | Live | Connect | Echo | Delivery* | p50 / p95 / p99 ms | Reconnect | Server CPU % | Server RSS MB | Gen CPU % | PG conns |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 3000 | **FAIL** | 2277 | 0.26 | 0.9 | 0.88 | 646 / 11408 / 15889 | 0.41 | 44.38 | 245.57 | 24.33 | 5 |

\* Delivery is approximate: the generator estimates how many bots were in a room when a message was sent. Latency is measured on the generator's clock from the send time in the message to the arrival time, so it is only meaningful when generator and server share a machine or have synchronised clocks.

**No stage met the acceptance criteria.**

### Stage 3000: FAIL

- Reasons: [connect success 0.258 < 0.99 echo success 0.896 < 0.99 delivery 0.880 < 0.97 (approximate) delivery p95 11408 ms > 1000 reconnect success 0.411 < 0.98 server_busy rate 0.1819 > 0.0050 only 2279 of 3000 clients are connected ramp connect success 0.575 < 0.99]
- Bottleneck: shared host CPU is saturated (server 44%, generator 24% of one core): on one machine the two can not be separated cleanly; run the generator on another machine to confirm
- Ramp: map[auth_failures:3204 connect_attempts:7540 connect_ok:4336 connect_success:0.58 connection_errors:map[auth_read:93] duration:2m1.966s error_codes:map[handle_taken:1353 no_credits:6 no_such_user:31 server_busy:3590]]
- Error codes: map[handle_taken:1561 no_credits:1 server_busy:2898], connection errors: map[auth_read:26]
- Resources: map[generator_cpu_percent_of_one_core:24.33 generator_goroutines_max:8248 generator_rss_mb_max:160.03 generator_rss_mb_start_end:[156.27 160.03] generator_scheduler_lag_max_ms:131.69 generator_scheduler_lag_p95_ms:25.21 host_cpu_busy_percent_all_cores:99.97 postgres_active_max:3 postgres_connections_max:5 postgres_deadlocks_in_window:0 postgres_lock_waiters_max:0 postgres_rollbacks_in_window:18 samples:30 server_cpu_percent_of_one_core:44.38 server_fds_max:2933 server_process_alive_at_end:true server_rss_growth_mb_per_min:0.07 server_rss_mb_max:245.57 server_rss_mb_start_end:[245.5 245.57] server_threads_max:13]
- Metrics: map[auth_failures:2875 connect_attempts:4667 connect_ok:1203 connect_success:0.26 deliveries_expected_approx:943079 deliveries_seen:829532 delivery_ratio_approx:0.88 dms_confirmed:2563 dms_sent:3782 drops_injected:676 echo_success:0.9 joins:2325 joins_confirmed:3495 leaves:2319 messages_echoed:3438 messages_sent:3840 queries:6004 queries_confirmed:3965 reconnect_attempts:2925 reconnect_ok:1203 reconnect_success:0.41 server_busy:2901 server_busy_rate:0.18193 unexpected_disconnects:697]

  - connect: n=4661 p50=233 p95=996 p99=1232 max=1516 ms
  - auth: n=1203 p50=9171 p95=16429 p99=18660 max=19749 ms
  - message_delivery: n=828852 p50=646 p95=11408 p99=15889 max=28540 ms
  - message_echo_roundtrip: n=3438 p50=377 p95=11059 p99=15299 max=25897 ms
  - reconnect_recovery: n=1432 p50=19934 p95=69464 p99=90359 max=119683 ms

