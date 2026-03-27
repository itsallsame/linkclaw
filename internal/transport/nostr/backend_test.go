package nostr

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type captureRelayClient struct {
	publishRelayURL string
	publishEvent    Event
	publishReceipt  PublishReceipt
	publishErr      error

	queryRelayURL string
	querySubID    string
	queryFilter   Filter
	queryEvents   []Event
	queryErr      error
}

func (c *captureRelayClient) Publish(_ context.Context, relayURL string, event Event) (PublishReceipt, error) {
	c.publishRelayURL = relayURL
	c.publishEvent = event
	if c.publishErr != nil {
		return PublishReceipt{}, c.publishErr
	}
	return c.publishReceipt, nil
}

func (c *captureRelayClient) Query(_ context.Context, relayURL string, subscriptionID string, filter Filter) ([]Event, error) {
	c.queryRelayURL = relayURL
	c.querySubID = subscriptionID
	c.queryFilter = filter
	if c.queryErr != nil {
		return nil, c.queryErr
	}
	return append([]Event(nil), c.queryEvents...), nil
}

func TestBackendPublishBuildsEventAndReturnsSendResult(t *testing.T) {
	signer, err := NewSchnorrSignerFromPrivateKeyHex("74f9d3c7d2dd87c46071cb22de755f6319ebd8571c98fcef70ccf88be7fca8e7")
	if err != nil {
		t.Fatalf("NewSchnorrSignerFromPrivateKeyHex() error = %v", err)
	}
	client := &captureRelayClient{
		publishReceipt: PublishReceipt{
			EventID:  "evt_ack_1",
			Accepted: true,
			Message:  "ok",
		},
	}
	backend := NewBackendWithSigner(client, signer)
	backend.now = func() time.Time { return time.Unix(1_711_000_100, 0).UTC() }

	result, err := backend.Publish(context.Background(), transport.Envelope{
		MessageID:          "msg_1",
		SenderID:           "did:key:z6MkSender",
		RecipientID:        "did:key:z6MkRecipient",
		SenderSigningKey:   "sender_signing_key",
		EphemeralPublicKey: "ephemeral",
		Nonce:              "nonce",
		Ciphertext:         "cipher",
		SentAt:             "2026-03-27T01:23:45Z",
	}, transport.RouteCandidate{
		Type:   transport.RouteTypeNostr,
		Target: "wss://relay.example?recipient=npub_override",
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if got, want := result.RemoteID, "evt_ack_1"; got != want {
		t.Fatalf("Publish() remote_id = %q, want %q", got, want)
	}
	if !result.Delivered {
		t.Fatalf("Publish() delivered = %t, want true", result.Delivered)
	}
	if got, want := client.publishRelayURL, "wss://relay.example"; got != want {
		t.Fatalf("relay url = %q, want %q", got, want)
	}
	if got, want := client.publishEvent.Kind, defaultEventKind; got != want {
		t.Fatalf("event kind = %d, want %d", got, want)
	}
	if got, want := client.publishEvent.CreatedAt, int64(1_711_000_100); got != want {
		t.Fatalf("event created_at = %d, want %d", got, want)
	}
	if got, want := client.publishEvent.PubKey, signer.PublicKey(); got != want {
		t.Fatalf("event pubkey = %q, want %q", got, want)
	}
	if got := len(client.publishEvent.Sig); got != 128 {
		t.Fatalf("event sig len = %d, want 128", got)
	}
	verifyEventSignature(t, client.publishEvent)
	if len(client.publishEvent.Tags) == 0 || client.publishEvent.Tags[0][0] != "p" || client.publishEvent.Tags[0][1] != "npub_override" {
		t.Fatalf("event tags = %#v, want recipient p-tag from route query", client.publishEvent.Tags)
	}
	for _, tag := range client.publishEvent.Tags {
		if len(tag) > 0 && (tag[0] == "linkclaw_sender" || tag[0] == "linkclaw_recipient") {
			t.Fatalf("event tags = %#v, want no canonical sender/recipient tags", client.publishEvent.Tags)
		}
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(client.publishEvent.Content), &payload); err != nil {
		t.Fatalf("decode event content: %v", err)
	}
	if got, want := payload["message_id"], "msg_1"; got != want {
		t.Fatalf("payload message_id = %q, want %q", got, want)
	}
	if got, want := payload["recipient_pubkey"], "npub_override"; got != want {
		t.Fatalf("payload recipient_pubkey = %q, want %q", got, want)
	}
	if got, want := payload["ciphertext"], "cipher"; got != want {
		t.Fatalf("payload ciphertext = %q, want %q", got, want)
	}
	if _, ok := payload["plaintext"]; ok {
		t.Fatalf("payload plaintext should not be present: %#v", payload)
	}
	if _, ok := payload["sender_id"]; ok {
		t.Fatalf("payload sender_id should not be present: %#v", payload)
	}
	if _, ok := payload["recipient_id"]; ok {
		t.Fatalf("payload recipient_id should not be present: %#v", payload)
	}
}

func TestBackendPublishReturnsErrorWhenSignerIsMissing(t *testing.T) {
	client := &captureRelayClient{
		publishReceipt: PublishReceipt{
			EventID:  "evt_ack_1",
			Accepted: true,
		},
	}
	backend := NewBackend(client)

	_, err := backend.Publish(context.Background(), transport.Envelope{
		MessageID:  "msg_1",
		Ciphertext: "cipher",
	}, transport.RouteCandidate{
		Type:   transport.RouteTypeNostr,
		Target: "wss://relay.example?recipient=npub_1",
	})
	if err == nil {
		t.Fatal("Publish() error = nil, want signer missing error")
	}
}

func verifyEventSignature(t *testing.T, event Event) {
	t.Helper()
	eventHash, err := hex.DecodeString(event.ID)
	if err != nil {
		t.Fatalf("decode event id: %v", err)
	}
	signatureBytes, err := hex.DecodeString(event.Sig)
	if err != nil {
		t.Fatalf("decode event signature: %v", err)
	}
	signature, err := schnorr.ParseSignature(signatureBytes)
	if err != nil {
		t.Fatalf("parse event signature: %v", err)
	}
	pubKeyBytes, err := hex.DecodeString(event.PubKey)
	if err != nil {
		t.Fatalf("decode event pubkey: %v", err)
	}
	pubKey, err := schnorr.ParsePubKey(pubKeyBytes)
	if err != nil {
		t.Fatalf("parse event pubkey: %v", err)
	}
	if !signature.Verify(eventHash, pubKey) {
		t.Fatalf("event signature verification failed: id=%q sig=%q pubkey=%q", event.ID, event.Sig, event.PubKey)
	}
}

func TestBackendPublishReturnsErrorWhenRelayRejectsEvent(t *testing.T) {
	client := &captureRelayClient{
		publishReceipt: PublishReceipt{
			EventID:  "evt_ack_1",
			Accepted: false,
			Message:  "blocked",
		},
	}
	backend := NewBackend(client)

	_, err := backend.Publish(context.Background(), transport.Envelope{
		MessageID: "msg_1",
	}, transport.RouteCandidate{
		Type:   transport.RouteTypeNostr,
		Target: "wss://relay.example",
	})
	if err == nil {
		t.Fatal("Publish() error = nil, want relay rejection error")
	}
}

func TestBackendRecoverUsesRouteFilterAndBuildsCursor(t *testing.T) {
	client := &captureRelayClient{
		queryEvents: []Event{
			{ID: "evt_1", CreatedAt: 100},
			{ID: "evt_2", CreatedAt: 105},
		},
	}
	backend := NewBackend(client)
	backend.now = func() time.Time { return time.Unix(1_711_000_200, 0).UTC() }

	result, err := backend.Recover(context.Background(), transport.RouteCandidate{
		Type:   transport.RouteTypeNostr,
		Target: "wss://relay.example?recipient=npub_1&since=90&limit=2",
	})
	if err != nil {
		t.Fatalf("Recover() error = %v", err)
	}
	if got, want := result.Recovered, 2; got != want {
		t.Fatalf("Recover() recovered = %d, want %d", got, want)
	}
	if got, want := result.AdvancedCursor, "105"; got != want {
		t.Fatalf("Recover() advanced_cursor = %q, want %q", got, want)
	}
	if got, want := client.queryRelayURL, "wss://relay.example"; got != want {
		t.Fatalf("query relay url = %q, want %q", got, want)
	}
	if len(client.queryFilter.Recipient) != 1 || client.queryFilter.Recipient[0] != "npub_1" {
		t.Fatalf("query recipient filter = %#v, want [npub_1]", client.queryFilter.Recipient)
	}
	if client.queryFilter.Since == nil || *client.queryFilter.Since != 90 {
		t.Fatalf("query since filter = %#v, want 90", client.queryFilter.Since)
	}
	if got, want := client.queryFilter.Limit, 2; got != want {
		t.Fatalf("query limit = %d, want %d", got, want)
	}
	if client.querySubID == "" {
		t.Fatalf("query subscription id = empty")
	}
}

func TestBackendAcknowledgeValidatesRoute(t *testing.T) {
	backend := NewBackend(&captureRelayClient{})
	if err := backend.Acknowledge(context.Background(), transport.RouteCandidate{
		Type:   transport.RouteTypeNostr,
		Target: "wss://relay.example",
	}, "cursor-1"); err != nil {
		t.Fatalf("Acknowledge() error = %v", err)
	}
}

func TestBackendRejectsUnsupportedRouteType(t *testing.T) {
	backend := NewBackend(&captureRelayClient{})
	route := transport.RouteCandidate{
		Type:   transport.RouteTypeDirect,
		Target: "wss://relay.example",
	}
	if _, err := backend.Publish(context.Background(), transport.Envelope{}, route); err == nil {
		t.Fatal("Publish() error = nil, want unsupported route type error")
	}
	if _, err := backend.Recover(context.Background(), route); err == nil {
		t.Fatal("Recover() error = nil, want unsupported route type error")
	}
	if err := backend.Acknowledge(context.Background(), route, "cursor-1"); err == nil {
		t.Fatal("Acknowledge() error = nil, want unsupported route type error")
	}
}
