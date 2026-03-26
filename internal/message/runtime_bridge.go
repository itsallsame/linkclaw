package message

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	agentdiscovery "github.com/xiewanpeng/claw-identity/internal/discovery"
	discoverylibp2p "github.com/xiewanpeng/claw-identity/internal/discovery/libp2p"
	"github.com/xiewanpeng/claw-identity/internal/routing"
	agentruntime "github.com/xiewanpeng/claw-identity/internal/runtime"
	"github.com/xiewanpeng/claw-identity/internal/transport"
	transportlibp2p "github.com/xiewanpeng/claw-identity/internal/transport/libp2p"
	transportnostr "github.com/xiewanpeng/claw-identity/internal/transport/nostr"
	transportstoreforward "github.com/xiewanpeng/claw-identity/internal/transport/storeforward"
)

type staticDiscoveryService struct {
	view agentdiscovery.PeerPresenceView
}

func (s staticDiscoveryService) ResolvePeer(context.Context, string) (agentdiscovery.PeerPresenceView, error) {
	return s.view, nil
}

func (s staticDiscoveryService) RefreshPeer(context.Context, string) (agentdiscovery.PeerPresenceView, error) {
	return s.view, nil
}

func (s staticDiscoveryService) PublishSelf(context.Context) error { return nil }

type staticPlanner struct {
	sendRoutes    []transport.RouteCandidate
	recoverRoutes []transport.RouteCandidate
	record        func(context.Context, routing.RouteOutcome) error
}

func (s staticPlanner) PlanSend(context.Context, routing.ContactRuntimeView, agentdiscovery.PeerPresenceView) ([]transport.RouteCandidate, error) {
	return s.sendRoutes, nil
}

func (s staticPlanner) PlanRecover(context.Context, routing.ContactRuntimeView, agentdiscovery.PeerPresenceView) ([]transport.RouteCandidate, error) {
	return s.recoverRoutes, nil
}

func (s staticPlanner) RecordOutcome(ctx context.Context, outcome routing.RouteOutcome) error {
	if s.record == nil {
		return nil
	}
	return s.record(ctx, outcome)
}

type connectPresenceProvider struct {
	resolve func(context.Context, string) (agentdiscovery.PeerPresenceView, error)
	refresh func(context.Context, string) (agentdiscovery.PeerPresenceView, error)
}

func (s connectPresenceProvider) ResolvePeer(ctx context.Context, canonicalID string) (agentdiscovery.PeerPresenceView, error) {
	if s.resolve != nil {
		return s.resolve(ctx, canonicalID)
	}
	if s.refresh != nil {
		return s.refresh(ctx, canonicalID)
	}
	return agentdiscovery.PeerPresenceView{}, fmt.Errorf("connect presence resolver is not configured")
}

func (s connectPresenceProvider) RefreshPeer(ctx context.Context, canonicalID string) (agentdiscovery.PeerPresenceView, error) {
	if s.refresh != nil {
		return s.refresh(ctx, canonicalID)
	}
	return s.ResolvePeer(ctx, canonicalID)
}

func (s connectPresenceProvider) PublishSelf(context.Context) error { return nil }

type queryBackedDiscoveryService struct {
	query    *agentdiscovery.QueryService
	fallback agentdiscovery.Service
	now      func() time.Time
}

func (s queryBackedDiscoveryService) ResolvePeer(ctx context.Context, canonicalID string) (agentdiscovery.PeerPresenceView, error) {
	if s.query != nil {
		result, err := s.query.Show(ctx, agentdiscovery.ShowOptions{CanonicalID: canonicalID})
		if err == nil {
			return normalizePresenceSource(presenceViewFromDiscoveryEntry(result.Record, s.nowUTC())), nil
		}
		if !isDiscoveryRecordNotFound(err) {
			return agentdiscovery.PeerPresenceView{}, err
		}
	}
	if s.fallback != nil {
		view, err := s.fallback.ResolvePeer(ctx, canonicalID)
		if err != nil {
			return agentdiscovery.PeerPresenceView{}, err
		}
		return normalizePresenceSource(view), nil
	}
	return agentdiscovery.PeerPresenceView{}, fmt.Errorf("discovery record %q not found", strings.TrimSpace(canonicalID))
}

func (s queryBackedDiscoveryService) RefreshPeer(ctx context.Context, canonicalID string) (agentdiscovery.PeerPresenceView, error) {
	if s.query != nil {
		result, err := s.query.Refresh(ctx, agentdiscovery.RefreshOptions{CanonicalID: canonicalID})
		if err == nil {
			return normalizePresenceSource(presenceViewFromDiscoveryEntry(result.Record, s.nowUTC())), nil
		}
		if !isDiscoveryRecordNotFound(err) || s.fallback == nil {
			return agentdiscovery.PeerPresenceView{}, err
		}
	}
	if s.fallback != nil {
		view, err := s.fallback.RefreshPeer(ctx, canonicalID)
		if err != nil {
			return agentdiscovery.PeerPresenceView{}, err
		}
		return normalizePresenceSource(view), nil
	}
	return agentdiscovery.PeerPresenceView{}, fmt.Errorf("discovery record %q not found", strings.TrimSpace(canonicalID))
}

func (s queryBackedDiscoveryService) PublishSelf(ctx context.Context) error {
	if s.fallback == nil {
		return nil
	}
	return s.fallback.PublishSelf(ctx)
}

func (s queryBackedDiscoveryService) nowUTC() time.Time {
	if s.now == nil {
		return time.Now().UTC()
	}
	return s.now().UTC()
}

func normalizePresenceSource(view agentdiscovery.PeerPresenceView) agentdiscovery.PeerPresenceView {
	view.Source = agentdiscovery.NormalizeSource(view.Source)
	return view
}

type presenceRoutesPlanner struct{}

func (s presenceRoutesPlanner) PlanSend(_ context.Context, _ routing.ContactRuntimeView, presence agentdiscovery.PeerPresenceView) ([]transport.RouteCandidate, error) {
	return routesFromPresenceView(presence), nil
}

