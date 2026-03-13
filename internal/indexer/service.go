package indexer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/ids"
	"github.com/xiewanpeng/claw-identity/internal/layout"
	"github.com/xiewanpeng/claw-identity/internal/migrate"
	"github.com/xiewanpeng/claw-identity/internal/resolver"

	_ "modernc.org/sqlite"
)

const (
	ConflictClear   = "clear"
	ConflictPartial = "partial"
	ConflictMarked  = "conflict"
)

type CrawlOptions struct {
	Home  string
	Input string
}

type SearchOptions struct {
	Home  string
	Query string
}

type CrawlResult struct {
	Home      string `json:"home"`
	DBPath    string `json:"db_path"`
	Record    Record `json:"record"`
	CrawledAt string `json:"crawled_at"`
}

type SearchResult struct {
	Home       string   `json:"home"`
	DBPath     string   `json:"db_path"`
	Query      string   `json:"query,omitempty"`
	Records    []Record `json:"records"`
	SearchedAt string   `json:"searched_at"`
}

type Record struct {
	RecordID         string   `json:"record_id"`
	SeedInput        string   `json:"seed_input"`
	NormalizedOrigin string   `json:"normalized_origin"`
	CanonicalID      string   `json:"canonical_id,omitempty"`
	DisplayName      string   `json:"display_name"`
	ProfileURL       string   `json:"profile_url,omitempty"`
	ResolverStatus   string   `json:"resolver_status"`
	ConflictState    string   `json:"conflict_state"`
	Freshness        string   `json:"freshness"`
	SourceURLs       []string `json:"source_urls"`
	SourceCount      int      `json:"source_count"`
	Warnings         []string `json:"warnings,omitempty"`
	Mismatches       []string `json:"mismatches,omitempty"`
}

type rowScanner interface {
	Scan(dest ...any) error
}

type Service struct {
	Resolver *resolver.Service
	Now      func() time.Time
}

func NewService() *Service {
	return &Service{
		Resolver: resolver.NewService(),
		Now:      time.Now,
	}
}

