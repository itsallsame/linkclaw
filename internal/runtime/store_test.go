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
