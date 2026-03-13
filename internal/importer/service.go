package importer

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/ids"
	"github.com/xiewanpeng/claw-identity/internal/layout"
	"github.com/xiewanpeng/claw-identity/internal/migrate"
	"github.com/xiewanpeng/claw-identity/internal/resolver"

	_ "modernc.org/sqlite"
)

type Options struct {
	Home            string
	Input           string
	AllowDiscovered bool
	AllowMismatch   bool
}

type Result struct {
	Home          string          `json:"home"`
	DBPath        string          `json:"db_path"`
	ContactID     string          `json:"contact_id"`
	TrustID       string          `json:"trust_id"`
	EventID       string          `json:"event_id"`
	Created       bool            `json:"created"`
	SnapshotCount int             `json:"snapshot_count"`
	ProofCount    int             `json:"proof_count"`
	Inspection    resolver.Result `json:"inspection"`
	ImportedAt    string          `json:"imported_at"`
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

func (s *Service) Import(ctx context.Context, opts Options) (Result, error) {
	if s.Resolver == nil {
		s.Resolver = resolver.NewService()
	}
	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn().UTC()

	inspection, err := s.Resolver.Inspect(ctx, opts.Input)
	if err != nil {
		return Result{}, err
	}
	if err := ensureImportable(inspection.Status, opts); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(inspection.CanonicalID) == "" {
		return Result{}, errors.New("resolved identity is missing canonical_id")
	}

	home, err := layout.ResolveHome(opts.Home)
	if err != nil {
		return Result{}, err
	}
	paths := layout.BuildPaths(home)
	if _, err := os.Stat(paths.DB); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Result{}, fmt.Errorf("state db not found at %q; run linkclaw init first", paths.DB)
		}
		return Result{}, fmt.Errorf("stat state db: %w", err)
	}

	db, err := sql.Open("sqlite", paths.DB)
	if err != nil {
		return Result{}, fmt.Errorf("open sqlite database: %w", err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return Result{}, fmt.Errorf("ping sqlite database: %w", err)
	}
	if _, err := migrate.Apply(ctx, db, now); err != nil {
		return Result{}, fmt.Errorf("apply migrations: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return Result{}, fmt.Errorf("begin import transaction: %w", err)
	}
	defer tx.Rollback()

	contactID, created, err := upsertContact(ctx, tx, inspection, now)
	if err != nil {
		return Result{}, err
	}
	trustID, err := upsertTrustRecord(ctx, tx, contactID, inspection, now)
	if err != nil {
		return Result{}, err
	}
	snapshotCount, err := insertArtifactSnapshots(ctx, tx, contactID, inspection.Artifacts, now)
	if err != nil {
		return Result{}, err
	}
	proofCount, err := insertProofs(ctx, tx, contactID, inspection.Proofs, now)
	if err != nil {
		return Result{}, err
	}
	eventID, err := insertImportEvent(ctx, tx, contactID, inspection, now)
	if err != nil {
		return Result{}, err
	}

	if err := tx.Commit(); err != nil {
		return Result{}, fmt.Errorf("commit import transaction: %w", err)
	}

	return Result{
		Home:          home,
		DBPath:        paths.DB,
		ContactID:     contactID,
		TrustID:       trustID,
		EventID:       eventID,
		Created:       created,
		SnapshotCount: snapshotCount,
		ProofCount:    proofCount,
		Inspection:    inspection,
		ImportedAt:    now.Format(time.RFC3339Nano),
	}, nil
}

func ensureImportable(status string, opts Options) error {
	switch status {
	case resolver.StatusConsistent, resolver.StatusResolved:
		return nil
	case resolver.StatusDiscovered:
		if opts.AllowDiscovered {
			return nil
		}
		return errors.New("import requires resolved or consistent identity by default; use an override only when you explicitly want discovered identities")
	case resolver.StatusMismatch:
		if opts.AllowMismatch {
			return nil
		}
		return errors.New("import refuses mismatched identity by default; use an override only when you explicitly want conflicting artifacts")
	default:
		return fmt.Errorf("unsupported inspection status %q", status)
	}
}

func upsertContact(ctx context.Context, tx *sql.Tx, inspection resolver.Result, now time.Time) (string, bool, error) {
	const selectSQL = `
		SELECT contact_id
		FROM contacts
		WHERE canonical_id = ?
		LIMIT 1
	`

	var contactID string
	err := tx.QueryRowContext(ctx, selectSQL, inspection.CanonicalID).Scan(&contactID)
	switch {
	case err == nil:
		_, execErr := tx.ExecContext(
			ctx,
			`UPDATE contacts
			 SET display_name = CASE WHEN ? <> '' THEN ? ELSE display_name END,
			     home_origin = CASE WHEN ? <> '' THEN ? ELSE home_origin END,
			     profile_url = CASE WHEN ? <> '' THEN ? ELSE profile_url END,
			     status = ?,
			     last_seen_at = ?
			 WHERE contact_id = ?`,
			inspection.DisplayName, inspection.DisplayName,
			inspection.NormalizedOrigin, inspection.NormalizedOrigin,
			inspection.ProfileURL, inspection.ProfileURL,
			inspection.Status,
			now.Format(time.RFC3339Nano),
			contactID,
		)
		if execErr != nil {
			return "", false, fmt.Errorf("update contact: %w", execErr)
		}
		return contactID, false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return "", false, fmt.Errorf("query contact: %w", err)
	}

	contactID, err = ids.New("contact")
	if err != nil {
		return "", false, err
	}
	displayName := strings.TrimSpace(inspection.DisplayName)
	if displayName == "" {
		displayName = inspection.CanonicalID
	}
	stamp := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO contacts (
			contact_id, canonical_id, display_name, home_origin, profile_url, status, last_seen_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		contactID,
		inspection.CanonicalID,
		displayName,
		inspection.NormalizedOrigin,
		inspection.ProfileURL,
		inspection.Status,
		stamp,
		stamp,
	); err != nil {
		return "", false, fmt.Errorf("insert contact: %w", err)
	}
	return contactID, true, nil
}

func upsertTrustRecord(ctx context.Context, tx *sql.Tx, contactID string, inspection resolver.Result, now time.Time) (string, error) {
	const selectSQL = `
		SELECT trust_id, trust_level, risk_flags
		FROM trust_records
		WHERE contact_id = ?
		LIMIT 1
	`

	var trustID string
	var trustLevel string
	var riskFlags string
	err := tx.QueryRowContext(ctx, selectSQL, contactID).Scan(&trustID, &trustLevel, &riskFlags)
	switch {
	case err == nil:
		if trustLevel == "" {
			trustLevel = "unknown"
		}
		if riskFlags == "" {
			riskFlags = "[]"
		}
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE trust_records
			 SET verification_state = ?, decision_reason = ?, updated_at = ?
			 WHERE trust_id = ?`,
			inspection.Status,
			importDecisionReason(inspection),
			now.Format(time.RFC3339Nano),
			trustID,
		); err != nil {
			return "", fmt.Errorf("update trust record: %w", err)
		}
		return trustID, nil
	case !errors.Is(err, sql.ErrNoRows):
		return "", fmt.Errorf("query trust record: %w", err)
	}

	trustID, err = ids.New("trust")
	if err != nil {
		return "", err
	}
	stamp := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO trust_records (
			trust_id, contact_id, trust_level, risk_flags, verification_state, decision_reason, updated_at, created_at
		) VALUES (?, ?, 'unknown', '[]', ?, ?, ?, ?)`,
		trustID,
		contactID,
		inspection.Status,
		importDecisionReason(inspection),
		stamp,
		stamp,
	); err != nil {
		return "", fmt.Errorf("insert trust record: %w", err)
	}
	return trustID, nil
}

func insertArtifactSnapshots(ctx context.Context, tx *sql.Tx, contactID string, artifacts []resolver.Artifact, now time.Time) (int, error) {
	count := 0
	stamp := now.Format(time.RFC3339Nano)
	for _, artifact := range artifacts {
		if !artifact.OK {
			continue
		}
		snapshotID, err := ids.New("snapshot")
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO artifact_snapshots (
				snapshot_id, contact_id, artifact_type, source_url, fetched_at, http_status, content_hash, parsed_summary, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			snapshotID,
			contactID,
			artifact.Type,
			artifact.URL,
			stamp,
			artifact.HTTPStatus,
			artifact.ContentHash,
			artifact.Summary,
			stamp,
		); err != nil {
			return 0, fmt.Errorf("insert artifact snapshot: %w", err)
		}
		count++
	}
	return count, nil
}

func insertProofs(ctx context.Context, tx *sql.Tx, contactID string, proofs []resolver.Proof, now time.Time) (int, error) {
	count := 0
	stamp := now.Format(time.RFC3339Nano)
	for _, proof := range proofs {
		proofID, err := ids.New("proof")
		if err != nil {
			return 0, err
		}
		urlValue := strings.TrimSpace(proof.URL)
		if urlValue == "" {
			urlValue = strings.TrimSpace(proof.ObservedValue)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO proofs (
				proof_id, contact_id, proof_type, proof_url, observed_value, verified_status, verified_at, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			proofID,
			contactID,
			proof.Type,
			urlValue,
			proof.ObservedValue,
			proof.VerifiedStatus,
			stamp,
			stamp,
		); err != nil {
			return 0, fmt.Errorf("insert proof: %w", err)
		}
		count++
	}
	return count, nil
}

func insertImportEvent(ctx context.Context, tx *sql.Tx, contactID string, inspection resolver.Result, now time.Time) (string, error) {
	eventID, err := ids.New("event")
	if err != nil {
		return "", err
	}
	stamp := now.Format(time.RFC3339Nano)
	summary := fmt.Sprintf("imported %s with status=%s from %s", inspection.CanonicalID, inspection.Status, inspection.Input)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO interaction_events (
			event_id, contact_id, channel, event_type, summary, event_at, created_at
		) VALUES (?, ?, 'linkclaw', 'import', ?, ?, ?)`,
		eventID,
		contactID,
		summary,
		stamp,
		stamp,
	); err != nil {
		return "", fmt.Errorf("insert interaction event: %w", err)
	}
	return eventID, nil
}

func importDecisionReason(inspection resolver.Result) string {
	return fmt.Sprintf("imported via public artifacts with verification_state=%s", inspection.Status)
}
