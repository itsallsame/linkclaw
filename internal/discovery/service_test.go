package discovery

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

func TestQueryServiceFindFiltersByCapabilityAndRanksByPolicy(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openStoreDB(t)
	defer db.Close()

	now := time.Date(2026, 3, 23, 9, 0, 0, 0, time.UTC)
	store := NewStoreWithDB(db, now)

	mustUpsertDiscovery(t, ctx, store, Record{
		CanonicalID:           "did:key:alpha",
		PeerID:                "peer-alpha",
		RouteCandidates:       []transport.RouteCandidate{{Type: transport.RouteTypeDirect, Label: "alpha", Priority: 100, Target: "libp2p://alpha"}},
		TransportCapabilities: []string{"direct"},
		DirectHints:           []string{"libp2p://alpha"},
		Source:                "refresh",
		Reachable:             true,
		ResolvedAt:            now.Add(-10 * time.Minute).Format(time.RFC3339Nano),
		FreshUntil:            now.Add(20 * time.Minute).Format(time.RFC3339Nano),
	})
	mustUpsertDiscovery(t, ctx, store, Record{
		CanonicalID:           "did:key:beta",
		PeerID:                "peer-beta",
		RouteCandidates:       []transport.RouteCandidate{{Type: transport.RouteTypeDirect, Label: "beta", Priority: 90, Target: "libp2p://beta"}},
		TransportCapabilities: []string{"direct"},
		DirectHints:           []string{"libp2p://beta"},
		Source:                "import",
		Reachable:             true,
		ResolvedAt:            now.Add(-15 * time.Minute).Format(time.RFC3339Nano),
		FreshUntil:            now.Add(15 * time.Minute).Format(time.RFC3339Nano),
	})
	mustUpsertDiscovery(t, ctx, store, Record{
		CanonicalID:           "did:key:gamma",
		PeerID:                "peer-gamma",
		RouteCandidates:       []transport.RouteCandidate{{Type: transport.RouteTypeDirect, Label: "gamma", Priority: 70, Target: "libp2p://gamma"}},
		TransportCapabilities: []string{"direct"},
		DirectHints:           []string{"libp2p://gamma"},
		Source:                "dht",
		Reachable:             false,
		ResolvedAt:            now.Add(-6 * time.Hour).Format(time.RFC3339Nano),
		FreshUntil:            now.Add(-2 * time.Hour).Format(time.RFC3339Nano),
	})

	mustInsertRuntimeTrustRecord(t, db, now, "did:key:alpha", "verified", "consistent", `["manual"]`, "import")
	mustInsertRuntimeTrustRecord(t, db, now, "did:key:beta", "trusted", "consistent", `[]`, "known-trust")
	mustInsertRuntimeTrustRecord(t, db, now, "did:key:gamma", "seen", "resolved", `[]`, "import")

	service := NewQueryServiceWithDB(db, now, nil)
	result, err := service.Find(ctx, FindOptions{Capability: "direct"})
	if err != nil {
		t.Fatalf("Find() error = %v", err)
	}
	if got, want := len(result.Records), 3; got != want {
		t.Fatalf("len(result.Records) = %d, want %d", got, want)
	}

	if got, want := result.Records[0].CanonicalID, "did:key:alpha"; got != want {
		t.Fatalf("result.Records[0].CanonicalID = %q, want %q", got, want)
	}
	if got, want := result.Records[1].CanonicalID, "did:key:beta"; got != want {
		t.Fatalf("result.Records[1].CanonicalID = %q, want %q", got, want)
	}
	if got, want := result.Records[2].CanonicalID, "did:key:gamma"; got != want {
		t.Fatalf("result.Records[2].CanonicalID = %q, want %q", got, want)
	}
	if result.Records[0].Freshness.State != FreshnessStateFresh {
		t.Fatalf("result.Records[0].Freshness.State = %q, want %q", result.Records[0].Freshness.State, FreshnessStateFresh)
	}
	if result.Records[2].Freshness.State != FreshnessStateStale {
		t.Fatalf("result.Records[2].Freshness.State = %q, want %q", result.Records[2].Freshness.State, FreshnessStateStale)
	}
	if !(result.Records[0].SourceRank > result.Records[1].SourceRank) {
		t.Fatalf("source rank order unexpected: first=%d second=%d", result.Records[0].SourceRank, result.Records[1].SourceRank)
	}
	if got, want := result.Records[0].TrustSummary.TrustLevel, "verified"; got != want {
		t.Fatalf("result.Records[0].TrustSummary.TrustLevel = %q, want %q", got, want)
	}
	if got, want := result.Records[0].TrustSummary.CanonicalID, "did:key:alpha"; got != want {
		t.Fatalf("result.Records[0].TrustSummary.CanonicalID = %q, want %q", got, want)
	}
	if got, want := result.Records[0].TrustSummary.Reachability, "reachable"; got != want {
		t.Fatalf("result.Records[0].TrustSummary.Reachability = %q, want %q", got, want)
	}
}

