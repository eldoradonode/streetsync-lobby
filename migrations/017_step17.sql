-- Step 17: DM delivery receipts and faster people search. Safe to run more than once.
ALTER TABLE dm_messages ADD COLUMN IF NOT EXISTS delivered_at timestamptz;
CREATE INDEX IF NOT EXISTS dm_undelivered ON dm_messages (recipient_id, sender_id) WHERE delivered_at IS NULL AND read_at IS NULL;
-- Substring search on handles. Optional: skipped quietly where the extension cannot be created.
DO $$ BEGIN
  CREATE EXTENSION IF NOT EXISTS pg_trgm;
  CREATE INDEX IF NOT EXISTS users_handle_trgm ON users USING gin (handle_key gin_trgm_ops);
EXCEPTION WHEN OTHERS THEN
  RAISE NOTICE 'pg_trgm index skipped: %', SQLERRM;
END $$;
