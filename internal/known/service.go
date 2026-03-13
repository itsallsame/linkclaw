package known

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/ids"
	"github.com/xiewanpeng/claw-identity/internal/importer"
	"github.com/xiewanpeng/claw-identity/internal/layout"
	"github.com/xiewanpeng/claw-identity/internal/migrate"
	"github.com/xiewanpeng/claw-identity/internal/resolver"

	_ "modernc.org/sqlite"
)

var allowedTrustLevels = map[string]struct{}{
	"unknown":  {},
	"seen":     {},
	"verified": {},
	"trusted":  {},
	"pinned":   {},
}

type rowScanner interface {
	Scan(dest ...any) error
}

type ListOptions struct {
	Home string
}

type LookupOptions struct {
	Home       string
	Identifier string
}

type TrustOptions struct {
	Home         string
	Identifier   string
	Level        string
	RiskFlags    []string
	HasRiskFlags bool
	Reason       string
}

type NoteOptions struct {
	Home       string
	Identifier string
	Body       string
}

type RefreshOptions struct {
	Home       string
	Identifier string
}

type RemoveOptions struct {
	Home       string
	Identifier string
}

type ListResult struct {
	Home     string           `json:"home"`
	Contacts []ContactSummary `json:"contacts"`
	ListedAt string           `json:"listed_at"`
}

type ShowResult struct {
	Home    string        `json:"home"`
	Contact ContactDetail `json:"contact"`
	ShownAt string        `json:"shown_at"`
}

type TrustResult struct {
	Home      string         `json:"home"`
	Contact   ContactSummary `json:"contact"`
	EventID   string         `json:"event_id"`
	UpdatedAt string         `json:"updated_at"`
}

type NoteResult struct {
	Home      string         `json:"home"`
	Contact   ContactSummary `json:"contact"`
	Note      NoteEntry      `json:"note"`
	EventID   string         `json:"event_id"`
	CreatedAt string         `json:"created_at"`
}

type RefreshResult struct {
	Home          string          `json:"home"`
	Contact       ContactSummary  `json:"contact"`
	EventID       string          `json:"event_id"`
	HandleCount   int             `json:"handle_count"`
	SnapshotCount int             `json:"snapshot_count"`
	ProofCount    int             `json:"proof_count"`
	Inspection    resolver.Result `json:"inspection"`
	RefreshedAt   string          `json:"refreshed_at"`
}

type RemoveResult struct {
	Home      string         `json:"home"`
	Contact   ContactSummary `json:"contact"`
	Removed   RemoveCounts   `json:"removed"`
	RemovedAt string         `json:"removed_at"`
}

type RemoveCounts struct {
	Contacts        int `json:"contacts"`
	TrustRecords    int `json:"trust_records"`
	Handles         int `json:"handles"`
	Artifacts       int `json:"artifacts"`
	Proofs          int `json:"proofs"`
	Notes           int `json:"notes"`
	Events          int `json:"events"`
	PinnedMaterials int `json:"pinned_materials"`
	PolicyHints     int `json:"policy_hints"`
}

type ContactSummary struct {
	ContactID   string      `json:"contact_id"`
	CanonicalID string      `json:"canonical_id"`
	DisplayName string      `json:"display_name"`
	HomeOrigin  string      `json:"home_origin,omitempty"`
	ProfileURL  string      `json:"profile_url,omitempty"`
	Status      string      `json:"status"`
	LastSeenAt  string      `json:"last_seen_at,omitempty"`
	Trust       TrustRecord `json:"trust"`
	NoteCount   int         `json:"note_count"`
	LastEventAt string      `json:"last_event_at,omitempty"`
}

type ContactDetail struct {
	ContactSummary
	Handles   []HandleRecord     `json:"handles"`
	Artifacts []ArtifactRecord   `json:"artifacts"`
	Proofs    []ProofRecord      `json:"proofs"`
	Notes     []NoteEntry        `json:"notes"`
	Events    []InteractionEvent `json:"events"`
}