func (s presenceRoutesPlanner) PlanRecover(context.Context, routing.ContactRuntimeView, agentdiscovery.PeerPresenceView) ([]transport.RouteCandidate, error) {
	return nil, nil
}

func (s presenceRoutesPlanner) RecordOutcome(context.Context, routing.RouteOutcome) error {
	return nil
}

func presenceViewFromDiscoveryEntry(entry agentdiscovery.DiscoveryEntry, fallbackNow time.Time) agentdiscovery.PeerPresenceView {
	resolvedAt := parseTimestamp(entry.ResolvedAt)
	if resolvedAt.IsZero() {
		resolvedAt = fallbackNow.UTC()
	}
	freshUntil := parseTimestampOrFallback(entry.FreshUntil, resolvedAt.Add(5*time.Minute))
	return agentdiscovery.PeerPresenceView{
		CanonicalID:           strings.TrimSpace(entry.CanonicalID),
		PeerID:                strings.TrimSpace(entry.PeerID),
		Reachable:             entry.Reachable,
		RouteCandidates:       append([]transport.RouteCandidate(nil), entry.RouteCandidates...),
		TransportCapabilities: append([]string(nil), entry.TransportCapabilities...),
		DirectHints:           append([]string(nil), entry.DirectHints...),
		StoreForwardHints:     append([]string(nil), entry.StoreForwardHints...),
		SignedPeerRecord:      strings.TrimSpace(entry.SignedPeerRecord),
		Source:                strings.TrimSpace(entry.Source),
		ResolvedAt:            resolvedAt,
		FreshUntil:            freshUntil,
		AnnouncedAt:           parseTimestamp(entry.AnnouncedAt),
	}
}

func routesFromPresenceView(view agentdiscovery.PeerPresenceView) []transport.RouteCandidate {
	return appendHintsToRoutes(view.RouteCandidates, view.DirectHints, view.StoreForwardHints)
}

func isDiscoveryRecordNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "discovery record") && strings.Contains(msg, "not found")
}

type legacyStoreForwardBackend struct {
	service     *Service
	home        string
	now         time.Time
	record      MessageRecord
	contact     contactRecord
	selfProfile selfMessagingProfile
}

func (b legacyStoreForwardBackend) Send(ctx context.Context, _ transport.Envelope, route transport.RouteCandidate) (transport.SendResult, error) {
	updated, err := b.service.deliverOutgoing(ctx, b.home, b.record, b.contact, b.now)
	if err != nil {
		return transport.SendResult{}, err
	}
	return transport.SendResult{
		Route:       route,
		Delivered:   false,
		Retryable:   true,
		Description: updated.Status,
	}, nil
}

func (b legacyStoreForwardBackend) Recover(ctx context.Context, route transport.RouteCandidate) (transport.SyncResult, error) {
	count, nextCursor, err := b.service.syncStoreForward(ctx, b.home, b.selfProfile, route.Target, b.now)
	if err != nil {
		return transport.SyncResult{}, err
	}
	return transport.SyncResult{
		Route:          route,
		Recovered:      count,
		AdvancedCursor: nextCursor,
	}, nil
}

func (b legacyStoreForwardBackend) Acknowledge(ctx context.Context, route transport.RouteCandidate, cursor string) error {
	if cursor == "" {
		return nil
	}
	return b.service.storeForwardBackend().Ack(ctx, route.Target, transportstoreforward.MailboxAckRequest{
		RecipientID: b.selfProfile.RecipientID,
		Cursor:      cursor,
	})
}

type directInboxReceiver struct {
	service     *Service
	home        string
	selfProfile selfMessagingProfile
	now         time.Time
}

func (r directInboxReceiver) ReceiveDirect(ctx context.Context, env transport.Envelope) error {
	return r.service.receiveDirectEnvelope(ctx, r.home, r.selfProfile, env, r.now)
}

func runtimeContactView(contact contactRecord) routing.ContactRuntimeView {
	caps := []string{}
	directHints := []string{}
	storeForwardHints := []string{}
	relayURL := strings.TrimSpace(contact.RelayURL)
	peerID := strings.TrimSpace(contact.RecipientID)
	if directTarget := buildDirectRouteTarget(contact.DirectURL, contact.DirectToken); directTarget != "" {
		caps = appendIfMissing(caps, string(transport.RouteTypeDirect))
		directHints = appendIfMissing(directHints, directTarget)
	}
	if peerIdentity, err := derivePeerIdentity(contact.CanonicalID, contact.SigningPublicKey, contact.EncryptionPublicKey); err == nil {
		peerID = peerIdentity.PeerID
		caps = appendIfMissing(caps, string(transport.RouteTypeDirect))
		directHints = appendIfMissing(directHints, "libp2p://"+peerIdentity.PeerID)
	}
	storeForwardTargets := storeForwardTargetsFromContact(contact)
	if len(storeForwardTargets) > 0 {
		relayURL = storeForwardTargets[0]
		caps = appendIfMissing(caps, string(transport.RouteTypeStoreForward))
		for _, target := range storeForwardTargets {
			storeForwardHints = appendIfMissing(storeForwardHints, target)
		}
	}
	if len(nostrTargetsFromContact(contact)) > 0 {
		caps = appendIfMissing(caps, string(transport.RouteTypeNostr))
	}
	return routing.ContactRuntimeView{
		ContactID:             contact.ContactID,
		CanonicalID:           contact.CanonicalID,
		DisplayName:           contact.DisplayName,
		PeerID:                peerID,
		RecipientID:           contact.RecipientID,
		DirectURL:             strings.TrimSpace(contact.DirectURL),
		DirectToken:           strings.TrimSpace(contact.DirectToken),
		RelayURL:              relayURL,
		DirectHints:           directHints,
		StoreForwardHints:     storeForwardHints,
		TransportCapabilities: caps,
	}
}

func derivePeerIdentity(canonicalID string, signingPublicKey string, encryptionPublicKey string) (discoverylibp2p.PeerIdentity, error) {
	return discoverylibp2p.DerivePeerIdentity(discoverylibp2p.IdentityInput{
		CanonicalID:         canonicalID,
		SigningPublicKey:    signingPublicKey,
		EncryptionPublicKey: encryptionPublicKey,
	})
}

