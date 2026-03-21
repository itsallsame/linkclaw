package storeforward

import (
	"context"
	"testing"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

type stubBackend struct {
	sent      bool
	recovered bool
	acked     []string
}

func (s *stubBackend) Send(_ context.Context, env transport.Envelope, route transport.RouteCandidate) (transport.SendResult, error) {
	s.sent = true
	return transport.SendResult{Route: route, RemoteID: env.MessageID, Delivered: true}, nil
}

func (s *stubBackend) Recover(_ context.Context, route transport.RouteCandidate) (transport.SyncResult, error) {
	s.recovered = true
	return transport.SyncResult{Route: route, Recovered: 1, AdvancedCursor: "cursor-1"}, nil
}

func (s *stubBackend) Acknowledge(_ context.Context, _ transport.RouteCandidate, cursor string) error {
	s.acked = append(s.acked, cursor)
	return nil
}

func TestTransportSupportsStoreForwardAndRecovery(t *testing.T) {
	tr := New(nil)
	if !tr.Supports(transport.RouteCandidate{Type: transport.RouteTypeStoreForward}) {
		t.Fatal("Supports(store_forward) = false, want true")
	}
	if !tr.Supports(transport.RouteCandidate{Type: transport.RouteTypeRecovery}) {
		t.Fatal("Supports(recovery) = false, want true")
	}
}

func TestTransportDelegatesToBackend(t *testing.T) {
	backend := &stubBackend{}
	tr := New(backend)
	route := transport.RouteCandidate{Type: transport.RouteTypeStoreForward, Target: "sf://test"}

	sendResult, err := tr.Send(context.Background(), transport.Envelope{MessageID: "msg_1"}, route)
	if err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if !backend.sent || !sendResult.Delivered {
		t.Fatalf("backend.sent=%v delivered=%v, want true/true", backend.sent, sendResult.Delivered)
	}

	syncResult, err := tr.Sync(context.Background(), transport.RouteCandidate{Type: transport.RouteTypeRecovery, Target: "sf://test"})
	if err != nil {
		t.Fatalf("Sync() error = %v", err)
	}
	if !backend.recovered || syncResult.Recovered != 1 {
		t.Fatalf("backend.recovered=%v recovered=%d, want true/1", backend.recovered, syncResult.Recovered)
	}

	if err := tr.Ack(context.Background(), route, "cursor-1"); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	if len(backend.acked) != 1 || backend.acked[0] != "cursor-1" {
		t.Fatalf("backend.acked=%#v, want [cursor-1]", backend.acked)
	}
}
