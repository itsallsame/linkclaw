package trust

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/discovery"
)

type Service struct {
	db             *sql.DB
	trustStore     *Store
	discoveryStore *discovery.Store
	policy         Policy
	now            func() time.Time
}

type TrustConfidence struct {
	Score   float64  `json:"score"`
	Level   string   `json:"level"`
	Factors []string `json:"factors,omitempty"`
}

type TrustDiscovery struct {
	CanonicalID         string   `json:"canonical_id,omitempty"`
	PeerID              string   `json:"peer_id,omitempty"`
	Reachable           bool     `json:"reachable"`
	RouteTypes          []string `json:"route_types,omitempty"`
	TransportCaps       []string `json:"transport_capabilities,omitempty"`
	DirectHints         []string `json:"direct_hints,omitempty"`
	StoreForwardHints   []string `json:"store_forward_hints,omitempty"`
	Source              string   `json:"source,omitempty"`
	ResolvedAt          string   `json:"resolved_at,omitempty"`
	FreshUntil          string   `json:"fresh_until,omitempty"`
	AnnouncedAt         string   `json:"announced_at,omitempty"`
	HasSignedPeerRecord bool     `json:"has_signed_peer_record"`
}

type TrustProfile struct {
	CanonicalID       string          `json:"canonical_id"`
	ContactID         string          `json:"contact_id,omitempty"`
	TrustLevel        string          `json:"trust_level"`
	RiskFlags         []string        `json:"risk_flags,omitempty"`
	VerificationState string          `json:"verification_state,omitempty"`
	DecisionReason    string          `json:"decision_reason,omitempty"`
	Source            string          `json:"source,omitempty"`
	DecidedAt         string          `json:"decided_at,omitempty"`
	UpdatedAt         string          `json:"updated_at,omitempty"`
	CreatedAt         string          `json:"created_at,omitempty"`
	Discovery         TrustDiscovery  `json:"discovery"`
	Confidence        TrustConfidence `json:"confidence"`
	Summary           TrustSummary    `json:"summary"`
}

type trustEvent struct {
	CanonicalID       string
	ContactID         string
	TrustLevel        string
	RiskFlags         []string
	VerificationState string
	DecisionReason    string
	Source            string
	DecidedAt         string
	CreatedAt         string
}

func NewService(db *sql.DB) *Service {
	return NewServiceWithClock(db, time.Now)
}

func NewServiceWithDB(db *sql.DB, now time.Time) *Service {
	return NewServiceWithClock(db, func() time.Time { return now.UTC() })
}

func NewServiceWithClock(db *sql.DB, nowFn func() time.Time) *Service {
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn().UTC()
	return &Service{
		db:             db,
		trustStore:     NewStoreWithDB(db, now),
		discoveryStore: discovery.NewStoreWithDB(db, now),
		policy:         DefaultPolicy(),
		now:            nowFn,
	}
}