func directTransportEnabled() bool {
	return discoverylibp2p.DirectEnabledFromEnv()
}

func buildSendRuntimeBoundary(selfProfile selfMessagingProfile, contact contactRecord, now time.Time) (agentdiscovery.PeerPresenceView, []transport.Transport, []transport.RouteCandidate) {
	storeForwardTargets := storeForwardTargetsFromContact(contact)
	nostrTargets := nostrTargetsFromContact(contact)
	routes := make([]transport.RouteCandidate, 0, len(storeForwardTargets)+len(nostrTargets)+2)
	transports := make([]transport.Transport, 0, 3)
	view := agentdiscovery.PeerPresenceView{
		CanonicalID: contact.CanonicalID,
		ResolvedAt:  now.UTC(),
		FreshUntil:  now.UTC().Add(5 * time.Minute),
		Source:      "runtime-send",
	}
	directTarget := buildDirectRouteTarget(contact.DirectURL, contact.DirectToken)
	directEnabled := directTransportEnabled() || directTarget != ""
	var directSession *discoverylibp2p.Session

	if directEnabled {
		session, err := discoverylibp2p.BootSession(discoverylibp2p.SessionConfig{
			Enabled:             true,
			CanonicalID:         selfProfile.CanonicalID,
			SigningPublicKey:    selfProfile.SigningPublicKey,
			EncryptionPublicKey: "",
			Now:                 now,
		})
		if err == nil && session != nil && session.Enabled {
			directSession = session
			transports = append(transports, transportlibp2p.New(session))
		}
	}

	if directTarget != "" {
		route := transport.RouteCandidate{
			Type:     transport.RouteTypeDirect,
			Label:    contact.CanonicalID,
			Priority: 100,
			Target:   directTarget,
		}
		routes = append(routes, route)
		view.RouteCandidates = append(view.RouteCandidates, route)
		view.TransportCapabilities = appendIfMissing(view.TransportCapabilities, string(transport.RouteTypeDirect))
		view.DirectHints = appendIfMissing(view.DirectHints, directTarget)
		view.Reachable = true
	}

	if contactPeer, contactErr := derivePeerIdentity(contact.CanonicalID, contact.SigningPublicKey, contact.EncryptionPublicKey); contactErr == nil {
		view.PeerID = contactPeer.PeerID
		view.SignedPeerRecord = contactPeer.SignedPeerRecord
		view.TransportCapabilities = appendIfMissing(view.TransportCapabilities, string(transport.RouteTypeDirect))
		if directTarget == "" {
			libp2pTarget := "libp2p://" + contactPeer.PeerID
			view.DirectHints = appendIfMissing(view.DirectHints, libp2pTarget)
			route := transport.RouteCandidate{
				Type:     transport.RouteTypeDirect,
				Label:    contactPeer.PeerID,
				Priority: 100,
				Target:   libp2pTarget,
			}
			routes = append(routes, route)
			view.RouteCandidates = append(view.RouteCandidates, route)
		}
		if directSession != nil {
			view.Reachable = true
		}
	}

	if len(nostrTargets) > 0 {
		transports = append(transports, transportnostr.New(transportnostr.NewBackend(nil)))
		route := transport.RouteCandidate{
			Type:     transport.RouteTypeNostr,
			Label:    nostrTargets[0],
			Priority: 30,
			Target:   nostrTargets[0],
		}
		for _, target := range nostrTargets {
			route.Label = target
			route.Target = target
			routes = append(routes, route)
			view.RouteCandidates = append(view.RouteCandidates, route)
		}
		view.TransportCapabilities = appendIfMissing(view.TransportCapabilities, string(transport.RouteTypeNostr))
		if view.PeerID == "" {
			view.PeerID = contact.RecipientID
		}
		view.Reachable = true
	}

	if len(storeForwardTargets) > 0 {
		route := transport.RouteCandidate{
			Type:     transport.RouteTypeStoreForward,
			Label:    storeForwardTargets[0],
			Priority: 1,
			Target:   storeForwardTargets[0],
		}
		for _, target := range storeForwardTargets {
			route.Label = target
			route.Target = target
			routes = append(routes, route)
			view.RouteCandidates = append(view.RouteCandidates, route)
			view.StoreForwardHints = appendIfMissing(view.StoreForwardHints, target)
		}
		view.TransportCapabilities = appendIfMissing(view.TransportCapabilities, string(transport.RouteTypeStoreForward))
		if view.PeerID == "" {
			view.PeerID = contact.RecipientID
		}
		view.Reachable = true
	}

	return view, transports, routes
}

func runtimePeerViewFromDiscovery(record agentdiscovery.Record) routing.ContactRuntimeView {
	return routing.ContactRuntimeView{
		CanonicalID:           strings.TrimSpace(record.CanonicalID),
		PeerID:                strings.TrimSpace(record.PeerID),
		DirectHints:           append([]string(nil), record.DirectHints...),
		StoreForwardHints:     append([]string(nil), record.StoreForwardHints...),
		TransportCapabilities: append([]string(nil), record.TransportCapabilities...),
	}
}

