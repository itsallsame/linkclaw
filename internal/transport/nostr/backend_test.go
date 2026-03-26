package nostr

import (
	"context"
	"encoding/json"
	"testing"
	"time"

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
	client := &captureRelayClient{
		publishReceipt: PublishReceipt{
			EventID:  "evt_ack_1",
			Accepted: true,
			Message:  "ok",
		},
	}
	backend := NewBackend(client)
	backend.now = func() time.Time { return time.Unix(1_711_000_100, 0).UTC() }

	result, err := backend.Publish(context.Background(), transport.Envelope{
		MessageID:   "msg_1",
		SenderID:    "npub_sender",
		RecipientID: "npub_recipient",
		Plaintext:   "hello",
		Ciphertext:  "cipher",
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
	if len(client.publishEvent.Tags) == 0 || client.publishEvent.Tags[0][0] != "p" || client.publishEvent.Tags[0][1] != "npub_override" {
		t.Fatalf("event tags = %#v, want recipient p-tag from route query", client.publishEvent.Tags)
	}

	var payload map[string]string
	if err := json.Unmarshal([]byte(client.publishEvent.Content), &payload); err != nil {
		t.Fatalf("decode event content: %v", err)
	}
	if got, want := payload["message_id"], "msg_1"; got != want {
		t.Fatalf("payload message_id = %q, want %q", got, want)
	}
	if got, want := payload["ciphertext"], "cipher"; got != want {
		t.Fatalf("payload ciphertext = %q, want %q", got, want)
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
