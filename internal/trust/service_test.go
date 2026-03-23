package trust

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/discovery"
	"github.com/xiewanpeng/claw-identity/internal/transport"
)

func TestServiceProfileBuildsFromRuntimeStores(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openStoreDB(t)
	defer db.Close()

	now := time.Date(2026, 3, 23, 7, 0, 0, 0, time.UTC)
	trustStore := NewStoreWithDB(db, now)
	if err := trustStore.Upsert(ctx, Record{
		CanonicalID:       "did:key:z6MkAlice",
		ContactID:         "contact_1",
		TrustLevel:        "verified",
		RiskFlags:         []string{"manual"},
		VerificationState: "consistent",
		DecisionReason:    "imported",
		Source:            "import",
		DecidedAt:         "2026-03-23T06:58:00Z",
	}); err != nil {
		t.Fatalf("trust Upsert() error = %v", err)
	}

	discoveryStore := discovery.NewStoreWithDB(db, now)
	if err := discoveryStore.Upsert(ctx, discovery.Record{
		CanonicalID:           "did:key:z6MkAlice",
		PeerID:                "lcpeer:alice",
		RouteCandidates:       []transport.RouteCandidate{{Type: transport.RouteTypeDirect, Label: "direct", Priority: 100, Target: "libp2p://lcpeer:alice"}, {Type: transport.RouteTypeStoreForward, Label: "sf", Priority: 30, Target: "sf://alice"}},
		TransportCapabilities: []string{"store_forward", "direct"},
		DirectHints:           []string{"libp2p://lcpeer:alice"},
		StoreForwardHints:     []string{"sf://alice"},
		SignedPeerRecord:      `{"peer_id":"lcpeer:alice"}`,
		Source:                "libp2p",
		Reachable:             true,
		ResolvedAt:            "2026-03-23T06:59:00Z",
		FreshUntil:            "2026-03-23T07:10:00Z",
	}); err != nil {
		t.Fatalf("discovery Upsert() error = %v", err)
	}

	service := NewServiceWithDB(db, now)
	profile, ok, err := service.Profile(ctx, "did:key:z6MkAlice")
	if err != nil {
		t.Fatalf("Profile() error = %v", err)
	}
	if !ok {
		t.Fatal("Profile() ok = false, want true")
	}
	if profile.TrustLevel != "verified" {
		t.Fatalf("profile.TrustLevel = %q, want verified", profile.TrustLevel)
	}
	if profile.Confidence.Level != ConfidenceLevelHigh {
		t.Fatalf("profile.Confidence.Level = %q, want high", profile.Confidence.Level)
	}
	if got, want := strings.Join(profile.Discovery.RouteTypes, ","), "direct,store_forward"; got != want {
		t.Fatalf("profile.Discovery.RouteTypes = %q, want %q", got, want)
	}
	if profile.Summary.Status != "verified|high|reachable" {
		t.Fatalf("profile.Summary.Status = %q", profile.Summary.Status)
	}

	summary, ok, err := service.Summary(ctx, "did:key:z6MkAlice")
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if !ok {
		t.Fatal("Summary() ok = false, want true")
	}
	if !reflect.DeepEqual(summary, profile.Summary) {
		t.Fatalf("Summary() mismatch with profile summary\nsummary=%#v\nprofile=%#v", summary, profile.Summary)
	}
}

