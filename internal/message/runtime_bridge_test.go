package message

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/initflow"
	"github.com/xiewanpeng/claw-identity/internal/messagecrypto"
	agentruntime "github.com/xiewanpeng/claw-identity/internal/runtime"
	transportstoreforward "github.com/xiewanpeng/claw-identity/internal/transport/storeforward"

	_ "modernc.org/sqlite"
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

func TestSyncStoreForwardUsesCreatedAtEventIDCheckpoint(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	home, profile, recipientEncryptionPublicKey := setupRuntimeBridgeSyncHome(t, now)
	senderSigningPublicKey, senderSigningPrivateKeyPath := writeEd25519SigningKeyPair(t)

	const relayURL = "https://relay.example"
	const sentAt = "2026-03-26T08:00:00Z"

	store, _, err := agentruntime.OpenStore(ctx, home, now)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	if err := store.SaveRelaySyncState(ctx, agentruntime.RelaySyncStateRecord{
		SelfID:              profile.SelfID,
		RelayURL:            relayURL,
		LastCursor:          sentAt + "|evt_1",
		LastEventAt:         sentAt,
		LastSyncStartedAt:   now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
		LastSyncCompletedAt: now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
		LastResult:          "success",
		RecoveredCountTotal: 2,
		UpdatedAt:           now.Add(-2 * time.Minute).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("SaveRelaySyncState() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() runtime store error = %v", err)
	}

	backend := &stubMailboxBackend{
		pullResponse: transportstoreforward.MailboxPullResponse{
			Messages: []transportstoreforward.MailboxPullMessage{
				buildSignedMailboxPullMessage(
					t,
					recipientEncryptionPublicKey,
					senderSigningPrivateKeyPath,
					senderSigningPublicKey,
					profile.RecipientID,
					"did:key:z6MkSender",
					"msg_old",
					"evt_1",
					sentAt,
					"old",
				),
				buildSignedMailboxPullMessage(
					t,
					recipientEncryptionPublicKey,
					senderSigningPrivateKeyPath,
					senderSigningPublicKey,
					profile.RecipientID,
					"did:key:z6MkSender",
					"msg_new",
					"evt_2",
					sentAt,
					"new",
				),
			},
			NextCursor: "cursor-2",
		},
	}
	service := NewService()
	service.StoreForwardBackend = backend
	service.Now = func() time.Time { return now }

	synced, nextCursor, err := service.syncStoreForward(ctx, home, profile, relayURL, now)
	if err != nil {
		t.Fatalf("syncStoreForward() error = %v", err)
	}
	if synced != 1 {
		t.Fatalf("syncStoreForward() synced = %d, want 1", synced)
	}
	if nextCursor != "cursor-2" {
		t.Fatalf("syncStoreForward() next cursor = %q, want cursor-2", nextCursor)
	}
	if len(backend.pullCalls) != 1 {
		t.Fatalf("pull calls = %d, want 1", len(backend.pullCalls))
	}

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT message_id FROM messages WHERE direction = 'incoming' ORDER BY message_id ASC`)
	if err != nil {
		t.Fatalf("query incoming messages: %v", err)
	}
	defer rows.Close()
	messageIDs := make([]string, 0, 2)
	for rows.Next() {
		var messageID string
		if err := rows.Scan(&messageID); err != nil {
			t.Fatalf("scan message id: %v", err)
		}
		messageIDs = append(messageIDs, messageID)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate message ids: %v", err)
	}
	if len(messageIDs) != 1 || messageIDs[0] != "msg_new" {
		t.Fatalf("incoming message ids = %#v, want [msg_new]", messageIDs)
	}

	store, _, err = agentruntime.OpenStore(ctx, home, now)
	if err != nil {
		t.Fatalf("OpenStore() reload error = %v", err)
	}
	defer store.Close()

	syncState, found, err := store.LoadRelaySyncState(ctx, profile.SelfID, relayURL)
	if err != nil {
		t.Fatalf("LoadRelaySyncState() error = %v", err)
	}
	if !found {
		t.Fatal("LoadRelaySyncState() found = false, want true")
	}
	if got, want := syncState.LastCursor, sentAt+"|evt_2"; got != want {
		t.Fatalf("LastCursor = %q, want %q", got, want)
	}
	if got, want := syncState.LastEventAt, sentAt; got != want {
		t.Fatalf("LastEventAt = %q, want %q", got, want)
	}
	if got, want := syncState.RecoveredCountTotal, 3; got != want {
		t.Fatalf("RecoveredCountTotal = %d, want %d", got, want)
	}
}

func TestSyncStoreForwardSkipsRecipientBindingMismatch(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 26, 12, 30, 0, 0, time.UTC)
	home, profile, recipientEncryptionPublicKey := setupRuntimeBridgeSyncHome(t, now)
	senderSigningPublicKey, senderSigningPrivateKeyPath := writeEd25519SigningKeyPair(t)

	backend := &stubMailboxBackend{
		pullResponse: transportstoreforward.MailboxPullResponse{
			Messages: []transportstoreforward.MailboxPullMessage{
				buildSignedMailboxPullMessage(
					t,
					recipientEncryptionPublicKey,
					senderSigningPrivateKeyPath,
					senderSigningPublicKey,
					"rcpt_other",
					"did:key:z6MkSender",
					"msg_wrong_recipient",
					"evt_wrong_recipient",
					"2026-03-26T08:30:00Z",
					"ignored",
				),
			},
			NextCursor: "cursor-1",
		},
	}
	service := NewService()
	service.StoreForwardBackend = backend
	service.Now = func() time.Time { return now }

	synced, nextCursor, err := service.syncStoreForward(ctx, home, profile, "https://relay.example", now)
	if err != nil {
		t.Fatalf("syncStoreForward() error = %v", err)
	}
	if synced != 0 {
		t.Fatalf("syncStoreForward() synced = %d, want 0", synced)
	}
	if nextCursor != "cursor-1" {
		t.Fatalf("syncStoreForward() next cursor = %q, want cursor-1", nextCursor)
	}

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()
	var incomingCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM messages WHERE direction = 'incoming'`).Scan(&incomingCount); err != nil {
		t.Fatalf("count incoming messages: %v", err)
	}
	if incomingCount != 0 {
		t.Fatalf("incoming messages count = %d, want 0", incomingCount)
	}
}

