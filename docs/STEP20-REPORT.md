# STEP 20 report: room discovery, favourites, recent rooms and the member list

Built on the Step 19 bug fix code (`streetsync-step19-bugfix.zip`). The poll creation fix and the room presence fixes are untouched and their tests still pass. Everything below says what was run in this environment (Ubuntu 24, Go 1.22.2, Postgres 16, Node 22). Nothing is claimed that was not run.

## Decisions taken (read these first)

- **"Online" in the member list means in this room now.** It follows the same facts as the join and leave notices, so it obeys the 60 second grace period (and the 5 minute background hold), several tabs and devices, and the one room per account rule. It is not "connected to the app somewhere". A member who is chatting in another room shows as Offline here.
- **Who is listed.** Community room: the host and everyone whose Joined state has not ended, plus people there now. Official room: only people there now (official rooms have no membership). Deleted accounts are never listed.
- **Switching rooms (the Step 19 rule) ends the Joined state of the old community room**, so the person drops out of that room's list. The saved right to enter again (invite, password, fee) is not touched. A test checks the database row stays.
- **Online-visibility privacy.** A person who hides their online state is shown as "Status hidden" in rooms they belong to, whether they are in the room or not, and **no presence frame is ever sent about them** (a frame at the moment they arrive or leave would give them away). A hidden visitor in a room they do not belong to (an official room) is not listed at all. Until a person's settings are known they count as hidden, the same rule the conversation header already uses.
- **Trending is hidden when a sort other than "Most active" is chosen**, so the list really is in the order the server ranked. With the default sort the page is as before.
- Not changed, but worth knowing: anyone who entered a community room before keeps a standing right to re-enter it, so a host who adds a password later does not lock out earlier members. That is Step 18 behaviour and I did not change it.

## Features

### 1. Favourite rooms
- `fav {r, fav}` toggles a favourite. Saved first, then confirmed, for every tab and device. Limit 50 (`favLimit`), over it answers `fav_full`. Duplicates and two tabs toggling at once leave one answer, the same in memory and in the database (a per-person lock).
- A room the person may not see (a private room, or a password room they never got into) answers `no_such_room`, the same as a room that does not exist, so favouriting is not a way to probe for rooms.
- Dedicated **Favourites** section on the Rooms page, a star on every room tile, and a Favourite button inside the room.
- A deleted or closed room takes its favourite rows with it (database cascade). The page never shows a stale entry: the server drops entries for rooms that are gone or that the person may no longer see.

### 2. Recently visited rooms
- Recorded only after an entry went through every check (password, fee, tier, ban, membership), so a refused entry leaves nothing. The newest 15 are kept (`recentLimit`) and persist across refreshes and logins.
- The list is a shortcut, never a key: opening an entry sends the normal join, so every rule applies today. Entries for rooms the person can no longer see are left out.

### 3. Discovery and sorting
- `dir` and `find` take `sort`: `active` (people in the room now, then last message), `recent` (last message), `new` (creation time, newly stored from `rooms.created_at`). Anything else is `active`. The answer names the order used. Ranking happens on the server from live data; the page shows the order as sent.
- Private and password protected rooms stay out of the directory and out of search results in every sort (tested, including searching their exact names). Categories, text search and filters are unchanged.

### 4. Member list
- A People button in the room opens a sheet (bottom sheet on a phone, a side panel on a wide screen) with Close, Escape to close, and Show more.
- `members {off}` is answered only for the room the person is in, so it can never look into a room they could not enter. People there now come from memory, the others from the database 40 at a time, with one count query on the first page. Rate limited (one request per 300 ms per person).
- Each entry has only: handle, avatar look, status, mod or admin badge, host flag. No ids, emails, tiers, points or credits.
- Status is written (Online, Offline, Status hidden) **and** shown by a different mark (filled circle, empty circle, half circle), so colour is not the only signal.
- Live: `mp` frames move people between groups as the join and leave notices happen, add newcomers, and remove people who left the room. They go only to connections that have the list open, and the list closes when the person changes room.
- Nothing here awards points or credits (tested).