func TestServiceProfilePrefersNewerTrustEvent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openStoreDB(t)
	defer db.Close()

	now := time.Date(2026, 3, 23, 8, 0, 0, 0, time.UTC)
	store := NewStoreWithDB(db, now)
	if err := store.Upsert(ctx, Record{
		CanonicalID:       "did:key:z6MkBob",
		ContactID:         "contact_2",
		TrustLevel:        "unknown",
		RiskFlags:         []string{},
		VerificationState: "resolved",
		DecisionReason:    "imported",
		Source:            "import",
		DecidedAt:         "2026-03-23T07:00:00Z",
	}); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO trust_events (
			event_id, trust_id, contact_id, canonical_id, trust_level, risk_flags_json,
			verification_state, decision_reason, source, decided_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"trust_event_1",
		"trust_1",
		"contact_2",
		"did:key:z6MkBob",
		"trusted",
		`["manual"]`,
		"consistent",
		"manual review",
		"known-trust",
		"2026-03-23T07:30:00Z",
		"2026-03-23T07:30:00Z",
	); err != nil {
		t.Fatalf("insert trust_event: %v", err)
	}

	service := NewServiceWithDB(db, now)
	profile, ok, err := service.Profile(ctx, "did:key:z6MkBob")
	if err != nil {
		t.Fatalf("Profile() error = %v", err)
	}
	if !ok {
		t.Fatal("Profile() ok = false, want true")
	}
	if profile.TrustLevel != "trusted" {
		t.Fatalf("profile.TrustLevel = %q, want trusted", profile.TrustLevel)
	}
	if profile.Source != "known-trust" {
		t.Fatalf("profile.Source = %q, want known-trust", profile.Source)
	}
	if profile.DecidedAt != "2026-03-23T07:30:00Z" {
		t.Fatalf("profile.DecidedAt = %q, want event decided_at", profile.DecidedAt)
	}
}

func TestServiceProfileNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openStoreDB(t)
	defer db.Close()

	service := NewServiceWithDB(db, time.Now().UTC())
	_, ok, err := service.Profile(ctx, "did:key:missing")
	if err != nil {
		t.Fatalf("Profile() error = %v", err)
	}
	if ok {
		t.Fatal("Profile() ok = true, want false")
	}
}

func TestDefaultPolicyMismatchDropsConfidence(t *testing.T) {
	t.Parallel()

	policy := DefaultPolicy()
	high := policy.Evaluate(PolicyInput{
		TrustLevel:        "trusted",
		VerificationState: "consistent",
		HasDiscoveryData:  true,
		Reachable:         true,
		DiscoveryFresh:    true,
		RouteTypes:        []string{"direct"},
		HasSignedPeer:     true,
	})
	low := policy.Evaluate(PolicyInput{
		TrustLevel:        "trusted",
		VerificationState: "mismatch",
		RiskFlags:         []string{"spoofing"},
		HasDiscoveryData:  true,
		Reachable:         false,
		DiscoveryFresh:    false,
		RouteTypes:        []string{"store_forward"},
	})

	if high.Level != ConfidenceLevelHigh {
		t.Fatalf("high.Level = %q, want high", high.Level)
	}
	if low.Level != ConfidenceLevelLow {
		t.Fatalf("low.Level = %q, want low", low.Level)
	}
	if !(high.Score > low.Score) {
		t.Fatalf("high.Score = %.4f, low.Score = %.4f, want high > low", high.Score, low.Score)
	}
}

func TestBuildTrustSummaryStable(t *testing.T) {
	t.Parallel()

	profile := TrustProfile{
		CanonicalID:       "did:key:z6MkStable",
		TrustLevel:        "trusted",
		RiskFlags:         []string{"spoofing", "manual", "spoofing"},
		VerificationState: "consistent",
		Source:            "known-trust",
		DecidedAt:         "2026-03-23T09:00:00Z",
		Discovery: TrustDiscovery{
			CanonicalID: "did:key:z6MkStable",
			Reachable:   true,
			RouteTypes:  []string{"store_forward", "direct", "direct"},
		},
		Confidence: TrustConfidence{
			Score: 0.91,
			Level: ConfidenceLevelHigh,
		},
	}

	first := BuildTrustSummary(profile)
	second := BuildTrustSummary(profile)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("summary is not stable\nfirst=%#v\nsecond=%#v", first, second)
	}
	if got, want := strings.Join(first.RouteTypes, ","), "direct,store_forward"; got != want {
		t.Fatalf("first.RouteTypes = %q, want %q", got, want)
	}
	if got, want := strings.Join(first.RiskFlags, ","), "manual,spoofing"; got != want {
		t.Fatalf("first.RiskFlags = %q, want %q", got, want)
	}
}
