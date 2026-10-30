# STREETSYNC (through step 15)

A hub of chat rooms with a database behind it, and the web interface that goes with it.

- Pick an @handle and chat. Guests need nothing else.
- Protect your handle with a password to log in from any device.
- Real messages earn points. Points move you through five tiers: Newbie, Regular, Insider, Elite, Legend.
  The tier badge shows on every message.
- Credits live in a wallet. New accounts get 20, and real chatting earns more. Every movement is kept in a ledger.
- Friends and direct messages: an Inbox tab with chats, friend requests and a block list.
- A landing page on the sign in screen, avatars you pick at sign up, and a profile page for anyone you tap.
- Tips: send a treat (1, 5, 10, 25 or 50 credits) to someone in a room. It shows as a highlighted card in the feed.
- A new look: deep navy with cyan and violet, a sidebar on wide screens and a bottom bar on phones.
- Rooms people create: public or private, optional password and entry fee, with the host moderating their own room.
- Handles, sessions, points, credits and the last days of chat survive restarts and sleeps.
- Step 15: room descriptions, a Featured section and category tiles, people search with public profiles, room membership
  (join, leave, rejoin without paying again) and a room management page for hosts, reports about messages, people and
  rooms, permanent bans, a protected channel for reporting staff, account deletion, and a repeatable load test.

## The interface

The web app is written in Svelte and lives in `ui/`. Its screens are the sign in page, the **Hub** (room list),
the **room** (chat) and the **Profile** (avatar, tier and points, password, log out, and the reports panel for
moderators). Everyone gets an illustrated avatar made from their handle, so nothing is downloaded for it.

`web/` is the built result: `index.html` (under 1 KB) and `assets/` (the app script and stylesheet, plus the admin and moderator screens in a file of their own that only staff download). The files in `assets/` have a fingerprint in their name, so browsers keep them for a year; the page is checked on every visit and answered with "not modified" when nothing changed. The server compresses everything once at start (gzip). About 64 KB is sent on a first visit and 0.3 KB on later visits. Commit `web/` so deploys need no Node.
The Go server hands it out. It is committed, so deploying needs no Node.

To change the interface:

```
cd ui
npm install
npm run build
```

That rewrites `web/` (the real site) and `tests/build/page.html` (one file build used only by the page tests). Commit both. The tool versions in `ui/package.json` are pinned.

**Members vs staff.** Moderators and admins can enter every room, and they show an ADMIN or MOD badge instead of a tier
badge. Their points and tier are not shown. Members see their tier, points and progress on the Profile screen.

## 1. Create the database (Supabase, free)

1. In Supabase, create a **new project** for this app. Do not reuse another app's project.
   The free plan allows 2 active projects, so check you have room.
2. Save the database password you choose. Letters and numbers only keeps the next step simple.
3. Click **Connect** at the top of the project and copy the **Session pooler** connection string.
   Replace `[YOUR-PASSWORD]` in it with your password.
   (The pooler string works from hosts that only have IPv4. The plain "Direct connection" string may not.)

The server creates its own tables on first start. Row level security is switched on for them,
so Supabase's web API cannot read them.

## 2. Run it on your Mac

```
brew install go
DATABASE_URL="paste-the-connection-string-here" go run .
```

Open http://localhost:8080. Your login is saved in the browser, so to try a second person on the
same computer, use a private window.

## 3. Put it online (Render)

1. Push this folder to GitHub (you already have the repo).
2. In Render, open the service, then **Environment**, and add `DATABASE_URL` with the same connection string.
   Save. Render restarts the service.
3. Open `/healthz` on your address. It says `ok` when the server is up.

Never put the connection string in a file in the repo.

**Deploying step 15 (exact steps).**
1. Back up the database (Supabase: Database, Backups, or `pg_dump`).
2. Unzip `streetsync-step15.zip` over your repo (it contains the built `web/`, so no Node is needed) and push. No new environment variables are required.
3. On start the server applies `schema.sql` (additive; one constraint change on `credit_ledger`, see "Database changes"). Watch the log for `lobby listening`.
4. Open `/healthz`, sign in, open Settings (you should see Help and Delete account), and open Rooms (you should see Rooms and People tabs).
5. As admin: open Admin, Reports, and Staff complaints. Ban and unban a throwaway guest account to check the flow.
6. Run the tests (`tests/README.md`) against a **throwaway** database, never production. Never run the load test against the production URL.

## Settings (environment variables)

