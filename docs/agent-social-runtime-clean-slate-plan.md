# Agent-Social Runtime Clean-Slate Plan

## Status

This document assumes there are no production users and no migration compatibility requirement for the current communication database.

Under this assumption:

- old communication schema may be deleted
- relay-shaped persistence may be replaced directly
- the new runtime may define its own clean storage model

## Design Rule

The communication refactor should optimize for correct long-term architecture, not backward compatibility with V0 messaging storage.

That means:

- do not design around the current single-route relay model
- do not carry forward schema decisions made only for HTTP relay MVP
- keep only user-facing command continuity where useful

## Clean-Slate Storage Model

The runtime storage should be rebuilt around five core entities.

## 1. Self Identity

Represents the local runtime identity and transport posture.

Suggested fields:

- `self_id`
- `display_name`
- `signing_public_key`
- `encryption_public_key`
- `signing_private_key_ref`
- `encryption_private_key_ref`
- `peer_id`
- `transport_capabilities`
- `created_at`
- `updated_at`

Purpose:

- stable local identity root
- direct transport identity
- future announce/discovery anchor

## 2. Contact

Represents a known peer in the local runtime.

Suggested fields:

- `contact_id`
- `canonical_id`
- `display_name`
- `signing_public_key`
- `encryption_public_key`
- `peer_id`
- `trust_state`
- `transport_capabilities`
- `direct_hints`
- `store_forward_hints`
- `signed_peer_record`
- `last_seen_at`
- `last_successful_route`
- `raw_identity_card_json`
- `created_at`
- `updated_at`

Purpose:

- peer identity
- route planning seed data
- trust entry point

## 3. Conversation

Represents the local thread view between self and a contact.

Suggested fields:

- `conversation_id`
- `contact_id`
- `last_message_id`
- `last_message_preview`
- `last_message_at`
- `unread_count`
- `created_at`
- `updated_at`

Purpose:

- local inbox/thread read model
- independent from transport implementation

## 4. Message

Represents the canonical local message object.

Suggested fields:

- `message_id`
- `conversation_id`
- `sender_id`
- `recipient_id`
- `direction`
- `plaintext_preview`
- `ciphertext`
- `ciphertext_version`
- `status`
- `selected_route`
- `created_at`
- `delivered_at`
- `acked_at`

Purpose:

- one canonical message record
- transport-agnostic persistence

## 5. Route Attempt

Represents delivery and recovery attempts made by the runtime.

Suggested fields:

- `attempt_id`
- `message_id`
- `conversation_id`
- `route_type`
- `route_label`
- `priority`
- `outcome`
- `error`
- `retryable`
- `cursor`
- `attempted_at`

Purpose:

- routing observability
- retry and recovery bookkeeping
- future route learning

## Optional 6. Presence Cache

Represents discovery outputs cached locally.

Suggested fields:

- `canonical_id`
- `peer_id`
- `direct_hints`
- `signed_peer_record`
- `source`
- `fresh_until`
- `resolved_at`

Purpose:

- decouple discovery lookups from contact records
- support short-lived reachability data

## Runtime Status Model

The runtime should expose a first-class status model, not just low-level table state.

Suggested runtime status sections:

- identity readiness
- transport readiness
- discovery readiness
- message queue summary
- unread summary
- direct reachability summary

This is important because the plugin and future status surfaces should describe:

- whether the runtime is usable
- whether direct connectivity is available
- whether recovery is pending

without exposing storage internals.

## Route Types

The runtime should define stable route types from day one.

Suggested route types:

- `direct`
- `store_forward`
- `recovery`
- `nostr` (reserved)

This avoids re-centering the design around any one adapter.

## libp2p Integration Boundary

libp2p should enter the system through dedicated adapter modules, not through the message domain.

Suggested boundary:

- `internal/transport/libp2p`
- `internal/discovery/libp2p`

### `internal/transport/libp2p`

Owns:

- direct send channel
- peer connection setup
- direct delivery results

Does not own:

- contact persistence
- inbox logic
- trust logic

### `internal/discovery/libp2p`

Owns:

- peer reachability lookup
- future DHT integration
- future peer announce support

Does not own:

- route policy
- conversation state

## libp2p First Milestone

The first libp2p milestone should stay small.

It should define and prove:

1. local peer identity exists
2. runtime can construct a libp2p node/session boundary
3. contact records can reference a peer-oriented route model
4. route planner can prefer `direct` over `store_forward`

It does not need to solve:

- full DHT rollout
- rich NAT traversal
- background always-on behavior
- Nostr integration

## Plugin Contract After Refactor

The OpenClaw plugin should not need to understand libp2p details.

Its role remains:

- onboarding
- status
- share/connect
- send/reply/thread/inbox

The plugin should speak only to runtime operations such as:

- `onboarding`
- `status`
- `export_card`
- `import_contact`
- `send_message`
- `sync`
- `thread`

This keeps the plugin stable while transport internals change.

## Deletion Strategy For Existing V0 Communication State

Because there are no users, the implementation may take a hard reset approach:

1. remove old communication-specific tables that encode relay-only assumptions
2. replace them with the clean-slate runtime schema
3. treat V0 local state as disposable during this refactor

This should be stated explicitly in implementation work so engineers do not waste time on compatibility shims.

## First Code-Facing Deliverable

The first code deliverable under the clean-slate plan should be:

- a new runtime storage schema
- runtime interfaces checked into `internal/runtime`, `internal/routing`, `internal/discovery`, and `internal/transport`
- current message CLI/plugin actions routed through the new runtime entry point

At that point, the project will have structurally left the V0 relay-centric design behind.
