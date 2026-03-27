package nostr

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
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
	signer    EventSigner
	now       func() time.Time
	eventKind int
	pullLimit int
}

func NewBackend(client RelayClient) *Backend {
	return NewBackendWithSigner(client, nil)
}

func NewBackendWithSigner(client RelayClient, signer EventSigner) *Backend {
	if client == nil {
		client = NewWebSocketRelayClient()
	}
	return &Backend{
		client:    client,
		signer:    signer,
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
	if strings.TrimSpace(config.Recipient) == "" {
		return transport.SendResult{}, fmt.Errorf("nostr recipient public key is required in route target")
	}

	event, err := b.buildEvent(env, config.Recipient, config.Sender)
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

func (b *Backend) buildEvent(env transport.Envelope, routeRecipient string, routeSender string) (Event, error) {
	recipient := strings.TrimSpace(routeRecipient)
	if recipient == "" {
		return Event{}, fmt.Errorf("nostr route recipient is required")
	}
	ciphertext := strings.TrimSpace(env.Ciphertext)
	if ciphertext == "" {
		return Event{}, fmt.Errorf("nostr ciphertext payload is required")
	}
	senderPubKey := firstNonEmpty(strings.TrimSpace(routeSender), strings.TrimSpace(env.SenderTransportID), recipient)
	if b != nil && b.signer != nil {
		if signerPubKey := strings.TrimSpace(b.signer.PublicKey()); signerPubKey != "" {
			senderPubKey = signerPubKey
		}
	}

	payload := map[string]string{}
	setPayloadField(payload, "message_id", env.MessageID)
	setPayloadField(payload, "sender_pubkey", senderPubKey)
	setPayloadField(payload, "recipient_pubkey", recipient)
	setPayloadField(payload, "sender_signing_key", env.SenderSigningKey)
	setPayloadField(payload, "ephemeral_public_key", env.EphemeralPublicKey)
	setPayloadField(payload, "nonce", env.Nonce)
	setPayloadField(payload, "ciphertext", ciphertext)
	setPayloadField(payload, "signature", env.Signature)
	setPayloadField(payload, "sent_at", env.SentAt)
	content, err := json.Marshal(payload)
	if err != nil {
		return Event{}, fmt.Errorf("encode nostr relay payload: %w", err)
	}

	event := Event{
		PubKey:    senderPubKey,
		CreatedAt: b.nowUTC().Unix(),
		Kind:      b.eventKindValue(),
		Tags:      buildEventTags(env, recipient),
		Content:   string(content),
	}
	event.ID = ComputeEventID(event)
	event.Sig, err = b.signEvent(event.ID, env.Signature)
	if err != nil {
		return Event{}, err
	}
	return event, nil
}

func buildEventTags(env transport.Envelope, routeRecipient string) [][]string {
	tags := make([][]string, 0, 2)

	recipient := firstNonEmpty(strings.TrimSpace(routeRecipient), strings.TrimSpace(env.RecipientID))
	if recipient != "" {
		tags = append(tags, []string{"p", recipient})
	}
	if messageID := strings.TrimSpace(env.MessageID); messageID != "" {
		tags = append(tags, []string{"linkclaw_message_id", messageID})
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
	Sender    string
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
		Sender:    strings.TrimSpace(query.Get("sender")),
	}
	if cfg.Recipient == "" {
		cfg.Recipient = strings.TrimSpace(query.Get("p"))
	}
	if cfg.Sender == "" {
		cfg.Sender = strings.TrimSpace(query.Get("pubkey"))
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

func setPayloadField(payload map[string]string, key string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	payload[key] = value
}

func (b *Backend) signEvent(eventID string, envelopeSignature string) (string, error) {
	if b != nil && b.signer != nil {
		signature, err := b.signer.SignEventID(eventID)
		if err != nil {
			return "", fmt.Errorf("sign nostr event: %w", err)
		}
		signature = strings.ToLower(strings.TrimSpace(signature))
		if !isStrictHexSignature(signature) {
			return "", fmt.Errorf("nostr signer returned invalid signature")
		}
		return signature, nil
	}
	if signature, ok := decodeEnvelopeSignature(envelopeSignature); ok {
		return signature, nil
	}
	return "", fmt.Errorf("nostr event signer is required")
}

func decodeEnvelopeSignature(envelopeSignature string) (string, bool) {
	envelopeSignature = strings.TrimSpace(envelopeSignature)
	if envelopeSignature != "" {
		if decoded, err := base64.RawStdEncoding.DecodeString(envelopeSignature); err == nil && len(decoded) == 64 {
			return hex.EncodeToString(decoded), true
		}
		normalized := strings.ToLower(strings.TrimSpace(envelopeSignature))
		if isStrictHexSignature(normalized) {
			return normalized, true
		}
	}
	return "", false
}

func isStrictHexSignature(raw string) bool {
	if len(raw) != 128 {
		return false
	}
	for _, ch := range raw {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
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
