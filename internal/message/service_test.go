package message

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/xiewanpeng/claw-identity/internal/card"
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
}
