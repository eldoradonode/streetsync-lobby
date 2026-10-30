# STEP 19 report: Room engagement

Built on the Step 18 code (`streetsync-step18.zip`). Everything below says what was actually run in this build environment (Ubuntu 24, Go 1.22.2, Postgres 16, Node 22.22, 1 CPU). Nothing is claimed that was not run.

## Decisions taken before building

- **One room per account means presence and the Joined state.** Joining a room moves the account's presence out of every other room (all tabs and devices) and ends its Joined state in the other community rooms. The right to come back without paying again (`room_members` row, `ou.rooms`) is **kept**, exactly like the existing Leave room button, so nobody pays twice. Rooms a person hosts are never left this way. This was confirmed with you before the build.
- **Who may manage** (pin, unpin, create and close polls): the host of a community room, and moderators and admins everywhere. The platform has no per-room moderator role, so official rooms are staff only. Members cannot do any of it.

## Features implemented

### 1. Room membership and leave notices
- `joinRoom` (hub.go) now runs under the hub lock and removes every connection of the account from every other room before the join completes. Other tabs receive `{"t":"left","w":"moved"}` and land on the hub. A "left" notice still pending for another room (grace running) is sent immediately, once, and the old timer cannot send a second one.
- Two devices joining different rooms at the same moment are serialized (hub lock for presence, a per-account `enterMu` for the whole entry including the Joined state). The last join wins, and the account is in exactly one room.
- `enter` finishes through `settle`: join, then `leaveOtherJoined` ends the old Joined state (database `left_at`, access right kept) and pushes the new room list.
- Reconnection grace is now **60 seconds** (`graceLeave` in presence.go, was 30). Reconnecting inside it restores presence with no join or leave notice. Not reconnecting sends one leave notice. Silence never leaves: there is no idle logic anywhere (checked, none exists).
- Back arrow still only starts the grace period. The Leave room button still leaves at once. Counts use distinct accounts, so two tabs count once.
- Join, Joining and Joined UI states are untouched. System notices still never earn points (still only `post()` scores).
- `graceDur()` plus `graceMu` make the grace value safe to read while tests change it.

### 2. Pinned message
- One pin per room (`room_pins`, primary key on room). Pinning another message replaces it. Pinning the same message again does nothing and sends nothing.
- Server enforced (`pinMsg`, `unpinMsg` in engage19.go). Tip cards cannot be pinned. A message that is not in the room, or was deleted, is refused.
- The pin stores a snapshot of sender and text, so an announcement survives the 7 day history cleanup. Deleting the pinned message (moderator delete, own delete, account deletion) clears the pin and tells the room.
- Everyone in the room gets `{"t":"pin"}` live. Joiners get the pin inside one `x19` frame.
- UI: compact banner under the Joined pill. Tap scrolls to and flashes the message, or opens the full text in place when the message is no longer on screen. Hosts and moderators get an unpin button and a "Pin to room" action in the message menu. Text is always rendered as text.

### 3. Reactions
- Six fixed emoji (server list in engage19.go, same list in `ui/src/lib/engage.js`). No emoji library.
- One reaction per account per message (primary key `msg_id, user_id`). Pick another to change it, tap your own to remove it. This is stricter than "no duplicate per emoji", on purpose.
- Server checks: in a room, membership of community rooms, not muted (global or room), 12 actions per 5 seconds, message exists in this room's buffer and is not deleted. Anything outside the six emoji is refused.
- Counts are broadcast after the database write. Joiners get the reactions of the messages in their history snapshot with one query inside the `x19` frame. Persisted, so refresh and reconnect show the same numbers.
- Reactions never touch `ou.score`, credits, tiers or messages. DMs and E2EE are not involved.
- UI: chips under the message (mine highlighted), a small smiley button in the message header opens a 6 button picker.

