package dht

import (
	"context"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

func TestServiceResolvePeerReturnsDirectRoute(t *testing.T) {
	service := NewService(PresenceConfig{
		CanonicalID: "did:key:z6MkDHT",
		PeerID:      "lcpeer:dht-1",
		DirectHint:  "libp2p://lcpeer:dht-1",
		Reachable:   true,
		ResolvedAt:  time.Now().UTC(),
	})

	view, err := service.ResolvePeer(context.Background(), "did:key:z6MkDHT")
	if err != nil {
		t.Fatalf("ResolvePeer() error = %v", err)
	}
	if view.Source != "dht" {
		t.Fatalf("view.Source = %q, want dht", view.Source)
	}
	if len(view.RouteCandidates) != 1 || view.RouteCandidates[0].Type != transport.RouteTypeDirect {
		t.Fatalf("view.RouteCandidates = %+v, want one direct route", view.RouteCandidates)
	}
}

func TestServicePublishSelfMarksDHTAnnounce(t *testing.T) {
	service := NewService(PresenceConfig{
		CanonicalID: "did:key:z6MkDHT",
		PeerID:      "lcpeer:dht-1",
		DirectHint:  "libp2p://lcpeer:dht-1",
		Reachable:   true,
	})
	if err := service.PublishSelf(context.Background()); err != nil {
		t.Fatalf("PublishSelf() error = %v", err)
	}
	view, err := service.ResolvePeer(context.Background(), "did:key:z6MkDHT")
	if err != nil {
		t.Fatalf("ResolvePeer() error = %v", err)
	}
	if view.Source != "dht-announce" {
		t.Fatalf("view.Source = %q, want dht-announce", view.Source)
	}
	if view.AnnouncedAt.IsZero() {
		t.Fatal("view.AnnouncedAt = zero, want timestamp")
	}
}
