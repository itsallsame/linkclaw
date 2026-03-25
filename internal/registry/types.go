package registry

import "github.com/xiewanpeng/claw-identity/internal/card"

type PublishRequest struct {
	IdentityCard card.Card `json:"identity_card"`
	Summary      string    `json:"summary,omitempty"`
	Capabilities []string  `json:"capabilities,omitempty"`
	Tags         []string  `json:"tags,omitempty"`
}

type AgentRecord struct {
	AgentID      string    `json:"agent_id"`
	CanonicalID  string    `json:"canonical_id"`
	DisplayName  string    `json:"display_name"`
	Summary      string    `json:"summary,omitempty"`
	Capabilities []string  `json:"capabilities,omitempty"`
	Tags         []string  `json:"tags,omitempty"`
	IdentityCard card.Card `json:"identity_card"`
	PublishedAt  string    `json:"published_at"`
	UpdatedAt    string    `json:"updated_at"`
	ProfileURL   string    `json:"profile_url,omitempty"`
	CardURL      string    `json:"card_url,omitempty"`
}

type SearchOptions struct {
	Query      string
	Capability string
	Tag        string
	Limit      int
}

type SearchResult struct {
	Records []AgentRecord `json:"records"`
}
