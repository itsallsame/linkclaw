CREATE TABLE IF NOT EXISTS self_identities (
  self_id TEXT PRIMARY KEY,
  canonical_id TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  home_origin TEXT NOT NULL DEFAULT '',
  default_profile_url TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'active',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS handles (
  handle_id TEXT PRIMARY KEY,
  owner_type TEXT NOT NULL,
  owner_id TEXT NOT NULL,
  handle_type TEXT NOT NULL,
  value TEXT NOT NULL,
  is_primary INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  UNIQUE(owner_type, owner_id, handle_type, value)
);

CREATE TABLE IF NOT EXISTS keys (
  key_id TEXT PRIMARY KEY,
  owner_type TEXT NOT NULL,
  owner_id TEXT NOT NULL,
  algorithm TEXT NOT NULL,
  public_key TEXT NOT NULL,
  private_key_ref TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  published_status TEXT NOT NULL DEFAULT 'local_only',
  rotates_from TEXT NOT NULL DEFAULT '',
  valid_from TEXT NOT NULL,
  valid_until TEXT NOT NULL DEFAULT '',
  retired_at TEXT NOT NULL DEFAULT '',
  revoked_at TEXT NOT NULL DEFAULT '',
  grace_until TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_keys_owner_status ON keys(owner_type, owner_id, status);

CREATE TABLE IF NOT EXISTS contacts (
  contact_id TEXT PRIMARY KEY,
  canonical_id TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL DEFAULT '',
  home_origin TEXT NOT NULL DEFAULT '',
  profile_url TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'discovered',
  last_seen_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS artifact_snapshots (
  snapshot_id TEXT PRIMARY KEY,
  contact_id TEXT NOT NULL,
  artifact_type TEXT NOT NULL,
  source_url TEXT NOT NULL,
  fetched_at TEXT NOT NULL,
  http_status INTEGER NOT NULL DEFAULT 0,
  content_hash TEXT NOT NULL DEFAULT '',
  parsed_summary TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS proofs (
  proof_id TEXT PRIMARY KEY,
  contact_id TEXT NOT NULL,
  proof_type TEXT NOT NULL,
  proof_url TEXT NOT NULL,
  observed_value TEXT NOT NULL DEFAULT '',
  verified_status TEXT NOT NULL DEFAULT 'unknown',
  verified_at TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS pinned_materials (
  pin_id TEXT PRIMARY KEY,
  contact_id TEXT NOT NULL,
  material_type TEXT NOT NULL,
  material_value TEXT NOT NULL,
  reason TEXT NOT NULL DEFAULT '',
  pinned_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS policy_hints (
  policy_id TEXT PRIMARY KEY,
  owner_type TEXT NOT NULL,
  owner_id TEXT NOT NULL,
  adapter_type TEXT NOT NULL,
  action TEXT NOT NULL,
  effect TEXT NOT NULL,
  conditions_json TEXT NOT NULL DEFAULT '{}',
  updated_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);
