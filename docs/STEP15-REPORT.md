# Step 15 completion report

## Test results (actually executed, on the final build)

| Suite | Result |
|---|---|
| `go vet ./...`, `gofmt -l` | clean |
| `go test ./...` (56 top level tests, database backed) | **all pass**; also pass with `-race` |
| New Go integration tests over real WebSocket frames (`step15_test.go`, `step15b_test.go`, `step15c_test.go`) | pass (staff shield and hierarchy, ban, deletion, retries, rooms, search) |
| `loadtest` safety unit test | pass |
| e2e scripts: chat 58, moderation 53, rooms 61, wallet 30, dm 68, userrooms 97, tips 39, profiles 38, admin 76, signup 18, notify 66 | **all pass, 0 failed** |
| Page tests (jsdom): 13 files incl. new `page_step15` (41 checks) | **all pass, 0 failed** |
| Migration of a Step 14 database with data, run twice | no data lost, no errors, ledger key changed to NO ACTION |
| Load test | smoke 100 (pass), staged 100, 500, 1000, 1500 pass; 2000 and 3000 fail (see `LOADTEST-REPORT.md`) |

**Not run:** `tests/browser_layout.py` (needs Playwright and Chromium, not available here); no test against a real hosted Supabase database; no soak test; the load test generator ran on the same single CPU as the server and database.

Failures met and resolved along the way: two baseline Go test failures and an unreadable inbox badge were one real bug (a query passed two arguments for one placeholder); five existing page tests and one e2e script had expectations that the new rules change on purpose (staff messages can no longer be tipped by members, the report menu is now a sheet, `croom` carries `d`, staff messages are no longer reportable the ordinary way) and were updated. An e2e run that failed once was a leftover server from an earlier hung run holding the port.

## Role and staff findings

Existing code already stopped moderators from acting on admins in `modAction`, `hostMod` and `deleteMsg`. Gaps found and fixed:
1. Tips, direct messages, friend requests and reports to staff had no check at all (HTTP does not exist; all paths are WebSocket frames). Now refused in the server and inside the SQL transactions.
2. **`deleteReportedDM`: any moderator could delete any direct message by number, including an admin's, whether reported or not.** Now only reported messages, and never above the actor's rank.
3. A host could delete the older message of someone later promoted to staff (stored role empty). The current role is now read from the database.
4. Reports about staff were visible to moderators. They now go to a separate admin-only complaint channel; moderators' queue and counter exclude reports about staff.
5. A message could be reported by id from any room (probing other rooms). Now only from the room you are in.
6. Promote and demote were admin only already; ban and unban are too. Staff cannot earn entry fees (rooms of staff are free).
7. Guest pruning would now fail on the ledger key, so it was made ledger-safe.

## Files changed (summary)

Backend new: `policy.go`, `store15.go`, `handlers15.go`, `rooms15.go`. Changed: `schema.sql`, `store.go`, `hub.go`, `ws.go`, `mod.go`, `dm.go`, `dm_store.go`, `tips.go`, `profiles.go`, `rooms_user.go`, `rooms_store.go`, `admin.go`. Goroutines that touch the database now use `safeGo` (`recover.go` was already present in this zip, together with the five `ws.go` replacements you listed; `dm.go`, `notify.go`, `hub.go` got more). Not changed: the database driver and connection code.
Frontend: `ui/src/lib/chat.svelte.js`, `App.svelte`, new `ReportSheet`, `ManageRoom`, `StaffComplaints`; changed `Hub`, `Rooms`, `RoomTile`, `CreateRoom`, `Message`, `UserProfile`, `Settings`, `StaffReports`, `StaffUsers`, `StaffArea`, styles; `web/` and `tests/build/page.html` rebuilt.
Tests/tools/docs: `*_test.go` (new files above, updated `store_test.go`), `tests/page_step15.test.js` and updated page and e2e tests, `loadtest/`, `scripts/loadtest-local.sh`, `README.md`, `HANDOVER.md`, `tests/README.md`, `docs/`.

## Remaining issues and decisions for you

- Settings: **Appearance** was not added: the app has a single theme, so there is nothing real to switch. Account (bio), Privacy, Notifications and Security already existed from step 14 and were reused. Added Help and Delete account.
- Rooms: no ownership transfer on deletion (rooms are closed). Bans are per account. A complaint about the only admin has no independent reader.
- Hub still shows free public rooms only while people are in them (existing decision); joined state shows on the card.
- `main.go` still starts three long-running goroutines (`persistLoop`, `run`, shutdown) with plain `go`; they are the process lifecycle and I left them as they were.
- Load test: highest passing level 1,500 on one shared 1 vCPU machine. Repeat on separate machines before drawing conclusions about 3,000.
