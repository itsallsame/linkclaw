package discovery

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type FindOptions struct {
	Capability   string
	Capabilities []string
	Source       string
	FreshOnly    bool
	Limit        int
}

type ShowOptions struct {
	CanonicalID string
}

type RefreshOptions struct {
	CanonicalID string
}

type FindQuery struct {
	Capabilities []string `json:"capabilities,omitempty"`
	Source       string   `json:"source,omitempty"`
	FreshOnly    bool     `json:"fresh_only,omitempty"`
	Limit        int      `json:"limit,omitempty"`
}

type FindResult struct {
	Query   FindQuery        `json:"query"`
	Records []DiscoveryEntry `json:"records"`
	FoundAt string           `json:"found_at"`
}

type ShowResult struct {
	Record  DiscoveryEntry `json:"record"`
	ShownAt string         `json:"shown_at"`
}

type RefreshResult struct {
	Record      DiscoveryEntry `json:"record"`
	Refreshed   bool           `json:"refreshed"`
	RefreshedAt string         `json:"refreshed_at"`
}

type DiscoveryEntry struct {
	CanonicalID           string                     `json:"canonical_id"`
	PeerID                string                     `json:"peer_id,omitempty"`
	Reachable             bool                       `json:"reachable"`
	RouteTypes            []string                   `json:"route_types,omitempty"`
	RouteCandidates       []transport.RouteCandidate `json:"route_candidates,omitempty"`
	TransportCapabilities []string                   `json:"transport_capabilities,omitempty"`
	DirectHints           []string                   `json:"direct_hints,omitempty"`
	StoreForwardHints     []string                   `json:"store_forward_hints,omitempty"`
	SignedPeerRecord      string                     `json:"signed_peer_record,omitempty"`
	Source                string                     `json:"source,omitempty"`
	SourceRank            int                        `json:"source_rank"`
	ResolvedAt            string                     `json:"resolved_at,omitempty"`
	FreshUntil            string                     `json:"fresh_until,omitempty"`
	AnnouncedAt           string                     `json:"announced_at,omitempty"`
	Freshness             Freshness                  `json:"freshness"`
	TrustSummary          TrustSummary               `json:"trust_summary"`
}

type TrustSummary struct {
	CanonicalID       string   `json:"canonical_id"`
	TrustLevel        string   `json:"trust_level"`
	VerificationState string   `json:"verification_state,omitempty"`
	ConfidenceScore   float64  `json:"confidence_score"`
	ConfidenceLevel   string   `json:"confidence_level"`
	Reachability      string   `json:"reachability"`
	RiskFlags         []string `json:"risk_flags,omitempty"`
	Source            string   `json:"source,omitempty"`
	AsOf              string   `json:"as_of,omitempty"`
	Status            string   `json:"status"`
}

type QueryService struct {
	db             *sql.DB
	presence       Service
	now            func() time.Time
	freshness      FreshnessPolicy
	sourceRanking  SourceRanking
	defaultRefresh time.Duration
}

func NewQueryService(db *sql.DB, presence Service) *QueryService {
	return NewQueryServiceWithClock(db, presence, time.Now)
}

func NewQueryServiceWithDB(db *sql.DB, now time.Time, presence Service) *QueryService {
	return NewQueryServiceWithClock(db, presence, func() time.Time { return now.UTC() })
}

func NewQueryServiceWithClock(db *sql.DB, presence Service, nowFn func() time.Time) *QueryService {
	if nowFn == nil {
		nowFn = time.Now
	}
	policy := DefaultFreshnessPolicy()
	return &QueryService{
		db:             db,
		presence:       presence,
		now:            nowFn,
		freshness:      policy,
		sourceRanking:  DefaultSourceRanking(),
		defaultRefresh: policy.FreshWindow,
	}
}

