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
	RecipientID           string
	DirectURL             string
	DirectToken           string
	RelayURL              string
	DirectHints           []string
	StoreForwardHints     []string
	TransportCapabilities []string
	LastSuccessfulRoute   string
	LastSeenAt            time.Time
}

type RouteOutcome struct {
	MessageID  string
	Route      transport.RouteCandidate
	Outcome    string
	Success    bool
	Retryable  bool
	Error      string
	Cursor     string
	OccurredAt time.Time
}

const (
	RouteOutcomeDelivered = "delivered"
	RouteOutcomeQueued    = "queued"
	RouteOutcomeFailed    = "failed"
	RouteOutcomeRecovered = "recovered"
	RouteOutcomeAcked     = "acked"
	RouteOutcomeAckFailed = "ack_failed"
)

type Planner interface {
	PlanSend(ctx context.Context, contact ContactRuntimeView, presence discovery.PeerPresenceView) ([]transport.RouteCandidate, error)
	PlanRecover(ctx context.Context, contact ContactRuntimeView, presence discovery.PeerPresenceView) ([]transport.RouteCandidate, error)
	RecordOutcome(ctx context.Context, outcome RouteOutcome) error
}
