# Agent-Social Runtime API And Roadmap

## Status

This document is the surviving historical summary for the communication-layer refactor. It defines:

- the runtime API contract
- CLI/plugin-to-runtime mapping
- staged development TODO lists

The earlier design-set documents have been retired after implementation landed; this file remains as the compact record of the final runtime/API direction and milestone status.

## Runtime API Contract

The runtime should expose a small, stable internal contract.

The contract must be:

- transport-agnostic
- plugin-friendly
- CLI-friendly
- stable even while direct/store-and-forward implementations evolve

## Core Runtime Operations

### 1. Onboarding

Purpose:

- ensure local identity exists
- ensure runtime storage exists
- ensure direct/store-and-forward capabilities are initialized enough for use

Suggested request shape:

- `display_name`
- `force_refresh` (optional)

Suggested result shape:

- `self_id`
- `display_name`
- `peer_id`
- `transport_capabilities`
- `status_summary`

### 2. Status

Purpose:

- report whether the local runtime is healthy and usable

Suggested result shape:

- `identity_ready`
- `transport_ready`
- `discovery_ready`
- `message_queue_summary`
- `unread_count`
- `last_sync_at`
- `presence_summary`

### 3. ExportCard

Purpose:

- export a peer-shareable identity surface

Suggested result shape:

- `card`
- `self_id`
- `transport_capabilities`

### 4. ImportContact

Purpose:

- add or update a contact from an identity card or future identity surface

Suggested request shape:

- `card_json`

Suggested result shape:

- `contact_id`
- `canonical_id`
- `display_name`
- `transport_capabilities`
- `updated`

### 5. SendMessage

Purpose:

- create and route an outgoing message

Suggested request shape:

- `contact_ref`
- `plaintext`

Suggested result shape:

- `conversation_id`
- `message_id`
- `status`
- `selected_route`

### 6. Sync

Purpose:

- recover pending deliveries and update local read models

Suggested request shape:

- optional `contact_ref`

Suggested result shape:

- `synced_count`
- `updated_conversations`
- `routes_used`

### 7. ListInbox

Purpose:

- return conversation summaries for product/UI surfaces

Suggested result shape:

- `conversations[]`
  - `conversation_id`
  - `contact_ref`
  - `display_name`
  - `last_message_preview`
  - `last_message_at`
  - `unread_count`

### 8. GetThread

Purpose:

- return the local thread view for a contact or conversation

Suggested request shape:

- `contact_ref` or `conversation_id`

Suggested result shape:

- `conversation`
- `messages[]`
  - `message_id`
  - `direction`
  - `plaintext_preview`
  - `created_at`
  - `status`

## Internal Service Boundaries

These operations should be implemented by composing smaller services.

### Runtime Service

Responsible for:

- public runtime contract
- orchestration
- transaction boundaries

### Identity Service

Responsible for:

- local identity creation
- self identity loading
- card export inputs

### Contact Service

Responsible for:

- contact persistence
- contact resolution
- card import handling

### Routing Service

Responsible for:

- route planning
- route scoring
- route outcome recording

### Discovery Service

Responsible for:

- reachability lookup
- presence resolution
- future announce / DHT integration

### Transport Service

Responsible for:

- sending via a specific adapter
- syncing via a specific adapter
- returning normalized transport outcomes

### Read Model Service

Responsible for:

- inbox summaries
- thread views
- unread accounting

## CLI Mapping

The current CLI should eventually map cleanly onto the runtime contract.

Suggested mapping:

- `linkclaw init`
  - maps to `Onboarding`
- `linkclaw status` or future equivalent
  - maps to `Status`
- `linkclaw card export`
  - maps to `ExportCard`
- `linkclaw card import`
  - maps to `ImportContact`
- `linkclaw message send`
  - maps to `SendMessage`
- `linkclaw message sync`
  - maps to `Sync`
- `linkclaw message inbox`
  - maps to `ListInbox`
- `linkclaw message thread`
  - maps to `GetThread`

The CLI should become a thin shell over runtime operations rather than owning communication logic itself.

## OpenClaw Plugin Mapping

The plugin should map its current user-facing flows onto the same runtime contract.

Suggested mapping:

- `linkclaw_onboarding`
  - `Onboarding`
- `linkclaw_status`
  - `Status`
- `linkclaw_share_card`
  - `ExportCard`
- `linkclaw_connect_card`
  - `ImportContact`
- `linkclaw_send_message`
  - `SendMessage`
