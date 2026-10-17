-- Applied at every startup. Safe to run again.

CREATE TABLE IF NOT EXISTS users (
  id          bigserial PRIMARY KEY,
  handle      text NOT NULL,
  handle_key  text NOT NULL UNIQUE,          -- lowercase handle, used for uniqueness
  pass_hash   text,                          -- empty for guests
  points      bigint NOT NULL DEFAULT 0,
  streak_days int NOT NULL DEFAULT 0,
  streak_day  int NOT NULL DEFAULT 0,        -- last day counted, days since 1970-01-01 UTC
  created_at  timestamptz NOT NULL DEFAULT now(),
  last_seen   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS users_guest_idle ON users (last_seen) WHERE pass_hash IS NULL;

CREATE TABLE IF NOT EXISTS sessions (
  token_hash text PRIMARY KEY,               -- sha256 of the token, never the token itself
  user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_used  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sessions_user ON sessions (user_id);
CREATE INDEX IF NOT EXISTS sessions_last_used ON sessions (last_used);

CREATE TABLE IF NOT EXISTS messages (
  id         bigint PRIMARY KEY,
  room       text NOT NULL,
  user_id    bigint,
  handle     text NOT NULL,
  tier       smallint NOT NULL DEFAULT 0,    -- sender's tier when the message was sent
  body       text NOT NULL,
  created_at timestamptz NOT NULL
);
-- Replies keep the id, sender and a short snippet of the message they answer.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS reply_to     bigint;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS reply_handle text;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS reply_snip   text;
CREATE INDEX IF NOT EXISTS messages_room_id ON messages (room, id DESC);
CREATE INDEX IF NOT EXISTS messages_created ON messages (created_at);

-- Supabase exposes tables in the public schema through its own web API.
-- Row level security with no policies keeps these tables closed to that API.
-- This server connects as the database owner, which is not affected.
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE messages ENABLE ROW LEVEL SECURITY;

-- ---- moderation ----

-- user, mod or admin. The admin account is created from settings at startup.
ALTER TABLE users ADD COLUMN IF NOT EXISTS role text NOT NULL DEFAULT 'user';

-- Mutes and kicks. A row is active until its "until" time passes.
CREATE TABLE IF NOT EXISTS sanctions (
  id         bigserial PRIMARY KEY,
  user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind       text NOT NULL CHECK (kind IN ('mute', 'kick')),
  until      timestamptz NOT NULL,
  by_handle  text NOT NULL,
  reason     text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS sanctions_active ON sanctions (user_id, kind, until);

-- Reports keep a copy of the message, because old messages are deleted to save space.
CREATE TABLE IF NOT EXISTS reports (
  id              bigserial PRIMARY KEY,
  reporter_id     bigint NOT NULL,
  reporter_handle text NOT NULL,
  msg_id          bigint NOT NULL,
  target_handle   text NOT NULL,
  body            text NOT NULL,
  reason          text NOT NULL,
  status          text NOT NULL DEFAULT 'open',
  created_at      timestamptz NOT NULL DEFAULT now(),
  handled_by      text,
  handled_at      timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS reports_once ON reports (reporter_id, msg_id);
CREATE INDEX IF NOT EXISTS reports_open ON reports (status, id DESC);

-- Who did what, kept for accountability.
CREATE TABLE IF NOT EXISTS mod_log (
  id         bigserial PRIMARY KEY,
  actor      text NOT NULL,
  action     text NOT NULL,
  target     text,
  detail     text,
  created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE sanctions ENABLE ROW LEVEL SECURITY;
ALTER TABLE reports ENABLE ROW LEVEL SECURITY;
ALTER TABLE mod_log ENABLE ROW LEVEL SECURITY;

-- ---- rooms ----

-- Official rooms shown on the hub. Edit them here: rename, reorder, change the tier needed.
-- They are only created when missing, so your edits are never overwritten at startup.
CREATE TABLE IF NOT EXISTS rooms (
  slug       text PRIMARY KEY,
  name       text NOT NULL,
  category   text NOT NULL DEFAULT 'General',
  tags       text NOT NULL DEFAULT '',        -- comma separated, shown under the name
  min_tier   smallint NOT NULL DEFAULT 0,     -- 0 Newbie, 1 Regular, 2 Insider, 3 Elite, 4 Legend
  sort       int NOT NULL DEFAULT 100,
  created_at timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE rooms ENABLE ROW LEVEL SECURITY;

-- The sender's role when the message was sent (mod or admin), empty for normal users.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS role text;

-- ---- credits and wallet ----

-- The balance lives on the user row. Every change is also written to the ledger in the same transaction.
ALTER TABLE users ADD COLUMN IF NOT EXISTS credits  bigint  NOT NULL DEFAULT 0 CHECK (credits >= 0);
ALTER TABLE users ADD COLUMN IF NOT EXISTS welcomed boolean NOT NULL DEFAULT false;

-- One row per credit movement. Positive amounts add, negative amounts spend.
-- "ref" is an idempotency key: the same (user, ref) can only be applied once,
-- so a repeated webhook, a retry or a double click never pays or charges twice.
CREATE TABLE IF NOT EXISTS credit_ledger (
  id            bigserial PRIMARY KEY,
  user_id       bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind          text   NOT NULL,   -- welcome, daily, streak, grant (later: buy, tip, gift, spend, refund)
  amount        bigint NOT NULL CHECK (amount <> 0),
  balance_after bigint NOT NULL,
  ref           text   NOT NULL,
  note          text   NOT NULL DEFAULT '',
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS credit_ledger_ref ON credit_ledger (user_id, ref);
CREATE INDEX IF NOT EXISTS credit_ledger_user ON credit_ledger (user_id, id DESC);
ALTER TABLE credit_ledger ENABLE ROW LEVEL SECURITY;

-- ---- friends, blocks and direct messages ----

-- One row per pair of people. user_a is always the smaller id, so a pair can only exist once.
CREATE TABLE IF NOT EXISTS friendships (
  user_a      bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  user_b      bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  requester   bigint NOT NULL,                 -- who sent the request
  status      text   NOT NULL CHECK (status IN ('pending', 'accepted')),
  created_at  timestamptz NOT NULL DEFAULT now(),
  accepted_at timestamptz,
  PRIMARY KEY (user_a, user_b),
  CHECK (user_a < user_b)
);
CREATE INDEX IF NOT EXISTS friendships_b ON friendships (user_b);

CREATE TABLE IF NOT EXISTS blocks (
  blocker    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  blocked    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (blocker, blocked)
);
CREATE INDEX IF NOT EXISTS blocks_blocked ON blocks (blocked);

-- Ids come from the same counter as room messages, so a report can point at either kind.
CREATE TABLE IF NOT EXISTS dm_messages (
  id           bigint PRIMARY KEY,
  sender_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  recipient_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  body         text   NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  read_at      timestamptz
);
CREATE INDEX IF NOT EXISTS dm_pair ON dm_messages (LEAST(sender_id, recipient_id), GREATEST(sender_id, recipient_id), id DESC);
CREATE INDEX IF NOT EXISTS dm_sender ON dm_messages (sender_id, id DESC);
CREATE INDEX IF NOT EXISTS dm_recipient ON dm_messages (recipient_id, id DESC);
CREATE INDEX IF NOT EXISTS dm_unread ON dm_messages (recipient_id) WHERE read_at IS NULL;
CREATE INDEX IF NOT EXISTS dm_created ON dm_messages (created_at);

ALTER TABLE friendships ENABLE ROW LEVEL SECURITY;
ALTER TABLE blocks ENABLE ROW LEVEL SECURITY;
ALTER TABLE dm_messages ENABLE ROW LEVEL SECURITY;

-- ---- rooms people create ----

-- kind: official (yours, from the list in rooms.go), public (listed on the hub while people are in it) or private (only by invite code).
-- A room with a password is never listed, whatever its kind.
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS kind        text NOT NULL DEFAULT 'official';
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS owner_id    bigint REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS pass_hash   text;
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS entry_fee   int NOT NULL DEFAULT 0 CHECK (entry_fee >= 0);
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS invite      text;
ALTER TABLE rooms ADD COLUMN IF NOT EXISTS last_active timestamptz NOT NULL DEFAULT now();
CREATE UNIQUE INDEX IF NOT EXISTS rooms_invite ON rooms (invite) WHERE invite IS NOT NULL;
CREATE INDEX IF NOT EXISTS rooms_owner ON rooms (owner_id) WHERE owner_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS rooms_name ON rooms (lower(name));

-- People who got into a private, password or fee room. They come back in without code, password or fee.
CREATE TABLE IF NOT EXISTS room_members (
  room      text   NOT NULL REFERENCES rooms(slug) ON DELETE CASCADE,
  user_id   bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  joined_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (room, user_id)
);
CREATE INDEX IF NOT EXISTS room_members_user ON room_members (user_id);
ALTER TABLE room_members ENABLE ROW LEVEL SECURITY;

-- ---- tips ----

-- A tip card in a room is a message with a recipient and an amount. Its body is the name of the treat.
ALTER TABLE messages ADD COLUMN IF NOT EXISTS tip_to     text;
ALTER TABLE messages ADD COLUMN IF NOT EXISTS tip_amount int;

-- ---- avatars, and the Sport room ----

-- The look a person picked at sign up, like "f3". Empty means the page makes one from the handle.
ALTER TABLE users    ADD COLUMN IF NOT EXISTS avatar text NOT NULL DEFAULT '';
ALTER TABLE messages ADD COLUMN IF NOT EXISTS avatar text;

-- The Football room became Sport. Only an untouched official room is renamed.
UPDATE rooms SET name = 'Sport', tags = 'Football,Basketball,Athletics'
 WHERE slug = 'football' AND kind = 'official' AND name = 'Football';

-- ---- profile bio ----
ALTER TABLE users ADD COLUMN IF NOT EXISTS bio text NOT NULL DEFAULT '';
