# Handover: 2go-style chat platform

Written at the end of build step 10, for a new chat to continue from here.
The project is in the zip next to this file. Start by unzipping it and reading `README.md` and `tests/README.md`.

## 0. How to start the new chat

1. Upload `lobby-step5-with-tests.zip` and this file.
2. Say: "Continue the project from HANDOVER.md." Nothing else needs explaining.
   (The user also asked: after every finished step, tell them the next step to build.)
3. First actions for the assistant: unzip, set up the sandbox as in section 9, run the test suites to prove the
   environment works, then ask the user the questions in section 11 before building anything.

## 1. The person and how to work with them

- Solo founder in Abuja, Nigeria. Builds everything alone. Works on a Mac with 8 GB RAM. Not deep in Go or Svelte,
  so explain what pieces do in plain words when asked, but do not over explain.
- Saved style preferences (follow them): brief and to the point, ask questions when something is unclear
  (one question at a time is best), human tone, no generic AI wording, **never use em dashes**, give complete
  replacement files and not snippets, make surgical fixes and ask before rebuilding something.
- How they work: they deploy each zip to their own GitHub repo, test with friends in the live chat, and send
  screenshots with feedback. They often reply "Continue" or "next". They care about getting a real product live
  for **$0** first, and about not paying for trial-style free tiers that expire.
- Say what you are doing in a few words when a task runs long. They get nervous when a chat goes quiet.
- Check prices and limits of services with a search before advising. Several free tiers changed in 2026.

## 2. The product in one page

A web-native recreation of the 2010-era **2go** chat network, with modern additions.

- Zero friction: open the URL, pick an @handle, chat. No phone number or SMS provider, ever (user decision).
- Rooms by interest and city. Live hub with online counts and heat. Rank system where **only real chatting earns
  points, never idle time**.
- Revenue plan: credits (earned free or bought), tips and gifts, cosmetics, fee based rooms, a fee to message
  strangers, plus **ads and sponsorship** (user wants this).
- Launch market: Nigeria first, then the whole world. Everything must work for any country, language and person.
  Low data use is a core promise.
- The name is **STREETSYNC** (chosen at step 9, shown on the sign in screen, sidebar and page title). Go package and repo are still called lobby.

### Decisions the user has confirmed (do not reopen without asking)

