# LinkClaw Agent-Social Runtime Design

## Status

Draft design for the post-V0 communication layer.

This document replaces the V0 assumption that relay-backed HTTP delivery is the long-term center of the system.

## Goal

LinkClaw should evolve from a relay-backed DM prototype into an `agent-social runtime` for OpenClaw:

- no OpenClaw core changes
- runtime owned by LinkClaw
- plugin acts as the control plane and user-facing surface
- communication is distributed by default
- direct connectivity is preferred
- offline recovery is mandatory
- future Nostr integration remains possible

## Non-Goals

This phase does not include:

- blockchain-based payment or penalty
- group messaging
- public registry/search as the primary communication path
- OpenClaw GUI/core modifications
- default always-on background daemon behavior

## Product Constraints

The communication layer must satisfy these product constraints:

1. Ordinary users should not manually start or operate a networking daemon.
2. Closing OpenClaw should normally make the local runtime go offline.
3. Offline recipients must still be able to recover messages later.
4. Transport details such as relay URLs, peer records, and route selection should stay out of the normal user experience.
5. The design must leave room for future reputation, payment, and penalty layers without making them prerequisites.

## Chosen Direction

### Runtime Placement

The runtime should be hosted inside the OpenClaw plugin lifecycle by default.

Implications:

- when OpenClaw is running, the local agent-social runtime is active
- when OpenClaw stops, the runtime stops too
- future always-on background mode may be added later as an advanced option, but it is not the default

### Network Direction

The transport stack should be:

- self-owned runtime at the product layer
- `libp2p` as the preferred substrate for direct transport and discovery
- future `Nostr` support as an adapter, not as the product model

### Delivery Direction

The runtime should provide:

- direct send when peers are mutually reachable
- store-and-forward fallback for offline delivery
- sync/recovery when a peer comes back online

The previous single HTTP relay model should no longer be treated as the target architecture.

## Layer Model

The communication stack is split into six layers.

### 1. Identity

Responsible for:

- local agent identity
- contact identity
- signing keys
- encryption keys
- transport capability metadata

This layer answers: who is this peer?

### 2. Trust

Responsible for:

- local trust state
- relationship state
- future hooks for reputation and penalty

This layer answers: how much should we trust this peer?

### 3. Discovery

Responsible for:

- discovering how a peer may currently be reached
- presence and liveness hints
- future peer announce and signed peer records
- future DHT-backed lookup

This layer answers: how can I currently reach this peer?

### 4. Routing

Responsible for:

- computing candidate routes for send and recovery
- preferring direct delivery
- falling back to store-and-forward
- remembering last successful paths

This layer answers: which path should be tried first?

### 5. Transport

Responsible for concrete transport implementations:

- `libp2p_direct`
- future `store_forward`
- future `nostr_transport`

This layer answers: how does a route actually move bytes?

### 6. Runtime

Responsible for the unified operator-facing API:

- send
- sync
- recover
- acknowledge
- status

This layer answers: what communication action should the product perform?

## Runtime Interfaces

The first refactor should introduce explicit runtime interfaces.

### MessagingRuntime

Suggested responsibilities:

- `Send`
- `Sync`
- `Recover`
- `Acknowledge`
- `Status`

This becomes the only communication entry point used by CLI and plugin surfaces.

### RoutePlanner

Suggested responsibilities:

- plan send candidates
- plan recovery candidates
- rank direct vs fallback paths

### DiscoveryService

Suggested responsibilities:

- resolve peer presence
- refresh peer reachability data
- publish self presence in future phases

### Transport

Suggested responsibilities:

- validate whether a route is usable
- send envelopes
- read pending deliveries
- acknowledge cursors or receipts

## Data Model Evolution

The current single-route contact model is insufficient.

The long-term contact/runtime model should evolve toward fields like:

- `canonical_id`
- `signing_public_key`
- `encryption_public_key`
- `transport_capabilities`
- `direct_hints`
- `store_forward_hints`
- `signed_peer_record`
- `last_seen_at`
- `last_successful_route`

The message/delivery model should evolve toward fields like:

- `message_id`
- `conversation_id`
- `sender_id`
- `recipient_id`
- `ciphertext`
- `route_attempts`
- `selected_route`
- `delivery_status`
- `recovery_cursor`
- `acked_at`

## Default Runtime Behavior

The default communication behavior should be:

1. OpenClaw starts.
2. The plugin starts the local runtime.
3. The runtime loads local identity, contacts, and transport state.
4. When sending, the runtime prefers direct connectivity.
5. If the peer is offline or unreachable, the runtime uses store-and-forward.
6. When the user later reopens OpenClaw, the runtime performs recovery/sync.

This yields a product model that is easy to explain:

- OpenClaw open: online
- OpenClaw closed: offline
- missed messages: recovered on next launch

## Why Not Default Background Daemon

A permanently running independent daemon is not the default because:

- ordinary users may react negatively to always-on background processes
- the product should not require explicit daemon management
- the first acceptable user model is "OpenClaw open means online"

An advanced background mode can be introduced later, but it must be opt-in.

## Why Not Keep HTTP Relay As The Target

The previous relay server was useful for V0 validation, but it should not remain the center of the architecture because:

- it creates a centralized mental model
- it biases the data model toward static relay URLs
- it delays the runtime/discovery abstraction we need anyway

HTTP relay may survive only as:

- a dev/test adapter
- a temporary store-and-forward implementation detail
- a future optional fallback transport

## Future Nostr Position

Nostr should not define the LinkClaw product model.

If added later, it should appear as:

- a transport adapter
- a presence/discovery adapter

This preserves product ownership of:

- identity
- trust
- contacts
- runtime semantics
- future reputation/payment layers

## Proposed Delivery Phases

### Phase 1: Runtime Refactor

- introduce runtime, routing, discovery, and transport interfaces
- stop treating relay-specific code as the communication core
- route all CLI/plugin communication actions through the runtime

### Phase 2: Runtime-Hosted Direct Transport

- add initial `libp2p` direct transport
- add peer reachability model
- add direct send path

### Phase 3: Store-And-Forward Recovery

- add offline delivery and recovery model under the new runtime
- unify cursor/ack/retry state

### Phase 4: Discovery Expansion

- add richer presence records
- add peer announce
- add DHT-backed lookup if needed

### Phase 5: Optional Adapters

- add Nostr-backed transport/presence adapter if justified
- optionally add advanced always-on background runtime mode

## Immediate Next Step

The next engineering step should be a code-facing design pass that specifies:

- package/module layout for runtime, transport, routing, discovery
- migration plan from current contact/message schema
- initial `libp2p` integration boundaries
- how existing CLI and OpenClaw plugin commands map to the new runtime API
