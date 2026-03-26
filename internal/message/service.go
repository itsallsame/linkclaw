package message

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	agentdiscovery "github.com/xiewanpeng/claw-identity/internal/discovery"
	"github.com/xiewanpeng/claw-identity/internal/ids"
	"github.com/xiewanpeng/claw-identity/internal/layout"
	"github.com/xiewanpeng/claw-identity/internal/messagecrypto"
	"github.com/xiewanpeng/claw-identity/internal/migrate"
	"github.com/xiewanpeng/claw-identity/internal/routing"
	agentruntime "github.com/xiewanpeng/claw-identity/internal/runtime"
	"github.com/xiewanpeng/claw-identity/internal/transport"
	transportstoreforward "github.com/xiewanpeng/claw-identity/internal/transport/storeforward"
	"github.com/xiewanpeng/claw-identity/internal/trust"

	_ "modernc.org/sqlite"
)

const (
	DirectionIncoming = "incoming"
	DirectionOutgoing = "outgoing"

	StatusPending    = agentruntime.MessageStatusPending
	StatusQueued     = agentruntime.MessageStatusQueued
	StatusRecovering = agentruntime.MessageStatusRecovering
	StatusRecovered  = agentruntime.MessageStatusRecovered
	StatusDelivered  = agentruntime.MessageStatusDelivered
	StatusFailed     = agentruntime.MessageStatusFailed

	TransportStatusDirect    = "direct"
	TransportStatusDeferred  = "deferred"
	TransportStatusRecovered = "recovered"
	TransportStatusFailed    = "failed"
)

type SendOptions struct {
	Home       string
	ContactRef string
	Body       string
}

type ReceiveDirectOptions struct {
	Home    string
	Payload string
}

type SyncOptions struct {
	Home string
}

type InspectTrustOptions struct {
	Home       string
	Identifier string
}

type ListDiscoveryOptions struct {
	Home         string
	Capability   string
	Capabilities []string
	Source       string
	FreshOnly    bool
	Limit        int
}

type ConnectPeerOptions struct {
	Home    string
	PeerRef string
	Refresh bool
}

type ListOptions struct {
	Home string
}

type ThreadOptions struct {
	Home       string
	ContactRef string
	Limit      int
	MarkRead   bool
}

type MessageRecord struct {
	MessageID          string                   `json:"message_id"`
	ConversationID     string                   `json:"conversation_id"`
	Direction          string                   `json:"direction"`
	SenderContactID    string                   `json:"sender_contact_id,omitempty"`
	RecipientContactID string                   `json:"recipient_contact_id,omitempty"`
	SenderCanonicalID  string                   `json:"sender_canonical_id,omitempty"`
	RecipientRouteID   string                   `json:"recipient_route_id,omitempty"`
	Body               string                   `json:"body"`
	Preview            string                   `json:"preview"`
	Status             string                   `json:"status"`
	TransportStatus    string                   `json:"transport_status,omitempty"`
	CreatedAt          string                   `json:"created_at"`
	DeliveredAt        string                   `json:"delivered_at,omitempty"`
	SelectedRoute      transport.RouteCandidate `json:"selected_route,omitempty"`
}

type Conversation struct {
	ConversationID     string          `json:"conversation_id"`
	ContactID          string          `json:"contact_id"`
	ContactDisplayName string          `json:"contact_display_name"`
	ContactCanonicalID string          `json:"contact_canonical_id"`
	ContactStatus      string          `json:"contact_status"`
	LastMessageAt      string          `json:"last_message_at,omitempty"`
	LastMessagePreview string          `json:"last_message_preview,omitempty"`
	UnreadCount        int             `json:"unread_count"`
	Messages           []MessageRecord `json:"messages,omitempty"`
}

type SendResult struct {
	Home         string        `json:"home"`
	Conversation Conversation  `json:"conversation"`
	Message      MessageRecord `json:"message"`
	SentAt       string        `json:"sent_at"`
}

type InboxResult struct {
	Home          string         `json:"home"`
	Conversations []Conversation `json:"conversations"`
	ListedAt      string         `json:"listed_at"`
}

type OutboxResult struct {
	Home     string          `json:"home"`
	Messages []MessageRecord `json:"messages"`
	ListedAt string          `json:"listed_at"`
}

type SyncResult struct {
	Home       string `json:"home"`
	Synced     int    `json:"synced"`
	RelayCalls int    `json:"relay_calls"`
	SyncedAt   string `json:"synced_at"`
}

type RouteOutcomeStatus struct {
	RouteType   string `json:"route_type"`
	RouteLabel  string `json:"route_label,omitempty"`
	Outcome     string `json:"outcome"`
	Retryable   bool   `json:"retryable"`
	Cursor      string `json:"cursor,omitempty"`
	Error       string `json:"error,omitempty"`
	AttemptedAt string `json:"attempted_at"`
}

type StatusResult struct {
	Home                   string               `json:"home"`
	SelfID                 string               `json:"self_id,omitempty"`
	DisplayName            string               `json:"display_name,omitempty"`
	PeerID                 string               `json:"peer_id,omitempty"`
	TransportCapabilities  []string             `json:"transport_capabilities,omitempty"`
	IdentityReady          bool                 `json:"identity_ready"`
	TransportReady         bool                 `json:"transport_ready"`
	DiscoveryReady         bool                 `json:"discovery_ready"`
	Contacts               int                  `json:"contacts"`
	Conversations          int                  `json:"conversations"`
	Unread                 int                  `json:"unread"`
	PendingOutbox          int                  `json:"pending_outbox"`
	MessageStatusDirect    int                  `json:"message_status_direct"`
	MessageStatusDeferred  int                  `json:"message_status_deferred"`
	MessageStatusRecovered int                  `json:"message_status_recovered"`
	PresenceEntries        int                  `json:"presence_entries"`
	ReachablePresence      int                  `json:"reachable_presence"`
	StoreForwardRoutes     int                  `json:"store_forward_routes"`
	LastStoreForwardSyncAt string               `json:"last_store_forward_sync_at,omitempty"`
	LastStoreForwardResult string               `json:"last_store_forward_result,omitempty"`
	LastStoreForwardError  string               `json:"last_store_forward_error,omitempty"`
	LastRecoveredCount     int                  `json:"last_recovered_count"`
	LastAnnounceAt         string               `json:"last_announce_at,omitempty"`
	RuntimeMode            string               `json:"runtime_mode,omitempty"`
	BackgroundRuntime      bool                 `json:"background_runtime"`
	DirectEnabled          bool                 `json:"direct_enabled"`
	RecentRouteOutcomes    []RouteOutcomeStatus `json:"recent_route_outcomes,omitempty"`
	StatusAt               string               `json:"status_at"`
}

type ThreadResult struct {
	Home         string       `json:"home"`
	Conversation Conversation `json:"conversation"`
	ListedAt     string       `json:"listed_at"`
}

type Service struct {
	Now                 func() time.Time
	StoreForwardBackend transportstoreforward.MailboxBackend
}

type selfMessagingProfile struct {
	SelfID                   string
	CanonicalID              string
	RecipientID              string
	RelayURL                 string
	DirectURL                string
	DirectToken              string
	SigningPublicKey         string
	SigningPrivateKeyPath    string
	EncryptionPrivateKeyPath string
}

type contactRecord struct {
	ContactID           string
	CanonicalID         string
	DisplayName         string
	RecipientID         string
	SigningPublicKey    string
	EncryptionPublicKey string
	RelayURL            string
	StoreForwardHints   []string
	DirectURL           string
	DirectToken         string
	Status              string
}

type signedMessagePayload struct {
	MessageID          string `json:"message_id"`
	SenderID           string `json:"sender_id"`
	SenderSigningKey   string `json:"sender_signing_key"`
	RecipientID        string `json:"recipient_id"`
	EphemeralPublicKey string `json:"ephemeral_public_key"`
	Nonce              string `json:"nonce"`
	Ciphertext         string `json:"ciphertext"`
	SentAt             string `json:"sent_at"`
}

func NewService() *Service {
	return &Service{
		Now:                 time.Now,
		StoreForwardBackend: transportstoreforward.LegacyHTTPMailboxBackend{},
	}
}

