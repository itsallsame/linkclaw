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
	Peer    routing.ContactRuntimeView `json:"peer"`
	Refresh bool                       `json:"refresh"`
}

type ConnectPeerPromotion struct {
	ContactID              string `json:"contact_id,omitempty"`
	ContactStatus          string `json:"contact_status,omitempty"`
	ContactCreated         bool   `json:"contact_created"`
	TrustLinked            bool   `json:"trust_linked"`
	TrustLevel             string `json:"trust_level,omitempty"`
	TrustVerificationState string `json:"trust_verification_state,omitempty"`
	TrustSource            string `json:"trust_source,omitempty"`
	DiscoveryUpdated       bool   `json:"discovery_updated"`
	DiscoverySource        string `json:"discovery_source,omitempty"`
	NoteWritten            bool   `json:"note_written"`
	PinWritten             bool   `json:"pin_written"`
	EventID                string `json:"event_id,omitempty"`
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
	Promotion     ConnectPeerPromotion       `json:"promotion"`
}
