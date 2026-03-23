package importer

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
	"github.com/xiewanpeng/claw-identity/internal/layout"
	"github.com/xiewanpeng/claw-identity/internal/migrate"
	"github.com/xiewanpeng/claw-identity/internal/resolver"
	"github.com/xiewanpeng/claw-identity/internal/transport"

	_ "modernc.org/sqlite"
)

type Options struct {
	Home                string
	Input               string
	AllowDiscovered     bool
	AllowMismatch       bool
	Action              string
	TargetContactID     string
	ExpectedCanonicalID string
}

type Result struct {
	Home          string          `json:"home"`
	DBPath        string          `json:"db_path"`
	Action        string          `json:"action"`
	ContactID     string          `json:"contact_id"`
	TrustID       string          `json:"trust_id"`
	EventID       string          `json:"event_id"`
	Created       bool            `json:"created"`
	HandleCount   int             `json:"handle_count"`
	SnapshotCount int             `json:"snapshot_count"`
	ProofCount    int             `json:"proof_count"`
	Inspection    resolver.Result `json:"inspection"`
	ImportedAt    string          `json:"imported_at"`
}

type handleCandidate struct {
	Type    string
	Value   string
	Primary bool
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
	action, err := normalizeAction(opts.Action)
	if err != nil {
		return Result{}, err
	}

	inspection, err := s.Resolver.Inspect(ctx, opts.Input)
	if err != nil {
		return Result{}, err
	}
	alignInspectionWithKnownContact(&inspection, opts.ExpectedCanonicalID)
	if err := ensureImportable(inspection.Status, opts); err != nil {
		return Result{}, err
	}
	if strings.TrimSpace(inspection.CanonicalID) == "" && strings.TrimSpace(opts.TargetContactID) == "" {
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

	contactID, created, err := upsertContact(ctx, tx, inspection, now, opts)
	if err != nil {
		return Result{}, err
	}
	trustID, err := upsertTrustRecord(ctx, tx, contactID, inspection, now, action)
	if err != nil {
		return Result{}, err
	}
	canonicalID := strings.TrimSpace(inspection.CanonicalID)
	if canonicalID == "" {
		canonicalID = strings.TrimSpace(opts.ExpectedCanonicalID)
	}
	if err := upsertRuntimeTrustRecord(ctx, tx, canonicalID, contactID, inspection, now, action); err != nil {
		return Result{}, err
	}
	if err := upsertRuntimeDiscoveryRecord(ctx, tx, canonicalID, inspection, now, action); err != nil {
		return Result{}, err
	}
	handleCount, err := upsertHandles(ctx, tx, contactID, inspection, now)
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
	eventID, err := insertActionEvent(ctx, tx, contactID, inspection, now, action)
	if err != nil {
		return Result{}, err
	}

	if err := tx.Commit(); err != nil {
		return Result{}, fmt.Errorf("commit import transaction: %w", err)
	}

	return Result{
		Home:          home,
		DBPath:        paths.DB,
		Action:        action,
		ContactID:     contactID,
		TrustID:       trustID,
		EventID:       eventID,
		Created:       created,
		HandleCount:   handleCount,
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

func upsertContact(ctx context.Context, tx *sql.Tx, inspection resolver.Result, now time.Time, opts Options) (string, bool, error) {
	if targetContactID := strings.TrimSpace(opts.TargetContactID); targetContactID != "" {
		var contactID string
		var canonicalID string
		err := tx.QueryRowContext(
			ctx,
			`SELECT contact_id, canonical_id
			 FROM contacts
			 WHERE contact_id = ?
			 LIMIT 1`,
			targetContactID,
		).Scan(&contactID, &canonicalID)
		switch {
		case err == nil:
			if expected := strings.TrimSpace(opts.ExpectedCanonicalID); expected != "" && canonicalID != expected {
				return "", false, fmt.Errorf("target contact %q canonical_id %q does not match expected %q", contactID, canonicalID, expected)
			}
			if _, err := tx.ExecContext(
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
			); err != nil {
				return "", false, fmt.Errorf("update known contact: %w", err)
			}
			return contactID, false, nil
		case errors.Is(err, sql.ErrNoRows):
			return "", false, fmt.Errorf("target contact %q not found", targetContactID)
		default:
			return "", false, fmt.Errorf("query target contact: %w", err)
		}
	}

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

func upsertTrustRecord(ctx context.Context, tx *sql.Tx, contactID string, inspection resolver.Result, now time.Time, action string) (string, error) {
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
			actionDecisionReason(action, inspection),
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
		actionDecisionReason(action, inspection),
		stamp,
		stamp,
	); err != nil {
		return "", fmt.Errorf("insert trust record: %w", err)
	}
	return trustID, nil
}

func upsertRuntimeTrustRecord(ctx context.Context, tx *sql.Tx, canonicalID, contactID string, inspection resolver.Result, now time.Time, action string) error {
	canonicalID = strings.TrimSpace(canonicalID)
	if canonicalID == "" {
		return nil
	}
	stamp := now.Format(time.RFC3339Nano)
	decidedAt := strings.TrimSpace(inspection.ResolvedAt)
	if decidedAt == "" {
		decidedAt = stamp
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO runtime_trust_records (
			canonical_id, contact_id, trust_level, risk_flags_json, verification_state,
			decision_reason, source, decided_at, updated_at, created_at
		) VALUES (?, ?, 'unknown', '[]', ?, ?, ?, ?, ?, ?)
		ON CONFLICT(canonical_id) DO UPDATE SET
			contact_id = excluded.contact_id,
			verification_state = excluded.verification_state,
			decision_reason = excluded.decision_reason,
			source = excluded.source,
			decided_at = excluded.decided_at,
			updated_at = excluded.updated_at`,
		canonicalID,
		contactID,
		inspection.Status,
		actionDecisionReason(action, inspection),
		action,
		decidedAt,
		stamp,
		stamp,
	); err != nil {
		return fmt.Errorf("upsert runtime trust record: %w", err)
	}
	return nil
}

func upsertRuntimeDiscoveryRecord(ctx context.Context, tx *sql.Tx, canonicalID string, inspection resolver.Result, now time.Time, action string) error {
	canonicalID = strings.TrimSpace(canonicalID)
	if canonicalID == "" {
		return nil
	}
	fact := deriveRuntimeDiscoveryFact(inspection)
	routeCandidatesJSON, err := json.Marshal(fact.RouteCandidates)
	if err != nil {
		return fmt.Errorf("marshal runtime discovery route candidates: %w", err)
	}
	transportCapsJSON, err := json.Marshal(fact.TransportCapabilities)
	if err != nil {
		return fmt.Errorf("marshal runtime discovery transport capabilities: %w", err)
	}
	directHintsJSON, err := json.Marshal(fact.DirectHints)
	if err != nil {
		return fmt.Errorf("marshal runtime discovery direct hints: %w", err)
	}
	storeForwardHintsJSON, err := json.Marshal(fact.StoreForwardHints)
	if err != nil {
		return fmt.Errorf("marshal runtime discovery store-forward hints: %w", err)
	}

	stamp := now.Format(time.RFC3339Nano)
	resolvedAt := strings.TrimSpace(inspection.ResolvedAt)
	if resolvedAt == "" {
		resolvedAt = stamp
	}
	freshUntil := deriveFreshUntil(resolvedAt)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO runtime_discovery_records (
			canonical_id, peer_id, route_candidates_json, transport_capabilities_json, direct_hints_json,
			store_forward_hints_json, signed_peer_record, source, reachable, resolved_at, fresh_until,
			announced_at, updated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?)
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
			updated_at = excluded.updated_at`,
		canonicalID,
		fact.PeerID,
		string(routeCandidatesJSON),
		string(transportCapsJSON),
		string(directHintsJSON),
		string(storeForwardHintsJSON),
		fact.SignedPeerRecord,
		action,
		boolToInt(fact.Reachable),
		resolvedAt,
		freshUntil,
		stamp,
		stamp,
	); err != nil {
		return fmt.Errorf("upsert runtime discovery record: %w", err)
	}
	return nil
}

type runtimeDiscoveryFact struct {
	PeerID                string
	TransportCapabilities []string
	DirectHints           []string
	StoreForwardHints     []string
	SignedPeerRecord      string
	RouteCandidates       []transport.RouteCandidate
	Reachable             bool
}

func deriveRuntimeDiscoveryFact(inspection resolver.Result) runtimeDiscoveryFact {
	peerID := strings.TrimSpace(inspection.PeerID)
	directHints := normalizeStringList(inspection.DirectHints)
	storeForwardHints := normalizeStringList(inspection.StoreForwardHints)
	transportCaps := normalizeCapabilityList(inspection.TransportCapabilities)
	signedPeerRecord := strings.TrimSpace(inspection.SignedPeerRecord)

	if len(directHints) > 0 {
		transportCaps = appendUniqueValue(transportCaps, string(transport.RouteTypeDirect))
	}
	if len(storeForwardHints) > 0 {
		transportCaps = appendUniqueValue(transportCaps, string(transport.RouteTypeStoreForward))
	}
	if peerID != "" && len(directHints) == 0 && containsValue(transportCaps, string(transport.RouteTypeDirect)) {
		directHints = append(directHints, "libp2p://"+peerID)
	}

	routes := make([]transport.RouteCandidate, 0, len(directHints)+len(storeForwardHints))
	for _, target := range directHints {
		routes = append(routes, transport.RouteCandidate{
			Type:     transport.RouteTypeDirect,
			Label:    target,
			Priority: 100,
			Target:   target,
		})
	}
	for _, target := range storeForwardHints {
		routes = append(routes, transport.RouteCandidate{
			Type:     transport.RouteTypeStoreForward,
			Label:    target,
			Priority: 30,
			Target:   target,
		})
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Type == routes[j].Type {
			return routes[i].Target < routes[j].Target
		}
		return routes[i].Type < routes[j].Type
	})

	return runtimeDiscoveryFact{
		PeerID:                peerID,
		TransportCapabilities: transportCaps,
		DirectHints:           directHints,
		StoreForwardHints:     storeForwardHints,
		SignedPeerRecord:      signedPeerRecord,
		RouteCandidates:       routes,
		Reachable:             len(routes) > 0,
	}
}

func deriveFreshUntil(resolvedAt string) string {
	parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(resolvedAt))
	if err != nil {
		return ""
	}
	return parsed.Add(24 * time.Hour).Format(time.RFC3339Nano)
}

func normalizeCapabilityList(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		capability := normalizeCapability(value)
		if capability == "" {
			continue
		}
		normalized = appendUniqueValue(normalized, capability)
	}
	sort.Strings(normalized)
	return normalized
}

