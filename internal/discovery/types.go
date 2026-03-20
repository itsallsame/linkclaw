package discovery

import (
	"context"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type PeerPresenceView struct {
	CanonicalID           string
	PeerID                string
	Reachable             bool
	RouteCandidates       []transport.RouteCandidate
	TransportCapabilities []string
	DirectHints           []string
	StoreForwardHints     []string
	ResolvedAt            time.Time
	FreshUntil            time.Time
	AnnouncedAt           time.Time
	Source                string
	SignedPeerRecord      string
}

type Service interface {
	ResolvePeer(ctx context.Context, canonicalID string) (PeerPresenceView, error)
	RefreshPeer(ctx context.Context, canonicalID string) (PeerPresenceView, error)
	PublishSelf(ctx context.Context) error
}