| Name | Default | What it does |
|---|---|---|
| `DATABASE_URL` | none, required | Postgres connection string. |
| `PORT` | `8080` | Port to listen on. Hosting platforms set this for you. |
| `ALLOWED_ORIGINS` | empty | Extra web addresses allowed to open a socket, comma separated. Empty means only the page this server serves. |
| `SESSION_DAYS` | `30` | A login stays valid this many days after its last use. |
| `REQUIRE_PASSWORD` | on | New accounts must choose a password (8 to 128 characters) when they sign up. Set to `0` only for tests. Old passwordless guests still work. |
| `GUEST_DAYS` | `14` | Guests who never set a password lose their handle after this many idle days. |
| `HISTORY_DAYS` | `7` | Chat messages older than this are deleted, to keep the free database small. |
| `DM_DAYS` | `7` | Direct messages older than this many days are deleted (default seven, the agreed policy). |
| `HISTORY_DAYS` | `7` | Room messages older than this many days are deleted (default seven). |
| `PRUNE_BATCH` | `1000` | Rows per cleanup DELETE statement (10 to 100000). Large backlogs are removed in batches. |
| `DB_MAX_OPEN_CONNS` | `4` | Most database connections one instance opens (1 to 100). Budget: instances x this + migrations + admin sessions must stay under the database or pooler limit. |
| `DB_MAX_IDLE_CONNS` | `2` | Connections kept open while idle (never more than the open limit). |
| `DB_CONN_MAX_LIFETIME_MIN` | `30` | Minutes before a connection is replaced. |
| `DB_CONN_MAX_IDLE_MIN` | `10` | Minutes an idle connection is kept (0 keeps it until the lifetime ends). |
| `ROOM_IDLE_DAYS` | `3` | A community room with nobody in it and no message for this long is closed. |
| `FIRST_CLEANUP_SECONDS` | `60` | How long after start the first cleanup runs (the tests use 2). |
| `PHOTO_STORAGE` | unset (photos off) | Where profile photos are kept: `supabase` or `dir`. Unset means the upload button is hidden and everything else works. |
| `SUPABASE_URL`, `SUPABASE_SERVICE_KEY` | none | Needed with `PHOTO_STORAGE=supabase`. Use your project address and the service key. The key stays on the server and is never sent to the page. |
| `PHOTO_BUCKET` | `avatars` | Name of the private Supabase Storage bucket. Create it first (Storage, New bucket, keep it private). |
| `PHOTO_DIR` | none | Folder for `PHOTO_STORAGE=dir`. Development only: Render's free disk is erased on every deploy. |
| `HASH_ITERATIONS` | `600000` | Password hashing strength. One hash takes about a third of a second on a full CPU core, and longer on a small free instance. Only setting a password and logging in with one do this work. Resuming a saved login does not. |
| `FLUSH_MS` | `5000` | How often points are written to the database. |
| `ADMIN_HANDLE` | empty | Handle of the admin account. Set it together with `ADMIN_PASSWORD` the first time. |
| `ADMIN_PASSWORD` | empty | Password for the admin account, at least 8 characters. Keep it in Render's environment settings only. |

## How points work (starting values, easy to change in `rank.go`)

- A message of 3 or more characters earns 1 point. Shorter messages are sent but earn nothing.
- The same text again within 30 seconds earns nothing.
- At most 60 message points per hour.
- First scoring message of each day: a streak bonus of 10 points times your streak days (up to 7).
- Time spent connected earns nothing.
- Tiers start at 0, 300, 2000, 10000 and 40000 points.

## Credits and wallet (starting values, easy to change in `wallet.go`)

- Every account gets **20 welcome credits** once (existing accounts get them the next time they connect).
- **2 credits** on your first real message of each UTC day. Extra bonus when the streak reaches exactly
  3 days (+5), 7 days (+15), 14 days (+30) and 30 days (+60). Being online earns nothing.
- The balance is on the `users` row and every change is also a row in `credit_ledger`, written in one
  transaction. Each ledger row has a `ref` key. The same key can only be used once per person, so a
  retry, a repeated webhook or a double click can never pay or charge twice. A spend larger than the
  balance is refused and changes nothing.
- The Profile tab shows the balance and the last 30 movements. The header shows the balance on the Hub and Profile.
- The admin has a small "Give credits" form on the Profile tab (handle and amount). Grants are logged in `mod_log`.
- Guests who never set a password are deleted after 14 days and their credits go with them. Anyone who will
  hold bought credits must protect their handle first. The buying step has to enforce that.

## Friends and direct messages (starting values, easy to change in `dm.go`)

- Open the Inbox tab, or tap `...` on any room message and choose **Message** or **Add friend**. You can also type a handle in the Inbox.
- **Who pays.** Between friends everything is free. Between strangers, whoever sent the first message in the
  conversation pays **1 credit per message**, and the person who was written to **replies for free**, even with zero credits.
  Accepting a friend request makes the conversation free for both sides for good. Staff messages are free.
  Credits are charged in the same database transaction as the message, with the message id as the key, so a retry can not charge twice,
  and a message that fails is never charged.
- **Friend requests** are always free. At most 100 friends, 50 requests waiting for you and 30 requests out. If two people ask each
  other at the same time they become friends at once. Declining, cancelling and removing are all one tap.
