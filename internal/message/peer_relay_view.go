package message

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	agentdiscovery "github.com/xiewanpeng/claw-identity/internal/discovery"
	agentruntime "github.com/xiewanpeng/claw-identity/internal/runtime"
	"github.com/xiewanpeng/claw-identity/internal/transport"
)

const (
	relaySourceManualOverride = "manual_override"
	relaySourceDiscovery      = "discovery_runtime"
	relaySourceCardPublish    = "card_publish"
	relaySourceRegistry       = "registry"
	relaySourceDefaultPublic  = "default_public_relay"
)

type peerRelayPubKeyView struct {
	CanonicalID                 string
	RelayURLs                   []string
	StoreForwardRelayURLs       []string
	NostrRelayURLs              []string
	NostrPublicKeys             []string
	NostrPrimaryPublicKey       string
	EffectiveStoreForwardRelay  string
	EffectiveStoreForwardSource string
}

type peerRelaySourceFact struct {
	RelayURLs        []string
	PublicKeys       []string
	PrimaryPublicKey string
}

type namedRelaySourceFact struct {
	Source string
	Fact   peerRelaySourceFact
}

func resolvePeerRelayPubKeyView(
	ctx context.Context,
	db *sql.DB,
	selfID string,
	canonicalID string,
	manualRelayURL string,
	now time.Time,
) (peerRelayPubKeyView, error) {
	canonicalID = strings.TrimSpace(canonicalID)
	if canonicalID == "" {
		return peerRelayPubKeyView{}, fmt.Errorf("canonical_id is required for relay/pubkey aggregation")
	}
	now = now.UTC()

	manualFact := peerRelaySourceFact{}
	manualFact.addRelay(manualRelayURL)

	discoveryFact, err := loadDiscoveryRuntimeRelayFact(ctx, db, selfID, canonicalID, now)
	if err != nil {
		return peerRelayPubKeyView{}, err
	}
	cardFact, err := loadCardPublishRelayFact(ctx, db, canonicalID)
	if err != nil {
		return peerRelayPubKeyView{}, err
	}
	registryFact, err := loadRegistryRelayFact(ctx, db, canonicalID)
	if err != nil {
		return peerRelayPubKeyView{}, err
	}
	defaultFact, err := loadDefaultPublicRelayFact(ctx, db, now)
	if err != nil {
		return peerRelayPubKeyView{}, err
	}

	return mergePeerRelayFacts(canonicalID, []namedRelaySourceFact{
		{Source: relaySourceManualOverride, Fact: manualFact},
		{Source: relaySourceDiscovery, Fact: discoveryFact},
		{Source: relaySourceCardPublish, Fact: cardFact},
		{Source: relaySourceRegistry, Fact: registryFact},
		{Source: relaySourceDefaultPublic, Fact: defaultFact},
	}), nil
}

func loadDiscoveryRuntimeRelayFact(
	ctx context.Context,
	db *sql.DB,
	selfID string,
	canonicalID string,
	now time.Time,
) (peerRelaySourceFact, error) {
	fact := peerRelaySourceFact{}
	store := agentdiscovery.NewStoreWithDB(db, now.UTC())
	record, ok, err := store.Get(ctx, canonicalID)
	if err != nil {
		return fact, err
	}
	if ok {
		fact.addRelay(record.StoreForwardHints...)
		for _, route := range record.RouteCandidates {
			if route.Type != transport.RouteTypeStoreForward {
				continue
			}
			fact.addRelay(route.Target, route.Label)
		}
	}

	runtimeStore := agentruntime.NewStoreWithDB(db, now.UTC())
	bindings, err := runtimeStore.ListTransportBindings(ctx, strings.TrimSpace(selfID))
	if err != nil {
		return fact, err
	}
	for _, binding := range bindings {
		if strings.TrimSpace(binding.CanonicalID) != canonicalID {
			continue
		}
		if strings.TrimSpace(binding.Transport) != string(transport.RouteTypeNostr) {
			continue
		}
		if !binding.Enabled {
			continue
		}
		fact.addRelay(binding.RelayURL)
		fact.consumeMetadata(binding.MetadataJSON)
	}
	return fact, nil
}