func buildDiscoveryConnectRuntimeBoundary(selfProfile selfMessagingProfile, record agentdiscovery.Record, now time.Time) (agentdiscovery.PeerPresenceView, []transport.Transport, []transport.RouteCandidate) {
	routes := discoveryRoutes(record)
	view := agentdiscovery.PeerPresenceView{
		CanonicalID:           strings.TrimSpace(record.CanonicalID),
		PeerID:                strings.TrimSpace(record.PeerID),
		Reachable:             record.Reachable,
		RouteCandidates:       append([]transport.RouteCandidate(nil), routes...),
		TransportCapabilities: append([]string(nil), record.TransportCapabilities...),
		DirectHints:           append([]string(nil), record.DirectHints...),
		StoreForwardHints:     append([]string(nil), record.StoreForwardHints...),
		SignedPeerRecord:      strings.TrimSpace(record.SignedPeerRecord),
		Source:                strings.TrimSpace(record.Source),
		ResolvedAt:            parseTimestampOrFallback(record.ResolvedAt, now.UTC()),
		FreshUntil:            parseTimestampOrFallback(record.FreshUntil, now.UTC().Add(5*time.Minute)),
		AnnouncedAt:           parseTimestamp(record.AnnouncedAt),
	}

	transports := make([]transport.Transport, 0, 1)
	if directTransportEnabled() && hasRouteType(routes, transport.RouteTypeDirect) {
		session, err := discoverylibp2p.BootSession(discoverylibp2p.SessionConfig{
			Enabled:             true,
			CanonicalID:         selfProfile.CanonicalID,
			SigningPublicKey:    selfProfile.SigningPublicKey,
			EncryptionPublicKey: "",
			Now:                 now,
		})
		if err == nil && session != nil && session.Enabled {
			transports = append(transports, transportlibp2p.New(session))
			view.Reachable = true
		}
	}
	if hasRouteType(routes, transport.RouteTypeStoreForward) {
		view.Reachable = true
	}

	return view, transports, routes
}

func discoveryRoutes(record agentdiscovery.Record) []transport.RouteCandidate {
	return appendHintsToRoutes(record.RouteCandidates, record.DirectHints, record.StoreForwardHints)
}

func appendHintsToRoutes(base []transport.RouteCandidate, directHints []string, storeForwardHints []string) []transport.RouteCandidate {
	routes := append([]transport.RouteCandidate(nil), base...)
	for _, hint := range directHints {
		target := strings.TrimSpace(hint)
		if target == "" || hasRoute(routes, transport.RouteTypeDirect, target) {
			continue
		}
		routes = append(routes, transport.RouteCandidate{
			Type:     transport.RouteTypeDirect,
			Label:    target,
			Priority: 100,
			Target:   target,
		})
	}
	for _, hint := range storeForwardHints {
		target := strings.TrimSpace(hint)
		if target == "" || hasRoute(routes, transport.RouteTypeStoreForward, target) {
			continue
		}
		routes = append(routes, transport.RouteCandidate{
			Type:     transport.RouteTypeStoreForward,
			Label:    target,
			Priority: 1,
			Target:   target,
		})
	}
	return routes
}

func hasRoute(routes []transport.RouteCandidate, routeType transport.RouteType, target string) bool {
	target = strings.TrimSpace(target)
	for _, route := range routes {
		if route.Type != routeType {
			continue
		}
		if strings.TrimSpace(route.Target) == target {
			return true
		}
	}
	return false
}

func hasRouteType(routes []transport.RouteCandidate, routeType transport.RouteType) bool {
	for _, route := range routes {
		if route.Type == routeType {
			return true
		}
	}
	return false
}

func parseTimestamp(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return parsed.UTC()
}

func parseTimestampOrFallback(raw string, fallback time.Time) time.Time {
	parsed := parseTimestamp(raw)
	if parsed.IsZero() {
		return fallback
	}
	return parsed
}

