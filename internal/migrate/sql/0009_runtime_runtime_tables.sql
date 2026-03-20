CREATE TABLE IF NOT EXISTS runtime_self_identities (
  self_id TEXT PRIMARY KEY,
  display_name TEXT NOT NULL DEFAULT '',
  peer_id TEXT NOT NULL DEFAULT '',
  signing_public_key TEXT NOT NULL DEFAULT '',
  encryption_public_key TEXT NOT NULL DEFAULT '',
  signing_private_key_ref TEXT NOT NULL DEFAULT '',
  encryption_private_key_ref TEXT NOT NULL DEFAULT '',
  transport_capabilities_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runtime_contacts (
  contact_id TEXT PRIMARY KEY,
  canonical_id TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL DEFAULT '',
  peer_id TEXT NOT NULL DEFAULT '',
  signing_public_key TEXT NOT NULL DEFAULT '',
  encryption_public_key TEXT NOT NULL DEFAULT '',
  trust_state TEXT NOT NULL DEFAULT 'unknown',
  transport_capabilities_json TEXT NOT NULL DEFAULT '[]',
  direct_hints_json TEXT NOT NULL DEFAULT '[]',
  store_forward_hints_json TEXT NOT NULL DEFAULT '[]',
  signed_peer_record TEXT NOT NULL DEFAULT '',
  last_seen_at TEXT NOT NULL DEFAULT '',
  last_successful_route TEXT NOT NULL DEFAULT '',
  raw_identity_card_json TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_contacts_display_name
  ON runtime_contacts(display_name);

CREATE TABLE IF NOT EXISTS runtime_conversations (
  conversation_id TEXT PRIMARY KEY,
  contact_id TEXT NOT NULL UNIQUE,
  last_message_id TEXT NOT NULL DEFAULT '',
  last_message_preview TEXT NOT NULL DEFAULT '',
  last_message_at TEXT NOT NULL DEFAULT '',
  unread_count INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runtime_messages (
  message_id TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL,
  sender_id TEXT NOT NULL DEFAULT '',
  recipient_id TEXT NOT NULL DEFAULT '',
  direction TEXT NOT NULL,
  plaintext_body TEXT NOT NULL DEFAULT '',
  plaintext_preview TEXT NOT NULL DEFAULT '',
  ciphertext TEXT NOT NULL DEFAULT '',
  ciphertext_version TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'pending',
  selected_route_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  delivered_at TEXT NOT NULL DEFAULT '',
  acked_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_runtime_messages_conversation_created_at
  ON runtime_messages(conversation_id, created_at);

CREATE TABLE IF NOT EXISTS runtime_route_attempts (
  attempt_id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL DEFAULT '',
  conversation_id TEXT NOT NULL DEFAULT '',
  route_type TEXT NOT NULL,
  route_label TEXT NOT NULL DEFAULT '',
  priority INTEGER NOT NULL DEFAULT 0,
  outcome TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  retryable INTEGER NOT NULL DEFAULT 0,
  cursor_value TEXT NOT NULL DEFAULT '',
  attempted_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_route_attempts_message_id
  ON runtime_route_attempts(message_id, attempted_at);

CREATE TABLE IF NOT EXISTS runtime_presence_cache (
  canonical_id TEXT PRIMARY KEY,
  peer_id TEXT NOT NULL DEFAULT '',
  direct_hints_json TEXT NOT NULL DEFAULT '[]',
  signed_peer_record TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  fresh_until TEXT NOT NULL DEFAULT '',
  resolved_at TEXT NOT NULL
);