func (s *QueryService) Find(ctx context.Context, opts FindOptions) (FindResult, error) {
	if err := s.validate(); err != nil {
		return FindResult{}, err
	}

	now := s.nowUTC()
	store := NewStoreWithDB(s.db, now)
	records, err := store.List(ctx)
	if err != nil {
		return FindResult{}, err
	}

	requiredCaps := normalizeRequestedCapabilities(opts.Capability, opts.Capabilities)
	sourceFilter := normalizeSource(opts.Source)
	hasSourceFilter := strings.TrimSpace(opts.Source) != ""
	entries := make([]DiscoveryEntry, 0, len(records))
	for _, record := range records {
		if hasSourceFilter && normalizeSource(record.Source) != sourceFilter {
			continue
		}
		if len(requiredCaps) > 0 && !matchesCapabilities(record, requiredCaps) {
			continue
		}
		entry, buildErr := s.buildEntry(ctx, record, now)
		if buildErr != nil {
			return FindResult{}, buildErr
		}
		if opts.FreshOnly && entry.Freshness.State != FreshnessStateFresh {
			continue
		}
		entries = append(entries, entry)
	}

	sort.SliceStable(entries, func(i, j int) bool {
		return compareEntries(entries[i], entries[j])
	})
	if opts.Limit > 0 && len(entries) > opts.Limit {
		entries = entries[:opts.Limit]
	}

	return FindResult{
		Query: FindQuery{
			Capabilities: requiredCaps,
			Source:       strings.TrimSpace(opts.Source),
			FreshOnly:    opts.FreshOnly,
			Limit:        opts.Limit,
		},
		Records: entries,
		FoundAt: now.Format(time.RFC3339Nano),
	}, nil
}

func (s *QueryService) Show(ctx context.Context, opts ShowOptions) (ShowResult, error) {
	if err := s.validate(); err != nil {
		return ShowResult{}, err
	}
	canonicalID := strings.TrimSpace(opts.CanonicalID)
	if canonicalID == "" {
		return ShowResult{}, fmt.Errorf("canonical_id is required")
	}

	now := s.nowUTC()
	store := NewStoreWithDB(s.db, now)
	record, ok, err := store.Get(ctx, canonicalID)
	if err != nil {
		return ShowResult{}, err
	}
	if !ok {
		return ShowResult{}, fmt.Errorf("discovery record %q not found", canonicalID)
	}
	entry, err := s.buildEntry(ctx, record, now)
	if err != nil {
		return ShowResult{}, err
	}
	return ShowResult{
		Record:  entry,
		ShownAt: now.Format(time.RFC3339Nano),
	}, nil
}

func (s *QueryService) Refresh(ctx context.Context, opts RefreshOptions) (RefreshResult, error) {
	if err := s.validate(); err != nil {
		return RefreshResult{}, err
	}
	canonicalID := strings.TrimSpace(opts.CanonicalID)
	if canonicalID == "" {
		return RefreshResult{}, fmt.Errorf("canonical_id is required")
	}

	now := s.nowUTC()
	refreshed := false
	store := NewStoreWithDB(s.db, now)

	if s.presence != nil {
		view, err := s.presence.RefreshPeer(ctx, canonicalID)
		if err != nil {
			return RefreshResult{}, fmt.Errorf("refresh peer presence: %w", err)
		}
		record := s.recordFromPresence(canonicalID, view, now)
		if err := store.Upsert(ctx, record); err != nil {
			return RefreshResult{}, err
		}
		refreshed = true
	}

	record, ok, err := store.Get(ctx, canonicalID)
	if err != nil {
		return RefreshResult{}, err
	}
	if !ok {
		return RefreshResult{}, fmt.Errorf("discovery record %q not found", canonicalID)
	}
	entry, err := s.buildEntry(ctx, record, now)
	if err != nil {
		return RefreshResult{}, err
	}
	return RefreshResult{
		Record:      entry,
		Refreshed:   refreshed,
		RefreshedAt: now.Format(time.RFC3339Nano),
	}, nil
}

