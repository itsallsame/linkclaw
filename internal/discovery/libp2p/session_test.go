package libp2p

import (
	"context"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

func TestBootSessionDisabled(t *testing.T) {
	session, err := BootSession(SessionConfig{})
	if err != nil {
		t.Fatalf("BootSession() error = %v", err)
	}
	if session == nil {
		t.Fatal("BootSession() returned nil session")
	}
	if session.Enabled {
		t.Fatal("session.Enabled = true, want false")
	}
}

func TestBootSessionDerivesPeerIdentity(t *testing.T) {
	now := time.Now().UTC()
	session, err := BootSession(SessionConfig{
		Enabled:          true,
		CanonicalID:      "did:key:z6MkAlice",
		SigningPublicKey: "alice-signing-key",
		Now:              now,
	})
	if err != nil {
		t.Fatalf("BootSession() error = %v", err)
	}
	if !session.Enabled {
		t.Fatal("session.Enabled = false, want true")
	}
	if session.Peer.PeerID == "" {
		t.Fatal("session peer id is empty")
	}
	if !session.StartedAt.Equal(now) {
		t.Fatalf("startedAt = %s, want %s", session.StartedAt, now)
	}
}

func TestSessionSendDirectReturnsBoundaryError(t *testing.T) {
	session, err := BootSession(SessionConfig{
		Enabled:          true,
		CanonicalID:      "did:key:z6MkAlice",
		SigningPublicKey: "alice-signing-key",
	})
	if err != nil {
		t.Fatalf("BootSession() error = %v", err)
	}
	_, err = session.SendDirect(context.Background(), transport.Envelope{MessageID: "msg_1"}, transport.RouteCandidate{
		Type:   transport.RouteTypeDirect,
		Target: "libp2p://peer",
	})
	if err == nil {
		t.Fatal("SendDirect() error = nil, want boundary error")
	}
}
