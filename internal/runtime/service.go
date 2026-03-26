package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/discovery"
	"github.com/xiewanpeng/claw-identity/internal/routing"
	"github.com/xiewanpeng/claw-identity/internal/transport"
	"github.com/xiewanpeng/claw-identity/internal/trust"
)

type MessagingRuntime interface {
	Send(ctx context.Context, contact routing.ContactRuntimeView, req SendRequest) (SendResult, error)
	Sync(ctx context.Context, contact routing.ContactRuntimeView) (SyncResult, error)
	Recover(ctx context.Context, contact routing.ContactRuntimeView) (RecoverResult, error)
	Acknowledge(ctx context.Context, req AckRequest) error
	Status(ctx context.Context) (Status, error)
	InspectTrust(ctx context.Context, req InspectTrustRequest) (InspectTrustResult, error)
	ListDiscovery(ctx context.Context, req ListDiscoveryRequest) (ListDiscoveryResult, error)
	ConnectPeer(ctx context.Context, req ConnectPeerRequest) (ConnectPeerResult, error)
}

type TrustInspector interface {
	Profile(ctx context.Context, canonicalID string) (trust.TrustProfile, bool, error)
}

type DiscoveryQuery interface {
	Find(ctx context.Context, opts discovery.FindOptions) (discovery.FindResult, error)
	Show(ctx context.Context, opts discovery.ShowOptions) (discovery.ShowResult, error)
	Refresh(ctx context.Context, opts discovery.RefreshOptions) (discovery.RefreshResult, error)
}

type Service struct {
	Planner        routing.Planner
	Discovery      discovery.Service
	DiscoveryQuery DiscoveryQuery
	Trust          TrustInspector
	Transports     []transport.Transport
	Hooks          Hooks
	Now            func() time.Time
}

func NewService(planner routing.Planner, discoverySvc discovery.Service, transports ...transport.Transport) *Service {
	return &Service{
		Planner:    planner,
		Discovery:  discoverySvc,
		Transports: transports,
		Hooks:      NoopHooks{},
		Now:        time.Now,
	}
}

func (s *Service) Send(ctx context.Context, contact routing.ContactRuntimeView, req SendRequest) (SendResult, error) {
	presence, routes, err := s.resolveSendRoutes(ctx, contact)
	if err != nil {
		return SendResult{}, err
	}
	_ = presence
	messageID := req.MessageID
	if messageID == "" {
		messageID = req.SenderID + "->" + req.RecipientID
	}
	envelope := transport.Envelope{
		MessageID:   messageID,
		SenderID:    req.SenderID,
		RecipientID: req.RecipientID,
		Plaintext:   req.Plaintext,
	}

	for idx := 0; idx < len(routes); idx++ {
		route := routes[idx]
		if route.Type == transport.RouteTypeNostr {
			groupEnd := idx
			nostrRoutes := make([]transport.RouteCandidate, 0, 1)
			for groupEnd < len(routes) && routes[groupEnd].Type == transport.RouteTypeNostr {
				nostrRoutes = append(nostrRoutes, routes[groupEnd])
				groupEnd++
			}
			result, ok := s.sendNostrFanout(ctx, envelope, nostrRoutes)
			if ok {
				return result, nil
			}
			idx = groupEnd - 1
			continue
		}
		adapter := s.findTransport(route)
		if adapter == nil {
			continue
		}
		sendResult, sendErr := adapter.Send(ctx, envelope, route)
		if sendErr != nil {
			s.recordOutcome(ctx, route, envelope.MessageID, routing.RouteOutcomeFailed, false, true, sendErr.Error(), "")
			_ = s.hooks().OnDeliveryOutcome(ctx, DeliveryOutcomeEvent{
				MessageID: envelope.MessageID,
				Route:     route,
				Transport: adapter.Name(),
				Delivered: false,
				Retryable: true,
				Error:     sendErr.Error(),
			})
			continue
		}
		deliveryOutcome := routing.RouteOutcomeQueued
		if sendResult.Delivered {
			deliveryOutcome = routing.RouteOutcomeDelivered
		}
		s.recordOutcome(ctx, route, envelope.MessageID, deliveryOutcome, sendResult.Delivered, sendResult.Retryable, "", "")
		_ = s.hooks().OnDeliveryOutcome(ctx, DeliveryOutcomeEvent{
			MessageID: envelope.MessageID,
			Route:     route,
			Transport: adapter.Name(),
			Delivered: sendResult.Delivered,
			Retryable: sendResult.Retryable,
		})
		status := MessageStatusQueued
		if sendResult.Delivered {
			status = MessageStatusDelivered
		}
		return SendResult{
			MessageID:     envelope.MessageID,
			Status:        status,
			SelectedRoute: route,
			Transport:     adapter.Name(),
		}, nil
	}
	return SendResult{}, fmt.Errorf("no usable transport route for contact %q", contact.CanonicalID)
}

