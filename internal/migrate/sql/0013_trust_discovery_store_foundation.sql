CREATE TABLE IF NOT EXISTS runtime_trust_records (
  canonical_id TEXT PRIMARY KEY,
  contact_id TEXT NOT NULL DEFAULT '',
  trust_level TEXT NOT NULL DEFAULT 'unknown',
  risk_flags_json TEXT NOT NULL DEFAULT '[]',
  verification_state TEXT NOT NULL DEFAULT '',
  decision_reason TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  decided_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_trust_records_contact
  ON runtime_trust_records(contact_id);

CREATE INDEX IF NOT EXISTS idx_runtime_trust_records_level
  ON runtime_trust_records(trust_level);

CREATE TABLE IF NOT EXISTS runtime_discovery_records (
  canonical_id TEXT PRIMARY KEY,
  peer_id TEXT NOT NULL DEFAULT '',
  route_candidates_json TEXT NOT NULL DEFAULT '[]',
  transport_capabilities_json TEXT NOT NULL DEFAULT '[]',
  direct_hints_json TEXT NOT NULL DEFAULT '[]',
  store_forward_hints_json TEXT NOT NULL DEFAULT '[]',
  signed_peer_record TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT '',
  reachable INTEGER NOT NULL DEFAULT 0,
  resolved_at TEXT NOT NULL DEFAULT '',
  fresh_until TEXT NOT NULL DEFAULT '',
  announced_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_discovery_records_resolved_at
  ON runtime_discovery_records(resolved_at);

CREATE INDEX IF NOT EXISTS idx_runtime_discovery_records_source
  ON runtime_discovery_records(source);