func loadCardPublishRelayFact(ctx context.Context, db *sql.DB, canonicalID string) (peerRelaySourceFact, error) {
	row := db.QueryRowContext(ctx, `
		SELECT raw_identity_card_json
		FROM contacts
		WHERE canonical_id = ?
		ORDER BY created_at DESC
		LIMIT 1
	`, canonicalID)
	var rawCard string
	if err := row.Scan(&rawCard); err != nil {
		if err == sql.ErrNoRows {
			return peerRelaySourceFact{}, nil
		}
		return peerRelaySourceFact{}, fmt.Errorf("query contact card for relay/pubkey aggregation: %w", err)
	}
	return parseIdentityCardRelayFact(rawCard), nil
}

func loadRegistryRelayFact(ctx context.Context, db *sql.DB, canonicalID string) (peerRelaySourceFact, error) {
	row := db.QueryRowContext(ctx, `
		SELECT identity_card_json
		FROM registry_agents
		WHERE canonical_id = ?
		ORDER BY updated_at DESC
		LIMIT 1
	`, canonicalID)
	var rawCard string
	if err := row.Scan(&rawCard); err != nil {
		if err == sql.ErrNoRows || isMissingTableError(err) {
			return peerRelaySourceFact{}, nil
		}
		return peerRelaySourceFact{}, fmt.Errorf("query registry card for relay/pubkey aggregation: %w", err)
	}
	return parseIdentityCardRelayFact(rawCard), nil
}

func loadDefaultPublicRelayFact(ctx context.Context, db *sql.DB, now time.Time) (peerRelaySourceFact, error) {
	fact := peerRelaySourceFact{}
	store := agentruntime.NewStoreWithDB(db, now.UTC())
	relays, err := store.ListTransportRelays(ctx, string(transport.RouteTypeNostr))
	if err != nil {
		if isMissingTableError(err) {
			return fact, nil
		}
		return fact, err
	}
	for _, relay := range relays {
		if strings.TrimSpace(relay.Transport) != string(transport.RouteTypeNostr) {
			continue
		}
		if strings.TrimSpace(relay.Status) != "active" {
			continue
		}
		if !relay.ReadEnabled && !relay.WriteEnabled {
			continue
		}
		fact.addRelay(relay.RelayURL)
		fact.consumeMetadata(relay.MetadataJSON)
	}
	return fact, nil
}

func parseIdentityCardRelayFact(rawCard string) peerRelaySourceFact {
	rawCard = strings.TrimSpace(rawCard)
	if rawCard == "" {
		return peerRelaySourceFact{}
	}
	var payload struct {
		RelayURLs             []string `json:"relay_urls"`
		NostrPublicKeys       []string `json:"nostr_public_keys"`
		NostrPrimaryPublicKey string   `json:"nostr_primary_public_key"`
	}
	if err := json.Unmarshal([]byte(rawCard), &payload); err != nil {
		return peerRelaySourceFact{}
	}
	fact := peerRelaySourceFact{}
	fact.addRelay(payload.RelayURLs...)
	fact.addPublicKey(payload.NostrPublicKeys...)
	fact.PrimaryPublicKey = strings.TrimSpace(payload.NostrPrimaryPublicKey)
	if fact.PrimaryPublicKey != "" {
		fact.addPublicKey(fact.PrimaryPublicKey)
	}
	return fact
}