type nostrSendAttempt struct {
	route   transport.RouteCandidate
	adapter transport.Transport
	result  transport.SendResult
	err     error
}

func (s *Service) sendNostrFanout(
	ctx context.Context,
	envelope transport.Envelope,
	routes []transport.RouteCandidate,
) (SendResult, bool) {
	attempts := make([]nostrSendAttempt, len(routes))
	var wg sync.WaitGroup
	for idx, route := range routes {
		adapter := s.findTransport(route)
		attempts[idx] = nostrSendAttempt{
			route:   route,
			adapter: adapter,
		}
		if adapter == nil {
			continue
		}
		wg.Add(1)
		go func(i int, transportAdapter transport.Transport, candidate transport.RouteCandidate) {
			defer wg.Done()
			sendResult, sendErr := transportAdapter.Send(ctx, envelope, candidate)
			attempts[i].result = sendResult
			attempts[i].err = sendErr
		}(idx, adapter, route)
	}
	wg.Wait()

	var firstSuccess *SendResult
	for _, attempt := range attempts {
		if attempt.adapter == nil {
			continue
		}
		if attempt.err != nil {
			s.recordOutcome(ctx, attempt.route, envelope.MessageID, routing.RouteOutcomeFailed, false, true, attempt.err.Error(), "")
			_ = s.hooks().OnDeliveryOutcome(ctx, DeliveryOutcomeEvent{
				MessageID: envelope.MessageID,
				Route:     attempt.route,
				Transport: attempt.adapter.Name(),
				Delivered: false,
				Retryable: true,
				Error:     attempt.err.Error(),
			})
			continue
		}

		// Nostr fallback is recoverable async delivery: accepted publish means queued/deferred.
		delivery := attempt.result
		delivery.Delivered = false
		if !delivery.Retryable {
			delivery.Retryable = true
		}
		s.recordOutcome(ctx, attempt.route, envelope.MessageID, routing.RouteOutcomeQueued, false, delivery.Retryable, "", "")
		_ = s.hooks().OnDeliveryOutcome(ctx, DeliveryOutcomeEvent{
			MessageID: envelope.MessageID,
			Route:     attempt.route,
			Transport: attempt.adapter.Name(),
			Delivered: false,
			Retryable: delivery.Retryable,
		})

		if firstSuccess == nil {
			result := SendResult{
				MessageID:     envelope.MessageID,
				Status:        MessageStatusQueued,
				SelectedRoute: attempt.route,
				Transport:     attempt.adapter.Name(),
			}
			firstSuccess = &result
		}
	}
	if firstSuccess == nil {
		return SendResult{}, false
	}
	return *firstSuccess, true
}

