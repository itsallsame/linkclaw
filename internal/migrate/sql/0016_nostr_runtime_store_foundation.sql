CREATE TABLE IF NOT EXISTS runtime_transport_bindings (
  binding_id TEXT PRIMARY KEY,
  self_id TEXT NOT NULL DEFAULT '',
  canonical_id TEXT NOT NULL DEFAULT '',
  transport TEXT NOT NULL DEFAULT '',
  relay_url TEXT NOT NULL DEFAULT '',
  route_label TEXT NOT NULL DEFAULT '',
  route_type TEXT NOT NULL DEFAULT '',
  direction TEXT NOT NULL DEFAULT 'both',
  enabled INTEGER NOT NULL DEFAULT 1,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_transport_bindings_self_transport
  ON runtime_transport_bindings(self_id, transport);

CREATE INDEX IF NOT EXISTS idx_runtime_transport_bindings_canonical
  ON runtime_transport_bindings(canonical_id);

CREATE TABLE IF NOT EXISTS runtime_transport_relays (
  relay_id TEXT PRIMARY KEY,
  transport TEXT NOT NULL DEFAULT '',
  relay_url TEXT NOT NULL UNIQUE,
  read_enabled INTEGER NOT NULL DEFAULT 1,
  write_enabled INTEGER NOT NULL DEFAULT 1,
  priority INTEGER NOT NULL DEFAULT 0,
  source TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  last_error TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_transport_relays_transport_priority
  ON runtime_transport_relays(transport, priority DESC, relay_url ASC);

CREATE INDEX IF NOT EXISTS idx_runtime_transport_relays_status
  ON runtime_transport_relays(status);

CREATE TABLE IF NOT EXISTS runtime_relay_sync_state (
  self_id TEXT NOT NULL,
  relay_url TEXT NOT NULL,
  last_cursor TEXT NOT NULL DEFAULT '',
  last_event_at TEXT NOT NULL DEFAULT '',
  last_sync_started_at TEXT NOT NULL DEFAULT '',
  last_sync_completed_at TEXT NOT NULL DEFAULT '',
  last_result TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  recovered_count_total INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (self_id, relay_url)
);

CREATE INDEX IF NOT EXISTS idx_runtime_relay_sync_state_updated_at
  ON runtime_relay_sync_state(updated_at);

CREATE TABLE IF NOT EXISTS runtime_relay_delivery_attempts (
  attempt_id TEXT PRIMARY KEY,
  message_id TEXT NOT NULL DEFAULT '',
  event_id TEXT NOT NULL DEFAULT '',
  self_id TEXT NOT NULL DEFAULT '',
  canonical_id TEXT NOT NULL DEFAULT '',
  relay_url TEXT NOT NULL DEFAULT '',
  operation TEXT NOT NULL DEFAULT '',
  outcome TEXT NOT NULL DEFAULT '',
  error TEXT NOT NULL DEFAULT '',
  retryable INTEGER NOT NULL DEFAULT 0,
  acknowledged INTEGER NOT NULL DEFAULT 0,
  metadata_json TEXT NOT NULL DEFAULT '{}',
  attempted_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_runtime_relay_delivery_attempts_message
  ON runtime_relay_delivery_attempts(message_id, attempted_at);

CREATE INDEX IF NOT EXISTS idx_runtime_relay_delivery_attempts_relay
  ON runtime_relay_delivery_attempts(relay_url, attempted_at);

CREATE TABLE IF NOT EXISTS runtime_recovered_event_observations (
  self_id TEXT NOT NULL,
  event_id TEXT NOT NULL,
  relay_url TEXT NOT NULL DEFAULT '',
  canonical_id TEXT NOT NULL DEFAULT '',
  message_id TEXT NOT NULL DEFAULT '',
  observed_at TEXT NOT NULL,
  payload_hash TEXT NOT NULL DEFAULT '',
  payload_json TEXT NOT NULL DEFAULT '',
  metadata_json TEXT NOT NULL DEFAULT '{}',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (self_id, event_id, relay_url)
);

CREATE INDEX IF NOT EXISTS idx_runtime_recovered_event_observations_observed_at
  ON runtime_recovered_event_observations(observed_at);

CREATE INDEX IF NOT EXISTS idx_runtime_recovered_event_observations_canonical
  ON runtime_recovered_event_observations(canonical_id, observed_at);