func (s *QueryService) buildEntry(ctx context.Context, record Record, now time.Time) (DiscoveryEntry, error) {
	routeTypes := deriveRouteTypes(record)
	freshness := EvaluateFreshness(now, record.ResolvedAt, record.FreshUntil, s.freshness)
	trustSummary, err := s.loadTrustSummary(ctx, record, routeTypes, freshness)
	if err != nil {
		return DiscoveryEntry{}, err
	}
	return DiscoveryEntry{
		CanonicalID:           strings.TrimSpace(record.CanonicalID),
		PeerID:                strings.TrimSpace(record.PeerID),
		Reachable:             record.Reachable,
		RouteTypes:            routeTypes,
		RouteCandidates:       record.RouteCandidates,
		TransportCapabilities: normalizeCapabilities(record.TransportCapabilities),
		DirectHints:           normalizeStringList(record.DirectHints),
		StoreForwardHints:     normalizeStringList(record.StoreForwardHints),
		SignedPeerRecord:      strings.TrimSpace(record.SignedPeerRecord),
		Source:                strings.TrimSpace(record.Source),
		SourceRank:            s.sourceRanking.Rank(record.Source),
		ResolvedAt:            strings.TrimSpace(record.ResolvedAt),
		FreshUntil:            strings.TrimSpace(record.FreshUntil),
		AnnouncedAt:           strings.TrimSpace(record.AnnouncedAt),
		Freshness:             freshness,
		TrustSummary:          trustSummary,
	}, nil
}

func (s *QueryService) recordFromPresence(canonicalID string, view PeerPresenceView, now time.Time) Record {
	if strings.TrimSpace(view.CanonicalID) != "" {
		canonicalID = strings.TrimSpace(view.CanonicalID)
	}
	resolvedAt := ""
	if !view.ResolvedAt.IsZero() {
		resolvedAt = view.ResolvedAt.UTC().Format(time.RFC3339Nano)
	} else {
		resolvedAt = now.Format(time.RFC3339Nano)
	}

	freshUntil := ""
	if !view.FreshUntil.IsZero() {
		freshUntil = view.FreshUntil.UTC().Format(time.RFC3339Nano)
	} else if ts, ok := parseTimestamp(resolvedAt); ok {
		freshUntil = ts.Add(s.defaultRefresh).Format(time.RFC3339Nano)
	}

	announcedAt := ""
	if !view.AnnouncedAt.IsZero() {
		announcedAt = view.AnnouncedAt.UTC().Format(time.RFC3339Nano)
	}

	transportCaps := normalizeCapabilities(view.TransportCapabilities)
	if len(view.DirectHints) > 0 {
		transportCaps = appendUniqueCapability(transportCaps, string(transport.RouteTypeDirect))
	}
	if len(view.StoreForwardHints) > 0 {
		transportCaps = appendUniqueCapability(transportCaps, string(transport.RouteTypeStoreForward))
	}
	for _, route := range view.RouteCandidates {
		transportCaps = appendUniqueCapability(transportCaps, string(route.Type))
	}

	source := strings.TrimSpace(view.Source)
	if source == "" {
		source = "refresh"
	}
	return Record{
		CanonicalID:           canonicalID,
		PeerID:                strings.TrimSpace(view.PeerID),
		RouteCandidates:       view.RouteCandidates,
		TransportCapabilities: transportCaps,
		DirectHints:           normalizeStringList(view.DirectHints),
		StoreForwardHints:     normalizeStringList(view.StoreForwardHints),
		SignedPeerRecord:      strings.TrimSpace(view.SignedPeerRecord),
		Source:                source,
		Reachable:             view.Reachable,
		ResolvedAt:            resolvedAt,
		FreshUntil:            freshUntil,
		AnnouncedAt:           announcedAt,
	}
}

type trustSnapshot struct {
	TrustLevel        string
	RiskFlags         []string
	VerificationState string
	Source            string
	DecidedAt         string
	UpdatedAt         string
	CreatedAt         string
}

