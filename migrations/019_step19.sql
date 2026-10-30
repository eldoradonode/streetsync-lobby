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
