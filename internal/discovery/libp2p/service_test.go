package libp2p

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

func TestDerivePeerIdentity(t *testing.T) {
	identity, err := DerivePeerIdentity(IdentityInput{
		CanonicalID:         "did:key:z6MkAlice",
		SigningPublicKey:    "alice-signing-key",
		EncryptionPublicKey: "alice-encryption-key",
	})
	if err != nil {
		t.Fatalf("DerivePeerIdentity() error = %v", err)
	}
	if !strings.HasPrefix(identity.PeerID, "lcpeer:") {
		t.Fatalf("peer id = %q, want lcpeer:*", identity.PeerID)
	}
	if !strings.Contains(identity.SignedPeerRecord, "libp2p-boundary-v1") {
		t.Fatalf("signed peer record = %q, want libp2p boundary marker", identity.SignedPeerRecord)
	}
}

func TestServiceResolvePeerReturnsDirectRouteCandidate(t *testing.T) {
	identity, err := DerivePeerIdentity(IdentityInput{
		CanonicalID:      "did:key:z6MkAlice",
		SigningPublicKey: "alice-signing-key",
	})
	if err != nil {
		t.Fatalf("DerivePeerIdentity() error = %v", err)
	}
	service := NewService(PresenceConfig{
		Peer:          identity,
		DirectAddress: "/ip4/127.0.0.1/tcp/4001/p2p/" + identity.PeerID,
		Reachable:     true,
		ResolvedAt:    time.Now().UTC(),
	})
	view, err := service.ResolvePeer(context.Background(), identity.CanonicalID)
	if err != nil {
		t.Fatalf("ResolvePeer() error = %v", err)
	}
	if !view.Reachable {
		t.Fatalf("reachable = false, want true")
	}
	if len(view.RouteCandidates) != 1 {
		t.Fatalf("route candidates = %d, want 1", len(view.RouteCandidates))
	}
	if view.RouteCandidates[0].Type != transport.RouteTypeDirect {
		t.Fatalf("route type = %q, want %q", view.RouteCandidates[0].Type, transport.RouteTypeDirect)
	}
}

func TestServiceResolvePeerUsesPeerIDFallbackTarget(t *testing.T) {
	identity, err := DerivePeerIdentity(IdentityInput{
		CanonicalID:      "did:key:z6MkAlice",
		SigningPublicKey: "alice-signing-key",
	})
	if err != nil {
		t.Fatalf("DerivePeerIdentity() error = %v", err)
	}
	service := NewService(PresenceConfig{
		Peer:       identity,
		Reachable:  true,
		ResolvedAt: time.Now().UTC(),
	})
	view, err := service.ResolvePeer(context.Background(), identity.CanonicalID)
	if err != nil {
		t.Fatalf("ResolvePeer() error = %v", err)
	}
	if len(view.RouteCandidates) != 1 {
		t.Fatalf("route candidates = %d, want 1", len(view.RouteCandidates))
	}
	if got, want := view.RouteCandidates[0].Target, "libp2p://"+identity.PeerID; got != want {
		t.Fatalf("route target = %q, want %q", got, want)
	}
}

func TestServicePublishSelfMarksAnnouncedPresence(t *testing.T) {
	identity, err := DerivePeerIdentity(IdentityInput{
		CanonicalID:      "did:key:z6MkCarol",
		SigningPublicKey: "carol-signing-key",
	})
	if err != nil {
		t.Fatalf("DerivePeerIdentity() error = %v", err)
	}
	service := NewService(PresenceConfig{
		Peer:          identity,
		DirectAddress: "libp2p://peer-carol",
		Reachable:     true,
		ResolvedAt:    time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC),
	})
	if err := service.PublishSelf(context.Background()); err != nil {
		t.Fatalf("PublishSelf() error = %v", err)
	}
	view, err := service.ResolvePeer(context.Background(), identity.CanonicalID)
	if err != nil {
		t.Fatalf("ResolvePeer() error = %v", err)
	}
	if view.AnnouncedAt.IsZero() {
		t.Fatal("view.AnnouncedAt = zero, want publish timestamp")
	}
	if view.Source != "libp2p-announce" {
		t.Fatalf("view.Source = %q, want libp2p-announce", view.Source)
	}
	if len(view.TransportCapabilities) != 1 || view.TransportCapabilities[0] != string(transport.RouteTypeDirect) {
		t.Fatalf("view.TransportCapabilities = %+v, want [direct]", view.TransportCapabilities)
	}
}
