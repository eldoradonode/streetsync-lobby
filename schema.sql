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

-- ---- notifications and settings ----
-- One row per event. Chat style events (a new direct message, a tag in a room) are grouped per person and place,
-- so ten messages from one friend are one notification with a count, not ten.
CREATE TABLE IF NOT EXISTS notifications (
  id          bigserial PRIMARY KEY,
  user_id     bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  kind        text NOT NULL,                 -- dm, tag, reply, friend, tip
  actor       text NOT NULL,                 -- the handle of the person who caused it
  ref         text NOT NULL,                 -- what makes it the same event again
  room        text,                          -- room slug, when it happened in a room
  room_name   text,
  body        text,
  amount      integer NOT NULL DEFAULT 0,
  n           integer NOT NULL DEFAULT 1,
  created_at  timestamptz NOT NULL DEFAULT now(),
  read_at     timestamptz,
  UNIQUE (user_id, ref)
);
CREATE INDEX IF NOT EXISTS notifications_user_idx ON notifications (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS notifications_unread_idx ON notifications (user_id) WHERE read_at IS NULL;
ALTER TABLE notifications ENABLE ROW LEVEL SECURITY;

-- Settings people can change. A person with no row has the defaults.
CREATE TABLE IF NOT EXISTS user_prefs (
  user_id         bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  n_dm            boolean NOT NULL DEFAULT true,
  n_tag           boolean NOT NULL DEFAULT true,
  n_tip           boolean NOT NULL DEFAULT true,
  n_friend        boolean NOT NULL DEFAULT true,
  dm_friends_only boolean NOT NULL DEFAULT false,
  hide_online     boolean NOT NULL DEFAULT false
);
ALTER TABLE user_prefs ENABLE ROW LEVEL SECURITY;

-- ---- step 15: bans, deletion, descriptions, reports, complaints ----
-- Safe to run again at every start. Nothing here deletes data.

-- A permanent ban is a column on the account, so it survives logout, reconnecting and a new session.
-- deleted_at marks a deleted account. The row stays as an anonymous tombstone (see README, "Account deletion")
-- so the credit ledger keeps a valid pseudonymous reference.
ALTER TABLE users ADD COLUMN IF NOT EXISTS banned_at  timestamptz;
ALTER TABLE users ADD COLUMN IF NOT EXISTS banned_by  text;
ALTER TABLE users ADD COLUMN IF NOT EXISTS ban_reason text;
ALTER TABLE users ADD COLUMN IF NOT EXISTS deleted_at timestamptz;
CREATE INDEX IF NOT EXISTS users_banned ON users (banned_at) WHERE banned_at IS NOT NULL;

-- The ledger must never be removed by deleting a user row. It used to cascade; it now refuses (NO ACTION).
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'credit_ledger_user_id_fkey' AND confdeltype = 'c') THEN
    ALTER TABLE credit_ledger DROP CONSTRAINT credit_ledger_user_id_fkey;
    ALTER TABLE credit_ledger ADD CONSTRAINT credit_ledger_user_id_fkey
      FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE NO ACTION;
  END IF;
END $$;

-- Room descriptions and leaving a room. A left room keeps its row, so paying again is never needed.
ALTER TABLE rooms        ADD COLUMN IF NOT EXISTS description text NOT NULL DEFAULT '';
ALTER TABLE room_members ADD COLUMN IF NOT EXISTS left_at timestamptz;

-- Reports now cover messages, people and rooms.
ALTER TABLE reports ADD COLUMN IF NOT EXISTS kind           text NOT NULL DEFAULT 'message';
ALTER TABLE reports ADD COLUMN IF NOT EXISTS target_user_id bigint;
ALTER TABLE reports ADD COLUMN IF NOT EXISTS target_room    text;
ALTER TABLE reports ADD COLUMN IF NOT EXISTS details        text NOT NULL DEFAULT '';
ALTER TABLE reports ALTER COLUMN msg_id DROP NOT NULL;
DROP INDEX IF EXISTS reports_once;
CREATE UNIQUE INDEX IF NOT EXISTS reports_once_msg  ON reports (reporter_id, msg_id) WHERE kind = 'message';
CREATE UNIQUE INDEX IF NOT EXISTS reports_once_user ON reports (reporter_id, target_user_id) WHERE kind = 'user' AND status = 'open';
CREATE UNIQUE INDEX IF NOT EXISTS reports_once_room ON reports (reporter_id, target_room) WHERE kind = 'room' AND status = 'open';

