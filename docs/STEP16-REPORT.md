# Step 16 report

## 1. Implemented
Seven day retention for DMs and room history (batched, scheduled, tested); password change hardening and eye toggles; end-to-end encrypted DMs for a single device with selected-message reporting; admin no-wallet policy; Brotli, q-value negotiation and per-encoding ETags; configurable database pool; load-test bots updated for the encrypted DM protocol.

Defects found and fixed: expired messages stayed in in-memory room buffers until restart; cleanup stopped at the first failing job and deleted without batching; a password change left other live connections open and was not atomic; DM reports and notifications exposed plaintext on the server; a Step 15 test passed only on a pre-seeded database.

## 2. Verified guarantees
See `SECURITY-VERIFICATION.md`.

## 3. Files and migrations
New: `retention.go`, `pool.go`, `e2ee.go`, tests (`retention_test.go`, `password_test.go`, `e2ee_test.go`, `e2ee_helpers_test.go`, `e2ee_interop_test.go`, `step16_policy_test.go`, `static16_test.go`), `ui/src/lib/e2ee.js`, `ui/src/components/PasswordField.svelte`, `tests/helpers16.js`, `tests/page_step16_*.test.js`, `tests/e2ee_interop.test.js`, `tests/fixtures/`, `scripts/admin_wallet_report.sql`, docs.
Changed: `schema.sql`, `hub.go`, `ws.go`, `main.go`, `store.go`, `dm.go`, `dm_store.go`, `notify.go`, `wallet.go`, `tips.go`, `rooms_store.go`, `static.go`, `go.mod` (adds `github.com/andybalholm/brotli`), UI components, `loadtest/bot.go`.
Migrations: idempotent, in `schema.sql`, applied at startup. `dm_messages` gets `enc, ct, nonce, sfp, rfp, mid` and index `dm_mid`; new tables `e2e_keys` and `e2e_key_log`. Nothing is dropped or rewritten. Not yet tested against a copy of representative production data.

## 4. Tests
Passed: gofmt, go vet, full Go suite on a fresh database, Go suite with `-race`, 786 browser-side assertions (15 existing page test files, 3 new, interop).
Not run: real browser end-to-end (no Chromium), hosted Supabase/Render checks, database interruption and recovery, soak test, migration test on representative data.

## 5. Transfer measurements (loopback, this sandbox; `STEP16-TRANSFER-MEASUREMENTS.txt`)
| | Step 15 | Step 16 |
|---|---|---|
| Raw, page + 2 assets | 233,910 B | 252,358 B |
| gzip | 71,627 B | 77,683 B |
| Brotli | not offered | 66,850 B |
| Repeat visit | 304, 0 B body; assets immutable for a year | same |
Step 16 is larger because it contains the encryption code. Browsers that accept Brotli download about 6.7 percent less than Step 15 gzip. Latency on loopback was about 0.5 ms for both and says nothing about mobile networks. Whether Render's edge adds its own compression was not verifiable. Lazy loading: the staff screens were already a separate lazily loaded chunk; nothing further was changed.

## 6. Load tests (`loadtest/results/step16-*.md`)
Environment: one shared vCPU, 4 GB, the generator, the server and PostgreSQL all on the same machine. These numbers are a lower bound for this sandbox, not product capacity, and the generator's CPU competed with the server.
- Staged run, pool 4: 100 PASS, 500 PASS, 1,000 FAIL narrowly (connect success 0.989 against a 0.99 threshold; p50/p95/p99 10/79/145 ms; host CPU 84 percent).
- Repeat at 1,000 with DMs every ~6 s: pool 4 PASS (8/49/78 ms), pool 12 PASS (8/74/148 ms). So 1,000 passes or narrowly fails on this machine depending on the run.
- Pool evidence at 1,000 clients: pool 4 recorded 9,762 waits (167 s total wait), pool 12 recorded 3,008 waits (51 s). Fewer waits did not improve latency (p95 49 ms vs 74 ms), and the host was already at 80 percent CPU. Conclusion: on this machine the CPU is the bottleneck, not the pool. The default stays at 4. Raise `DB_MAX_OPEN_CONNS` only after measuring on a separate database machine, and keep instances x limit under the database limit.
- 2,000 and 3,000 were not run. The Step 15 result (pass at 1,500, fail at 2,000 and 3,000 on a shared machine) is a historical baseline only.
- The encrypted DM path was barely exercised: 3 to 8 DMs were sent in these runs (bots look up the contact key first and the windows were short). DM throughput under load is therefore NOT measured.
- Not done: separate generator machine, soak test, database interruption.
- The first attempt failed with `server_busy` for every bot: the test server must run with `REQUIRE_PASSWORD=0`, because the bots sign in as guests. That was a setup error, not a regression.

## 7. Known issues and incomplete requirements
- Multi-device E2EE is not supported (documented, visible in Settings).
- Staff complaint against the only admin has no independent reviewer. Unresolved.
- Frozen admin balances (if any) need an operator decision.
- DM search: no DM search endpoint was found in the server code that stores or reads plaintext; none was added. Not separately audited beyond that.
- Real-browser tests, DB interruption and soak tests are outstanding.
- E2EE has had no independent cryptographic review.

## 8. Deploy and migrate
1. Back up the database (Supabase dashboard or `pg_dump`).
2. Read-only checks: run `scripts/admin_wallet_report.sql`; `SELECT count(*) FROM dm_messages` to know how many legacy rows exist.
3. Optional env changes: `DM_DAYS` and `HISTORY_DAYS` default to 7 now (remove old overrides of 30 if you want the new policy). Pool variables are optional.
4. Deploy the new image. The schema additions apply on startup and are safe to repeat. Watch the log for `database pool:` and `cleanup removed ...`.
5. Existing conversations: people must open the app once so a key is created. Until both sides have keys, sending shows "has not set up encrypted messages yet".
6. Legacy plaintext DMs disappear within 7 days through normal cleanup.
7. Rollback: the old version still runs against the new schema (columns are additive), but it cannot read encrypted rows.
