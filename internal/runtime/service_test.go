package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/discovery"
	"github.com/xiewanpeng/claw-identity/internal/routing"
	"github.com/xiewanpeng/claw-identity/internal/transport"
	"github.com/xiewanpeng/claw-identity/internal/trust"
)

type stubDiscovery struct {
	view discovery.PeerPresenceView
	err  error
}

func (s stubDiscovery) ResolvePeer(context.Context, string) (discovery.PeerPresenceView, error) {
	return s.view, s.err
}
func (s stubDiscovery) RefreshPeer(context.Context, string) (discovery.PeerPresenceView, error) {
	return s.view, s.err
}
func (s stubDiscovery) PublishSelf(context.Context) error { return nil }

type refreshAwareDiscovery struct {
	resolveView  discovery.PeerPresenceView
	refreshView  discovery.PeerPresenceView
	resolveCalls int
	refreshCalls int
}

func (s *refreshAwareDiscovery) ResolvePeer(context.Context, string) (discovery.PeerPresenceView, error) {
	s.resolveCalls++
	return s.resolveView, nil
}

func (s *refreshAwareDiscovery) RefreshPeer(context.Context, string) (discovery.PeerPresenceView, error) {
	s.refreshCalls++
	return s.refreshView, nil
}

func (s *refreshAwareDiscovery) PublishSelf(context.Context) error { return nil }

type presenceDrivenPlanner struct {
	lastPresence discovery.PeerPresenceView
}

func (s *presenceDrivenPlanner) PlanSend(_ context.Context, _ routing.ContactRuntimeView, presence discovery.PeerPresenceView) ([]transport.RouteCandidate, error) {
	s.lastPresence = presence
	return append([]transport.RouteCandidate(nil), s.lastPresence.RouteCandidates...), nil
}

func (s *presenceDrivenPlanner) PlanRecover(context.Context, routing.ContactRuntimeView, discovery.PeerPresenceView) ([]transport.RouteCandidate, error) {
	return nil, nil
}

func (s *presenceDrivenPlanner) RecordOutcome(context.Context, routing.RouteOutcome) error {
	return nil
}

type stubPlanner struct {
	sendRoutes    []transport.RouteCandidate
	recoverRoutes []transport.RouteCandidate
	outcomes      []routing.RouteOutcome
}

func (s *stubPlanner) PlanSend(context.Context, routing.ContactRuntimeView, discovery.PeerPresenceView) ([]transport.RouteCandidate, error) {
	return s.sendRoutes, nil
}
func (s *stubPlanner) PlanRecover(context.Context, routing.ContactRuntimeView, discovery.PeerPresenceView) ([]transport.RouteCandidate, error) {
	return s.recoverRoutes, nil
}
func (s *stubPlanner) RecordOutcome(_ context.Context, outcome routing.RouteOutcome) error {
	s.outcomes = append(s.outcomes, outcome)
	return nil
}

type stubTransport struct {
	name       string
	routeType  transport.RouteType
	sendCalls  int
	syncCalls  int
	sendErr    error
	sendResult *transport.SendResult
	syncErr    error
	syncCount  int
	syncCursor string
	ackCalls   []string
	ackErr     error
}

func (s *stubTransport) Name() string { return s.name }
func (s *stubTransport) Supports(route transport.RouteCandidate) bool {
	return route.Type == s.routeType
}
func (s *stubTransport) Send(_ context.Context, env transport.Envelope, route transport.RouteCandidate) (transport.SendResult, error) {
	s.sendCalls++
	if s.sendErr != nil {
		return transport.SendResult{}, s.sendErr
	}
	if s.sendResult != nil {
		result := *s.sendResult
		if result.Route.Type == "" {
			result.Route = route
		}
		if result.RemoteID == "" {
			result.RemoteID = env.MessageID
		}
		return result, nil
	}
	return transport.SendResult{Route: route, Delivered: true, RemoteID: env.MessageID}, nil
}
func (s *stubTransport) Sync(_ context.Context, route transport.RouteCandidate) (transport.SyncResult, error) {
	s.syncCalls++
	if s.syncErr != nil {
		return transport.SyncResult{}, s.syncErr
	}
	return transport.SyncResult{Route: route, Recovered: s.syncCount, AdvancedCursor: s.syncCursor}, nil
}
func (s *stubTransport) Ack(_ context.Context, _ transport.RouteCandidate, cursor string) error {
	if s.ackErr != nil {
		return s.ackErr
	}
	s.ackCalls = append(s.ackCalls, cursor)
	return nil
}

