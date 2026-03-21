package runtime

import "github.com/xiewanpeng/claw-identity/internal/transport"

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
