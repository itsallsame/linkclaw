package nostr

import (
	"context"
	"testing"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

func TestServiceResolvePeerReturnsNostrRoute(t *testing.T) {
	service := NewService(PresenceConfig{
		CanonicalID: "did:key:z6MkNostr",
		RelayHint:   "wss://relay.example",
	})
	view, err := service.ResolvePeer(context.Background(), "did:key:z6MkNostr")
	if err != nil {
		t.Fatalf("ResolvePeer() error = %v", err)
	}
	if view.Source != "nostr" {
		t.Fatalf("view.Source = %q, want nostr", view.Source)
	}
	if len(view.RouteCandidates) != 1 || view.RouteCandidates[0].Type != transport.RouteTypeNostr {
		t.Fatalf("view.RouteCandidates = %+v, want one nostr route", view.RouteCandidates)
	}
}