type stubHooks struct {
	deliveries []DeliveryOutcomeEvent
	recoveries []RecoveryEvent
}

func (s *stubHooks) OnDeliveryOutcome(_ context.Context, event DeliveryOutcomeEvent) error {
	s.deliveries = append(s.deliveries, event)
	return nil
}
func (s *stubHooks) OnRecovery(_ context.Context, event RecoveryEvent) error {
	s.recoveries = append(s.recoveries, event)
	return nil
}
func (s *stubHooks) OnReputationSignal(context.Context, ReputationSignal) error { return nil }
func (s *stubHooks) OnPaymentIntent(context.Context, PaymentIntent) error       { return nil }
func (s *stubHooks) OnPenaltySignal(context.Context, PenaltySignal) error       { return nil }

type stubTrustInspector struct {
	profile trust.TrustProfile
	found   bool
	err     error
}

func (s stubTrustInspector) Profile(context.Context, string) (trust.TrustProfile, bool, error) {
	return s.profile, s.found, s.err
}

type stubDiscoveryQuery struct {
	result   discovery.FindResult
	err      error
	lastFind discovery.FindOptions
}

func (s *stubDiscoveryQuery) Find(_ context.Context, opts discovery.FindOptions) (discovery.FindResult, error) {
	s.lastFind = opts
	return s.result, s.err
}

func (s *stubDiscoveryQuery) Show(context.Context, discovery.ShowOptions) (discovery.ShowResult, error) {
	return discovery.ShowResult{}, nil
}

func (s *stubDiscoveryQuery) Refresh(context.Context, discovery.RefreshOptions) (discovery.RefreshResult, error) {
	return discovery.RefreshResult{}, nil
}