func buildDirectRouteTarget(rawURL string, token string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	if strings.TrimSpace(query.Get("token")) == "" {
		query.Set("token", token)
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func appendIfMissing(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.TrimSpace(existing) == value {
			return values
		}
	}
	return append(values, value)
}

func (s *Service) ensureDirectRuntimeRegistration(ctx context.Context, home string, selfProfile selfMessagingProfile, now time.Time) error {
	if !directTransportEnabled() {
		return nil
	}
	session, err := discoverylibp2p.BootSession(discoverylibp2p.SessionConfig{
		Enabled:             true,
		CanonicalID:         selfProfile.CanonicalID,
		SigningPublicKey:    selfProfile.SigningPublicKey,
		EncryptionPublicKey: "",
		ListenAddress:       buildDirectRouteTarget(selfProfile.DirectURL, selfProfile.DirectToken),
		Now:                 now,
		Receiver: directInboxReceiver{
			service:     s,
			home:        home,
			selfProfile: selfProfile,
			now:         now,
		},
	})
	if err != nil || session == nil || !session.Enabled {
		return err
	}
	discoverylibp2p.RegisterSession(session)
	store, _, err := agentruntime.OpenStore(ctx, home, now)
	if err != nil {
		return err
	}
	defer store.Close()
	selfPresence := discoverylibp2p.NewService(discoverylibp2p.PresenceConfig{
		Peer:          session.Peer,
		DirectAddress: session.ListenAddress,
		Reachable:     true,
		ResolvedAt:    now.UTC(),
	})
	if err := selfPresence.PublishSelf(ctx); err != nil {
		return err
	}
	view, err := selfPresence.ResolvePeer(ctx, selfProfile.CanonicalID)
	if err != nil {
		return err
	}
	if err := store.UpsertPresence(ctx, presenceRecordFromView(view)); err != nil {
		return err
	}
	return nil
}

func (s *Service) sendThroughRuntime(ctx context.Context, home string, selfProfile selfMessagingProfile, contact contactRecord, record MessageRecord, now time.Time) (agentruntime.SendResult, error) {
	store, _, err := agentruntime.OpenStore(ctx, home, now)
	if err != nil {
		return agentruntime.SendResult{}, err
	}
	defer store.Close()

	view, extraTransports, routes := buildSendRuntimeBoundary(selfProfile, contact, now)
	if view.CanonicalID != "" {
		if err := store.UpsertPresence(ctx, presenceRecordFromView(view)); err != nil {
			return agentruntime.SendResult{}, err
		}
	}
	runtimeSvc := agentruntime.NewService(
		staticPlanner{
			sendRoutes: routes,
			record: func(ctx context.Context, outcome routing.RouteOutcome) error {
				return store.RecordRouteAttempt(ctx, outcome, record.ConversationID, "")
			},
		},
		staticDiscoveryService{view: view},
		transportstoreforward.New(legacyStoreForwardBackend{
			service: s,
			home:    home,
			now:     now,
			record:  record,
			contact: contact,
		}),
	)
	runtimeSvc.Transports = append(extraTransports, runtimeSvc.Transports...)
	return runtimeSvc.Send(ctx, runtimeContactView(contact), agentruntime.SendRequest{
		MessageID:   record.MessageID,
		ContactRef:  contact.ContactID,
		SenderID:    selfProfile.CanonicalID,
		RecipientID: contact.RecipientID,
		Plaintext:   record.Body,
	})
}

func presenceRecordFromView(view agentdiscovery.PeerPresenceView) agentruntime.PresenceRecord {
	return agentruntime.PresenceRecord{
		CanonicalID:           view.CanonicalID,
		PeerID:                view.PeerID,
		TransportCapabilities: append([]string(nil), view.TransportCapabilities...),
		DirectHints:           append([]string(nil), view.DirectHints...),
		StoreForwardHints:     append([]string(nil), view.StoreForwardHints...),
		SignedPeerRecord:      view.SignedPeerRecord,
		Source:                view.Source,
		Reachable:             view.Reachable,
		FreshUntil:            formatOptionalTime(view.FreshUntil),
		ResolvedAt:            formatOptionalTime(view.ResolvedAt),
		AnnouncedAt:           formatOptionalTime(view.AnnouncedAt),
	}
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func syncRuntimeSendState(ctx context.Context, home string, contact contactRecord, conversation Conversation, record MessageRecord, now time.Time) error {
	db, _, err := openStateDB(ctx, home, now)
	if err != nil {
		return err
	}
	defer db.Close()

	store := agentruntime.NewStoreWithDB(db, now)
	if err := syncRuntimeSelfIdentity(ctx, db, store); err != nil {
		return err
	}

	caps := []string{}
	directHints := []string{}
	storeForwardHints := []string{}
	peerID := contact.RecipientID
	signedPeerRecord := ""
	if directTarget := buildDirectRouteTarget(contact.DirectURL, contact.DirectToken); directTarget != "" {
		caps = appendIfMissing(caps, string(transport.RouteTypeDirect))
		directHints = appendIfMissing(directHints, directTarget)
	}
	if peerIdentity, err := derivePeerIdentity(contact.CanonicalID, contact.SigningPublicKey, contact.EncryptionPublicKey); err == nil {
		peerID = peerIdentity.PeerID
		if signedPeerRecord == "" {
			signedPeerRecord = peerIdentity.SignedPeerRecord
		}
		caps = appendIfMissing(caps, string(transport.RouteTypeDirect))
		directHints = appendIfMissing(directHints, "libp2p://"+peerIdentity.PeerID)
	}
	for _, target := range storeForwardTargetsFromContact(contact) {
		caps = appendIfMissing(caps, string(transport.RouteTypeStoreForward))
		storeForwardHints = appendIfMissing(storeForwardHints, target)
	}
	if len(nostrTargetsFromContact(contact)) > 0 {
		caps = appendIfMissing(caps, string(transport.RouteTypeNostr))
	}
	if err := store.UpsertContact(ctx, agentruntime.ContactRecord{
		ContactID:             contact.ContactID,
		CanonicalID:           contact.CanonicalID,
		DisplayName:           contact.DisplayName,
		PeerID:                peerID,
		SigningPublicKey:      contact.SigningPublicKey,
		EncryptionPublicKey:   contact.EncryptionPublicKey,
		TrustState:            contact.Status,
		TransportCapabilities: caps,
		DirectHints:           directHints,
		StoreForwardHints:     storeForwardHints,
		SignedPeerRecord:      signedPeerRecord,
	}); err != nil {
		return err
	}
	if err := store.UpsertConversation(ctx, agentruntime.ConversationRecord{
		ConversationID:     conversation.ConversationID,
		ContactID:          conversation.ContactID,
		LastMessageID:      record.MessageID,
		LastMessagePreview: conversation.LastMessagePreview,
		LastMessageAt:      conversation.LastMessageAt,
		UnreadCount:        conversation.UnreadCount,
	}); err != nil {
		return err
	}
	return store.UpsertMessage(ctx, agentruntime.MessageRecord{
		MessageID:         record.MessageID,
		ConversationID:    record.ConversationID,
		SenderID:          record.SenderCanonicalID,
		RecipientID:       record.RecipientContactID,
		Direction:         record.Direction,
		PlaintextBody:     record.Body,
		PlaintextPreview:  record.Preview,
		Status:            record.Status,
		SelectedRoute:     record.SelectedRoute,
		CiphertextVersion: "v0",
		CreatedAt:         record.CreatedAt,
		DeliveredAt:       record.DeliveredAt,
	})
}

func (s *Service) syncThroughRuntime(ctx context.Context, home string, selfProfile selfMessagingProfile, relayURL string, now time.Time) (agentruntime.SyncResult, error) {
	store, _, err := agentruntime.OpenStore(ctx, home, now)
	if err != nil {
		return agentruntime.SyncResult{}, err
	}
	defer store.Close()

	route := transport.RouteCandidate{
		Type:     transport.RouteTypeRecovery,
		Label:    relayURL,
		Priority: 1,
		Target:   relayURL,
	}
	runtimeSvc := agentruntime.NewService(
		staticPlanner{
			recoverRoutes: []transport.RouteCandidate{route},
			record: func(ctx context.Context, outcome routing.RouteOutcome) error {
				return store.RecordRouteAttempt(ctx, outcome, "", "")
			},
		},
		staticDiscoveryService{
			view: agentdiscovery.PeerPresenceView{
				CanonicalID:     selfProfile.CanonicalID,
				PeerID:          selfProfile.RecipientID,
				Reachable:       selfProfile.RelayURL != "",
				RouteCandidates: []transport.RouteCandidate{route},
				ResolvedAt:      now.UTC(),
				FreshUntil:      now.UTC().Add(5 * time.Minute),
			},
		},
		transportstoreforward.New(legacyStoreForwardBackend{
			service:     s,
			home:        home,
			now:         now,
			selfProfile: selfProfile,
		}),
	)
	return runtimeSvc.Sync(ctx, routing.ContactRuntimeView{
		CanonicalID:           selfProfile.CanonicalID,
		PeerID:                selfProfile.RecipientID,
		TransportCapabilities: []string{string(transport.RouteTypeRecovery)},
	})
}

func (s *Service) receiveDirectEnvelope(ctx context.Context, home string, selfProfile selfMessagingProfile, env transport.Envelope, now time.Time) error {
	db, _, err := openStateDB(ctx, home, now)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin direct message receive transaction: %w", err)
	}
	defer tx.Rollback()

	contact, err := ensureDirectIncomingContact(ctx, tx, env, now)
	if err != nil {
		return err
	}
	conversation, err := ensureConversation(ctx, tx, contact, now)
	if err != nil {
		return err
	}
	inserted, err := insertDirectIncomingMessage(ctx, tx, conversation, contact, selfProfile, env, now)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit direct message receive transaction: %w", err)
	}
	if !inserted {
		return nil
	}
	return syncRuntimeRecoveredState(ctx, db, []contactRecord{contact}, []Conversation{{
		ConversationID:     conversation.ConversationID,
		ContactID:          conversation.ContactID,
		ContactDisplayName: contact.DisplayName,
		ContactCanonicalID: contact.CanonicalID,
		ContactStatus:      contact.Status,
		LastMessageAt:      now.Format(time.RFC3339Nano),
		LastMessagePreview: makePreview(env.Plaintext),
		UnreadCount:        1,
	}}, []MessageRecord{{
		MessageID:         env.MessageID,
		ConversationID:    conversation.ConversationID,
		Direction:         DirectionIncoming,
		SenderContactID:   contact.ContactID,
		SenderCanonicalID: env.SenderID,
		RecipientRouteID:  selfProfile.RecipientID,
		Body:              env.Plaintext,
		Preview:           makePreview(env.Plaintext),
		Status:            StatusDelivered,
		TransportStatus:   TransportStatusDirect,
		SelectedRoute: transport.RouteCandidate{
			Type:     transport.RouteTypeDirect,
			Label:    "direct",
			Priority: 100,
			Target:   env.SenderID,
		},
		CreatedAt:   now.Format(time.RFC3339Nano),
		DeliveredAt: now.Format(time.RFC3339Nano),
	}}, now)
}