func mergePeerRelayFacts(canonicalID string, sources []namedRelaySourceFact) peerRelayPubKeyView {
	view := peerRelayPubKeyView{
		CanonicalID: strings.TrimSpace(canonicalID),
	}
	relaySeen := map[string]struct{}{}
	storeForwardSeen := map[string]struct{}{}
	nostrRelaySeen := map[string]struct{}{}
	publicKeySeen := map[string]struct{}{}

	for _, source := range sources {
		for _, relayURL := range source.Fact.RelayURLs {
			relayURL = strings.TrimSpace(relayURL)
			if relayURL == "" {
				continue
			}
			if _, ok := relaySeen[relayURL]; !ok {
				relaySeen[relayURL] = struct{}{}
				view.RelayURLs = append(view.RelayURLs, relayURL)
			}
			switch relayURLKind(relayURL) {
			case relayKindNostr:
				if _, ok := nostrRelaySeen[relayURL]; !ok {
					nostrRelaySeen[relayURL] = struct{}{}
					view.NostrRelayURLs = append(view.NostrRelayURLs, relayURL)
				}
			default:
				if _, ok := storeForwardSeen[relayURL]; !ok {
					storeForwardSeen[relayURL] = struct{}{}
					view.StoreForwardRelayURLs = append(view.StoreForwardRelayURLs, relayURL)
				}
			}
		}
		for _, publicKey := range source.Fact.PublicKeys {
			publicKey = strings.TrimSpace(publicKey)
			if publicKey == "" {
				continue
			}
			if _, ok := publicKeySeen[publicKey]; ok {
				continue
			}
			publicKeySeen[publicKey] = struct{}{}
			view.NostrPublicKeys = append(view.NostrPublicKeys, publicKey)
		}
	}

	for _, source := range sources {
		if candidate := strings.TrimSpace(source.Fact.PrimaryPublicKey); candidate != "" {
			view.NostrPrimaryPublicKey = candidate
			break
		}
	}
	if view.NostrPrimaryPublicKey == "" && len(view.NostrPublicKeys) > 0 {
		view.NostrPrimaryPublicKey = view.NostrPublicKeys[0]
	}
	if view.NostrPrimaryPublicKey != "" {
		if _, ok := publicKeySeen[view.NostrPrimaryPublicKey]; !ok {
			view.NostrPublicKeys = append(view.NostrPublicKeys, view.NostrPrimaryPublicKey)
		}
	}

	for _, source := range sources {
		if selected := firstStoreForwardRelay(source.Fact.RelayURLs); selected != "" {
			view.EffectiveStoreForwardRelay = selected
			view.EffectiveStoreForwardSource = source.Source
			break
		}
	}

	return view
}

func applyRelayViewToContact(contact contactRecord, relayView peerRelayPubKeyView) contactRecord {
	updated := contact
	relayURL := strings.TrimSpace(updated.RelayURL)
	if relayURL == "" {
		relayURL = strings.TrimSpace(relayView.EffectiveStoreForwardRelay)
	}
	updated.RelayURL = relayURL

	storeForwardHints := make([]string, 0, len(relayView.StoreForwardRelayURLs)+1)
	for _, relay := range relayView.StoreForwardRelayURLs {
		if isStoreForwardRelayURL(relay) {
			storeForwardHints = appendIfMissing(storeForwardHints, relay)
		}
	}
	if isStoreForwardRelayURL(relayURL) {
		storeForwardHints = appendIfMissing(storeForwardHints, relayURL)
	}
	updated.StoreForwardHints = storeForwardHints

	nostrHints := make([]string, 0, len(relayView.NostrRelayURLs))
	for _, relay := range relayView.NostrRelayURLs {
		if relayURLKind(relay) == relayKindNostr {
			nostrHints = appendIfMissing(nostrHints, relay)
		}
	}
	if relayURLKind(relayURL) == relayKindNostr {
		nostrHints = appendIfMissing(nostrHints, relayURL)
	}
	updated.NostrRelayHints = nostrHints
	return updated
}

func applyRelayViewToDiscoveryRecord(record agentdiscovery.Record, relayView peerRelayPubKeyView) agentdiscovery.Record {
	updated := record
	storeForwardHints := make([]string, 0, len(relayView.StoreForwardRelayURLs)+len(updated.StoreForwardHints))
	for _, relay := range relayView.StoreForwardRelayURLs {
		if isStoreForwardRelayURL(relay) {
			storeForwardHints = appendIfMissing(storeForwardHints, relay)
		}
	}
	for _, relay := range updated.StoreForwardHints {
		if isStoreForwardRelayURL(relay) {
			storeForwardHints = appendIfMissing(storeForwardHints, relay)
		}
	}
	updated.StoreForwardHints = storeForwardHints

	updated.RouteCandidates = appendHintsToRoutes(updated.RouteCandidates, nil, updated.StoreForwardHints)
	if len(updated.StoreForwardHints) > 0 {
		updated.TransportCapabilities = appendIfMissing(updated.TransportCapabilities, string(transport.RouteTypeStoreForward))
	}
	return updated
}

