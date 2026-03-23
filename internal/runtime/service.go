package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/discovery"
	"github.com/xiewanpeng/claw-identity/internal/routing"
	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type MessagingRuntime interface {
	Send(ctx context.Context, contact routing.ContactRuntimeView, req SendRequest) (SendResult, error)
	Sync(ctx context.Context, contact routing.ContactRuntimeView) (SyncResult, error)
	Recover(ctx context.Context, contact routing.ContactRuntimeView) (RecoverResult, error)
	Acknowledge(ctx context.Context, req AckRequest) error
	Status(ctx context.Context) (Status, error)
}

type Service struct {
	Planner    routing.Planner
	Discovery  discovery.Service
	Transports []transport.Transport
	Hooks      Hooks
	Now        func() time.Time
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

	for _, route := range routes {
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
		status := "queued"
		if sendResult.Delivered {
			status = "delivered"
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
	// P0 runtime only dispatches first-batch route types.
	if !route.IsP0() {
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
	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	_ = s.Planner.RecordOutcome(ctx, routing.RouteOutcome{
		MessageID:  messageID,
		Route:      route,
		Outcome:    outcome,
		Success:    success,
		Retryable:  retryable,
		Error:      errMsg,
		Cursor:     cursor,
		OccurredAt: nowFn().UTC(),
	})
}

func (s *Service) hooks() Hooks {
	if s == nil || s.Hooks == nil {
		return NoopHooks{}
	}
	return s.Hooks
}