func (s *Service) syncStoreForward(ctx context.Context, home string, selfProfile selfMessagingProfile, relayURL string, now time.Time) (int, string, error) {
	db, _, err := openStateDB(ctx, home, now)
	if err != nil {
		return 0, "", err
	}
	defer db.Close()
	store := agentruntime.NewStoreWithDB(db, now)

	cursor, err := store.LoadStoreForwardCursor(ctx, selfProfile.SelfID, relayURL)
	if err != nil {
		return 0, "", err
	}
	pulled, err := s.storeForwardBackend().Pull(ctx, relayURL, selfProfile.RecipientID, cursor)
	if err != nil {
		return 0, "", err
	}
	if len(pulled.Messages) == 0 {
		if err := store.SaveStoreForwardState(ctx, agentruntime.StoreForwardStateRecord{
			SelfID:             selfProfile.SelfID,
			RouteLabel:         relayURL,
			CursorValue:        pulled.NextCursor,
			LastResult:         "success",
			LastRecoveredCount: 0,
			UpdatedAt:          now.Format(time.RFC3339Nano),
		}); err != nil {
			return 0, "", err
		}
		return 0, pulled.NextCursor, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", fmt.Errorf("begin message sync transaction: %w", err)
	}
	defer tx.Rollback()

	synced := 0
	contacts := make([]contactRecord, 0, len(pulled.Messages))
	conversations := make([]Conversation, 0, len(pulled.Messages))
	messages := make([]MessageRecord, 0, len(pulled.Messages))
	observations := make([]agentruntime.RecoveredEventObservationRecord, 0, len(pulled.Messages))
	seenObservationKeys := make(map[string]struct{}, len(pulled.Messages))
	recoveryRoute := transport.RouteCandidate{
		Type:     transport.RouteTypeRecovery,
		Label:    relayURL,
		Priority: 1,
		Target:   relayURL,
	}
	for _, pulledMessage := range pulled.Messages {
		observation, hasObservation, err := buildRecoveredObservationRecord(selfProfile.SelfID, relayURL, pulledMessage, now)
		if err != nil {
			return 0, "", err
		}
		if hasObservation {
			observationKey := observation.EventID + "|" + observation.RelayURL
			if _, seenInBatch := seenObservationKeys[observationKey]; seenInBatch {
				continue
			}
			seenInStore, err := store.HasRecoveredEventObservation(ctx, selfProfile.SelfID, observation.EventID)
			if err != nil {
				return 0, "", err
			}
			if seenInStore {
				seenObservationKeys[observationKey] = struct{}{}
				continue
			}
		}
		plaintext, preview, err := decryptIncomingMessage(selfProfile, pulledMessage)
		if err != nil {
			return 0, "", err
		}
		contact, err := ensureIncomingContact(ctx, tx, pulledMessage, now)
		if err != nil {
			return 0, "", err
		}
		conversation, err := ensureConversation(ctx, tx, contact, now)
		if err != nil {
			return 0, "", err
		}
		inserted, err := insertIncomingMessage(ctx, tx, conversation, contact, pulledMessage, plaintext, preview, now)
		if err != nil {
			return 0, "", err
		}
		if hasObservation {
			observation.CanonicalID = firstNonEmptyString(strings.TrimSpace(observation.CanonicalID), contact.CanonicalID, pulledMessage.SenderID)
			observation.MessageID = firstNonEmptyString(strings.TrimSpace(observation.MessageID), pulledMessage.MessageID)
			observations = append(observations, observation)
			seenObservationKeys[observation.EventID+"|"+observation.RelayURL] = struct{}{}
		}
		if !inserted {
			continue
		}
		contacts = append(contacts, contact)
		conversation.LastMessageAt = strings.TrimSpace(pulledMessage.SentAt)
		if conversation.LastMessageAt == "" {
			conversation.LastMessageAt = now.Format(time.RFC3339Nano)
		}
		conversation.LastMessagePreview = preview
		conversation.UnreadCount++
		conversations = append(conversations, conversation)
		messages = append(messages, MessageRecord{
			MessageID:         pulledMessage.MessageID,
			ConversationID:    conversation.ConversationID,
			Direction:         DirectionIncoming,
			SenderContactID:   contact.ContactID,
			SenderCanonicalID: pulledMessage.SenderID,
			RecipientRouteID:  pulledMessage.RecipientID,
			Body:              plaintext,
			Preview:           preview,
			Status:            StatusRecovered,
			TransportStatus:   TransportStatusRecovered,
			SelectedRoute:     recoveryRoute,
			CreatedAt:         conversation.LastMessageAt,
		})
		synced++
	}
	if err := saveSyncCursor(ctx, tx, selfProfile.SelfID, relayURL, pulled.NextCursor, now); err != nil {
		return 0, "", err
	}
	if err := tx.Commit(); err != nil {
		return 0, "", fmt.Errorf("commit message sync transaction: %w", err)
	}
	if err := store.SaveStoreForwardState(ctx, agentruntime.StoreForwardStateRecord{
		SelfID:             selfProfile.SelfID,
		RouteLabel:         relayURL,
		CursorValue:        pulled.NextCursor,
		LastResult:         "success",
		LastRecoveredCount: synced,
		UpdatedAt:          now.Format(time.RFC3339Nano),
	}); err != nil {
		return 0, "", err
	}
	if err := persistRecoveredObservations(ctx, store, observations); err != nil {
		return 0, "", err
	}
	if synced == 0 {
		return 0, pulled.NextCursor, nil
	}
	if err := syncRuntimeRecoveredState(ctx, db, contacts, conversations, messages, now); err != nil {
		return 0, "", err
	}
	return synced, pulled.NextCursor, nil
}