| Area | Decision |
|---|---|
| Tiers | Newbie, Regular, Insider, Elite, Legend (universal names, chosen over local slang) |
| Points | Real messages only. Idle or connected time earns nothing. |
| Credits | Basic chat, friend requests and recovery are always free. Free ways to earn: streaks, activity, referrals. Buying: Paystack and Flutterwave first, a global provider later. |
| Direct messages | Friends and people who shared a room: free. Strangers: small credit fee. Friend list has a cap. |
| Rooms | Official rooms for every interest, open to everyone. Later: user hosted rooms (public, private, password, fee). Some rooms need a tier. |
| Sponsors | Sponsored rooms, hub placement, brand gifts and badges, sponsored events. Sponsor ads carry the **real brand image or a 3D version people recognize**. Text or small images only, no video. |
| Avatars | Illustrated, never real photos. Room cover images are fine unless they cost too much data, then use icons. |
| Look | Dark neon. A mix of "feature phone revival" and "night arcade". Reference image: four screens (Live Hub, Room chat, Profile, Wallet and Store). **The Live Hub and Room chat stay.** Other screens may change. |
| Stack | Go server, PostgreSQL on Supabase, Svelte web app, Cloudflare later for static files, images (R2) and CDN. Redis only when there is more than one server. |
| Hosting | $0 first. Nothing that expires in a month. Convert to paid when revenue starts. |
| Staff | Admins and moderators can enter every room. They show an ADMIN or MOD badge and no tier or points. |
| Messages | Anyone can delete their own message. Replies show a quote. A friend suggested slide to reply, and it is built. |
| Credits start | Every account gets 20 welcome credits once. |
| Direct message price | Only accepted friends message free (shared-room people are NOT free, user chose option A). Between strangers the person who sent the first message in the conversation pays 1 credit per message, the person written to replies free, and a friend request accepted makes it free for both. |
| App name | STREETSYNC. |
| New look (from the 15 screen mockup, step 9) | Deep navy dark theme: base #080D16, panels #111C2B and #0D1420, inputs #0A1320, text #F3F6FC and #94A3B8, cyan #00D9E8 accent (primary buttons, active nav), violet #8B5CF6 for rooms, tips and locks, restrained green, amber and red. Compact sidebar on desktop, bottom bar on mobile. Premium and calm, no heavy glow, never a 2go replica, not an admin dashboard. This **supersedes** the earlier "dark neon, feature phone and night arcade" look. The user wants the screens brought into line progressively. |
| Paystack | **On hold** until the user is happy with how each page looks. Keep it in the backlog as the option, do not build it unless asked. |
| Avatars | Picked at sign up (gender then one of eight pictures), stored only as a look code like f3 (m, f or x, 0 to 7), changeable on Profile, the same picture for everyone. No gender is stored. |
| Profiles | Tap any name or avatar to open a profile. Tapping a name does not mention. Remove friend and Block live on the Friends tab and the profile, not in a conversation. |
| Football room | Renamed Sport (slug stays football). |
| Tips | Fixed amounts only: Suya 1, Burger 5, Pizza 10, Cake 25, Feast 50 credits. Highlighted cards in the room feed, atomic transfer, key based duplicate protection, separate from room fees. |
| Order of work | Credits and wallet (step 6), then direct messages (step 7), then rooms people create (step 8). The user asked for all three. |
| Rooms people create | Needs Regular tier, costs 25 credits, one room each (Legend two). Community rooms get their own hub section and show only while people are in them. Private and password rooms are never listed. Rooms with no messages for 3 days are deleted. Entry fees allowed, host keeps 80%. |

## 3. What exists now

| Step | What | State |
|---|---|---|
| 1 and 2 | Handles, resume after a dropped connection, a room with history | Live, confirmed by the user |
| 3 | Postgres storage, optional password, stars and tier badges, points, streaks | Live, confirmed |
| 3b | Replies with quotes, slide to reply, mentions, reply and mention highlight | Live, confirmed |
| 3c | Moderation: mute, kick, delete, report, reports panel, roles, admin account, mod log | Live, confirmed |
| 4 | Rooms and the Hub: 10 official rooms, counts, heat, tier gates, staff badges on messages, members delete own messages | Live, confirmed (the user tried entering the Elite Lounge) |
| 5 | Real Svelte interface (Hub, Room, Profile, tabs, generated avatars), staff enter every room, staff hide tier and points, log out | Live, confirmed |
| 6 | Credits and wallet: ledger, 20 welcome credits, daily and streak credits from real chatting, wallet card on Profile, balance in the header, admin give credits form | Delivered (ask if confirmed) |
| 7 | Friends and direct messages: Inbox tab, conversation screen, friend requests, block list, paid stranger messages, DM reports, unread badge | Delivered (ask if confirmed) |
| 9 | Tips (fixed treats, feed cards, atomic ledger transfer) and the STREETSYNC design foundation | Live, confirmed (desktop and phone screenshots) |
| 10 | Redesign round 2 and landing: full width desktop, sidebar works inside rooms, Messages tabs, cleaner conversations, profiles for anyone, avatars picked at sign up, guest logout warning, Sport room, room pictures, landing hero on the sign in page | **Delivered, not yet confirmed by the user** |
| 8 | Rooms people create: create form, public/private, password, entry fee, invite codes and links, search, host tools (mute, kick, delete, settings, close), idle cleanup | Delivered (ask if confirmed) |

The 10 official rooms (editable in the `rooms` table): General Chat (`lobby`), Abuja, Lagos, Campus Gist, Football,
Music Heads, Tech Hustle, Side Hustle, Creators Hub, Elite Lounge (tier 3). Changes need a service restart.

## 4. Architecture and files

One Go program serves both the web page and the WebSocket. State lives in memory with Postgres behind it.
Messages are saved in the background in batches so chat never waits for the database. Points are saved every 5 s.