func TestQueryServiceFindFiltersBySourceIncludingUnknownAndLegacyAliases(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openStoreDB(t)
	defer db.Close()

	now := time.Date(2026, 3, 23, 9, 5, 0, 0, time.UTC)
	store := NewStoreWithDB(db, now)

	mustUpsertDiscovery(t, ctx, store, Record{
		CanonicalID:           "did:key:known",
		PeerID:                "peer-known",
		RouteCandidates:       []transport.RouteCandidate{{Type: transport.RouteTypeDirect, Label: "known", Priority: 100, Target: "libp2p://known"}},
		TransportCapabilities: []string{"direct"},
		DirectHints:           []string{"libp2p://known"},
		Source:                "import",
		Reachable:             true,
		ResolvedAt:            now.Add(-5 * time.Minute).Format(time.RFC3339Nano),
		FreshUntil:            now.Add(10 * time.Minute).Format(time.RFC3339Nano),
	})
	mustUpsertDiscovery(t, ctx, store, Record{
		CanonicalID:           "did:key:unknown",
		PeerID:                "peer-unknown",
		RouteCandidates:       []transport.RouteCandidate{{Type: transport.RouteTypeStoreForward, Label: "unknown", Priority: 80, Target: "nostr://relay/unknown"}},
		TransportCapabilities: []string{"store_forward"},
		StoreForwardHints:     []string{"nostr://relay/unknown"},
		Source:                "legacy-custom-source",
		Reachable:             false,
		ResolvedAt:            now.Add(-10 * time.Minute).Format(time.RFC3339Nano),
		FreshUntil:            now.Add(5 * time.Minute).Format(time.RFC3339Nano),
	})
	mustUpsertDiscovery(t, ctx, store, Record{
		CanonicalID:           "did:key:cache",
		PeerID:                "peer-cache",
		RouteCandidates:       []transport.RouteCandidate{{Type: transport.RouteTypeStoreForward, Label: "cache", Priority: 70, Target: "nostr://relay/cache"}},
		TransportCapabilities: []string{"store_forward"},
		StoreForwardHints:     []string{"nostr://relay/cache"},
		Source:                "stale-cache",
		Reachable:             false,
		ResolvedAt:            now.Add(-8 * time.Minute).Format(time.RFC3339Nano),
		FreshUntil:            now.Add(2 * time.Minute).Format(time.RFC3339Nano),
	})

	service := NewQueryServiceWithDB(db, now, nil)

	unknownResult, err := service.Find(ctx, FindOptions{Source: "unknown"})
	if err != nil {
		t.Fatalf("Find(source=unknown) error = %v", err)
	}
	if got, want := len(unknownResult.Records), 1; got != want {
		t.Fatalf("len(unknownResult.Records) = %d, want %d", got, want)
	}
	if got, want := unknownResult.Records[0].CanonicalID, "did:key:unknown"; got != want {
		t.Fatalf("unknownResult.Records[0].CanonicalID = %q, want %q", got, want)
	}
	if got, want := unknownResult.Records[0].Source, SourceUnknown; got != want {
		t.Fatalf("unknownResult.Records[0].Source = %q, want %q", got, want)
	}

	cacheResult, err := service.Find(ctx, FindOptions{Source: "cache"})
	if err != nil {
		t.Fatalf("Find(source=cache) error = %v", err)
	}
	if got, want := len(cacheResult.Records), 1; got != want {
		t.Fatalf("len(cacheResult.Records) = %d, want %d", got, want)
	}
	if got, want := cacheResult.Records[0].CanonicalID, "did:key:cache"; got != want {
		t.Fatalf("cacheResult.Records[0].CanonicalID = %q, want %q", got, want)
	}
	if got, want := cacheResult.Records[0].Source, SourceCache; got != want {
		t.Fatalf("cacheResult.Records[0].Source = %q, want %q", got, want)
	}

	cacheAliasResult, err := service.Find(ctx, FindOptions{Source: "stale-cache"})
	if err != nil {
		t.Fatalf("Find(source=stale-cache) error = %v", err)
	}
	if got, want := len(cacheAliasResult.Records), 1; got != want {
		t.Fatalf("len(cacheAliasResult.Records) = %d, want %d", got, want)
	}
	if got, want := cacheAliasResult.Query.Source, SourceCache; got != want {
		t.Fatalf("cacheAliasResult.Query.Source = %q, want %q", got, want)
	}

	importResult, err := service.Find(ctx, FindOptions{Source: "import"})
	if err != nil {
		t.Fatalf("Find(source=import) error = %v", err)
	}
	if got, want := len(importResult.Records), 1; got != want {
		t.Fatalf("len(importResult.Records) = %d, want %d", got, want)
	}
	if got, want := importResult.Records[0].CanonicalID, "did:key:known"; got != want {
		t.Fatalf("importResult.Records[0].CanonicalID = %q, want %q", got, want)
	}

	_, err = service.Find(ctx, FindOptions{Source: "future-source"})
	if err == nil {
		t.Fatal("Find(source=future-source) error = nil, want validation error")
	}
	if !strings.Contains(err.Error(), "unsupported discovery source filter") {
		t.Fatalf("Find(source=future-source) error = %v, want unsupported filter error", err)
	}
}

func TestQueryServiceShowReturnsNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openStoreDB(t)
	defer db.Close()

	service := NewQueryServiceWithDB(db, time.Date(2026, 3, 23, 9, 10, 0, 0, time.UTC), nil)
	_, err := service.Show(ctx, ShowOptions{CanonicalID: "did:key:missing"})
	if err == nil {
		t.Fatal("Show() error = nil, want not found error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("Show() error = %v, want not found", err)
	}
}

func TestQueryServiceRefreshUsesPresenceResolver(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openStoreDB(t)
	defer db.Close()

	now := time.Date(2026, 3, 23, 9, 30, 0, 0, time.UTC)
	store := NewStoreWithDB(db, now)
	mustUpsertDiscovery(t, ctx, store, Record{
		CanonicalID:           "did:key:delta",
		PeerID:                "peer-delta-old",
		TransportCapabilities: []string{"store_forward"},
		StoreForwardHints:     []string{"sf://delta"},
		Source:                "import",
		Reachable:             false,
		ResolvedAt:            now.Add(-3 * time.Hour).Format(time.RFC3339Nano),
		FreshUntil:            now.Add(-2 * time.Hour).Format(time.RFC3339Nano),
	})
	mustInsertRuntimeTrustRecord(t, db, now, "did:key:delta", "trusted", "consistent", `[]`, "known-trust")

	resolver := stubPresenceResolver{
		view: PeerPresenceView{
			CanonicalID:           "did:key:delta",
			PeerID:                "peer-delta-new",
			Reachable:             true,
			RouteCandidates:       []transport.RouteCandidate{{Type: transport.RouteTypeDirect, Label: "delta-direct", Priority: 100, Target: "libp2p://delta"}},
			TransportCapabilities: []string{"direct"},
			DirectHints:           []string{"libp2p://delta"},
			ResolvedAt:            now,
			FreshUntil:            now.Add(30 * time.Minute),
			Source:                "libp2p",
		},
	}
	service := NewQueryServiceWithDB(db, now, resolver)
	result, err := service.Refresh(ctx, RefreshOptions{CanonicalID: "did:key:delta"})
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	if !result.Refreshed {
		t.Fatal("Refresh() Refreshed = false, want true")
	}
	if got, want := result.Record.PeerID, "peer-delta-new"; got != want {
		t.Fatalf("result.Record.PeerID = %q, want %q", got, want)
	}
	if got, want := result.Record.Source, "libp2p"; got != want {
		t.Fatalf("result.Record.Source = %q, want %q", got, want)
	}
	if !containsString(result.Record.RouteTypes, "direct") {
		t.Fatalf("result.Record.RouteTypes = %v, want direct", result.Record.RouteTypes)
	}
	if got, want := result.Record.Freshness.State, FreshnessStateFresh; got != want {
		t.Fatalf("result.Record.Freshness.State = %q, want %q", got, want)
	}

	stored, ok, err := NewStoreWithDB(db, now).Get(ctx, "did:key:delta")
	if err != nil {
		t.Fatalf("Store.Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Store.Get() ok = false, want true")
	}
	if got, want := stored.PeerID, "peer-delta-new"; got != want {
		t.Fatalf("stored.PeerID = %q, want %q", got, want)
	}
}

