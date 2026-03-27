package runtime

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/routing"
	"github.com/xiewanpeng/claw-identity/internal/transport"
)

func TestOpenStoreCreatesRuntimeTables(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	store, _, err := OpenStore(ctx, home, time.Now().UTC())
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	dbPath := filepath.Join(home, "state.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	for _, table := range []string{
		"runtime_self_identities",
		"runtime_contacts",
		"runtime_conversations",
		"runtime_messages",
		"runtime_route_attempts",
		"runtime_presence_cache",
		"runtime_store_forward_state",
		"runtime_transport_bindings",
		"runtime_transport_relays",
		"runtime_relay_sync_state",
		"runtime_relay_delivery_attempts",
		"runtime_recovered_event_observations",
	} {
		var name string
		err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&name)
		if err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
	}
}

func TestStorePersistsRuntimeRecords(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	store, _, err := OpenStore(ctx, home, time.Now().UTC())
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	if err := store.UpsertSelfIdentity(ctx, SelfIdentityRecord{
		SelfID:                  "self_1",
		DisplayName:             "Alice",
		PeerID:                  "peer-self-1",
		SigningPublicKey:        "signing-pub",
		EncryptionPublicKey:     "enc-pub",
		SigningPrivateKeyRef:    "signing-key",
		EncryptionPrivateKeyRef: "enc-key",
		TransportCapabilities:   []string{"direct", "store_forward"},
	}); err != nil {
		t.Fatalf("UpsertSelfIdentity() error = %v", err)
	}

	if err := store.UpsertContact(ctx, ContactRecord{
		ContactID:             "contact_1",
		CanonicalID:           "did:key:z6MkContact",
		DisplayName:           "Bob",
		PeerID:                "peer-contact-1",
		SigningPublicKey:      "peer-signing",
		EncryptionPublicKey:   "peer-encryption",
		TrustState:            "known",
		TransportCapabilities: []string{"direct"},
		DirectHints:           []string{"peer-contact-1"},
		StoreForwardHints:     []string{"sf://peer-contact-1"},
		RawIdentityCardJSON:   `{"id":"did:key:z6MkContact"}`,
	}); err != nil {
		t.Fatalf("UpsertContact() error = %v", err)
	}

	if err := store.UpsertConversation(ctx, ConversationRecord{
		ConversationID:     "conv_1",
		ContactID:          "contact_1",
		LastMessageID:      "msg_1",
		LastMessagePreview: "hello",
		LastMessageAt:      time.Now().UTC().Format(time.RFC3339Nano),
		UnreadCount:        1,
	}); err != nil {
		t.Fatalf("UpsertConversation() error = %v", err)
	}

	if err := store.InsertMessage(ctx, MessageRecord{
		MessageID:         "msg_1",
		ConversationID:    "conv_1",
		SenderID:          "self_1",
		RecipientID:       "contact_1",
		Direction:         "outgoing",
		PlaintextPreview:  "hello",
		Ciphertext:        "ciphertext",
		CiphertextVersion: "v1",
		Status:            "pending",
		SelectedRoute:     transport.RouteCandidate{Type: transport.RouteTypeDirect, Label: "peer-direct", Priority: 10},
		CreatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("InsertMessage() error = %v", err)
	}

	if err := store.RecordRouteAttempt(ctx, routing.RouteOutcome{
		MessageID:  "msg_1",
		Route:      transport.RouteCandidate{Type: transport.RouteTypeDirect, Label: "peer-direct", Priority: 10},
		Success:    true,
		Retryable:  false,
		OccurredAt: time.Now().UTC(),
	}, "conv_1", ""); err != nil {
		t.Fatalf("RecordRouteAttempt() error = %v", err)
	}
	allMessages, err := store.ListMessages(ctx)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}
	if len(allMessages) != 1 {
		t.Fatalf("ListMessages() len = %d, want 1", len(allMessages))
	}
	routeAttempts, err := store.ListRecentRouteAttempts(ctx, 5)
	if err != nil {
		t.Fatalf("ListRecentRouteAttempts() error = %v", err)
	}
	if len(routeAttempts) != 1 {
		t.Fatalf("ListRecentRouteAttempts() len = %d, want 1", len(routeAttempts))
	}

	dbPath := filepath.Join(home, "state.db")
	raw, err := os.ReadFile(dbPath)
	if err != nil || len(raw) == 0 {
		t.Fatalf("runtime sqlite file missing or empty: err=%v len=%d", err, len(raw))
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM runtime_contacts").Scan(&count); err != nil {
		t.Fatalf("count runtime_contacts: %v", err)
	}
	if count != 1 {
		t.Fatalf("runtime_contacts count = %d, want 1", count)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM runtime_route_attempts").Scan(&count); err != nil {
		t.Fatalf("count runtime_route_attempts: %v", err)
	}
	if count != 1 {
		t.Fatalf("runtime_route_attempts count = %d, want 1", count)
	}

	if err := store.SaveStoreForwardState(ctx, StoreForwardStateRecord{
		SelfID:             "self_1",
		RouteLabel:         "sf://peer-contact-1",
		CursorValue:        "cursor-1",
		LastResult:         "success",
		LastRecoveredCount: 2,
	}); err != nil {
		t.Fatalf("SaveStoreForwardState() error = %v", err)
	}
	cursor, err := store.LoadStoreForwardCursor(ctx, "self_1", "sf://peer-contact-1")
	if err != nil {
		t.Fatalf("LoadStoreForwardCursor() error = %v", err)
	}
	if cursor != "cursor-1" {
		t.Fatalf("LoadStoreForwardCursor() = %q, want %q", cursor, "cursor-1")
	}
	if err := store.UpsertPresence(ctx, PresenceRecord{
		CanonicalID:           "did:key:z6MkPresence",
		PeerID:                "lcpeer:presence-1",
		TransportCapabilities: []string{string(transport.RouteTypeDirect)},
		DirectHints:           []string{"libp2p://lcpeer:presence-1"},
		SignedPeerRecord:      `{"peer_id":"lcpeer:presence-1"}`,
		Source:                "libp2p-announce",
		Reachable:             true,
		ResolvedAt:            "2026-03-20T00:00:00Z",
		FreshUntil:            "2026-03-20T00:05:00Z",
		AnnouncedAt:           "2026-03-20T00:00:00Z",
	}); err != nil {
		t.Fatalf("UpsertPresence() error = %v", err)
	}

	if err := store.UpsertSelfIdentity(ctx, SelfIdentityRecord{
		SelfID:                "self_1",
		DisplayName:           "Alice",
		PeerID:                "peer-self-1",
		TransportCapabilities: []string{"store_forward", "recovery"},
	}); err != nil {
		t.Fatalf("UpsertSelfIdentity() error = %v", err)
	}
	if err := store.UpsertConversation(ctx, ConversationRecord{
		ConversationID:     "conv_1",
		ContactID:          "contact_1",
		LastMessageID:      "msg_1",
		LastMessagePreview: "hello",
		LastMessageAt:      "2026-03-20T00:00:00Z",
		UnreadCount:        2,
	}); err != nil {
		t.Fatalf("UpsertConversation() error = %v", err)
	}
	if err := store.UpsertMessage(ctx, MessageRecord{
		MessageID:        "msg_1",
		ConversationID:   "conv_1",
		SenderID:         "self_1",
		RecipientID:      "contact_1",
		Direction:        "outgoing",
		PlaintextBody:    "hello",
		PlaintextPreview: "hello",
		Status:           "queued",
		CreatedAt:        "2026-03-20T00:00:00Z",
	}); err != nil {
		t.Fatalf("InsertMessage() error = %v", err)
	}

	summary, err := store.LoadStatusSummary(ctx)
	if err != nil {
		t.Fatalf("LoadStatusSummary() error = %v", err)
	}
	if summary.SelfID != "self_1" {
		t.Fatalf("summary.SelfID = %q, want self_1", summary.SelfID)
	}
	if summary.Contacts != 1 || summary.Conversations != 1 || summary.Unread != 2 {
		t.Fatalf("unexpected counts: %+v", summary)
	}
	if summary.PendingOutbox != 1 {
		t.Fatalf("summary.PendingOutbox = %d, want 1", summary.PendingOutbox)
	}
	if summary.PresenceEntries != 1 {
		t.Fatalf("summary.PresenceEntries = %d, want 1", summary.PresenceEntries)
	}
	if summary.ReachablePresence != 1 {
		t.Fatalf("summary.ReachablePresence = %d, want 1", summary.ReachablePresence)
	}
	if summary.StoreForwardRoutes != 1 {
		t.Fatalf("summary.StoreForwardRoutes = %d, want 1", summary.StoreForwardRoutes)
	}
	if summary.LastAnnounceAt == "" {
		t.Fatal("summary.LastAnnounceAt = empty, want timestamp")
	}
}

