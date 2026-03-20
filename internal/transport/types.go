package transport

import "context"

type RouteType string

const (
	RouteTypeDirect       RouteType = "direct"
	RouteTypeStoreForward RouteType = "store_forward"
	RouteTypeRecovery     RouteType = "recovery"
	RouteTypeNostr        RouteType = "nostr"
)

type RouteCandidate struct {
	Type     RouteType
	Label    string
	Priority int
	Target   string
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
	Route         RouteCandidate
	Recovered     int
	AdvancedCursor string
}

type Transport interface {
	Name() string
	Supports(route RouteCandidate) bool
	Send(ctx context.Context, env Envelope, route RouteCandidate) (SendResult, error)
	Sync(ctx context.Context, route RouteCandidate) (SyncResult, error)
	Ack(ctx context.Context, route RouteCandidate, cursor string) error
}