- `linkclaw_sync_inbox`
  - `Sync` + `ListInbox`

Slash commands and natural-language flows should keep using these high-level actions and stay isolated from transport details.

## Implementation Principles

1. The runtime contract should be designed first.
2. Transports should be replaceable without changing CLI or plugin surfaces.
3. Discovery must remain a separate concern from transport.
4. Store-and-forward should be a transport/runtime behavior, not the system identity.
5. libp2p integration must be hidden behind runtime and discovery interfaces.

## Development TODO List

The roadmap below is staged from architecture-first to user-facing validation.

### Phase 0: Freeze The Direction

Goal:

- stop further investment in relay-centric architecture

TODO:

- [ ] mark current HTTP relay flow as V0/deprecated in design docs
- [ ] confirm runtime-first, libp2p-first, plugin-control-plane direction across docs
- [ ] decide initial package/module names for runtime/discovery/transport

### Phase 1: Runtime Skeleton

Goal:

- create the structural center of the new communication layer

Current status:

- complete
- runtime/routing/discovery/transport packages have been introduced as the initial skeleton
- basic runtime tests exist for send/sync orchestration with stub transports

TODO:

- [x] add `internal/runtime`
- [x] add `internal/routing`
- [x] add `internal/discovery`
- [x] add `internal/transport`
- [x] define `MessagingRuntime`, `RoutePlanner`, `DiscoveryService`, `Transport`
- [x] add basic runtime tests with stub transports

### Phase 2: Clean-Slate Storage

Goal:

- replace V0 relay-shaped communication persistence

Current status:

- complete
- clean-slate runtime tables have been introduced
- a first runtime store implementation now persists self identity, contacts, conversations, messages, route attempts, and presence cache independently of the old relay-shaped tables
- runtime persistence no longer depends on the previous relay-shaped schema for new storage modeling

TODO:

- [x] design new runtime schema for self identity/contact/conversation/message/route attempts
- [x] implement fresh migrations for the new communication runtime
- [x] isolate old relay-shaped message tables by keeping new runtime persistence in dedicated clean-slate tables
- [x] add persistence helpers for route outcomes and presence cache
- [x] add tests for new storage reads/writes

### Phase 3: Route-Driven Message Flow

Goal:

- make communication runtime-driven instead of relay-driven

Current status:

- in progress
- `message send` and `message sync` now route through `MessagingRuntime`
- legacy store-and-forward behavior has been wrapped behind a runtime transport callback
- the old full-table runtime snapshot bridge has been removed; send and recovery paths now perform targeted runtime upserts instead of full-table mirroring
- inbox and thread reads now load directly from runtime read models backed by `runtime_conversations` and `runtime_messages`
- outbox reads now load from runtime message state instead of the legacy message table
- send no longer relies on a full runtime snapshot; it now performs targeted runtime upserts for the affected self/contact/conversation/message records
- sync no longer relies on a full runtime snapshot after recovery; recovered contacts/conversations/messages are written into runtime state directly
- read-mark flows still update both runtime and legacy unread state during the transition period

TODO:

- [x] route `message send` through `MessagingRuntime`
- [x] route `message sync` through `MessagingRuntime`
- [x] route inbox/thread reads through runtime/read-model services
- [x] record route attempts and selected route on every send/recover
- [x] make current store-and-forward behavior a transport adapter

### Phase 4: libp2p Boundary

Goal:

- establish the direct transport/discovery integration surface

Current status:

- in progress
- peer identity mapping from local self identity to a stable libp2p-facing identity boundary now exists
- `internal/transport/libp2p` and `internal/discovery/libp2p` have been introduced as boundary packages
- direct route candidates can now be produced from discovery hints without exposing transport details to higher layers
- a minimal libp2p session boundary now exists behind an experimental direct-transport feature flag
- direct-first send paths can now fall back to store-and-forward without changing user-facing message flows
- the current implementation is still a boundary skeleton; it does not yet boot a real libp2p node or session manager

TODO:

- [x] define peer identity mapping from local self identity to libp2p identity
- [x] add `internal/transport/libp2p`
- [x] add `internal/discovery/libp2p`
- [x] implement runtime boot of a minimal libp2p node/session boundary
- [x] add direct route candidate generation
- [x] feature-flag direct transport until basic validation passes

### Phase 5: Store-And-Forward Under New Runtime

Goal:

- preserve offline delivery without centering the design on relay

Current status:

