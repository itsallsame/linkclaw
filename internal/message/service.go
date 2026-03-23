package message

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/card"
	"github.com/xiewanpeng/claw-identity/internal/ids"
	"github.com/xiewanpeng/claw-identity/internal/layout"
	"github.com/xiewanpeng/claw-identity/internal/messagecrypto"
	"github.com/xiewanpeng/claw-identity/internal/migrate"
	agentruntime "github.com/xiewanpeng/claw-identity/internal/runtime"
	"github.com/xiewanpeng/claw-identity/internal/transport"
	transportstoreforward "github.com/xiewanpeng/claw-identity/internal/transport/storeforward"

	_ "modernc.org/sqlite"
)

const (
	DirectionIncoming = "incoming"
	DirectionOutgoing = "outgoing"

	StatusPending   = "pending"
	StatusQueued    = "queued"
	StatusDelivered = "delivered"
	StatusFailed    = "failed"

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
	if err := syncRuntimeSendState(ctx, home, contact, conversation, record, now); err != nil {
		return SendResult{}, err
	}
	if contact.RelayURL != "" || strings.TrimSpace(contact.DirectURL) != "" || directTransportEnabled() {
		runtimeResult, err := s.sendThroughRuntime(ctx, home, selfProfile, contact, record, now)
		if err != nil {
			record.Status = StatusFailed
			record.TransportStatus = deriveTransportStatus(record.Direction, record.Status, record.SelectedRoute)
			if syncErr := syncRuntimeSendState(ctx, home, contact, conversation, record, now); syncErr != nil {
				return SendResult{}, syncErr
			}
		} else {
			record.Status = runtimeResult.Status
			record.SelectedRoute = runtimeResult.SelectedRoute
			if runtimeResult.Status == StatusDelivered {
				record.DeliveredAt = now.Format(time.RFC3339Nano)
			}
			record.TransportStatus = deriveTransportStatus(record.Direction, record.Status, record.SelectedRoute)
			if err := syncRuntimeSendState(ctx, home, contact, conversation, record, now); err != nil {
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
		relayURL = strings.TrimSpace(os.Getenv(card.EnvRelayURL))
		if relayURL == "" {
			return SyncResult{Home: home, Synced: 0, RelayCalls: 0, SyncedAt: now.Format(time.RFC3339Nano)}, nil
		}
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

func (s *Service) Status(ctx context.Context, opts ListOptions) (StatusResult, error) {
	now := s.now()
	db, home, err := openStateDB(ctx, opts.Home, now)
	if err != nil {
		return StatusResult{}, err
	}
	defer db.Close()

	store := agentruntime.NewStoreWithDB(db, now)
	if err := syncRuntimeSelfIdentity(ctx, db, store); err != nil {
		return StatusResult{}, err
	}
	if selfProfile, err := loadSelfMessagingProfile(ctx, db, home); err == nil {
		if err := s.ensureDirectRuntimeRegistration(ctx, home, selfProfile, now); err != nil {
			return StatusResult{}, err
		}
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
	discoveryReady := summary.PresenceEntries > 0 || summary.ReachablePresence > 0 || directTransportEnabled()

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
		DirectEnabled:          directTransportEnabled(),
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
	if status == StatusFailed {
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
	switch status {
	case StatusPending, StatusQueued:
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
	response, err := s.storeForwardBackend().Send(ctx, contact.RelayURL, transportstoreforward.MailboxSendRequest{
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
		_ = updateMessageDeliveryState(ctx, home, now, record.MessageID, StatusFailed, "", contact.RelayURL, "failed", err.Error(), encrypted.Ciphertext, signature)
		return record, err
	}
	if err := updateMessageDeliveryState(ctx, home, now, record.MessageID, StatusQueued, response.RemoteMessageID, contact.RelayURL, "queued", "", encrypted.Ciphertext, signature); err != nil {
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

func insertIncomingMessage(ctx context.Context, tx *sql.Tx, conversation Conversation, contact contactRecord, msg transportstoreforward.MailboxPullMessage, body, preview string, now time.Time) error {
	createdAt := strings.TrimSpace(msg.SentAt)
	if createdAt == "" {
		createdAt = now.Format(time.RFC3339Nano)
	}
	if _, err := tx.ExecContext(
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
		StatusQueued,
		msg.RelayMessageID,
		createdAt,
		now.Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("insert incoming message: %w", err)
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
		return fmt.Errorf("update conversation for incoming message: %w", err)
	}
	return nil
}

func insertDirectIncomingMessage(ctx context.Context, tx *sql.Tx, conversation Conversation, contact contactRecord, selfProfile selfMessagingProfile, env transport.Envelope, now time.Time) error {
	createdAt := now.Format(time.RFC3339Nano)
	preview := makePreview(env.Plaintext)
	if _, err := tx.ExecContext(
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
	); err != nil {
		return fmt.Errorf("insert direct incoming message: %w", err)
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
		return fmt.Errorf("update conversation for direct incoming message: %w", err)
	}
	return nil
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