```
main.go        start up, settings, admin account setup, clean shutdown (saves everything)
hub.go         rooms, joining, messages, points, heat, background workers, wire format
mod.go         roles, mute, kick, delete, reports, promote and demote
ws.go          the WebSocket: sign in, resume, login, password, log out, read and write loops
auth.go        PBKDF2-SHA256 password hashing (600,000 rounds, stdlib only), session tokens, login limits
store.go       all database code (lib/pq), batch inserts, cleanup, rooms, sanctions, reports
rank.go        tier thresholds and point rules (the numbers to tune)
rooms.go       the default room list
wallet.go      credits: ledger rules, welcome and streak rewards, wallet frame, admin grant
dm.go          direct messages, friend requests, blocks, DM reports, unread badge (hub side)
dm_store.go    database code for dm_messages, friendships and blocks, and the pricing rule `dmPays`
rooms_user.go  rooms people create: create, find, enter rules, host tools, close, idle cleanup (hub side)
profiles.go    avatars (`cleanAvatar`, `av` frame) and the `profile` frame (hub side and store code)
tips.go        tips: fixed treat list, atomic credit transfer (`transferCredits`), the `tip` handler that posts the card
rooms_store.go database code for creating, entering (fee and host share), settings and deleting community rooms
schema.sql     tables, applied at every start, safe to repeat, row level security on
web/index.html the BUILT interface (one file, 77 KB). Committed so deploys need no Node.
ui/            Svelte source. `cd ui && npm install && npm run build` rewrites web/index.html
tests/         end to end scripts, page tests, README with setup
Dockerfile     builds the Go server only (the ui and tests folders are ignored)
```

Libraries: Go 1.22 in `go.mod` (Docker image uses 1.23), `github.com/coder/websocket` v1.8.13,
`github.com/lib/pq` v1.10.9. UI tools are pinned to versions at least two weeks old on purpose:
Svelte 5.57.1, Vite 8.3.1, @sveltejs/vite-plugin-svelte 7.3.1.

### Interface notes

- `ui/src/lib/chat.svelte.js` holds all logic (connection, reconnect, rooms, messages, moderation).
  Components only read `chat` and call its functions. Screens stay in the page and are shown or hidden with the
  `hidden` attribute, so the tests can check them.
- Avatars are generated from the handle (`lib/avatar.js`, `Avatar.svelte`). Same handle, same face everywhere.
- The page reconnects by itself, checks the connection when the tab comes back, and returns you to the same room.

## 5. Wire protocol (JSON text frames, short keys)

First frame: `{"t":"hi","h":"ada"}` new guest, `{"t":"hi","k":"<token>"}` returning, `{"t":"login","h":"ada","p":"pw"}`.
`hi` and `login` may add `"r":"football","l":42` to land in a room (l = last message id seen there).

Client to server: `join {r,l}`, `leave`, `m {b, q?}` (message, optional reply id), `del {i}` (own message),
`pw {p}`, `rep {i,c}` (spam, abuse, other), `p` (ping), `bye` (log out), `reports`, `wallet`, `dm {h,b}`, `thread {h}`, `read {h}`, `inbox`, `fr {a,h}` (add, accept, decline, cancel, remove), `blk {a,h}` (add, del), `drep {i,c}`, `profile {h}`, `av {c}`, `hi {h,v}`, `tip {i,amt,c}`, `croom {n,k,g,p,f}`, `find {n}`, `rinfo {c}`, `join {r,l,c,p,f}`, `rset {f?,pw?,k?}`, `hmod {a,h,m}`, `rclose {r?}`,
`mod {a, h?, m?, i?, id?, rid?}` with `a` = mute, kick, unmute, del, dismiss, promote, demote, grant (admin only, m = credits).

