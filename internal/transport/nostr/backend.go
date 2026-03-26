package nostr

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

const (
	defaultEventKind = 4
	defaultPullLimit = 50
)

type Backend struct {
	client    RelayClient
	now       func() time.Time
	eventKind int
	pullLimit int
}

func NewBackend(client RelayClient) *Backend {
	if client == nil {
		client = NewWebSocketRelayClient()
	}
	return &Backend{
		client:    client,
		now:       time.Now,
		eventKind: defaultEventKind,
		pullLimit: defaultPullLimit,
	}
}

func (b *Backend) Publish(ctx context.Context, env transport.Envelope, route transport.RouteCandidate) (transport.SendResult, error) {
	if route.Type != transport.RouteTypeNostr {
		return transport.SendResult{}, fmt.Errorf("unsupported route type %q", route.Type)
	}
	config, err := parseRouteConfig(route.Target)
	if err != nil {
		return transport.SendResult{}, err
	}

	event, err := b.buildEvent(env, config.Recipient)
	if err != nil {
		return transport.SendResult{}, err
	}
	receipt, err := b.client.Publish(ctx, config.RelayURL, event)
	if err != nil {
		return transport.SendResult{}, err
	}
	if !receipt.Accepted {
		reason := strings.TrimSpace(receipt.Message)
		if reason == "" {
			reason = "nostr relay rejected publish request"
		}
		return transport.SendResult{}, fmt.Errorf("%s", reason)
	}

	remoteID := firstNonEmpty(strings.TrimSpace(receipt.EventID), event.ID, strings.TrimSpace(env.MessageID))
	return transport.SendResult{
		Route:       route,
		RemoteID:    remoteID,
		Delivered:   true,
		Retryable:   false,
		Description: "published to nostr relay",
	}, nil
}

func (b *Backend) Recover(ctx context.Context, route transport.RouteCandidate) (transport.SyncResult, error) {
	if route.Type != transport.RouteTypeNostr {
		return transport.SyncResult{}, fmt.Errorf("unsupported route type %q", route.Type)
	}
	config, err := parseRouteConfig(route.Target)
	if err != nil {
		return transport.SyncResult{}, err
	}

	filter := Filter{
		Kinds: []int{b.eventKindValue()},
		Limit: b.pullLimitValue(),
	}
	if config.Limit > 0 {
		filter.Limit = config.Limit
	}
	if config.Since != nil {
		filter.Since = config.Since
	}
	if config.Recipient != "" {
		filter.Recipient = []string{config.Recipient}
	}

	subID := buildSubscriptionID(route.Target, b.nowUTC())
	events, err := b.client.Query(ctx, config.RelayURL, subID, filter)
	if err != nil {
		return transport.SyncResult{}, err
	}

	maxCreatedAt := int64(0)
	for _, event := range events {
		if event.CreatedAt > maxCreatedAt {
			maxCreatedAt = event.CreatedAt
		}
	}
	advancedCursor := ""
	if maxCreatedAt > 0 {
		advancedCursor = strconv.FormatInt(maxCreatedAt, 10)
	}

	return transport.SyncResult{
		Route:          route,
		Recovered:      len(events),
		AdvancedCursor: advancedCursor,
	}, nil
}

func (b *Backend) Acknowledge(_ context.Context, route transport.RouteCandidate, _ string) error {
	if route.Type != transport.RouteTypeNostr {
		return fmt.Errorf("unsupported route type %q", route.Type)
	}
	_, err := parseRouteConfig(route.Target)
	return err
}

func (b *Backend) buildEvent(env transport.Envelope, routeRecipient string) (Event, error) {
	payload := map[string]string{
		"message_id":   strings.TrimSpace(env.MessageID),
		"sender_id":    strings.TrimSpace(env.SenderID),
		"recipient_id": strings.TrimSpace(env.RecipientID),
		"plaintext":    env.Plaintext,
		"ciphertext":   env.Ciphertext,
	}
	content, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("encode nostr relay payload: %w", err)
	}

	event := Event{
		PubKey:    strings.TrimSpace(env.SenderID),
		CreatedAt: b.nowUTC().Unix(),
		Kind:      b.eventKindValue(),
		Tags:      buildEventTags(env, routeRecipient),
		Content:   string(content),
	}
	event.ID = ComputeEventID(event)
	return event, nil
}

func buildEventTags(env transport.Envelope, routeRecipient string) [][]string {
	tags := make([][]string, 0, 4)

	recipient := firstNonEmpty(strings.TrimSpace(routeRecipient), strings.TrimSpace(env.RecipientID))
	if recipient != "" {
		tags = append(tags, []string{"p", recipient})
	}
	if messageID := strings.TrimSpace(env.MessageID); messageID != "" {
		tags = append(tags, []string{"linkclaw_message_id", messageID})
	}
	if sender := strings.TrimSpace(env.SenderID); sender != "" {
		tags = append(tags, []string{"linkclaw_sender", sender})
	}
	if rawRecipient := strings.TrimSpace(env.RecipientID); rawRecipient != "" {
		tags = append(tags, []string{"linkclaw_recipient", rawRecipient})
	}
	return tags
}

func ComputeEventID(event Event) string {
	tags := event.Tags
	if tags == nil {
		tags = [][]string{}
	}
	payload := []any{
		0,
		strings.TrimSpace(event.PubKey),
		event.CreatedAt,
		event.Kind,
		tags,
		event.Content,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

type routeConfig struct {
	RelayURL  string
	Recipient string
	Since     *int64
	Limit     int
}

func parseRouteConfig(raw string) (routeConfig, error) {
	normalizedURL, err := normalizeRelayURL(raw)
	if err != nil {
		return routeConfig{}, err
	}
	parsed, err := url.Parse(normalizedURL)
	if err != nil {
		return routeConfig{}, fmt.Errorf("parse nostr route config: %w", err)
	}
	query := parsed.Query()
	parsed.RawQuery = ""
	parsed.Fragment = ""
	cfg := routeConfig{
		RelayURL:  parsed.String(),
		Recipient: strings.TrimSpace(query.Get("recipient")),
	}
	if cfg.Recipient == "" {
		cfg.Recipient = strings.TrimSpace(query.Get("p"))
	}

	for _, key := range []string{"since", "cursor"} {
		value := strings.TrimSpace(query.Get(key))
		if value == "" {
			continue
		}
		since, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return routeConfig{}, fmt.Errorf("parse nostr %s value %q: %w", key, value, err)
		}
		cfg.Since = &since
		break
	}

	limitRaw := strings.TrimSpace(query.Get("limit"))
	if limitRaw != "" {
		limit, err := strconv.Atoi(limitRaw)
		if err != nil || limit < 0 {
			return routeConfig{}, fmt.Errorf("parse nostr limit value %q", limitRaw)
		}
		cfg.Limit = limit
	}

	return cfg, nil
}

func buildSubscriptionID(rawRoute string, now time.Time) string {
	seed := strings.TrimSpace(rawRoute) + "|" + now.UTC().Format(time.RFC3339Nano)
	sum := sha256.Sum256([]byte(seed))
	return "linkclaw-" + hex.EncodeToString(sum[:6])
}

func (b *Backend) eventKindValue() int {
	if b == nil || b.eventKind <= 0 {
		return defaultEventKind
	}
	return b.eventKind
}

func (b *Backend) pullLimitValue() int {
	if b == nil || b.pullLimit <= 0 {
		return defaultPullLimit
	}
	return b.pullLimit
}

func (b *Backend) nowUTC() time.Time {
	if b == nil || b.now == nil {
		return time.Now().UTC()
	}
	return b.now().UTC()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
