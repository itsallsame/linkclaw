package dht

import (
	"context"
	"strings"
	"sync"
	"time"

	agentdiscovery "github.com/xiewanpeng/claw-identity/internal/discovery"
	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type PresenceConfig struct {
	CanonicalID      string
	PeerID           string
	DirectHint       string
	SignedPeerRecord string
	Reachable        bool
	ResolvedAt       time.Time
	TTL              time.Duration
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
		ttl = 10 * time.Minute
	}
	directHint := strings.TrimSpace(cfg.DirectHint)
	routes := make([]transport.RouteCandidate, 0, 1)
	if directHint != "" {
		routes = append(routes, transport.RouteCandidate{
			Type:     transport.RouteTypeDirect,
			Label:    "dht:" + strings.TrimSpace(cfg.PeerID),
			Priority: 80,
			Target:   directHint,
		})
	}
	return &Service{
		view: agentdiscovery.PeerPresenceView{
			CanonicalID:           strings.TrimSpace(cfg.CanonicalID),
			PeerID:                strings.TrimSpace(cfg.PeerID),
			Reachable:             cfg.Reachable && len(routes) > 0,
			RouteCandidates:       routes,
			TransportCapabilities: nonEmptyList(string(transport.RouteTypeDirect)),
			DirectHints:           nonEmptyList(directHint),
			ResolvedAt:            resolvedAt,
			FreshUntil:            resolvedAt.Add(ttl),
			Source:                "dht",
			SignedPeerRecord:      strings.TrimSpace(cfg.SignedPeerRecord),
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
		s.view.FreshUntil = now.Add(10 * time.Minute)
	}
	return s.view, nil
}

func (s *Service) PublishSelf(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	s.view.AnnouncedAt = now
	s.view.Source = "dht-announce"
	if s.view.ResolvedAt.IsZero() {
		s.view.ResolvedAt = now
	}
	if s.view.FreshUntil.Before(now) {
		s.view.FreshUntil = now.Add(10 * time.Minute)
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
