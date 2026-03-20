# Agent-Social Runtime Implementation Plan

## Status

Engineering plan for implementing the runtime architecture described in [agent-social-runtime-design.md](/home/ubuntu/fork_lk/linkclaw/docs/agent-social-runtime-design.md).

This document is code-facing and focused on module boundaries, interfaces, and migration order.

## Current Code Baseline

The current communication-related core is concentrated in:

- [internal/message/service.go](/home/ubuntu/fork_lk/linkclaw/internal/message/service.go)
- [internal/card/service.go](/home/ubuntu/fork_lk/linkclaw/internal/card/service.go)
- [internal/importer/service.go](/home/ubuntu/fork_lk/linkclaw/internal/importer/service.go)
- [internal/relayserver/server.go](/home/ubuntu/fork_lk/linkclaw/internal/relayserver/server.go)
- [internal/relayclient](/home/ubuntu/fork_lk/linkclaw/internal/relayclient)
- [internal/messagingprofile](/home/ubuntu/fork_lk/linkclaw/internal/messagingprofile)

The current shape is still V0-oriented:

- contacts persist a single messaging route well enough for relay-backed DM
- send/sync behavior is tightly coupled to the relay-backed model
- identity cards expose a single `relay_url` route
- there is no transport/runtime abstraction for direct connectivity or discovery

## Refactor Objectives

The implementation refactor should achieve these concrete outcomes:

1. message flows no longer depend on a single relay client path
2. transport choice is runtime-driven
3. discovery becomes an explicit dependency
4. HTTP relay code becomes an adapter rather than the communication core
5. libp2p can be integrated without rewriting CLI/plugin entry points later

## Proposed New Internal Modules

The runtime refactor should introduce these new modules under `internal/`.

### `internal/runtime`

Owns the high-level communication orchestration.

Responsibilities:

- expose the runtime-level send/sync/recover/status API
- coordinate routing, discovery, storage, and transport
- remain transport-agnostic

Suggested files:

- `internal/runtime/service.go`
- `internal/runtime/types.go`
- `internal/runtime/service_test.go`

### `internal/routing`

Owns route planning and route selection.

Responsibilities:

- compute candidate routes for a contact
- prioritize direct routes over store-and-forward routes
- persist route outcomes for future planning

Suggested files:

- `internal/routing/planner.go`
- `internal/routing/types.go`
- `internal/routing/planner_test.go`

### `internal/discovery`

Owns peer reachability and presence lookups.

Responsibilities:

- resolve currently known reachability data for a contact
- manage future peer announce / signed peer record support
- integrate future DHT-backed discovery

Suggested files:

- `internal/discovery/service.go`
- `internal/discovery/types.go`
- `internal/discovery/service_test.go`

### `internal/transport`

Owns transport abstraction and concrete implementations.

Responsibilities:

- define transport interface
- host concrete direct/store-and-forward adapters
- keep adapter-specific logic out of runtime orchestration

Suggested layout:

- `internal/transport/types.go`
- `internal/transport/direct/`
- `internal/transport/storeforward/`
- `internal/transport/libp2p/`
- `internal/transport/nostr/` (future)

### `internal/storeforward`

Optional explicit module if store-and-forward grows beyond a simple transport adapter.

Responsibilities:

- recipient mailbox logic
- sync cursor management
- ack handling
- retry/recovery state

If kept small, this can remain inside `internal/transport/storeforward`.

## Existing Module Migration

### `internal/message`

Current state:

- owns local message logic
- still carries relay-shaped assumptions

Target state:

- becomes local message store + conversation domain logic
- no direct transport selection
- no direct relay-specific branching

Keep in `internal/message`:

- conversation/message persistence
- thread/inbox/outbox queries
- plaintext/ciphertext persistence rules
- read-model updates

Move out of `internal/message`:

- route selection
- transport invocation
- recovery policy

### `internal/card`

Current state:

- exports identity cards with V0 messaging route info

Target state:

- exports transport metadata compatible with runtime planning
- stops assuming a single route is sufficient as the final model

Near-term compatibility plan:

- keep current `relay_url` support for V0 compatibility
- internally prepare to add:
  - `transport_capabilities`
  - `direct_hints`
  - `store_forward_hints`

### `internal/importer`

Current state:

- imports contacts from identity cards
- persists the minimal route data needed by V0 relay messaging

Target state:

- imports richer runtime contact metadata
- persists future routing/discovery hints
- remains backward-compatible with existing identity-card v1