func (s *Service) Send(ctx context.Context, opts SendOptions) (SendResult, error) {
	body := strings.TrimSpace(opts.Body)
	if body == "" {
		return SendResult{}, fmt.Errorf("message body is required")
	}
	now := s.now()
	db, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return SendResult{}, err
	}
	defer db.Close()
	selfProfile, err := loadSelfMessagingProfile(ctx, db, home)
	if err != nil {
		return SendResult{}, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return SendResult{}, fmt.Errorf("begin message send transaction: %w", err)
	}
	defer tx.Rollback()

	contact, err := resolveContact(ctx, tx, opts.ContactRef)
	if err != nil {
		return SendResult{}, err
	}
	conversation, err := ensureConversation(ctx, tx, contact, now)
	if err != nil {
		return SendResult{}, err
	}
	record, err := insertOutgoingMessage(ctx, tx, conversation, contact, body, now)
	if err != nil {
		return SendResult{}, err
	}
	record.TransportStatus = deriveTransportStatus(record.Direction, record.Status, record.SelectedRoute)
	conversation.LastMessageAt = record.CreatedAt
	conversation.LastMessagePreview = record.Preview
	conversation.Messages = []MessageRecord{record}

	if err := tx.Commit(); err != nil {
		return SendResult{}, fmt.Errorf("commit message send transaction: %w", err)
	}
	runtimeContact, err := resolveRuntimeContactWithRelayView(ctx, db, selfProfile, contact, now)
	if err != nil {
		return SendResult{}, err
	}
	if err := syncRuntimeSendState(ctx, home, runtimeContact, conversation, record, now); err != nil {
		return SendResult{}, err
	}
	if hasRuntimeStoreForwardTarget(runtimeContact) || strings.TrimSpace(runtimeContact.DirectURL) != "" || directTransportEnabled() {
		runtimeResult, err := s.sendThroughRuntime(ctx, home, selfProfile, runtimeContact, record, now)
		if err != nil {
			record.Status = StatusFailed
			record.TransportStatus = deriveTransportStatus(record.Direction, record.Status, record.SelectedRoute)
			if syncErr := syncRuntimeSendState(ctx, home, runtimeContact, conversation, record, now); syncErr != nil {
				return SendResult{}, syncErr
			}
		} else {
			record.Status = runtimeResult.Status
			record.SelectedRoute = runtimeResult.SelectedRoute
			if runtimeResult.Status == StatusDelivered {
				record.DeliveredAt = now.Format(time.RFC3339Nano)
			}
			record.TransportStatus = deriveTransportStatus(record.Direction, record.Status, record.SelectedRoute)
			if err := syncRuntimeSendState(ctx, home, runtimeContact, conversation, record, now); err != nil {
				return SendResult{}, err
			}
		}
	}
	conversation.Messages = []MessageRecord{record}
	return SendResult{
		Home:         home,
		Conversation: conversation,
		Message:      record,
		SentAt:       now.Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) ReceiveDirect(ctx context.Context, opts ReceiveDirectOptions) error {
	payload := strings.TrimSpace(opts.Payload)
	if payload == "" {
		return fmt.Errorf("direct envelope payload is required")
	}
	now := s.now()
	db, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return err
	}
	defer db.Close()

	selfProfile, err := loadSelfMessagingProfile(ctx, db, home)
	if err != nil {
		return err
	}
	var env transport.Envelope
	if err := json.Unmarshal([]byte(payload), &env); err != nil {
		return fmt.Errorf("decode direct envelope: %w", err)
	}
	return s.receiveDirectEnvelope(ctx, home, selfProfile, env, now)
}

func (s *Service) Inbox(ctx context.Context, opts ListOptions) (InboxResult, error) {
	now := s.now()
	_, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return InboxResult{}, err
	}
	conversations, err := loadRuntimeInbox(ctx, home, now)
	if err != nil {
		return InboxResult{}, err
	}
	return InboxResult{
		Home:          home,
		Conversations: conversations,
		ListedAt:      now.Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) Outbox(ctx context.Context, opts ListOptions) (OutboxResult, error) {
	now := s.now()
	_, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return OutboxResult{}, err
	}
	messages, err := loadRuntimeOutbox(ctx, home, now)
	if err != nil {
		return OutboxResult{}, err
	}
	return OutboxResult{
		Home:     home,
		Messages: messages,
		ListedAt: now.Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) Thread(ctx context.Context, opts ThreadOptions) (ThreadResult, error) {
	now := s.now()
	db, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return ThreadResult{}, err
	}
	defer db.Close()
	conversation, err := loadRuntimeThread(ctx, home, opts.ContactRef, normalizeThreadLimit(opts.Limit), opts.MarkRead, now)
	if err != nil {
		return ThreadResult{}, err
	}
	if opts.MarkRead && strings.TrimSpace(conversation.ConversationID) != "" {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return ThreadResult{}, fmt.Errorf("begin message thread read-mark transaction: %w", err)
		}
		if err := markConversationRead(ctx, tx, conversation.ConversationID, now); err != nil {
			tx.Rollback()
			return ThreadResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return ThreadResult{}, fmt.Errorf("commit message thread read-mark transaction: %w", err)
		}
	}

	return ThreadResult{
		Home:         home,
		Conversation: conversation,
		ListedAt:     now.Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) Sync(ctx context.Context, opts SyncOptions) (SyncResult, error) {
	now := s.now()
	db, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return SyncResult{}, err
	}
	defer db.Close()

	selfProfile, err := loadSelfMessagingProfile(ctx, db, home)
	if err != nil {
		return SyncResult{}, err
	}
	relayURL := selfProfile.RelayURL
	if relayURL == "" {
		return SyncResult{Home: home, Synced: 0, RelayCalls: 0, SyncedAt: now.Format(time.RFC3339Nano)}, nil
	}
	syncResult, err := s.syncThroughRuntime(ctx, home, selfProfile, relayURL, now)
	if err != nil {
		return SyncResult{}, err
	}
	return SyncResult{
		Home:       home,
		Synced:     syncResult.Synced,
		RelayCalls: len(syncResult.RoutesUsed),
		SyncedAt:   now.Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) InspectTrust(ctx context.Context, opts InspectTrustOptions) (agentruntime.InspectTrustResult, error) {
	identifier := strings.TrimSpace(opts.Identifier)
	if identifier == "" {
		return agentruntime.InspectTrustResult{}, fmt.Errorf("message inspect-trust requires exactly one contact reference")
	}

	now := s.now()
	db, _, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return agentruntime.InspectTrustResult{}, err
	}
	defer db.Close()

	canonicalID := identifier
	if !strings.HasPrefix(canonicalID, "did:") {
		contact, resolveErr := resolveContactLookup(ctx, db, identifier)
		if resolveErr != nil {
			return agentruntime.InspectTrustResult{}, resolveErr
		}
		canonicalID = contact.CanonicalID
	}

	runtimeSvc := agentruntime.NewService(nil, nil)
	runtimeSvc.Trust = trust.NewServiceWithDB(db, now)
	runtimeSvc.Now = func() time.Time { return now }
	return runtimeSvc.InspectTrust(ctx, agentruntime.InspectTrustRequest{CanonicalID: canonicalID})
}

func (s *Service) ListDiscovery(ctx context.Context, opts ListDiscoveryOptions) (agentruntime.ListDiscoveryResult, error) {
	now := s.now()
	db, _, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return agentruntime.ListDiscoveryResult{}, err
	}
	defer db.Close()

	runtimeSvc := agentruntime.NewService(nil, nil)
	runtimeSvc.DiscoveryQuery = agentdiscovery.NewQueryServiceWithDB(db, now, nil)
	runtimeSvc.Now = func() time.Time { return now }
	return runtimeSvc.ListDiscovery(ctx, agentruntime.ListDiscoveryRequest{
		Capability:   strings.TrimSpace(opts.Capability),
		Capabilities: append([]string(nil), opts.Capabilities...),
		Source:       strings.TrimSpace(opts.Source),
		FreshOnly:    opts.FreshOnly,
		Limit:        opts.Limit,
	})
}

func (s *Service) ConnectPeer(ctx context.Context, opts ConnectPeerOptions) (agentruntime.ConnectPeerResult, error) {
	peerRef := strings.TrimSpace(opts.PeerRef)
	if peerRef == "" {
		return agentruntime.ConnectPeerResult{}, fmt.Errorf("message connect-peer requires exactly one peer reference")
	}

	now := s.now()
	db, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return agentruntime.ConnectPeerResult{}, err
	}
	defer db.Close()

	selfProfile, err := loadSelfMessagingProfile(ctx, db, home)
	if err != nil {
		return agentruntime.ConnectPeerResult{}, err
	}
	target, err := resolveConnectPeerTarget(ctx, db, peerRef, now)
	if err != nil {
		return agentruntime.ConnectPeerResult{}, err
	}

	var (
		extraTransports []transport.Transport
		peer            routing.ContactRuntimeView
		presenceSvc     agentdiscovery.Service
	)
	switch {
	case target.contact != nil:
		contact, err := resolveRuntimeContactWithRelayView(ctx, db, selfProfile, *target.contact, now)
		if err != nil {
			return agentruntime.ConnectPeerResult{}, err
		}
		target.contact = &contact
		_, extraTransports, _ = buildSendRuntimeBoundary(selfProfile, contact, now)
		peer = runtimeContactView(contact)
		presenceSvc = connectPresenceProvider{
			resolve: func(context.Context, string) (agentdiscovery.PeerPresenceView, error) {
				view, _, _ := buildSendRuntimeBoundary(selfProfile, contact, now)
				return view, nil
			},
			refresh: func(context.Context, string) (agentdiscovery.PeerPresenceView, error) {
				view, _, _ := buildSendRuntimeBoundary(selfProfile, contact, now)
				return view, nil
			},
		}
	case target.discovery != nil:
		record, err := resolveRuntimeDiscoveryWithRelayView(ctx, db, selfProfile, *target.discovery, now)
		if err != nil {
			return agentruntime.ConnectPeerResult{}, err
		}
		target.discovery = &record
		_, extraTransports, _ = buildDiscoveryConnectRuntimeBoundary(selfProfile, record, now)
		peer = runtimePeerViewFromDiscovery(record)
		presenceSvc = connectPresenceProvider{
			resolve: func(ctx context.Context, canonicalID string) (agentdiscovery.PeerPresenceView, error) {
				lookup, ok, err := resolveDiscoveryLookup(ctx, db, canonicalID, now)
				if err != nil {
					return agentdiscovery.PeerPresenceView{}, err
				}
				if !ok {
					return agentdiscovery.PeerPresenceView{}, fmt.Errorf("discovery record %q not found", strings.TrimSpace(canonicalID))
				}
				lookup, err = resolveRuntimeDiscoveryWithRelayView(ctx, db, selfProfile, lookup, now)
				if err != nil {
					return agentdiscovery.PeerPresenceView{}, err
				}
				view, _, _ := buildDiscoveryConnectRuntimeBoundary(selfProfile, lookup, now)
				return view, nil
			},
			refresh: func(ctx context.Context, canonicalID string) (agentdiscovery.PeerPresenceView, error) {
				lookup, ok, err := resolveDiscoveryLookup(ctx, db, canonicalID, now)
				if err != nil {
					return agentdiscovery.PeerPresenceView{}, err
				}
				if !ok {
					return agentdiscovery.PeerPresenceView{}, fmt.Errorf("discovery record %q not found", strings.TrimSpace(canonicalID))
				}
				lookup, err = resolveRuntimeDiscoveryWithRelayView(ctx, db, selfProfile, lookup, now)
				if err != nil {
					return agentdiscovery.PeerPresenceView{}, err
				}
				view, _, _ := buildDiscoveryConnectRuntimeBoundary(selfProfile, lookup, now)
				refreshedAt := now.UTC()
				view.ResolvedAt = refreshedAt
				if view.FreshUntil.Before(refreshedAt) {
					view.FreshUntil = refreshedAt.Add(5 * time.Minute)
				}
				view.Source = "refresh-peer"
				return view, nil
			},
		}
	default:
		return agentruntime.ConnectPeerResult{}, fmt.Errorf("peer %q not found in contacts or discovery", peerRef)
	}

	querySvc := agentdiscovery.NewQueryServiceWithClock(db, presenceSvc, func() time.Time { return now })
	var discoverySvc agentdiscovery.Service = presenceSvc
	if target.discovery != nil {
		discoverySvc = queryBackedDiscoveryService{
			query:    querySvc,
			fallback: presenceSvc,
			now:      func() time.Time { return now },
		}
	}
	runtimeSvc := agentruntime.NewService(
		presenceRoutesPlanner{},
		discoverySvc,
		connectReadinessTransport{name: "store_forward_ready", routeType: transport.RouteTypeStoreForward},
	)
	runtimeSvc.Transports = append(extraTransports, runtimeSvc.Transports...)
	runtimeSvc.Trust = trust.NewServiceWithDB(db, now)
	runtimeSvc.DiscoveryQuery = querySvc
	runtimeSvc.Now = func() time.Time { return now }

	result, err := runtimeSvc.ConnectPeer(ctx, agentruntime.ConnectPeerRequest{
		Peer:    peer,
		Refresh: opts.Refresh,
	})
	if err != nil {
		return agentruntime.ConnectPeerResult{}, err
	}
	if err := applyConnectPeerPromotion(ctx, db, target, &result, now); err != nil {
		return agentruntime.ConnectPeerResult{}, err
	}
	return result, nil
}

