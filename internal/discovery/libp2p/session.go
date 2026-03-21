package libp2p

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/transport"
)

const (
	EnvExperimentalDirect = "LINKCLAW_EXPERIMENTAL_DIRECT"
	EnvDirectAddress      = "LINKCLAW_DIRECT_ADDRESS"
)

type SessionConfig struct {
	CanonicalID         string
	SigningPublicKey    string
	EncryptionPublicKey string
	ListenAddress       string
	Receiver            DirectReceiver
	Enabled             bool
	Now                 time.Time
}

type DirectReceiver interface {
	ReceiveDirect(ctx context.Context, env transport.Envelope) error
}

type Session struct {
	Enabled       bool
	Peer          PeerIdentity
	ListenAddress string
	StartedAt     time.Time
	Receiver      DirectReceiver
}

var (
	sessionRegistryMu sync.Mutex
	sessionRegistry   = map[string]*Session{}
)

func DirectEnabledFromEnv() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(EnvExperimentalDirect))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func BootSession(cfg SessionConfig) (*Session, error) {
	if !cfg.Enabled {
		return &Session{}, nil
	}
	peer, err := DerivePeerIdentity(IdentityInput{
		CanonicalID:         cfg.CanonicalID,
		SigningPublicKey:    cfg.SigningPublicKey,
		EncryptionPublicKey: cfg.EncryptionPublicKey,
	})
	if err != nil {
		return nil, fmt.Errorf("derive libp2p peer identity: %w", err)
	}
	startedAt := cfg.Now.UTC()
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	listenAddress := strings.TrimSpace(cfg.ListenAddress)
	if listenAddress == "" {
		listenAddress = strings.TrimSpace(os.Getenv(EnvDirectAddress))
	}
	return &Session{
		Enabled:       true,
		Peer:          peer,
		ListenAddress: listenAddress,
		StartedAt:     startedAt,
		Receiver:      cfg.Receiver,
	}, nil
}

func (s *Session) SendDirect(ctx context.Context, env transport.Envelope, route transport.RouteCandidate) (transport.SendResult, error) {
	if s == nil || !s.Enabled {
		return transport.SendResult{}, fmt.Errorf("libp2p direct session is disabled")
	}
	RegisterSession(s)
	target := strings.TrimSpace(route.Target)
	if target == "" {
		return transport.SendResult{}, fmt.Errorf("direct route target is required")
	}
	if !strings.HasPrefix(target, "http://") && !strings.HasPrefix(target, "https://") {
		return transport.SendResult{}, fmt.Errorf("libp2p direct session is not connected")
	}
	body, err := json.Marshal(env)
	if err != nil {
		return transport.SendResult{}, fmt.Errorf("encode direct envelope: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
	if err != nil {
		return transport.SendResult{}, fmt.Errorf("build direct request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return transport.SendResult{}, fmt.Errorf("send direct request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return transport.SendResult{}, fmt.Errorf("direct endpoint returned http %d", res.StatusCode)
	}
	return transport.SendResult{
		Route:       route,
		RemoteID:    env.MessageID,
		Delivered:   true,
		Retryable:   false,
		Description: "direct delivered",
	}, nil
}

func RegisterSession(session *Session) {
	if session == nil || !session.Enabled {
		return
	}
	sessionRegistryMu.Lock()
	defer sessionRegistryMu.Unlock()
	sessionRegistry[session.Peer.PeerID] = session
	if session.ListenAddress != "" {
		sessionRegistry[session.ListenAddress] = session
	}
}

func ResolveSession(target string) *Session {
	sessionRegistryMu.Lock()
	defer sessionRegistryMu.Unlock()
	target = strings.TrimSpace(target)
	target = strings.TrimPrefix(target, "libp2p://")
	return sessionRegistry[target]
}