Server to client: `ok` (handle, token, points, tier, next, streak, g = has password, o = role, c = credits), `rooms`, `hub`,
`joined`, `left`, `hist`, `m` (i, h, b, ts, r = tier, o = role, q, qh, qb for replies), `n` (head count), `me` (same fields as ok, sent whenever points or credits change), `up`,
`uprofile {h,v,j,on,s,r,p|o}`, `me`/`ok` carry `v`, messages carry `av`, `dm` carries `fa`, `thread` and `inbox` carry `av` maps, `tipok {a,n,h,dup?}`, `tipgot {f,n,a}`, tip cards are `m` frames with `tt` and `ta`; `rooms` entries can carry `k` kind, `f` fee, `w` password, `o` host, `y` I can enter freely; `found`, `rinfo`, `croomok`, `rsetok`, `rmuted`, `rkicked`, `rclosed`, and `joined` carries `k f w o y i`; `dm` (i, f = from, w = to, b, ts), `thread` (h, s = none friend sent got blocked, c = price for me, m = messages), `inbox` (c chats, f friends, q incoming requests, p outgoing, x blocked), `ib` (u unread, q requests), `fr` (a = request, friends, removed; h), `frok`, `blkok`, `wallet` (c = balance, l = latest ledger rows i, k, a, b, n, ts), `del`, `mod`, `muted`, `unmuted`, `kicked`, `repok`, `rn`, `reps`, `modok`, `pwok`, `byeok`, `pong`,
`err {c, m?}`. Error codes include handle_taken, bad_handle, bad_login, too_many_attempts, session_expired,
slow_down, too_long, muted, kicked, no_room, no_such_room, tier_locked, reply_gone, forbidden, needs_password,
self_tip, bad_request, bad_avatar, bad_room_name, bad_room_kind, bad_fee, bad_room_password, room_limit, rooms_full, room_name_taken, needs_invite, fee_changed, room_banned, room_muted, not_host, bad_query, report_gone, already_reported, self_report, msg_gone, bad_amount, blocked, no_credits, self_dm, empty, friend_cap, already_friends, no_request, too_many_requests, bad_action, server_busy. Full list is in the top of `hub.go` and `README.md`.

## 6. Rules and numbers (starting values, easy to tune)

- Handle 3 to 16 letters, numbers or underscore, unique ignoring case. Message up to 300 characters.
  Rate limit: burst of 5 then 1 per second. Report limit: 10 an hour, one report per message per person.
- Points: 1 per message of 3 or more characters, same text within 30 s earns nothing, 60 an hour cap, first scoring
  message each UTC day adds a streak bonus of 10 times streak days (max 70). Tiers start at 0, 300, 2000, 10000, 40000.
- Heat flames from messages in the last 5 minutes: 1 to 4 one, 5 to 14 two, 15 to 39 three, 40 to 99 four, 100 or more five.
- Server keeps the last 200 messages per room in memory and sends the last 30 to new joiners.
- Logins last 30 days after last use. Idle guests who never set a password lose their handle after 14 days.
  Messages older than 7 days are deleted to keep the free database under 500 MB. Mutes and kicks last up to 24 hours.
- Credits: 20 welcome (once per account), 2 on the first scoring message of each UTC day, streak bonus on exactly day 3 (+5),
  7 (+15), 14 (+30), 30 (+60). Balance is on `users.credits`, every change is a `credit_ledger` row with a unique `(user_id, ref)`
  key, written in one transaction. Overspending is refused. The database is the source of truth, the server only caches the balance.
- Direct messages: 300 characters, same rate limit bucket as room chat, no points. Price per message to a stranger 1 credit (`dmFee` in
  `dm.go`), charged in the same transaction as the insert (`ref` = `dm:<message id>`). Limits: 100 friends, 50 requests waiting, 30 out.
  Messages kept 30 days (`DM_DAYS`). Moderators and admins send free. Muted people cannot send. Reports of DMs are stored with the text
  prefixed `[direct message]` and a moderator's Delete removes the DM.
- Community rooms: name 3 to 30 characters, unique ignoring case, up to 3 tags, password 4 to 32, fee 0 to 50 (host share 80%), 200 rooms in total.
  Creation fee, limit check and insert are one transaction (`createRoom`). Entry is checked against the database price and the visitor must send the price
  they saw (`enterRoom`). Invite codes are 8 characters. Hub lists a community room only while it has people (public, no password) or for its host and members.