type TrustRecord struct {
	TrustID           string   `json:"trust_id,omitempty"`
	TrustLevel        string   `json:"trust_level"`
	RiskFlags         []string `json:"risk_flags"`
	VerificationState string   `json:"verification_state,omitempty"`
	DecisionReason    string   `json:"decision_reason,omitempty"`
	UpdatedAt         string   `json:"updated_at,omitempty"`
	CreatedAt         string   `json:"created_at,omitempty"`
}

type HandleRecord struct {
	HandleID   string `json:"handle_id"`
	HandleType string `json:"handle_type"`
	Value      string `json:"value"`
	IsPrimary  bool   `json:"is_primary"`
}

type ArtifactRecord struct {
	SnapshotID    string `json:"snapshot_id"`
	ArtifactType  string `json:"artifact_type"`
	SourceURL     string `json:"source_url"`
	FetchedAt     string `json:"fetched_at"`
	HTTPStatus    int    `json:"http_status"`
	ContentHash   string `json:"content_hash,omitempty"`
	ParsedSummary string `json:"parsed_summary,omitempty"`
}

type ProofRecord struct {
	ProofID        string `json:"proof_id"`
	ProofType      string `json:"proof_type"`
	ProofURL       string `json:"proof_url"`
	ObservedValue  string `json:"observed_value"`
	VerifiedStatus string `json:"verified_status"`
	VerifiedAt     string `json:"verified_at,omitempty"`
	CreatedAt      string `json:"created_at,omitempty"`
}

type NoteEntry struct {
	NoteID    string `json:"note_id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type InteractionEvent struct {
	EventID   string `json:"event_id"`
	Channel   string `json:"channel"`
	EventType string `json:"event_type"`
	Summary   string `json:"summary"`
	EventAt   string `json:"event_at"`
	CreatedAt string `json:"created_at,omitempty"`
}

type Service struct {
	Importer *importer.Service
	Now      func() time.Time
}

func NewService() *Service {
	return &Service{
		Importer: importer.NewService(),
		Now:      time.Now,
	}
}

func (s *Service) List(ctx context.Context, opts ListOptions) (ListResult, error) {
	now := s.now()
	db, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return ListResult{}, err
	}
	defer db.Close()

	contacts, err := listContactSummaries(ctx, db)
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{
		Home:     home,
		Contacts: contacts,
		ListedAt: now.Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) Show(ctx context.Context, opts LookupOptions) (ShowResult, error) {
	now := s.now()
	db, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return ShowResult{}, err
	}
	defer db.Close()

	contact, err := resolveContactReference(ctx, db, opts.Identifier)
	if err != nil {
		return ShowResult{}, err
	}
	detail, err := loadContactDetail(ctx, db, contact.ContactID)
	if err != nil {
		return ShowResult{}, err
	}
	return ShowResult{
		Home:    home,
		Contact: detail,
		ShownAt: now.Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) Trust(ctx context.Context, opts TrustOptions) (TrustResult, error) {
	level := strings.TrimSpace(strings.ToLower(opts.Level))
	if level == "" && !opts.HasRiskFlags {
		return TrustResult{}, errors.New("known trust requires --level, --risk, or both")
	}
	if level != "" {
		if err := validateTrustLevel(level); err != nil {
			return TrustResult{}, err
		}
	}

	now := s.now()
	db, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return TrustResult{}, err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return TrustResult{}, fmt.Errorf("begin trust transaction: %w", err)
	}
	defer tx.Rollback()

	contact, err := resolveContactReferenceTx(ctx, tx, opts.Identifier)
	if err != nil {
		return TrustResult{}, err
	}
	trust, err := ensureTrustRecord(ctx, tx, contact, now)
	if err != nil {
		return TrustResult{}, err
	}
	if level != "" {
		trust.TrustLevel = level
	}
	if opts.HasRiskFlags {
		trust.RiskFlags = normalizeStringList(opts.RiskFlags)
	}

	stamp := now.Format(time.RFC3339Nano)
	reason := strings.TrimSpace(opts.Reason)
	if reason == "" {
		reason = summarizeTrustChange(trust)
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE trust_records
		 SET trust_level = ?, risk_flags = ?, decision_reason = ?, updated_at = ?
		 WHERE trust_id = ?`,
		trust.TrustLevel,
		encodeStringArray(trust.RiskFlags),
		reason,
		stamp,
		trust.TrustID,
	); err != nil {
		return TrustResult{}, fmt.Errorf("update trust record: %w", err)
	}
	eventID, err := insertEvent(ctx, tx, contact.ContactID, "trust", summarizeTrustChange(trust), now)
	if err != nil {
		return TrustResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return TrustResult{}, fmt.Errorf("commit trust transaction: %w", err)
	}

	summary, err := loadContactSummaryByID(ctx, db, contact.ContactID)
	if err != nil {
		return TrustResult{}, err
	}
	return TrustResult{
		Home:      home,
		Contact:   summary,
		EventID:   eventID,
		UpdatedAt: stamp,
	}, nil
}

