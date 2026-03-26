package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/layout"
	"github.com/xiewanpeng/claw-identity/internal/migrate"
	"github.com/xiewanpeng/claw-identity/internal/routing"
	"github.com/xiewanpeng/claw-identity/internal/transport"

	_ "modernc.org/sqlite"
)

type Store struct {
	db  *sql.DB
	now func() time.Time
}

func NewStoreWithDB(db *sql.DB, now time.Time) *Store {
	return &Store{
		db:  db,
		now: func() time.Time { return now.UTC() },
	}
}

type SelfIdentityRecord struct {
	SelfID                  string
	DisplayName             string
	PeerID                  string
	SigningPublicKey        string
	EncryptionPublicKey     string
	SigningPrivateKeyRef    string
	EncryptionPrivateKeyRef string
	TransportCapabilities   []string
}

type ContactRecord struct {
	ContactID             string
	CanonicalID           string
	DisplayName           string
	PeerID                string
	SigningPublicKey      string
	EncryptionPublicKey   string
	TrustState            string
	TransportCapabilities []string
	DirectHints           []string
	StoreForwardHints     []string
	SignedPeerRecord      string
	LastSeenAt            string
	LastSuccessfulRoute   string
	RawIdentityCardJSON   string
}

type ConversationRecord struct {
	ConversationID     string
	ContactID          string
	LastMessageID      string
	LastMessagePreview string
	LastMessageAt      string
	UnreadCount        int
}

type MessageRecord struct {
	MessageID         string
	ConversationID    string
	SenderID          string
	RecipientID       string
	Direction         string
	PlaintextBody     string
	PlaintextPreview  string
	Ciphertext        string
	CiphertextVersion string
	Status            string
	SelectedRoute     transport.RouteCandidate
	CreatedAt         string
	DeliveredAt       string
	AckedAt           string
}

type RouteAttemptRecord struct {
	AttemptID      string
	MessageID      string
	ConversationID string
	RouteType      string
	RouteLabel     string
	Priority       int
	Outcome        string
	Error          string
	Retryable      bool
	CursorValue    string
	AttemptedAt    string
}

type ConversationReadModel struct {
	ConversationID     string
	ContactID          string
	ContactDisplayName string
	ContactCanonicalID string
	ContactTrustState  string
	LastMessagePreview string
	LastMessageAt      string
	UnreadCount        int
	Messages           []MessageRecord
}

type StoreForwardStateRecord struct {
	SelfID             string
	RouteLabel         string
	CursorValue        string
	LastResult         string
	LastError          string
	LastRecoveredCount int
	UpdatedAt          string
}

type TransportBindingRecord struct {
	BindingID    string
	SelfID       string
	CanonicalID  string
	Transport    string
	RelayURL     string
	RouteLabel   string
	RouteType    string
	Direction    string
	Enabled      bool
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
}

type TransportRelayRecord struct {
	RelayID      string
	Transport    string
	RelayURL     string
	ReadEnabled  bool
	WriteEnabled bool
	Priority     int
	Source       string
	Status       string
	LastError    string
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
}

type RelaySyncStateRecord struct {
	SelfID              string
	RelayURL            string
	LastCursor          string
	LastEventAt         string
	LastSyncStartedAt   string
	LastSyncCompletedAt string
	LastResult          string
	LastError           string
	RecoveredCountTotal int
	UpdatedAt           string
}

type RelayDeliveryAttemptRecord struct {
	AttemptID    string
	MessageID    string
	EventID      string
	SelfID       string
	CanonicalID  string
	RelayURL     string
	Operation    string
	Outcome      string
	Error        string
	Retryable    bool
	Acknowledged bool
	MetadataJSON string
	AttemptedAt  string
}

type RecoveredEventObservationRecord struct {
	SelfID       string
	EventID      string
	RelayURL     string
	CanonicalID  string
	MessageID    string
	ObservedAt   string
	PayloadHash  string
	PayloadJSON  string
	MetadataJSON string
	CreatedAt    string
	UpdatedAt    string
}

type PresenceRecord struct {
	CanonicalID           string
	PeerID                string
	TransportCapabilities []string
	DirectHints           []string
	StoreForwardHints     []string
	SignedPeerRecord      string
	Source                string
	Reachable             bool
	FreshUntil            string
	ResolvedAt            string
	AnnouncedAt           string
}

type StatusSummary struct {
	SelfID                  string
	DisplayName             string
	PeerID                  string
	TransportCapabilities   []string
	Contacts                int
	Conversations           int
	Unread                  int
	PendingOutbox           int
	PresenceEntries         int
	ReachablePresence       int
	StoreForwardRoutes      int
	LastStoreForwardSyncAt  string
	LastStoreForwardResult  string
	LastStoreForwardError   string
	LastStoreForwardRecover int
	LastAnnounceAt          string
}