- Tips: amounts 1, 5, 10, 25, 50 with names Suya, Burger, Pizza, Cake, Feast (`tipOptions` in `tips.go` and `TIPS` in `ui/src/lib/tips.js`, keep both in step).
  Sender and recipient rows are locked in id order, the sender's entry key is `tip:<page key>`, the recipient's is `tip:<page key>:from:<sender id>`.
  A tip needs the author of a message in the room you are in, never yourself, never a tip card. Rate limit shared with chat.
- Settings (environment variables): `DATABASE_URL` (required), `PORT`, `ALLOWED_ORIGINS`, `SESSION_DAYS`,
  `GUEST_DAYS`, `HISTORY_DAYS`, `DM_DAYS`, `ROOM_IDLE_DAYS`, `FIRST_CLEANUP_SECONDS`, `HASH_ITERATIONS`, `FLUSH_MS`, `ADMIN_HANDLE`, `ADMIN_PASSWORD`.
- Moderation by account only: a muted guest can pick a new handle. Admin account is created or adopted at start from
  `ADMIN_HANDLE` and `ADMIN_PASSWORD` (adopting an existing guest handle ends its old logins). The admin is **Eldorado**.

## 7. Hosting and accounts

- Code: public GitHub repo `eldoradonode/streetsync-lobby`. No secrets in it.
- Server: **Render free web service** (Docker build from the repo, new Render account, no card). Sleeps after about
  15 minutes with no traffic and takes about a minute to wake. Memory resets on a sleep, the database does not.
  Do not use Render's free Postgres: it expires after 30 days (this is what ended the user's earlier n8n hosting).
- Database: **Supabase free**, a project made only for this app (Flintage, the user's other product, must stay
  separate). The Session pooler connection string is stored as `DATABASE_URL` in Render's environment.
- Northflank failed card verification with three cards, so it is out. Oracle Always Free works with the user's card
  but only the small 1 GB instance is available and n8n already uses it. It is the backup server if Render fails.
- Cloudflare account exists (it hosts Flintage). Not used by this app yet. No domain bought yet. Free addresses are used.
- Deploy routine: unzip, copy over the repo files, `git add . && git commit && git push`. Render rebuilds. The database
  updates itself at start.

## 8. Known gaps and risks

- **No real load test yet.** The only load check was 800 local connections on the very first single-room version.
  The planned "load test gate" before scaling is still owed. Broadcasts are not batched yet.
- Single server only: message ids and online state are in memory. Scaling out needs Redis or NATS and a new id scheme.
- Not verified on a real phone or browser by the assistant (no browser in the sandbox). The user's screenshots are the
  only visual check. Slide to reply, keyboard behaviour and spacing on phones are untested by anyone but the user.
- No password reset or email. No permanent bans. No mod log viewer (read the `mod_log` table). No promote or demote
  buttons (a command only). Mention highlights only reach people who are online. Quotes of deleted messages remain.
- Staff earn points silently but never see them. Tier gates are checked at join time.
- Direct messages are written to the database one by one when sent (the charge must be atomic), so heavy DM traffic is not batched
  like room chat. Known loophole: someone written to replies free, so a person who baits a reply and then floods is only stopped by block,
  the rate limit and reports. Free replies are not capped. If the first message of a conversation is deleted (30 day cleanup or a moderator),
  who counts as the first sender can flip. No typing indicator, read receipts, unsend, or loading older messages (the last 50 load).
  Friend online status is not shown. No push for people who are away beyond the unread badge when they return.
- Looks were checked against the user's screenshots (desktop and phone Home, Messages, Profile, room, conversation, sign in). The step 10 screens (landing hero, profile page, Messages tabs,
  avatar picker, tip sheet on a phone) have not been seen yet: ask for screenshots.
- Avatars: a person's look reaches others when a message, direct message, conversation or Messages list carries it, then it is cached for the session. A look changed while others are online
  shows on their screen after the next such frame. Looks are drawn from a small table in `ui/src/lib/avatar.js`: never reorder it, a look code is an index into it. Room pictures are emoji (`ui/src/lib/roomicons.js`).