## Files and database changes
New: `discover20.go`, `migrations/020_step20.sql`, `step20_test.go`, `ui/src/lib/rooms20.js`, `ui/src/components/MemberList.svelte`, `ui/tests/rooms20.test.mjs`, `tests/page_step20.test.js`, this report.
Changed (Go): `hub.go` (fields, new frame fields), `store.go` (rooms read `created_at`), `rooms_user.go` (directory and search use the shared ranking, creation time on new rooms, member removal notice), `rooms15.go` (visit recording, member removal notice), `presence.go` and `presence_online.go` (member list hooks), `ws.go` (`fav`, `myrooms`, `members`, `mclose`; saved rooms load on connect), `schema.sql` (020 appended).
Changed (page): `chat.svelte.js`, `Rooms.svelte`, `RoomTile.svelte`, `Room.svelte`, `ui/src/styles/step19.css`. Rebuilt `web/`, `ui/dist`, `ui/dist-full`, `tests/build/page.html`.
Changed (tests): one expectation in `tests/page_userrooms.test.js` (the search frame now also carries `sort`). Nothing was loosened.
Database: `room_favourites` and `room_visits`, both with row level security on and `ON DELETE CASCADE` to users and rooms. No existing table or row is changed. Safe to run more than once.

## Tests actually run
| What | Result |
|---|---|
| `gofmt -l .`, `go vet ./...` | clean |
| `go test -count=1 -p 1 -v ./...` | **122 passed, 0 failed** (10 new `TestStep20*`) |
| `go test -race -count=1 -p 1 ./...` | **0 data races, passed** |
| Page tests (jsdom), all files | **942 PASS, 0 FAIL** (37 new in `page_step20.test.js`) |
| `ui` helper tests | 10 passed (5 new) |
| `npm run build` | succeeded |
| Python end to end: chat 58, rooms 31, moderation 53, wallet 30, userrooms 97, tips 39, signup 18, admin 76 | 0 failures |
| Migration | Step 19 bugfix schema with data, `020_step20.sql` run twice and `schema.sql` once more: 0 errors, row counts identical, deleting a room removed its favourite and visit rows, server boots on it |
| Real server over a real WebSocket | favourite saved to the database, recent rooms in the right order, member list correct, a second person's arrival announced live, a closed connection not announced inside the grace period |

New Go tests cover: favourites (add, duplicate, missing flag, unknown room, limit, persistence across connections, other tab updated, two tabs toggling at once, no rewards); a closed room leaving no entry; recent rooms (order, persistence, cap, refused entry not recorded, a now locked room still refused, no rewards); shortcuts not being keys (a host-removed person meeting a password, private and password rooms not favouritable or listed); sorting in all three orders plus default and invalid values and search; private and password rooms absent from directory and search in every sort; the member list (statuses, counts, only allowed fields, host flag, no room, another room's list, official rooms, someone who left the room dropping out, rate limit, no rewards); member status following presence rules (arrival, two tabs, closing one tab, lost connection, reconnect inside the grace period, expiry, a quiet member staying online five grace periods, moving rooms keeping the saved right); online privacy (no frames for hidden people in either direction, "Status hidden" for members, hidden visitors absent, live show and hide); paging (47 people over two pages, no repeats, correct total).

Mutation check: with each of these undone in turn, the matching test failed and the code was restored: announcing hidden people, ignoring the newest sort, letting anyone see private rooms, announcing a member offline at once. (The first attempt at one mutation did not compile, so I redid it in a form that does; the result above is from the redone run.)

## Failed, skipped or unavailable
- `e2e_dm.py`, `e2e_profiles.py`, `e2e_notify.py`: same results as in Step 18 and Step 19 (dm: no PASS lines, profiles: 19 pass 1 fail, notify: 2 pass). Not touched, not investigated.
- Not run: browser layout checks, the load test, a real phone or a slow network (the page tests are jsdom). The bottom sheet and side panel layout is therefore untested in a real browser.
- Two defects of my own were found while testing and fixed before delivery: a race where the background load of saved rooms could overwrite a room visit recorded a moment earlier (for example a page reload that re-enters its room at once), and the privacy gap that a frame about a hidden person would itself reveal their arrival.

## Known limitations
- The member list is a snapshot plus live changes. People joining or leaving between "Show more" requests can shift later pages by a person; the page never shows anyone twice.
- If more than 40 people are online, only the first page of them is loaded at first; the online count is exact and "Show more" loads the rest.
- A person who hides their online state is listed under "Status hidden" for rooms they belong to. That tells others they are a member, which the member list is for, but nothing about whether they are there.
- The first `mr` message after connecting can be an empty list (it is sent as soon as the connection is made, before the database answers); the page applies messages in order, so the final state is right.
- State for presence and room members lives in the single server process, like rooms and presence before it. Two server instances are still unsupported.
- No new configuration is required.
