package libp2p

import (
	"context"
	"strings"
	"sync"
	"time"

	agentdiscovery "github.com/xiewanpeng/claw-identity/internal/discovery"
	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type PresenceConfig struct {
	Peer          PeerIdentity
	DirectAddress string
	Reachable     bool
	ResolvedAt    time.Time
	TTL           time.Duration
}

type Service struct {
	mu   sync.Mutex
	view agentdiscovery.PeerPresenceView
}

func NewService(cfg PresenceConfig) *Service {
	resolvedAt := cfg.ResolvedAt.UTC()
	if resolvedAt.IsZero() {
		resolvedAt = time.Now().UTC()
	}
	ttl := cfg.TTL
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	routes := make([]transport.RouteCandidate, 0, 1)
	target := strings.TrimSpace(cfg.DirectAddress)
	if target == "" && strings.TrimSpace(cfg.Peer.PeerID) != "" {
		target = "libp2p://" + strings.TrimSpace(cfg.Peer.PeerID)
	}
	if target != "" {
		routes = append(routes, transport.RouteCandidate{
			Type:     transport.RouteTypeDirect,
			Label:    cfg.Peer.PeerID,
			Priority: 100,
			Target:   target,
		})
	}
	return &Service{
		view: agentdiscovery.PeerPresenceView{
			CanonicalID:           cfg.Peer.CanonicalID,
			PeerID:                cfg.Peer.PeerID,
			Reachable:             cfg.Reachable && len(routes) > 0,
			RouteCandidates:       routes,
			TransportCapabilities: []string{string(transport.RouteTypeDirect)},
			DirectHints:           nonEmptyList(target),
			ResolvedAt:            resolvedAt,
			FreshUntil:            resolvedAt.Add(ttl),
			Source:                "libp2p",
			SignedPeerRecord:      cfg.Peer.SignedPeerRecord,
		},
	}
}

func (s *Service) ResolvePeer(context.Context, string) (agentdiscovery.PeerPresenceView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.view, nil
}

func (s *Service) RefreshPeer(context.Context, string) (agentdiscovery.PeerPresenceView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.view.ResolvedAt = now
	if s.view.FreshUntil.Before(now) {
		s.view.FreshUntil = now.Add(5 * time.Minute)
	}
	return s.view, nil
}

func (s *Service) PublishSelf(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.view.AnnouncedAt = now
	s.view.Source = "libp2p-announce"
	if s.view.ResolvedAt.IsZero() {
		s.view.ResolvedAt = now
	}
	if s.view.FreshUntil.Before(now) {
		s.view.FreshUntil = now.Add(5 * time.Minute)
	}
	return nil
}

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