### `internal/relayserver`

Current state:

- treated as the V0 delivery center

Target state:

- no longer treated as the target architecture
- either:
  - becomes a dev/test store-and-forward adapter
  - or is gradually replaced by a new store-and-forward adapter/service

Important rule:

- new communication orchestration must not depend directly on `internal/relayserver`

## Runtime Interfaces

These interfaces should be defined before major implementation work.

### `MessagingRuntime`

Suggested methods:

- `Send(ctx, req SendRequest) (SendResult, error)`
- `Sync(ctx, req SyncRequest) (SyncResult, error)`
- `Recover(ctx, req RecoverRequest) (RecoverResult, error)`
- `Status(ctx) (RuntimeStatus, error)`
- `Acknowledge(ctx, req AckRequest) error`

### `RoutePlanner`

Suggested methods:

- `PlanSend(ctx, contact ContactRuntimeView) ([]RouteCandidate, error)`
- `PlanRecover(ctx, contact ContactRuntimeView) ([]RouteCandidate, error)`
- `RecordOutcome(ctx, outcome RouteOutcome) error`

### `DiscoveryService`

Suggested methods:

- `ResolvePeer(ctx, canonicalID string) (PeerPresenceView, error)`
- `RefreshPeer(ctx, canonicalID string) (PeerPresenceView, error)`
- `PublishSelf(ctx) error` (future-facing)

### `Transport`

Suggested methods:

- `Name() string`
- `Supports(route RouteCandidate) bool`
- `Send(ctx, env Envelope, route RouteCandidate) (TransportSendResult, error)`
- `Sync(ctx, route RouteCandidate) (TransportSyncResult, error)`
- `Ack(ctx, route RouteCandidate, cursor string) error`

## New Runtime Data Shapes

The runtime should introduce internal views that are not tied 1:1 to CLI JSON shapes.

### `ContactRuntimeView`

Suggested contents:

- canonical identity
- signing/encryption keys
- transport capabilities
- direct hints
- store-and-forward hints
- last successful route
- local trust state

### `PeerPresenceView`

Suggested contents:

- currently known direct reachability hints
- freshness timestamp
- optional signed peer record
- optional future DHT source metadata

### `RouteCandidate`

Suggested contents:

- route type
- route priority
- destination metadata
- freshness/source metadata

### `RouteOutcome`

Suggested contents:

- route attempted
- success/failure
- latency/error class
- retryability

## Schema Migration Direction

This refactor should not try to replace all V0 schema at once.

Instead:

1. keep current message/contact tables operational
2. add new columns or side tables for routing/runtime metadata
3. move read/write logic gradually to runtime views
4. deprecate relay-shaped assumptions after runtime adoption

Suggested additions:

- contact transport capability table or JSON field
- contact direct/store-forward hints field
- route attempt history table
- route outcome / last successful path fields
- optional peer presence cache table

## CLI And Plugin Mapping

The current CLI and OpenClaw plugin surfaces should remain stable.

That means:

- `linkclaw message send`
- `linkclaw message sync`
- `linkclaw message inbox`
- `linkclaw card export`
- OpenClaw onboarding/share/connect/message/inbox flows

should call into the new runtime rather than directly into relay-shaped code.

This preserves product behavior while allowing the transport layer to change underneath.

## Phased Implementation Order

### Phase 1: Interface Extraction

- add runtime, routing, discovery, and transport interfaces
- keep current relay-backed implementation behind a transport adapter
- rewire message send/sync through `MessagingRuntime`

### Phase 2: Schema Expansion

- add routing/runtime metadata persistence
- add peer presence cache
- add route attempt history

### Phase 3: Direct Transport Skeleton

- add `libp2p` integration boundary
- introduce peer/direct route candidates
- keep actual behavior feature-flagged if needed

### Phase 4: Store-And-Forward Under Runtime

- move mailbox/recovery behavior under runtime control
- remove direct dependence on old relay-centric orchestration

### Phase 5: Discovery Expansion

- add richer peer presence
- add announce/refresh hooks
- add future DHT-backed lookup support

## First Engineering Milestone

The first milestone for code work should be:

- `MessagingRuntime` exists
- current send/sync code is routed through it
- route planning is separated from message persistence
- relay-backed delivery is demoted to a transport adapter

If that milestone is complete, the project will no longer be structurally blocked by the V0 relay-centered design.
