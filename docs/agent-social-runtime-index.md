# Agent-Social Runtime Design Index

This index collects the post-V0 communication design set for LinkClaw's `agent-social runtime`.

Use this set when working on:

- communication-layer architecture
- runtime/discovery/transport refactors
- libp2p-first design work
- OpenClaw-hosted runtime behavior

## Reading Order

### 1. Architecture Direction

- [Agent-Social Runtime Design](./agent-social-runtime-design.md)

Read this first for:

- product direction
- runtime placement
- direct vs store-and-forward behavior
- why the new design is no longer relay-centered

### 2. Module And Migration Shape

- [Agent-Social Runtime Implementation Plan](./agent-social-runtime-implementation-plan.md)

Read this second for:

- new internal modules
- how existing `internal/message`, `card`, `importer`, and `relayserver` should evolve
- runtime/routing/discovery/transport interface boundaries

### 3. Clean-Slate Storage Model

- [Agent-Social Runtime Clean-Slate Plan](./agent-social-runtime-clean-slate-plan.md)

Read this third for:

- no-migration assumption
- new communication storage entities
- route types
- libp2p integration boundary

### 4. API Contract And Execution Roadmap

- [Agent-Social Runtime API And Roadmap](./agent-social-runtime-api-and-roadmap.md)

Read this last for:

- runtime API contract
- CLI/plugin mapping
- staged implementation TODO lists
- recommended build sequence

## Scope Boundary

These runtime design docs are intentionally separate from:

- [V0 Messaging Plan](./v0-messaging-plan.md)
- [OpenClaw User Manual (ZH)](./OPENCLAW_USER_MANUAL_ZH.md)
- [OpenClaw Plugin Release Checklist](./OPENCLAW_PLUGIN_RELEASE_CHECKLIST.md)

The V0 messaging plan explains the relay-backed prototype.
The runtime design set explains the architecture that should replace the relay-centric model.
