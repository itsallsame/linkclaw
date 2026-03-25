package registry

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/card"
	_ "modernc.org/sqlite"
)

type Service struct {
	db  *sql.DB
	now func() time.Time
}

func Open(ctx context.Context, dbPath string) (*Service, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("registry db path is required")
	}
	db, err := sql.Open("sqlite", filepath.Clean(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open registry db: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping registry db: %w", err)
	}
	service := &Service{db: db, now: time.Now}
	if err := service.initSchema(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return service, nil
}

func (s *Service) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Service) initSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS registry_agents (
  agent_id TEXT PRIMARY KEY,
  canonical_id TEXT NOT NULL UNIQUE,
  display_name TEXT NOT NULL,
  summary TEXT NOT NULL DEFAULT '',
  capabilities_json TEXT NOT NULL DEFAULT '[]',
  tags_json TEXT NOT NULL DEFAULT '[]',
  identity_card_json TEXT NOT NULL,
  published_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_registry_agents_display_name
  ON registry_agents(display_name);
CREATE INDEX IF NOT EXISTS idx_registry_agents_updated_at
  ON registry_agents(updated_at);
`)
	if err != nil {
		return fmt.Errorf("initialize registry schema: %w", err)
	}
	return nil
}

func (s *Service) Publish(ctx context.Context, req PublishRequest) (AgentRecord, error) {
	nowFn := s.now
	if nowFn == nil {
		nowFn = time.Now
	}
	verified, err := verifyPublishCard(ctx, req.IdentityCard)
	if err != nil {
		return AgentRecord{}, err
	}
	canonicalID := strings.TrimSpace(verified.ID)
	if canonicalID == "" {
		return AgentRecord{}, fmt.Errorf("identity card canonical id is required")
	}
	displayName := strings.TrimSpace(verified.DisplayName)
	if displayName == "" {
		displayName = canonicalID
	}
	agentID := stableAgentID(canonicalID)
	stamp := nowFn().UTC().Format(time.RFC3339Nano)
	capabilities := normalizeStringList(req.Capabilities)
	tags := normalizeStringList(req.Tags)
	cardJSON, err := json.Marshal(verified)
	if err != nil {
		return AgentRecord{}, fmt.Errorf("marshal identity card: %w", err)
	}
	capabilitiesJSON, err := json.Marshal(capabilities)
	if err != nil {
		return AgentRecord{}, fmt.Errorf("marshal capabilities: %w", err)
	}
	tagsJSON, err := json.Marshal(tags)
	if err != nil {
		return AgentRecord{}, fmt.Errorf("marshal tags: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AgentRecord{}, fmt.Errorf("begin registry publish transaction: %w", err)
	}
	defer tx.Rollback()

	var publishedAt string
	err = tx.QueryRowContext(ctx, `SELECT published_at FROM registry_agents WHERE canonical_id = ?`, canonicalID).Scan(&publishedAt)
	switch {
	case err == nil:
	case err == sql.ErrNoRows:
		publishedAt = stamp
	default:
		return AgentRecord{}, fmt.Errorf("lookup existing registry record: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO registry_agents (
  agent_id, canonical_id, display_name, summary, capabilities_json, tags_json, identity_card_json, published_at, updated_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(canonical_id) DO UPDATE SET
  agent_id = excluded.agent_id,
  display_name = excluded.display_name,
  summary = excluded.summary,
  capabilities_json = excluded.capabilities_json,
  tags_json = excluded.tags_json,
  identity_card_json = excluded.identity_card_json,
  updated_at = excluded.updated_at
`, agentID, canonicalID, displayName, strings.TrimSpace(req.Summary), string(capabilitiesJSON), string(tagsJSON), string(cardJSON), publishedAt, stamp); err != nil {
		return AgentRecord{}, fmt.Errorf("upsert registry record: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return AgentRecord{}, fmt.Errorf("commit registry publish transaction: %w", err)
	}

	return AgentRecord{
		AgentID:      agentID,
		CanonicalID:  canonicalID,
		DisplayName:  displayName,
		Summary:      strings.TrimSpace(req.Summary),
		Capabilities: capabilities,
		Tags:         tags,
		IdentityCard: verified,
		PublishedAt:  publishedAt,
		UpdatedAt:    stamp,
	}, nil
}