func OpenStore(ctx context.Context, rawHome string, now time.Time) (*Store, string, error) {
	home, err := layout.ResolveHome(rawHome)
	if err != nil {
		return nil, "", err
	}
	if _, err := layout.Ensure(home); err != nil {
		return nil, "", err
	}
	paths := layout.BuildPaths(home)
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
	return &Store{
		db:  db,
		now: func() time.Time { return now.UTC() },
	}, home, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) UpsertSelfIdentity(ctx context.Context, record SelfIdentityRecord) error {
	now := s.now().Format(time.RFC3339Nano)
	capsJSON, err := json.Marshal(record.TransportCapabilities)
	if err != nil {
		return fmt.Errorf("marshal self transport capabilities: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO runtime_self_identities (
			self_id, display_name, peer_id, signing_public_key, encryption_public_key,
			signing_private_key_ref, encryption_private_key_ref, transport_capabilities_json,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(self_id) DO UPDATE SET
			display_name = excluded.display_name,
			peer_id = excluded.peer_id,
			signing_public_key = excluded.signing_public_key,
			encryption_public_key = excluded.encryption_public_key,
			signing_private_key_ref = excluded.signing_private_key_ref,
			encryption_private_key_ref = excluded.encryption_private_key_ref,
			transport_capabilities_json = excluded.transport_capabilities_json,
			updated_at = excluded.updated_at
	`,
		record.SelfID, record.DisplayName, record.PeerID, record.SigningPublicKey, record.EncryptionPublicKey,
		record.SigningPrivateKeyRef, record.EncryptionPrivateKeyRef, string(capsJSON), now, now,
	)
	if err != nil {
		return fmt.Errorf("upsert runtime self identity: %w", err)
	}
	return nil
}

func (s *Store) UpsertContact(ctx context.Context, record ContactRecord) error {
	now := s.now().Format(time.RFC3339Nano)
	capsJSON, err := json.Marshal(record.TransportCapabilities)
	if err != nil {
		return fmt.Errorf("marshal contact transport capabilities: %w", err)
	}
	directJSON, err := json.Marshal(record.DirectHints)
	if err != nil {
		return fmt.Errorf("marshal contact direct hints: %w", err)
	}
	storeForwardJSON, err := json.Marshal(record.StoreForwardHints)
	if err != nil {
		return fmt.Errorf("marshal contact store forward hints: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO runtime_contacts (
			contact_id, canonical_id, display_name, peer_id, signing_public_key, encryption_public_key,
			trust_state, transport_capabilities_json, direct_hints_json, store_forward_hints_json,
			signed_peer_record, last_seen_at, last_successful_route, raw_identity_card_json,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(canonical_id) DO UPDATE SET
			display_name = excluded.display_name,
			peer_id = excluded.peer_id,
			signing_public_key = excluded.signing_public_key,
			encryption_public_key = excluded.encryption_public_key,
			trust_state = excluded.trust_state,
			transport_capabilities_json = excluded.transport_capabilities_json,
			direct_hints_json = excluded.direct_hints_json,
			store_forward_hints_json = excluded.store_forward_hints_json,
			signed_peer_record = excluded.signed_peer_record,
			last_seen_at = excluded.last_seen_at,
			last_successful_route = excluded.last_successful_route,
			raw_identity_card_json = excluded.raw_identity_card_json,
			updated_at = excluded.updated_at
	`,
		record.ContactID, record.CanonicalID, record.DisplayName, record.PeerID, record.SigningPublicKey, record.EncryptionPublicKey,
		record.TrustState, string(capsJSON), string(directJSON), string(storeForwardJSON),
		record.SignedPeerRecord, record.LastSeenAt, record.LastSuccessfulRoute, record.RawIdentityCardJSON,
		now, now,
	)
	if err != nil {
		return fmt.Errorf("upsert runtime contact: %w", err)
	}
	return nil
}

func (s *Store) UpsertConversation(ctx context.Context, record ConversationRecord) error {
	now := s.now().Format(time.RFC3339Nano)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runtime_conversations (
			conversation_id, contact_id, last_message_id, last_message_preview, last_message_at,
			unread_count, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(conversation_id) DO UPDATE SET
			last_message_id = excluded.last_message_id,
			last_message_preview = excluded.last_message_preview,
			last_message_at = excluded.last_message_at,
			unread_count = excluded.unread_count,
			updated_at = excluded.updated_at
	`,
		record.ConversationID, record.ContactID, record.LastMessageID, record.LastMessagePreview,
		record.LastMessageAt, record.UnreadCount, now, now,
	)
	if err != nil {
		return fmt.Errorf("upsert runtime conversation: %w", err)
	}
	return nil
}

func (s *Store) InsertMessage(ctx context.Context, record MessageRecord) error {
	return s.upsertMessage(ctx, record, false)
}

func (s *Store) UpsertMessage(ctx context.Context, record MessageRecord) error {
	return s.upsertMessage(ctx, record, true)
}

func (s *Store) upsertMessage(ctx context.Context, record MessageRecord, update bool) error {
	if normalized := NormalizeMessageStatus(record.Status); normalized != "" {
		record.Status = normalized
	}
	if update {
		existingStatus, found, err := s.loadMessageStatus(ctx, record.MessageID)
		if err != nil {
			return err
		}
		if found {
			record.Status = MergeMessageStatus(existingStatus, record.Status)
		}
	}
	if NormalizeMessageStatus(record.Status) == MessageStatusDelivered && strings.TrimSpace(record.DeliveredAt) == "" {
		record.DeliveredAt = s.now().Format(time.RFC3339Nano)
	}
	routeJSON, err := json.Marshal(record.SelectedRoute)
	if err != nil {
		return fmt.Errorf("marshal selected route: %w", err)
	}
	query := `
		INSERT INTO runtime_messages (
			message_id, conversation_id, sender_id, recipient_id, direction, plaintext_body, plaintext_preview,
			ciphertext, ciphertext_version, status, selected_route_json, created_at, delivered_at, acked_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	if update {
		query += `
		ON CONFLICT(message_id) DO UPDATE SET
			conversation_id = excluded.conversation_id,
			sender_id = excluded.sender_id,
			recipient_id = excluded.recipient_id,
			direction = excluded.direction,
			plaintext_body = excluded.plaintext_body,
			plaintext_preview = excluded.plaintext_preview,
			ciphertext = excluded.ciphertext,
			ciphertext_version = excluded.ciphertext_version,
			status = excluded.status,
			selected_route_json = excluded.selected_route_json,
			delivered_at = excluded.delivered_at,
			acked_at = excluded.acked_at
		`
	}
	_, err = s.db.ExecContext(ctx, query,
		record.MessageID, record.ConversationID, record.SenderID, record.RecipientID, record.Direction,
		record.PlaintextBody, record.PlaintextPreview, record.Ciphertext, record.CiphertextVersion, record.Status, string(routeJSON),
		record.CreatedAt, record.DeliveredAt, record.AckedAt,
	)
	if err != nil {
		return fmt.Errorf("insert runtime message: %w", err)
	}
	return nil
}

func (s *Store) loadMessageStatus(ctx context.Context, messageID string) (string, bool, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `
		SELECT status
		FROM runtime_messages
		WHERE message_id = ?
		LIMIT 1
	`, messageID).Scan(&status)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("load runtime message status: %w", err)
	}
	return status, true, nil
}

func (s *Store) ListConversations(ctx context.Context) ([]ConversationReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.conversation_id, c.contact_id, ct.display_name, ct.canonical_id, ct.trust_state,
		       c.last_message_preview, c.last_message_at, c.unread_count
		FROM runtime_conversations c
		JOIN runtime_contacts ct ON ct.contact_id = c.contact_id
		ORDER BY c.last_message_at DESC, c.conversation_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list runtime conversations: %w", err)
	}
	defer rows.Close()

	conversations := make([]ConversationReadModel, 0)
	for rows.Next() {
		var record ConversationReadModel
		if err := rows.Scan(
			&record.ConversationID,
			&record.ContactID,
			&record.ContactDisplayName,
			&record.ContactCanonicalID,
			&record.ContactTrustState,
			&record.LastMessagePreview,
			&record.LastMessageAt,
			&record.UnreadCount,
		); err != nil {
			return nil, fmt.Errorf("scan runtime conversation: %w", err)
		}
		conversations = append(conversations, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime conversations: %w", err)
	}
	return conversations, nil
}

func (s *Store) ListMessages(ctx context.Context) ([]MessageRecord, error) {
	return s.listMessagesWithQuery(ctx, `
		SELECT message_id, conversation_id, sender_id, recipient_id, direction, plaintext_body,
		       plaintext_preview, ciphertext, ciphertext_version, status, selected_route_json,
		       created_at, delivered_at, acked_at
		FROM runtime_messages
		ORDER BY created_at DESC, message_id DESC
	`, "list runtime messages")
}

func (s *Store) ListOutgoingMessages(ctx context.Context) ([]MessageRecord, error) {
	return s.listMessagesWithQuery(ctx, `
		SELECT message_id, conversation_id, sender_id, recipient_id, direction, plaintext_body,
		       plaintext_preview, ciphertext, ciphertext_version, status, selected_route_json,
		       created_at, delivered_at, acked_at
		FROM runtime_messages
		WHERE direction = 'outgoing'
		ORDER BY created_at DESC, message_id DESC
	`, "list runtime outgoing messages")
}

func (s *Store) listMessagesWithQuery(ctx context.Context, query string, operation string) ([]MessageRecord, error) {
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()

	messages := make([]MessageRecord, 0)
	for rows.Next() {
		var record MessageRecord
		var routeJSON string
		if err := rows.Scan(
			&record.MessageID,
			&record.ConversationID,
			&record.SenderID,
			&record.RecipientID,
			&record.Direction,
			&record.PlaintextBody,
			&record.PlaintextPreview,
			&record.Ciphertext,
			&record.CiphertextVersion,
			&record.Status,
			&routeJSON,
			&record.CreatedAt,
			&record.DeliveredAt,
			&record.AckedAt,
		); err != nil {
			return nil, fmt.Errorf("scan runtime message: %w", err)
		}
		_ = json.Unmarshal([]byte(routeJSON), &record.SelectedRoute)
		messages = append(messages, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime messages: %w", err)
	}
	return messages, nil
}

func (s *Store) LoadConversationByContactRef(ctx context.Context, ref string, limit int) (ConversationReadModel, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT ct.contact_id, ct.canonical_id, ct.display_name, ct.peer_id, ct.signing_public_key,
		       ct.encryption_public_key, ct.trust_state
		FROM runtime_contacts ct
		WHERE ct.contact_id = ? OR ct.canonical_id = ? OR ct.display_name = ?
		ORDER BY ct.contact_id ASC
	`, ref, ref, ref)
	if err != nil {
		return ConversationReadModel{}, fmt.Errorf("query runtime contact: %w", err)
	}
	defer rows.Close()

	type runtimeContact struct {
		ContactID   string
		CanonicalID string
		DisplayName string
		TrustState  string
	}
	matches := make([]runtimeContact, 0)
	for rows.Next() {
		var record runtimeContact
		var discard string
		if err := rows.Scan(&record.ContactID, &record.CanonicalID, &record.DisplayName, &discard, &discard, &discard, &record.TrustState); err != nil {
			return ConversationReadModel{}, fmt.Errorf("scan runtime contact: %w", err)
		}
		matches = append(matches, record)
	}
	if err := rows.Err(); err != nil {
		return ConversationReadModel{}, fmt.Errorf("iterate runtime contact matches: %w", err)
	}
	if len(matches) == 0 {
		return ConversationReadModel{}, sql.ErrNoRows
	}
	if len(matches) > 1 {
		return ConversationReadModel{}, fmt.Errorf("contact reference %q is ambiguous", ref)
	}

	var conversation ConversationReadModel
	conversation.ContactID = matches[0].ContactID
	conversation.ContactCanonicalID = matches[0].CanonicalID
	conversation.ContactDisplayName = matches[0].DisplayName
	conversation.ContactTrustState = matches[0].TrustState

	_ = s.db.QueryRowContext(ctx, `
		SELECT conversation_id, last_message_preview, last_message_at, unread_count
		FROM runtime_conversations
		WHERE contact_id = ?
		LIMIT 1
	`, matches[0].ContactID).Scan(
		&conversation.ConversationID,
		&conversation.LastMessagePreview,
		&conversation.LastMessageAt,
		&conversation.UnreadCount,
	)
	if strings.TrimSpace(conversation.ConversationID) == "" {
		return conversation, nil
	}

	if limit <= 0 {
		limit = 50
	}
	msgRows, err := s.db.QueryContext(ctx, `
		SELECT message_id, conversation_id, sender_id, recipient_id, direction, plaintext_body,
		       plaintext_preview, ciphertext, ciphertext_version, status, selected_route_json,
		       created_at, delivered_at, acked_at
		FROM runtime_messages
		WHERE conversation_id = ?
		ORDER BY created_at ASC
		LIMIT ?
	`, conversation.ConversationID, limit)
	if err != nil {
		return ConversationReadModel{}, fmt.Errorf("query runtime messages: %w", err)
	}
	defer msgRows.Close()
	for msgRows.Next() {
		var record MessageRecord
		var routeJSON string
		if err := msgRows.Scan(
			&record.MessageID,
			&record.ConversationID,
			&record.SenderID,
			&record.RecipientID,
			&record.Direction,
			&record.PlaintextBody,
			&record.PlaintextPreview,
			&record.Ciphertext,
			&record.CiphertextVersion,
			&record.Status,
			&routeJSON,
			&record.CreatedAt,
			&record.DeliveredAt,
			&record.AckedAt,
		); err != nil {
			return ConversationReadModel{}, fmt.Errorf("scan runtime message: %w", err)
		}
		_ = json.Unmarshal([]byte(routeJSON), &record.SelectedRoute)
		conversation.Messages = append(conversation.Messages, record)
	}
	if err := msgRows.Err(); err != nil {
		return ConversationReadModel{}, fmt.Errorf("iterate runtime messages: %w", err)
	}
	return conversation, nil
}

func (s *Store) MarkConversationRead(ctx context.Context, conversationID string) error {
	if strings.TrimSpace(conversationID) == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE runtime_conversations
		SET unread_count = 0, updated_at = ?
		WHERE conversation_id = ?
	`, s.now().Format(time.RFC3339Nano), conversationID)
	if err != nil {
		return fmt.Errorf("mark runtime conversation read: %w", err)
	}
	return nil
}

func (s *Store) RecordRouteAttempt(ctx context.Context, outcome routing.RouteOutcome, conversationID string, cursor string) error {
	outcomeLabel := strings.TrimSpace(outcome.Outcome)
	if outcomeLabel == "" {
		outcomeLabel = boolToOutcome(outcome.Success)
	}
	cursorValue := coalesceString(cursor, outcome.Cursor)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runtime_route_attempts (
			attempt_id, message_id, conversation_id, route_type, route_label, priority,
			outcome, error, retryable, cursor_value, attempted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		fmt.Sprintf("%s:%s:%d", outcome.MessageID, outcome.Route.Label, outcome.OccurredAt.UnixNano()),
		outcome.MessageID, conversationID, string(outcome.Route.Type), outcome.Route.Label, outcome.Route.Priority,
		outcomeLabel, outcome.Error, boolToInt(outcome.Retryable), cursorValue,
		outcome.OccurredAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return fmt.Errorf("record runtime route attempt: %w", err)
	}
	return nil
}

func (s *Store) ListRecentRouteAttempts(ctx context.Context, limit int) ([]RouteAttemptRecord, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT attempt_id, message_id, conversation_id, route_type, route_label, priority,
		       outcome, error, retryable, cursor_value, attempted_at
		FROM runtime_route_attempts
		ORDER BY attempted_at DESC, attempt_id DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list runtime route attempts: %w", err)
	}
	defer rows.Close()

	records := make([]RouteAttemptRecord, 0, limit)
	for rows.Next() {
		var record RouteAttemptRecord
		var retryable int
		if err := rows.Scan(
			&record.AttemptID,
			&record.MessageID,
			&record.ConversationID,
			&record.RouteType,
			&record.RouteLabel,
			&record.Priority,
			&record.Outcome,
			&record.Error,
			&retryable,
			&record.CursorValue,
			&record.AttemptedAt,
		); err != nil {
			return nil, fmt.Errorf("scan runtime route attempt: %w", err)
		}
		record.Retryable = retryable == 1
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime route attempts: %w", err)
	}
	return records, nil
}

