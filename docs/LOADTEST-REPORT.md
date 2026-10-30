# Load test report, step 15

Raw reports: `loadtest/results/run-st01.*` (100, 500, 1000), `run-st03.*` (1500, 2000), `run-st02.*` (3000), `run-f05c.*` (smoke, 100 clients, 20 s). Tool and method: `docs/LOADTEST.md`.

## Environment (what was actually used)

| | |
|---|---|
| Machine | one sandbox VM: **1 vCPU**, 4 GB RAM, no cgroup limit file, client file limit 20,000 |
| Shared by | the Go server, PostgreSQL and the load generator, **all on that one CPU** |
| Server | the real STREETSYNC build of this release, Go 1.22.2, `lib/pq`, DB pool `MaxOpenConns(4)` (unchanged, `store.go`) |
| Database | PostgreSQL 16.15, `max_connections` 100, `shared_buffers` 128 MB, fresh throwaway database `streetsync_load`, default settings, no pooler, same machine |
| Generator | Go 1.22.2, GOMAXPROCS 1, same machine, loopback network |
| Profile | stages run cumulatively (bots stay connected as the next stage adds more), 60 s measured per stage after the ramp (50 new connections per second), message every ~30 s per bot, DM every ~30 s, room change every ~45 s, lookups every ~20 s, 0.3 connection drops per bot per minute, 9 official rooms |
| Acceptance | connect >= 99%, echo >= 99%, delivery >= 97% (approximate), p95 <= 1000 ms, p99 <= 3000 ms, reconnect >= 98%, server_busy <= 0.5%, server alive, >= 95% clients connected |

Not representative of production: a real deployment has a network, a different CPU budget, and a hosted database with its own pooler.

## Results

| Stage | Verdict | Clients live | Connect | Echo | Delivery (approx.) | p50 / p95 / p99 ms | Reconnect | Server CPU* | Generator CPU* | Host CPU | Server RSS | PG conns (active) |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 100 | **PASS** | 98 | 100% | 100% | 100% | 1 / 2 / 4 | 100% (22) | 1% | 1% | 4% | 21 MB | 3 (1) |
| 500 | **PASS** | 498 | 100% | 100% | 100% | 2 / 5 / 9 | 100% (172) | 7% | 4% | 27% | 54 MB | 4 (2) |
| 1000 | **PASS** | 998 | 100% | 100% | 100% | 4 / 14 / 38 | 100% (299) | 17% | 10% | 51% | 92 MB | 4 (2) |
| 1500 | **PASS** | 1495 | 100% | 100% | 100% | 10 / 68 / 127 | 100% (444) | 31% | 18% | 90% | 132 MB | 5 (3) |
| 2000 | **FAIL** | 1983 | 97.5% | 100% | 100% | 60 / 345 / 1366 | 97.5% (606) | 42% | 24% | 99.7% | 170 MB | 5 (3) |
| 3000 | **FAIL** | 2277 | 25.8% | 89.6% | 88% | 646 / 11408 / 15889 | 41% (2925) | 44% | 24% | 100% | 246 MB | 5 (3) |

\* percent of the one core. Delivery is approximate (the generator estimates room occupancy). Latency is generator clock to generator clock on one machine.

**Highest level that met the acceptance criteria in this environment: 1,500 concurrent clients.** Tested levels: 100, 500, 1000, 1500, 2000, 3000. The 3,000 stage did **not** pass: only 2,277 clients were connected at the end of its ramp. This is not a statement that the product supports 1,500 users in general, nor that it cannot support more on bigger hardware. It only says what this one shared 1 vCPU machine did against these criteria.

## What the failures show

- **2,000:** the host CPU was at 99.7% shared between three programs. Connect success fell to 97.5% (threshold 99%) and p99 delivery to 1.4 s. Latency (p95 345 ms) was still inside the threshold. Cause: CPU saturation of the shared machine. The generator's timers were on average only 16 ms late, so the generator was not the sole limit, but on one core the server and generator cannot be cleanly separated.
- **3,000:** 2,898 requests were answered `server_busy` (18% of requests), which is the server's own database timeout. PostgreSQL showed only 5 connections, 3 active: the **server's pool of 4 connections** was the choke point (together with a saturated CPU), not PostgreSQL's `max_connections` (100). 1,561 `handle_taken` errors are a **generator artifact**: bots whose first sign in timed out retried creating the same guest account. 697 unexpected disconnects and 2,925 reconnect attempts show a reconnect storm: each sign in costs several database queries (session, welcome credit, memberships, badge, notification state, report count), so connection drops while the database is slow make more sign ins, which make it slower. Delivery p95 was 11.4 s, p99 15.9 s.
- **Memory** grew with connections (about 100 KB per connection in the server: 246 MB at about 2,300 clients). No resource leak was seen inside any window (RSS flat per window, no file descriptor growth beyond the connection count), but the runs were minutes long, not hours. A soak test is not done.
- No server crash, no panic, no PostgreSQL deadlock and no failed query were reported at any stage.

## Recommended fixes (not applied, evidence first)

1. Re-run 2,000 and 3,000 with the generator on a **separate machine** and the database on its own host. If 3,000 then passes, the limit here was the shared CPU.
2. Reduce database work per sign in (five concurrent queries are started for every connection by `welcome`, `loadMemberships`, `sendBadgeTo`, `sendNoteStateTo`, `sendReportCountTo`): combine or cache them. This helps most during reconnect storms.
3. Make the web client reconnect with exponential backoff and random jitter, so a server hiccup does not cause a synchronized wave. Check what the current client does.
4. If the pool of 4 is confirmed as the limit on separate hardware, raising `SetMaxOpenConns` is the first thing to try. That touches connection code, so it needs your explicit decision, and it must stay below the hosted database's connection limit (Supabase pooler limits are small on the free plan).
5. Redis or sharding is **not** justified by these results.

## Repeat

```
scripts/loadtest-local.sh custom   # with STAGES=100,500,1000,1500,2000,3000 STAGE_DURATION=60s, and: -msg-interval 30s -stop-on-fail=false
```
Compare the new `loadtest/results/run-*.md` tables with this one.
