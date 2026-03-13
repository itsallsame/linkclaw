CREATE TABLE IF NOT EXISTS trust_records (
  trust_id TEXT PRIMARY KEY,
  contact_id TEXT NOT NULL UNIQUE,
  trust_level TEXT NOT NULL DEFAULT 'unknown',
  risk_flags TEXT NOT NULL DEFAULT '[]',
  verification_state TEXT NOT NULL DEFAULT 'discovered',
  decision_reason TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_trust_records_contact ON trust_records(contact_id);

CREATE TABLE IF NOT EXISTS interaction_events (
  event_id TEXT PRIMARY KEY,
  contact_id TEXT NOT NULL,
  channel TEXT NOT NULL DEFAULT 'linkclaw',
  event_type TEXT NOT NULL,
  summary TEXT NOT NULL,
  event_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_interaction_events_contact_event_at ON interaction_events(contact_id, event_at);
