# Load testing STREETSYNC

The tool is `loadtest/` (Go, same module). It drives the **real server over the real WebSocket protocol** with simulated people. It does not touch the database directly, except for two optional things: reading PostgreSQL connection statistics, and `-cleanup`.

## Safety rules (enforced in `loadtest/safety.go`)

- `-url` is required. There is no default target.
- Allowed without ceremony: loopback, private network addresses, `localhost`, `*.local`, `*.internal`.
- A public staging host needs **both** `LOADTEST_ALLOWED_HOSTS=host` in the environment **and** `-confirm-host host`.
- `LOADTEST_FORBIDDEN_HOSTS=prod.example.com,...` always refuses. `LOADTEST_ENV=production` refuses everything. Put your production host in the first one.
- The same checks apply to `-db-url` (used for connection statistics and `-cleanup`).
- Bots are guest accounts named `lt<runid>_<n>`. `-cleanup` deletes only never-passworded `role='user'` accounts matching `^lt[0-9a-f]{4}_[0-9]+$` (their ledger rows and messages first), in one transaction.
- Do not create bots in a real database. Use the throwaway database `streetsync_load` that `scripts/loadtest-local.sh` makes.
- The server must run with `REQUIRE_PASSWORD=0` (guest sign in). Never set that on production.

## Run it

```
# local smoke test: 100 clients for 20 seconds
scripts/loadtest-local.sh smoke
# the staged profile 100, 500, 1000, 3000 (long: about 10 minutes)
scripts/loadtest-local.sh staged
# your own stages
STAGES=100,500 STAGE_DURATION=45s scripts/loadtest-local.sh custom -msg-interval 30s -stop-on-fail=false
```
Needs `psql`, a local PostgreSQL where `PGADMIN_URL` (default `postgres://lobby:lobby@127.0.0.1:5432/postgres`) may create databases, and Go. Run the big stages detached (`setsid nohup ... &`), because some shells kill background jobs.

Against a separate staging server (the recommended way for the 3,000 stage, with the generator on **another machine**):
```
export LOADTEST_ALLOWED_HOSTS=staging.example.com
export LOADTEST_FORBIDDEN_HOSTS=your-production-host
go run ./loadtest -url wss://staging.example.com/ws -confirm-host staging.example.com \
  -stages 100,500,1000,3000 -stage-duration 120s -db-url "$STAGING_DB_URL" -msg-interval 30s
go run ./loadtest -cleanup -db-url "$STAGING_DB_URL" -confirm-host staging-db-host
```
Raise the file limit first (`ulimit -n 65535`). The tool refuses to start if the limit is too low for the largest stage. `go run ./loadtest -h` lists every setting (stage count, duration, ramp rate, message / DM / room-change / lookup intervals, drop rate, rooms, thresholds).

## What each bot does

Signs in as a new guest (`hi`), joins a room, posts at a human pace (default every ~10 s, jittered), changes rooms (`leave` + `join`), sends direct messages to other bots (database transaction and a credit fee), looks people up (`usearch`, `profile`, `wallet`, `inbox`), and sometimes loses its connection (cut without a goodbye) and comes back with its saved login to the same room. Each message carries its own send time, so any bot that receives it measures delivery latency.

## What is measured, and how to read it

Per stage, after a ramp (new connections at `-connect-rate` per second) and a settle time, over the measured window:

- **Connections:** attempts, successes, auth failures, error codes, connect and auth latency (ramp figures are reported separately).
- **Messages:** sent, echoed back to the sender (send success), delivered to others (approximate delivery ratio) and **delivery latency p50/p95/p99** (send time inside the message, same clock).
- **Reconnects:** attempts, success, recovery time (cut to signed in again). Unexpected disconnects (not caused by the bot) are counted separately.
- **Server side** (with `-server-pid`): CPU, memory, file descriptors, threads. **PostgreSQL** (with `-db-url`): connections, active, lock waiters, rollbacks, deadlocks. **Generator:** its CPU, memory, goroutines and **scheduler lag** (how late its own timers fire).
- `server_busy` errors mean the server's database calls timed out. `handle_taken` during reconnect storms is a generator artifact (a bot retried creating an account whose first answer it never received).

**Acceptance criteria** (flags `-min-*`, `-max-*`): connect success >= 99%, echo success >= 99%, approximate delivery >= 97%, delivery p95 <= 1000 ms and p99 <= 3000 ms, reconnect success >= 98%, `server_busy` <= 0.5% of requests, the server process alive, >= 95% of clients connected. A stage is **PASS**, **FAIL**, or **INVALID** (generator timers p95 late by > 500 ms: the generator was the bottleneck and the stage says nothing about the server).

**Server or generator?** On one machine they share CPUs, and the report says so: it prints both CPU shares and the host load. If host CPU is saturated, treat the result as inconclusive until you repeat it with the generator on another machine. The reported "highest passing stage" is for that environment only. It is **not** a promise that the product supports that many users elsewhere.

Output: `loadtest/results/run-<id>.md` and `.json`. Repeat after a change and compare the tables. `loadtest/server.log` holds the server log of the local script.
