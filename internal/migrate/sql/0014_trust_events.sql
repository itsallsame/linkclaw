CREATE TABLE IF NOT EXISTS trust_events (
  event_id TEXT PRIMARY KEY,
  trust_id TEXT NOT NULL DEFAULT '',
  contact_id TEXT NOT NULL DEFAULT '',
  canonical_id TEXT NOT NULL DEFAULT '',
  trust_level TEXT NOT NULL DEFAULT 'unknown',
  risk_flags_json TEXT NOT NULL DEFAULT '[]',
  verification_state TEXT NOT NULL DEFAULT '',
  decision_reason TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  decided_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_trust_events_contact_decided_at
  ON trust_events(contact_id, decided_at);

CREATE INDEX IF NOT EXISTS idx_trust_events_canonical_decided_at
  ON trust_events(canonical_id, decided_at);
