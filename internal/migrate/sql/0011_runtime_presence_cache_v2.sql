ALTER TABLE runtime_presence_cache
  ADD COLUMN transport_capabilities_json TEXT NOT NULL DEFAULT '[]';

ALTER TABLE runtime_presence_cache
  ADD COLUMN store_forward_hints_json TEXT NOT NULL DEFAULT '[]';

ALTER TABLE runtime_presence_cache
  ADD COLUMN reachable INTEGER NOT NULL DEFAULT 0;

ALTER TABLE runtime_presence_cache
  ADD COLUMN announced_at TEXT NOT NULL DEFAULT '';