- in progress
- store-and-forward recovery now persists its cursor/state under runtime-owned storage instead of relying only on the old message sync cursor path
- runtime `Sync` now performs adapter `Ack` calls when transports advance a recovery cursor
- direct-first send paths continue to fall back to store-and-forward without changing user-facing flows
- the remaining work is to finish removing legacy cursor ownership from the old message tables and harden retry semantics around route outcomes

TODO:

- [x] define store-and-forward transport interface
- [x] move mailbox sync/recovery logic under runtime orchestration
- [x] normalize cursor/ack/retry handling
- [x] ensure runtime falls back cleanly when direct delivery fails
- [x] add tests for direct-fail -> store-and-forward fallback

### Phase 6: Plugin And CLI Rebinding

Goal:

- keep user-facing flows stable while internals change

Current status:

- complete
- runtime-backed `message status` now exists in core service/CLI
- OpenClaw `/linkclaw-status` now consumes runtime-backed status instead of reconstructing state from `known ls` and `message inbox`
- plugin status copy now prefers product language such as messaging/direct transport instead of exposing raw store-forward counters directly
- onboarding/share/connect/message/inbox user flows continue to pass through the rebinding layer

TODO:

- [x] rebind CLI commands to runtime contract
- [x] rebind plugin high-level tools to runtime contract
- [x] preserve onboarding/share/connect/message/inbox user flows
- [x] add status surface for runtime health, presence, and recovery state
- [x] remove transport-specific leakage from plugin responses

### Phase 7: End-To-End Validation

Goal:

- prove the new runtime works in the real OpenClaw flow

Current status:

- complete
- onboarding on clean homes is covered by runtime/plugin tests
- two-host card exchange and message flow are covered by plugin command tests
- direct delivery now has a real cross-host plugin HTTP endpoint path guarded by `directUrl` / `directToken`

TODO:

- [x] validate onboarding on a clean host
- [x] validate card exchange on two hosts
- [x] validate direct delivery when both hosts are online
- [x] validate plugin natural-language flows still work

### Phase 8: Optional Future Work

Goal:

- preserve room for later product evolution

Current status:

- in progress
- minimal `peer announce` support now exists in the libp2p discovery boundary through `PublishSelf`
- runtime presence cache now stores richer presence state, including reachability, transport capabilities, store-forward hints, and last announce time
- a minimal DHT discovery boundary now exists as a separate adapter package
- a minimal Nostr transport/discovery adapter boundary now exists as a separate package pair
- runtime now exposes an optional background mode flag while still defaulting to host-managed execution
- runtime now exposes delivery/recovery plus future reputation/payment/penalty hook interfaces

TODO:

- [x] add peer announce support
- [x] add richer presence cache model
- [x] evaluate DHT-backed discovery rollout
- [x] evaluate optional Nostr adapter
- [x] evaluate optional advanced background-runtime mode
- [x] add hooks for future reputation/payment/penalty layers

### Phase 9: Remove Legacy HTTP Transport

Goal:

- replace the standalone legacy transport entrypoint with runtime-owned store-and-forward transport and remove relay-centric product paths

Current status:

- complete
- the standalone legacy transport entrypoint has been removed
- plugin schema, bridge defaults, and end-user docs no longer expose the legacy relay config key
- legacy OpenClaw relay tests and relay-specific status copy have been removed from the current product surface
- remaining transport code under `internal/transport/storeforward` is now internal cleanup debt rather than an exposed compatibility path

TODO:

- [x] introduce a runtime-owned `storeforward` transport package
- [x] move `message send/sync/ack` relay operations behind a transport/backend contract instead of direct `relayclient` ownership in `message.Service`
- [x] remove the legacy relay config key from plugin/docs/default config
- [x] remove `internal/relayclient`
- [x] remove the standalone legacy transport entrypoint
- [x] remove legacy OpenClaw relay tests

## Recommended Immediate Build Sequence

If implementation starts now, the recommended sequence is:

1. Phase 1
2. Phase 2
3. Phase 3
4. first half of Phase 4
5. Phase 6 rebinding
6. Phase 7 validation

That order gets the architecture right before deep libp2p work.

## Definition Of Done For The First Major Milestone

The first major milestone should be considered complete when:

- the runtime contract exists
- communication persistence is no longer relay-shaped
- send/sync/inbox/thread all route through runtime orchestration
- relay-specific behavior is demoted to a transport adapter
- plugin and CLI keep working without transport-specific UX leakage

At that point, the project will have completed the architectural break from the old V0 communication model.