func normalizeCapability(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "none":
		return ""
	case "direct", "libp2p":
		return string(transport.RouteTypeDirect)
	case "store_forward", "storeforward", "store-forward", "relay", "recovery":
		return string(transport.RouteTypeStoreForward)
	default:
		return value
	}
}

func normalizeStringList(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized = appendUniqueValue(normalized, value)
	}
	sort.Strings(normalized)
	return normalized
}

func appendUniqueValue(values []string, value string) []string {
	if containsValue(values, value) {
		return values
	}
	return append(values, value)
}

func containsValue(values []string, value string) bool {
	for _, existing := range values {
		if strings.TrimSpace(existing) == strings.TrimSpace(value) {
			return true
		}
	}
	return false
}

func upsertHandles(ctx context.Context, tx *sql.Tx, contactID string, inspection resolver.Result, now time.Time) (int, error) {
	candidates := deriveHandles(inspection)
	if len(candidates) == 0 {
		return 0, nil
	}

	stamp := now.Format(time.RFC3339Nano)
	resetTypes := make(map[string]struct{})
	for _, candidate := range candidates {
		if candidate.Primary {
			if _, seen := resetTypes[candidate.Type]; !seen {
				if _, err := tx.ExecContext(
					ctx,
					`UPDATE handles
					 SET is_primary = 0
					 WHERE owner_type = 'contact' AND owner_id = ? AND handle_type = ?`,
					contactID,
					candidate.Type,
				); err != nil {
					return 0, fmt.Errorf("reset primary handles: %w", err)
				}
				resetTypes[candidate.Type] = struct{}{}
			}
		}

		handleID, err := ids.New("handle")
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO handles (
				handle_id, owner_type, owner_id, handle_type, value, is_primary, created_at
			) VALUES (?, 'contact', ?, ?, ?, ?, ?)
			ON CONFLICT(owner_type, owner_id, handle_type, value)
			DO UPDATE SET is_primary = excluded.is_primary`,
			handleID,
			contactID,
			candidate.Type,
			candidate.Value,
			boolToInt(candidate.Primary),
			stamp,
		); err != nil {
			return 0, fmt.Errorf("upsert handle: %w", err)
		}
	}

	return len(candidates), nil
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

func insertActionEvent(ctx context.Context, tx *sql.Tx, contactID string, inspection resolver.Result, now time.Time, action string) (string, error) {
	eventID, err := ids.New("event")
	if err != nil {
		return "", err
	}
	stamp := now.Format(time.RFC3339Nano)
	subject := strings.TrimSpace(inspection.CanonicalID)
	if subject == "" {
		subject = strings.TrimSpace(inspection.Input)
	}
	summary := fmt.Sprintf("%s %s with status=%s from %s", actionPastTense(action), subject, inspection.Status, inspection.Input)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO interaction_events (
			event_id, contact_id, channel, event_type, summary, event_at, created_at
		) VALUES (?, ?, 'linkclaw', ?, ?, ?, ?)`,
		eventID,
		contactID,
		action,
		summary,
		stamp,
		stamp,
	); err != nil {
		return "", fmt.Errorf("insert interaction event: %w", err)
	}
	return eventID, nil
}