func (s *Store) UpsertPresence(ctx context.Context, record PresenceRecord) error {
	now := s.now().Format(time.RFC3339Nano)
	capsJSON, err := json.Marshal(record.TransportCapabilities)
	if err != nil {
		return fmt.Errorf("marshal presence transport capabilities: %w", err)
	}
	directJSON, err := json.Marshal(record.DirectHints)
	if err != nil {
		return fmt.Errorf("marshal presence direct hints: %w", err)
	}
	storeForwardJSON, err := json.Marshal(record.StoreForwardHints)
	if err != nil {
		return fmt.Errorf("marshal presence store-forward hints: %w", err)
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO runtime_presence_cache (
			canonical_id, peer_id, direct_hints_json, signed_peer_record, source, fresh_until, resolved_at,
			transport_capabilities_json, store_forward_hints_json, reachable, announced_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(canonical_id) DO UPDATE SET
			peer_id = excluded.peer_id,
			direct_hints_json = excluded.direct_hints_json,
			signed_peer_record = excluded.signed_peer_record,
			source = excluded.source,
			fresh_until = excluded.fresh_until,
			resolved_at = excluded.resolved_at,
			transport_capabilities_json = excluded.transport_capabilities_json,
			store_forward_hints_json = excluded.store_forward_hints_json,
			reachable = excluded.reachable,
			announced_at = excluded.announced_at
	`,
		record.CanonicalID,
		record.PeerID,
		string(directJSON),
		record.SignedPeerRecord,
		record.Source,
		record.FreshUntil,
		coalesceString(record.ResolvedAt, now),
		string(capsJSON),
		string(storeForwardJSON),
		boolToInt(record.Reachable),
		record.AnnouncedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert runtime presence cache: %w", err)
	}
	return nil
}

func (s *Store) LoadStoreForwardCursor(ctx context.Context, selfID string, routeLabel string) (string, error) {
	var cursor string
	err := s.db.QueryRowContext(ctx, `
		SELECT cursor_value
		FROM runtime_store_forward_state
		WHERE self_id = ? AND route_label = ?
	`, selfID, routeLabel).Scan(&cursor)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load runtime store-forward cursor: %w", err)
	}
	return cursor, nil
}

func (s *Store) SaveStoreForwardState(ctx context.Context, record StoreForwardStateRecord) error {
	now := record.UpdatedAt
	if strings.TrimSpace(now) == "" {
		now = s.now().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runtime_store_forward_state (
			self_id, route_label, cursor_value, last_result, last_error, last_recovered_count, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(self_id, route_label) DO UPDATE SET
			cursor_value = excluded.cursor_value,
			last_result = excluded.last_result,
			last_error = excluded.last_error,
			last_recovered_count = excluded.last_recovered_count,
			updated_at = excluded.updated_at
	`,
		record.SelfID,
		record.RouteLabel,
		record.CursorValue,
		record.LastResult,
		record.LastError,
		record.LastRecoveredCount,
		now,
	)
	if err != nil {
		return fmt.Errorf("save runtime store-forward state: %w", err)
	}
	return nil
}

func (s *Store) UpsertTransportBinding(ctx context.Context, record TransportBindingRecord) error {
	now := coalesceString(record.UpdatedAt, s.now().Format(time.RFC3339Nano))
	createdAt := coalesceString(record.CreatedAt, now)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runtime_transport_bindings (
			binding_id, self_id, canonical_id, transport, relay_url, route_label, route_type,
			direction, enabled, metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(binding_id) DO UPDATE SET
			self_id = excluded.self_id,
			canonical_id = excluded.canonical_id,
			transport = excluded.transport,
			relay_url = excluded.relay_url,
			route_label = excluded.route_label,
			route_type = excluded.route_type,
			direction = excluded.direction,
			enabled = excluded.enabled,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at
	`,
		record.BindingID,
		record.SelfID,
		record.CanonicalID,
		record.Transport,
		record.RelayURL,
		record.RouteLabel,
		record.RouteType,
		record.Direction,
		boolToInt(record.Enabled),
		coalesceString(record.MetadataJSON, "{}"),
		createdAt,
		now,
	)
	if err != nil {
		return fmt.Errorf("upsert runtime transport binding: %w", err)
	}
	return nil
}

func (s *Store) ListTransportBindings(ctx context.Context, selfID string) ([]TransportBindingRecord, error) {
	query := `
		SELECT binding_id, self_id, canonical_id, transport, relay_url, route_label, route_type,
		       direction, enabled, metadata_json, created_at, updated_at
		FROM runtime_transport_bindings
	`
	args := make([]any, 0, 1)
	if strings.TrimSpace(selfID) != "" {
		query += " WHERE self_id = ?"
		args = append(args, selfID)
	}
	query += " ORDER BY transport ASC, relay_url ASC, binding_id ASC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list runtime transport bindings: %w", err)
	}
	defer rows.Close()

	records := make([]TransportBindingRecord, 0)
	for rows.Next() {
		var record TransportBindingRecord
		var enabled int
		if err := rows.Scan(
			&record.BindingID,
			&record.SelfID,
			&record.CanonicalID,
			&record.Transport,
			&record.RelayURL,
			&record.RouteLabel,
			&record.RouteType,
			&record.Direction,
			&enabled,
			&record.MetadataJSON,
			&record.CreatedAt,
			&record.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan runtime transport binding: %w", err)
		}
		record.Enabled = enabled == 1
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime transport bindings: %w", err)
	}
	return records, nil
}

func (s *Store) UpsertTransportRelay(ctx context.Context, record TransportRelayRecord) error {
	now := coalesceString(record.UpdatedAt, s.now().Format(time.RFC3339Nano))
	createdAt := coalesceString(record.CreatedAt, now)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runtime_transport_relays (
			relay_id, transport, relay_url, read_enabled, write_enabled, priority, source, status,
			last_error, metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(relay_url) DO UPDATE SET
			relay_id = excluded.relay_id,
			transport = excluded.transport,
			read_enabled = excluded.read_enabled,
			write_enabled = excluded.write_enabled,
			priority = excluded.priority,
			source = excluded.source,
			status = excluded.status,
			last_error = excluded.last_error,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at
	`,
		record.RelayID,
		record.Transport,
		record.RelayURL,
		boolToInt(record.ReadEnabled),
		boolToInt(record.WriteEnabled),
		record.Priority,
		record.Source,
		record.Status,
		record.LastError,
		coalesceString(record.MetadataJSON, "{}"),
		createdAt,
		now,
	)
	if err != nil {
		return fmt.Errorf("upsert runtime transport relay: %w", err)
	}
	return nil
}

func (s *Store) ListTransportRelays(ctx context.Context, transportName string) ([]TransportRelayRecord, error) {
	query := `
		SELECT relay_id, transport, relay_url, read_enabled, write_enabled, priority, source, status,
		       last_error, metadata_json, created_at, updated_at
		FROM runtime_transport_relays
	`
	args := make([]any, 0, 1)
	if strings.TrimSpace(transportName) != "" {
		query += " WHERE transport = ?"
		args = append(args, transportName)
	}
	query += " ORDER BY priority DESC, relay_url ASC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list runtime transport relays: %w", err)
	}
	defer rows.Close()

	records := make([]TransportRelayRecord, 0)
	for rows.Next() {
		var record TransportRelayRecord
		var readEnabled, writeEnabled int
		if err := rows.Scan(
			&record.RelayID,
			&record.Transport,
			&record.RelayURL,
			&readEnabled,
			&writeEnabled,
			&record.Priority,
			&record.Source,
			&record.Status,
			&record.LastError,
			&record.MetadataJSON,
			&record.CreatedAt,
			&record.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan runtime transport relay: %w", err)
		}
		record.ReadEnabled = readEnabled == 1
		record.WriteEnabled = writeEnabled == 1
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime transport relays: %w", err)
	}
	return records, nil
}