func (s *Service) Status(ctx context.Context, opts ListOptions) (StatusResult, error) {
	now := s.now()
	db, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return StatusResult{}, err
	}
	defer db.Close()

	store := agentruntime.NewStoreWithDB(db, now)
	selfDirectConfigured := false
	if err := syncRuntimeSelfIdentity(ctx, db, store); err != nil {
		return StatusResult{}, err
	}
	if selfProfile, err := loadSelfMessagingProfile(ctx, db, home); err == nil {
		if err := s.ensureDirectRuntimeRegistration(ctx, home, selfProfile, now); err != nil {
			return StatusResult{}, err
		}
		selfDirectConfigured = strings.TrimSpace(selfProfile.DirectURL) != ""
	}
	summary, err := store.LoadStatusSummary(ctx)
	if err != nil {
		return StatusResult{}, err
	}
	messages, err := store.ListMessages(ctx)
	if err != nil {
		return StatusResult{}, err
	}
	directCount, deferredCount, recoveredCount := summarizeTransportStatuses(messages)
	routeAttempts, err := store.ListRecentRouteAttempts(ctx, 6)
	if err != nil {
		return StatusResult{}, err
	}
	recentOutcomes := make([]RouteOutcomeStatus, 0, len(routeAttempts))
	for _, attempt := range routeAttempts {
		recentOutcomes = append(recentOutcomes, RouteOutcomeStatus{
			RouteType:   attempt.RouteType,
			RouteLabel:  attempt.RouteLabel,
			Outcome:     attempt.Outcome,
			Retryable:   attempt.Retryable,
			Cursor:      attempt.CursorValue,
			Error:       attempt.Error,
			AttemptedAt: attempt.AttemptedAt,
		})
	}
	identityReady := strings.TrimSpace(summary.SelfID) != ""
	transportReady := identityReady && len(summary.TransportCapabilities) > 0
	discoveryReady := summary.PresenceEntries > 0 || summary.ReachablePresence > 0

	return StatusResult{
		Home:                   home,
		SelfID:                 summary.SelfID,
		DisplayName:            summary.DisplayName,
		PeerID:                 summary.PeerID,
		TransportCapabilities:  summary.TransportCapabilities,
		IdentityReady:          identityReady,
		TransportReady:         transportReady,
		DiscoveryReady:         discoveryReady,
		Contacts:               summary.Contacts,
		Conversations:          summary.Conversations,
		Unread:                 summary.Unread,
		PendingOutbox:          summary.PendingOutbox,
		MessageStatusDirect:    directCount,
		MessageStatusDeferred:  deferredCount,
		MessageStatusRecovered: recoveredCount,
		PresenceEntries:        summary.PresenceEntries,
		ReachablePresence:      summary.ReachablePresence,
		StoreForwardRoutes:     summary.StoreForwardRoutes,
		LastStoreForwardSyncAt: summary.LastStoreForwardSyncAt,
		LastStoreForwardResult: summary.LastStoreForwardResult,
		LastStoreForwardError:  summary.LastStoreForwardError,
		LastRecoveredCount:     summary.LastStoreForwardRecover,
		LastAnnounceAt:         summary.LastAnnounceAt,
		RuntimeMode:            agentruntime.RuntimeMode(),
		BackgroundRuntime:      agentruntime.BackgroundRuntimeEnabled(),
		DirectEnabled:          directTransportEnabled() || selfDirectConfigured,
		RecentRouteOutcomes:    recentOutcomes,
		StatusAt:               now.Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) now() time.Time {
	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	return nowFn().UTC()
}

func summarizeTransportStatuses(records []agentruntime.MessageRecord) (direct int, deferred int, recovered int) {
	for _, record := range records {
		switch deriveTransportStatus(record.Direction, record.Status, record.SelectedRoute) {
		case TransportStatusDirect:
			direct++
		case TransportStatusDeferred:
			deferred++
		case TransportStatusRecovered:
			recovered++
		}
	}
	return direct, deferred, recovered
}

func deriveTransportStatus(direction string, status string, selected transport.RouteCandidate) string {
	normalizedStatus := agentruntime.NormalizeMessageStatus(status)
	if normalizedStatus == StatusFailed {
		return TransportStatusFailed
	}
	switch selected.Type {
	case transport.RouteTypeDirect:
		return TransportStatusDirect
	case transport.RouteTypeStoreForward:
		return TransportStatusDeferred
	case transport.RouteTypeRecovery:
		return TransportStatusRecovered
	}
	switch normalizedStatus {
	case StatusRecovered:
		return TransportStatusRecovered
	case StatusPending, StatusQueued, StatusRecovering:
		if direction == DirectionIncoming {
			return TransportStatusRecovered
		}
		return TransportStatusDeferred
	case StatusDelivered:
		if direction == DirectionIncoming {
			return TransportStatusDirect
		}
		return TransportStatusDirect
	default:
		return ""
	}
}

func (s *Service) storeForwardBackend() transportstoreforward.MailboxBackend {
	if s == nil || s.StoreForwardBackend == nil {
		return transportstoreforward.LegacyHTTPMailboxBackend{}
	}
	return s.StoreForwardBackend
}

