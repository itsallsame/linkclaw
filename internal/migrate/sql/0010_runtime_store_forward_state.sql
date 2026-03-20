CREATE TABLE IF NOT EXISTS runtime_store_forward_state (
  self_id TEXT NOT NULL,
  route_label TEXT NOT NULL,
  cursor_value TEXT NOT NULL DEFAULT '',
  last_result TEXT NOT NULL DEFAULT '',
  last_error TEXT NOT NULL DEFAULT '',
  last_recovered_count INTEGER NOT NULL DEFAULT 0,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (self_id, route_label)
);

CREATE INDEX IF NOT EXISTS idx_runtime_store_forward_state_updated_at
  ON runtime_store_forward_state(updated_at);