func (s *Store) SaveRelaySyncState(ctx context.Context, record RelaySyncStateRecord) error {
	now := coalesceString(record.UpdatedAt, s.now().Format(time.RFC3339Nano))
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runtime_relay_sync_state (
			self_id, relay_url, last_cursor, last_event_at, last_sync_started_at, last_sync_completed_at,
			last_result, last_error, recovered_count_total, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(self_id, relay_url) DO UPDATE SET
			last_cursor = excluded.last_cursor,
			last_event_at = excluded.last_event_at,
			last_sync_started_at = excluded.last_sync_started_at,
			last_sync_completed_at = excluded.last_sync_completed_at,
			last_result = excluded.last_result,
			last_error = excluded.last_error,
			recovered_count_total = excluded.recovered_count_total,
			updated_at = excluded.updated_at
	`,
		record.SelfID,
		record.RelayURL,
		record.LastCursor,
		record.LastEventAt,
		record.LastSyncStartedAt,
		record.LastSyncCompletedAt,
		record.LastResult,
		record.LastError,
		record.RecoveredCountTotal,
		now,
	)
	if err != nil {
		return fmt.Errorf("save runtime relay sync state: %w", err)
	}
	return nil
}

func (s *Store) LoadRelaySyncState(ctx context.Context, selfID string, relayURL string) (RelaySyncStateRecord, bool, error) {
	var record RelaySyncStateRecord
	err := s.db.QueryRowContext(ctx, `
		SELECT self_id, relay_url, last_cursor, last_event_at, last_sync_started_at, last_sync_completed_at,
		       last_result, last_error, recovered_count_total, updated_at
		FROM runtime_relay_sync_state
		WHERE self_id = ? AND relay_url = ?
	`, selfID, relayURL).Scan(
		&record.SelfID,
		&record.RelayURL,
		&record.LastCursor,
		&record.LastEventAt,
		&record.LastSyncStartedAt,
		&record.LastSyncCompletedAt,
		&record.LastResult,
		&record.LastError,
		&record.RecoveredCountTotal,
		&record.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return RelaySyncStateRecord{}, false, nil
	}
	if err != nil {
		return RelaySyncStateRecord{}, false, fmt.Errorf("load runtime relay sync state: %w", err)
	}
	return record, true, nil
}

func (s *Store) RecordRelayDeliveryAttempt(ctx context.Context, record RelayDeliveryAttemptRecord) error {
	attemptedAt := coalesceString(record.AttemptedAt, s.now().Format(time.RFC3339Nano))
	attemptID := strings.TrimSpace(record.AttemptID)
	if attemptID == "" {
		attemptID = fmt.Sprintf("%s:%s:%d", coalesceString(record.MessageID, record.EventID, "relay"), record.RelayURL, s.now().UnixNano())
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runtime_relay_delivery_attempts (
			attempt_id, message_id, event_id, self_id, canonical_id, relay_url, operation, outcome,
			error, retryable, acknowledged, metadata_json, attempted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		attemptID,
		record.MessageID,
		record.EventID,
		record.SelfID,
		record.CanonicalID,
		record.RelayURL,
		record.Operation,
		record.Outcome,
		record.Error,
		boolToInt(record.Retryable),
		boolToInt(record.Acknowledged),
		coalesceString(record.MetadataJSON, "{}"),
		attemptedAt,
	)
	if err != nil {
		return fmt.Errorf("record runtime relay delivery attempt: %w", err)
	}
	return nil
}

func (s *Store) ListRecentRelayDeliveryAttempts(ctx context.Context, relayURL string, limit int) ([]RelayDeliveryAttemptRecord, error) {
	if limit <= 0 {
		limit = 10
	}
	query := `
		SELECT attempt_id, message_id, event_id, self_id, canonical_id, relay_url, operation, outcome,
		       error, retryable, acknowledged, metadata_json, attempted_at
		FROM runtime_relay_delivery_attempts
	`
	args := make([]any, 0, 2)
	if strings.TrimSpace(relayURL) != "" {
		query += " WHERE relay_url = ?"
		args = append(args, relayURL)
	}
	query += " ORDER BY attempted_at DESC, attempt_id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list runtime relay delivery attempts: %w", err)
	}
	defer rows.Close()

	records := make([]RelayDeliveryAttemptRecord, 0, limit)
	for rows.Next() {
		var record RelayDeliveryAttemptRecord
		var retryable, acknowledged int
		if err := rows.Scan(
			&record.AttemptID,
			&record.MessageID,
			&record.EventID,
			&record.SelfID,
			&record.CanonicalID,
			&record.RelayURL,
			&record.Operation,
			&record.Outcome,
			&record.Error,
			&retryable,
			&acknowledged,
			&record.MetadataJSON,
			&record.AttemptedAt,
		); err != nil {
			return nil, fmt.Errorf("scan runtime relay delivery attempt: %w", err)
		}
		record.Retryable = retryable == 1
		record.Acknowledged = acknowledged == 1
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime relay delivery attempts: %w", err)
	}
	return records, nil
}

func (s *Store) UpsertRecoveredEventObservation(ctx context.Context, record RecoveredEventObservationRecord) error {
	now := coalesceString(record.UpdatedAt, s.now().Format(time.RFC3339Nano))
	createdAt := coalesceString(record.CreatedAt, now)
	observedAt := coalesceString(record.ObservedAt, now)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO runtime_recovered_event_observations (
			self_id, event_id, relay_url, canonical_id, message_id, observed_at, payload_hash,
			payload_json, metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(self_id, event_id, relay_url) DO UPDATE SET
			canonical_id = excluded.canonical_id,
			message_id = excluded.message_id,
			observed_at = excluded.observed_at,
			payload_hash = excluded.payload_hash,
			payload_json = excluded.payload_json,
			metadata_json = excluded.metadata_json,
			updated_at = excluded.updated_at
	`,
		record.SelfID,
		record.EventID,
		record.RelayURL,
		record.CanonicalID,
		record.MessageID,
		observedAt,
		record.PayloadHash,
		record.PayloadJSON,
		coalesceString(record.MetadataJSON, "{}"),
		createdAt,
		now,
	)
	if err != nil {
		return fmt.Errorf("upsert runtime recovered event observation: %w", err)
	}
	return nil
}

