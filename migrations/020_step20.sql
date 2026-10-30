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
