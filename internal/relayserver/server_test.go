package relayserver

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/messagecrypto"
	"github.com/xiewanpeng/claw-identity/internal/relayclient"
)

func TestRelaySendPullAck(t *testing.T) {
	t.Parallel()

	server, result, err := Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay server: %v", err)
	}
	defer server.Shutdown(context.Background())

	client := relayclient.New()
	publicKey, _, err := messagecrypto.GenerateX25519KeyPair()
	if err != nil {
		t.Fatalf("generate x25519 key pair: %v", err)
	}
	encrypted, err := messagecrypto.EncryptForRecipient(publicKey, []byte("hello"))
	if err != nil {
		t.Fatalf("encrypt relay payload: %v", err)
	}
	sent, err := client.Send(context.Background(), result.URL, relayclient.SendRequest{
		MessageID:          "msg_1",
		SenderID:           "did:key:sender",
		SenderSigningKey:   "c2lnbmluZ19rZXk",
		RecipientID:        "rcpt_1",
		EphemeralPublicKey: encrypted.EphemeralPublicKey,
		Nonce:              encrypted.Nonce,
		Ciphertext:         encrypted.Ciphertext,
		Signature:          "c2lnbmF0dXJl",
		SentAt:             time.Now().UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		t.Fatalf("send relay message: %v", err)
	}
	if sent.RelayMessageID == "" || sent.Cursor == "" {
		t.Fatalf("expected relay message id and cursor")
	}

	pulled, err := client.Pull(context.Background(), result.URL, "rcpt_1", "")
	if err != nil {
		t.Fatalf("pull relay messages: %v", err)
	}
	if len(pulled.Messages) != 1 {
		t.Fatalf("pulled messages = %d, want 1", len(pulled.Messages))
	}
	if pulled.Messages[0].Ciphertext == "" {
		t.Fatalf("expected ciphertext in pulled message")
	}

	if err := client.Ack(context.Background(), result.URL, relayclient.AckRequest{
		RecipientID: "rcpt_1",
		Cursor:      pulled.NextCursor,
	}); err != nil {
		t.Fatalf("ack relay messages: %v", err)
	}

	pulledAgain, err := client.Pull(context.Background(), result.URL, "rcpt_1", "")
	if err != nil {
		t.Fatalf("pull relay messages again: %v", err)
	}
	if len(pulledAgain.Messages) != 0 {
		t.Fatalf("expected no remaining messages, got %d", len(pulledAgain.Messages))
	}
}
