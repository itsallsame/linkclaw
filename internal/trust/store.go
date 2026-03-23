package trust

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

type Record struct {
	CanonicalID       string
	ContactID         string
	TrustLevel        string
	RiskFlags         []string
	VerificationState string
	DecisionReason    string
	Source            string
	DecidedAt         string
	UpdatedAt         string
	CreatedAt         string
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
		return fmt.Errorf("trust canonical_id is required")
	}
	now := s.now().Format(time.RFC3339Nano)
	riskFlagsJSON, err := json.Marshal(normalizeStringList(record.RiskFlags))
	if err != nil {
		return fmt.Errorf("marshal trust risk flags: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO runtime_trust_records (
			canonical_id, contact_id, trust_level, risk_flags_json, verification_state,
			decision_reason, source, decided_at, updated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(canonical_id) DO UPDATE SET
			contact_id = excluded.contact_id,
			trust_level = excluded.trust_level,
			risk_flags_json = excluded.risk_flags_json,
			verification_state = excluded.verification_state,
			decision_reason = excluded.decision_reason,
			source = excluded.source,
			decided_at = excluded.decided_at,
			updated_at = excluded.updated_at
	`,
		canonicalID,
		strings.TrimSpace(record.ContactID),
		defaultString(record.TrustLevel, "unknown"),
		string(riskFlagsJSON),
		strings.TrimSpace(record.VerificationState),
		strings.TrimSpace(record.DecisionReason),
		strings.TrimSpace(record.Source),
		strings.TrimSpace(record.DecidedAt),
		now,
		now,
	)
	if err != nil {
		return fmt.Errorf("upsert runtime trust record: %w", err)
	}
	return nil
}

func (s *Store) Get(ctx context.Context, canonicalID string) (Record, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			canonical_id, contact_id, trust_level, risk_flags_json, verification_state,
			decision_reason, source, decided_at, updated_at, created_at
		FROM runtime_trust_records
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
			canonical_id, contact_id, trust_level, risk_flags_json, verification_state,
			decision_reason, source, decided_at, updated_at, created_at
		FROM runtime_trust_records
		ORDER BY canonical_id
	`)
	if err != nil {
		return nil, fmt.Errorf("query runtime trust records: %w", err)
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
		return nil, fmt.Errorf("iterate runtime trust records: %w", err)
	}
	return records, nil
}

func (s *Store) Delete(ctx context.Context, canonicalID string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM runtime_trust_records
		WHERE canonical_id = ?
	`,
		strings.TrimSpace(canonicalID),
	)
	if err != nil {
		return false, fmt.Errorf("delete runtime trust record: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("runtime trust delete rows affected: %w", err)
	}
	return rowsAffected > 0, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRecord(scanner rowScanner) (Record, error) {
	var record Record
	var rawRiskFlags string
	if err := scanner.Scan(
		&record.CanonicalID,
		&record.ContactID,
		&record.TrustLevel,
		&rawRiskFlags,
		&record.VerificationState,
		&record.DecisionReason,
		&record.Source,
		&record.DecidedAt,
		&record.UpdatedAt,
		&record.CreatedAt,
	); err != nil {
		return Record{}, fmt.Errorf("scan runtime trust record: %w", err)
	}
	decoded, err := decodeStringArray(rawRiskFlags)
	if err != nil {
		return Record{}, fmt.Errorf("decode runtime trust risk_flags_json: %w", err)
	}
	record.RiskFlags = decoded
	return record, nil
}

func decodeStringArray(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return []string{}, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return normalizeStringList(values), nil
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

func defaultString(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
