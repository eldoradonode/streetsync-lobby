# Lobby server (steps 1 to 3)

One public room with a database behind it.

- Pick an @handle and chat. Guests need nothing else.
- Protect your handle with a password to log in from any device.
- Real messages earn points. Points move you through five tiers: Newbie, Regular, Insider, Elite, Legend.
  The tier badge shows on every message.
- Handles, sessions, points and the last days of chat survive restarts and sleeps.

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
| `GUEST_DAYS` | `14` | Guests who never set a password lose their handle after this many idle days. |
| `HISTORY_DAYS` | `7` | Chat messages older than this are deleted, to keep the free database small. |
| `HASH_ITERATIONS` | `600000` | Password hashing strength. One hash takes about a third of a second on a full CPU core, and longer on a small free instance. Only setting a password and logging in with one do this work. Resuming a saved login does not. |
| `FLUSH_MS` | `5000` | How often points are written to the database. |

## How points work (starting values, easy to change in `rank.go`)

- A message of 3 or more characters earns 1 point. Shorter messages are sent but earn nothing.
- The same text again within 30 seconds earns nothing.
- At most 60 message points per hour.
- First scoring message of each day: a streak bonus of 10 points times your streak days (up to 7).
- Time spent connected earns nothing.
- Tiers start at 0, 300, 2000, 10000 and 40000 points.

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
- `{"t":"hi","k":"<token>","l":42}` returning user, `l` is the last message id seen
- `{"t":"login","h":"ada","p":"password"}` user with a password

After that: `{"t":"m","b":"text"}` sends a message, `{"t":"pw","p":"password"}` protects the handle,
and `{"t":"p"}` is a ping.

Limits: handle 3 to 16 letters, numbers or underscores. Messages up to 300 characters.
Rate limit is a burst of 5 messages, then 1 per second. 8 failed logins per handle in 15 minutes locks that handle for the rest of the window.
