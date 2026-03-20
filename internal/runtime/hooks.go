package runtime

import (
	"context"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type Hooks interface {
	OnDeliveryOutcome(ctx context.Context, event DeliveryOutcomeEvent) error
	OnRecovery(ctx context.Context, event RecoveryEvent) error
	OnReputationSignal(ctx context.Context, event ReputationSignal) error
	OnPaymentIntent(ctx context.Context, event PaymentIntent) error
	OnPenaltySignal(ctx context.Context, event PenaltySignal) error
}

type DeliveryOutcomeEvent struct {
	MessageID string
	Route     transport.RouteCandidate
	Transport string
	Delivered bool
	Retryable bool
	Error     string
}

type RecoveryEvent struct {
	Route          transport.RouteCandidate
	Transport      string
	Recovered      int
	AdvancedCursor string
}

type ReputationSignal struct {
	CanonicalID string
	SignalType  string
	Reason      string
}

type PaymentIntent struct {
	CanonicalID string
	Action      string
	Amount      string
}

type PenaltySignal struct {
	CanonicalID string
	PenaltyType string
	Reason      string
}

type NoopHooks struct{}

func (NoopHooks) OnDeliveryOutcome(context.Context, DeliveryOutcomeEvent) error { return nil }
func (NoopHooks) OnRecovery(context.Context, RecoveryEvent) error               { return nil }
func (NoopHooks) OnReputationSignal(context.Context, ReputationSignal) error    { return nil }
func (NoopHooks) OnPaymentIntent(context.Context, PaymentIntent) error          { return nil }
func (NoopHooks) OnPenaltySignal(context.Context, PenaltySignal) error          { return nil }