-- Complaints about staff. Moderators never see these, only admins, and never the admin complained about.
CREATE TABLE IF NOT EXISTS staff_complaints (
  id              bigserial PRIMARY KEY,
  reporter_id     bigint NOT NULL,
  reporter_handle text NOT NULL,
  target_id       bigint NOT NULL,
  target_handle   text NOT NULL,
  target_role     text NOT NULL,
  msg_id          bigint,
  body            text NOT NULL DEFAULT '',
  category        text NOT NULL,
  details         text NOT NULL DEFAULT '',
  status          text NOT NULL DEFAULT 'open',
  note            text NOT NULL DEFAULT '',
  created_at      timestamptz NOT NULL DEFAULT now(),
  handled_by      text,
  handled_at      timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS staff_complaints_once ON staff_complaints (reporter_id, target_id) WHERE status = 'open';
CREATE INDEX IF NOT EXISTS staff_complaints_open ON staff_complaints (status, id DESC);
ALTER TABLE staff_complaints ENABLE ROW LEVEL SECURITY;

-- ---- Step 16: end-to-end encrypted direct messages ----
-- The server stores only ciphertext for new direct messages. enc = 0 rows are legacy plaintext from before
-- this step; they are not converted and they are deleted by the normal retention window (DM_DAYS, default 7).
-- Encrypted rows keep body = '' and hold the envelope in ct / nonce / sfp / rfp / mid.
ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS enc   smallint NOT NULL DEFAULT 0;
ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS ct    text;
ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS nonce text;
ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS sfp   text;   -- fingerprint of the sender's public key
ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS rfp   text;   -- fingerprint of the recipient's public key
ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS mid   text;   -- random message id chosen by the sender, bound into the encryption
CREATE UNIQUE INDEX IF NOT EXISTS dm_mid ON dm_messages (sender_id, mid) WHERE mid IS NOT NULL;

-- One public encryption key per account. Public only: the private key never leaves the person's device.
CREATE TABLE IF NOT EXISTS e2e_keys (
  user_id    bigint PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  pub        text NOT NULL,                 -- P-256 public key, uncompressed point, base64
  fp         text NOT NULL,                 -- SHA-256 of pub, hex
  ver        int  NOT NULL DEFAULT 1,       -- goes up each time the key is replaced
  updated_at timestamptz NOT NULL DEFAULT now()
);
-- Append-only history of key registrations, so a replaced key leaves a trace.
CREATE TABLE IF NOT EXISTS e2e_key_log (
  id         bigserial PRIMARY KEY,
  user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  fp         text NOT NULL,
  ver        int  NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS e2e_key_log_user ON e2e_key_log (user_id, id DESC);
ALTER TABLE e2e_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE e2e_key_log ENABLE ROW LEVEL SECURITY;

-- ---- Step 17 (also in migrations/017_step17.sql) ----
ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS delivered_at timestamptz;
CREATE INDEX IF NOT EXISTS dm_undelivered ON dm_messages (recipient_id, sender_id) WHERE delivered_at IS NULL AND read_at IS NULL;
-- Substring search on handles. Optional: skipped quietly where the extension cannot be created.
DO $$ BEGIN
  CREATE EXTENSION IF NOT EXISTS pg_trgm;
  CREATE INDEX IF NOT EXISTS users_handle_trgm ON users USING gin (handle_key gin_trgm_ops);
EXCEPTION WHEN OTHERS THEN
  RAISE NOTICE 'pg_trgm index skipped: %', SQLERRM;
END $$;

-- ---- Step 18 (also in migrations/018_step18.sql) ----
ALTER TABLE users ADD COLUMN IF NOT EXISTS photo_key text;
ALTER TABLE users ADD COLUMN IF NOT EXISTS photo_ver integer NOT NULL DEFAULT 0;

-- ===== Step 19 (also in migrations/019_step19.sql) =====
-- Step 19: pinned room message, message reactions, room polls. Safe to run more than once. No existing data is changed.
-- There is deliberately no foreign key to messages: room messages are saved in batches a moment after they are sent.

-- One pinned message per room. The text is a snapshot, so an announcement survives the seven day history cleanup.
CREATE TABLE IF NOT EXISTS room_pins (
  room      text PRIMARY KEY REFERENCES rooms(slug) ON DELETE CASCADE,
  msg_id    bigint NOT NULL,
  handle    text   NOT NULL,
  body      text   NOT NULL,
  msg_ts    bigint NOT NULL,
  pinned_by text   NOT NULL,
  pinned_at timestamptz NOT NULL DEFAULT now()
);

-- One reaction per account per message (add, change, remove). Only emoji from a fixed list are accepted by the server.
CREATE TABLE IF NOT EXISTS message_reactions (
  msg_id     bigint NOT NULL,
  room       text   NOT NULL REFERENCES rooms(slug) ON DELETE CASCADE,
  user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  emoji      text   NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (msg_id, user_id)
);
CREATE INDEX IF NOT EXISTS message_reactions_room ON message_reactions (room, msg_id);

-- Polls. At most one open poll per room (partial unique index). One vote per account per poll (primary key).
CREATE TABLE IF NOT EXISTS polls (
  id                bigserial PRIMARY KEY,
  room              text   NOT NULL REFERENCES rooms(slug) ON DELETE CASCADE,
  question          text   NOT NULL,
  options           jsonb  NOT NULL,
  created_by        bigint NOT NULL,
  created_by_handle text   NOT NULL,
  created_at        timestamptz NOT NULL DEFAULT now(),
  closed_at         timestamptz,
  closed_by         text
);
CREATE UNIQUE INDEX IF NOT EXISTS polls_one_open ON polls (room) WHERE closed_at IS NULL;
CREATE INDEX IF NOT EXISTS polls_room ON polls (room, id DESC);
CREATE TABLE IF NOT EXISTS poll_votes (
  poll_id  bigint   NOT NULL REFERENCES polls(id) ON DELETE CASCADE,
  user_id  bigint   NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  opt      smallint NOT NULL CHECK (opt >= 0),
  voted_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (poll_id, user_id)
);

-- Same lock-down as the other tables: the server connects as the owner, the public web API gets nothing.
ALTER TABLE room_pins          ENABLE ROW LEVEL SECURITY;
ALTER TABLE message_reactions  ENABLE ROW LEVEL SECURITY;
ALTER TABLE polls              ENABLE ROW LEVEL SECURITY;
ALTER TABLE poll_votes         ENABLE ROW LEVEL SECURITY;

-- ===== Step 20 (also in migrations/020_step20.sql) =====
-- Step 20: favourite rooms and recently visited rooms. Safe to run more than once. No existing data is changed.
-- Both tables follow the room: when a room is deleted or closed its favourite and visit rows go with it.
CREATE TABLE IF NOT EXISTS room_favourites (
  user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  room       text   NOT NULL REFERENCES rooms(slug) ON DELETE CASCADE,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, room)
);
CREATE INDEX IF NOT EXISTS room_favourites_user ON room_favourites (user_id, created_at DESC);

CREATE TABLE IF NOT EXISTS room_visits (
  user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  room       text   NOT NULL REFERENCES rooms(slug) ON DELETE CASCADE,
  visited_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, room)
);
CREATE INDEX IF NOT EXISTS room_visits_user ON room_visits (user_id, visited_at DESC);

ALTER TABLE room_favourites ENABLE ROW LEVEL SECURITY;
ALTER TABLE room_visits     ENABLE ROW LEVEL SECURITY;