func buildRecoveredObservationRecord(
	selfID string,
	relayURL string,
	message transportstoreforward.MailboxPullMessage,
	now time.Time,
) (agentruntime.RecoveredEventObservationRecord, bool, error) {
	selfID = strings.TrimSpace(selfID)
	eventID := firstNonEmptyString(strings.TrimSpace(message.RelayMessageID), strings.TrimSpace(message.MessageID))
	if selfID == "" || eventID == "" {
		return agentruntime.RecoveredEventObservationRecord{}, false, nil
	}
	payloadJSON, err := json.Marshal(message)
	if err != nil {
		return agentruntime.RecoveredEventObservationRecord{}, false, fmt.Errorf("marshal recovered message observation payload: %w", err)
	}
	digest := sha256.Sum256(payloadJSON)
	return agentruntime.RecoveredEventObservationRecord{
		SelfID:       selfID,
		EventID:      eventID,
		RelayURL:     strings.TrimSpace(relayURL),
		CanonicalID:  strings.TrimSpace(message.SenderID),
		MessageID:    strings.TrimSpace(message.MessageID),
		ObservedAt:   firstNonEmptyString(strings.TrimSpace(message.SentAt), now.Format(time.RFC3339Nano)),
		PayloadHash:  "sha256:" + hex.EncodeToString(digest[:]),
		PayloadJSON:  string(payloadJSON),
		MetadataJSON: `{"source":"store_forward_sync"}`,
	}, true, nil
}

func persistRecoveredObservations(
	ctx context.Context,
	store *agentruntime.Store,
	observations []agentruntime.RecoveredEventObservationRecord,
) error {
	if len(observations) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(observations))
	for _, observation := range observations {
		if strings.TrimSpace(observation.SelfID) == "" || strings.TrimSpace(observation.EventID) == "" {
			continue
		}
		key := observation.SelfID + "|" + observation.EventID + "|" + observation.RelayURL
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		if err := store.UpsertRecoveredEventObservation(ctx, observation); err != nil {
			return err
		}
	}
	return nil
}

func syncRuntimeRecoveredState(
	ctx context.Context,
	db *sql.DB,
	contacts []contactRecord,
	conversations []Conversation,
	messages []MessageRecord,
	now time.Time,
) error {
	store := agentruntime.NewStoreWithDB(db, now)
	if err := syncRuntimeSelfIdentity(ctx, db, store); err != nil {
		return err
	}
	for _, contact := range contacts {
		caps := []string{}
		directHints := []string{}
		storeForwardHints := []string{}
		if contact.RecipientID != "" {
			directHints = append(directHints, contact.RecipientID)
		}
		for _, target := range storeForwardTargetsFromContact(contact) {
			caps = appendIfMissing(caps, string(transport.RouteTypeStoreForward))
			storeForwardHints = appendIfMissing(storeForwardHints, target)
		}
		if err := store.UpsertContact(ctx, agentruntime.ContactRecord{
			ContactID:             contact.ContactID,
			CanonicalID:           contact.CanonicalID,
			DisplayName:           contact.DisplayName,
			PeerID:                contact.RecipientID,
			SigningPublicKey:      contact.SigningPublicKey,
			EncryptionPublicKey:   contact.EncryptionPublicKey,
			TrustState:            contact.Status,
			TransportCapabilities: caps,
			DirectHints:           directHints,
			StoreForwardHints:     storeForwardHints,
		}); err != nil {
			return err
		}
	}
	for _, conversation := range conversations {
		if err := store.UpsertConversation(ctx, agentruntime.ConversationRecord{
			ConversationID:     conversation.ConversationID,
			ContactID:          conversation.ContactID,
			LastMessagePreview: conversation.LastMessagePreview,
			LastMessageAt:      conversation.LastMessageAt,
			UnreadCount:        conversation.UnreadCount,
		}); err != nil {
			return err
		}
	}
	for _, record := range messages {
		if err := store.UpsertMessage(ctx, agentruntime.MessageRecord{
			MessageID:         record.MessageID,
			ConversationID:    record.ConversationID,
			SenderID:          record.SenderCanonicalID,
			RecipientID:       record.RecipientRouteID,
			Direction:         record.Direction,
			PlaintextBody:     record.Body,
			PlaintextPreview:  record.Preview,
			Status:            record.Status,
			SelectedRoute:     record.SelectedRoute,
			CiphertextVersion: "v0",
			CreatedAt:         record.CreatedAt,
			DeliveredAt:       now.Format(time.RFC3339Nano),
		}); err != nil {
			return err
		}
	}
	return nil
}