func (s *Service) Crawl(ctx context.Context, opts CrawlOptions) (CrawlResult, error) {
	if s.Resolver == nil {
		s.Resolver = resolver.NewService()
	}

	now := s.now()
	inspection, err := s.Resolver.Inspect(ctx, opts.Input)
	if err != nil {
		return CrawlResult{}, err
	}

	db, home, paths, err := openIndexDB(ctx, opts.Home, now)
	if err != nil {
		return CrawlResult{}, err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return CrawlResult{}, fmt.Errorf("begin index crawl transaction: %w", err)
	}
	defer tx.Rollback()

	record, err := upsertRecord(ctx, tx, inspection, opts.Input, now)
	if err != nil {
		return CrawlResult{}, err
	}
	if err := replaceSources(ctx, tx, record.RecordID, inspection.Artifacts, now); err != nil {
		return CrawlResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return CrawlResult{}, fmt.Errorf("commit index crawl transaction: %w", err)
	}

	record.SourceURLs = sourceURLsFromArtifacts(inspection.Artifacts)
	record.SourceCount = len(record.SourceURLs)

	return CrawlResult{
		Home:      home,
		DBPath:    paths.DB,
		Record:    record,
		CrawledAt: now.Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) Search(ctx context.Context, opts SearchOptions) (SearchResult, error) {
	now := s.now()
	db, home, paths, err := openIndexDB(ctx, opts.Home, now)
	if err != nil {
		return SearchResult{}, err
	}
	defer db.Close()

	records, err := searchRecords(ctx, db, strings.TrimSpace(opts.Query))
	if err != nil {
		return SearchResult{}, err
	}

	return SearchResult{
		Home:       home,
		DBPath:     paths.DB,
		Query:      strings.TrimSpace(opts.Query),
		Records:    records,
		SearchedAt: now.Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) now() time.Time {
	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	return nowFn().UTC()
}

func openIndexDB(ctx context.Context, rawHome string, now time.Time) (*sql.DB, string, layout.Paths, error) {
	home, err := layout.ResolveHome(rawHome)
	if err != nil {
		return nil, "", layout.Paths{}, err
	}
	if _, err := layout.Ensure(home); err != nil {
		return nil, "", layout.Paths{}, err
	}

	paths := layout.BuildPaths(home)
	db, err := sql.Open("sqlite", paths.DB)
	if err != nil {
		return nil, "", layout.Paths{}, fmt.Errorf("open sqlite database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, "", layout.Paths{}, fmt.Errorf("ping sqlite database: %w", err)
	}
	if _, err := migrate.Apply(ctx, db, now); err != nil {
		db.Close()
		return nil, "", layout.Paths{}, fmt.Errorf("apply migrations: %w", err)
	}
	return db, home, paths, nil
}

func upsertRecord(ctx context.Context, tx *sql.Tx, inspection resolver.Result, seedInput string, now time.Time) (Record, error) {
	record := recordFromInspection(inspection, seedInput, now)
	stamp := now.Format(time.RFC3339Nano)
	warningsJSON := encodeStringArray(record.Warnings)
	mismatchesJSON := encodeStringArray(record.Mismatches)

	var recordID string
	err := tx.QueryRowContext(
		ctx,
		`SELECT record_id
		 FROM index_records
		 WHERE normalized_origin = ?
		 LIMIT 1`,
		record.NormalizedOrigin,
	).Scan(&recordID)
	switch {
	case err == nil:
		record.RecordID = recordID
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE index_records
			 SET seed_input = ?, canonical_id = ?, display_name = ?, profile_url = ?,
			     resolver_status = ?, conflict_state = ?, warnings_json = ?, mismatches_json = ?,
			     freshness_at = ?, updated_at = ?
			 WHERE record_id = ?`,
			record.SeedInput,
			record.CanonicalID,
			record.DisplayName,
			record.ProfileURL,
			record.ResolverStatus,
			record.ConflictState,
			warningsJSON,
			mismatchesJSON,
			record.Freshness,
			stamp,
			record.RecordID,
		); err != nil {
			return Record{}, fmt.Errorf("update index record: %w", err)
		}
		return record, nil
	case err != sql.ErrNoRows:
		return Record{}, fmt.Errorf("query index record: %w", err)
	}

	recordID, err = ids.New("index")
	if err != nil {
		return Record{}, err
	}
	record.RecordID = recordID
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO index_records (
			record_id, normalized_origin, seed_input, canonical_id, display_name, profile_url,
			resolver_status, conflict_state, warnings_json, mismatches_json, freshness_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.RecordID,
		record.NormalizedOrigin,
		record.SeedInput,
		record.CanonicalID,
		record.DisplayName,
		record.ProfileURL,
		record.ResolverStatus,
		record.ConflictState,
		warningsJSON,
		mismatchesJSON,
		record.Freshness,
		stamp,
		stamp,
	); err != nil {
		return Record{}, fmt.Errorf("insert index record: %w", err)
	}
	return record, nil
}

func replaceSources(ctx context.Context, tx *sql.Tx, recordID string, artifacts []resolver.Artifact, now time.Time) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM index_sources WHERE record_id = ?`, recordID); err != nil {
		return fmt.Errorf("delete previous index sources: %w", err)
	}

	stamp := now.Format(time.RFC3339Nano)
	for _, artifact := range artifacts {
		if !artifact.OK {
			continue
		}
		sourceID, err := ids.New("source")
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO index_sources (
				source_id, record_id, artifact_type, source_url, fetched_at, http_status, content_hash, parsed_summary, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			sourceID,
			recordID,
			artifact.Type,
			artifact.URL,
			stamp,
			artifact.HTTPStatus,
			artifact.ContentHash,
			artifact.Summary,
			stamp,
		); err != nil {
			return fmt.Errorf("insert index source: %w", err)
		}
	}

	return nil
}

func searchRecords(ctx context.Context, db *sql.DB, query string) ([]Record, error) {
	baseSQL := `
SELECT
	record_id,
	seed_input,
	normalized_origin,
	canonical_id,
	display_name,
	profile_url,
	resolver_status,
	conflict_state,
	warnings_json,
	mismatches_json,
	freshness_at
FROM index_records
`

	args := make([]any, 0)
	if query != "" {
		pattern := "%" + strings.ToLower(query) + "%"
		baseSQL += `WHERE LOWER(seed_input) LIKE ?
		   OR LOWER(normalized_origin) LIKE ?
		   OR LOWER(canonical_id) LIKE ?
		   OR LOWER(display_name) LIKE ?
		   OR LOWER(profile_url) LIKE ?
`
		args = append(args, pattern, pattern, pattern, pattern, pattern)
	}
	baseSQL += `ORDER BY freshness_at DESC,
	                     CASE WHEN display_name <> '' THEN display_name ELSE canonical_id END,
	                     record_id`

	rows, err := db.QueryContext(ctx, baseSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("query index records: %w", err)
	}
	defer rows.Close()

	records := make([]Record, 0)
	for rows.Next() {
		record, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		record.SourceURLs, err = loadSourceURLs(ctx, db, record.RecordID)
		if err != nil {
			return nil, err
		}
		record.SourceCount = len(record.SourceURLs)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate index records: %w", err)
	}

	return records, nil
}

func scanRecord(scanner rowScanner) (Record, error) {
	var record Record
	var warningsJSON string
	var mismatchesJSON string
	if err := scanner.Scan(
		&record.RecordID,
		&record.SeedInput,
		&record.NormalizedOrigin,
		&record.CanonicalID,
		&record.DisplayName,
		&record.ProfileURL,
		&record.ResolverStatus,
		&record.ConflictState,
		&warningsJSON,
		&mismatchesJSON,
		&record.Freshness,
	); err != nil {
		return Record{}, fmt.Errorf("scan index record: %w", err)
	}

	var err error
	record.Warnings, err = decodeStringArray(warningsJSON)
	if err != nil {
		return Record{}, fmt.Errorf("decode warnings for %q: %w", record.RecordID, err)
	}
	record.Mismatches, err = decodeStringArray(mismatchesJSON)
	if err != nil {
		return Record{}, fmt.Errorf("decode mismatches for %q: %w", record.RecordID, err)
	}
	if record.DisplayName == "" {
		record.DisplayName = fallbackDisplayName(record.CanonicalID, record.NormalizedOrigin)
	}
	return record, nil
}

func loadSourceURLs(ctx context.Context, db *sql.DB, recordID string) ([]string, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT source_url
		 FROM index_sources
		 WHERE record_id = ?
		 ORDER BY fetched_at DESC, artifact_type, source_url`,
		recordID,
	)
	if err != nil {
		return nil, fmt.Errorf("query index sources: %w", err)
	}
	defer rows.Close()

	urls := make([]string, 0)
	for rows.Next() {
		var sourceURL string
		if err := rows.Scan(&sourceURL); err != nil {
			return nil, fmt.Errorf("scan index source: %w", err)
		}
		urls = append(urls, sourceURL)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate index sources: %w", err)
	}
	return dedupeStrings(urls), nil
}

func recordFromInspection(inspection resolver.Result, seedInput string, now time.Time) Record {
	record := Record{
		SeedInput:        strings.TrimSpace(seedInput),
		NormalizedOrigin: strings.TrimSpace(inspection.NormalizedOrigin),
		CanonicalID:      strings.TrimSpace(inspection.CanonicalID),
		DisplayName:      strings.TrimSpace(inspection.DisplayName),
		ProfileURL:       strings.TrimSpace(inspection.ProfileURL),
		ResolverStatus:   strings.TrimSpace(inspection.Status),
		ConflictState:    conflictState(inspection.Status),
		Freshness:        now.Format(time.RFC3339Nano),
		Warnings:         dedupeStrings(inspection.Warnings),
		Mismatches:       dedupeStrings(inspection.Mismatches),
	}
	record.DisplayName = fallbackDisplayName(record.DisplayName, record.CanonicalID, record.NormalizedOrigin)
	return record
}

func conflictState(status string) string {
	switch strings.TrimSpace(status) {
	case resolver.StatusMismatch:
		return ConflictMarked
	case resolver.StatusDiscovered:
		return ConflictPartial
	default:
		return ConflictClear
	}
}

func sourceURLsFromArtifacts(artifacts []resolver.Artifact) []string {
	urls := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		if !artifact.OK {
			continue
		}
		urls = append(urls, strings.TrimSpace(artifact.URL))
	}
	return dedupeStrings(urls)
}

func fallbackDisplayName(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func encodeStringArray(values []string) string {
	encoded, err := json.Marshal(dedupeStrings(values))
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func decodeStringArray(raw string) ([]string, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}
	return dedupeStrings(values), nil
}

func dedupeStrings(values []string) []string {
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
