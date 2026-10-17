# Tests

Three kinds, all kept here so any new chat or developer can run them.

| What | Files | Needs |
|---|---|---|
| Go unit and database tests | `../*_test.go` | Go, and Postgres for the database ones |
| End to end tests of the real server | `e2e_chat.py`, `e2e_moderation.py`, `e2e_rooms.py`, `e2e_wallet.py`, `e2e_dm.py`, `e2e_userrooms.py`, `e2e_tips.py`, `e2e_profiles.py`, `e2e_signup.py`, `e2e_admin.py` | Go, Postgres, `psql`, Python with `websockets` |
| Page tests in a simulated browser | `page_*.test.js` | Node and `jsdom` |

**Warning:** the end to end scripts and the Go database tests delete data. Run them only against a throwaway
local database, never against Supabase or any real data.

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

# Page tests (they read web/index.html)
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