func storeForwardTargetsFromContact(contact contactRecord) []string {
	targets := make([]string, 0, len(contact.StoreForwardHints)+1)
	for _, hint := range contact.StoreForwardHints {
		if isStoreForwardRelayURL(hint) {
			targets = appendIfMissing(targets, hint)
		}
	}
	if isStoreForwardRelayURL(contact.RelayURL) {
		targets = appendIfMissing(targets, contact.RelayURL)
	}
	return targets
}

func nostrTargetsFromContact(contact contactRecord) []string {
	targets := make([]string, 0, len(contact.NostrRelayHints)+1)
	for _, hint := range contact.NostrRelayHints {
		if relayURLKind(hint) == relayKindNostr {
			targets = appendIfMissing(targets, hint)
		}
	}
	if relayURLKind(contact.RelayURL) == relayKindNostr {
		targets = appendIfMissing(targets, contact.RelayURL)
	}
	return targets
}

func firstStoreForwardRelay(values []string) string {
	for _, value := range values {
		if isStoreForwardRelayURL(value) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

type relayKind string

const (
	relayKindStoreForward relayKind = "store_forward"
	relayKindNostr        relayKind = "nostr"
)

func relayURLKind(raw string) relayKind {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return relayKindStoreForward
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		lower := strings.ToLower(raw)
		if strings.HasPrefix(lower, "ws://") || strings.HasPrefix(lower, "wss://") {
			return relayKindNostr
		}
		return relayKindStoreForward
	}
	switch strings.ToLower(strings.TrimSpace(parsed.Scheme)) {
	case "ws", "wss":
		return relayKindNostr
	default:
		// Keep legacy compatibility: unknown schemes still flow through store-forward path.
		return relayKindStoreForward
	}
}

func isStoreForwardRelayURL(relayURL string) bool {
	return relayURLKind(relayURL) == relayKindStoreForward
}

func (f *peerRelaySourceFact) addRelay(values ...string) {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		exists := false
		for _, current := range f.RelayURLs {
			if current == value {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		f.RelayURLs = append(f.RelayURLs, value)
	}
}

func (f *peerRelaySourceFact) addPublicKey(values ...string) {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		exists := false
		for _, current := range f.PublicKeys {
			if current == value {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		f.PublicKeys = append(f.PublicKeys, value)
	}
}

func (f *peerRelaySourceFact) consumeMetadata(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return
	}
	f.consumeMetadataMap(payload)
	if nested, ok := payload["nostr"].(map[string]any); ok {
		f.consumeMetadataMap(nested)
	}
}

func (f *peerRelaySourceFact) consumeMetadataMap(payload map[string]any) {
	if payload == nil {
		return
	}
	f.addRelay(extractMetadataStrings(payload["relay_urls"])...)
	f.addRelay(extractMetadataStrings(payload["relay_url"])...)
	f.addPublicKey(extractMetadataStrings(payload["nostr_public_keys"])...)
	f.addPublicKey(extractMetadataStrings(payload["nostr_public_key"])...)

	if primary := firstMetadataString(payload["nostr_primary_public_key"]); primary != "" {
		f.PrimaryPublicKey = primary
		f.addPublicKey(primary)
	}
}

func extractMetadataStrings(raw any) []string {
	switch value := raw.(type) {
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		return []string{value}
	case []string:
		out := make([]string, 0, len(value))
		for _, item := range value {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			out = append(out, item)
		}
		return out
	case []any:
		out := make([]string, 0, len(value))
		for _, item := range value {
			str, ok := item.(string)
			if !ok {
				continue
			}
			str = strings.TrimSpace(str)
			if str == "" {
				continue
			}
			out = append(out, str)
		}
		return out
	default:
		return nil
	}
}

func firstMetadataString(raw any) string {
	values := extractMetadataStrings(raw)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func isMissingTableError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "no such table")
}
