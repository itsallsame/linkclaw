package nostr

import (
	"context"
	"strings"
	"time"

	agentdiscovery "github.com/xiewanpeng/claw-identity/internal/discovery"
	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type PresenceConfig struct {
	CanonicalID      string
	RelayHint        string
	SignedPeerRecord string
	ResolvedAt       time.Time
	TTL              time.Duration
}

type Service struct {
	view agentdiscovery.PeerPresenceView
}

func NewService(cfg PresenceConfig) *Service {
	resolvedAt := cfg.ResolvedAt.UTC()
	if resolvedAt.IsZero() {
		resolvedAt = time.Now().UTC()
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	relayHint := strings.TrimSpace(cfg.RelayHint)
	routes := make([]transport.RouteCandidate, 0, 1)
	if relayHint != "" {
		routes = append(routes, transport.RouteCandidate{
			Type:     transport.RouteTypeNostr,
			Label:    relayHint,
			Priority: 20,
			Target:   relayHint,
		})
	}
	return &Service{
		view: agentdiscovery.PeerPresenceView{
			CanonicalID:           strings.TrimSpace(cfg.CanonicalID),
			Reachable:             len(routes) > 0,
			RouteCandidates:       routes,
			TransportCapabilities: nonEmptyList(string(transport.RouteTypeNostr)),
			StoreForwardHints:     nonEmptyList(relayHint),
			ResolvedAt:            resolvedAt,
			FreshUntil:            resolvedAt.Add(ttl),
			Source:                "nostr",
			SignedPeerRecord:      strings.TrimSpace(cfg.SignedPeerRecord),
		},
	}
}

func (s *Service) ResolvePeer(context.Context, string) (agentdiscovery.PeerPresenceView, error) {
	return s.view, nil
}

func (s *Service) RefreshPeer(context.Context, string) (agentdiscovery.PeerPresenceView, error) {
	return s.view, nil
}

func (s *Service) PublishSelf(context.Context) error { return nil }

func nonEmptyList(values ...string) []string {
	items := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		items = append(items, value)
	}
	return items
}
