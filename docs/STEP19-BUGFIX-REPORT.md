# STEP 19 bug fix report: poll creation and false leave notices

Baseline: the Step 19 code (`streetsync-step19.zip`). Only these two bugs were worked on. Everything below states what was run in this environment (Ubuntu 24, Go 1.22.2, Postgres 16, Node 22). I diagnosed first and changed code after.

## Bug 1: "The server is busy" when starting a poll

### The flow that was traced
Tap "Start a poll" in `PollCard.svelte` -> `createPoll` (`chat.svelte.js`) sends `{"t":"pcreate"}` -> `pollCreate` (`engage19.go`) -> permission check (`canManageRoomLocked`: host of a community room, or moderator or admin) -> one open poll rule -> database insert -> broadcast of `{"t":"poll"}`, or an error frame -> the page shows the text for the code.

`server_busy` is the only code `pollCreate` could send on a failed insert, and it sent it for **every** database error, whatever the cause.

### Root cause (reproduced)
The poll options were passed to Postgres as a Go `[]byte` for a `jsonb` column. With `binary_parameters=yes` in the connection string (a setting commonly added behind a connection pooler), `lib/pq` sends `[]byte` as raw bytes, and Postgres reads those bytes as binary jsonb and refuses them. Server log, reproduced with the real server binary:

`creating a poll failed: pq: unsupported jsonb version number 91`

Without that option the same code works, which is why all Step 19 tests passed (the test database URL does not have it) and why a fresh database and an upgraded Step 18 database both worked in my reproductions. Reproduced both ways with the real binary: works without the option, fails every time with it.

What I could **not** do: see your production `DATABASE_URL` or logs. So "your URL contains `binary_parameters=yes`" is the reproduced mechanism that fits "fails every single time", not something I observed on your server. To confirm, search the Render log for `creating a poll failed: pq: unsupported jsonb version number 91` from before the fix. If that line is not there, the log line from the fixed build (it now names the exact database error and SQLSTATE) will say what it is, and the page now says "server fault" for such a failure instead of "busy".

### Fix
- `store.createPoll` (`engage19.go`): options are sent as text with an explicit `$3::jsonb` cast. Works with and without `binary_parameters`. It is the only place that sent JSON as bytes (checked).
- `pollFailCode`: failures are no longer all "busy". A duplicate open poll is `poll_open`. Connection trouble, timeouts, overload and serialization failures stay `server_busy` (retry can help). Everything else (missing table, refused permission, bad value, missing room) is the new code `poll_failed` ("server fault, tell the room admin"). The log line now carries the room, the person, the code and the SQLSTATE.
- Duplicate submissions: the page keeps one request at a time (`chat.pollBusy`): extra taps do nothing, the button reads "Starting..." and is disabled, and it comes back on the answer or after 10 seconds without one. A failure is shown inside the form with what you typed kept for another try. On the server, the same person sending the same question and options within 10 seconds gets the existing poll back instead of an error or a second poll. A different poll while one is open is still `poll_open`.
- Not changed: permissions, the one open poll per room index, voting, closing, live updates.

## Bug 2: people announced as having left while still in the room

### The presence lifecycle that was traced
A person counts as here from the moment their first connection enters the room. A leave notice is made only when (a) Leave room or the back arrow is used, (b) a connection is lost, or (c) a newer room join moves them (Step 19). Nothing looks at messages: there is **no inactivity logic anywhere** (searched the whole server). Both sides keep the connection alive without messages: the page sends a ping every 15 seconds, the server pings every 25.

### What I could and could not reproduce
- A quiet person for 100 seconds with the real binary: no notice. Refresh within a second: no notice. Closing a second tab: no notice. A 120 step randomized run of opens, refreshes, drops, back arrows and silences, repeated: notices alternate correctly and match the truth. So **silence itself never produced a leave, and I could not reproduce a false leave for a person whose connection is open.**
- What can make a still-present person get announced is a connection that is gone for more than 60 seconds. Real cause on phones: a page that is in the background or has the screen locked has its connection cut by the system, and people who only read are exactly the ones whose screens time out. That was treated the same as a network loss. This is the most likely real trigger of the symptom you describe. I could not test it on a real phone, so it is reasoned from the symptom and the code, not observed.