func (s *QueryService) loadTrustSummary(ctx context.Context, record Record, routeTypes []string, freshness Freshness) (TrustSummary, error) {
	snapshot, ok, err := s.loadTrustSnapshot(ctx, record.CanonicalID)
	if err != nil {
		return TrustSummary{}, err
	}

	trustLevel := "unknown"
	if ok {
		trustLevel = defaultTrustLevel(snapshot.TrustLevel)
	}
	reachability := "unknown"
	if strings.TrimSpace(record.CanonicalID) != "" || len(routeTypes) > 0 || strings.TrimSpace(record.Source) != "" {
		if record.Reachable {
			reachability = "reachable"
		} else {
			reachability = "unreachable"
		}
	}

	confidenceScore := evaluateTrustConfidence(trustLevel, snapshot.VerificationState, snapshot.RiskFlags, record.Reachable, freshness.State)
	confidenceLevel := confidenceLevel(confidenceScore)
	asOf := firstNonEmpty(
		strings.TrimSpace(snapshot.DecidedAt),
		strings.TrimSpace(snapshot.UpdatedAt),
		strings.TrimSpace(snapshot.CreatedAt),
		strings.TrimSpace(record.ResolvedAt),
		strings.TrimSpace(record.UpdatedAt),
		strings.TrimSpace(record.CreatedAt),
	)

	status := strings.Join([]string{trustLevel, confidenceLevel, reachability}, "|")
	return TrustSummary{
		CanonicalID:       strings.TrimSpace(record.CanonicalID),
		TrustLevel:        trustLevel,
		VerificationState: strings.TrimSpace(snapshot.VerificationState),
		ConfidenceScore:   confidenceScore,
		ConfidenceLevel:   confidenceLevel,
		Reachability:      reachability,
		RiskFlags:         normalizeStringList(snapshot.RiskFlags),
		Source:            firstNonEmpty(strings.TrimSpace(snapshot.Source), strings.TrimSpace(record.Source)),
		AsOf:              asOf,
		Status:            status,
	}, nil
}