- The landing page is the sign in screen with a hero, not a separate page. No legal pages exist: the footer is one line. Profiles have no bio, followers or activity yet.
- A handle without a password that logs out is lost for good (now warned, not prevented). Guests are deleted 14 days after last use.
- (Older note, now partly out of date.) **The new look has never been seen by the assistant.** There is no browser in the sandbox. The page tests prove structure and behaviour only, not
  layout, spacing, the 900px breakpoint, safe areas or the keyboard on phones. The user's screenshots are the only visual check. Expect spacing fixes.
- The design is applied to the screens that exist. Not built yet from the mockup (and the spec the user pasted): landing page, a real sign up and login
  with email (the app has handles and passwords only, no email, no password reset), a separate Rooms directory page with category filters and
  featured and trending rows, notifications centre (no notification feature exists yet), settings pages, search results page with Rooms and Users tabs,
  create room as a modal with room type cards, room management page with Details, Members, Moderation and Reports tabs, restyled moderation screen,
  profile page with activity lists and a Message action, loading, empty and error states, connection indicator, right hand room details panel.
  The sidebar and bottom bar only have the three screens that exist: Home, Messages and Profile.
- Tips use emoji as treat icons (light, no images). They look different per device.
- Rooms people create: host mutes and kicks live in memory and end on a restart. The hub rebuilds each hub user's room list whenever a community
  room appears or disappears, which is fine for hundreds of users but must be batched for thousands. No room banners or images, no host-assigned
  co-moderators, no transfer of a room, no refund when a room closes. The creation fee and the 20% platform cut are credit sinks with no
  reporting yet. A deleted account leaves its rooms until they go idle.
- Credits are test credits only: nothing can be bought or spent yet. No refund or expiry rules. Guests without a password are deleted
  after 14 days and their ledger goes with them, so buying must require a protected handle first.
- Free tier limits: Supabase 500 MB and it pauses after 7 days without activity; Render sleep.
- The interface and the points numbers are untuned guesses until real users react.

## 9. Sandbox notes (they cost hours to learn)

- The sandbox has Node 22. Go was NOT preinstalled in the last sandbox: `apt-get install -y golang-go postgresql postgresql-client`
  gives Go 1.22.2 and Postgres 16 (the apt mirror is reachable). It cannot reach `golang.org`, `gopkg.in` or the Go checksum server. Use
  `GOPROXY=direct GOSUMDB=off GOFLAGS=-mod=mod` and pin libraries that work with Go 1.22. Do not add dependencies that
  need `golang.org/x` unless you find a way around it.
- Postgres is installed with `apt-get update && apt-get install -y postgresql postgresql-client`. Processes do not
  survive between tool calls, so start it again each time: `pg_ctlcluster 16 main start`.
- `pkill -x name`, never `pkill -f`, which can kill your own shell.
- The page mounts a keep-alive timer, so any node script that loads it must end with `process.exit()`.
- Svelte updates the screen one tick late. Page tests use `window.__lobbyFlush()`.
- Run each long command with `timeout`, and split big runs. A single call is limited to 300 seconds.
- Postgres stops between tool calls and the cluster can end up with a stale pid file. Start it before every test run with
  `pg_ctlcluster 16 main start`. A Go test run that prints nothing usually means Postgres is down.
- Use `tests/README.md` for the exact setup and run commands. Everything passes as of this handover:
  Go tests, e2e chat 58, moderation 53, rooms 61, wallet 30, dm 68, userrooms 97, tips 39, profiles 38 (all on the race detector, no races), page tests 502 (profiles 66, dm 72, tips 48, userrooms 69, dm 61, mod 55, page 52, reply 28, rooms 39, ui 47, wallet 24).
- Credit balance updates arrive as `me` frames at their own pace. Test checks about points must wait for the frame they mean
  (`recv_me` and `points_quiet` in the e2e scripts), never take the next `me` frame blindly.

## 10. Backlog, in the order I recommend

The user asked for steps 7 and 8 explicitly. After each finished step, tell the user the next step.

