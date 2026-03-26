package nostr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	defaultSocketReadTimeout  = 10 * time.Second
	defaultSocketWriteTimeout = 10 * time.Second
)

type Event struct {
	ID        string     `json:"id,omitempty"`
	PubKey    string     `json:"pubkey,omitempty"`
	CreatedAt int64      `json:"created_at"`
	Kind      int        `json:"kind"`
	Tags      [][]string `json:"tags,omitempty"`
	Content   string     `json:"content"`
	Sig       string     `json:"sig,omitempty"`
}

type Filter struct {
	Kinds     []int    `json:"kinds,omitempty"`
	Recipient []string `json:"#p,omitempty"`
	Since     *int64   `json:"since,omitempty"`
	Limit     int      `json:"limit,omitempty"`
}

type PublishReceipt struct {
	EventID  string
	Accepted bool
	Message  string
}

type RelayClient interface {
	Publish(ctx context.Context, relayURL string, event Event) (PublishReceipt, error)
	Query(ctx context.Context, relayURL string, subscriptionID string, filter Filter) ([]Event, error)
}

type WebSocketRelayClient struct {
	Dialer       *websocket.Dialer
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

func NewWebSocketRelayClient() *WebSocketRelayClient {
	return &WebSocketRelayClient{
		Dialer:       websocket.DefaultDialer,
		ReadTimeout:  defaultSocketReadTimeout,
		WriteTimeout: defaultSocketWriteTimeout,
	}
}

func (c *WebSocketRelayClient) Publish(ctx context.Context, relayURL string, event Event) (PublishReceipt, error) {
	conn, err := c.dial(ctx, relayURL)
	if err != nil {
		return PublishReceipt{}, err
	}
	defer conn.Close()

	if err := c.writeJSON(conn, []any{"EVENT", event}); err != nil {
		return PublishReceipt{}, err
	}

	for {
		kind, parts, err := c.readFrame(conn)
		if err != nil {
			return PublishReceipt{}, err
		}
		switch kind {
		case "OK":
			if len(parts) < 2 {
				return PublishReceipt{}, fmt.Errorf("nostr relay OK frame is malformed")
			}
			eventID, err := decodeFrameString(parts[0])
			if err != nil {
				return PublishReceipt{}, fmt.Errorf("decode nostr relay OK event id: %w", err)
			}
			accepted, err := decodeFrameBool(parts[1])
			if err != nil {
				return PublishReceipt{}, fmt.Errorf("decode nostr relay OK accepted flag: %w", err)
			}
			message := ""
			if len(parts) > 2 {
				message, _ = decodeFrameString(parts[2])
			}
			return PublishReceipt{
				EventID:  strings.TrimSpace(eventID),
				Accepted: accepted,
				Message:  strings.TrimSpace(message),
			}, nil
		case "NOTICE":
			notice := ""
			if len(parts) > 0 {
				notice, _ = decodeFrameString(parts[0])
			}
			return PublishReceipt{}, fmt.Errorf("nostr relay notice: %s", strings.TrimSpace(notice))
		}
	}
}

func (c *WebSocketRelayClient) Query(ctx context.Context, relayURL string, subscriptionID string, filter Filter) ([]Event, error) {
	subscriptionID = strings.TrimSpace(subscriptionID)
	if subscriptionID == "" {
		return nil, fmt.Errorf("nostr subscription id is required")
	}
	conn, err := c.dial(ctx, relayURL)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := c.writeJSON(conn, []any{"REQ", subscriptionID, filter}); err != nil {
		return nil, err
	}

	events := make([]Event, 0)
	for {
		kind, parts, err := c.readFrame(conn)
		if err != nil {
			return nil, err
		}
		switch kind {
		case "EVENT":
			if len(parts) < 2 {
				return nil, fmt.Errorf("nostr relay EVENT frame is malformed")
			}
			subID, err := decodeFrameString(parts[0])
			if err != nil {
				return nil, fmt.Errorf("decode nostr relay EVENT subscription id: %w", err)
			}
			if strings.TrimSpace(subID) != subscriptionID {
				continue
			}
			var event Event
			if err := json.Unmarshal(parts[1], &event); err != nil {
				return nil, fmt.Errorf("decode nostr relay EVENT payload: %w", err)
			}
			events = append(events, event)
		case "EOSE":
			if len(parts) < 1 {
				return nil, fmt.Errorf("nostr relay EOSE frame is malformed")
			}
			subID, err := decodeFrameString(parts[0])
			if err != nil {
				return nil, fmt.Errorf("decode nostr relay EOSE subscription id: %w", err)
			}
			if strings.TrimSpace(subID) != subscriptionID {
				continue
			}
			_ = c.writeJSON(conn, []any{"CLOSE", subscriptionID})
			return events, nil
		case "NOTICE":
			notice := ""
			if len(parts) > 0 {
				notice, _ = decodeFrameString(parts[0])
			}
			return nil, fmt.Errorf("nostr relay notice: %s", strings.TrimSpace(notice))
		}
	}
}

func (c *WebSocketRelayClient) dial(ctx context.Context, relayURL string) (*websocket.Conn, error) {
	target, err := normalizeRelayURL(relayURL)
	if err != nil {
		return nil, err
	}
	dialer := c.Dialer
	if dialer == nil {
		dialer = websocket.DefaultDialer
	}
	conn, _, err := dialer.DialContext(ctx, target, nil)
	if err != nil {
		return nil, fmt.Errorf("dial nostr relay %q: %w", target, err)
	}
	return conn, nil
}

func (c *WebSocketRelayClient) writeJSON(conn *websocket.Conn, payload any) error {
	if conn == nil {
		return fmt.Errorf("nostr relay connection is nil")
	}
	if err := conn.SetWriteDeadline(time.Now().Add(c.writeTimeout())); err != nil {
		return fmt.Errorf("set nostr relay write deadline: %w", err)
	}
	if err := conn.WriteJSON(payload); err != nil {
		return fmt.Errorf("write nostr relay frame: %w", err)
	}
	return nil
}

func (c *WebSocketRelayClient) readFrame(conn *websocket.Conn) (string, []json.RawMessage, error) {
	if conn == nil {
		return "", nil, fmt.Errorf("nostr relay connection is nil")
	}
	if err := conn.SetReadDeadline(time.Now().Add(c.readTimeout())); err != nil {
		return "", nil, fmt.Errorf("set nostr relay read deadline: %w", err)
	}
	_, payload, err := conn.ReadMessage()
	if err != nil {
		return "", nil, fmt.Errorf("read nostr relay frame: %w", err)
	}
	return decodeRelayFrame(payload)
}

func (c *WebSocketRelayClient) readTimeout() time.Duration {
	if c == nil || c.ReadTimeout <= 0 {
		return defaultSocketReadTimeout
	}
	return c.ReadTimeout
}

func (c *WebSocketRelayClient) writeTimeout() time.Duration {
	if c == nil || c.WriteTimeout <= 0 {
		return defaultSocketWriteTimeout
	}
	return c.WriteTimeout
}

func decodeRelayFrame(payload []byte) (string, []json.RawMessage, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(payload, &raw); err != nil {
		return "", nil, fmt.Errorf("decode nostr relay frame: %w", err)
	}
	if len(raw) == 0 {
		return "", nil, fmt.Errorf("nostr relay frame is empty")
	}
	kind, err := decodeFrameString(raw[0])
	if err != nil {
		return "", nil, fmt.Errorf("decode nostr relay frame kind: %w", err)
	}
	return strings.TrimSpace(kind), raw[1:], nil
}

func decodeFrameString(raw json.RawMessage) (string, error) {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func decodeFrameBool(raw json.RawMessage) (bool, error) {
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, err
	}
	return value, nil
}

func normalizeRelayURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("nostr relay route target is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse nostr relay url: %w", err)
	}
	if strings.TrimSpace(parsed.Host) == "" {
		return "", fmt.Errorf("nostr relay url %q is missing host", raw)
	}
	switch strings.ToLower(strings.TrimSpace(parsed.Scheme)) {
	case "ws", "wss":
	default:
		return "", fmt.Errorf("nostr relay url %q must use ws or wss", raw)
	}
	parsed.Fragment = ""
	return parsed.String(), nil
}