func (s *Service) Profile(ctx context.Context, canonicalID string) (TrustProfile, bool, error) {
	if s == nil {
		return TrustProfile{}, false, fmt.Errorf("trust service is nil")
	}
	if s.db == nil || s.trustStore == nil || s.discoveryStore == nil {
		return TrustProfile{}, false, fmt.Errorf("trust service is not configured")
	}

	canonicalID = strings.TrimSpace(canonicalID)
	if canonicalID == "" {
		return TrustProfile{}, false, fmt.Errorf("canonical_id is required")
	}

	trustRecord, hasTrust, err := s.trustStore.Get(ctx, canonicalID)
	if err != nil {
		return TrustProfile{}, false, err
	}
	discoveryRecord, hasDiscovery, err := s.discoveryStore.Get(ctx, canonicalID)
	if err != nil {
		return TrustProfile{}, false, err
	}
	event, hasEvent, err := s.latestTrustEvent(ctx, canonicalID)
	if err != nil {
		return TrustProfile{}, false, err
	}
	if !hasTrust && !hasDiscovery && !hasEvent {
		return TrustProfile{}, false, nil
	}

	profile := TrustProfile{
		CanonicalID: canonicalID,
		TrustLevel:  "unknown",
	}
	if hasTrust {
		profile = applyStoreRecord(profile, trustRecord)
	}
	if hasEvent && shouldUseEvent(hasTrust, trustRecord, event) {
		profile = applyEvent(profile, event)
	}
	if profile.TrustLevel == "" {
		profile.TrustLevel = "unknown"
	}
	profile.RiskFlags = normalizeStringList(profile.RiskFlags)

	profile.Discovery = buildDiscovery(discoveryRecord, hasDiscovery)

	policyResult := s.policy.Evaluate(PolicyInput{
		TrustLevel:        profile.TrustLevel,
		VerificationState: profile.VerificationState,
		RiskFlags:         profile.RiskFlags,
		Source:            profile.Source,
		HasDiscoveryData:  hasDiscovery,
		Reachable:         profile.Discovery.Reachable,
		DiscoveryFresh:    s.isDiscoveryFresh(discoveryRecord, hasDiscovery),
		RouteTypes:        profile.Discovery.RouteTypes,
		HasSignedPeer:     profile.Discovery.HasSignedPeerRecord,
	})
	profile.Confidence = TrustConfidence{
		Score:   policyResult.Score,
		Level:   policyResult.Level,
		Factors: policyResult.Factors,
	}
	profile.Summary = BuildTrustSummary(profile)

	return profile, true, nil
}

func (s *Service) GetProfile(ctx context.Context, canonicalID string) (TrustProfile, bool, error) {
	return s.Profile(ctx, canonicalID)
}

func (s *Service) Summary(ctx context.Context, canonicalID string) (TrustSummary, bool, error) {
	profile, ok, err := s.Profile(ctx, canonicalID)
	if err != nil || !ok {
		return TrustSummary{}, ok, err
	}
	return profile.Summary, true, nil
}

func (s *Service) GetSummary(ctx context.Context, canonicalID string) (TrustSummary, bool, error) {
	return s.Summary(ctx, canonicalID)
}

