CREATE TABLE IF NOT EXISTS self_messaging_profiles (
  self_id TEXT PRIMARY KEY,
  recipient_id TEXT NOT NULL UNIQUE,
  relay_url TEXT NOT NULL DEFAULT '',
  signing_key_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_self_messaging_profiles_signing_key_id
  ON self_messaging_profiles(signing_key_id);