func (s *Service) Sync(ctx context.Context, contact routing.ContactRuntimeView) (SyncResult, error) {
	presence, err := s.resolvePresence(ctx, contact)
	if err != nil {
		return SyncResult{}, err
	}
	routes, err := s.Planner.PlanRecover(ctx, contact, presence)
	if err != nil {
		return SyncResult{}, err
	}
	result := SyncResult{}
	for _, route := range routes {
		adapter := s.findTransport(route)
		if adapter == nil {
			continue
		}
		syncResult, syncErr := adapter.Sync(ctx, route)
		if syncErr != nil {
			s.recordOutcome(ctx, route, "", routing.RouteOutcomeFailed, false, true, syncErr.Error(), "")
			continue
		}
		result.Synced += syncResult.Recovered
		result.RoutesUsed = append(result.RoutesUsed, adapter.Name())
		s.recordOutcome(ctx, route, "", routing.RouteOutcomeRecovered, true, true, "", syncResult.AdvancedCursor)
		_ = s.hooks().OnRecovery(ctx, RecoveryEvent{
			Route:          route,
			Transport:      adapter.Name(),
			Recovered:      syncResult.Recovered,
			AdvancedCursor: syncResult.AdvancedCursor,
		})
		if syncResult.AdvancedCursor != "" {
			if ackErr := adapter.Ack(ctx, route, syncResult.AdvancedCursor); ackErr != nil {
				s.recordOutcome(ctx, route, "", routing.RouteOutcomeAckFailed, false, true, ackErr.Error(), syncResult.AdvancedCursor)
			} else {
				s.recordOutcome(ctx, route, "", routing.RouteOutcomeAcked, true, false, "", syncResult.AdvancedCursor)
			}
		}
	}
	return result, nil
}

func (s *Service) Recover(ctx context.Context, contact routing.ContactRuntimeView) (RecoverResult, error) {
	syncResult, err := s.Sync(ctx, contact)
	if err != nil {
		return RecoverResult{}, err
	}
	return RecoverResult{
		Recovered:  syncResult.Synced,
		RoutesUsed: syncResult.RoutesUsed,
	}, nil
}

func (s *Service) Acknowledge(ctx context.Context, req AckRequest) error {
	for _, adapter := range s.Transports {
		if adapter.Name() != req.RouteName {
			continue
		}
		return adapter.Ack(ctx, transport.RouteCandidate{Label: req.RouteName}, req.Cursor)
	}
	return fmt.Errorf("transport %q not found", req.RouteName)
}

func (s *Service) Status(_ context.Context) (Status, error) {
	return Status{
		IdentityReady:     true,
		TransportReady:    len(s.Transports) > 0,
		DiscoveryReady:    s.Discovery != nil,
		RuntimeMode:       RuntimeMode(),
		BackgroundRuntime: BackgroundRuntimeEnabled(),
	}, nil
}