func (s *Service) latestTrustEvent(ctx context.Context, canonicalID string) (trustEvent, bool, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT
			canonical_id, contact_id, trust_level, risk_flags_json, verification_state,
			decision_reason, source, decided_at, created_at
		FROM trust_events
		WHERE canonical_id = ?
		ORDER BY
			CASE WHEN decided_at <> '' THEN decided_at ELSE created_at END DESC,
			created_at DESC,
			event_id DESC
		LIMIT 1
	`,
		canonicalID,
	)

	var event trustEvent
	var riskFlagsJSON string
	if err := row.Scan(
		&event.CanonicalID,
		&event.ContactID,
		&event.TrustLevel,
		&riskFlagsJSON,
		&event.VerificationState,
		&event.DecisionReason,
		&event.Source,
		&event.DecidedAt,
		&event.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return trustEvent{}, false, nil
		}
		return trustEvent{}, false, fmt.Errorf("query latest trust event: %w", err)
	}
	riskFlags, err := decodeStringArray(riskFlagsJSON)
	if err != nil {
		return trustEvent{}, false, fmt.Errorf("decode trust event risk flags: %w", err)
	}
	event.RiskFlags = riskFlags
	return event, true, nil
}

func shouldUseEvent(hasTrust bool, record Record, event trustEvent) bool {
	if !hasTrust {
		return true
	}
	eventTS := parseFirstTimestamp(event.DecidedAt, event.CreatedAt)
	recordTS := parseFirstTimestamp(record.DecidedAt, record.UpdatedAt, record.CreatedAt)
	if recordTS.IsZero() {
		return true
	}
	if eventTS.IsZero() {
		return false
	}
	return !eventTS.Before(recordTS)
}

func applyStoreRecord(profile TrustProfile, record Record) TrustProfile {
	profile.ContactID = strings.TrimSpace(record.ContactID)
	profile.TrustLevel = defaultString(record.TrustLevel, "unknown")
	profile.RiskFlags = normalizeStringList(record.RiskFlags)
	profile.VerificationState = strings.TrimSpace(record.VerificationState)
	profile.DecisionReason = strings.TrimSpace(record.DecisionReason)
	profile.Source = strings.TrimSpace(record.Source)
	profile.DecidedAt = strings.TrimSpace(record.DecidedAt)
	profile.UpdatedAt = strings.TrimSpace(record.UpdatedAt)
	profile.CreatedAt = strings.TrimSpace(record.CreatedAt)
	return profile
}

func applyEvent(profile TrustProfile, event trustEvent) TrustProfile {
	if contactID := strings.TrimSpace(event.ContactID); contactID != "" {
		profile.ContactID = contactID
	}
	profile.TrustLevel = defaultString(event.TrustLevel, "unknown")
	profile.RiskFlags = normalizeStringList(event.RiskFlags)
	profile.VerificationState = strings.TrimSpace(event.VerificationState)
	profile.DecisionReason = strings.TrimSpace(event.DecisionReason)
	profile.Source = strings.TrimSpace(event.Source)
	profile.DecidedAt = firstNonEmpty(event.DecidedAt, event.CreatedAt, profile.DecidedAt)
	return profile
}

func buildDiscovery(record discovery.Record, hasDiscovery bool) TrustDiscovery {
	if !hasDiscovery {
		return TrustDiscovery{}
	}
	return TrustDiscovery{
		CanonicalID:         strings.TrimSpace(record.CanonicalID),
		PeerID:              strings.TrimSpace(record.PeerID),
		Reachable:           record.Reachable,
		RouteTypes:          routeTypes(record),
		TransportCaps:       normalizeCapabilityList(record.TransportCapabilities),
		DirectHints:         normalizeStringList(record.DirectHints),
		StoreForwardHints:   normalizeStringList(record.StoreForwardHints),
		Source:              strings.TrimSpace(record.Source),
		ResolvedAt:          strings.TrimSpace(record.ResolvedAt),
		FreshUntil:          strings.TrimSpace(record.FreshUntil),
		AnnouncedAt:         strings.TrimSpace(record.AnnouncedAt),
		HasSignedPeerRecord: strings.TrimSpace(record.SignedPeerRecord) != "",
	}
}

func (s *Service) isDiscoveryFresh(record discovery.Record, hasDiscovery bool) bool {
	if !hasDiscovery {
		return false
	}
	nowFn := s.now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn().UTC()

	if freshUntil := parseFirstTimestamp(record.FreshUntil); !freshUntil.IsZero() {
		return !freshUntil.Before(now)
	}
	if resolvedAt := parseFirstTimestamp(record.ResolvedAt); !resolvedAt.IsZero() {
		return now.Sub(resolvedAt) <= 24*time.Hour
	}
	return false
}

func routeTypes(record discovery.Record) []string {
	values := make([]string, 0, len(record.RouteCandidates)+len(record.TransportCapabilities))
	for _, route := range record.RouteCandidates {
		trimmed := strings.TrimSpace(string(route.Type))
		if trimmed == "" {
			continue
		}
		values = append(values, normalizeCapability(trimmed))
	}
	for _, capability := range record.TransportCapabilities {
		trimmed := normalizeCapability(capability)
		if trimmed == "" {
			continue
		}
		values = append(values, trimmed)
	}
	return normalizeCapabilityList(values)
}

func normalizeCapabilityList(values []string) []string {
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
		return "direct"
	case "store_forward", "storeforward", "store-forward", "relay", "recovery":
		return "store_forward"
	default:
		return value
	}
}

func parseFirstTimestamp(values ...string) time.Time {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			continue
		}
		return ts.UTC()
	}
	return time.Time{}
}
