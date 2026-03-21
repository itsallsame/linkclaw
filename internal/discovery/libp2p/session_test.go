package libp2p

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestSessionSendDirectPostsEnvelopeToHTTPRoute(t *testing.T) {
	var method string
	var contentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		contentType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	session, err := BootSession(SessionConfig{
		Enabled:          true,
		CanonicalID:      "did:key:z6MkAlice",
		SigningPublicKey: "alice-signing-key",
	})
	if err != nil {
		t.Fatalf("BootSession() error = %v", err)
	}
	result, err := session.SendDirect(context.Background(), transport.Envelope{MessageID: "msg_1"}, transport.RouteCandidate{
		Type:   transport.RouteTypeDirect,
		Target: server.URL,
	})
	if err != nil {
		t.Fatalf("SendDirect() error = %v", err)
	}
	if !result.Delivered {
		t.Fatal("SendDirect() delivered = false, want true")
	}
	if method != http.MethodPost {
		t.Fatalf("method = %q, want POST", method)
	}
	if contentType != "application/json" {
		t.Fatalf("content-type = %q, want application/json", contentType)
	}
}