func (s *Service) Note(ctx context.Context, opts NoteOptions) (NoteResult, error) {
	body := strings.TrimSpace(opts.Body)
	if body == "" {
		return NoteResult{}, errors.New("known note requires a non-empty body")
	}

	now := s.now()
	db, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return NoteResult{}, err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return NoteResult{}, fmt.Errorf("begin note transaction: %w", err)
	}
	defer tx.Rollback()

	contact, err := resolveContactReferenceTx(ctx, tx, opts.Identifier)
	if err != nil {
		return NoteResult{}, err
	}
	noteID, err := ids.New("note")
	if err != nil {
		return NoteResult{}, err
	}
	stamp := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO notes (note_id, contact_id, body, created_at) VALUES (?, ?, ?, ?)`,
		noteID,
		contact.ContactID,
		body,
		stamp,
	); err != nil {
		return NoteResult{}, fmt.Errorf("insert note: %w", err)
	}
	eventID, err := insertEvent(ctx, tx, contact.ContactID, "note", summarizeNoteEvent(body), now)
	if err != nil {
		return NoteResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return NoteResult{}, fmt.Errorf("commit note transaction: %w", err)
	}

	summary, err := loadContactSummaryByID(ctx, db, contact.ContactID)
	if err != nil {
		return NoteResult{}, err
	}
	return NoteResult{
		Home:    home,
		Contact: summary,
		Note: NoteEntry{
			NoteID:    noteID,
			Body:      body,
			CreatedAt: stamp,
		},
		EventID:   eventID,
		CreatedAt: stamp,
	}, nil
}

func (s *Service) Refresh(ctx context.Context, opts RefreshOptions) (RefreshResult, error) {
	now := s.now()
	db, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return RefreshResult{}, err
	}
	defer db.Close()

	detail, err := resolveContactDetail(ctx, db, opts.Identifier)
	if err != nil {
		return RefreshResult{}, err
	}
	input := refreshInput(detail)
	if input == "" {
		return RefreshResult{}, fmt.Errorf("contact %q does not have a refreshable public URL", detail.ContactID)
	}

	if s.Importer == nil {
		s.Importer = importer.NewService()
	}
	importResult, err := s.Importer.Import(ctx, importer.Options{
		Home:                home,
		Input:               input,
		AllowDiscovered:     true,
		AllowMismatch:       true,
		Action:              "refresh",
		TargetContactID:     detail.ContactID,
		ExpectedCanonicalID: detail.CanonicalID,
	})
	if err != nil {
		return RefreshResult{}, err
	}

	summary, err := loadContactSummaryByID(ctx, db, detail.ContactID)
	if err != nil {
		return RefreshResult{}, err
	}
	return RefreshResult{
		Home:          home,
		Contact:       summary,
		EventID:       importResult.EventID,
		HandleCount:   importResult.HandleCount,
		SnapshotCount: importResult.SnapshotCount,
		ProofCount:    importResult.ProofCount,
		Inspection:    importResult.Inspection,
		RefreshedAt:   importResult.ImportedAt,
	}, nil
}

func (s *Service) Remove(ctx context.Context, opts RemoveOptions) (RemoveResult, error) {
	now := s.now()
	db, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return RemoveResult{}, err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return RemoveResult{}, fmt.Errorf("begin remove transaction: %w", err)
	}
	defer tx.Rollback()

	contact, err := resolveContactReferenceTx(ctx, tx, opts.Identifier)
	if err != nil {
		return RemoveResult{}, err
	}
	counts := RemoveCounts{}
	if counts.Handles, err = execDelete(ctx, tx, `DELETE FROM handles WHERE owner_type = 'contact' AND owner_id = ?`, contact.ContactID); err != nil {
		return RemoveResult{}, err
	}
	if counts.Artifacts, err = execDelete(ctx, tx, `DELETE FROM artifact_snapshots WHERE contact_id = ?`, contact.ContactID); err != nil {
		return RemoveResult{}, err
	}
	if counts.Proofs, err = execDelete(ctx, tx, `DELETE FROM proofs WHERE contact_id = ?`, contact.ContactID); err != nil {
		return RemoveResult{}, err
	}
	if counts.Notes, err = execDelete(ctx, tx, `DELETE FROM notes WHERE contact_id = ?`, contact.ContactID); err != nil {
		return RemoveResult{}, err
	}
	if counts.Events, err = execDelete(ctx, tx, `DELETE FROM interaction_events WHERE contact_id = ?`, contact.ContactID); err != nil {
		return RemoveResult{}, err
	}
	if counts.PinnedMaterials, err = execDelete(ctx, tx, `DELETE FROM pinned_materials WHERE contact_id = ?`, contact.ContactID); err != nil {
		return RemoveResult{}, err
	}
	if counts.PolicyHints, err = execDelete(ctx, tx, `DELETE FROM policy_hints WHERE owner_type = 'contact' AND owner_id = ?`, contact.ContactID); err != nil {
		return RemoveResult{}, err
	}
	if counts.TrustRecords, err = execDelete(ctx, tx, `DELETE FROM trust_records WHERE contact_id = ?`, contact.ContactID); err != nil {
		return RemoveResult{}, err
	}
	if counts.Contacts, err = execDelete(ctx, tx, `DELETE FROM contacts WHERE contact_id = ?`, contact.ContactID); err != nil {
		return RemoveResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RemoveResult{}, fmt.Errorf("commit remove transaction: %w", err)
	}

	return RemoveResult{
		Home:      home,
		Contact:   contact,
		Removed:   counts,
		RemovedAt: now.Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) now() time.Time {
	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	return nowFn().UTC()
}

func openStateDB(ctx context.Context, rawHome string, now time.Time) (*sql.DB, string, error) {
	home, err := layout.ResolveHome(rawHome)
	if err != nil {
		return nil, "", err
	}
	paths := layout.BuildPaths(home)
	if _, err := os.Stat(paths.DB); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, "", fmt.Errorf("state db not found at %q; run linkclaw init first", paths.DB)
		}
		return nil, "", fmt.Errorf("stat state db: %w", err)
	}

	db, err := sql.Open("sqlite", paths.DB)
	if err != nil {
		return nil, "", fmt.Errorf("open sqlite database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, "", fmt.Errorf("ping sqlite database: %w", err)
	}
	if _, err := migrate.Apply(ctx, db, now); err != nil {
		db.Close()
		return nil, "", fmt.Errorf("apply migrations: %w", err)
	}
	return db, home, nil
}

func listContactSummaries(ctx context.Context, db *sql.DB) ([]ContactSummary, error) {
	rows, err := db.QueryContext(ctx, contactSummarySelect+`
		ORDER BY CASE WHEN c.display_name <> '' THEN c.display_name ELSE c.canonical_id END,
		         c.contact_id
	`)
	if err != nil {
		return nil, fmt.Errorf("query contacts: %w", err)
	}
	defer rows.Close()

	contacts := make([]ContactSummary, 0)
	for rows.Next() {
		contact, err := scanContactSummary(rows)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, contact)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contacts: %w", err)
	}
	return contacts, nil
}

func resolveContactDetail(ctx context.Context, db *sql.DB, identifier string) (ContactDetail, error) {
	contact, err := resolveContactReference(ctx, db, identifier)
	if err != nil {
		return ContactDetail{}, err
	}
	return loadContactDetail(ctx, db, contact.ContactID)
}

func resolveContactReference(ctx context.Context, db *sql.DB, identifier string) (ContactSummary, error) {
	ref := strings.TrimSpace(identifier)
	if ref == "" {
		return ContactSummary{}, errors.New("contact reference is required")
	}
	candidates, err := queryContactIDsByReference(ctx, db, ref)
	if err != nil {
		return ContactSummary{}, err
	}
	if len(candidates) == 0 && !strings.Contains(ref, "://") {
		candidates, err = queryContactIDsByReference(ctx, db, "https://"+ref)
		if err != nil {
			return ContactSummary{}, err
		}
		if len(candidates) == 0 {
			candidates, err = queryContactIDsByReference(ctx, db, "http://"+ref)
			if err != nil {
				return ContactSummary{}, err
			}
		}
	}
	if len(candidates) == 0 {
		return ContactSummary{}, fmt.Errorf("known contact %q not found", ref)
	}
	if len(candidates) > 1 {
		return ContactSummary{}, fmt.Errorf("contact reference %q is ambiguous", ref)
	}
	return loadContactSummaryByID(ctx, db, candidates[0])
}

func resolveContactReferenceTx(ctx context.Context, tx *sql.Tx, identifier string) (ContactSummary, error) {
	ref := strings.TrimSpace(identifier)
	if ref == "" {
		return ContactSummary{}, errors.New("contact reference is required")
	}
	candidates, err := queryContactIDsByReferenceTx(ctx, tx, ref)
	if err != nil {
		return ContactSummary{}, err
	}
	if len(candidates) == 0 && !strings.Contains(ref, "://") {
		candidates, err = queryContactIDsByReferenceTx(ctx, tx, "https://"+ref)
		if err != nil {
			return ContactSummary{}, err
		}
		if len(candidates) == 0 {
			candidates, err = queryContactIDsByReferenceTx(ctx, tx, "http://"+ref)
			if err != nil {
				return ContactSummary{}, err
			}
		}
	}
	if len(candidates) == 0 {
		return ContactSummary{}, fmt.Errorf("known contact %q not found", ref)
	}
	if len(candidates) > 1 {
		return ContactSummary{}, fmt.Errorf("contact reference %q is ambiguous", ref)
	}
	return loadContactSummaryByIDTx(ctx, tx, candidates[0])
}

func queryContactIDsByReference(ctx context.Context, db *sql.DB, ref string) ([]string, error) {
	rows, err := db.QueryContext(ctx, contactReferenceSelect, ref, ref, ref, ref, ref)
	if err != nil {
		return nil, fmt.Errorf("query contact reference: %w", err)
	}
	defer rows.Close()
	return scanContactIDs(rows)
}

func queryContactIDsByReferenceTx(ctx context.Context, tx *sql.Tx, ref string) ([]string, error) {
	rows, err := tx.QueryContext(ctx, contactReferenceSelect, ref, ref, ref, ref, ref)
	if err != nil {
		return nil, fmt.Errorf("query contact reference: %w", err)
	}
	defer rows.Close()
	return scanContactIDs(rows)
}

func scanContactIDs(rows *sql.Rows) ([]string, error) {
	ids := make([]string, 0)
	for rows.Next() {
		var contactID string
		if err := rows.Scan(&contactID); err != nil {
			return nil, fmt.Errorf("scan contact reference: %w", err)
		}
		ids = append(ids, contactID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate contact references: %w", err)
	}
	return dedupeStrings(ids), nil
}

func loadContactSummaryByID(ctx context.Context, db *sql.DB, contactID string) (ContactSummary, error) {
	row := db.QueryRowContext(ctx, contactSummarySelect+` WHERE c.contact_id = ?`, contactID)
	contact, err := scanContactSummary(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ContactSummary{}, fmt.Errorf("known contact %q not found", contactID)
		}
		return ContactSummary{}, err
	}
	return contact, nil
}

func loadContactSummaryByIDTx(ctx context.Context, tx *sql.Tx, contactID string) (ContactSummary, error) {
	row := tx.QueryRowContext(ctx, contactSummarySelect+` WHERE c.contact_id = ?`, contactID)
	contact, err := scanContactSummary(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ContactSummary{}, fmt.Errorf("known contact %q not found", contactID)
		}
		return ContactSummary{}, err
	}
	return contact, nil
}

func scanContactSummary(scanner rowScanner) (ContactSummary, error) {
	var contact ContactSummary
	var riskFlags string
	if err := scanner.Scan(
		&contact.ContactID,
		&contact.CanonicalID,
		&contact.DisplayName,
		&contact.HomeOrigin,
		&contact.ProfileURL,
		&contact.Status,
		&contact.LastSeenAt,
		&contact.Trust.TrustID,
		&contact.Trust.TrustLevel,
		&riskFlags,
		&contact.Trust.VerificationState,
		&contact.Trust.DecisionReason,
		&contact.Trust.UpdatedAt,
		&contact.Trust.CreatedAt,
		&contact.NoteCount,
		&contact.LastEventAt,
	); err != nil {
		return ContactSummary{}, err
	}
	decoded, err := decodeStringArray(riskFlags)
	if err != nil {
		return ContactSummary{}, fmt.Errorf("decode trust risk_flags for %q: %w", contact.ContactID, err)
	}
	contact.Trust.RiskFlags = decoded
	if contact.DisplayName == "" {
		contact.DisplayName = contact.CanonicalID
	}
	if contact.Trust.TrustLevel == "" {
		contact.Trust.TrustLevel = "unknown"
	}
	return contact, nil
}

func loadContactDetail(ctx context.Context, db *sql.DB, contactID string) (ContactDetail, error) {
	summary, err := loadContactSummaryByID(ctx, db, contactID)
	if err != nil {
		return ContactDetail{}, err
	}

	handles, err := loadHandles(ctx, db, contactID)
	if err != nil {
		return ContactDetail{}, err
	}
	artifacts, err := loadArtifacts(ctx, db, contactID)
	if err != nil {
		return ContactDetail{}, err
	}
	proofs, err := loadProofs(ctx, db, contactID)
	if err != nil {
		return ContactDetail{}, err
	}
	notes, err := loadNotes(ctx, db, contactID)
	if err != nil {
		return ContactDetail{}, err
	}
	events, err := loadEvents(ctx, db, contactID)
	if err != nil {
		return ContactDetail{}, err
	}

	return ContactDetail{
		ContactSummary: summary,
		Handles:        handles,
		Artifacts:      artifacts,
		Proofs:         proofs,
		Notes:          notes,
		Events:         events,
	}, nil
}

func loadHandles(ctx context.Context, db *sql.DB, contactID string) ([]HandleRecord, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT handle_id, handle_type, value, is_primary
		 FROM handles
		 WHERE owner_type = 'contact' AND owner_id = ?
		 ORDER BY handle_type, is_primary DESC, value`,
		contactID,
	)
	if err != nil {
		return nil, fmt.Errorf("query handles: %w", err)
	}
	defer rows.Close()

	records := make([]HandleRecord, 0)
	for rows.Next() {
		var record HandleRecord
		var isPrimary int
		if err := rows.Scan(&record.HandleID, &record.HandleType, &record.Value, &isPrimary); err != nil {
			return nil, fmt.Errorf("scan handle: %w", err)
		}
		record.IsPrimary = isPrimary != 0
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate handles: %w", err)
	}
	return records, nil
}

