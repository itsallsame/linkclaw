package routing

import (
	"context"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/discovery"
	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type ContactRuntimeView struct {
	ContactID             string
	CanonicalID           string
	DisplayName           string
	PeerID                string
	TransportCapabilities []string
	LastSuccessfulRoute   string
	LastSeenAt            time.Time
}

type RouteOutcome struct {
	MessageID  string
	Route      transport.RouteCandidate
	Success    bool
	Retryable  bool
	Error      string
	OccurredAt time.Time
}

type Planner interface {
	PlanSend(ctx context.Context, contact ContactRuntimeView, presence discovery.PeerPresenceView) ([]transport.RouteCandidate, error)
	PlanRecover(ctx context.Context, contact ContactRuntimeView, presence discovery.PeerPresenceView) ([]transport.RouteCandidate, error)
	RecordOutcome(ctx context.Context, outcome RouteOutcome) error
}
