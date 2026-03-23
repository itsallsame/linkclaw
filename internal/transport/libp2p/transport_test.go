package libp2p

import (
	"context"
	"testing"

	discoverylibp2p "github.com/xiewanpeng/claw-identity/internal/discovery/libp2p"
	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type stubDialer struct {
	called bool
	result transport.SendResult
	err    error
}

func (s *stubDialer) SendDirect(_ context.Context, _ transport.Envelope, _ transport.RouteCandidate) (transport.SendResult, error) {
	s.called = true
	return s.result, s.err
}

func TestTransportSendRequiresDialer(t *testing.T) {
	tr := New(nil)
	if !tr.Supports(transport.RouteCandidate{Type: transport.RouteTypeDirect}) {
		t.Fatal("Supports(direct) = false, want true")
	}
	if tr.Supports(transport.RouteCandidate{Type: transport.RouteTypeStoreForward}) {
		t.Fatal("Supports(store_forward) = true, want false")
	}
	_, err := tr.Send(context.Background(), transport.Envelope{}, transport.RouteCandidate{
		Type:   transport.RouteTypeDirect,
		Target: "/ip4/127.0.0.1/tcp/4001/p2p/peer",
	})
	if err == nil {
		t.Fatal("Send() error = nil, want error")
	}
}

func TestTransportSendUsesConfiguredDialer(t *testing.T) {
	dialer := &stubDialer{
		result: transport.SendResult{
			Route:       transport.RouteCandidate{Type: transport.RouteTypeDirect, Target: "/ip4/127.0.0.1/tcp/4001/p2p/peer"},
			Delivered:   true,
			Retryable:   true,
			Description: "direct delivered",
		},
	}
	tr := New(dialer)
	result, err := tr.Send(context.Background(), transport.Envelope{MessageID: "msg_1"}, transport.RouteCandidate{
		Type:   transport.RouteTypeDirect,
		Target: "/ip4/127.0.0.1/tcp/4001/p2p/peer",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !dialer.called {
		t.Fatal("dialer was not called")
	}
	if !result.Delivered {
		t.Fatalf("delivered = false, want true")
	}
}

func TestTransportRejectsUnsupportedRouteTypes(t *testing.T) {
	dialer := &stubDialer{}
	tr := New(dialer)
	unsupported := transport.RouteCandidate{
		Type:   transport.RouteTypeStoreForward,
		Target: "sf://relay.example",
	}

	if _, err := tr.Send(context.Background(), transport.Envelope{MessageID: "msg_unsupported"}, unsupported); err == nil {
		t.Fatal("Send() error = nil, want unsupported route type error")
	}
	if dialer.called {
		t.Fatal("dialer should not be called for unsupported route types")
	}
	if _, err := tr.Sync(context.Background(), unsupported); err == nil {
		t.Fatal("Sync() error = nil, want unsupported route type error")
	}
	if err := tr.Ack(context.Background(), unsupported, "cursor-1"); err == nil {
		t.Fatal("Ack() error = nil, want unsupported route type error")
	}
}

type captureReceiver struct {
	called bool
	env    transport.Envelope
}

func (c *captureReceiver) ReceiveDirect(_ context.Context, env transport.Envelope) error {
	c.called = true
	c.env = env
	return nil
}

func TestTransportSendUsesRegisteredSessionBeforeDialer(t *testing.T) {
	receiver := &captureReceiver{}
	discoverylibp2p.RegisterSession(&discoverylibp2p.Session{
		Enabled:  true,
		Peer:     discoverylibp2p.PeerIdentity{PeerID: "peer-direct-1"},
		Receiver: receiver,
	})

	dialer := &stubDialer{}
	tr := New(dialer)
	result, err := tr.Send(context.Background(), transport.Envelope{MessageID: "msg_direct"}, transport.RouteCandidate{
		Type:   transport.RouteTypeDirect,
		Target: "libp2p://peer-direct-1",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !receiver.called {
		t.Fatal("receiver was not called")
	}
	if dialer.called {
		t.Fatal("dialer should not be called when a direct session is registered")
	}
	if !result.Delivered {
		t.Fatalf("delivered = false, want true")
	}
}
