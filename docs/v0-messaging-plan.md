# LinkClaw V0 Messaging Plan

## Goal

LinkClaw V0 must let unfamiliar OpenClaw users establish one-to-one asynchronous messaging without requiring a domain.

The complete user loop is:

1. Install the LinkClaw plugin.
2. Auto-initialize a local identity.
3. Export an identity card.
4. Import the other user's identity card as a contact.
5. Send a message to a contact by name.
6. Store the message in a relay while the recipient is offline.
7. Sync, decrypt, and read the message when the recipient comes online.

## Scope

V0 includes:

- local messaging identity bootstrap
- signed identity-card export and verification
- contact import from identity card
- one-to-one message send/sync/inbox/outbox
- relay-backed store-and-forward delivery
- OpenClaw plugin commands for share/connect/message/inbox/sync

V0 excludes:

- domains and `did:web` as a prerequisite
- group messaging
- public registry/search as part of the messaging loop
- social anchors
- Nostr compatibility
- libp2p/P2P transport
- payment, reputation, task delegation

## Deliverables

### LinkClaw Core

- identity bootstrap
- identity-card schema and signature
- contact store
- local message store
- relay client
- CLI commands

### LinkClaw Relay

- encrypted message intake
- recipient-based mailbox reads
- ack/delete cursor support

### OpenClaw Plugin

- `/linkclaw-share`
- `/linkclaw-connect`
- `/linkclaw-contacts`
- `/linkclaw-message`
- `/linkclaw-inbox`
- `/linkclaw-sync`

## Milestones

### M1: Identity Card Foundation

- add messaging profile persistence for self identities
- implement `card export`
- implement `card verify`
- define stable identity-card JSON shape

### M2: Contact Import

- import verified identity cards into contacts
- persist raw card JSON and messaging route info
- expose contact list/show commands

### M3: Message Local Model

- add conversations/messages/delivery-attempt tables
- implement local send queue and inbox/outbox queries

### M4: Relay MVP

- implement `linkclaw-relay`
- support `POST /messages`
- support `GET /messages`
- support ack/cursor advancement

### M5: End-to-End Messaging

- encrypt/sign outgoing messages
- sync/decrypt incoming messages
- wire CLI send/sync/inbox/outbox

### M6: Plugin Experience

- expose share/connect/message/inbox/sync flows
- auto-sync on startup
- prompt before opening unknown-sender messages

## P0 Backlog

1. Define identity-card v1 schema and signing rules.
2. Add migration for self messaging profiles.
3. Implement self messaging profile bootstrap.
4. Implement card export in core and CLI.
5. Implement card verify in core and CLI.
6. Add tests for export/verify and tamper rejection.
7. Add migration for contact messaging route fields.
8. Implement contact import from identity card.
9. Add local message/conversation schema.
10. Implement relay MVP.
11. Implement send/sync/inbox/outbox.
12. Wire minimal plugin commands.

## Suggested Delivery Order

Week 1:

- M1 complete
- M2 started

Week 2:

- M2 complete
- M3 complete
- M4 started

Week 3:

- M4 complete
- M5 complete
- M6 started

Week 4:

- M6 complete
- polish, docs, and e2e test coverage
