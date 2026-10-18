# STREETSYNC (steps 1 to 10)

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
| `DM_DAYS` | `30` | Direct messages older than this are deleted. |
| `ROOM_IDLE_DAYS` | `3` | A community room with nobody in it and no message for this long is closed. |
| `FIRST_CLEANUP_SECONDS` | `60` | How long after start the first cleanup runs (the tests use 2). |
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
- Direct messages earn no points. They are kept for 30 days (`DM_DAYS`). The Inbox tab shows a badge for unread messages plus waiting requests.
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

## Moderation

**Roles.** `admin` can do everything. `mod` can handle normal users. Nobody can act on an admin, and
nobody can act on themselves.

**The admin account** is set up at every start from `ADMIN_HANDLE` and `ADMIN_PASSWORD`.
- If the handle does not exist, it is created with that password.
- If it exists as a guest, it is adopted: it gets the password and every old login of it is ended,
  so only someone who knows the password can use it. Log in with "I already have a password".
- If it already has a password, only the admin role is set. The password is not changed.
- Nobody else can take that handle.

**What moderators can do.** Tap the three dots on a message.
- **Mute** (10 minutes or 1 hour): the person can read but not post. Survives restarts.
- **Kick** (10 minutes): removed, and cannot come back until the time is up. Survives restarts.
- **Delete** a message for everyone. A message deleted before it was saved is never saved.
- **Reports** button: the open reports with Mute, Kick, Delete message and Dismiss. Acting on a report closes it.
- Admins can also promote someone to mod, or demote them, with `{"t":"mod","a":"promote","h":"name"}`.
  The person needs a password first. There is no button for this yet.

**Reporting.** Anyone can report another person's message as spam, abuse or other. Each person can report a
message once and file 10 reports an hour. A report keeps a copy of the message.

Every moderator action is written to the `mod_log` table, with who did it. Read it in the Supabase table editor.

**Limits you should know.**
- Moderation is by account. A guest who is muted or kicked can pick a new handle. Their points start again at zero.
- Mutes and kicks last up to 24 hours. There is no permanent ban yet.
- Reports and deletes work on the last 200 messages of each room.

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

```
go test ./...
TEST_DATABASE_URL="postgres://user:pass@localhost/db?sslmode=disable" go test ./...
```

The second form also runs the database tests. They only touch rows whose handle starts with `t_`
and a test room named `testroom`.

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

and `{"t":"p"}` is a ping. The `me` frame now also carries the balance as `c`. Moderator frames are listed at the top of `hub.go`.

Limits: handle 3 to 16 letters, numbers or underscores. Messages up to 300 characters.
Rate limit is a burst of 5 messages, then 1 per second. 8 failed logins per handle in 15 minutes locks that handle for the rest of the window.
