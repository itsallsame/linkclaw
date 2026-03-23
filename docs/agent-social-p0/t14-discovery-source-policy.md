# T14 Discovery Source Policy (P0)

This document defines the canonical discovery source semantics used by runtime discovery query, filtering, ranking, CLI output, and plugin output.

## Canonical Enum

- `refresh`
- `import`
- `libp2p-announce`
- `libp2p`
- `dht-announce`
- `dht`
- `nostr`
- `manual`
- `cache`
- `unknown`

## Ranking (High to Low)

1. `refresh` (100)
2. `import` (90)
3. `libp2p-announce` (85)
4. `libp2p` (80)
5. `dht-announce` (75)
6. `dht` (70)
7. `nostr` (60)
8. `manual` (55)
9. `cache` (40)
10. `unknown` (10)

## Legacy Compatibility Mapping

- `refresh-peer`, `known-refresh` -> `refresh`
- `known-import` -> `import`
- `stale-cache`, `runtime-send`, `runtime-cache` -> `cache`
- empty string, `none`, and unrecognized values -> `unknown`

## Filter Semantics

- `message list-discovery --source <value>` accepts canonical enum values and the legacy aliases above.
- Outputs always use canonical enum values after normalization.