func TestEvaluateFreshnessStates(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 23, 10, 0, 0, 0, time.UTC)
	policy := DefaultFreshnessPolicy()

	fresh := EvaluateFreshness(now, now.Add(-5*time.Minute).Format(time.RFC3339Nano), now.Add(10*time.Minute).Format(time.RFC3339Nano), policy)
	if fresh.State != FreshnessStateFresh {
		t.Fatalf("fresh.State = %q, want %q", fresh.State, FreshnessStateFresh)
	}

	stale := EvaluateFreshness(now, now.Add(-2*time.Hour).Format(time.RFC3339Nano), now.Add(-30*time.Minute).Format(time.RFC3339Nano), policy)
	if stale.State != FreshnessStateStale {
		t.Fatalf("stale.State = %q, want %q", stale.State, FreshnessStateStale)
	}

	expired := EvaluateFreshness(now, now.Add(-2*time.Hour).Format(time.RFC3339Nano), now.Add(-26*time.Hour).Format(time.RFC3339Nano), policy)
	if expired.State != FreshnessStateExpired {
		t.Fatalf("expired.State = %q, want %q", expired.State, FreshnessStateExpired)
	}

	unknown := EvaluateFreshness(now, "", "", policy)
	if unknown.State != FreshnessStateUnknown {
		t.Fatalf("unknown.State = %q, want %q", unknown.State, FreshnessStateUnknown)
	}
}

type stubPresenceResolver struct {
	view PeerPresenceView
	err  error
}

func (s stubPresenceResolver) ResolvePeer(context.Context, string) (PeerPresenceView, error) {
	return s.view, s.err
}

func (s stubPresenceResolver) RefreshPeer(context.Context, string) (PeerPresenceView, error) {
	return s.view, s.err
}

func (s stubPresenceResolver) PublishSelf(context.Context) error {
	return s.err
}

func mustUpsertDiscovery(t *testing.T, ctx context.Context, store *Store, record Record) {
	t.Helper()
	if err := store.Upsert(ctx, record); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
}

func mustInsertRuntimeTrustRecord(t *testing.T, db *sql.DB, now time.Time, canonicalID, trustLevel, verificationState, riskFlagsJSON, source string) {
	t.Helper()
	stamp := now.Format(time.RFC3339Nano)
	if _, err := db.Exec(
		`INSERT INTO runtime_trust_records (
			canonical_id, contact_id, trust_level, risk_flags_json, verification_state,
			decision_reason, source, decided_at, updated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		canonicalID,
		"contact_"+strings.TrimPrefix(strings.ReplaceAll(canonicalID, ":", "_"), "did_key_"),
		trustLevel,
		riskFlagsJSON,
		verificationState,
		fmt.Sprintf("seed trust for %s", canonicalID),
		source,
		stamp,
		stamp,
		stamp,
	); err != nil {
		t.Fatalf("insert runtime trust record: %v", err)
	}
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == strings.TrimSpace(expected) {
			return true
		}
	}
	return false
}