- **Block** ends any friendship and request and shuts messages and requests both ways, with no charge for refused messages.
  The blocker no longer sees the chat in the inbox. The Inbox lists blocked people so they can be unblocked.
- **Report** a message you received from the conversation screen. Moderators see it in the reports panel marked as a direct
  message, and the Delete button removes it. Muted people can not send direct messages.
- Direct messages earn no points. They are end-to-end encrypted (see docs/E2EE.md) and kept for 7 days (`DM_DAYS`), the same as room history (`HISTORY_DAYS`). The Inbox tab shows a badge for unread messages plus waiting requests.
- Tables: `friendships` (one row per pair, status pending or accepted), `blocks`, `dm_messages` (ids share the room message counter).

## Rooms people create (starting values, easy to change in `rooms_user.go`)

- **Who can create:** Regular tier (300 points) and up, **25 credits**, one room each (two for Legends), at most 200 community rooms in total.
  The cost, the room and the limit check happen in one database transaction, so a failed creation never charges.
- **Kinds:** *public* rooms show on the Hub under Community rooms **only while people are in them**, and can be found by search even when empty.
  *Private* rooms are entered only with the 8 letter invite code (or a link `/?room=CODE`). A room with a **password** is never listed or searchable,
  whatever its kind: people need the code and the password. Anyone who gets in on a code, password or fee comes back later without them.
- **Entry fee** 0 to 50 credits, paid once per person. The visitor must send the price they were shown, so a host can not raise it under them.
  The host keeps **80%** (`hostShare`), the platform keeps the rest. Both ledger entries are written in the same transaction as the membership.
  Wrong passwords: 5 in 10 minutes, then locked for a while.
- **Host tools** (Room tools bar inside the room): copy the invite link, change the fee, set or remove a password, switch public or private, close the room.
  Hosts can mute (room only, up to 1 day), kick (blocked from that room for the same time) and delete messages of ordinary members, never staff.
  Staff can open the tools in any community room and close any community room. Mutes and kicks by hosts are kept in memory and end on a restart.
- **Closing:** the host closes it (deletes the room and its messages, no refund), or it closes itself after `ROOM_IDLE_DAYS` without messages and nobody inside.
  Rooms owned by an account that was deleted stay until they go idle.
- Tables: columns `kind`, `owner_id`, `pass_hash`, `entry_fee`, `invite`, `last_active` on `rooms`, and `room_members`.

## Tips (starting values, easy to change in `tips.go` and `ui/src/lib/tips.js`)

- Tap `...` on someone's message in a room and choose **Send a tip**. Pick a treat, then Send:
  **Suya 1**, **Burger 5**, **Pizza 10**, **Cake 25**, **Feast 50** credits. Nothing else can be sent, so amounts are never typed.
- The credits move in **one database transaction**: the sender's balance goes down, the recipient's goes up, and both get a ledger entry
  (`tip_sent`, `tip_received`) with the treat and the other person's handle. Either everything happens or nothing does.
- **No duplicates:** the page makes a key when the tip sheet opens and sends the same key on every tap and retry. The server pays a key once
  and answers repeats with "already sent". The recipient's ledger entry includes the sender, so two people can never clash on a key.
- **Refused before any money moves:** your own message, a tip card, more credits than you have, blocked either way, an amount that is not on the list.
- Tips are **not room fees**: separate ledger kinds, separate colours in the wallet (violet for tips, amber for room entry, creation and income).
- The tip appears to everyone in the room as a card, is kept in room history like a message (`messages.tip_to`, `messages.tip_amount`), and can
  not be deleted, replied to or tipped. The person tipped also gets a short notice wherever they are. Tips earn no points.

## Look and feel

- Tokens are at the top of `ui/src/app.css`: base `#080D16`, panels `#111C2B` and `#0D1420`, inputs `#0A1320`, text `#F3F6FC` and `#94A3B8`,
  accent cyan `#00D9E8`, violet `#8B5CF6` (rooms, tips, locks), green, amber and red for status. Below the tokens, the "design layer" holds the
  shell and the restyled screens. Icons are drawn inline (`Icon.svelte`), the logo is `Brand.svelte`.
- Wide screens (900px and up): a sidebar with the logo, Home, Messages and Profile, and room lists in a grid.
  Phones: a bottom bar with the same three, hidden inside a room or a conversation so the composer has the space.

## Profiles, avatars and navigation (step 10)

- **Profiles:** tap any name or avatar (in a room, a conversation, the Messages lists, or the host line of a community room) to open that person.
  It shows the avatar, name, online status, tier and points (staff show a badge instead), and when they joined. From there: Message, Add friend,
  Cancel or Accept or Decline, Remove friend, Block or Unblock. Tapping your own name shows a link to your wallet and settings. Tapping a name no longer
  mentions them: **Mention** is in the message menu.
