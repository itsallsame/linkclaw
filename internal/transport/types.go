package transport

import "context"

type RouteType string

const (
	RouteTypeDirect       RouteType = "direct"
	RouteTypeStoreForward RouteType = "store_forward"
	RouteTypeRecovery     RouteType = "recovery"
	// RouteTypeNostr is intentionally reserved for future rollout, not P0.
	RouteTypeNostr RouteType = "nostr"
)

func IsKnownRouteType(routeType RouteType) bool {
	switch routeType {
	case RouteTypeDirect, RouteTypeStoreForward, RouteTypeRecovery, RouteTypeNostr:
		return true
	default:
		return false
	}
}

func IsP0RouteType(routeType RouteType) bool {
	switch routeType {
	case RouteTypeDirect, RouteTypeStoreForward, RouteTypeRecovery:
		return true
	default:
		return false
	}
}

func IsReservedRouteType(routeType RouteType) bool {
	return routeType == RouteTypeNostr
}

type RouteCandidate struct {
	Type     RouteType
	Label    string
	Priority int
	Target   string
}

func (r RouteCandidate) IsP0() bool {
	return IsP0RouteType(r.Type)
}

type Envelope struct {
	MessageID   string
	SenderID    string
	RecipientID string
	Plaintext   string
	Ciphertext  string
}

type SendResult struct {
	Route       RouteCandidate
	RemoteID    string
	Delivered   bool
	Retryable   bool
	Description string
}

type SyncResult struct {
	Route          RouteCandidate
	Recovered      int
	AdvancedCursor string
}

type Transport interface {
	Name() string
	Supports(route RouteCandidate) bool
	Send(ctx context.Context, env Envelope, route RouteCandidate) (SendResult, error)
	Sync(ctx context.Context, route RouteCandidate) (SyncResult, error)
	Ack(ctx context.Context, route RouteCandidate, cursor string) error
}