### Gaps found and fixed (all in the presence code)
1. **Backgrounded page vs network loss.** The page now tells the server when it goes to the background and comes back (`bg` and `fg` frames, on the browser's visibility event, also after a reconnect that happens while hidden). A connection lost while the page said it was in the background is kept in the room for `backgroundGrace` (5 minutes, `presence.go`), not announced at 60 seconds. Network loss, refresh and everything else keep the 60 second grace (unchanged). Not a global timeout increase.
2. **Expiry did not confirm the person's other connection.** When the grace timer ran out it only checked whether a connection was already in the room. A connection that had reconnected and was still being put back into the room (the reconnect names its room in the first message, the join finishes a moment later) did not count, so the old loss was announced and then a join. Now such a connection is tracked (`entering`) and the notice waits one more `enterSettle` (15 seconds) and then decides on the facts.
3. **Leave room only removed the tab pressed.** Now it removes the person from the room in every tab and device, once, at once, and the other tabs are told (the community room Leave already worked that way). The back arrow stays per tab.
4. **A login ended on purpose, a ban or an account deletion waited 60 seconds** as if the network had dropped. Now the notice is immediate for those (the person is not coming back).
- Kept: 60 second grace for network loss, silent reconnect inside it, closing one tab never announces while another remains, counts use distinct accounts, notices never earn points or credits, room switching, access rights.
- Contradictory or duplicate notices: every decision is made under the hub lock with a single timer per person and room; a randomized concurrency run and the tests below check that notices alternate and match the truth.

## Files changed
Go: `engage19.go` (createPoll, pollFailCode, duplicate request handling), `presence.go` (departure kinds, background grace, entering check, `setHidden`), `hub.go` (client fields, account-wide explicit leave, detach), `ws.go` (`bg` and `fg` frames, entering marker, logout marker).
Page: `ui/src/lib/chat.svelte.js` (poll request state, inline error, `poll_failed` text, bg and fg hints), `ui/src/components/PollCard.svelte`. Rebuilt `web/`, `ui/dist`, `ui/dist-full`, `tests/build/page.html`.
Tests: new `bugfix_test.go` (13 tests) and `presence_stress_test.go` (randomized lifecycle), helper `setGraces` in `step19_test.go`, 10 new checks in `tests/page_step19.test.js`. No existing test was edited or loosened.
Docs: README, HANDOVER, this report. No database change and no new dependency.

## Tests actually run
| What | Result |
|---|---|
| `gofmt -l .`, `go vet ./...` | clean |
| `go test -count=1 -p 1 -v ./...` | **112 passed, 0 failed** (14 new `TestBugfix*` and randomized lifecycle) |
| `go test -race -count=1 -p 1 ./...` | **0 data races, passed** |
| Page tests (jsdom), all files | **905 PASS, 0 FAIL** (52 in `page_step19.test.js`) |
| `ui` helper tests | 5 passed |
| `npm run build` (site and full) | succeeded |
| Python end to end: rooms 31, moderation 53, wallet 30, userrooms 97, tips 39, signup 18, admin 76 | 0 failures |
| `e2e_chat.py` | 58 pass, 0 fail on reruns. **One run directly after the Go suite showed 2 failures** (`me frame shows Regular`, `points saved during shutdown`). It passed twice on rerun and passes on the untouched Step 18 binary. I read it as leftover rows from the Go tests in the shared database, not as a regression, but I did not prove that. |
| `e2e_dm.py`, `e2e_profiles.py`, `e2e_notify.py` | same results as before Step 19 and on the Step 18 binary (dm: no PASS lines, profiles: 19 pass 1 fail, notify: 2 pass). Not touched. |
| Replay of the original poll failure (real binary, `binary_parameters=yes`) | before the fix: `server_busy`. After: the poll is created. |

Mutation check: with each fix undone in turn (bytes again, no entering check, background ignored, explicit leave per tab, logout not immediate, duplicates not recognised, every failure called busy), the matching regression test failed. All fixes restored afterwards.

New regression tests cover. Polls: authorized creation (host, moderator), refused creation (member, person in no room, member in an official room, nothing stored), the insert in both connection modes plus the negative control that the old way fails, failure code table, a missing table giving `poll_failed` and recovering afterwards, double taps and a second tab giving one poll, a different poll while one is open refused, voting and closing still working. Presence: quiet person stays past many grace periods with correct counts and no points change, network loss silent then announced once after the grace period, reconnect inside it silent, backgrounded page kept longer then announced once, foreground loss at 60s rules, several tabs and back arrow never announcing, explicit leave immediate for the person in every tab, logout immediate, a connection on its way in deferring then deciding, and the randomized concurrency run.

## Remaining limitations
- Bug 2's most likely real trigger (a locked phone) was not reproduced on a device. If false leave notices still appear after this, the new log lines and `bg` and `fg` hints make the next step clear, but I would need a real device or server logs.
- A page backgrounded for more than 5 minutes without coming back is announced as left. A page backgrounded whose connection the system does not cut stays as it is. `bg` is a hint from the page: a modified client could claim it to stay up to 5 minutes, which is bounded and harmless.
- A poll that failed for a reason other than `binary_parameters` (for example a missing migration or permissions) now reports `poll_failed` and logs the SQLSTATE, but I could not see your production database to check it.
- If your `DATABASE_URL` has `binary_parameters=yes`, the Step 19 reaction and pin queries are not affected (they do not send JSON bytes). I checked every `[]byte` and JSON parameter in the server.
