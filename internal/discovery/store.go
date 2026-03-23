package discovery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

type Record struct {
	CanonicalID           string
	PeerID                string
	RouteCandidates       []transport.RouteCandidate
	TransportCapabilities []string
	DirectHints           []string
	StoreForwardHints     []string
	SignedPeerRecord      string
	Source                string
	Reachable             bool
	ResolvedAt            string
	FreshUntil            string
	AnnouncedAt           string
	UpdatedAt             string
	CreatedAt             string
}

func NewStoreWithDB(db *sql.DB, now time.Time) *Store {
	return &Store{
		db:  db,
		now: func() time.Time { return now.UTC() },
	}
}

func (s *Store) Upsert(ctx context.Context, record Record) error {
	canonicalID := strings.TrimSpace(record.CanonicalID)
	if canonicalID == "" {
		return fmt.Errorf("discovery canonical_id is required")
	}
	routeCandidatesJSON, err := json.Marshal(record.RouteCandidates)
	if err != nil {
		return fmt.Errorf("marshal discovery route candidates: %w", err)
	}
	transportCapsJSON, err := json.Marshal(normalizeStringList(record.TransportCapabilities))
	if err != nil {
		return fmt.Errorf("marshal discovery transport capabilities: %w", err)
	}
	directHintsJSON, err := json.Marshal(normalizeStringList(record.DirectHints))
	if err != nil {
		return fmt.Errorf("marshal discovery direct hints: %w", err)
	}
	storeForwardHintsJSON, err := json.Marshal(normalizeStringList(record.StoreForwardHints))
	if err != nil {
		return fmt.Errorf("marshal discovery store-forward hints: %w", err)
	}
	now := s.now().Format(time.RFC3339Nano)
	source := NormalizeSource(record.Source)
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO runtime_discovery_records (
			canonical_id, peer_id, route_candidates_json, transport_capabilities_json, direct_hints_json,
			store_forward_hints_json, signed_peer_record, source, reachable, resolved_at, fresh_until,
			announced_at, updated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(canonical_id) DO UPDATE SET
			peer_id = excluded.peer_id,
			route_candidates_json = excluded.route_candidates_json,
			transport_capabilities_json = excluded.transport_capabilities_json,
			direct_hints_json = excluded.direct_hints_json,
			store_forward_hints_json = excluded.store_forward_hints_json,
			signed_peer_record = excluded.signed_peer_record,
			source = excluded.source,
			reachable = excluded.reachable,
			resolved_at = excluded.resolved_at,
			fresh_until = excluded.fresh_until,
			announced_at = excluded.announced_at,
			updated_at = excluded.updated_at
	`,
		canonicalID,
		strings.TrimSpace(record.PeerID),
		string(routeCandidatesJSON),
		string(transportCapsJSON),
		string(directHintsJSON),
		string(storeForwardHintsJSON),
		strings.TrimSpace(record.SignedPeerRecord),
		source,
		boolToInt(record.Reachable),
		strings.TrimSpace(record.ResolvedAt),
		strings.TrimSpace(record.FreshUntil),
		strings.TrimSpace(record.AnnouncedAt),
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("upsert runtime discovery record: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, canonicalID string) (Record, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			canonical_id, peer_id, route_candidates_json, transport_capabilities_json, direct_hints_json,
			store_forward_hints_json, signed_peer_record, source, reachable, resolved_at, fresh_until,
			announced_at, updated_at, created_at
		FROM runtime_discovery_records
		WHERE canonical_id = ?
	`,
		strings.TrimSpace(canonicalID),
	)
	record, err := scanRecord(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Record{}, false, nil
		}
		return Record{}, false, err
	}
	return record, true, nil
}

func (s *Store) List(ctx context.Context) ([]Record, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			canonical_id, peer_id, route_candidates_json, transport_capabilities_json, direct_hints_json,
			store_forward_hints_json, signed_peer_record, source, reachable, resolved_at, fresh_until,
			announced_at, updated_at, created_at
		FROM runtime_discovery_records
		ORDER BY canonical_id
	`)
	if err != nil {
		return nil, fmt.Errorf("query runtime discovery records: %w", err)
	}
	defer rows.Close()

	records := make([]Record, 0)
	for rows.Next() {
		record, scanErr := scanRecord(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime discovery records: %w", err)
	}
	return records, nil
}

func (s *Store) Delete(ctx context.Context, canonicalID string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM runtime_discovery_records
		WHERE canonical_id = ?
	`,
		strings.TrimSpace(canonicalID),
	)
	if err != nil {
		return false, fmt.Errorf("delete runtime discovery record: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("runtime discovery delete rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRecord(scanner rowScanner) (Record, error) {
	var record Record
	var routeCandidatesRaw string
	var transportCapsRaw string
	var directHintsRaw string
	var storeForwardHintsRaw string
	var reachable int
	if err := scanner.Scan(
		&record.CanonicalID,
		&record.PeerID,
		&routeCandidatesRaw,
		&transportCapsRaw,
		&directHintsRaw,
		&storeForwardHintsRaw,
		&record.SignedPeerRecord,
		&record.Source,
		&reachable,
		&record.ResolvedAt,
		&record.FreshUntil,
		&record.AnnouncedAt,
		&record.UpdatedAt,
		&record.CreatedAt,
	); err != nil {
		return Record{}, fmt.Errorf("scan runtime discovery record: %w", err)
	}
	if err := decodeRouteCandidates(routeCandidatesRaw, &record.RouteCandidates); err != nil {
		return Record{}, fmt.Errorf("decode runtime discovery route_candidates_json: %w", err)
	}
	if err := decodeStringArray(transportCapsRaw, &record.TransportCapabilities); err != nil {
		return Record{}, fmt.Errorf("decode runtime discovery transport_capabilities_json: %w", err)
	}
	if err := decodeStringArray(directHintsRaw, &record.DirectHints); err != nil {
		return Record{}, fmt.Errorf("decode runtime discovery direct_hints_json: %w", err)
	}
	if err := decodeStringArray(storeForwardHintsRaw, &record.StoreForwardHints); err != nil {
		return Record{}, fmt.Errorf("decode runtime discovery store_forward_hints_json: %w", err)
	}
	record.Source = NormalizeSource(record.Source)
	record.Reachable = reachable != 0
	return record, nil
}

func decodeRouteCandidates(raw string, out *[]transport.RouteCandidate) error {
	if strings.TrimSpace(raw) == "" {
		*out = []transport.RouteCandidate{}
		return nil
	}
	var records []transport.RouteCandidate
	if err := json.Unmarshal([]byte(raw), &records); err != nil {
		return err
	}
	*out = records
	return nil
}

func decodeStringArray(raw string, out *[]string) error {
	if strings.TrimSpace(raw) == "" {
		*out = []string{}
		return nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return err
	}
	*out = normalizeStringList(values)
	return nil
}

func normalizeStringList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
