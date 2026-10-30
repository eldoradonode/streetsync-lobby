# Tests

Three kinds, all kept here so any new chat or developer can run them.

| What | Files | Needs |
|---|---|---|
| Go unit and database tests, and in-process WebSocket integration tests | `../*_test.go`, `../loadtest/*_test.go` | Go, and Postgres for the database ones |
| End to end tests of the real server | `e2e_notify.py`, `e2e_chat.py`, `e2e_moderation.py`, `e2e_rooms.py`, `e2e_wallet.py`, `e2e_dm.py`, `e2e_userrooms.py`, `e2e_tips.py`, `e2e_profiles.py`, `e2e_signup.py`, `e2e_admin.py` | Go, Postgres, `psql`, Python with `websockets` |
| Page tests in a simulated browser | `page_*.test.js` | Node and `jsdom` |

**Warning:** the end to end scripts and the Go database tests delete data. Run them only against a throwaway
local database, never against Supabase or any real data.

## What the step 15 tests cover

- `../step15_test.go`: staff shield and role hierarchy over real WebSocket frames (tips, DMs, friend requests, reports, complaints, moderation, ban and unban), with checks that rejected requests change nothing (balances, ledger, friendships, messages, reports). `harness_test.go` is the in-process server and client they use.
- `../step15b_test.go`: account deletion (wrong password, forged and foreign tokens, two stages, closed connections, dead tokens, ledger preserved and still adding up, foreign keys, room ownership, concurrent retries, guests, pruning).
- `../step15c_test.go`: room descriptions, join, leave, rejoin without a second fee, parallel joins, management page, host limits, room reports, people search and public profiles.
- `page_step15.test.js`: Settings deletion flow, Help, create room description, home Featured and categories, people tab, profile bio and staff shield, report sheet, staff complaint, joined and Leave, ban screen.
- `../loadtest/safety_test.go`: the load tool refuses production and unconfirmed hosts.

## One time setup (Ubuntu)

```
apt-get update && apt-get install -y postgresql postgresql-client
pg_ctlcluster 16 main start        # use your installed version number
su postgres -c "psql -c \"CREATE USER lobby WITH PASSWORD 'lobby';\" -c \"CREATE DATABASE lobbytest OWNER lobby;\""
pip install websockets
cd tests && npm install && cd ..
```

If processes stop between commands (some sandboxes do this), start Postgres again before each run.

## Run them (from the project folder)

```
# Go tests, including the database ones
TEST_DATABASE_URL="postgres://lobby:lobby@localhost/lobbytest?sslmode=disable" go test -count=1 ./...

# End to end: build the server to ./lobby-test first, then run each script
go build -o lobby-test .
python3 tests/e2e_chat.py
python3 tests/e2e_moderation.py
python3 tests/e2e_rooms.py
python3 tests/e2e_wallet.py
python3 tests/e2e_dm.py
python3 tests/e2e_userrooms.py
python3 tests/e2e_tips.py
python3 tests/e2e_profiles.py
python3 tests/e2e_admin.py    # roles, permissions, admin queries, bio, categories, directory
python3 tests/e2e_signup.py   # runs with REQUIRE_PASSWORD on; all the others turn it off

# The same with Go's race detector (needs gcc)
CGO_ENABLED=1 go build -race -o lobby-test .

# Page tests (they read tests/build/page.html, the one file test build written by `npm run build`)
cd tests && for f in page_*.test.js; do node $f | tail -1; done
```

Each script prints PASS or FAIL per check and a total at the end. The scripts use ports 8095 to 8102, and
write server logs to `/tmp`. If a script crashes it can leave a server running: stop it with `pkill -x lobby-test`.

## Notes that save time

- Page tests call `window.__lobbyFlush()` because Svelte redraws one tick after a change. The harness does this for you.
- A node script that mounts the page keeps a timer alive. End it with `process.exit()`.
- In a sandbox, never use `pkill -f` with the binary path: it can match and kill your own shell. Use `pkill -x`.
- When a check fails, first ask whether the test or the server is wrong. Many early failures were wrong expectations
  in the test, but one real bug (reconnecting while removed) was found this way.

## Real browser layout checks

`python3 tests/browser_layout.py` opens the built page in headless Chromium (needs `pip install playwright`; the browser
is found automatically in the sandbox) with a fake socket and checks the layout: bottom tabs on screen on phones, no
page scroll, no sideways overflow, composer above a shrunken (keyboard sized) screen, sidebar and room grid on desktop.
Run it after every `npm run build`.

## Server and real browser checks (step 14)

- `python3 tests/e2e_notify.py` notifications, settings, privacy, blocking and password change, against the real server.
- `python3 tests/site_check.py` starts the real server and fetches the site as a browser does: compression, cache headers,
  "not modified", what is not served, and prints the bytes actually sent. Needs a fresh `go build -o lobby-test .` after `npm run build`.
- `python3 tests/browser_layout.py` now serves the real production build (`web/`) over http and checks in Chromium that the staff
  screens are downloaded only on demand and only once, that a failed download is shown, the two pane layout, notifications, settings,
  and that no page throws an error.

## Step 17 tests

- `../step17_test.go`: DM receipts (sent, delivered, read over real frames; duplicates, late and out of order events; offline and hidden recipients; unauthorized receipts; blocked sender), room notices (join, duplicate tabs, disconnect cleanup, grace period, reconnect, explicit leave, no storage and no points), membership leave, search echo and case insensitive people search.
- `page_step17.test.js`: phone style search (capitalised first letter, debounce, stale answers, counts, empty states, slow down), Create room description, homepage without the category grid, ticks and receipt handling, hidden page behaviour, Leave room and join/leave lines.
- Known: `e2e_dm.py` already timed out on the Step 16 server (it sends plaintext direct messages, which are refused now). It was not changed.
- Not covered anywhere: a real browser or phone. `browser_layout.py` needs Playwright and Chromium, which could not be installed when Step 17 was built.

## What the step 19 tests cover

- `../step19_test.go`: one room per account across tabs and devices, 60 second grace, silent reconnect, accurate counts, explicit leave, Joined state moves while access rows stay, pin permissions and sync, reactions, polls, concurrent votes and reactions, and that none of it earns anything.
- `page_step19.test.js`: pin banner, reaction chips and picker, poll card and form, validation on the page, nothing rendered as markup, state cleared when changing room.
- `../ui/tests/engage.test.mjs`: pure helpers (`cd ui && npm test`).
- `page_mod.test.js` and `page_userrooms.test.js` now expect "Pin to room" in the message menu for hosts and moderators.

## Bug fix tests (after step 19)

- `../bugfix_test.go`: poll insert in both connection modes (with the negative control), failure codes, missing table, permissions, duplicate requests; quiet person, network loss, backgrounded page, several tabs, explicit leave, logout, connection on its way in.
- `../presence_stress_test.go`: randomized lifecycle of one person across tabs, refreshes, drops, background losses and Leave room, checked against what an observer is told.
- `page_step19.test.js`: poll button state and errors, and the bg and fg hints.

## Step 20 tests

- `../step20_test.go`: favourites, closed rooms, recent rooms, shortcuts not being keys, directory sorting, private and password rooms staying out of discovery, the member list, its live updates under the presence rules, online privacy, paging.
- `page_step20.test.js`: Favourites and Recently visited sections, stars, sort choices and server order, the member list sheet, status labels and marks, live updates, paging, closing.
- `../ui/tests/rooms20.test.mjs`: pure helpers (`cd ui && npm test`).