func loadArtifacts(ctx context.Context, db *sql.DB, contactID string) ([]ArtifactRecord, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT snapshot_id, artifact_type, source_url, fetched_at, http_status, content_hash, parsed_summary
		 FROM artifact_snapshots
		 WHERE contact_id = ?
		 ORDER BY fetched_at DESC, snapshot_id DESC`,
		contactID,
	)
	if err != nil {
		return nil, fmt.Errorf("query artifacts: %w", err)
	}
	defer rows.Close()

	records := make([]ArtifactRecord, 0)
	for rows.Next() {
		var record ArtifactRecord
		if err := rows.Scan(
			&record.SnapshotID,
			&record.ArtifactType,
			&record.SourceURL,
			&record.FetchedAt,
			&record.HTTPStatus,
			&record.ContentHash,
			&record.ParsedSummary,
		); err != nil {
			return nil, fmt.Errorf("scan artifact: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate artifacts: %w", err)
	}
	return records, nil
}

func loadProofs(ctx context.Context, db *sql.DB, contactID string) ([]ProofRecord, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT proof_id, proof_type, proof_url, observed_value, verified_status, verified_at, created_at
		 FROM proofs
		 WHERE contact_id = ?
		 ORDER BY created_at DESC, proof_type, observed_value`,
		contactID,
	)
	if err != nil {
		return nil, fmt.Errorf("query proofs: %w", err)
	}
	defer rows.Close()

	records := make([]ProofRecord, 0)
	for rows.Next() {
		var record ProofRecord
		if err := rows.Scan(
			&record.ProofID,
			&record.ProofType,
			&record.ProofURL,
			&record.ObservedValue,
			&record.VerifiedStatus,
			&record.VerifiedAt,
			&record.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan proof: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate proofs: %w", err)
	}
	return records, nil
}