func TestServiceSendPrefersMatchingTransport(t *testing.T) {
	planner := &stubPlanner{
		sendRoutes: []transport.RouteCandidate{
			{Type: transport.RouteTypeDirect, Label: "peer-direct", Priority: 10},
		},
	}
	service := NewService(
		planner,
		stubDiscovery{view: discovery.PeerPresenceView{CanonicalID: "did:key:test", ResolvedAt: time.Now()}},
		&stubTransport{name: "libp2p_direct", routeType: transport.RouteTypeDirect},
	)

	result, err := service.Send(context.Background(), routing.ContactRuntimeView{
		CanonicalID: "did:key:test",
	}, SendRequest{
		SenderID:    "self",
		RecipientID: "peer",
		Plaintext:   "hello",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if result.Transport != "libp2p_direct" {
		t.Fatalf("Send() transport = %q, want %q", result.Transport, "libp2p_direct")
	}
	if result.Status != "delivered" {
		t.Fatalf("Send() status = %q, want delivered", result.Status)
	}
	if len(planner.outcomes) != 1 || !planner.outcomes[0].Success {
		t.Fatalf("Send() planner outcomes = %#v, want one successful outcome", planner.outcomes)
	}
}

func TestServiceSendEmitsDeliveryHook(t *testing.T) {
	planner := &stubPlanner{
		sendRoutes: []transport.RouteCandidate{{Type: transport.RouteTypeDirect, Label: "peer-direct", Priority: 10}},
	}
	hooks := &stubHooks{}
	service := NewService(
		planner,
		stubDiscovery{view: discovery.PeerPresenceView{CanonicalID: "did:key:test", ResolvedAt: time.Now()}},
		&stubTransport{name: "libp2p_direct", routeType: transport.RouteTypeDirect},
	)
	service.Hooks = hooks

	if _, err := service.Send(context.Background(), routing.ContactRuntimeView{CanonicalID: "did:key:test"}, SendRequest{
		SenderID: "self", RecipientID: "peer", Plaintext: "hello",
	}); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if len(hooks.deliveries) != 1 || !hooks.deliveries[0].Delivered {
		t.Fatalf("delivery hooks = %#v, want one delivered event", hooks.deliveries)
	}
}

func TestServiceSendFallsBackToStoreForwardAndRecordsQueuedOutcome(t *testing.T) {
	planner := &stubPlanner{
		sendRoutes: []transport.RouteCandidate{
			{Type: transport.RouteTypeDirect, Label: "peer-direct", Priority: 100},
			{Type: transport.RouteTypeStoreForward, Label: "sf-relay", Priority: 1},
		},
	}
	direct := &stubTransport{
		name:      "libp2p_direct",
		routeType: transport.RouteTypeDirect,
		sendErr:   context.DeadlineExceeded,
	}
	storeForward := &stubTransport{
		name:      "store_forward",
		routeType: transport.RouteTypeStoreForward,
		sendResult: &transport.SendResult{
			Delivered: false,
			Retryable: true,
		},
	}
	service := NewService(
		planner,
		stubDiscovery{view: discovery.PeerPresenceView{CanonicalID: "did:key:test", ResolvedAt: time.Now()}},
		direct,
		storeForward,
	)

	result, err := service.Send(context.Background(), routing.ContactRuntimeView{
		CanonicalID: "did:key:test",
	}, SendRequest{
		SenderID:    "self",
		RecipientID: "peer",
		Plaintext:   "hello",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if got, want := result.Transport, "store_forward"; got != want {
		t.Fatalf("Send() transport = %q, want %q", got, want)
	}
	if got, want := result.Status, "queued"; got != want {
		t.Fatalf("Send() status = %q, want %q", got, want)
	}
	if direct.sendCalls != 1 || storeForward.sendCalls != 1 {
		t.Fatalf("send calls direct=%d store_forward=%d, want 1/1", direct.sendCalls, storeForward.sendCalls)
	}
	if got, want := len(planner.outcomes), 2; got != want {
		t.Fatalf("planner outcomes len = %d, want %d", got, want)
	}
	if got, want := planner.outcomes[0].Outcome, routing.RouteOutcomeFailed; got != want {
		t.Fatalf("planner.outcomes[0].Outcome = %q, want %q", got, want)
	}
	if got, want := planner.outcomes[1].Outcome, routing.RouteOutcomeQueued; got != want {
		t.Fatalf("planner.outcomes[1].Outcome = %q, want %q", got, want)
	}
}

func TestServiceSyncAggregatesTransportRecovery(t *testing.T) {
	planner := &stubPlanner{
		recoverRoutes: []transport.RouteCandidate{
			{Type: transport.RouteTypeStoreForward, Label: "sf-1", Priority: 1},
			{Type: transport.RouteTypeRecovery, Label: "recovery-1", Priority: 2},
		},
	}
	storeForward := &stubTransport{name: "store_forward", routeType: transport.RouteTypeStoreForward, syncCount: 2}
	recovery := &stubTransport{name: "recovery", routeType: transport.RouteTypeRecovery, syncCount: 1}
	service := NewService(
		planner,
		stubDiscovery{view: discovery.PeerPresenceView{CanonicalID: "did:key:test", ResolvedAt: time.Now()}},
		storeForward,
		recovery,
	)

	result, err := service.Sync(context.Background(), routing.ContactRuntimeView{
		CanonicalID: "did:key:test",
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Synced != 3 {
		t.Fatalf("Sync() recovered = %d, want 3", result.Synced)
	}
	if len(result.RoutesUsed) != 2 {
		t.Fatalf("Sync() routes = %#v, want 2 routes", result.RoutesUsed)
	}
}

func TestServiceSyncAcknowledgesAdvancedCursor(t *testing.T) {
	planner := &stubPlanner{
		recoverRoutes: []transport.RouteCandidate{
			{Type: transport.RouteTypeRecovery, Label: "recovery-1", Priority: 2},
		},
	}
	recovery := &stubTransport{name: "recovery", routeType: transport.RouteTypeRecovery, syncCount: 1}
	service := NewService(
		planner,
		stubDiscovery{view: discovery.PeerPresenceView{CanonicalID: "did:key:test", ResolvedAt: time.Now()}},
		recovery,
	)
	recovery.syncCursor = "cursor-1"
	result, err := service.Sync(context.Background(), routing.ContactRuntimeView{
		CanonicalID: "did:key:test",
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Synced != 1 {
		t.Fatalf("Sync() recovered = %d, want 1", result.Synced)
	}
	if len(recovery.ackCalls) != 1 || recovery.ackCalls[0] != "cursor-1" {
		t.Fatalf("ack calls = %#v, want [cursor-1]", recovery.ackCalls)
	}
}

func TestServiceSyncRecordsRecoveryAndAckOutcomes(t *testing.T) {
	planner := &stubPlanner{
		recoverRoutes: []transport.RouteCandidate{
			{Type: transport.RouteTypeRecovery, Label: "recovery-1", Priority: 2},
		},
	}
	recovery := &stubTransport{name: "recovery", routeType: transport.RouteTypeRecovery, syncCount: 1, syncCursor: "cursor-1"}
	service := NewService(
		planner,
		stubDiscovery{view: discovery.PeerPresenceView{CanonicalID: "did:key:test", ResolvedAt: time.Now()}},
		recovery,
	)

	if _, err := service.Sync(context.Background(), routing.ContactRuntimeView{CanonicalID: "did:key:test"}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if got, want := len(planner.outcomes), 2; got != want {
		t.Fatalf("planner outcomes len = %d, want %d", got, want)
	}
	if got, want := planner.outcomes[0].Outcome, routing.RouteOutcomeRecovered; got != want {
		t.Fatalf("planner.outcomes[0].Outcome = %q, want %q", got, want)
	}
	if got, want := planner.outcomes[0].Cursor, "cursor-1"; got != want {
		t.Fatalf("planner.outcomes[0].Cursor = %q, want %q", got, want)
	}
	if got, want := planner.outcomes[1].Outcome, routing.RouteOutcomeAcked; got != want {
		t.Fatalf("planner.outcomes[1].Outcome = %q, want %q", got, want)
	}
	if got, want := planner.outcomes[1].Cursor, "cursor-1"; got != want {
		t.Fatalf("planner.outcomes[1].Cursor = %q, want %q", got, want)
	}
}

func TestServiceSyncRecordsAckFailureOutcome(t *testing.T) {
	planner := &stubPlanner{
		recoverRoutes: []transport.RouteCandidate{
			{Type: transport.RouteTypeRecovery, Label: "recovery-1", Priority: 2},
		},
	}
	recovery := &stubTransport{
		name:       "recovery",
		routeType:  transport.RouteTypeRecovery,
		syncCount:  1,
		syncCursor: "cursor-1",
		ackErr:     context.DeadlineExceeded,
	}
	service := NewService(
		planner,
		stubDiscovery{view: discovery.PeerPresenceView{CanonicalID: "did:key:test", ResolvedAt: time.Now()}},
		recovery,
	)

	if _, err := service.Sync(context.Background(), routing.ContactRuntimeView{CanonicalID: "did:key:test"}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if got, want := len(planner.outcomes), 2; got != want {
		t.Fatalf("planner outcomes len = %d, want %d", got, want)
	}
	if got, want := planner.outcomes[1].Outcome, routing.RouteOutcomeAckFailed; got != want {
		t.Fatalf("planner.outcomes[1].Outcome = %q, want %q", got, want)
	}
	if got, want := planner.outcomes[1].Cursor, "cursor-1"; got != want {
		t.Fatalf("planner.outcomes[1].Cursor = %q, want %q", got, want)
	}
	if planner.outcomes[1].Error == "" {
		t.Fatalf("planner.outcomes[1].Error = empty, want ack failure error")
	}
}

func TestServiceSyncEmitsRecoveryHook(t *testing.T) {
	planner := &stubPlanner{
		recoverRoutes: []transport.RouteCandidate{{Type: transport.RouteTypeRecovery, Label: "recovery-1", Priority: 2}},
	}
	hooks := &stubHooks{}
	recovery := &stubTransport{name: "recovery", routeType: transport.RouteTypeRecovery, syncCount: 2}
	service := NewService(
		planner,
		stubDiscovery{view: discovery.PeerPresenceView{CanonicalID: "did:key:test", ResolvedAt: time.Now()}},
		recovery,
	)
	service.Hooks = hooks

	if _, err := service.Sync(context.Background(), routing.ContactRuntimeView{CanonicalID: "did:key:test"}); err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if len(hooks.recoveries) != 1 || hooks.recoveries[0].Recovered != 2 {
		t.Fatalf("recovery hooks = %#v, want one recovery event with count 2", hooks.recoveries)
	}
}

func TestServiceStatusReportsRuntimeMode(t *testing.T) {
	t.Setenv(EnvExperimentalBackgroundRuntime, "1")
	service := NewService(
		&stubPlanner{},
		stubDiscovery{view: discovery.PeerPresenceView{CanonicalID: "did:key:test", ResolvedAt: time.Now()}},
	)
	status, err := service.Status(context.Background())
	if err != nil {
		t.Fatalf("Status() error = %v", err)
	}
	if status.RuntimeMode != "background-experimental" || !status.BackgroundRuntime {
		t.Fatalf("status = %#v, want background-experimental enabled", status)
	}
}

func TestServiceSendIgnoresNonP0Routes(t *testing.T) {
	planner := &stubPlanner{
		sendRoutes: []transport.RouteCandidate{
			{Type: transport.RouteTypeNostr, Label: "nostr-relay", Priority: 10, Target: "wss://relay.example"},
		},
	}
	nostr := &stubTransport{name: "nostr", routeType: transport.RouteTypeNostr}
	service := NewService(
		planner,
		stubDiscovery{view: discovery.PeerPresenceView{CanonicalID: "did:key:test", ResolvedAt: time.Now()}},
		nostr,
	)

	_, err := service.Send(context.Background(), routing.ContactRuntimeView{
		CanonicalID: "did:key:test",
	}, SendRequest{
		SenderID:    "self",
		RecipientID: "peer",
		Plaintext:   "hello",
	})
	if err == nil {
		t.Fatal("Send() error = nil, want no usable transport route error")
	}
	if nostr.sendCalls != 0 {
		t.Fatalf("nostr send calls = %d, want 0 (non-P0 routes should be ignored)", nostr.sendCalls)
	}
}

func TestServiceSyncIgnoresNonP0Routes(t *testing.T) {
	planner := &stubPlanner{
		recoverRoutes: []transport.RouteCandidate{
			{Type: transport.RouteTypeNostr, Label: "nostr-relay", Priority: 5, Target: "wss://relay.example"},
		},
	}
	nostr := &stubTransport{name: "nostr", routeType: transport.RouteTypeNostr, syncCount: 3}
	service := NewService(
		planner,
		stubDiscovery{view: discovery.PeerPresenceView{CanonicalID: "did:key:test", ResolvedAt: time.Now()}},
		nostr,
	)

	result, err := service.Sync(context.Background(), routing.ContactRuntimeView{
		CanonicalID: "did:key:test",
	})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if result.Synced != 0 {
		t.Fatalf("Sync() recovered = %d, want 0", result.Synced)
	}
	if nostr.syncCalls != 0 {
		t.Fatalf("nostr sync calls = %d, want 0 (non-P0 routes should be ignored)", nostr.syncCalls)
	}
}

func TestServiceInspectTrustReturnsProfile(t *testing.T) {
	service := NewService(&stubPlanner{}, stubDiscovery{})
	service.Trust = stubTrustInspector{
		found: true,
		profile: trust.TrustProfile{
			CanonicalID: "did:key:test",
			TrustLevel:  "trusted",
			Summary: trust.TrustSummary{
				CanonicalID: "did:key:test",
				TrustLevel:  "trusted",
			},
		},
	}

	result, err := service.InspectTrust(context.Background(), InspectTrustRequest{CanonicalID: "did:key:test"})
	if err != nil {
		t.Fatalf("InspectTrust() error = %v", err)
	}
	if !result.Found {
		t.Fatalf("InspectTrust() Found = %t, want true", result.Found)
	}
	if got, want := result.Summary.TrustLevel, "trusted"; got != want {
		t.Fatalf("InspectTrust() summary trust level = %q, want %q", got, want)
	}
	if result.InspectedAt == "" {
		t.Fatal("InspectTrust() inspected_at = empty")
	}
}

func TestServiceListDiscoveryDelegatesToQueryService(t *testing.T) {
	query := &stubDiscoveryQuery{
		result: discovery.FindResult{
			Query: discovery.FindQuery{
				Capabilities: []string{"direct"},
				Source:       "import",
				FreshOnly:    true,
				Limit:        3,
			},
			Records: []discovery.DiscoveryEntry{{CanonicalID: "did:key:test"}},
			FoundAt: "2026-03-23T09:00:00Z",
		},
	}
	service := NewService(&stubPlanner{}, stubDiscovery{})
	service.DiscoveryQuery = query

	result, err := service.ListDiscovery(context.Background(), ListDiscoveryRequest{
		Capability: "direct",
		Source:     "import",
		FreshOnly:  true,
		Limit:      3,
	})
	if err != nil {
		t.Fatalf("ListDiscovery() error = %v", err)
	}
	if got, want := len(result.Records), 1; got != want {
		t.Fatalf("ListDiscovery() records len = %d, want %d", got, want)
	}
	if got, want := query.lastFind.Capability, "direct"; got != want {
		t.Fatalf("delegated capability = %q, want %q", got, want)
	}
	if got, want := query.lastFind.Source, "import"; got != want {
		t.Fatalf("delegated source = %q, want %q", got, want)
	}
}

func TestServiceConnectPeerRefreshUsesRefreshedPresenceForRoutePlanning(t *testing.T) {
	planner := &presenceDrivenPlanner{}
	discoverySvc := &refreshAwareDiscovery{
		resolveView: discovery.PeerPresenceView{
			CanonicalID: "did:key:test",
			ResolvedAt:  time.Now().Add(-2 * time.Hour),
			FreshUntil:  time.Now().Add(-time.Hour),
			Source:      "stale-cache",
		},
		refreshView: discovery.PeerPresenceView{
			CanonicalID: "did:key:test",
			ResolvedAt:  time.Now(),
			FreshUntil:  time.Now().Add(5 * time.Minute),
			Source:      "refresh",
			RouteCandidates: []transport.RouteCandidate{
				{Type: transport.RouteTypeStoreForward, Label: "relay", Priority: 1, Target: "https://relay.example"},
			},
		},
	}
	service := NewService(
		planner,
		discoverySvc,
		&stubTransport{name: "store_forward_ready", routeType: transport.RouteTypeStoreForward},
	)
	service.Trust = stubTrustInspector{
		found: true,
		profile: trust.TrustProfile{
			CanonicalID: "did:key:test",
			TrustLevel:  "seen",
			Summary: trust.TrustSummary{
				CanonicalID: "did:key:test",
				TrustLevel:  "seen",
			},
		},
	}

	staleResult, err := service.ConnectPeer(context.Background(), ConnectPeerRequest{
		Peer: routing.ContactRuntimeView{
			CanonicalID: "did:key:test",
		},
	})
	if err != nil {
		t.Fatalf("ConnectPeer() stale error = %v", err)
	}
	if staleResult.Connected {
		t.Fatalf("ConnectPeer() stale connected = true, want false")
	}
	if got, want := discoverySvc.resolveCalls, 1; got != want {
		t.Fatalf("ResolvePeer() calls = %d, want %d", got, want)
	}
	if got, want := discoverySvc.refreshCalls, 0; got != want {
		t.Fatalf("RefreshPeer() calls = %d, want %d", got, want)
	}
	if planner.lastPresence.Source != "stale-cache" {
		t.Fatalf("planner stale presence source = %q, want stale-cache", planner.lastPresence.Source)
	}

	freshResult, err := service.ConnectPeer(context.Background(), ConnectPeerRequest{
		Peer: routing.ContactRuntimeView{
			CanonicalID: "did:key:test",
		},
		Refresh: true,
	})
	if err != nil {
		t.Fatalf("ConnectPeer() refresh error = %v", err)
	}
	if !freshResult.Connected {
		t.Fatalf("ConnectPeer() refresh connected = false, want true; result=%#v", freshResult)
	}
	if got, want := freshResult.Transport, "store_forward_ready"; got != want {
		t.Fatalf("ConnectPeer() refresh transport = %q, want %q", got, want)
	}
	if got, want := discoverySvc.refreshCalls, 1; got != want {
		t.Fatalf("RefreshPeer() calls = %d, want %d", got, want)
	}
	if planner.lastPresence.Source != "refresh" {
		t.Fatalf("planner refresh presence source = %q, want refresh", planner.lastPresence.Source)
	}
	if got := len(planner.lastPresence.RouteCandidates); got != 1 {
		t.Fatalf("planner refresh routes = %d, want 1", got)
	}
}

func TestServiceConnectPeerUsesTrustAndDiscovery(t *testing.T) {
	planner := &stubPlanner{
		sendRoutes: []transport.RouteCandidate{
			{Type: transport.RouteTypeDirect, Label: "peer-direct", Priority: 100, Target: "libp2p://peer-direct"},
		},
	}
	service := NewService(
		planner,
		stubDiscovery{view: discovery.PeerPresenceView{
			CanonicalID: "did:key:test",
			PeerID:      "peer-direct",
			Reachable:   true,
			ResolvedAt:  time.Now(),
		}},
		&stubTransport{name: "libp2p_direct", routeType: transport.RouteTypeDirect},
	)
	service.Trust = stubTrustInspector{
		found: true,
		profile: trust.TrustProfile{
			CanonicalID: "did:key:test",
			TrustLevel:  "verified",
			Summary: trust.TrustSummary{
				CanonicalID: "did:key:test",
				TrustLevel:  "verified",
			},
		},
	}

	result, err := service.ConnectPeer(context.Background(), ConnectPeerRequest{
		Peer: routing.ContactRuntimeView{
			CanonicalID: "did:key:test",
		},
	})
	if err != nil {
		t.Fatalf("ConnectPeer() error = %v", err)
	}
	if !result.Connected {
		t.Fatalf("ConnectPeer() Connected = %t, want true; result=%#v", result.Connected, result)
	}
	if got, want := result.Transport, "libp2p_direct"; got != want {
		t.Fatalf("ConnectPeer() transport = %q, want %q", got, want)
	}
	if got, want := result.Trust.TrustLevel, "verified"; got != want {
		t.Fatalf("ConnectPeer() trust level = %q, want %q", got, want)
	}
}

func TestServiceConnectPeerReturnsUnconnectedWhenNoUsableRoute(t *testing.T) {
	planner := &stubPlanner{
		sendRoutes: []transport.RouteCandidate{
			{Type: transport.RouteTypeNostr, Label: "nostr-relay", Priority: 10, Target: "wss://relay.example"},
		},
	}
	service := NewService(
		planner,
		stubDiscovery{view: discovery.PeerPresenceView{
			CanonicalID: "did:key:test",
			ResolvedAt:  time.Now(),
		}},
		&stubTransport{name: "nostr", routeType: transport.RouteTypeNostr},
	)
	service.Trust = stubTrustInspector{
		found: true,
		profile: trust.TrustProfile{
			CanonicalID: "did:key:test",
			TrustLevel:  "unknown",
			Summary: trust.TrustSummary{
				CanonicalID: "did:key:test",
				TrustLevel:  "unknown",
			},
		},
	}

	result, err := service.ConnectPeer(context.Background(), ConnectPeerRequest{
		Peer: routing.ContactRuntimeView{
			CanonicalID: "did:key:test",
		},
	})
	if err != nil {
		t.Fatalf("ConnectPeer() error = %v", err)
	}
	if result.Connected {
		t.Fatalf("ConnectPeer() Connected = true, want false")
	}
	if !strings.Contains(result.Reason, "no usable transport route") {
		t.Fatalf("ConnectPeer() reason = %q, want no usable transport route", result.Reason)
	}
}