func openStateDB(ctx context.Context, rawHome string, now time.Time) (*sql.DB, string, error) {
	home, err := layout.ResolveHome(rawHome)
	if err != nil {
		return nil, "", err
	}
	if _, err := layout.Ensure(home); err != nil {
		return nil, "", err
	}
	paths := layout.BuildPaths(home)
	if _, err := os.Stat(paths.DB); err != nil {
		if os.IsNotExist(err) {
			return nil, "", fmt.Errorf("state db not found at %q; run linkclaw init first", paths.DB)
		}
		return nil, "", fmt.Errorf("stat state db: %w", err)
	}
	db, err := sql.Open("sqlite", paths.DB)
	if err != nil {
		return nil, "", fmt.Errorf("open sqlite database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, "", fmt.Errorf("ping sqlite database: %w", err)
	}
	if _, err := migrate.Apply(ctx, db, now); err != nil {
		db.Close()
		return nil, "", fmt.Errorf("apply migrations: %w", err)
	}
	return db, home, nil
}

type connectReadinessTransport struct {
	name      string
	routeType transport.RouteType
}

type connectPeerTarget struct {
	contact   *contactRecord
	discovery *agentdiscovery.Record
}

type contactLookupNotFoundError struct {
	Reference string
}

func (e contactLookupNotFoundError) Error() string {
	return fmt.Sprintf("contact %q not found; import an identity card first", e.Reference)
}

func (t connectReadinessTransport) Name() string {
	if strings.TrimSpace(t.name) == "" {
		return "connect_ready"
	}
	return t.name
}

func (t connectReadinessTransport) Supports(route transport.RouteCandidate) bool {
	return route.Type == t.routeType
}

func (t connectReadinessTransport) Send(context.Context, transport.Envelope, transport.RouteCandidate) (transport.SendResult, error) {
	return transport.SendResult{}, fmt.Errorf("connect readiness transport does not support send")
}

func (t connectReadinessTransport) Sync(context.Context, transport.RouteCandidate) (transport.SyncResult, error) {
	return transport.SyncResult{}, fmt.Errorf("connect readiness transport does not support sync")
}

func (t connectReadinessTransport) Ack(context.Context, transport.RouteCandidate, string) error {
	return nil
}

func hasRuntimeStoreForwardTarget(contact contactRecord) bool {
	return len(storeForwardTargetsFromContact(contact)) > 0
}

func resolveRuntimeContactWithRelayView(
	ctx context.Context,
	db *sql.DB,
	selfProfile selfMessagingProfile,
	contact contactRecord,
	now time.Time,
) (contactRecord, error) {
	if strings.TrimSpace(contact.CanonicalID) == "" {
		return contact, nil
	}
	relayView, err := resolvePeerRelayPubKeyView(
		ctx,
		db,
		selfProfile.SelfID,
		contact.CanonicalID,
		contact.RelayURL,
		now,
	)
	if err != nil {
		return contactRecord{}, err
	}
	return applyRelayViewToContact(contact, relayView), nil
}

func resolveRuntimeDiscoveryWithRelayView(
	ctx context.Context,
	db *sql.DB,
	selfProfile selfMessagingProfile,
	record agentdiscovery.Record,
	now time.Time,
) (agentdiscovery.Record, error) {
	canonicalID := strings.TrimSpace(record.CanonicalID)
	if canonicalID == "" {
		return record, nil
	}
	relayView, err := resolvePeerRelayPubKeyView(
		ctx,
		db,
		selfProfile.SelfID,
		canonicalID,
		"",
		now,
	)
	if err != nil {
		return agentdiscovery.Record{}, err
	}
	return applyRelayViewToDiscoveryRecord(record, relayView), nil
}

func resolveConnectPeerTarget(ctx context.Context, db *sql.DB, ref string, now time.Time) (connectPeerTarget, error) {
	needle := strings.TrimSpace(ref)
	if needle == "" {
		return connectPeerTarget{}, fmt.Errorf("peer reference is required")
	}

	contact, err := resolveContactLookup(ctx, db, needle)
	if err == nil {
		return connectPeerTarget{contact: &contact}, nil
	}
	var notFound contactLookupNotFoundError
	if !errors.As(err, &notFound) {
		return connectPeerTarget{}, err
	}

	record, ok, err := resolveDiscoveryLookup(ctx, db, needle, now)
	if err != nil {
		return connectPeerTarget{}, err
	}
	if !ok {
		return connectPeerTarget{}, fmt.Errorf("peer %q not found in contacts or discovery; run message list-discovery first or import an identity card", needle)
	}
	return connectPeerTarget{discovery: &record}, nil
}

func resolveDiscoveryLookup(ctx context.Context, db *sql.DB, ref string, now time.Time) (agentdiscovery.Record, bool, error) {
	needle := strings.TrimSpace(ref)
	if needle == "" {
		return agentdiscovery.Record{}, false, fmt.Errorf("peer reference is required")
	}
	store := agentdiscovery.NewStoreWithDB(db, now)
	records, err := store.List(ctx)
	if err != nil {
		return agentdiscovery.Record{}, false, err
	}

	var peerMatches []agentdiscovery.Record
	for _, record := range records {
		if strings.TrimSpace(record.CanonicalID) == needle {
			return record, true, nil
		}
		if strings.TrimSpace(record.PeerID) == needle {
			peerMatches = append(peerMatches, record)
		}
	}
	switch len(peerMatches) {
	case 0:
		return agentdiscovery.Record{}, false, nil
	case 1:
		return peerMatches[0], true, nil
	default:
		canonicalIDs := make([]string, 0, len(peerMatches))
		for _, match := range peerMatches {
			canonicalIDs = append(canonicalIDs, strings.TrimSpace(match.CanonicalID))
		}
		sort.Strings(canonicalIDs)
		return agentdiscovery.Record{}, false, fmt.Errorf("peer reference %q is ambiguous in discovery records; canonical ids: %s", needle, strings.Join(canonicalIDs, ", "))
	}
}

func resolveContactLookup(ctx context.Context, db *sql.DB, ref string) (contactRecord, error) {
	needle := strings.TrimSpace(ref)
	if needle == "" {
		return contactRecord{}, fmt.Errorf("contact reference is required")
	}

	rows, err := db.QueryContext(
		ctx,
		`SELECT contact_id, canonical_id, display_name, recipient_id, signing_public_key, encryption_public_key, relay_url, direct_url, direct_token, status
		 FROM contacts
		 WHERE contact_id = ? OR canonical_id = ? OR display_name = ?
		 ORDER BY created_at ASC`,
		needle,
		needle,
		needle,
	)
	if err != nil {
		return contactRecord{}, fmt.Errorf("query contact reference: %w", err)
	}
	defer rows.Close()

	matches := []contactRecord{}
	for rows.Next() {
		var record contactRecord
		if err := rows.Scan(&record.ContactID, &record.CanonicalID, &record.DisplayName, &record.RecipientID, &record.SigningPublicKey, &record.EncryptionPublicKey, &record.RelayURL, &record.DirectURL, &record.DirectToken, &record.Status); err != nil {
			return contactRecord{}, fmt.Errorf("scan contact reference: %w", err)
		}
		matches = append(matches, record)
	}
	if err := rows.Err(); err != nil {
		return contactRecord{}, fmt.Errorf("iterate contact reference matches: %w", err)
	}
	if len(matches) == 0 {
		return contactRecord{}, contactLookupNotFoundError{Reference: needle}
	}
	if len(matches) > 1 {
		sort.Slice(matches, func(i, j int) bool { return matches[i].ContactID < matches[j].ContactID })
		ids := make([]string, 0, len(matches))
		for _, match := range matches {
			ids = append(ids, match.ContactID)
		}
		return contactRecord{}, fmt.Errorf("contact reference %q is ambiguous; use one of: %s", needle, strings.Join(ids, ", "))
	}
	return matches[0], nil
}

func resolveContact(ctx context.Context, tx *sql.Tx, ref string) (contactRecord, error) {
	needle := strings.TrimSpace(ref)
	if needle == "" {
		return contactRecord{}, fmt.Errorf("message recipient is required")
	}
	rows, err := tx.QueryContext(
		ctx,
		`SELECT contact_id, canonical_id, display_name, recipient_id, signing_public_key, encryption_public_key, relay_url, direct_url, direct_token, status
		 FROM contacts
		 WHERE contact_id = ? OR canonical_id = ? OR display_name = ?
		 ORDER BY created_at ASC`,
		needle,
		needle,
		needle,
	)
	if err != nil {
		return contactRecord{}, fmt.Errorf("query contact for message send: %w", err)
	}
	defer rows.Close()

	var matches []contactRecord
	for rows.Next() {
		var record contactRecord
		if err := rows.Scan(&record.ContactID, &record.CanonicalID, &record.DisplayName, &record.RecipientID, &record.SigningPublicKey, &record.EncryptionPublicKey, &record.RelayURL, &record.DirectURL, &record.DirectToken, &record.Status); err != nil {
			return contactRecord{}, fmt.Errorf("scan contact for message send: %w", err)
		}
		matches = append(matches, record)
	}
	if err := rows.Err(); err != nil {
		return contactRecord{}, fmt.Errorf("iterate contact matches: %w", err)
	}
	if len(matches) == 0 {
		return contactRecord{}, fmt.Errorf("contact %q not found; import an identity card first", needle)
	}
	if len(matches) > 1 {
		sort.Slice(matches, func(i, j int) bool { return matches[i].ContactID < matches[j].ContactID })
		var ids []string
		for _, match := range matches {
			ids = append(ids, match.ContactID)
		}
		return contactRecord{}, fmt.Errorf("contact reference %q is ambiguous; use one of: %s", needle, strings.Join(ids, ", "))
	}
	if strings.TrimSpace(matches[0].EncryptionPublicKey) == "" {
		return contactRecord{}, fmt.Errorf("contact %q is missing an encryption key; re-import the identity card", needle)
	}
	return matches[0], nil
}

func applyConnectPeerPromotion(
	ctx context.Context,
	db *sql.DB,
	target connectPeerTarget,
	connectResult *agentruntime.ConnectPeerResult,
	now time.Time,
) error {
	if connectResult == nil {
		return fmt.Errorf("connect result is nil")
	}
	canonicalID := strings.TrimSpace(connectResult.CanonicalID)
	if canonicalID == "" {
		return fmt.Errorf("connect result canonical_id is required")
	}

	stamp := now.UTC().Format(time.RFC3339Nano)
	displayName := canonicalID
	if target.contact != nil {
		if value := strings.TrimSpace(target.contact.DisplayName); value != "" {
			displayName = value
		}
	}

	verificationState := normalizeConnectVerificationState(connectResult.Trust.VerificationState)
	if verificationState == "" {
		verificationState = "discovered"
	}
	trustLevel := normalizeConnectTrustLevel(connectResult.Trust.TrustLevel)
	trustSource := strings.TrimSpace(connectResult.Trust.Source)
	if trustSource == "" {
		trustSource = "connect-peer"
	}
	connectReason := summarizeConnectPromotionReason(*connectResult)
	decidedAt := firstNonEmptyString(strings.TrimSpace(connectResult.Trust.AsOf), strings.TrimSpace(connectResult.ConnectedAt), stamp)

	peerID := strings.TrimSpace(connectResult.Presence.PeerID)
	if peerID == "" && target.contact != nil {
		peerID = strings.TrimSpace(target.contact.RecipientID)
	}
	if peerID == "" && target.discovery != nil {
		peerID = strings.TrimSpace(target.discovery.PeerID)
	}
	relayURL := firstStoreForwardHint(connectResult.Presence, connectResult.Routes)
	if relayURL == "" && target.contact != nil {
		relayURL = strings.TrimSpace(target.contact.RelayURL)
	}
	if relayURL == "" && target.discovery != nil {
		relayURL = firstStoreForwardRouteTarget(target.discovery.RouteCandidates)
		if relayURL == "" {
			relayURL = firstNonEmptyString(target.discovery.StoreForwardHints...)
		}
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin connect promotion transaction: %w", err)
	}
	defer tx.Rollback()

	contact, contactCreated, err := ensureConnectContactTx(
		ctx,
		tx,
		canonicalID,
		displayName,
		peerID,
		relayURL,
		verificationState,
		stamp,
	)
	if err != nil {
		return err
	}

	if _, err := ensureConnectTrustRecordTx(
		ctx,
		tx,
		contact.ContactID,
		trustLevel,
		verificationState,
		connectReason,
		stamp,
	); err != nil {
		return err
	}

	if err := upsertConnectRuntimeTrustTx(
		ctx,
		tx,
		canonicalID,
		contact.ContactID,
		trustLevel,
		connectResult.Trust.RiskFlags,
		verificationState,
		trustSource,
		connectReason,
		decidedAt,
		stamp,
	); err != nil {
		return err
	}

	discoverySource, err := upsertConnectRuntimeDiscoveryTx(ctx, tx, canonicalID, *connectResult, now.UTC(), stamp)
	if err != nil {
		return err
	}

	eventID, err := insertConnectEventTx(ctx, tx, contact.ContactID, canonicalID, *connectResult, discoverySource, stamp)
	if err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit connect promotion transaction: %w", err)
	}

	trustSvc := trust.NewServiceWithDB(db, now)
	if summary, ok, err := trustSvc.Summary(ctx, canonicalID); err != nil {
		return err
	} else if ok {
		connectResult.Trust = summary
	}

	connectResult.Promotion = agentruntime.ConnectPeerPromotion{
		ContactID:              contact.ContactID,
		ContactStatus:          contact.Status,
		ContactCreated:         contactCreated,
		TrustLinked:            true,
		TrustLevel:             firstNonEmptyString(connectResult.Trust.TrustLevel, trustLevel),
		TrustVerificationState: firstNonEmptyString(connectResult.Trust.VerificationState, verificationState),
		TrustSource:            firstNonEmptyString(connectResult.Trust.Source, trustSource),
		DiscoveryUpdated:       true,
		DiscoverySource:        discoverySource,
		NoteWritten:            false,
		PinWritten:             false,
		EventID:                eventID,
	}
	return nil
}