1. **More redesign (step 11).** The user wants the pages brought in line with the mockup, one batch at a time. Not done yet: a separate Rooms page with search, category filters, featured and trending rows;
   create room as a dialog with room type cards; room management page (Details, Members, Moderation, Reports); notifications centre (needs a notification feature first); settings pages; search results page with
   Rooms and Users tabs; moderation screen restyle; empty, loading and error states; connection indicator; right hand room details panel; legal pages. Ask which batch first.
2. **Buying credits (Paystack), on hold.** Build only when the user says so. Paystack for Nigeria first (webhooks into the ledger using the webhook reference as the `ref`), pluggable for a global provider,
   requires a protected handle (see section 8).
3. **Direct message polish** if the user asks: typing hints, online dots for friends, older messages, cap on free replies.
3. **Store and avatar customizing** (name colors, badges, bubble styles, avatar parts).
4. **Sponsors and ads** (sponsored hub tile, brand gifts and badges, events). Needs a Sponsor, Campaign and Placement
   model with impression and click counts.
5. **Load test and operations**: thousands of bot users, batching of broadcasts, data saver mode, move off the sleeping
   free tier when revenue starts, Cloudflare in front, a real domain.
6. Quality: password reset, permanent bans, auto moderation, mod log viewer, promote and demote buttons, achievements,
   reactions, leaderboards, campus rooms, voice notes, notifications for people who are away, PWA install, languages
   and right-to-left text.

## Step 11 (this delivery)

- **Sign up needs handle and password.** Server: `hi` with a new handle must carry `p` (8 to 128 chars) or it fails with
  `weak_password`; the hash is written in the same insert as the handle (`createAccount`), so no passwordless accounts
  are created. `REQUIRE_PASSWORD=0` turns it off (all old e2e scripts set it; `e2e_signup.py` runs with it on).
  Old passwordless guests keep working and still see the "protect your handle" prompt on Profile.
- **Mobile sign up overlap fixed.** Cause: `#join` had `overflow-y:auto`, so its grid row collapsed and the example chat
  card painted over it. The landing is now `#landing > .landing-inner` (a grid that grows with its content) and
  `#landing #join` has `overflow:visible`. NOT checked in a real browser by the assistant.
- **Landing background** is `Scene.svelte`, a night skyline drawn as inline SVG (7 KB, no download). It is an
  illustration, not a photo. A real photo would be a separate asset and costs data.
- **Rooms tab** (`Rooms.svelte`): search, category chips, trending, all rooms. Tiles are `RoomTile.svelte`, shared with the Hub.
- **Create room dialog** is a modal (`chat.createOpen`, rendered once in `App.svelte`) with four type cards
  (Public, Private, Password, Paid). The old field ids are kept so tests still work.
- **Legal pages** (`Legal.svelte`, `chat.legal`): Terms and Privacy as an overlay, linked from sign up and Profile.
  Draft text. Minimum age 16 is an assumption. Have it reviewed before real money moves.
- Tests: Go ok, e2e chat 58, moderation 53, rooms 61, wallet 30, dm 68, userrooms 97, tips 39, profiles 38, signup 18,
  page tests 15 files all green (new: page_step11 34). Known quirk: `TestCreateRoomChargesOnce` fails once on a brand new empty
  test database (needs the official rooms seeded first) and passes on every rerun.
- Still open: password reset by email, per IP limit on sign ups, real load test, Paystack (on hold until the look is settled).

## Step 12 (visual pass, no new features)

- Styles are now split: `ui/src/styles/tokens.css` (colours, type scale, spacing, radii, layout widths), `shell.css`
  (sidebar, top bar, bottom tabs, page containers), `components.css` (buttons, fields, cards, chips, room cards, dialogs),
  `chat.css` (messages, composer, host tools, DMs, inbox). `app.css` is the older base and still holds landing, tips,
  wallet and profile rules; move those into the new files as each screen is redone. New screens: use the tokens and
  the primitives, no new raw numbers.
- Desktop: 236px sidebar, content up to 1120px wide, room cards are a responsive grid (join button at the bottom of each
  card). Phones keep the compact room rows and the bottom tabs.
