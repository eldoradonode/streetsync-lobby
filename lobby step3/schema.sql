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
CREATE INDEX IF NOT EXISTS messages_room_id ON messages (room, id DESC);
CREATE INDEX IF NOT EXISTS messages_created ON messages (created_at);

-- Supabase exposes tables in the public schema through its own web API.
-- Row level security with no policies keeps these tables closed to that API.
-- This server connects as the database owner, which is not affected.
ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE messages ENABLE ROW LEVEL SECURITY;