- **Messages page:** a tab header, **Chats**, **Friends**, **Requests** and **Blocked**, with counts and badges, stays at the top while the list scrolls.
  Remove friend and Block are on each friend in the Friends tab (and Block is on a profile). A conversation shows the person once, in the page header, and only shows
  a bar when you are not friends yet (Add friend, Accept, Cancel, Unblock and the price of a message).
- **Avatars:** at sign up pick Male, Female or Prefer not to say, then one of eight pictures (24 looks in all). The server stores only the look code like `f3`, never
  the gender. Change it later on the Profile tab. Everyone sees the same picture for you: it travels with your messages, direct messages, conversations and
  the Messages lists. People who never picked get a picture made from their handle, as before.
- **Sidebar and bottom bar** work from anywhere, also from inside a room or a conversation (the room is left first).
- **Logging out of a handle that has no password** asks first, because it can never be opened again: protect it with a password, log out anyway, or cancel.
- **Rooms have pictures** (drawn by the device) instead of letters. The Football room is now **Sport** (football, basketball, athletics). An untouched Football room is
  renamed at start, one the admin renamed is left alone.
- **Sign in page** is also the landing page: the promise, four points, an example room and the form side by side on wide screens, stacked on phones.

## Roles, moderation and the staff policy

**Roles, highest first:** `admin` (3), `mod` (2), `user` (1). The role is always read from the database (or from the
server's own record of the account). No frame carries a role, so a client cannot claim one. Every rule below is enforced
in the server, in `policy.go`, `mod.go` and the store transactions. The page only hides buttons.

| Action | user | mod | admin |
|---|---|---|---|
| Mute, unmute, kick (up to 24 hours), delete someone's message, clear a bio | no | on ordinary users | on mods and ordinary users |
| Act on an admin, or on yourself | no | no | no |
| Promote, demote, grant credits | no | no | yes |
| **Permanent ban and unban** | no | no | yes (never on an admin) |
| Read reports about messages, people and rooms | no | yes, except reports about staff | yes |
| Read **staff complaints** | no | **no** | yes, never one about themselves |
| Close any community room | no | yes | yes |

**Staff are shielded from ordinary accounts.** An ordinary user cannot, by any route (room message menu, profile, a hand
written frame):
- tip an admin or moderator (`staff_shield`; the transaction refuses it, so no credits ever move),
- start a direct message with one. They *can* reply to a staff member who wrote to them first, or who is their friend,
- send them a friend request. They *can* accept a request a staff member sent,
- file an ordinary report about them (`staff_report`).

The same rule applies everywhere the feature exists. Staff also cannot earn entry fees: a staff account cannot set a room
fee, and rooms of an account that becomes staff are set to free (`staff_no_fee`).
Staff accounts can still tip, message and befriend each other and ordinary users.

**Reporting staff misconduct (the safe channel).** Because ordinary reports are read by moderators, reports about staff
would otherwise be seen by the people they accuse. So there is a separate path: **Report staff misconduct** on a staff
member's message, profile or room (`{"t":"scomp","h":"staffhandle","i":42,"c":"abuse","b":"what happened"}`).
Reasons: `abuse` (abuse of power), `harassment`, `unfair`, `other`. They are stored in `staff_complaints`, visible only to
**admins** (Admin, Staff complaints). Moderators cannot read them, and a complaint about an admin is never shown to that admin.
Moderators can complain about admins the same way. A reply or direct message from a staff member that is reported goes to the
same channel. Limit: with exactly one admin, a complaint about that admin has no independent reader. Add a second admin
if that matters to you.

**The admin account** is set up at every start from `ADMIN_HANDLE` and `ADMIN_PASSWORD`.
- If the handle does not exist, it is created with that password.
- If it exists as a guest, it is adopted: it gets the password and every old login of it is ended,
  so only someone who knows the password can use it. Log in with "I already have a password".
- If it already has a password, only the admin role is set. The password is not changed.
- Nobody else can take that handle.

**Temporary sanctions versus permanent bans.**
- *Temporary:* mute (read but not post) or kick (removed), 10 minutes to 24 hours, by a moderator or admin. Stored in `sanctions`, they end by themselves and survive restarts.
- *Permanent:* a **ban**, by an admin only. It is a column on the account (`users.banned_at`), so it lasts until an admin unbans.
  Banning ends every login of the account in the same transaction, closes its open connections, and refuses `hi` with a token (`session_expired`) and `login` (`banned`, only shown after the right password).
  A ban also holds after logout, reconnecting and signing in again, and is re-checked right after a connection is accepted so a ban cannot race a sign in.
  Banned accounts disappear from search and profiles. Ban and unban are written to `mod_log` in the same transaction.
- *Limit:* a ban is by account. A banned person can make a brand new guest account. There is no IP or device ban.

**Reports.** Reasons: spam, abuse, harassment, scam, other. Targets: a message (`rep`), a person (`urep`), a room (`rrep`),
with optional details up to 300 characters (required for "other"). One open report per reporter and target, 10 reports an
hour per person across all kinds. A message can only be reported from the room you are standing in. Staff are not reported
this way (see above). A report of a room run by staff goes to the complaint channel. Room hosts can see the reasons for their room, never who reported.
Staff queue: Moderation or Admin, Reports (messages, people, rooms), with Mute, Kick, Delete message, Close room, Ban (admins), Dismiss.

Every staff action is written to the `mod_log` table, with who did it.

**Direct message deletion.** A moderator may delete a direct message only if somebody reported it, and never one written by someone above them.

**Limits you should know.**
- Moderation by account: a guest who is muted or kicked can pick a new handle.
- Reports and deletes work on the last 200 messages of each room.

## Account deletion and data retention

Settings, **Delete account**. Two stages: (1) the consequences are listed and you enter your password (guests type their handle); the server answers with a one time token valid for 5 minutes;
(2) you type `DELETE` and the token is sent back. The token works once and only for the account that asked. Admins cannot delete their account here (an admin must be demoted by another admin first).

It is one database transaction (`deleteAccount` in `store15.go`). It can be repeated safely: the second run finds the tombstone and does nothing. Concurrent runs are serialized by a row lock.

| What | Result |
|---|---|
| Login, all sessions, open WebSocket connections | **Ended immediately.** A deleted account cannot use any token. |
| Password hash, bio, avatar, points, streak, handle | **Removed.** The handle becomes free for anyone to register again. |
| Room messages and tip cards you wrote | **Deleted**, also from the live room. |
| Direct messages you sent or received | **Deleted for both sides.** |
| Friendships, blocks (yours and against you), notifications for you, settings, room memberships | **Deleted.** Notifications about you in other people's lists lose your name. |
| Community rooms you host | **Closed** with their messages. Members are told. There is no ownership transfer. |
| Credit balance | **Forfeited** through a ledger row `account_closed`, so the ledger still adds up. |
| **Credit ledger** | **Retained.** It points to an anonymous tombstone account `deleted_<id>` with no name, password or profile. The foreign key is `NO ACTION`, so a database delete of a user that has ledger rows is refused. |
| Reports you filed or that name you, staff complaints, the audit log, sanctions | **Retained** for safety and disputes, with your handle replaced by `deleted_<id>`. The copied message text inside a report stays. |
| The tombstone row itself | **Retained** (id, `deleted_<id>`, creation time, `deleted_at`). |

Nothing here promises that every trace is gone: the ledger, reports and the audit log stay by design. Database backups kept by your host are outside this app.
Handles starting with `deleted_` can not be chosen. Idle guests are pruned as before, except guests with any paid ledger history (tips, room fees), who are kept.

## People search, profiles and rooms (step 15)

- **Search** (Rooms page) has two tabs. *Rooms*: community rooms that are public and listed, by name, tag or description. Private and password rooms never appear. *People*: any signed in person (guests too) can search by handle (2 to 16 letters, numbers, underscore; 250 ms between searches; 20 results). Results show handle, avatar and a staff badge or tier. Banned and deleted accounts, yourself and people who blocked you do not appear.
- **Profiles** (tap a name or avatar) show avatar, bio, tier or staff badge, points, join date, relationship. Never a hash, token or private message. Banned accounts show as not found (to non staff). A person who blocked you is not revealed: the profile opens as before.
- **Room descriptions**: up to 200 characters, one line, saved in `rooms.description`, validated in the page and in the server (`bad_description`). Shown on room cards, in directory and search results, matched by search, and editable by the host.
- **Home page**: a *Featured* section (the busiest public rooms right now) and category tiles that open the Rooms page filtered.
- **Membership**: opening a community room records you as joined. Cards show *Joined*, *Open*, *Rejoin*, with a **Leave room** button (tap twice). Leaving keeps the right to come back, so **an entry fee is never charged twice**; a host removing you (Manage, Remove) ends it. A free public room still shows on the hub only while people are in it (existing decision). Hosts cannot leave their room; they close it.
- **Room management page** (Rooms, Manage, for the host or staff) with Members, Moderation (mute, kick, remove, description) and Reports (reasons only). It works outside the live chat. Hosts still cannot moderate staff.

## Database changes and migration (step 15)

`schema.sql` runs at every start and only adds. On an existing database it:
- adds `users.banned_at/banned_by/ban_reason/deleted_at`, `rooms.description`, `room_members.left_at`, and `reports.kind/target_user_id/target_room/details` (existing reports become `kind='message'`),
- makes `reports.msg_id` nullable and replaces the one unique index with three partial ones,
- creates `staff_complaints`,
- **changes `credit_ledger.user_id` from `ON DELETE CASCADE` to `NO ACTION`**, once (the `DO` block checks the current setting). No data is touched.

Verified on a Step 14 database with data: nothing lost, running the file twice gives no errors. Back up before deploying anyway. To go back, redeploy the old build: the new columns and table are simply ignored by it (the old build would again let a user delete cascade the ledger).

## Rooms and the hub

After signing in you land on the **hub**: the list of rooms with how many people are in each, a heat
level of 0 to 5 flames, and tags. Tap a room to enter it, and use the arrow at the top to go back.
You are in one room at a time per tab, and you can open another tab in another room.

- **Heat** counts messages sent in the last 5 minutes: 1 to 4 is one flame, 5 to 14 two, 15 to 39 three,
  40 to 99 four, 100 or more five.
- **Tier gates.** A room can need a minimum tier. The Elite Lounge needs Elite. Locked rooms show what they need.
- **Messages stay in their room.** History, replies, reports and deletes all work per room.
  The server keeps the last 200 messages of each room in memory and sends the last 30 to new joiners.
- **The official rooms** are created at startup from the list in `rooms.go`, only when missing. After that they
  belong to you. Open the `rooms` table in Supabase to rename a room, change its tags, set `min_tier`
  (0 Newbie to 4 Legend) or change `sort`. Add a row to add a room. Restart the service for changes to show.
  Messages in a room that you remove from the table stay in the database but nobody can reach them.

**Staff badges.** Admins and moderators show an ADMIN or MOD badge with their own colors instead of a tier badge,
and their messages have their own look, so nobody mistakes them for members.

**Deleting.** Anyone can delete their own message. Moderators can delete messages of people they could mute.

## Replies

Tap **Reply** on a message, or slide it to the right on a phone. The reply shows a short quote of the
original, and tapping the quote jumps to it. The server keeps the last 200 messages of the room,
so you can reply to anything still in that window. Older ones answer `reply_gone`.
A reply to one of your messages is highlighted for you, like a mention.

## Tests

See `tests/README.md` for setup. In short:

```
TEST_DATABASE_URL="postgres://lobby:lobby@localhost/lobbytest?sslmode=disable" go test -count=1 ./...
go build -o lobby-test . && python3 tests/e2e_chat.py      # and the other e2e_*.py scripts
cd tests && for f in page_*.test.js; do node $f | tail -1; done
```

Database tests only touch rows whose handle starts with `t_`, rooms named `T ...`, and the tombstones they create. Use a throwaway database.

## Load testing

`scripts/loadtest-local.sh smoke|staged|custom` and `go run ./loadtest -h`. Read `docs/LOADTEST.md` before running and
`docs/LOADTEST-REPORT.md` for the results of this release. It refuses any target that is not local or explicitly allow-listed,
and never runs the 3,000 stage against a public address by itself.

## What the server accepts

First frame decides who you are:

- `{"t":"hi","h":"ada"}` new guest
- `{"t":"hi","k":"<token>"}` returning user
- `{"t":"login","h":"ada","p":"password"}` user with a password

`hi` and `login` may also carry `"r":"football","l":42`. That puts you straight into that room,
and `l` is the last message id you saw there. A client that reconnects uses it to land back where it was.
Without `r` you start on the hub and get the room list.

After that: `{"t":"join","r":"football","l":42}` enters a room and leaves the one you were in,
`{"t":"leave"}` goes back to the hub, `{"t":"m","b":"text"}` sends a message to your room,
`{"t":"m","b":"text","q":42}` replies to message 42, `{"t":"del","i":42}` deletes your own message,
`{"t":"pw","p":"password"}` protects the handle, `{"t":"bye"}` logs out (ends only that login), `{"t":"rep","i":42,"c":"spam"}` reports message 42,
`{"t":"wallet"}` asks for your balance and latest credit movements (answer: `{"t":"wallet","c":27,"l":[...]}`),
`{"t":"dm","h":"bob","b":"hi"}` sends a direct message, `{"t":"thread","h":"bob"}` opens a conversation (answer: `thread` with `s` = none, friend, sent, got or blocked, `c` = price of one message for you, `m` = messages),
`{"t":"read","h":"bob"}`, `{"t":"inbox"}` (answer: `inbox` with chats `c`, friends `f`, incoming requests `q`, outgoing `p`, blocked `x`),
`{"t":"fr","a":"add|accept|decline|cancel|remove","h":"bob"}`, `{"t":"blk","a":"add|del","h":"bob"}` and `{"t":"drep","i":42,"c":"abuse"}`.
Rooms people create: `{"t":"croom","n":"Name","k":"public|private","g":"tags","p":"password","f":10}` (answer `croomok` with `s` and invite `i`),
`{"t":"find","n":"words"}` (answer `found`), `{"t":"rinfo","c":"CODE"}` (answer `rinfo`), `join` now also takes `c` (code), `p` (password) and `f` (the price you agreed to),
`{"t":"rset","f":10,"pw":"","k":"private"}`, `{"t":"hmod","a":"mute|unmute|kick","h":"bob","m":10}`, `{"t":"rclose"}` (staff may add `r`).
The server pushes `rmuted`, `rkicked`, `rclosed`, `rsetok`. `joined` carries `k`, `f`, `w`, `o`, `y` (I am the host) and `i` (invite code, host only).
New error codes: `bad_room_name`, `bad_room_kind`, `bad_fee`, `bad_room_password`, `room_limit`, `rooms_full`, `room_name_taken`, `needs_invite`, `fee_changed`, `room_banned`, `room_muted`, `not_host`, `bad_query`.
Avatars and profiles: `hi` and `ok` carry `v` (the look), `{"t":"av","c":"f3"}` changes it (answer: a `me` frame with `v`), messages carry `av`, `dm` frames carry `fa`, `thread` and `inbox` frames carry an `av` map of looks by handle,
`{"t":"profile","h":"bob"}` answers `uprofile` with `h`, `v`, `j` (joined, ms), `on` (online), `s` (how I stand: none, friend, sent, got, blocked), `r` and `p` (tier and points) or `o` (staff). New error code `bad_avatar`.
Tips: `{"t":"tip","i":42,"amt":5,"c":"<key of 8 to 64 letters, numbers, - or _>"}` tips the author of message 42 in the room you are in (answer `tipok` with `a`, `n`, `h`, and `dup` when it was a repeat).
The server pushes `tipgot` to the person tipped. Tip cards are `m` frames with `tt` (who) and `ta` (credits), their body is the treat name. New error codes: `self_tip`, `bad_request` (and `bad_amount`, `no_credits`, `blocked`).
The server pushes `dm`, `ib` (unread `u` and waiting requests `q`), `fr` (request, friends, removed) and `frok`, `blkok`.
New error codes: `blocked`, `no_credits`, `self_dm`, `empty`, `friend_cap`, `already_friends`, `no_request`, `too_many_requests`.

Step 15 frames: `usearch` (people search, answer `ufound`), `urep`, `rrep`, `scomp` (answer `scompok`), `acomp` and `acompdone` (admin only, answer `comps`),
`rleave` (answer `rleft`), `rmgr` (answer `rmgrinfo`), `hmod` also takes `r` (room) and the action `remove`, `croom`/`rset` take `d` (description),

Step 17 frames and changes (migration: `migrations/017_step17.sql`, also appended to `schema.sql`, safe to run again):
- Direct message receipts. `dm_messages.delivered_at` is new (`read_at` already existed). Status is derived: read if `read_at` is set, else delivered if `delivered_at` is set, else sent (1, 2, 3). It can only go up, because the updates only touch rows where the column is still empty.
- Client to server `{"t":"dack","h":"bob","ids":[12,13]}`: this device received those messages from bob (at most 100 ids). Only the recipient of a message can advance it. `{"t":"thread","h":"bob","nr":true}` opens a conversation without marking it read (the page sends it while hidden). `{"t":"read","h":"bob"}` and a normal `thread` mark everything from bob read, which also counts as delivered.
- Server to the sender, on every open tab: `{"t":"rcpt","h":"bob","s":2,"ids":[12]}` (delivered) or `{"t":"rcpt","h":"bob","s":3,"u":13}` (everything of mine up to id 13 is read). Ids and a number only. Nothing is sent when nothing changed, or when the reader blocked the sender. `thread` and `dm` frames carry `st` (1, 2 or 3) on each message; the page shows ticks only on its own messages.
- Room notices: `{"t":"rs","a":"j"|"l","h":"bob"}` is a live "joined" or "left" line for people in the room. It is never stored in the room history and never earns points. A dropped connection or the back arrow only starts a grace period (`graceLeave`, 30 s in `presence.go`), and a return inside it makes no notice. `{"t":"leave","a":"x"}` (the Leave room button) and `rleave` announce at once. Membership (`room_members`) is only ended by `rleave`.
- `find` answers now include `q`, the cleaned words searched. People search accepts any capitalisation; the answer `ufound.q` is lower case.
`mod` with `a` = `ban` or `unban` (admin only), `delreq` then `delgo` (answers `delready`, `delok`). Rooms frames carry `d` and `j` (joined). Profiles may carry `sh` (staff shield) and `dmok`.
New error codes: `staff_shield`, `staff_report`, `not_staff`, `bad_details`, `bad_description`, `staff_no_fee`, `owner_cannot_leave`, `not_member`, `already_banned`, `not_banned`, `banned`, `bad_password`, `bad_confirm`, `delete_expired`, `admin_cannot_delete`, `bad_query`.

and `{"t":"p"}` is a ping. The `me` frame now also carries the balance as `c`. Moderator frames are listed at the top of `hub.go`.

Limits: handle 3 to 16 letters, numbers or underscores. Messages up to 300 characters.
Rate limit is a burst of 5 messages, then 1 per second. 8 failed logins per handle in 15 minutes locks that handle for the rest of the window.


## Step 16

See `docs/STEP16-REPORT.md` for what changed, what was verified and what was not. Key points: direct messages and room history are kept 7 days;
direct messages are end-to-end encrypted on a single device (`docs/E2EE.md`, `docs/SECURITY-VERIFICATION.md`); admin accounts have no wallet
(`docs/WALLET-POLICY.md`); assets are served with Brotli or gzip; the database pool is configurable.


## Profile photos (step 18)

Photos are off until you configure storage. Nothing here costs money by itself, and no credential is in the project.

1. Run `migrations/018_step18.sql` on Supabase (SQL editor). The server also applies it at start, so this only matters if you prefer to run it yourself first.
2. In Supabase, open Storage, create a **private** bucket named `avatars`.
3. In Render, Environment, add `PHOTO_STORAGE=supabase`, `SUPABASE_URL` (your project URL) and `SUPABASE_SERVICE_KEY` (Project Settings, API, service role). Redeploy.
4. Open your profile: "Add a photo" appears. Pictures are served by the app at `/photo/<handle>`, so the bucket stays private.

The Supabase connection was tested only against a stand-in server in this step, not live. Try one upload after you configure it.

## Room engagement (step 19)

Details, decisions and test results are in `docs/STEP19-REPORT.md`.

- **One room per account.** Joining a room moves the account out of every other room, in every tab and on every device, and ends the Joined state of its other community rooms. Access rights (invite, password, fee) are kept, so nobody pays twice. If a connection drops, room presence is kept for 60 seconds (`graceLeave` in `presence.go`). Reconnecting inside that time makes no notice. The Leave room button leaves at once.
- **Pin.** One pinned message per room, set by the host of a community room or by a moderator or admin (official rooms: staff only). It keeps its own copy of the text, so it also works as an announcement.
- **Reactions.** Six fixed emoji (`reactionSet` in `engage19.go`, `REACTIONS` in `ui/src/lib/engage.js`), one reaction per account per message.
- **Polls.** Host or staff create and close them. One open poll per room, 2 to 6 options, one vote per account, vote changes allowed.
- None of this earns points, credits or tier progress. Limits are constants at the top of `engage19.go`.

New frames from the page: `pin {i}`, `unpin`, `rx {i, em}` (em empty removes), `pcreate {b, os}`, `pvote {pid, opt}`, `pclose {pid}`. New frames to the page: `x19 {r, pin, poll, rx}` (sent after joining a room), `pin {p}`, `rx {i, c, h, y}`, `poll {p}`. New error codes: `bad_emoji`, `bad_poll`, `poll_open`, `poll_closed`, `no_poll`.

Database: `migrations/019_step19.sql` (also appended to `schema.sql`, which the server applies at start). Safe to run more than once.

### Bug fix after step 19 (polls and leave notices)

Details in `docs/STEP19-BUGFIX-REPORT.md`.

- Poll options are now sent to Postgres as text cast to jsonb, so poll creation works whether or not `DATABASE_URL` contains `binary_parameters=yes`. A failed poll creation says `poll_failed` (server fault) instead of "server busy" unless the failure is a temporary one, and the server log line names the SQLSTATE.
- The page sends `bg` and `fg` frames when it goes to the background and comes back. A connection lost while the page was in the background keeps the person in the room for `backgroundGrace` (5 minutes, `presence.go`). Other connection losses keep the 60 second `graceLeave`. A person whose login was ended, who was banned or deleted, leaves at once. Leave room removes the person from the room in every tab.

## Room discovery and member list (step 20)

Details, decisions and test results are in `docs/STEP20-REPORT.md`.

- **Favourites and recent rooms** are saved on the server (`room_favourites`, `room_visits`) and follow the person across refreshes, devices and logins. At most 50 favourites and 15 recent rooms (`favLimit`, `recentLimit` in `discover20.go`). Both are shortcuts only: opening one is a normal join with every check. Rooms a person may not see are never listed or favourited, and a deleted room takes its entries with it.
- **Sorting** (`dir` and `find` with `sort`): `active`, `recent` or `new`, ranked on the server. Private and password rooms never appear in the directory or search.
- **Member list** (`members`, `mclose` from the page, `ml` and `mp` back): the host, the members who have not left, and people there now. "Online" means in this room now (grace periods and several tabs apply). People who hide their online state show as "Status hidden" and no live update is ever sent about them. Only handle, avatar, status, badge and host flag are sent.
- New frames from the page: `fav {r, fav}`, `myrooms`, `members {off}`, `mclose`, and `sort` on `dir` and `find`. New frames to the page: `mr {fs, f, v}`, `ml {r, m, off, more, on, tot}`, `mp {h, s | d}`. New error codes: `fav_full`.
- Database: `migrations/020_step20.sql` (also appended to `schema.sql`). Safe to run more than once.