type stubMailboxBackend struct {
	pullResponse transportstoreforward.MailboxPullResponse
	pullErr      error
	pullCalls    []stubMailboxPullCall
}

type stubMailboxPullCall struct {
	RouteLabel  string
	RecipientID string
	Cursor      string
}

func (b *stubMailboxBackend) Send(context.Context, string, transportstoreforward.MailboxSendRequest) (transportstoreforward.MailboxSendResponse, error) {
	return transportstoreforward.MailboxSendResponse{}, nil
}

func (b *stubMailboxBackend) Pull(_ context.Context, routeLabel string, recipientID string, cursor string) (transportstoreforward.MailboxPullResponse, error) {
	b.pullCalls = append(b.pullCalls, stubMailboxPullCall{
		RouteLabel:  routeLabel,
		RecipientID: recipientID,
		Cursor:      cursor,
	})
	if b.pullErr != nil {
		return transportstoreforward.MailboxPullResponse{}, b.pullErr
	}
	return b.pullResponse, nil
}

func (b *stubMailboxBackend) Ack(context.Context, string, transportstoreforward.MailboxAckRequest) error {
	return nil
}

func setupRuntimeBridgeSyncHome(t *testing.T, now time.Time) (string, selfMessagingProfile, string) {
	t.Helper()
	home := filepath.Join(t.TempDir(), "home")
	if _, err := initflow.NewService().Init(context.Background(), initflow.Options{
		Home:        home,
		CanonicalID: "did:key:z6MkReceiver",
		DisplayName: "Receiver",
	}); err != nil {
		t.Fatalf("init home: %v", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	profile, err := loadSelfMessagingProfile(context.Background(), db, home)
	if err != nil {
		t.Fatalf("loadSelfMessagingProfile() error = %v", err)
	}
	var encryptionPublicKey string
	if err := db.QueryRowContext(
		context.Background(),
		`SELECT encryption_public_key FROM self_messaging_profiles WHERE self_id = ? LIMIT 1`,
		profile.SelfID,
	).Scan(&encryptionPublicKey); err != nil {
		t.Fatalf("query encryption_public_key: %v", err)
	}
	if strings.TrimSpace(encryptionPublicKey) == "" {
		t.Fatal("encryption_public_key = empty")
	}
	return home, profile, encryptionPublicKey
}

func writeEd25519SigningKeyPair(t *testing.T) (string, string) {
	t.Helper()
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey() error = %v", err)
	}
	privateKeyPKCS8, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("x509.MarshalPKCS8PrivateKey() error = %v", err)
	}
	privateKeyPath := filepath.Join(t.TempDir(), "sender-signing.pem")
	if err := os.WriteFile(
		privateKeyPath,
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyPKCS8}),
		0o600,
	); err != nil {
		t.Fatalf("write sender signing private key: %v", err)
	}
	return base64.RawStdEncoding.EncodeToString(publicKey), privateKeyPath
}

func buildSignedMailboxPullMessage(
	t *testing.T,
	recipientEncryptionPublicKey string,
	senderSigningPrivateKeyPath string,
	senderSigningPublicKey string,
	recipientID string,
	senderID string,
	messageID string,
	relayMessageID string,
	sentAt string,
	body string,
) transportstoreforward.MailboxPullMessage {
	t.Helper()
	encrypted, err := messagecrypto.EncryptForRecipient(recipientEncryptionPublicKey, []byte(body))
	if err != nil {
		t.Fatalf("EncryptForRecipient() error = %v", err)
	}
	payload := signedMessagePayload{
		MessageID:          messageID,
		SenderID:           senderID,
		SenderSigningKey:   senderSigningPublicKey,
		RecipientID:        recipientID,
		EphemeralPublicKey: encrypted.EphemeralPublicKey,
		Nonce:              encrypted.Nonce,
		Ciphertext:         encrypted.Ciphertext,
		SentAt:             sentAt,
	}
	payloadBytes, err := marshalSignedPayload(payload)
	if err != nil {
		t.Fatalf("marshalSignedPayload() error = %v", err)
	}
	signature, err := messagecrypto.SignPayload(senderSigningPrivateKeyPath, payloadBytes)
	if err != nil {
		t.Fatalf("SignPayload() error = %v", err)
	}
	return transportstoreforward.MailboxPullMessage{
		MessageID:          messageID,
		RelayMessageID:     relayMessageID,
		SenderID:           senderID,
		SenderSigningKey:   senderSigningPublicKey,
		RecipientID:        recipientID,
		EphemeralPublicKey: encrypted.EphemeralPublicKey,
		Nonce:              encrypted.Nonce,
		Ciphertext:         encrypted.Ciphertext,
		Signature:          signature,
		SentAt:             sentAt,
	}
}
