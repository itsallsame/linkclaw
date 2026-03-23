package message

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/xiewanpeng/claw-identity/internal/card"
	discoverylibp2p "github.com/xiewanpeng/claw-identity/internal/discovery/libp2p"
	"github.com/xiewanpeng/claw-identity/internal/initflow"
	"github.com/xiewanpeng/claw-identity/internal/relayserver"
)

func TestSendAndOutbox(t *testing.T) {
	t.Parallel()

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	initService := initflow.NewService()
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAlice",
		DisplayName: "Alice",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}

	cardService := card.NewService()
	exported, err := cardService.Export(context.Background(), card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	cardJSON, err := json.Marshal(exported.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBob",
		DisplayName: "Bob",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	imported, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  bobHome,
		Input: string(cardJSON),
	})
	if err != nil {
		t.Fatalf("import alice card into bob home: %v", err)
	}

	service := NewService()
	sent, err := service.Send(context.Background(), SendOptions{
		Home:       bobHome,
		ContactRef: imported.ContactID,
		Body:       "hello alice",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if sent.Message.Status != StatusPending {
		t.Fatalf("message status = %q, want %q", sent.Message.Status, StatusPending)
	}
	if sent.Message.TransportStatus != TransportStatusDeferred {
		t.Fatalf("transport status = %q, want %q", sent.Message.TransportStatus, TransportStatusDeferred)
	}

	outbox, err := service.Outbox(context.Background(), ListOptions{Home: bobHome})
	if err != nil {
		t.Fatalf("load outbox: %v", err)
	}
	if len(outbox.Messages) != 1 {
		t.Fatalf("outbox messages = %d, want 1", len(outbox.Messages))
	}
	if outbox.Messages[0].Body != "hello alice" {
		t.Fatalf("outbox body = %q", outbox.Messages[0].Body)
	}
	if outbox.Messages[0].TransportStatus != TransportStatusDeferred {
		t.Fatalf("outbox transport status = %q, want %q", outbox.Messages[0].TransportStatus, TransportStatusDeferred)
	}

	inbox, err := service.Inbox(context.Background(), ListOptions{Home: bobHome})
	if err != nil {
		t.Fatalf("load inbox conversations: %v", err)
	}
	if len(inbox.Conversations) != 1 {
		t.Fatalf("inbox conversations = %d, want 1", len(inbox.Conversations))
	}
	if inbox.Conversations[0].ContactDisplayName != "Alice" {
		t.Fatalf("conversation display name = %q, want Alice", inbox.Conversations[0].ContactDisplayName)
	}
}

func TestSendAndSyncWithRelay(t *testing.T) {
	relay, relayResult, err := relayserver.Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Shutdown(context.Background())

	t.Setenv(card.EnvRelayURL, relayResult.URL)

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	initService := initflow.NewService()
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAlice",
		DisplayName: "Alice",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}
	cardService := card.NewService()
	aliceCard, err := cardService.Export(context.Background(), card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	aliceCardJSON, err := json.Marshal(aliceCard.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBob",
		DisplayName: "Bob",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	bobCard, err := cardService.Export(context.Background(), card.Options{Home: bobHome})
	if err != nil {
		t.Fatalf("export bob card: %v", err)
	}
	bobCardJSON, err := json.Marshal(bobCard.Card)
	if err != nil {
		t.Fatalf("marshal bob card: %v", err)
	}

	aliceImportedIntoBob, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  bobHome,
		Input: string(aliceCardJSON),
	})
	if err != nil {
		t.Fatalf("import alice into bob: %v", err)
	}
	if _, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  aliceHome,
		Input: string(bobCardJSON),
	}); err != nil {
		t.Fatalf("import bob into alice: %v", err)
	}

	service := NewService()
	sent, err := service.Send(context.Background(), SendOptions{
		Home:       bobHome,
		ContactRef: aliceImportedIntoBob.ContactID,
		Body:       "hello from bob",
	})
	if err != nil {
		t.Fatalf("send message through relay: %v", err)
	}
	if sent.Message.Status != StatusQueued {
		t.Fatalf("message status = %q, want %q", sent.Message.Status, StatusQueued)
	}
	if sent.Message.TransportStatus != TransportStatusDeferred {
		t.Fatalf("transport status = %q, want %q", sent.Message.TransportStatus, TransportStatusDeferred)
	}

	synced, err := service.Sync(context.Background(), SyncOptions{Home: aliceHome})
	if err != nil {
		t.Fatalf("sync alice inbox: %v", err)
	}
	if synced.Synced != 1 {
		t.Fatalf("synced count = %d, want 1", synced.Synced)
	}

	inbox, err := service.Inbox(context.Background(), ListOptions{Home: aliceHome})
	if err != nil {
		t.Fatalf("load alice inbox: %v", err)
	}
	if len(inbox.Conversations) != 1 {
		t.Fatalf("inbox conversations = %d, want 1", len(inbox.Conversations))
	}
	if inbox.Conversations[0].ContactCanonicalID != "did:key:z6MkBob" {
		t.Fatalf("incoming contact canonical id = %q, want bob", inbox.Conversations[0].ContactCanonicalID)
	}
	if inbox.Conversations[0].LastMessagePreview != "hello from bob" {
		t.Fatalf("incoming preview = %q", inbox.Conversations[0].LastMessagePreview)
	}

	thread, err := service.Thread(context.Background(), ThreadOptions{
		Home:       aliceHome,
		ContactRef: "did:key:z6MkBob",
		Limit:      10,
		MarkRead:   true,
	})
	if err != nil {
		t.Fatalf("load alice thread: %v", err)
	}
	if len(thread.Conversation.Messages) != 1 {
		t.Fatalf("thread messages = %d, want 1", len(thread.Conversation.Messages))
	}
	if thread.Conversation.Messages[0].Body != "hello from bob" {
		t.Fatalf("thread body = %q, want %q", thread.Conversation.Messages[0].Body, "hello from bob")
	}
	if thread.Conversation.Messages[0].TransportStatus != TransportStatusRecovered {
		t.Fatalf("thread transport status = %q, want %q", thread.Conversation.Messages[0].TransportStatus, TransportStatusRecovered)
	}
	if thread.Conversation.UnreadCount != 0 {
		t.Fatalf("thread unread count = %d, want 0", thread.Conversation.UnreadCount)
	}

	inboxAfterThread, err := service.Inbox(context.Background(), ListOptions{Home: aliceHome})
	if err != nil {
		t.Fatalf("load alice inbox after thread: %v", err)
	}
	if inboxAfterThread.Conversations[0].UnreadCount != 0 {
		t.Fatalf("inbox unread after thread = %d, want 0", inboxAfterThread.Conversations[0].UnreadCount)
	}
}