- Fixed two real layout bugs: the bottom tab bar sat below the screen on phones (`.screen` had a fixed `height:100dvh`
  and could not shrink), and the Rooms directory had no scroll container so the whole page scrolled.
- Keyboard: viewport meta has `interactive-widget=resizes-content`, and `main.js` sizes the shell to
  `visualViewport.height` (`--app-h`) so the message box stays above the keyboard on iOS too. Checked by shrinking the
  window in Chromium, not with a real phone keyboard.
- Host tools are a labelled panel (invite link, who can find it, entry fee, password, close room) instead of loose inputs.
- Not done on purpose: two pane Messages on desktop, a right hand room info panel, notifications and settings screens
  (the mockup shows them, the product has no such features yet), profile and wallet restyle, landing page.
- Tests: `tests/browser_layout.py` (12 checks, real Chromium) added; everything else unchanged and green.

## Step 13 (profile, wallet, admin console, moderator dashboard)

- **Server.** New `admin.go`: read only staff queries `astats`, `ausers`, `arooms` (staff) and `astaff`, `alog` (admin only);
  every one re-checks the role on the server. Moderators get a reduced `astats` (no totals, no credits) and `ausers`
  without credits or last seen. New `bio` frame (own bio, 160 chars, tidied) and mod action `clearbio`. `dir` frame returns
  every public community room, also empty ones (the hub list only shows rooms with people in them). `croom` takes a
  category `c` from a fixed list of ten (`roomCategories` in `rooms_user.go`), empty means General. `users.bio` column is
  added by `schema.sql` at start. Role rules were already right and are unchanged: promote and demote are admin only, nobody
  acts on themselves or on an admin, only an admin acts on a moderator, promote needs a password.
- **Admin accounts** are still created only from `ADMIN_HANDLE` and `ADMIN_PASSWORD`. There is deliberately no way to make an admin from the UI.
- **Client.** Wallet is its own page and tab (`Wallet.svelte`). Profile is rebuilt (`Profile.svelte`): hero card, bio editor,
  join date, tier, points, streak, credits, staff card. Staff tools moved out of Profile into two areas, `StaffArea.svelte`
  (views `admin` and `mod`, sections in `lib/areas.js`): overview with charts, users, moderators (admin), rooms, reports, audit log (admin).
  Admin is rose themed, moderator is teal, set by `data-area` on `#shell` (`styles/staff.css`). Phones get a section tab strip
  instead of the bottom bar inside these areas.
- **Rooms page.** A room in Trending is not repeated in the list; all other rooms stay. The directory (`dir`) is requested
  at sign in and on opening Home or Rooms. Hub shows 8 community rooms and a "See all" button.
- **Tests.** New `tests/e2e_admin.py` (76), `tests/page_step13.test.js` (69), `tests/browser_layout.py` now 34 checks in real Chromium.
  Moderator page tests were rewritten for the new dashboard. Bug caught on the way: `userBySession` still selected the old column list (it broke every token login); fixed.
- **Not done:** two pane Messages, notifications, settings, a ban list, search by anything other than handle, per day charts older than 7 days (messages are deleted after 7 days), room moderation by name inside admin (staff can open rooms like anyone).
- The page grew from 172 KB to 210 KB (about 65 KB over the wire). Staff screens ship to everyone; lazy loading them is the next size win.

## 11. Ask the user first

1. Did step 10 deploy cleanly? Ask for screenshots: the sign in page on desktop and phone, Messages with its tabs, a profile page, the avatar picker, a room. Try: sign up with a Female look, tap a name, Message,
   log out of a guest (should warn), use the sidebar from inside a room.
2. Then ask which batch of redesigned screens comes next (see the backlog).
3. Do they have a real app name yet? A name decides the domain and the header text.

## 12. Suggested first message for the new chat

> Continue the project from HANDOVER.md. I uploaded the latest zip. Step 10 (redesign round 2 and the landing page) is deployed and here is
> what I see: (write what you noticed). Next: (which screens to redesign).
