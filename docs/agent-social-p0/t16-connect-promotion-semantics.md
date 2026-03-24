# T16 Connect Promotion Semantics (P0)

This document defines the canonical post-connect local promotion rules for `message connect-peer`.

## Scope

- Applies to `message connect-peer` (CLI, runtime result envelope, plugin bridge consumers).
- Focuses on local state promotion after connect evaluation.
- Keeps trust escalation and manual annotation flows explicit.

## Promotion Rules

### 1) Contact promotion

- Resolve peer by canonical id from contact or discovery.
- Ensure one local `contacts` row exists for the canonical id.
- If missing, create a minimal contact (display name fallback to canonical id).
- If present, only backfill missing fields and refresh `last_seen_at`:
  - `recipient_id` (from peer presence when available)
  - `relay_url` (from store-forward hints/routes when available)

### 2) Trust promotion

- Ensure `trust_records` exists and is bound to the promoted contact.
- Ensure `runtime_trust_records.contact_id` is bound to the promoted contact.
- Preserve existing trust intent:
  - Do not auto-raise `trust_level`.
  - Do not clear or overwrite existing risk flags.
- Verification/source fields may be conservatively backfilled.

### 3) Discovery promotion

- Upsert `runtime_discovery_records` from the connect-time presence/routes snapshot.
- Normalize discovery source using the canonical discovery source policy.

### 4) Audit event

- Insert `interaction_events` with `event_type=connect`.
- Record connect outcome summary (`connected`, `transport`, `reason`, `source`).

## Explicit Non-Writes

`message connect-peer` must not auto-write:

- `notes`
- `pinned_materials`

Those remain explicit user actions (`known note`, `known trust --level pinned`, etc.).

## Connect Result Contract

`ConnectPeerResult` includes a `promotion` object to make local effects explicit:

- contact id/status + whether contact was newly created
- trust linkage + trust summary fields
- discovery update/source
- explicit `note_written=false` and `pin_written=false`
- event id

## Evidence

Regression coverage is provided by:

- `internal/message/service_test.go`
- `internal/cli/run_test.go`
- `openclaw-plugin/test/commands.test.ts`
- `internal/runtime/service_test.go`