func (s *Store) HasRecoveredEventObservation(ctx context.Context, selfID string, eventID string) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM runtime_recovered_event_observations
		WHERE self_id = ? AND event_id = ?
	`, selfID, eventID).Scan(&count); err != nil {
		return false, fmt.Errorf("check runtime recovered event observation: %w", err)
	}
	return count > 0, nil
}

func (s *Store) ListRecoveredEventObservations(ctx context.Context, selfID string, limit int) ([]RecoveredEventObservationRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	query := `
		SELECT self_id, event_id, relay_url, canonical_id, message_id, observed_at, payload_hash,
		       payload_json, metadata_json, created_at, updated_at
		FROM runtime_recovered_event_observations
	`
	args := make([]any, 0, 2)
	if strings.TrimSpace(selfID) != "" {
		query += " WHERE self_id = ?"
		args = append(args, selfID)
	}
	query += " ORDER BY observed_at DESC, event_id DESC, relay_url ASC LIMIT ?"
	args = append(args, limit)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list runtime recovered event observations: %w", err)
	}
	defer rows.Close()

	records := make([]RecoveredEventObservationRecord, 0, limit)
	for rows.Next() {
		var record RecoveredEventObservationRecord
		if err := rows.Scan(
			&record.SelfID,
			&record.EventID,
			&record.RelayURL,
			&record.CanonicalID,
			&record.MessageID,
			&record.ObservedAt,
			&record.PayloadHash,
			&record.PayloadJSON,
			&record.MetadataJSON,
			&record.CreatedAt,
			&record.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan runtime recovered event observation: %w", err)
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime recovered event observations: %w", err)
	}
	return records, nil
}

func (s *Store) LoadStatusSummary(ctx context.Context) (StatusSummary, error) {
	var summary StatusSummary
	var capsJSON string
	err := s.db.QueryRowContext(ctx, `
		SELECT self_id, display_name, peer_id, transport_capabilities_json
		FROM runtime_self_identities
		ORDER BY created_at ASC
		LIMIT 1
	`).Scan(
		&summary.SelfID,
		&summary.DisplayName,
		&summary.PeerID,
		&capsJSON,
	)
	if err != nil && err != sql.ErrNoRows {
		return StatusSummary{}, fmt.Errorf("load runtime self identity summary: %w", err)
	}
	if capsJSON != "" {
		_ = json.Unmarshal([]byte(capsJSON), &summary.TransportCapabilities)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_contacts`).Scan(&summary.Contacts); err != nil {
		return StatusSummary{}, fmt.Errorf("count runtime contacts: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_conversations`).Scan(&summary.Conversations); err != nil {
		return StatusSummary{}, fmt.Errorf("count runtime conversations: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(unread_count), 0) FROM runtime_conversations`).Scan(&summary.Unread); err != nil {
		return StatusSummary{}, fmt.Errorf("sum runtime unread count: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM runtime_messages
		WHERE direction = 'outgoing' AND status IN ('pending', 'queued', 'recovering')
	`).Scan(&summary.PendingOutbox); err != nil {
		return StatusSummary{}, fmt.Errorf("count runtime pending outbox: %w", err)
	}
	peerPresenceEntries, err := s.countPeerPresence(ctx, false)
	if err != nil {
		return StatusSummary{}, fmt.Errorf("count runtime peer presence cache: %w", err)
	}
	summary.PresenceEntries = peerPresenceEntries
	reachablePeerPresence, err := s.countPeerPresence(ctx, true)
	if err != nil {
		return StatusSummary{}, fmt.Errorf("count reachable runtime peer presence cache: %w", err)
	}
	summary.ReachablePresence = reachablePeerPresence
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_store_forward_state`).Scan(&summary.StoreForwardRoutes); err != nil {
		return StatusSummary{}, fmt.Errorf("count runtime store-forward routes: %w", err)
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT updated_at, last_result, last_error, last_recovered_count
		FROM runtime_store_forward_state
		ORDER BY updated_at DESC
		LIMIT 1
	`).Scan(
		&summary.LastStoreForwardSyncAt,
		&summary.LastStoreForwardResult,
		&summary.LastStoreForwardError,
		&summary.LastStoreForwardRecover,
	)
	if err != nil && err != sql.ErrNoRows {
		return StatusSummary{}, fmt.Errorf("load runtime store-forward summary: %w", err)
	}
	err = s.db.QueryRowContext(ctx, `
		SELECT announced_at
		FROM runtime_presence_cache
		WHERE announced_at <> ''
		ORDER BY announced_at DESC
		LIMIT 1
	`).Scan(&summary.LastAnnounceAt)
	if err != nil && err != sql.ErrNoRows {
		return StatusSummary{}, fmt.Errorf("load runtime presence announce summary: %w", err)
	}
	return summary, nil
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func (s *Store) countPeerPresence(ctx context.Context, reachableOnly bool) (int, error) {
	var query string
	if reachableOnly {
		query = `
			SELECT COUNT(*)
			FROM runtime_presence_cache
			WHERE reachable = 1
			  AND canonical_id NOT IN (
			    SELECT canonical_id
			    FROM self_identities
			    WHERE TRIM(canonical_id) <> ''
			  )
		`
	} else {
		query = `
			SELECT COUNT(*)
			FROM runtime_presence_cache
			WHERE canonical_id NOT IN (
			  SELECT canonical_id
			  FROM self_identities
			  WHERE TRIM(canonical_id) <> ''
			)
		`
	}
	var count int
	if err := s.db.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func boolToOutcome(success bool) string {
	if success {
		return "success"
	}
	return "failed"
}

func coalesceString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