func ensureConnectContactTx(
	ctx context.Context,
	tx *sql.Tx,
	canonicalID string,
	displayName string,
	peerID string,
	relayURL string,
	status string,
	stamp string,
) (contactRecord, bool, error) {
	canonicalID = strings.TrimSpace(canonicalID)
	if canonicalID == "" {
		return contactRecord{}, false, fmt.Errorf("connect promotion requires canonical_id")
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		displayName = canonicalID
	}
	status = normalizeConnectVerificationState(status)
	if status == "" {
		status = "discovered"
	}
	peerID = strings.TrimSpace(peerID)
	relayURL = strings.TrimSpace(relayURL)

	var contact contactRecord
	err := tx.QueryRowContext(
		ctx,
		`SELECT contact_id, canonical_id, display_name, recipient_id, signing_public_key, encryption_public_key, relay_url, direct_url, direct_token, status
		 FROM contacts
		 WHERE canonical_id = ?
		 LIMIT 1`,
		canonicalID,
	).Scan(
		&contact.ContactID,
		&contact.CanonicalID,
		&contact.DisplayName,
		&contact.RecipientID,
		&contact.SigningPublicKey,
		&contact.EncryptionPublicKey,
		&contact.RelayURL,
		&contact.DirectURL,
		&contact.DirectToken,
		&contact.Status,
	)
	switch {
	case err == nil:
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE contacts
			 SET display_name = CASE WHEN display_name = '' AND ? <> '' THEN ? ELSE display_name END,
			     recipient_id = CASE WHEN recipient_id = '' AND ? <> '' THEN ? ELSE recipient_id END,
			     relay_url = CASE WHEN relay_url = '' AND ? <> '' THEN ? ELSE relay_url END,
			     status = CASE WHEN status = '' AND ? <> '' THEN ? ELSE status END,
			     last_seen_at = ?
			 WHERE contact_id = ?`,
			displayName, displayName,
			peerID, peerID,
			relayURL, relayURL,
			status, status,
			stamp,
			contact.ContactID,
		); err != nil {
			return contactRecord{}, false, fmt.Errorf("update connect promotion contact: %w", err)
		}

		updated := contact
		if err := tx.QueryRowContext(
			ctx,
			`SELECT contact_id, canonical_id, display_name, recipient_id, signing_public_key, encryption_public_key, relay_url, direct_url, direct_token, status
			 FROM contacts
			 WHERE contact_id = ?
			 LIMIT 1`,
			contact.ContactID,
		).Scan(
			&updated.ContactID,
			&updated.CanonicalID,
			&updated.DisplayName,
			&updated.RecipientID,
			&updated.SigningPublicKey,
			&updated.EncryptionPublicKey,
			&updated.RelayURL,
			&updated.DirectURL,
			&updated.DirectToken,
			&updated.Status,
		); err != nil {
			return contactRecord{}, false, fmt.Errorf("reload connect promotion contact: %w", err)
		}
		if strings.TrimSpace(updated.Status) == "" {
			updated.Status = status
		}
		return updated, false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return contactRecord{}, false, fmt.Errorf("query connect promotion contact: %w", err)
	}

	contactID, err := ids.New("contact")
	if err != nil {
		return contactRecord{}, false, err
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO contacts (
			contact_id, canonical_id, display_name, recipient_id, relay_url, status, last_seen_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		contactID,
		canonicalID,
		displayName,
		peerID,
		relayURL,
		status,
		stamp,
		stamp,
	); err != nil {
		return contactRecord{}, false, fmt.Errorf("insert connect promotion contact: %w", err)
	}

	return contactRecord{
		ContactID:   contactID,
		CanonicalID: canonicalID,
		DisplayName: displayName,
		RecipientID: peerID,
		RelayURL:    relayURL,
		Status:      status,
	}, true, nil
}

func ensureConnectTrustRecordTx(
	ctx context.Context,
	tx *sql.Tx,
	contactID string,
	trustLevel string,
	verificationState string,
	reason string,
	stamp string,
) (bool, error) {
	contactID = strings.TrimSpace(contactID)
	if contactID == "" {
		return false, fmt.Errorf("connect promotion trust record requires contact_id")
	}
	trustLevel = normalizeConnectTrustLevel(trustLevel)
	verificationState = normalizeConnectVerificationState(verificationState)
	if verificationState == "" {
		verificationState = "discovered"
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "connect-peer promotion"
	}

	var trustID string
	var existingLevel string
	var existingVerification string
	var existingReason string
	err := tx.QueryRowContext(
		ctx,
		`SELECT trust_id, trust_level, verification_state, decision_reason
		 FROM trust_records
		 WHERE contact_id = ?
		 LIMIT 1`,
		contactID,
	).Scan(&trustID, &existingLevel, &existingVerification, &existingReason)
	switch {
	case err == nil:
		nextLevel := mergeConnectTrustLevel(existingLevel, trustLevel)
		nextVerification := mergeConnectVerificationState(existingVerification, verificationState)
		nextReason := firstNonEmptyString(strings.TrimSpace(existingReason), reason)
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE trust_records
			 SET trust_level = ?, verification_state = ?, decision_reason = ?, updated_at = ?
			 WHERE trust_id = ?`,
			nextLevel,
			nextVerification,
			nextReason,
			stamp,
			trustID,
		); err != nil {
			return false, fmt.Errorf("update connect promotion trust record: %w", err)
		}
		return false, nil
	case !errors.Is(err, sql.ErrNoRows):
		return false, fmt.Errorf("query connect promotion trust record: %w", err)
	}

	trustID, err = ids.New("trust")
	if err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO trust_records (
			trust_id, contact_id, trust_level, risk_flags, verification_state, decision_reason, updated_at, created_at
		) VALUES (?, ?, ?, '[]', ?, ?, ?, ?)`,
		trustID,
		contactID,
		trustLevel,
		verificationState,
		reason,
		stamp,
		stamp,
	); err != nil {
		return false, fmt.Errorf("insert connect promotion trust record: %w", err)
	}
	return true, nil
}

func upsertConnectRuntimeTrustTx(
	ctx context.Context,
	tx *sql.Tx,
	canonicalID string,
	contactID string,
	trustLevel string,
	riskFlags []string,
	verificationState string,
	source string,
	decisionReason string,
	decidedAt string,
	stamp string,
) error {
	canonicalID = strings.TrimSpace(canonicalID)
	if canonicalID == "" {
		return fmt.Errorf("runtime trust upsert requires canonical_id")
	}
	contactID = strings.TrimSpace(contactID)
	trustLevel = normalizeConnectTrustLevel(trustLevel)
	verificationState = normalizeConnectVerificationState(verificationState)
	if verificationState == "" {
		verificationState = "discovered"
	}
	source = strings.TrimSpace(source)
	if source == "" {
		source = "connect-peer"
	}
	decisionReason = strings.TrimSpace(decisionReason)
	if decisionReason == "" {
		decisionReason = "connect-peer promotion"
	}
	decidedAt = firstNonEmptyString(strings.TrimSpace(decidedAt), stamp)
	normalizedRiskFlags := normalizeConnectStringList(riskFlags)

	var existingTrustLevel string
	var existingRiskFlagsJSON string
	var existingVerification string
	var existingReason string
	var existingSource string
	var existingDecidedAt string
	err := tx.QueryRowContext(
		ctx,
		`SELECT trust_level, risk_flags_json, verification_state, decision_reason, source, decided_at
		 FROM runtime_trust_records
		 WHERE canonical_id = ?
		 LIMIT 1`,
		canonicalID,
	).Scan(
		&existingTrustLevel,
		&existingRiskFlagsJSON,
		&existingVerification,
		&existingReason,
		&existingSource,
		&existingDecidedAt,
	)
	switch {
	case err == nil:
		trustLevel = mergeConnectTrustLevel(existingTrustLevel, trustLevel)
		if len(normalizedRiskFlags) == 0 {
			normalizedRiskFlags = decodeConnectStringList(existingRiskFlagsJSON)
		}
		verificationState = mergeConnectVerificationState(existingVerification, verificationState)
		decisionReason = firstNonEmptyString(strings.TrimSpace(existingReason), decisionReason)
		source = firstNonEmptyString(strings.TrimSpace(existingSource), source)
		decidedAt = firstNonEmptyString(strings.TrimSpace(existingDecidedAt), decidedAt, stamp)
	case !errors.Is(err, sql.ErrNoRows):
		return fmt.Errorf("query runtime trust record for connect promotion: %w", err)
	}

	riskFlagsJSON, err := json.Marshal(normalizedRiskFlags)
	if err != nil {
		return fmt.Errorf("marshal connect promotion trust risk flags: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO runtime_trust_records (
			canonical_id, contact_id, trust_level, risk_flags_json, verification_state,
			decision_reason, source, decided_at, updated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(canonical_id) DO UPDATE SET
			contact_id = excluded.contact_id,
			trust_level = excluded.trust_level,
			risk_flags_json = excluded.risk_flags_json,
			verification_state = excluded.verification_state,
			decision_reason = excluded.decision_reason,
			source = excluded.source,
			decided_at = excluded.decided_at,
			updated_at = excluded.updated_at`,
		canonicalID,
		contactID,
		trustLevel,
		string(riskFlagsJSON),
		verificationState,
		decisionReason,
		source,
		decidedAt,
		stamp,
		stamp,
	); err != nil {
		return fmt.Errorf("upsert connect promotion runtime trust: %w", err)
	}
	return nil
}

