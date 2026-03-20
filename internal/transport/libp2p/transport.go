package libp2p

import (
	"context"
	"fmt"
	"strings"

	discoverylibp2p "github.com/xiewanpeng/claw-identity/internal/discovery/libp2p"
	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type DirectDialer interface {
	SendDirect(ctx context.Context, env transport.Envelope, route transport.RouteCandidate) (transport.SendResult, error)
}

type Transport struct {
	Dialer DirectDialer
}

func New(dialer DirectDialer) *Transport {
	return &Transport{Dialer: dialer}
}

func (t *Transport) Name() string { return "libp2p_direct" }

func (t *Transport) Supports(route transport.RouteCandidate) bool {
	return route.Type == transport.RouteTypeDirect
}

func (t *Transport) Send(ctx context.Context, env transport.Envelope, route transport.RouteCandidate) (transport.SendResult, error) {
	if !t.Supports(route) {
		return transport.SendResult{}, fmt.Errorf("unsupported route type %q", route.Type)
	}
	if t.Dialer == nil {
		return transport.SendResult{}, fmt.Errorf("libp2p direct transport is not configured")
	}
	if strings.TrimSpace(route.Target) == "" {
		return transport.SendResult{}, fmt.Errorf("direct route target is required")
	}
	if session := discoverylibp2p.ResolveSession(route.Target); session != nil && session.Receiver != nil {
		if err := session.Receiver.ReceiveDirect(ctx, env); err != nil {
			return transport.SendResult{}, err
		}
		return transport.SendResult{
			Route:       route,
			RemoteID:    env.MessageID,
			Delivered:   true,
			Retryable:   false,
			Description: "direct delivered",
		}, nil
	}
	return t.Dialer.SendDirect(ctx, env, route)
}

func (t *Transport) Sync(context.Context, transport.RouteCandidate) (transport.SyncResult, error) {
	return transport.SyncResult{}, fmt.Errorf("libp2p direct transport does not implement sync")
}

func (t *Transport) Ack(context.Context, transport.RouteCandidate, string) error {
	return nil
}
