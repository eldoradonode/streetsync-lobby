-- Step 18: profile photos. Only the storage key and a version live here, never the image. Safe to run more than once.
ALTER TABLE users ADD COLUMN IF NOT EXISTS photo_key text;
ALTER TABLE users ADD COLUMN IF NOT EXISTS photo_ver integer NOT NULL DEFAULT 0;