func TestSendAndSyncWithRelayAndExperimentalDirectFallback(t *testing.T) {
	relay, relayResult, err := relayserver.Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Shutdown(context.Background())

	t.Setenv(card.EnvRelayURL, relayResult.URL)
	t.Setenv(discoverylibp2p.EnvExperimentalDirect, "1")

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	initService := initflow.NewService()
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAlice",
		DisplayName: "Alice",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}
	cardService := card.NewService()
	aliceCard, err := cardService.Export(context.Background(), card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	aliceCardJSON, err := json.Marshal(aliceCard.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBob",
		DisplayName: "Bob",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	bobCard, err := cardService.Export(context.Background(), card.Options{Home: bobHome})
	if err != nil {
		t.Fatalf("export bob card: %v", err)
	}
	bobCardJSON, err := json.Marshal(bobCard.Card)
	if err != nil {
		t.Fatalf("marshal bob card: %v", err)
	}

	aliceImportedIntoBob, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  bobHome,
		Input: string(aliceCardJSON),
	})
	if err != nil {
		t.Fatalf("import alice into bob: %v", err)
	}
	if _, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  aliceHome,
		Input: string(bobCardJSON),
	}); err != nil {
		t.Fatalf("import bob into alice: %v", err)
	}

	service := NewService()
	sent, err := service.Send(context.Background(), SendOptions{
		Home:       bobHome,
		ContactRef: aliceImportedIntoBob.ContactID,
		Body:       "hello from bob with direct fallback",
	})
	if err != nil {
		t.Fatalf("send message through relay with direct fallback: %v", err)
	}
	if sent.Message.Status != StatusQueued {
		t.Fatalf("message status = %q, want %q", sent.Message.Status, StatusQueued)
	}
	if sent.Message.TransportStatus != TransportStatusDeferred {
		t.Fatalf("transport status = %q, want %q", sent.Message.TransportStatus, TransportStatusDeferred)
	}

	synced, err := service.Sync(context.Background(), SyncOptions{Home: aliceHome})
	if err != nil {
		t.Fatalf("sync alice inbox: %v", err)
	}
	if synced.Synced != 1 {
		t.Fatalf("synced count = %d, want 1", synced.Synced)
	}

	bobAttempts := loadRuntimeRouteAttempts(t, bobHome)
	requireRouteAttempt(t, bobAttempts, "direct", "failed", "")
	requireRouteAttempt(t, bobAttempts, "store_forward", "queued", "")

	aliceAttempts := loadRuntimeRouteAttempts(t, aliceHome)
	recoveryCursor := requireRouteAttemptWithNonEmptyCursor(t, aliceAttempts, "recovery", "recovered")
	requireRouteAttempt(t, aliceAttempts, "recovery", "acked", recoveryCursor)
}

