CREATE TABLE IF NOT EXISTS index_records (
  record_id TEXT PRIMARY KEY,
  normalized_origin TEXT NOT NULL UNIQUE,
  seed_input TEXT NOT NULL DEFAULT '',
  canonical_id TEXT NOT NULL DEFAULT '',
  display_name TEXT NOT NULL DEFAULT '',
  profile_url TEXT NOT NULL DEFAULT '',
  resolver_status TEXT NOT NULL DEFAULT 'discovered',
  conflict_state TEXT NOT NULL DEFAULT 'partial',
  warnings_json TEXT NOT NULL DEFAULT '[]',
  mismatches_json TEXT NOT NULL DEFAULT '[]',
  freshness_at TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_index_records_canonical_id ON index_records(canonical_id);
CREATE INDEX IF NOT EXISTS idx_index_records_display_name ON index_records(display_name);
CREATE INDEX IF NOT EXISTS idx_index_records_freshness_at ON index_records(freshness_at DESC);

CREATE TABLE IF NOT EXISTS index_sources (
  source_id TEXT PRIMARY KEY,
  record_id TEXT NOT NULL,
  artifact_type TEXT NOT NULL,
  source_url TEXT NOT NULL,
  fetched_at TEXT NOT NULL,
  http_status INTEGER NOT NULL DEFAULT 0,
  content_hash TEXT NOT NULL DEFAULT '',
  parsed_summary TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL,
  UNIQUE(record_id, artifact_type, source_url)
);

CREATE INDEX IF NOT EXISTS idx_index_sources_record ON index_sources(record_id, fetched_at DESC);
