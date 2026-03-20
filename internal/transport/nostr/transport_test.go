package nostr

import (
	"context"
	"testing"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type stubPublisher struct {
	published bool
}

func (s *stubPublisher) Publish(_ context.Context, env transport.Envelope, route transport.RouteCandidate) (transport.SendResult, error) {
	s.published = true
	return transport.SendResult{Route: route, RemoteID: env.MessageID, Delivered: true}, nil
}

func (s *stubPublisher) Recover(context.Context, transport.RouteCandidate) (transport.SyncResult, error) {
	return transport.SyncResult{Recovered: 1}, nil
}

func (s *stubPublisher) Acknowledge(context.Context, transport.RouteCandidate, string) error {
	return nil
}

func TestTransportSupportsNostrRoutes(t *testing.T) {
	tr := New(nil)
	if !tr.Supports(transport.RouteCandidate{Type: transport.RouteTypeNostr}) {
		t.Fatal("Supports() = false, want true")
	}
}

func TestTransportSendUsesPublisher(t *testing.T) {
	pub := &stubPublisher{}
	tr := New(pub)
	result, err := tr.Send(context.Background(), transport.Envelope{MessageID: "msg_nostr"}, transport.RouteCandidate{
		Type:   transport.RouteTypeNostr,
		Target: "wss://relay.example",
	})
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !pub.published {
		t.Fatal("publisher was not called")
	}
	if !result.Delivered {
		t.Fatal("Delivered = false, want true")
	}
}
