package nostr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestWebSocketRelayClientPublishReadsOKReceipt(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read publish frame: %v", err)
			return
		}
		kind, parts, err := decodeRelayFrame(payload)
		if err != nil {
			t.Errorf("decode publish frame: %v", err)
			return
		}
		if kind != "EVENT" || len(parts) < 1 {
			t.Errorf("publish frame kind/parts = %q/%d, want EVENT/1+", kind, len(parts))
			return
		}
		var event Event
		if err := json.Unmarshal(parts[0], &event); err != nil {
			t.Errorf("decode publish event: %v", err)
			return
		}
		if err := conn.WriteJSON([]any{"OK", event.ID, true, "accepted"}); err != nil {
			t.Errorf("write OK frame: %v", err)
		}
	}))
	defer server.Close()

	client := NewWebSocketRelayClient()
	client.ReadTimeout = 2 * time.Second
	client.WriteTimeout = 2 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	receipt, err := client.Publish(ctx, toWebSocketURL(server.URL), Event{
		ID:        "evt_publish_1",
		PubKey:    "npub_sender",
		CreatedAt: 1711000100,
		Kind:      4,
		Tags:      [][]string{{"p", "npub_recipient"}},
		Content:   `{"message_id":"msg_1"}`,
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if got, want := receipt.EventID, "evt_publish_1"; got != want {
		t.Fatalf("Publish() event_id = %q, want %q", got, want)
	}
	if !receipt.Accepted {
		t.Fatalf("Publish() accepted = %t, want true", receipt.Accepted)
	}
}

func TestWebSocketRelayClientQueryCollectsEventsUntilEOSE(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("read query frame: %v", err)
			return
		}
		kind, parts, err := decodeRelayFrame(payload)
		if err != nil {
			t.Errorf("decode query frame: %v", err)
			return
		}
		if kind != "REQ" || len(parts) < 2 {
			t.Errorf("query frame kind/parts = %q/%d, want REQ/2+", kind, len(parts))
			return
		}
		subID, err := decodeFrameString(parts[0])
		if err != nil {
			t.Errorf("decode query sub id: %v", err)
			return
		}
		if err := conn.WriteJSON([]any{"EVENT", subID, map[string]any{
			"id":         "evt_1",
			"created_at": 100,
			"kind":       4,
			"content":    "{}",
		}}); err != nil {
			t.Errorf("write EVENT #1: %v", err)
			return
		}
		if err := conn.WriteJSON([]any{"EVENT", subID, map[string]any{
			"id":         "evt_2",
			"created_at": 101,
			"kind":       4,
			"content":    "{}",
		}}); err != nil {
			t.Errorf("write EVENT #2: %v", err)
			return
		}
		if err := conn.WriteJSON([]any{"EOSE", subID}); err != nil {
			t.Errorf("write EOSE: %v", err)
		}
	}))
	defer server.Close()

	client := NewWebSocketRelayClient()
	client.ReadTimeout = 2 * time.Second
	client.WriteTimeout = 2 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	events, err := client.Query(ctx, toWebSocketURL(server.URL), "sub_1", Filter{
		Kinds: []int{4},
		Limit: 2,
	})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if got, want := len(events), 2; got != want {
		t.Fatalf("Query() events len = %d, want %d", got, want)
	}
	if got, want := events[0].ID, "evt_1"; got != want {
		t.Fatalf("Query() first event id = %q, want %q", got, want)
	}
}

func TestWebSocketRelayClientPublishReturnsRejectedReceipt(t *testing.T) {
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade websocket: %v", err)
			return
		}
		defer conn.Close()

		if _, _, err := conn.ReadMessage(); err != nil {
			t.Errorf("read publish frame: %v", err)
			return
		}
		_ = conn.WriteJSON([]any{"OK", "evt_publish_2", false, "blocked"})
	}))
	defer server.Close()

	client := NewWebSocketRelayClient()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	receipt, err := client.Publish(ctx, toWebSocketURL(server.URL), Event{
		ID:        "evt_publish_2",
		CreatedAt: 1711000101,
		Kind:      4,
		Content:   "{}",
	})
	if err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	if receipt.Accepted {
		t.Fatalf("Publish() accepted = %t, want false", receipt.Accepted)
	}
	if got, want := receipt.Message, "blocked"; got != want {
		t.Fatalf("Publish() message = %q, want %q", got, want)
	}
}

func toWebSocketURL(httpURL string) string {
	if strings.HasPrefix(httpURL, "https://") {
		return "wss://" + strings.TrimPrefix(httpURL, "https://")
	}
	return "ws://" + strings.TrimPrefix(httpURL, "http://")
}
