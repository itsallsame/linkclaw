package storeforward

import (
	"context"
	"fmt"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type Backend interface {
	Send(ctx context.Context, env transport.Envelope, route transport.RouteCandidate) (transport.SendResult, error)
	Recover(ctx context.Context, route transport.RouteCandidate) (transport.SyncResult, error)
	Acknowledge(ctx context.Context, route transport.RouteCandidate, cursor string) error
}

type Transport struct {
	backend Backend
}

func New(backend Backend) *Transport {
	return &Transport{backend: backend}
}

func (t *Transport) Name() string { return "store_forward" }

func (t *Transport) Supports(route transport.RouteCandidate) bool {
	return route.Type == transport.RouteTypeStoreForward || route.Type == transport.RouteTypeRecovery
}

func (t *Transport) Send(ctx context.Context, env transport.Envelope, route transport.RouteCandidate) (transport.SendResult, error) {
	if !t.Supports(route) {
		return transport.SendResult{}, fmt.Errorf("unsupported route type %q", route.Type)
	}
	if t.backend == nil {
		return transport.SendResult{}, fmt.Errorf("store-forward transport is not configured")
	}
	return t.backend.Send(ctx, env, route)
}

func (t *Transport) Sync(ctx context.Context, route transport.RouteCandidate) (transport.SyncResult, error) {
	if !t.Supports(route) {
		return transport.SyncResult{}, fmt.Errorf("unsupported route type %q", route.Type)
	}
	if t.backend == nil {
		return transport.SyncResult{}, fmt.Errorf("store-forward transport is not configured")
	}
	return t.backend.Recover(ctx, route)
}

func (t *Transport) Ack(ctx context.Context, route transport.RouteCandidate, cursor string) error {
	if !t.Supports(route) {
		return fmt.Errorf("unsupported route type %q", route.Type)
	}
	if t.backend == nil {
		return fmt.Errorf("store-forward transport is not configured")
	}
	return t.backend.Acknowledge(ctx, route, cursor)
}