func TestStatusSummary(t *testing.T) {
	relay, relayResult, err := relayserver.Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Shutdown(context.Background())

	t.Setenv(card.EnvRelayURL, relayResult.URL)

	home := filepath.Join(t.TempDir(), "status-home")
	initService := initflow.NewService()
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        home,
		CanonicalID: "did:key:z6MkStatusSummary",
		DisplayName: "StatusSummary",
	}); err != nil {
		t.Fatalf("init home: %v", err)
	}

	service := NewService()
	result, err := service.Status(context.Background(), ListOptions{Home: home})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if result.DisplayName != "StatusSummary" {
		t.Fatalf("display name = %q, want StatusSummary", result.DisplayName)
	}
	if result.StoreForwardRoutes != 0 {
		t.Fatalf("store forward routes = %d, want 0", result.StoreForwardRoutes)
	}
	if !result.IdentityReady || !result.TransportReady {
		t.Fatalf("expected identity/transport ready, got identity=%t transport=%t", result.IdentityReady, result.TransportReady)
	}
	if result.Contacts != 0 || result.Conversations != 0 || result.Unread != 0 {
		t.Fatalf("unexpected counts: %+v", result)
	}
	if len(result.TransportCapabilities) == 0 {
		t.Fatalf("expected transport capabilities, got none")
	}
}