func TestStoreUpsertContactPreservesLastSuccessfulRouteWhenUpdateIsEmpty(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	store, _, err := OpenStore(ctx, home, time.Now().UTC())
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	canonicalID := "did:key:z6MkRouteState"
	initialRoute := "wss://relay.initial.nostr.example?recipient=npub_initial"
	if err := store.UpsertContact(ctx, ContactRecord{
		ContactID:           "contact_route_state",
		CanonicalID:         canonicalID,
		DisplayName:         "Route State",
		LastSuccessfulRoute: initialRoute,
	}); err != nil {
		t.Fatalf("initial UpsertContact() error = %v", err)
	}

	if err := store.UpsertContact(ctx, ContactRecord{
		ContactID:           "contact_route_state",
		CanonicalID:         canonicalID,
		DisplayName:         "Route State Updated",
		LastSuccessfulRoute: "",
	}); err != nil {
		t.Fatalf("empty-route UpsertContact() error = %v", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	var preserved string
	if err := db.QueryRowContext(
		ctx,
		`SELECT last_successful_route FROM runtime_contacts WHERE canonical_id = ?`,
		canonicalID,
	).Scan(&preserved); err != nil {
		t.Fatalf("query preserved last_successful_route: %v", err)
	}
	if preserved != initialRoute {
		t.Fatalf("last_successful_route after empty update = %q, want %q", preserved, initialRoute)
	}

	nextRoute := "wss://relay.next.nostr.example?recipient=npub_next"
	if err := store.UpsertContact(ctx, ContactRecord{
		ContactID:           "contact_route_state",
		CanonicalID:         canonicalID,
		DisplayName:         "Route State Updated Again",
		LastSuccessfulRoute: nextRoute,
	}); err != nil {
		t.Fatalf("next-route UpsertContact() error = %v", err)
	}
	if err := db.QueryRowContext(
		ctx,
		`SELECT last_successful_route FROM runtime_contacts WHERE canonical_id = ?`,
		canonicalID,
	).Scan(&preserved); err != nil {
		t.Fatalf("query updated last_successful_route: %v", err)
	}
	if preserved != nextRoute {
		t.Fatalf("last_successful_route after non-empty update = %q, want %q", preserved, nextRoute)
	}
}

func TestStorePersistsNostrRuntimeFoundationRecords(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()

	store, _, err := OpenStore(ctx, home, time.Now().UTC())
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	if err := store.UpsertTransportRelay(ctx, TransportRelayRecord{
		RelayID:      "relay_1",
		Transport:    "nostr",
		RelayURL:     "wss://relay.example",
		ReadEnabled:  true,
		WriteEnabled: true,
		Priority:     10,
		Source:       "config",
		Status:       "active",
		MetadataJSON: `{"region":"apac"}`,
	}); err != nil {
		t.Fatalf("UpsertTransportRelay() error = %v", err)
	}

	relays, err := store.ListTransportRelays(ctx, "nostr")
	if err != nil {
		t.Fatalf("ListTransportRelays() error = %v", err)
	}
	if len(relays) != 1 {
		t.Fatalf("ListTransportRelays() len = %d, want 1", len(relays))
	}
	if !relays[0].ReadEnabled || !relays[0].WriteEnabled || relays[0].RelayURL != "wss://relay.example" {
		t.Fatalf("relay record = %+v, want enabled relay at wss://relay.example", relays[0])
	}

	if err := store.UpsertTransportBinding(ctx, TransportBindingRecord{
		BindingID:    "binding_1",
		SelfID:       "self_1",
		CanonicalID:  "did:key:z6MkPeer",
		Transport:    "nostr",
		RelayURL:     "wss://relay.example",
		RouteLabel:   "relay-main",
		RouteType:    string(transport.RouteTypeNostr),
		Direction:    "outgoing",
		Enabled:      true,
		MetadataJSON: `{"scope":"dm"}`,
	}); err != nil {
		t.Fatalf("UpsertTransportBinding() error = %v", err)
	}

	bindings, err := store.ListTransportBindings(ctx, "self_1")
	if err != nil {
		t.Fatalf("ListTransportBindings() error = %v", err)
	}
	if len(bindings) != 1 {
		t.Fatalf("ListTransportBindings() len = %d, want 1", len(bindings))
	}
	if bindings[0].RouteType != string(transport.RouteTypeNostr) || !bindings[0].Enabled {
		t.Fatalf("binding record = %+v, want enabled nostr route", bindings[0])
	}

	if err := store.SaveRelaySyncState(ctx, RelaySyncStateRecord{
		SelfID:              "self_1",
		RelayURL:            "wss://relay.example",
		LastCursor:          "cursor-42",
		LastEventAt:         "2026-03-26T08:00:00Z",
		LastSyncStartedAt:   "2026-03-26T08:00:01Z",
		LastSyncCompletedAt: "2026-03-26T08:00:02Z",
		LastResult:          "success",
		RecoveredCountTotal: 3,
	}); err != nil {
		t.Fatalf("SaveRelaySyncState() error = %v", err)
	}

	syncState, found, err := store.LoadRelaySyncState(ctx, "self_1", "wss://relay.example")
	if err != nil {
		t.Fatalf("LoadRelaySyncState() error = %v", err)
	}
	if !found {
		t.Fatal("LoadRelaySyncState() found = false, want true")
	}
	if syncState.LastCursor != "cursor-42" || syncState.RecoveredCountTotal != 3 {
		t.Fatalf("sync state = %+v, want cursor-42 and recovered total 3", syncState)
	}

	if err := store.RecordRelayDeliveryAttempt(ctx, RelayDeliveryAttemptRecord{
		AttemptID:    "attempt_1",
		MessageID:    "msg_1",
		EventID:      "evt_1",
		SelfID:       "self_1",
		CanonicalID:  "did:key:z6MkPeer",
		RelayURL:     "wss://relay.example",
		Operation:    "publish",
		Outcome:      "delivered",
		Retryable:    true,
		Acknowledged: true,
		MetadataJSON: `{"notice":"ok"}`,
		AttemptedAt:  "2026-03-26T08:00:03Z",
	}); err != nil {
		t.Fatalf("RecordRelayDeliveryAttempt() error = %v", err)
	}

	deliveryAttempts, err := store.ListRecentRelayDeliveryAttempts(ctx, "wss://relay.example", 5)
	if err != nil {
		t.Fatalf("ListRecentRelayDeliveryAttempts() error = %v", err)
	}
	if len(deliveryAttempts) != 1 {
		t.Fatalf("ListRecentRelayDeliveryAttempts() len = %d, want 1", len(deliveryAttempts))
	}
	if deliveryAttempts[0].Operation != "publish" || !deliveryAttempts[0].Retryable {
		t.Fatalf("delivery attempt = %+v, want publish and retryable=true", deliveryAttempts[0])
	}

	if err := store.UpsertRecoveredEventObservation(ctx, RecoveredEventObservationRecord{
		SelfID:       "self_1",
		EventID:      "evt_1",
		RelayURL:     "wss://relay.example",
		CanonicalID:  "did:key:z6MkPeer",
		MessageID:    "msg_1",
		ObservedAt:   "2026-03-26T08:00:04Z",
		PayloadHash:  "sha256:abc",
		PayloadJSON:  `{"kind":4}`,
		MetadataJSON: `{"source":"sync"}`,
	}); err != nil {
		t.Fatalf("UpsertRecoveredEventObservation() error = %v", err)
	}

	seen, err := store.HasRecoveredEventObservation(ctx, "self_1", "evt_1")
	if err != nil {
		t.Fatalf("HasRecoveredEventObservation() error = %v", err)
	}
	if !seen {
		t.Fatal("HasRecoveredEventObservation() = false, want true")
	}

	observations, err := store.ListRecoveredEventObservations(ctx, "self_1", 5)
	if err != nil {
		t.Fatalf("ListRecoveredEventObservations() error = %v", err)
	}
	if len(observations) != 1 {
		t.Fatalf("ListRecoveredEventObservations() len = %d, want 1", len(observations))
	}
	if observations[0].EventID != "evt_1" || observations[0].RelayURL != "wss://relay.example" {
		t.Fatalf("observation record = %+v, want evt_1 on wss://relay.example", observations[0])
	}
}

func TestStoreUpsertMessageRespectsRecoverableTransitions(t *testing.T) {
	ctx := context.Background()
	home := t.TempDir()
	now := time.Now().UTC()

	store, _, err := OpenStore(ctx, home, now)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	if err := store.InsertMessage(ctx, MessageRecord{
		MessageID:      "msg_transition_delivered",
		ConversationID: "conv_transition",
		Direction:      "outgoing",
		Status:         MessageStatusDelivered,
		CreatedAt:      now.Format(time.RFC3339Nano),
		DeliveredAt:    now.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("InsertMessage(delivered) error = %v", err)
	}

	if err := store.UpsertMessage(ctx, MessageRecord{
		MessageID:      "msg_transition_delivered",
		ConversationID: "conv_transition",
		Direction:      "outgoing",
		Status:         MessageStatusQueued,
		CreatedAt:      now.Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertMessage(queued rollback) error = %v", err)
	}

	if err := store.InsertMessage(ctx, MessageRecord{
		MessageID:      "msg_transition_retry",
		ConversationID: "conv_transition",
		Direction:      "outgoing",
		Status:         MessageStatusFailed,
		CreatedAt:      now.Add(1 * time.Second).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("InsertMessage(failed) error = %v", err)
	}

	if err := store.UpsertMessage(ctx, MessageRecord{
		MessageID:      "msg_transition_retry",
		ConversationID: "conv_transition",
		Direction:      "outgoing",
		Status:         MessageStatusQueued,
		CreatedAt:      now.Add(1 * time.Second).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertMessage(failed->queued) error = %v", err)
	}

	messages, err := store.ListMessages(ctx)
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}

	statusByID := make(map[string]MessageRecord, len(messages))
	for _, message := range messages {
		statusByID[message.MessageID] = message
	}

	delivered := statusByID["msg_transition_delivered"]
	if delivered.Status != MessageStatusDelivered {
		t.Fatalf("delivered status = %q, want %q", delivered.Status, MessageStatusDelivered)
	}
	if delivered.DeliveredAt == "" {
		t.Fatalf("delivered delivered_at = empty, want timestamp")
	}

	retried := statusByID["msg_transition_retry"]
	if retried.Status != MessageStatusQueued {
		t.Fatalf("retried status = %q, want %q", retried.Status, MessageStatusQueued)
	}
}