func loadNotes(ctx context.Context, db *sql.DB, contactID string) ([]NoteEntry, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT note_id, body, created_at
		 FROM notes
		 WHERE contact_id = ?
		 ORDER BY created_at DESC, note_id DESC`,
		contactID,
	)
	if err != nil {
		return nil, fmt.Errorf("query notes: %w", err)
	}
	defer rows.Close()

	notes := make([]NoteEntry, 0)
	for rows.Next() {
		var note NoteEntry
		if err := rows.Scan(&note.NoteID, &note.Body, &note.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan note: %w", err)
		}
		notes = append(notes, note)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notes: %w", err)
	}
	return notes, nil
}

func loadEvents(ctx context.Context, db *sql.DB, contactID string) ([]InteractionEvent, error) {
	rows, err := db.QueryContext(
		ctx,
		`SELECT event_id, channel, event_type, summary, event_at, created_at
		 FROM interaction_events
		 WHERE contact_id = ?
		 ORDER BY event_at DESC, event_id DESC`,
		contactID,
	)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer rows.Close()

	events := make([]InteractionEvent, 0)
	for rows.Next() {
		var event InteractionEvent
		if err := rows.Scan(&event.EventID, &event.Channel, &event.EventType, &event.Summary, &event.EventAt, &event.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}
	return events, nil
}

func ensureTrustRecord(ctx context.Context, tx *sql.Tx, contact ContactSummary, now time.Time) (TrustRecord, error) {
	if contact.Trust.TrustID != "" {
		return contact.Trust, nil
	}

	trustID, err := ids.New("trust")
	if err != nil {
		return TrustRecord{}, err
	}
	trust := TrustRecord{
		TrustID:           trustID,
		TrustLevel:        "unknown",
		RiskFlags:         []string{},
		VerificationState: contact.Status,
		DecisionReason:    "initialized from known contact state",
		UpdatedAt:         now.Format(time.RFC3339Nano),
		CreatedAt:         now.Format(time.RFC3339Nano),
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO trust_records (
			trust_id, contact_id, trust_level, risk_flags, verification_state, decision_reason, updated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		trust.TrustID,
		contact.ContactID,
		trust.TrustLevel,
		encodeStringArray(trust.RiskFlags),
		trust.VerificationState,
		trust.DecisionReason,
		trust.UpdatedAt,
		trust.CreatedAt,
	); err != nil {
		return TrustRecord{}, fmt.Errorf("insert trust record: %w", err)
	}
	return trust, nil
}

func insertEvent(ctx context.Context, tx *sql.Tx, contactID, eventType, summary string, now time.Time) (string, error) {
	eventID, err := ids.New("event")
	if err != nil {
		return "", err
	}
	stamp := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO interaction_events (
			event_id, contact_id, channel, event_type, summary, event_at, created_at
		) VALUES (?, ?, 'linkclaw', ?, ?, ?, ?)`,
		eventID,
		contactID,
		eventType,
		summary,
		stamp,
		stamp,
	); err != nil {
		return "", fmt.Errorf("insert %s event: %w", eventType, err)
	}
	return eventID, nil
}