func TestStatusSummaryTracksRecoveryState(t *testing.T) {
	relay, relayResult, err := relayserver.Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Shutdown(context.Background())

	t.Setenv(card.EnvRelayURL, relayResult.URL)

	initService := initflow.NewService()
	cardService := card.NewService()

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAliceStatusRecovery",
		DisplayName: "Alice Status Recovery",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}
	aliceCard, err := cardService.Export(context.Background(), card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	aliceCardJSON, err := json.Marshal(aliceCard.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBobStatusRecovery",
		DisplayName: "Bob Status Recovery",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	bobCard, err := cardService.Export(context.Background(), card.Options{Home: bobHome})
	if err != nil {
		t.Fatalf("export bob card: %v", err)
	}
	bobCardJSON, err := json.Marshal(bobCard.Card)
	if err != nil {
		t.Fatalf("marshal bob card: %v", err)
	}

	aliceImportedIntoBob, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  bobHome,
		Input: string(aliceCardJSON),
	})
	if err != nil {
		t.Fatalf("import alice into bob: %v", err)
	}
	if _, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  aliceHome,
		Input: string(bobCardJSON),
	}); err != nil {
		t.Fatalf("import bob into alice: %v", err)
	}

	service := NewService()
	if _, err := service.Send(context.Background(), SendOptions{
		Home:       bobHome,
		ContactRef: aliceImportedIntoBob.ContactID,
		Body:       "hello with recovery state",
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, err := service.Sync(context.Background(), SyncOptions{Home: aliceHome}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	status, err := service.Status(context.Background(), ListOptions{Home: aliceHome})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Conversations != 1 || status.Unread != 1 {
		t.Fatalf("unexpected conversation counters: %+v", status)
	}
	if status.StoreForwardRoutes != 1 {
		t.Fatalf("store forward routes = %d, want 1", status.StoreForwardRoutes)
	}
	if status.LastRecoveredCount != 1 {
		t.Fatalf("last recovered count = %d, want 1", status.LastRecoveredCount)
	}
	if status.LastStoreForwardResult != "success" {
		t.Fatalf("last store-forward result = %q, want success", status.LastStoreForwardResult)
	}
	if status.MessageStatusRecovered < 1 {
		t.Fatalf("message recovered counter = %d, want >= 1", status.MessageStatusRecovered)
	}
	if len(status.RecentRouteOutcomes) == 0 {
		t.Fatal("recent route outcomes should not be empty after sync")
	}
}

func TestDirectDeliveryWhenBothHostsAreOnline(t *testing.T) {
	relay, relayResult, err := relayserver.Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Shutdown(context.Background())

	t.Setenv(card.EnvRelayURL, relayResult.URL)
	t.Setenv(discoverylibp2p.EnvExperimentalDirect, "1")

	initService := initflow.NewService()
	cardService := card.NewService()

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAliceDirect",
		DisplayName: "Alice Direct",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}
	aliceCard, err := cardService.Export(context.Background(), card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	aliceCardJSON, err := json.Marshal(aliceCard.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBobDirect",
		DisplayName: "Bob Direct",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	bobCard, err := cardService.Export(context.Background(), card.Options{Home: bobHome})
	if err != nil {
		t.Fatalf("export bob card: %v", err)
	}
	bobCardJSON, err := json.Marshal(bobCard.Card)
	if err != nil {
		t.Fatalf("marshal bob card: %v", err)
	}

	aliceImportedIntoBob, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  bobHome,
		Input: string(aliceCardJSON),
	})
	if err != nil {
		t.Fatalf("import alice into bob: %v", err)
	}
	if _, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  aliceHome,
		Input: string(bobCardJSON),
	}); err != nil {
		t.Fatalf("import bob into alice: %v", err)
	}

	aliceService := NewService()
	if _, err := aliceService.Status(context.Background(), ListOptions{Home: aliceHome}); err != nil {
		t.Fatalf("prime alice direct runtime: %v", err)
	}

	bobService := NewService()
	sent, err := bobService.Send(context.Background(), SendOptions{
		Home:       bobHome,
		ContactRef: aliceImportedIntoBob.ContactID,
		Body:       "hello direct online",
	})
	if err != nil {
		t.Fatalf("direct send: %v", err)
	}
	if sent.Message.Status != StatusDelivered {
		t.Fatalf("message status = %q, want %q", sent.Message.Status, StatusDelivered)
	}
	if sent.Message.TransportStatus != TransportStatusDirect {
		t.Fatalf("transport status = %q, want %q", sent.Message.TransportStatus, TransportStatusDirect)
	}

	inbox, err := aliceService.Inbox(context.Background(), ListOptions{Home: aliceHome})
	if err != nil {
		t.Fatalf("alice inbox: %v", err)
	}
	if len(inbox.Conversations) != 1 {
		t.Fatalf("inbox conversations = %d, want 1", len(inbox.Conversations))
	}
	if inbox.Conversations[0].LastMessagePreview != "hello direct online" {
		t.Fatalf("preview = %q, want %q", inbox.Conversations[0].LastMessagePreview, "hello direct online")
	}
	if inbox.Conversations[0].UnreadCount != 1 {
		t.Fatalf("unread = %d, want 1", inbox.Conversations[0].UnreadCount)
	}
}