func actionDecisionReason(action string, inspection resolver.Result) string {
	return fmt.Sprintf("%s via public artifacts with verification_state=%s", actionPastTense(action), inspection.Status)
}

func normalizeAction(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "import", nil
	}
	switch value {
	case "import", "refresh":
		return value, nil
	default:
		return "", fmt.Errorf("unsupported importer action %q", value)
	}
}

func actionPastTense(action string) string {
	switch action {
	case "refresh":
		return "refreshed"
	default:
		return "imported"
	}
}

func alignInspectionWithKnownContact(inspection *resolver.Result, expectedCanonicalID string) {
	if inspection == nil {
		return
	}
	expected := strings.TrimSpace(expectedCanonicalID)
	if expected == "" {
		return
	}
	actual := strings.TrimSpace(inspection.CanonicalID)
	if actual == "" || actual == expected {
		return
	}
	inspection.Status = resolver.StatusMismatch
	inspection.Mismatches = appendUniqueString(
		inspection.Mismatches,
		fmt.Sprintf("resolved canonical_id %q does not match known contact %q", actual, expected),
	)
}

func deriveHandles(inspection resolver.Result) []handleCandidate {
	seen := make(map[string]handleCandidate)
	add := func(handleType, value string, primary bool) {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return
		}
		key := handleType + "|" + trimmed
		if existing, ok := seen[key]; ok {
			existing.Primary = existing.Primary || primary
			seen[key] = existing
			return
		}
		seen[key] = handleCandidate{
			Type:    handleType,
			Value:   trimmed,
			Primary: primary,
		}
	}

	add("origin", inspection.NormalizedOrigin, true)
	add("profile", inspection.ProfileURL, true)
	for _, proof := range inspection.Proofs {
		switch proof.Type {
		case "also_known_as", "webfinger_alias":
			add(proof.Type, proof.ObservedValue, false)
		}
	}

	handles := make([]handleCandidate, 0, len(seen))
	for _, candidate := range seen {
		handles = append(handles, candidate)
	}
	sort.Slice(handles, func(i, j int) bool {
		if handles[i].Type == handles[j].Type {
			return handles[i].Value < handles[j].Value
		}
		return handles[i].Type < handles[j].Type
	})
	return handles
}

func appendUniqueString(values []string, value string) []string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return values
	}
	for _, existing := range values {
		if strings.TrimSpace(existing) == trimmed {
			return values
		}
	}
	return append(values, trimmed)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