func execDelete(ctx context.Context, tx *sql.Tx, query string, args ...any) (int, error) {
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("exec delete: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return int(rowsAffected), nil
}

func refreshInput(detail ContactDetail) string {
	if strings.TrimSpace(detail.ProfileURL) != "" {
		return strings.TrimSpace(detail.ProfileURL)
	}
	if strings.TrimSpace(detail.HomeOrigin) != "" {
		return strings.TrimSpace(detail.HomeOrigin)
	}
	for _, handle := range detail.Handles {
		if !strings.HasPrefix(handle.Value, "http://") && !strings.HasPrefix(handle.Value, "https://") {
			continue
		}
		switch handle.HandleType {
		case "profile", "origin", "also_known_as":
			return handle.Value
		}
	}
	for _, proof := range detail.Proofs {
		if strings.HasPrefix(proof.ProofURL, "http://") || strings.HasPrefix(proof.ProofURL, "https://") {
			return proof.ProofURL
		}
	}
	return didWebURL(detail.CanonicalID)
}

func didWebURL(canonicalID string) string {
	value := strings.TrimSpace(canonicalID)
	if !strings.HasPrefix(value, "did:web:") {
		return ""
	}
	parts := strings.Split(strings.TrimPrefix(value, "did:web:"), ":")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return ""
	}
	host := strings.ReplaceAll(parts[0], "%3A", ":")
	if len(parts) == 1 {
		return "https://" + host
	}
	pathParts := make([]string, 0, len(parts)-1)
	for _, part := range parts[1:] {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		pathParts = append(pathParts, trimmed)
	}
	if len(pathParts) == 0 {
		return "https://" + host
	}
	return "https://" + host + "/" + strings.Join(pathParts, "/")
}