func upsertConnectRuntimeDiscoveryTx(
	ctx context.Context,
	tx *sql.Tx,
	canonicalID string,
	connectResult agentruntime.ConnectPeerResult,
	now time.Time,
	stamp string,
) (string, error) {
	canonicalID = strings.TrimSpace(canonicalID)
	if canonicalID == "" {
		return "", fmt.Errorf("runtime discovery upsert requires canonical_id")
	}

	routes := append([]transport.RouteCandidate(nil), connectResult.Presence.RouteCandidates...)
	if len(routes) == 0 {
		routes = append(routes, connectResult.Routes...)
	}
	routes = appendHintsToRoutes(routes, connectResult.Presence.DirectHints, connectResult.Presence.StoreForwardHints)

	transportCaps := normalizeConnectStringList(connectResult.Presence.TransportCapabilities)
	for _, route := range routes {
		transportCaps = appendConnectString(transportCaps, string(route.Type))
	}
	if len(connectResult.Presence.DirectHints) > 0 {
		transportCaps = appendConnectString(transportCaps, string(transport.RouteTypeDirect))
	}
	if len(connectResult.Presence.StoreForwardHints) > 0 {
		transportCaps = appendConnectString(transportCaps, string(transport.RouteTypeStoreForward))
	}

	directHints := normalizeConnectStringList(connectResult.Presence.DirectHints)
	storeForwardHints := normalizeConnectStringList(connectResult.Presence.StoreForwardHints)
	if len(storeForwardHints) == 0 {
		if fallback := firstStoreForwardRouteTarget(routes); fallback != "" {
			storeForwardHints = append(storeForwardHints, fallback)
		}
	}

	peerID := strings.TrimSpace(connectResult.Presence.PeerID)
	source := agentdiscovery.NormalizeSource(connectResult.Presence.Source)
	if source == agentdiscovery.SourceUnknown && strings.TrimSpace(connectResult.Presence.Source) == "" {
		source = agentdiscovery.SourceManual
	}
	resolvedAt := formatOptionalTime(connectResult.Presence.ResolvedAt)
	if resolvedAt == "" {
		resolvedAt = stamp
	}
	freshUntil := formatOptionalTime(connectResult.Presence.FreshUntil)
	if freshUntil == "" {
		freshUntil = now.Add(5 * time.Minute).UTC().Format(time.RFC3339Nano)
	}
	announcedAt := formatOptionalTime(connectResult.Presence.AnnouncedAt)
	reachable := connectResult.Presence.Reachable || connectResult.Connected || len(routes) > 0

	routeCandidatesJSON, err := json.Marshal(routes)
	if err != nil {
		return "", fmt.Errorf("marshal connect promotion discovery routes: %w", err)
	}
	transportCapsJSON, err := json.Marshal(transportCaps)
	if err != nil {
		return "", fmt.Errorf("marshal connect promotion discovery capabilities: %w", err)
	}
	directHintsJSON, err := json.Marshal(directHints)
	if err != nil {
		return "", fmt.Errorf("marshal connect promotion discovery direct hints: %w", err)
	}
	storeForwardHintsJSON, err := json.Marshal(storeForwardHints)
	if err != nil {
		return "", fmt.Errorf("marshal connect promotion discovery store-forward hints: %w", err)
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO runtime_discovery_records (
			canonical_id, peer_id, route_candidates_json, transport_capabilities_json, direct_hints_json,
			store_forward_hints_json, signed_peer_record, source, reachable, resolved_at, fresh_until,
			announced_at, updated_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(canonical_id) DO UPDATE SET
			peer_id = excluded.peer_id,
			route_candidates_json = excluded.route_candidates_json,
			transport_capabilities_json = excluded.transport_capabilities_json,
			direct_hints_json = excluded.direct_hints_json,
			store_forward_hints_json = excluded.store_forward_hints_json,
			signed_peer_record = excluded.signed_peer_record,
			source = excluded.source,
			reachable = excluded.reachable,
			resolved_at = excluded.resolved_at,
			fresh_until = excluded.fresh_until,
			announced_at = excluded.announced_at,
			updated_at = excluded.updated_at`,
		canonicalID,
		peerID,
		string(routeCandidatesJSON),
		string(transportCapsJSON),
		string(directHintsJSON),
		string(storeForwardHintsJSON),
		strings.TrimSpace(connectResult.Presence.SignedPeerRecord),
		source,
		boolToInt(reachable),
		resolvedAt,
		freshUntil,
		announcedAt,
		stamp,
		stamp,
	); err != nil {
		return "", fmt.Errorf("upsert connect promotion runtime discovery: %w", err)
	}
	return source, nil
}

func insertConnectEventTx(
	ctx context.Context,
	tx *sql.Tx,
	contactID string,
	canonicalID string,
	connectResult agentruntime.ConnectPeerResult,
	discoverySource string,
	stamp string,
) (string, error) {
	eventID, err := ids.New("event")
	if err != nil {
		return "", err
	}
	summary := fmt.Sprintf(
		"connect canonical_id=%s connected=%t transport=%s source=%s",
		strings.TrimSpace(canonicalID),
		connectResult.Connected,
		firstNonEmptyString(strings.TrimSpace(connectResult.Transport), "none"),
		firstNonEmptyString(strings.TrimSpace(discoverySource), "unknown"),
	)
	if reason := strings.TrimSpace(connectResult.Reason); reason != "" {
		summary += " reason=" + reason
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO interaction_events (
			event_id, contact_id, channel, event_type, summary, event_at, created_at
		) VALUES (?, ?, 'linkclaw', 'connect', ?, ?, ?)`,
		eventID,
		contactID,
		summary,
		stamp,
		stamp,
	); err != nil {
		return "", fmt.Errorf("insert connect interaction event: %w", err)
	}
	return eventID, nil
}

func summarizeConnectPromotionReason(result agentruntime.ConnectPeerResult) string {
	reason := fmt.Sprintf("connect-peer promotion connected=%t", result.Connected)
	if transportName := strings.TrimSpace(result.Transport); transportName != "" {
		reason += " transport=" + transportName
	}
	if detail := strings.TrimSpace(result.Reason); detail != "" {
		reason += " reason=" + detail
	}
	return reason
}

func firstStoreForwardHint(presence agentdiscovery.PeerPresenceView, routes []transport.RouteCandidate) string {
	if hint := firstNonEmptyString(presence.StoreForwardHints...); hint != "" {
		return hint
	}
	if hint := firstStoreForwardRouteTarget(presence.RouteCandidates); hint != "" {
		return hint
	}
	return firstStoreForwardRouteTarget(routes)
}

func firstStoreForwardRouteTarget(routes []transport.RouteCandidate) string {
	for _, route := range routes {
		if route.Type != transport.RouteTypeStoreForward {
			continue
		}
		if target := strings.TrimSpace(route.Target); target != "" {
			return target
		}
		if label := strings.TrimSpace(route.Label); label != "" {
			return label
		}
	}
	return ""
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizeConnectTrustLevel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "seen", "verified", "trusted", "pinned":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func mergeConnectTrustLevel(existing string, candidate string) string {
	existing = normalizeConnectTrustLevel(existing)
	candidate = normalizeConnectTrustLevel(candidate)
	if candidate == "unknown" && existing != "" {
		return existing
	}
	if candidate == "" {
		return firstNonEmptyString(existing, "unknown")
	}
	return candidate
}

func normalizeConnectVerificationState(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "discovered", "resolved", "consistent", "mismatch":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func mergeConnectVerificationState(existing string, candidate string) string {
	existing = normalizeConnectVerificationState(existing)
	candidate = normalizeConnectVerificationState(candidate)
	if candidate == "" {
		return existing
	}
	if existing == "" {
		return candidate
	}
	if connectVerificationStatePriority(candidate) >= connectVerificationStatePriority(existing) {
		return candidate
	}
	return existing
}

func connectVerificationStatePriority(value string) int {
	switch normalizeConnectVerificationState(value) {
	case "mismatch":
		return 4
	case "consistent":
		return 3
	case "resolved":
		return 2
	case "discovered":
		return 1
	default:
		return 0
	}
}

func normalizeConnectStringList(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	sort.Strings(result)
	return result
}

func appendConnectString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if strings.TrimSpace(existing) == value {
			return values
		}
	}
	return append(values, value)
}

func decodeConnectStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return []string{}
	}
	return normalizeConnectStringList(values)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func ensureConversation(ctx context.Context, tx *sql.Tx, contact contactRecord, now time.Time) (Conversation, error) {
	var conversation Conversation
	err := tx.QueryRowContext(
		ctx,
		`SELECT conversation_id, last_message_at, last_message_preview, unread_count
		 FROM conversations
		 WHERE contact_id = ?
		 LIMIT 1`,
		contact.ContactID,
	).Scan(&conversation.ConversationID, &conversation.LastMessageAt, &conversation.LastMessagePreview, &conversation.UnreadCount)
	switch {
	case err == nil:
		conversation.ContactID = contact.ContactID
		conversation.ContactDisplayName = contact.DisplayName
		conversation.ContactCanonicalID = contact.CanonicalID
		conversation.ContactStatus = contact.Status
		return conversation, nil
	case err != sql.ErrNoRows:
		return Conversation{}, fmt.Errorf("query conversation: %w", err)
	}
	conversationID, err := ids.New("conv")
	if err != nil {
		return Conversation{}, err
	}
	stamp := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO conversations (conversation_id, contact_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?)`,
		conversationID,
		contact.ContactID,
		stamp,
		stamp,
	); err != nil {
		return Conversation{}, fmt.Errorf("insert conversation: %w", err)
	}
	return Conversation{
		ConversationID:     conversationID,
		ContactID:          contact.ContactID,
		ContactDisplayName: contact.DisplayName,
		ContactCanonicalID: contact.CanonicalID,
		ContactStatus:      contact.Status,
	}, nil
}

func insertOutgoingMessage(ctx context.Context, tx *sql.Tx, conversation Conversation, contact contactRecord, body string, now time.Time) (MessageRecord, error) {
	messageID, err := ids.New("msg")
	if err != nil {
		return MessageRecord{}, err
	}
	preview := makePreview(body)
	stamp := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO messages (
			message_id, conversation_id, direction, recipient_contact_id,
			sender_canonical_id, recipient_route_id, plaintext_body, plaintext_preview,
			status, created_at
		) VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?)`,
		messageID,
		conversation.ConversationID,
		DirectionOutgoing,
		contact.ContactID,
		contact.RecipientID,
		body,
		preview,
		StatusPending,
		stamp,
	); err != nil {
		return MessageRecord{}, fmt.Errorf("insert outgoing message: %w", err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE conversations
		 SET last_message_at = ?, last_message_preview = ?, updated_at = ?
		 WHERE conversation_id = ?`,
		stamp,
		preview,
		stamp,
		conversation.ConversationID,
	); err != nil {
		return MessageRecord{}, fmt.Errorf("update conversation last message: %w", err)
	}
	return MessageRecord{
		MessageID:          messageID,
		ConversationID:     conversation.ConversationID,
		Direction:          DirectionOutgoing,
		RecipientContactID: contact.ContactID,
		RecipientRouteID:   contact.RecipientID,
		Body:               body,
		Preview:            preview,
		Status:             StatusPending,
		CreatedAt:          stamp,
	}, nil
}

func (s *Service) deliverOutgoing(ctx context.Context, home string, record MessageRecord, contact contactRecord, now time.Time) (MessageRecord, error) {
	selfProfile, err := loadSelfMessagingProfileByHome(ctx, home, now)
	if err != nil {
		return record, err
	}
	relayURL := firstNonEmptyString(storeForwardTargetsFromContact(contact)...)
	if relayURL == "" {
		return record, fmt.Errorf("store-forward relay route is not available for contact %q", strings.TrimSpace(contact.CanonicalID))
	}
	encrypted, err := messagecrypto.EncryptForRecipient(contact.EncryptionPublicKey, []byte(record.Body))
	if err != nil {
		return record, err
	}
	payload := signedMessagePayload{
		MessageID:          record.MessageID,
		SenderID:           selfProfile.CanonicalID,
		SenderSigningKey:   selfProfile.SigningPublicKey,
		RecipientID:        contact.RecipientID,
		EphemeralPublicKey: encrypted.EphemeralPublicKey,
		Nonce:              encrypted.Nonce,
		Ciphertext:         encrypted.Ciphertext,
		SentAt:             record.CreatedAt,
	}
	payloadBytes, err := marshalSignedPayload(payload)
	if err != nil {
		return record, err
	}
	signature, err := messagecrypto.SignPayload(selfProfile.SigningPrivateKeyPath, payloadBytes)
	if err != nil {
		return record, err
	}
	response, err := s.storeForwardBackend().Send(ctx, relayURL, transportstoreforward.MailboxSendRequest{
		MessageID:          record.MessageID,
		SenderID:           selfProfile.CanonicalID,
		SenderSigningKey:   selfProfile.SigningPublicKey,
		RecipientID:        contact.RecipientID,
		EphemeralPublicKey: encrypted.EphemeralPublicKey,
		Nonce:              encrypted.Nonce,
		Ciphertext:         encrypted.Ciphertext,
		Signature:          signature,
		SentAt:             record.CreatedAt,
	})
	if err != nil {
		_ = updateMessageDeliveryState(ctx, home, now, record.MessageID, StatusFailed, "", relayURL, "failed", err.Error(), encrypted.Ciphertext, signature)
		return record, err
	}
	if err := updateMessageDeliveryState(ctx, home, now, record.MessageID, StatusQueued, response.RemoteMessageID, relayURL, "queued", "", encrypted.Ciphertext, signature); err != nil {
		return record, err
	}
	record.Status = StatusQueued
	return record, nil
}

func loadSelfMessagingProfile(ctx context.Context, db *sql.DB, home string) (selfMessagingProfile, error) {
	var profile selfMessagingProfile
	err := db.QueryRowContext(
		ctx,
		`SELECT s.self_id, s.canonical_id, p.recipient_id, p.relay_url, p.direct_url, p.direct_token,
		        k.public_key, k.private_key_ref, p.encryption_private_key_ref
		 FROM self_identities s
		 JOIN self_messaging_profiles p ON p.self_id = s.self_id
		 JOIN keys k ON k.owner_type = 'self' AND k.owner_id = s.self_id AND k.status = 'active'
		 ORDER BY s.created_at ASC
		 LIMIT 1`,
	).Scan(
		&profile.SelfID,
		&profile.CanonicalID,
		&profile.RecipientID,
		&profile.RelayURL,
		&profile.DirectURL,
		&profile.DirectToken,
		&profile.SigningPublicKey,
		&profile.SigningPrivateKeyPath,
		&profile.EncryptionPrivateKeyPath,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return selfMessagingProfile{}, fmt.Errorf("self messaging profile not found; export a card first")
		}
		return selfMessagingProfile{}, fmt.Errorf("query self messaging profile: %w", err)
	}
	paths := layout.BuildPaths(home)
	if !filepath.IsAbs(profile.SigningPrivateKeyPath) {
		profile.SigningPrivateKeyPath = filepath.Join(paths.KeysDir, profile.SigningPrivateKeyPath)
	}
	if !filepath.IsAbs(profile.EncryptionPrivateKeyPath) {
		profile.EncryptionPrivateKeyPath = filepath.Join(paths.KeysDir, profile.EncryptionPrivateKeyPath)
	}
	return profile, nil
}

func loadSelfMessagingProfileByHome(ctx context.Context, home string, now time.Time) (selfMessagingProfile, error) {
	db, _, err := openStateDB(ctx, home, now)
	if err != nil {
		return selfMessagingProfile{}, err
	}
	defer db.Close()
	return loadSelfMessagingProfile(ctx, db, home)
}

func updateMessageDeliveryState(ctx context.Context, home string, now time.Time, messageID, status, remoteMessageID, relayURL, result, errorMessage, ciphertext, signature string) error {
	db, _, err := openStateDB(ctx, home, now)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin message delivery update transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE messages
		 SET status = ?, remote_message_id = ?, ciphertext = ?, signature = ?, synced_at = ?
		 WHERE message_id = ?`,
		status,
		remoteMessageID,
		ciphertext,
		signature,
		now.Format(time.RFC3339Nano),
		messageID,
	); err != nil {
		return fmt.Errorf("update outgoing message status: %w", err)
	}
	attemptID, err := ids.New("attempt")
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO message_delivery_attempts (attempt_id, message_id, relay_url, attempted_at, result, error)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		attemptID,
		messageID,
		relayURL,
		now.Format(time.RFC3339Nano),
		result,
		errorMessage,
	); err != nil {
		return fmt.Errorf("insert message delivery attempt: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit message delivery update: %w", err)
	}
	return nil
}