func (s *Service) Get(ctx context.Context, agentID string) (AgentRecord, bool, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT agent_id, canonical_id, display_name, summary, capabilities_json, tags_json, identity_card_json, published_at, updated_at
FROM registry_agents
WHERE agent_id = ?
`, strings.TrimSpace(agentID))
	record, err := scanRecord(row)
	if err == sql.ErrNoRows {
		return AgentRecord{}, false, nil
	}
	if err != nil {
		return AgentRecord{}, false, err
	}
	return record, true, nil
}

func (s *Service) Search(ctx context.Context, opts SearchOptions) (SearchResult, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT agent_id, canonical_id, display_name, summary, capabilities_json, tags_json, identity_card_json, published_at, updated_at
FROM registry_agents
ORDER BY updated_at DESC, display_name ASC
`)
	if err != nil {
		return SearchResult{}, fmt.Errorf("query registry records: %w", err)
	}
	defer rows.Close()

	query := strings.ToLower(strings.TrimSpace(opts.Query))
	capability := strings.ToLower(strings.TrimSpace(opts.Capability))
	tag := strings.ToLower(strings.TrimSpace(opts.Tag))
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	result := SearchResult{Records: make([]AgentRecord, 0, limit)}
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return SearchResult{}, err
		}
		if !matchesQuery(record, query, capability, tag) {
			continue
		}
		result.Records = append(result.Records, record)
		if len(result.Records) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return SearchResult{}, fmt.Errorf("iterate registry records: %w", err)
	}
	return result, nil
}

func scanRecord(scanner interface{ Scan(dest ...any) error }) (AgentRecord, error) {
	var record AgentRecord
	var capabilitiesJSON, tagsJSON, cardJSON string
	if err := scanner.Scan(
		&record.AgentID,
		&record.CanonicalID,
		&record.DisplayName,
		&record.Summary,
		&capabilitiesJSON,
		&tagsJSON,
		&cardJSON,
		&record.PublishedAt,
		&record.UpdatedAt,
	); err != nil {
		return AgentRecord{}, err
	}
	if err := json.Unmarshal([]byte(capabilitiesJSON), &record.Capabilities); err != nil {
		return AgentRecord{}, fmt.Errorf("decode registry capabilities: %w", err)
	}
	if err := json.Unmarshal([]byte(tagsJSON), &record.Tags); err != nil {
		return AgentRecord{}, fmt.Errorf("decode registry tags: %w", err)
	}
	if err := json.Unmarshal([]byte(cardJSON), &record.IdentityCard); err != nil {
		return AgentRecord{}, fmt.Errorf("decode registry card: %w", err)
	}
	return record, nil
}

func verifyPublishCard(ctx context.Context, input card.Card) (card.Card, error) {
	payload, err := json.Marshal(input)
	if err != nil {
		return card.Card{}, fmt.Errorf("encode publish card: %w", err)
	}
	service := card.NewService()
	result, err := service.Verify(ctx, card.VerifyOptions{Input: string(payload)})
	if err != nil {
		return card.Card{}, fmt.Errorf("verify published identity card: %w", err)
	}
	return result.Card, nil
}

func stableAgentID(canonicalID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(canonicalID)))
	return "agent_" + hex.EncodeToString(sum[:6])
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		normalized = append(normalized, trimmed)
	}
	slices.Sort(normalized)
	return slices.Compact(normalized)
}

func matchesQuery(record AgentRecord, query, capability, tag string) bool {
	if capability != "" {
		found := false
		for _, value := range record.Capabilities {
			if strings.Contains(strings.ToLower(value), capability) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if tag != "" {
		found := false
		for _, value := range record.Tags {
			if strings.Contains(strings.ToLower(value), tag) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if query == "" {
		return true
	}
	haystacks := []string{
		record.AgentID,
		record.DisplayName,
		record.CanonicalID,
		record.Summary,
		strings.Join(record.Capabilities, " "),
		strings.Join(record.Tags, " "),
	}
	for _, haystack := range haystacks {
		if strings.Contains(strings.ToLower(haystack), query) {
			return true
		}
	}
	return false
}
