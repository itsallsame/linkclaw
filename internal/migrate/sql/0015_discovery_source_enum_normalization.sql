UPDATE runtime_discovery_records
SET source = LOWER(TRIM(source));

UPDATE runtime_discovery_records
SET source = 'unknown'
WHERE source = '' OR source = 'none';

UPDATE runtime_discovery_records
SET source = 'refresh'
WHERE source IN ('refresh-peer', 'known-refresh');

UPDATE runtime_discovery_records
SET source = 'import'
WHERE source = 'known-import';

UPDATE runtime_discovery_records
SET source = 'cache'
WHERE source IN ('stale-cache', 'runtime-send', 'runtime-cache');

UPDATE runtime_discovery_records
SET source = 'unknown'
WHERE source NOT IN (
  'refresh',
  'import',
  'libp2p-announce',
  'libp2p',
  'dht-announce',
  'dht',
  'nostr',
  'manual',
  'cache',
  'unknown'
);

UPDATE runtime_presence_cache
SET source = LOWER(TRIM(source));

UPDATE runtime_presence_cache
SET source = 'unknown'
WHERE source = '' OR source = 'none';

UPDATE runtime_presence_cache
SET source = 'refresh'
WHERE source IN ('refresh-peer', 'known-refresh');

UPDATE runtime_presence_cache
SET source = 'import'
WHERE source = 'known-import';

UPDATE runtime_presence_cache
SET source = 'cache'
WHERE source IN ('stale-cache', 'runtime-send', 'runtime-cache');

UPDATE runtime_presence_cache
SET source = 'unknown'
WHERE source NOT IN (
  'refresh',
  'import',
  'libp2p-announce',
  'libp2p',
  'dht-announce',
  'dht',
  'nostr',
  'manual',
  'cache',
  'unknown'
);