func TestDirectDeliveryPreservesMultipleMessages(t *testing.T) {
	relay, relayResult, err := relayserver.Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Shutdown(context.Background())

	t.Setenv(card.EnvRelayURL, relayResult.URL)
	t.Setenv(discoverylibp2p.EnvExperimentalDirect, "1")

	initService := initflow.NewService()
	cardService := card.NewService()

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAliceDirectMulti",
		DisplayName: "Alice Direct Multi",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}
	aliceCard, err := cardService.Export(context.Background(), card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	aliceCardJSON, err := json.Marshal(aliceCard.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBobDirectMulti",
		DisplayName: "Bob Direct Multi",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	bobCard, err := cardService.Export(context.Background(), card.Options{Home: bobHome})
	if err != nil {
		t.Fatalf("export bob card: %v", err)
	}
	bobCardJSON, err := json.Marshal(bobCard.Card)
	if err != nil {
		t.Fatalf("marshal bob card: %v", err)
	}

	aliceImportedIntoBob, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  bobHome,
		Input: string(aliceCardJSON),
	})
	if err != nil {
		t.Fatalf("import alice into bob: %v", err)
	}
	if _, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  aliceHome,
		Input: string(bobCardJSON),
	}); err != nil {
		t.Fatalf("import bob into alice: %v", err)
	}

	aliceService := NewService()
	if _, err := aliceService.Status(context.Background(), ListOptions{Home: aliceHome}); err != nil {
		t.Fatalf("prime alice direct runtime: %v", err)
	}

	bobService := NewService()
	first, err := bobService.Send(context.Background(), SendOptions{
		Home:       bobHome,
		ContactRef: aliceImportedIntoBob.ContactID,
		Body:       "hello direct first",
	})
	if err != nil {
		t.Fatalf("first direct send: %v", err)
	}
	second, err := bobService.Send(context.Background(), SendOptions{
		Home:       bobHome,
		ContactRef: aliceImportedIntoBob.ContactID,
		Body:       "hello direct second",
	})
	if err != nil {
		t.Fatalf("second direct send: %v", err)
	}
	if first.Message.MessageID == second.Message.MessageID {
		t.Fatalf("message ids collided: %q", first.Message.MessageID)
	}

	thread, err := aliceService.Thread(context.Background(), ThreadOptions{
		Home:       aliceHome,
		ContactRef: "did:key:z6MkBobDirectMulti",
		Limit:      10,
		MarkRead:   false,
	})
	if err != nil {
		t.Fatalf("alice thread: %v", err)
	}
	if len(thread.Conversation.Messages) != 2 {
		t.Fatalf("thread messages = %d, want 2", len(thread.Conversation.Messages))
	}
	bodies := map[string]bool{}
	for _, msg := range thread.Conversation.Messages {
		bodies[msg.Body] = true
		if msg.TransportStatus != TransportStatusDirect {
			t.Fatalf("thread message transport status = %q, want %q", msg.TransportStatus, TransportStatusDirect)
		}
	}
	if !bodies["hello direct first"] || !bodies["hello direct second"] {
		t.Fatalf("thread bodies = %#v", thread.Conversation.Messages)
	}
}

type runtimeRouteAttempt struct {
	RouteType string
	Outcome   string
	Cursor    string
}

func loadRuntimeRouteAttempts(t *testing.T, home string) []runtimeRouteAttempt {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT route_type, outcome, cursor_value FROM runtime_route_attempts`)
	if err != nil {
		t.Fatalf("query runtime_route_attempts: %v", err)
	}
	defer rows.Close()

	attempts := make([]runtimeRouteAttempt, 0)
	for rows.Next() {
		var record runtimeRouteAttempt
		if err := rows.Scan(&record.RouteType, &record.Outcome, &record.Cursor); err != nil {
			t.Fatalf("scan runtime_route_attempts: %v", err)
		}
		attempts = append(attempts, record)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate runtime_route_attempts: %v", err)
	}
	return attempts
}

func requireRouteAttempt(t *testing.T, attempts []runtimeRouteAttempt, routeType, outcome, cursor string) {
	t.Helper()
	for _, attempt := range attempts {
		if attempt.RouteType == routeType && attempt.Outcome == outcome && attempt.Cursor == cursor {
			return
		}
	}
	t.Fatalf("route attempt (%s,%s,%s) not found, got %#v", routeType, outcome, cursor, attempts)
}

func requireRouteAttemptWithNonEmptyCursor(t *testing.T, attempts []runtimeRouteAttempt, routeType, outcome string) string {
	t.Helper()
	for _, attempt := range attempts {
		if attempt.RouteType == routeType && attempt.Outcome == outcome && attempt.Cursor != "" {
			return attempt.Cursor
		}
	}
	t.Fatalf("route attempt (%s,%s,<non-empty cursor>) not found, got %#v", routeType, outcome, attempts)
	return ""
}