func (s *Service) InspectTrust(ctx context.Context, req InspectTrustRequest) (InspectTrustResult, error) {
	canonicalID := strings.TrimSpace(req.CanonicalID)
	if canonicalID == "" {
		return InspectTrustResult{}, fmt.Errorf("canonical_id is required")
	}
	if s.Trust == nil {
		return InspectTrustResult{}, fmt.Errorf("trust service is not configured")
	}

	profile, found, err := s.Trust.Profile(ctx, canonicalID)
	if err != nil {
		return InspectTrustResult{}, err
	}
	if !found {
		profile = trust.TrustProfile{
			CanonicalID: canonicalID,
			TrustLevel:  "unknown",
		}
	}
	if strings.TrimSpace(profile.CanonicalID) == "" {
		profile.CanonicalID = canonicalID
	}
	if strings.TrimSpace(profile.Summary.CanonicalID) == "" {
		profile.Summary = trust.BuildTrustSummary(profile)
	}

	return InspectTrustResult{
		CanonicalID: canonicalID,
		Found:       found,
		Profile:     profile,
		Summary:     profile.Summary,
		InspectedAt: s.nowUTC().Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) ListDiscovery(ctx context.Context, req ListDiscoveryRequest) (ListDiscoveryResult, error) {
	if s.DiscoveryQuery == nil {
		return ListDiscoveryResult{}, fmt.Errorf("discovery query service is not configured")
	}

	result, err := s.DiscoveryQuery.Find(ctx, discovery.FindOptions{
		Capability:   req.Capability,
		Capabilities: req.Capabilities,
		Source:       req.Source,
		FreshOnly:    req.FreshOnly,
		Limit:        req.Limit,
	})
	if err != nil {
		return ListDiscoveryResult{}, err
	}
	return ListDiscoveryResult{
		Query:   result.Query,
		Records: result.Records,
		FoundAt: result.FoundAt,
	}, nil
}

func (s *Service) ConnectPeer(ctx context.Context, req ConnectPeerRequest) (ConnectPeerResult, error) {
	peer := req.Peer
	canonicalID := strings.TrimSpace(peer.CanonicalID)
	if canonicalID == "" {
		return ConnectPeerResult{}, fmt.Errorf("canonical_id is required")
	}
	if s.Planner == nil {
		return ConnectPeerResult{}, fmt.Errorf("route planner is not configured")
	}
	if s.Discovery == nil {
		return ConnectPeerResult{}, fmt.Errorf("discovery service is not configured")
	}

	trustResult, err := s.InspectTrust(ctx, InspectTrustRequest{CanonicalID: canonicalID})
	if err != nil {
		return ConnectPeerResult{}, err
	}

	var presence discovery.PeerPresenceView
	if req.Refresh {
		presence, err = s.Discovery.RefreshPeer(ctx, canonicalID)
	} else {
		presence, err = s.Discovery.ResolvePeer(ctx, canonicalID)
	}
	if err != nil {
		return ConnectPeerResult{}, err
	}
	presence.Source = discovery.NormalizeSource(presence.Source)

	routes, err := s.Planner.PlanSend(ctx, peer, presence)
	if err != nil {
		return ConnectPeerResult{}, err
	}

	result := ConnectPeerResult{
		CanonicalID: canonicalID,
		Trust:       trustResult.Summary,
		Presence:    presence,
		Routes:      routes,
		ConnectedAt: s.nowUTC().Format(time.RFC3339Nano),
	}
	for _, route := range routes {
		adapter := s.findTransport(route)
		if adapter == nil {
			continue
		}
		result.SelectedRoute = route
		result.Transport = adapter.Name()
		result.Connected = true
		return result, nil
	}

	result.Reason = fmt.Sprintf("no usable transport route for peer %q", canonicalID)
	return result, nil
}

func (s *Service) resolveSendRoutes(ctx context.Context, contact routing.ContactRuntimeView) (discovery.PeerPresenceView, []transport.RouteCandidate, error) {
	presence, err := s.resolvePresence(ctx, contact)
	if err != nil {
		return discovery.PeerPresenceView{}, nil, err
	}
	routes, err := s.Planner.PlanSend(ctx, contact, presence)
	if err != nil {
		return discovery.PeerPresenceView{}, nil, err
	}
	return presence, routes, nil
}

func (s *Service) resolvePresence(ctx context.Context, contact routing.ContactRuntimeView) (discovery.PeerPresenceView, error) {
	if s.Discovery == nil {
		return discovery.PeerPresenceView{}, fmt.Errorf("discovery service is not configured")
	}
	return s.Discovery.ResolvePeer(ctx, contact.CanonicalID)
}

func (s *Service) findTransport(route transport.RouteCandidate) transport.Transport {
	if !transport.IsKnownRouteType(route.Type) {
		return nil
	}
	for _, adapter := range s.Transports {
		if adapter.Supports(route) {
			return adapter
		}
	}
	return nil
}

func (s *Service) recordOutcome(
	ctx context.Context,
	route transport.RouteCandidate,
	messageID string,
	outcome string,
	success bool,
	retryable bool,
	errMsg string,
	cursor string,
) {
	if s.Planner == nil {
		return
	}
	_ = s.Planner.RecordOutcome(ctx, routing.RouteOutcome{
		MessageID:  messageID,
		Route:      route,
		Outcome:    outcome,
		Success:    success,
		Retryable:  retryable,
		Error:      errMsg,
		Cursor:     cursor,
		OccurredAt: s.nowUTC(),
	})
}

func (s *Service) hooks() Hooks {
	if s == nil || s.Hooks == nil {
		return NoopHooks{}
	}
	return s.Hooks
}

func (s *Service) nowUTC() time.Time {
	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	return nowFn().UTC()
}