func loadSyncCursor(ctx context.Context, db *sql.DB, profileID, relayURL string) (string, error) {
	var cursor string
	err := db.QueryRowContext(
		ctx,
		`SELECT last_cursor
		 FROM message_sync_cursors
		 WHERE profile_id = ? AND relay_url = ?
		 LIMIT 1`,
		profileID,
		relayURL,
	).Scan(&cursor)
	switch {
	case err == nil:
		return cursor, nil
	case err == sql.ErrNoRows:
		return "", nil
	default:
		return "", fmt.Errorf("query sync cursor: %w", err)
	}
}

func saveSyncCursor(ctx context.Context, tx *sql.Tx, profileID, relayURL, cursor string, now time.Time) error {
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO message_sync_cursors (profile_id, relay_url, last_cursor, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(profile_id) DO UPDATE SET
		   relay_url = excluded.relay_url,
		   last_cursor = excluded.last_cursor,
		   updated_at = excluded.updated_at`,
		profileID,
		relayURL,
		cursor,
		now.Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("upsert sync cursor: %w", err)
	}
	return nil
}

func ensureIncomingContact(ctx context.Context, tx *sql.Tx, msg transportstoreforward.MailboxPullMessage, now time.Time) (contactRecord, error) {
	var contact contactRecord
	err := tx.QueryRowContext(
		ctx,
		`SELECT contact_id, canonical_id, display_name, recipient_id, signing_public_key, encryption_public_key, relay_url, direct_url, direct_token, status
		 FROM contacts
		 WHERE canonical_id = ?
		 LIMIT 1`,
		msg.SenderID,
	).Scan(&contact.ContactID, &contact.CanonicalID, &contact.DisplayName, &contact.RecipientID, &contact.SigningPublicKey, &contact.EncryptionPublicKey, &contact.RelayURL, &contact.DirectURL, &contact.DirectToken, &contact.Status)
	switch {
	case err == nil:
		if contact.SigningPublicKey != "" && contact.SigningPublicKey != msg.SenderSigningKey {
			return contactRecord{}, fmt.Errorf("incoming sender signing key mismatch for %s", msg.SenderID)
		}
		return contact, nil
	case err != sql.ErrNoRows:
		return contactRecord{}, fmt.Errorf("query incoming sender contact: %w", err)
	}
	contactID, err := ids.New("contact")
	if err != nil {
		return contactRecord{}, err
	}
	stamp := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO contacts (contact_id, canonical_id, display_name, status, created_at, signing_public_key)
		 VALUES (?, ?, ?, 'discovered', ?, ?)`,
		contactID,
		msg.SenderID,
		msg.SenderID,
		stamp,
		msg.SenderSigningKey,
	); err != nil {
		return contactRecord{}, fmt.Errorf("insert discovered incoming sender: %w", err)
	}
	return contactRecord{
		ContactID:        contactID,
		CanonicalID:      msg.SenderID,
		DisplayName:      msg.SenderID,
		SigningPublicKey: msg.SenderSigningKey,
		Status:           "discovered",
	}, nil
}

