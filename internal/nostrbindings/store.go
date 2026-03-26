package nostrbindings

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

const CapabilityNostr = "nostr"

type Snapshot struct {
	RelayURLs        []string
	PublicKeys       []string
	PrimaryPublicKey string
}

func (s Snapshot) HasCapability() bool {
	return len(s.RelayURLs) > 0 || len(s.PublicKeys) > 0 || strings.TrimSpace(s.PrimaryPublicKey) != ""
}

func LoadSelfSnapshot(ctx context.Context, db *sql.DB, selfID string) (Snapshot, error) {
	collector := newCollector()

	if err := loadBindings(ctx, db, strings.TrimSpace(selfID), collector); err != nil {
		return Snapshot{}, err
	}
	if err := loadRelays(ctx, db, collector); err != nil {
		return Snapshot{}, err
	}

	return collector.snapshot(), nil
}

func loadBindings(ctx context.Context, db *sql.DB, selfID string, collector *bindingCollector) error {
	if selfID == "" {
		return nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT relay_url, metadata_json
		FROM runtime_transport_bindings
		WHERE self_id = ? AND transport = 'nostr' AND enabled = 1
		ORDER BY updated_at DESC, relay_url ASC, binding_id ASC
	`, selfID)
	if err != nil {
		if isMissingTable(err) {
			return nil
		}
		return fmt.Errorf("query runtime transport bindings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var relayURL, metadataJSON string
		if err := rows.Scan(&relayURL, &metadataJSON); err != nil {
			return fmt.Errorf("scan runtime transport binding: %w", err)
		}
		collector.addRelay(relayURL)
		collector.addMetadata(metadataJSON)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate runtime transport bindings: %w", err)
	}
	return nil
}

func loadRelays(ctx context.Context, db *sql.DB, collector *bindingCollector) error {
	rows, err := db.QueryContext(ctx, `
		SELECT relay_url, metadata_json
		FROM runtime_transport_relays
		WHERE transport = 'nostr' AND status = 'active' AND (read_enabled = 1 OR write_enabled = 1)
		ORDER BY priority DESC, relay_url ASC, relay_id ASC
	`)
	if err != nil {
		if isMissingTable(err) {
			return nil
		}
		return fmt.Errorf("query runtime transport relays: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var relayURL, metadataJSON string
		if err := rows.Scan(&relayURL, &metadataJSON); err != nil {
			return fmt.Errorf("scan runtime transport relay: %w", err)
		}
		collector.addRelay(relayURL)
		collector.addMetadata(metadataJSON)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate runtime transport relays: %w", err)
	}
	return nil
}

type bindingCollector struct {
	relayURLs     []string
	relaySet      map[string]struct{}
	publicKeys    []string
	publicKeySet  map[string]struct{}
	primaryPubKey string
}

func newCollector() *bindingCollector {
	return &bindingCollector{
		relayURLs:    make([]string, 0),
		relaySet:     make(map[string]struct{}),
		publicKeys:   make([]string, 0),
		publicKeySet: make(map[string]struct{}),
	}
}

func (c *bindingCollector) snapshot() Snapshot {
	primary := strings.TrimSpace(c.primaryPubKey)
	if primary == "" && len(c.publicKeys) > 0 {
		primary = c.publicKeys[0]
	}
	if primary != "" {
		c.addPublicKey(primary)
	}
	return Snapshot{
		RelayURLs:        append([]string(nil), c.relayURLs...),
		PublicKeys:       append([]string(nil), c.publicKeys...),
		PrimaryPublicKey: primary,
	}
}

func (c *bindingCollector) addRelay(values ...string) {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := c.relaySet[trimmed]; exists {
			continue
		}
		c.relaySet[trimmed] = struct{}{}
		c.relayURLs = append(c.relayURLs, trimmed)
	}
}

func (c *bindingCollector) addPublicKey(values ...string) {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := c.publicKeySet[trimmed]; exists {
			continue
		}
		c.publicKeySet[trimmed] = struct{}{}
		c.publicKeys = append(c.publicKeys, trimmed)
	}
}

func (c *bindingCollector) addMetadata(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return
	}
	c.consumeMetadataMap(payload)
	if nested, ok := payload["nostr"].(map[string]any); ok {
		c.consumeMetadataMap(nested)
	}
}

func (c *bindingCollector) consumeMetadataMap(payload map[string]any) {
	if payload == nil {
		return
	}
	c.addRelay(extractStrings(payload["relay_urls"])...)
	c.addRelay(extractStrings(payload["relay_url"])...)
	c.addPublicKey(extractStrings(payload["nostr_public_keys"])...)
	c.addPublicKey(extractStrings(payload["nostr_public_key"])...)

	if primary := firstString(payload["nostr_primary_public_key"]); primary != "" {
		c.primaryPubKey = primary
		c.addPublicKey(primary)
	}
}

func extractStrings(raw any) []string {
	switch value := raw.(type) {
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return nil
		}
		return []string{trimmed}
	case []string:
		items := make([]string, 0, len(value))
		for _, item := range value {
			trimmed := strings.TrimSpace(item)
			if trimmed == "" {
				continue
			}
			items = append(items, trimmed)
		}
		return items
	case []any:
		items := make([]string, 0, len(value))
		for _, item := range value {
			if str, ok := item.(string); ok {
				trimmed := strings.TrimSpace(str)
				if trimmed == "" {
					continue
				}
				items = append(items, trimmed)
			}
		}
		return items
	default:
		return nil
	}
}

func firstString(raw any) string {
	values := extractStrings(raw)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func isMissingTable(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "no such table")
}