func validateTrustLevel(level string) error {
	if _, ok := allowedTrustLevels[level]; ok {
		return nil
	}
	return fmt.Errorf("unsupported trust level %q", level)
}

func normalizeStringList(values []string) []string {
	seen := make(map[string]struct{})
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

func encodeStringArray(values []string) string {
	encoded, err := json.Marshal(normalizeStringList(values))
	if err != nil {
		return "[]"
	}
	return string(encoded)
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

func summarizeTrustChange(trust TrustRecord) string {
	if len(trust.RiskFlags) == 0 {
		return fmt.Sprintf("set trust_level=%s verification_state=%s", trust.TrustLevel, trust.VerificationState)
	}
	return fmt.Sprintf(
		"set trust_level=%s verification_state=%s risk_flags=%s",
		trust.TrustLevel,
		trust.VerificationState,
		strings.Join(trust.RiskFlags, ","),
	)
}

func summarizeNoteEvent(body string) string {
	const maxLen = 80
	body = strings.Join(strings.Fields(strings.TrimSpace(body)), " ")
	if len(body) <= maxLen {
		return "added note: " + body
	}
	return "added note: " + body[:maxLen] + "..."
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

const contactSummarySelect = `
SELECT
	c.contact_id,
	c.canonical_id,
	c.display_name,
	c.home_origin,
	c.profile_url,
	c.status,
	c.last_seen_at,
	COALESCE(t.trust_id, ''),
	COALESCE(t.trust_level, 'unknown'),
	COALESCE(t.risk_flags, '[]'),
	COALESCE(t.verification_state, ''),
	COALESCE(t.decision_reason, ''),
	COALESCE(t.updated_at, ''),
	COALESCE(t.created_at, ''),
	COALESCE(n.note_count, 0),
	COALESCE(e.last_event_at, '')
FROM contacts c
LEFT JOIN trust_records t
	ON t.contact_id = c.contact_id
LEFT JOIN (
	SELECT contact_id, COUNT(*) AS note_count
	FROM notes
	GROUP BY contact_id
) n
	ON n.contact_id = c.contact_id
LEFT JOIN (
	SELECT contact_id, MAX(event_at) AS last_event_at
	FROM interaction_events
	GROUP BY contact_id
) e
	ON e.contact_id = c.contact_id
`

const contactReferenceSelect = `
SELECT DISTINCT c.contact_id
FROM contacts c
LEFT JOIN handles h
	ON h.owner_type = 'contact'
	AND h.owner_id = c.contact_id
WHERE c.contact_id = ?
   OR c.canonical_id = ?
   OR c.home_origin = ?
   OR c.profile_url = ?
   OR h.value = ?
ORDER BY c.contact_id
`