func ensureDirectIncomingContact(ctx context.Context, tx *sql.Tx, env transport.Envelope, now time.Time) (contactRecord, error) {
	var contact contactRecord
	err := tx.QueryRowContext(
		ctx,
		`SELECT contact_id, canonical_id, display_name, recipient_id, signing_public_key, encryption_public_key, relay_url, direct_url, direct_token, status
		 FROM contacts
		 WHERE canonical_id = ?
		 LIMIT 1`,
		env.SenderID,
	).Scan(&contact.ContactID, &contact.CanonicalID, &contact.DisplayName, &contact.RecipientID, &contact.SigningPublicKey, &contact.EncryptionPublicKey, &contact.RelayURL, &contact.DirectURL, &contact.DirectToken, &contact.Status)
	switch {
	case err == nil:
		return contact, nil
	case err != sql.ErrNoRows:
		return contactRecord{}, fmt.Errorf("query incoming direct sender contact: %w", err)
	}
	contactID, err := ids.New("contact")
	if err != nil {
		return contactRecord{}, err
	}
	stamp := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO contacts (contact_id, canonical_id, display_name, status, created_at)
		 VALUES (?, ?, ?, 'discovered', ?)`,
		contactID,
		env.SenderID,
		env.SenderID,
		stamp,
	); err != nil {
		return contactRecord{}, fmt.Errorf("insert discovered direct sender: %w", err)
	}
	return contactRecord{
		ContactID:   contactID,
		CanonicalID: env.SenderID,
		DisplayName: env.SenderID,
		Status:      "discovered",
	}, nil
}

func decryptIncomingMessage(selfProfile selfMessagingProfile, msg transportstoreforward.MailboxPullMessage) (string, string, error) {
	payload := signedMessagePayload{
		MessageID:          msg.MessageID,
		SenderID:           msg.SenderID,
		SenderSigningKey:   msg.SenderSigningKey,
		RecipientID:        msg.RecipientID,
		EphemeralPublicKey: msg.EphemeralPublicKey,
		Nonce:              msg.Nonce,
		Ciphertext:         msg.Ciphertext,
		SentAt:             msg.SentAt,
	}
	payloadBytes, err := marshalSignedPayload(payload)
	if err != nil {
		return "", "", err
	}
	if err := messagecrypto.VerifyPayload(msg.SenderSigningKey, payloadBytes, msg.Signature); err != nil {
		return "", "", fmt.Errorf("verify incoming message signature: %w", err)
	}
	plaintext, err := messagecrypto.DecryptWithPrivateKeyFile(selfProfile.EncryptionPrivateKeyPath, msg.EphemeralPublicKey, msg.Nonce, msg.Ciphertext)
	if err != nil {
		return "", "", err
	}
	body := string(plaintext)
	return body, makePreview(body), nil
}

func insertIncomingMessage(ctx context.Context, tx *sql.Tx, conversation Conversation, contact contactRecord, msg transportstoreforward.MailboxPullMessage, body, preview string, now time.Time) (bool, error) {
	createdAt := strings.TrimSpace(msg.SentAt)
	if createdAt == "" {
		createdAt = now.Format(time.RFC3339Nano)
	}
	insertResult, err := tx.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO messages (
			message_id, conversation_id, direction, sender_contact_id,
			sender_canonical_id, recipient_route_id, plaintext_body, plaintext_preview,
			ciphertext, signature, status, remote_message_id, created_at, synced_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		msg.MessageID,
		conversation.ConversationID,
		DirectionIncoming,
		contact.ContactID,
		msg.SenderID,
		msg.RecipientID,
		body,
		preview,
		msg.Ciphertext,
		msg.Signature,
		StatusRecovered,
		msg.RelayMessageID,
		createdAt,
		now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return false, fmt.Errorf("insert incoming message: %w", err)
	}
	rowsAffected, err := insertResult.RowsAffected()
	if err == nil && rowsAffected == 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE conversations
		 SET last_message_at = ?, last_message_preview = ?, unread_count = unread_count + 1, updated_at = ?
		 WHERE conversation_id = ?`,
		createdAt,
		preview,
		now.Format(time.RFC3339Nano),
		conversation.ConversationID,
	); err != nil {
		return false, fmt.Errorf("update conversation for incoming message: %w", err)
	}
	return true, nil
}

func insertDirectIncomingMessage(ctx context.Context, tx *sql.Tx, conversation Conversation, contact contactRecord, selfProfile selfMessagingProfile, env transport.Envelope, now time.Time) (bool, error) {
	createdAt := now.Format(time.RFC3339Nano)
	preview := makePreview(env.Plaintext)
	insertResult, err := tx.ExecContext(
		ctx,
		`INSERT OR IGNORE INTO messages (
			message_id, conversation_id, direction, sender_contact_id,
			sender_canonical_id, recipient_route_id, plaintext_body, plaintext_preview,
			status, created_at, synced_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		env.MessageID,
		conversation.ConversationID,
		DirectionIncoming,
		contact.ContactID,
		env.SenderID,
		selfProfile.RecipientID,
		env.Plaintext,
		preview,
		StatusDelivered,
		createdAt,
		createdAt,
	)
	if err != nil {
		return false, fmt.Errorf("insert direct incoming message: %w", err)
	}
	rowsAffected, err := insertResult.RowsAffected()
	if err == nil && rowsAffected == 0 {
		return false, nil
	}
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE conversations
		 SET last_message_at = ?, last_message_preview = ?, unread_count = unread_count + 1, updated_at = ?
		 WHERE conversation_id = ?`,
		createdAt,
		preview,
		createdAt,
		conversation.ConversationID,
	); err != nil {
		return false, fmt.Errorf("update conversation for direct incoming message: %w", err)
	}
	return true, nil
}

func markConversationRead(ctx context.Context, tx *sql.Tx, conversationID string, now time.Time) error {
	if _, err := tx.ExecContext(
		ctx,
		`UPDATE conversations
		 SET unread_count = 0, updated_at = ?
		 WHERE conversation_id = ?`,
		now.Format(time.RFC3339Nano),
		conversationID,
	); err != nil {
		return fmt.Errorf("mark conversation read: %w", err)
	}
	return nil
}

func marshalSignedPayload(payload signedMessagePayload) ([]byte, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal signed message payload: %w", err)
	}
	return encoded, nil
}

func makePreview(body string) string {
	preview := body
	if len([]rune(preview)) > 80 {
		preview = string([]rune(preview)[:80])
	}
	return preview
}

func normalizeThreadLimit(limit int) int {
	if limit <= 0 {
		return 20
	}
	if limit > 100 {
		return 100
	}
	return limit
}
