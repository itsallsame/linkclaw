package nostr

import (
	"context"
	"testing"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type stubPublisher struct {
	published bool
	recovered bool
	acked     []string
}

func (s *stubPublisher) Publish(_ context.Context, env transport.Envelope, route transport.RouteCandidate) (transport.SendResult, error) {
	s.published = true
	return transport.SendResult{Route: route, RemoteID: env.MessageID, Delivered: true}, nil
}

func (s *stubPublisher) Recover(context.Context, transport.RouteCandidate) (transport.SyncResult, error) {
	s.recovered = true
	return transport.SyncResult{Recovered: 1}, nil
}

func (s *stubPublisher) Acknowledge(_ context.Context, _ transport.RouteCandidate, cursor string) error {
	s.acked = append(s.acked, cursor)
	return nil
}

func TestTransportSupportsNostrRoutes(t *testing.T) {
	tr := New(nil)
	if !tr.Supports(transport.RouteCandidate{Type: transport.RouteTypeNostr}) {
		t.Fatal("Supports() = false, want true")
	}
	if tr.Supports(transport.RouteCandidate{Type: transport.RouteTypeDirect}) {
		t.Fatal("Supports(direct) = true, want false")
	}
}

func TestTransportSendRequiresPublisher(t *testing.T) {
	tr := New(nil)
	if _, err := tr.Send(context.Background(), transport.Envelope{}, transport.RouteCandidate{
		Type:   transport.RouteTypeNostr,
		Target: "wss://relay.example",
	}); err == nil {
		t.Fatal("Send() error = nil, want publisher configuration error")
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

func TestTransportSyncAndAckUsePublisher(t *testing.T) {
	pub := &stubPublisher{}
	tr := New(pub)
	route := transport.RouteCandidate{
		Type:   transport.RouteTypeNostr,
		Target: "wss://relay.example",
	}
	syncResult, err := tr.Sync(context.Background(), route)
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if !pub.recovered || syncResult.Recovered != 1 {
		t.Fatalf("publisher recovered=%v recovered_count=%d, want true/1", pub.recovered, syncResult.Recovered)
	}
	if err := tr.Ack(context.Background(), route, "cursor-1"); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	if len(pub.acked) != 1 || pub.acked[0] != "cursor-1" {
		t.Fatalf("acked=%#v, want [cursor-1]", pub.acked)
	}
}

func TestTransportRejectsUnsupportedRouteTypes(t *testing.T) {
	pub := &stubPublisher{}
	tr := New(pub)
	unsupported := transport.RouteCandidate{
		Type:   transport.RouteTypeDirect,
		Target: "libp2p://peer",
	}
	if _, err := tr.Send(context.Background(), transport.Envelope{}, unsupported); err == nil {
		t.Fatal("Send() error = nil, want unsupported route type error")
	}
	if _, err := tr.Sync(context.Background(), unsupported); err == nil {
		t.Fatal("Sync() error = nil, want unsupported route type error")
	}
	if err := tr.Ack(context.Background(), unsupported, "cursor-1"); err == nil {
		t.Fatal("Ack() error = nil, want unsupported route type error")
	}
}