func (s *QueryService) loadTrustSnapshot(ctx context.Context, canonicalID string) (trustSnapshot, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT trust_level, risk_flags_json, verification_state, source, decided_at, updated_at, created_at
		FROM runtime_trust_records
		WHERE canonical_id = ?
	`,
		strings.TrimSpace(canonicalID),
	)

	var snapshot trustSnapshot
	var riskFlagsJSON string
	if err := row.Scan(
		&snapshot.TrustLevel,
		&riskFlagsJSON,
		&snapshot.VerificationState,
		&snapshot.Source,
		&snapshot.DecidedAt,
		&snapshot.UpdatedAt,
		&snapshot.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return trustSnapshot{}, false, nil
		}
		return trustSnapshot{}, false, fmt.Errorf("query runtime trust record: %w", err)
	}
	var decoded []string
	if err := decodeStringArray(riskFlagsJSON, &decoded); err != nil {
		return trustSnapshot{}, false, fmt.Errorf("decode runtime trust risk_flags_json: %w", err)
	}
	snapshot.RiskFlags = decoded
	return snapshot, true, nil
}

func (s *QueryService) nowUTC() time.Time {
	nowFn := s.now
	if nowFn == nil {
		nowFn = time.Now
	}
	return nowFn().UTC()
}

func (s *QueryService) validate() error {
	if s == nil {
		return fmt.Errorf("discovery query service is nil")
	}
	if s.db == nil {
		return fmt.Errorf("discovery query service database is not configured")
	}
	return nil
}

func normalizeRequestedCapabilities(single string, many []string) []string {
	values := make([]string, 0, len(many)+1)
	if strings.TrimSpace(single) != "" {
		values = append(values, single)
	}
	values = append(values, many...)
	return normalizeCapabilities(values)
}

func normalizeCapabilities(values []string) []string {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		capability := normalizeCapability(value)
		if capability == "" {
			continue
		}
		normalized = append(normalized, capability)
	}
	return normalizeStringList(normalized)
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

func matchesCapabilities(record Record, required []string) bool {
	if len(required) == 0 {
		return true
	}
	caps := capabilitySet(deriveRouteTypes(record))
	for _, capability := range required {
		if _, ok := caps[capability]; !ok {
			return false
		}
	}
	return true
}

func deriveRouteTypes(record Record) []string {
	values := make([]string, 0, len(record.RouteCandidates)+len(record.TransportCapabilities)+2)
	for _, route := range record.RouteCandidates {
		values = append(values, string(route.Type))
	}
	values = append(values, record.TransportCapabilities...)
	if len(record.DirectHints) > 0 {
		values = append(values, string(transport.RouteTypeDirect))
	}
	if len(record.StoreForwardHints) > 0 {
		values = append(values, string(transport.RouteTypeStoreForward))
	}
	return normalizeCapabilities(values)
}

func capabilitySet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func compareEntries(left, right DiscoveryEntry) bool {
	leftFreshness := freshnessWeight(left.Freshness.State)
	rightFreshness := freshnessWeight(right.Freshness.State)
	if leftFreshness != rightFreshness {
		return leftFreshness > rightFreshness
	}

	if left.SourceRank != right.SourceRank {
		return left.SourceRank > right.SourceRank
	}

	if left.Reachable != right.Reachable {
		return left.Reachable
	}

	if left.TrustSummary.ConfidenceScore != right.TrustSummary.ConfidenceScore {
		return left.TrustSummary.ConfidenceScore > right.TrustSummary.ConfidenceScore
	}

	if strings.TrimSpace(left.CanonicalID) != strings.TrimSpace(right.CanonicalID) {
		return strings.TrimSpace(left.CanonicalID) < strings.TrimSpace(right.CanonicalID)
	}

	return strings.TrimSpace(left.Source) < strings.TrimSpace(right.Source)
}

func appendUniqueCapability(values []string, value string) []string {
	value = normalizeCapability(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if normalizeCapability(existing) == value {
			return values
		}
	}
	return append(values, value)
}

func evaluateTrustConfidence(trustLevel, verificationState string, riskFlags []string, reachable bool, freshnessState string) float64 {
	trustLevel = defaultTrustLevel(trustLevel)
	verificationState = strings.ToLower(strings.TrimSpace(verificationState))
	freshnessState = strings.ToLower(strings.TrimSpace(freshnessState))
	riskFlags = normalizeStringList(riskFlags)

	baseByTrust := map[string]float64{
		"unknown":  0.35,
		"seen":     0.50,
		"verified": 0.70,
		"trusted":  0.85,
		"pinned":   0.95,
	}
	verificationAdjust := map[string]float64{
		"discovered": -0.08,
		"resolved":   0.02,
		"consistent": 0.10,
		"mismatch":   -0.45,
	}
	riskPenalty := map[string]float64{
		"manual":      0.01,
		"fixture":     0.01,
		"unverified":  0.12,
		"mismatch":    0.35,
		"spoofing":    0.40,
		"impostor":    0.35,
		"phishing":    0.35,
		"suspended":   0.30,
		"compromised": 0.45,
	}

	score := baseByTrust[trustLevel]
	if adjust, ok := verificationAdjust[verificationState]; ok {
		score += adjust
	}
	if reachable {
		score += 0.05
	} else {
		score -= 0.04
	}
	switch freshnessState {
	case FreshnessStateFresh:
		score += 0.04
	case FreshnessStateStale:
		score -= 0.06
	case FreshnessStateExpired:
		score -= 0.12
	}
	for _, risk := range riskFlags {
		penalty := 0.05
		if specific, ok := riskPenalty[risk]; ok {
			penalty = specific
		}
		score -= penalty
	}

	score = clamp(score, 0, 1)
	if verificationState == "mismatch" && score > 0.35 {
		score = 0.35
	}
	return roundTo(score, 4)
}

func defaultTrustLevel(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "unknown", "seen", "verified", "trusted", "pinned":
		return value
	default:
		return "unknown"
	}
}

func confidenceLevel(score float64) string {
	switch {
	case score >= 0.80:
		return "high"
	case score >= 0.55:
		return "medium"
	default:
		return "low"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func clamp(value, minValue, maxValue float64) float64 {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func roundTo(value float64, digits int) float64 {
	if digits <= 0 {
		return math.Round(value)
	}
	multiplier := math.Pow(10, float64(digits))
	return math.Round(value*multiplier) / multiplier
}
