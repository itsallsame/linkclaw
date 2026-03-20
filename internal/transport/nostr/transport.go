package nostr

import (
	"context"
	"fmt"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type Publisher interface {
	Publish(ctx context.Context, env transport.Envelope, route transport.RouteCandidate) (transport.SendResult, error)
	Recover(ctx context.Context, route transport.RouteCandidate) (transport.SyncResult, error)
	Acknowledge(ctx context.Context, route transport.RouteCandidate, cursor string) error
}

type Transport struct {
	publisher Publisher
}

func New(publisher Publisher) *Transport {
	return &Transport{publisher: publisher}
}

func (t *Transport) Name() string { return "nostr" }

func (t *Transport) Supports(route transport.RouteCandidate) bool {
	return route.Type == transport.RouteTypeNostr
}

func (t *Transport) Send(ctx context.Context, env transport.Envelope, route transport.RouteCandidate) (transport.SendResult, error) {
	if t.publisher == nil {
		return transport.SendResult{}, fmt.Errorf("nostr transport is not configured")
	}
	return t.publisher.Publish(ctx, env, route)
}

func (t *Transport) Sync(ctx context.Context, route transport.RouteCandidate) (transport.SyncResult, error) {
	if t.publisher == nil {
		return transport.SyncResult{}, fmt.Errorf("nostr transport is not configured")
	}
	return t.publisher.Recover(ctx, route)
}

func (t *Transport) Ack(ctx context.Context, route transport.RouteCandidate, cursor string) error {
	if t.publisher == nil {
		return fmt.Errorf("nostr transport is not configured")
	}
	return t.publisher.Acknowledge(ctx, route, cursor)
}
