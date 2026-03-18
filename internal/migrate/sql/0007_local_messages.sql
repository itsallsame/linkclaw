CREATE TABLE IF NOT EXISTS conversations (
  conversation_id TEXT PRIMARY KEY,
  contact_id TEXT NOT NULL UNIQUE,
  last_message_at TEXT NOT NULL DEFAULT '',
  last_message_preview TEXT NOT NULL DEFAULT '',
  unread_count INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS messages (
  message_id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  direction TEXT NOT NULL,
  sender_contact_id TEXT NOT NULL DEFAULT '',
  recipient_contact_id TEXT NOT NULL DEFAULT '',
  sender_canonical_id TEXT NOT NULL DEFAULT '',
  recipient_route_id TEXT NOT NULL DEFAULT '',
  plaintext_body TEXT NOT NULL DEFAULT '',
  plaintext_preview TEXT NOT NULL DEFAULT '',
  ciphertext TEXT NOT NULL DEFAULT '',
  signature TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  remote_message_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  synced_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_messages_conversation_created_at
  ON messages(conversation_id, created_at);

CREATE INDEX IF NOT EXISTS idx_messages_direction_status
  ON messages(direction, status);

CREATE TABLE IF NOT EXISTS message_delivery_attempts (
  attempt_id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL,
  relay_url TEXT NOT NULL DEFAULT '',
  attempted_at TEXT NOT NULL,
  result TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_message_delivery_attempts_message_id
  ON message_delivery_attempts(message_id);

CREATE TABLE IF NOT EXISTS message_sync_cursors (
  profile_id TEXT PRIMARY KEY,
  relay_url TEXT NOT NULL DEFAULT '',
  last_cursor TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL
);