### 4. Polls
- Question 3 to 140 characters, 2 to 6 options of 1 to 60 characters, case-insensitive duplicates refused. Same limits on the page and on the server.
- One open poll per room (partial unique index `polls_one_open`). Votes: one row per account per poll (primary key). Changing your vote is supported and moves the vote, it never adds one.
- A vote only lands while the poll is open in the database (conditional insert). Votes and results survive refresh and reconnect (`loadEngagement`). Voters are never shown to others. Each person gets their own choice back; everyone gets updated totals.
- Create and close are host or staff only. Closing twice is harmless. Closed polls older than the history window are deleted by the existing cleanup job.
- UI: compact card (question, option bars with percent and count, total), fold button, "Start a poll" form for managers, two tap close.

All changes in one room are serialized by `rm.engMu` (database call included), and the hub lock is never held during a query.

## Files and database changes

New: `engage19.go`, `migrations/019_step19.sql`, `step19_test.go`, `ui/src/lib/engage.js`, `ui/src/components/PinBanner.svelte`, `ui/src/components/PollCard.svelte`, `ui/src/styles/step19.css`, `ui/tests/engage.test.mjs`, `tests/page_step19.test.js`, this report.

Changed (Go): `hub.go` (joinRoom, room and user fields, new inbound fields, load), `rooms15.go` (settle, leaveOtherJoined), `rooms_user.go` (enter uses entry lock and settle), `presence.go` (60s, accessor), `ws.go` (six new frame types), `store.go` and `store15.go` (clean up reactions, pins and votes with messages and accounts), `handlers15.go` (account deletion adjusts live pin and poll), `mod.go` (unpin on delete), `retention.go` (`pruneEngagement`), `schema.sql` (019 appended).
Changed (page): `chat.svelte.js`, `Room.svelte`, `Message.svelte`, `main.js`, `ui/package.json` (test script). Rebuilt: `web/`, `ui/dist`, `ui/dist-full`, `tests/build/page.html`.
Changed (tests): `store_test.go` (clears the new tables between tests), `page_mod.test.js` and `page_userrooms.test.js` (the message menu now lists "Pin to room" for hosts and moderators, so three expected lists gained that item; no assertion was loosened and a regular member's menu is still asserted unchanged).

Database: four new tables `room_pins`, `message_reactions`, `polls`, `poll_votes`, indexes, row level security enabled like the other tables. No existing table or row is changed or deleted. There is deliberately no foreign key to `messages` (messages are saved in batches after they are sent). Reactions, polls and pins cascade with rooms; reactions and votes cascade with deleted users.

## Tests actually executed

| What | Result |
|---|---|
| Step 18 baseline, before any change (`go test ./...`, pristine copy) | all passed (`ok lobby 29.2s`) |
| `gofmt -l .` and `go vet ./...` | clean |
| `go test -count=1 -p 1 -v ./...` | **98 passed, 0 failed** (12 are new Step 19 tests) |
| `go test -race -count=1 -p 1 ./...` | **0 data races, all passed** |
| Step 19 tests on the untouched Step 18 code | the 4 one room and 60 second tests fail there, as they should (30s grace, no eviction), pass on Step 19 |
| Step 19 tests repeated under `-race` | 6 consecutive clean runs after the last fix |
| Page tests in jsdom (`tests/page_*.test.js`, `e2ee_interop.test.js`) | **895 PASS, 0 FAIL** (42 are new in `page_step19.test.js`) |
| `ui` helper tests (`npm test`, node:test) | 5 passed |
| Production builds `npm run build` (site and full) | succeeded |
| Python end to end against the real server binary | chat 58, moderation 53, rooms 31, wallet 30, userrooms 97, tips 39, signup 18, admin 76: all 0 failures |
| Migration | Step 18 schema loaded with data, `019_step19.sql` run twice, `schema.sql` run again: 0 errors, row counts identical, server starts on it |
| ZIP | extracted into a clean directory and built, see the delivery check at the end of this file |

New Go tests (`step19_test.go`): grace is 60s; one room across tabs (moved tab, one leave notice, one join notice, counts, duplicate join, same account twice counts once, no points or messages); 20 rounds of simultaneous joins from two connections of one account; a pending leave flushed when the person moves; reconnect inside grace is silent, silence never leaves, no reconnect leaves once; Joined state moves and the access row is kept; explicit leave is immediate and the back arrow is graceful; pin permissions, duplicate pin, replace, delete clears, persistence, late joiner, official rooms staff only, no rewards; reactions (add, change, remove, duplicates, bad emoji, missing message, outside room, no membership, persistence, delete, 12 people at once plus one account from two tabs, no rewards); polls (permissions, 7 kinds of bad input, one open poll, vote, change vote, forged votes, other room, persistence, close, vote after close, new poll after close, no rewards); 12 voters at once plus a vote racing a close, with live counts checked against the database.

## Failed, skipped or unavailable

- **`e2e_dm.py`, `e2e_profiles.py`, `e2e_notify.py`**: still fail exactly as documented in the Step 18 report. I ran them against the untouched Step 18 binary as well and got identical results (dm: no PASS lines, profiles: 19 pass 1 fail "a direct message carries the sender's look", notify: 2 pass). I did not fix or investigate them. Not weakened.
- **One flaky Step 18 test seen once**: `TestStep18Presence` failed in one full run that I was running while page tests and builds loaded the single CPU. It passed 8 of 8 times alone, on both the Step 19 and the untouched Step 18 code, and in both final full runs. It has 150 ms timing windows, so it is load sensitive. Treat as pre-existing timing sensitivity, not proven fixed or proven unrelated beyond that evidence.
- **Test races found and fixed**: my first versions of two tests changed the grace value while a previous test's connections were closing. Fixed with `graceMu`. Separately, in subset runs under `-race` the existing Step 18 test assigns `presenceGrace` without a lock and raced once with a closing connection (`step18_test.go:117`). I left that test alone; the full-suite race runs were clean.
- **Not run**: browser layout and site checks (`tests/browser_layout.py`, `site_check.py`, need a real browser), the load test, any real phone or slow network, and Supabase (migration tested on local Postgres 16 only). Page tests are jsdom, not a real browser.
- A Python dependency (`websockets`) was installed into the sandbox to run the end to end scripts. Go modules were fetched straight from GitHub because the module proxy was blocked; `go.mod` and `go.sum` are unchanged.

## Known limitations and deployment

- Required configuration: none new. The migration is in `schema.sql`, which the server applies at start, and also as `migrations/019_step19.sql` for manual runs. It is safe to repeat.
- State lives in the single server process (like rooms and presence in Step 18). Running two server instances is still unsupported. The one open poll per room rule is also enforced by the database.
- You can react only to messages still in the room's in-memory buffer (200). Older messages answer "already gone".
- A reaction count on a message does not shrink live for connected people when an account that reacted is deleted; it is correct after the next join. Votes of a deleted account do adjust live counts. Guest cleanup (which hard deletes guests) can leave live poll counts stale until restart.
- A closed poll removed by the cleanup job stays on screen in rooms that are open until the server restarts.
- Pins and polls are one each per room by design. There is no pin history and no poll history screen.
- Switching rooms ends the Joined state of the previous community room. That is the literal reading of the requirement and you accepted it, but people who hop between rooms will see Joined disappear from the room they left (their right to re-enter free stays).

## Step 18 preserved

Go and page and end to end suites above pass except the three scripts that already failed. Join, Joining, Joined states, Leave room, back arrow, DMs and E2EE (including `e2ee_interop` and the no-plaintext Go test), read receipts, online status, notifications, profiles, wallets, tips, moderation and administration were all exercised by those suites. The only existing assertions edited are the three message-menu lists named above.

## Delivery check

`streetsync-step19.zip` was extracted into a clean directory and, from its source alone: `go build ./...` and `go vet ./...` passed and `gofmt -l .` listed nothing; `npm ci`, `npm run build` and `npm test` in `ui` passed (5 of 5); the rebuilt `web/` was byte identical to the shipped one; `go.mod` and `go.sum` are unchanged from Step 18; `go test -run Step19` passed (12 tests); `page_step19.test.js` (42) and `page_mod.test.js` (59) passed. The ZIP leaves out `node_modules` and the test server binary. The full suites in the table above were run in the working copy that the ZIP was made from, not repeated in the extracted copy.
