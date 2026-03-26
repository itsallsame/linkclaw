package message

import (
	"context"
	"strings"
	"testing"
	"time"

	agentruntime "github.com/xiewanpeng/claw-identity/internal/runtime"
	transportstoreforward "github.com/xiewanpeng/claw-identity/internal/transport/storeforward"
)

func TestBuildRecoveredObservationRecord(t *testing.T) {
	now := time.Date(2026, 3, 26, 8, 0, 0, 0, time.UTC)
	message := transportstoreforward.MailboxPullMessage{
		MessageID:      "msg_1",
		RelayMessageID: "evt_1",
		SenderID:       "did:key:z6MkPeer",
		Ciphertext:     "ciphertext",
		SentAt:         "2026-03-26T08:00:00Z",
	}

	record, ok, err := buildRecoveredObservationRecord("self_1", "https://relay.example", message, now)
	if err != nil {
		t.Fatalf("buildRecoveredObservationRecord() error = %v", err)
	}
	if !ok {
		t.Fatal("buildRecoveredObservationRecord() ok = false, want true")
	}
	if record.EventID != "evt_1" {
		t.Fatalf("record.EventID = %q, want evt_1", record.EventID)
	}
	if record.MessageID != "msg_1" {
		t.Fatalf("record.MessageID = %q, want msg_1", record.MessageID)
	}
	if record.RelayURL != "https://relay.example" {
		t.Fatalf("record.RelayURL = %q, want relay url", record.RelayURL)
	}
	if !strings.HasPrefix(record.PayloadHash, "sha256:") {
		t.Fatalf("record.PayloadHash = %q, want sha256 prefix", record.PayloadHash)
	}
	if !strings.Contains(record.PayloadJSON, "\"relay_message_id\":\"evt_1\"") {
		t.Fatalf("record.PayloadJSON = %q, want relay_message_id field", record.PayloadJSON)
	}
}

func TestPersistRecoveredObservationsDeduplicates(t *testing.T) {
	ctx := context.Background()
	store, _, err := agentruntime.OpenStore(ctx, t.TempDir(), time.Now().UTC())
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()

	observations := []agentruntime.RecoveredEventObservationRecord{
		{
			SelfID:      "self_1",
			EventID:     "evt_1",
			RelayURL:    "https://relay.example",
			CanonicalID: "did:key:z6MkPeer",
			MessageID:   "msg_1",
			ObservedAt:  "2026-03-26T08:00:00Z",
			PayloadHash: "sha256:a",
			PayloadJSON: `{"message_id":"msg_1"}`,
		},
		{
			SelfID:      "self_1",
			EventID:     "evt_1",
			RelayURL:    "https://relay.example",
			CanonicalID: "did:key:z6MkPeer",
			MessageID:   "msg_1",
			ObservedAt:  "2026-03-26T08:00:01Z",
			PayloadHash: "sha256:b",
			PayloadJSON: `{"message_id":"msg_1"}`,
		},
	}

	if err := persistRecoveredObservations(ctx, store, observations); err != nil {
		t.Fatalf("persistRecoveredObservations() error = %v", err)
	}

	rows, err := store.ListRecoveredEventObservations(ctx, "self_1", 10)
	if err != nil {
		t.Fatalf("ListRecoveredEventObservations() error = %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListRecoveredEventObservations() len = %d, want 1", len(rows))
	}
	if rows[0].EventID != "evt_1" || rows[0].RelayURL != "https://relay.example" {
		t.Fatalf("observation row = %+v, want evt_1 on relay", rows[0])
	}
}