func syncRuntimeSelfIdentity(ctx context.Context, db *sql.DB, store *agentruntime.Store) error {
	var selfID, displayName string
	if err := db.QueryRowContext(ctx, `
		SELECT self_id, display_name
		FROM self_identities
		ORDER BY created_at ASC
		LIMIT 1
	`).Scan(&selfID, &displayName); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("query runtime self identity snapshot: %w", err)
	}

	var peerID, signingPublicKey, signingPrivateKeyRef, encryptionPublicKey, encryptionPrivateKeyRef string
	_ = db.QueryRowContext(ctx, `
		SELECT recipient_id, relay_url, encryption_public_key, encryption_private_key_ref
		FROM self_messaging_profiles
		WHERE self_id = ?
		LIMIT 1
	`, selfID).Scan(&peerID, new(string), &encryptionPublicKey, &encryptionPrivateKeyRef)
	_ = db.QueryRowContext(ctx, `
		SELECT public_key, private_key_ref
		FROM keys
		WHERE owner_type='self' AND owner_id = ? AND status='active'
		ORDER BY created_at ASC
		LIMIT 1
	`, selfID).Scan(&signingPublicKey, &signingPrivateKeyRef)

	caps := []string{string(transport.RouteTypeStoreForward), string(transport.RouteTypeRecovery)}
	if peerIdentity, err := derivePeerIdentity(selfID, signingPublicKey, encryptionPublicKey); err == nil {
		peerID = peerIdentity.PeerID
		caps = append(caps, string(transport.RouteTypeDirect))
	}

	return store.UpsertSelfIdentity(ctx, agentruntime.SelfIdentityRecord{
		SelfID:                  selfID,
		DisplayName:             displayName,
		PeerID:                  peerID,
		SigningPublicKey:        signingPublicKey,
		EncryptionPublicKey:     encryptionPublicKey,
		SigningPrivateKeyRef:    signingPrivateKeyRef,
		EncryptionPrivateKeyRef: encryptionPrivateKeyRef,
		TransportCapabilities:   caps,
	})
}

func loadRuntimeInbox(ctx context.Context, home string, now time.Time) ([]Conversation, error) {
	store, _, err := agentruntime.OpenStore(ctx, home, now)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	records, err := store.ListConversations(ctx)
	if err != nil {
		return nil, err
	}
	conversations := make([]Conversation, 0, len(records))
	for _, record := range records {
		conversations = append(conversations, Conversation{
			ConversationID:     record.ConversationID,
			ContactID:          record.ContactID,
			ContactDisplayName: record.ContactDisplayName,
			ContactCanonicalID: record.ContactCanonicalID,
			ContactStatus:      record.ContactTrustState,
			LastMessageAt:      record.LastMessageAt,
			LastMessagePreview: record.LastMessagePreview,
			UnreadCount:        record.UnreadCount,
		})
	}
	return conversations, nil
}

func loadRuntimeOutbox(ctx context.Context, home string, now time.Time) ([]MessageRecord, error) {
	store, _, err := agentruntime.OpenStore(ctx, home, now)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	records, err := store.ListOutgoingMessages(ctx)
	if err != nil {
		return nil, err
	}
	messages := make([]MessageRecord, 0, len(records))
	for _, record := range records {
		transportStatus := deriveTransportStatus(record.Direction, record.Status, record.SelectedRoute)
		messages = append(messages, MessageRecord{
			MessageID:          record.MessageID,
			ConversationID:     record.ConversationID,
			Direction:          record.Direction,
			SenderCanonicalID:  record.SenderID,
			RecipientContactID: record.RecipientID,
			Body:               record.PlaintextBody,
			Preview:            record.PlaintextPreview,
			Status:             record.Status,
			TransportStatus:    transportStatus,
			SelectedRoute:      record.SelectedRoute,
			CreatedAt:          record.CreatedAt,
			DeliveredAt:        record.DeliveredAt,
		})
	}
	return messages, nil
}

func loadRuntimeThread(ctx context.Context, home string, contactRef string, limit int, markRead bool, now time.Time) (Conversation, error) {
	store, _, err := agentruntime.OpenStore(ctx, home, now)
	if err != nil {
		return Conversation{}, err
	}
	defer store.Close()

	record, err := store.LoadConversationByContactRef(ctx, contactRef, limit)
	if err != nil {
		if err == sql.ErrNoRows {
			return Conversation{}, fmt.Errorf("contact %q not found; import an identity card first", contactRef)
		}
		return Conversation{}, err
	}
	if markRead && record.ConversationID != "" {
		if err := store.MarkConversationRead(ctx, record.ConversationID); err != nil {
			return Conversation{}, err
		}
		record.UnreadCount = 0
	}

	conversation := Conversation{
		ConversationID:     record.ConversationID,
		ContactID:          record.ContactID,
		ContactDisplayName: record.ContactDisplayName,
		ContactCanonicalID: record.ContactCanonicalID,
		ContactStatus:      record.ContactTrustState,
		LastMessageAt:      record.LastMessageAt,
		LastMessagePreview: record.LastMessagePreview,
		UnreadCount:        record.UnreadCount,
	}
	for _, msg := range record.Messages {
		transportStatus := deriveTransportStatus(msg.Direction, msg.Status, msg.SelectedRoute)
		conversation.Messages = append(conversation.Messages, MessageRecord{
			MessageID:          msg.MessageID,
			ConversationID:     msg.ConversationID,
			Direction:          msg.Direction,
			SenderCanonicalID:  msg.SenderID,
			RecipientContactID: msg.RecipientID,
			Body:               msg.PlaintextBody,
			Preview:            msg.PlaintextPreview,
			Status:             msg.Status,
			TransportStatus:    transportStatus,
			SelectedRoute:      msg.SelectedRoute,
			CreatedAt:          msg.CreatedAt,
			DeliveredAt:        msg.DeliveredAt,
		})
	}
	return conversation, nil
}
