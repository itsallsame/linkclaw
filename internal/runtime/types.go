package runtime

import (
	"github.com/xiewanpeng/claw-identity/internal/discovery"
	"github.com/xiewanpeng/claw-identity/internal/routing"
	"github.com/xiewanpeng/claw-identity/internal/transport"
	"github.com/xiewanpeng/claw-identity/internal/trust"
)

type SendRequest struct {
	MessageID   string
	ContactRef  string
	SenderID    string
	RecipientID string
	Plaintext   string
}

type SendResult struct {
	MessageID     string
	Status        string
	SelectedRoute transport.RouteCandidate
	Transport     string
}

type SyncRequest struct {
	ContactRef string
}

type SyncResult struct {
	Synced     int
	RoutesUsed []string
}

type RecoverRequest struct {
	ContactRef string
}

type RecoverResult struct {
	Recovered  int
	RoutesUsed []string
}

type AckRequest struct {
	RouteName string
	Cursor    string
}

type Status struct {
	IdentityReady     bool
	TransportReady    bool
	DiscoveryReady    bool
	RuntimeMode       string
	BackgroundRuntime bool
}

type InspectTrustRequest struct {
	CanonicalID string
}

type InspectTrustResult struct {
	CanonicalID string             `json:"canonical_id"`
	Found       bool               `json:"found"`
	Profile     trust.TrustProfile `json:"profile"`
	Summary     trust.TrustSummary `json:"summary"`
	InspectedAt string             `json:"inspected_at"`
}

type ListDiscoveryRequest struct {
	Capability   string   `json:"capability,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Source       string   `json:"source,omitempty"`
	FreshOnly    bool     `json:"fresh_only,omitempty"`
	Limit        int      `json:"limit,omitempty"`
}

type ListDiscoveryResult struct {
	Query   discovery.FindQuery        `json:"query"`
	Records []discovery.DiscoveryEntry `json:"records"`
	FoundAt string                     `json:"found_at"`
}

type ConnectPeerRequest struct {
	Contact routing.ContactRuntimeView `json:"contact"`
	Refresh bool                       `json:"refresh"`
}

type ConnectPeerResult struct {
	CanonicalID   string                     `json:"canonical_id"`
	Trust         trust.TrustSummary         `json:"trust"`
	Presence      discovery.PeerPresenceView `json:"presence"`
	Routes        []transport.RouteCandidate `json:"routes"`
	SelectedRoute transport.RouteCandidate   `json:"selected_route,omitempty"`
	Transport     string                     `json:"transport,omitempty"`
	Connected     bool                       `json:"connected"`
	Reason        string                     `json:"reason,omitempty"`
	ConnectedAt   string                     `json:"connected_at"`
}
