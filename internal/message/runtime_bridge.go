package message

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"time"

	agentdiscovery "github.com/xiewanpeng/claw-identity/internal/discovery"
	discoverylibp2p "github.com/xiewanpeng/claw-identity/internal/discovery/libp2p"
	"github.com/xiewanpeng/claw-identity/internal/routing"
	agentruntime "github.com/xiewanpeng/claw-identity/internal/runtime"
	"github.com/xiewanpeng/claw-identity/internal/transport"
	transportlibp2p "github.com/xiewanpeng/claw-identity/internal/transport/libp2p"
	transportstoreforward "github.com/xiewanpeng/claw-identity/internal/transport/storeforward"
)

type staticDiscoveryService struct {
	view agentdiscovery.PeerPresenceView
}

func (s staticDiscoveryService) ResolvePeer(context.Context, string) (agentdiscovery.PeerPresenceView, error) {
	return s.view, nil
}

func (s staticDiscoveryService) RefreshPeer(context.Context, string) (agentdiscovery.PeerPresenceView, error) {
	return s.view, nil
}

func (s staticDiscoveryService) PublishSelf(context.Context) error { return nil }

type staticPlanner struct {
	sendRoutes    []transport.RouteCandidate
	recoverRoutes []transport.RouteCandidate
	record        func(context.Context, routing.RouteOutcome) error
}

func (s staticPlanner) PlanSend(context.Context, routing.ContactRuntimeView, agentdiscovery.PeerPresenceView) ([]transport.RouteCandidate, error) {
	return s.sendRoutes, nil
}

func (s staticPlanner) PlanRecover(context.Context, routing.ContactRuntimeView, agentdiscovery.PeerPresenceView) ([]transport.RouteCandidate, error) {
	return s.recoverRoutes, nil
}

func (s staticPlanner) RecordOutcome(ctx context.Context, outcome routing.RouteOutcome) error {
	if s.record == nil {
		return nil
	}
	return s.record(ctx, outcome)
}

type legacyStoreForwardBackend struct {
	service     *Service
	home        string
	now         time.Time
	record      MessageRecord
	contact     contactRecord
	selfProfile selfMessagingProfile
}

func (b legacyStoreForwardBackend) Send(ctx context.Context, _ transport.Envelope, route transport.RouteCandidate) (transport.SendResult, error) {
	updated, err := b.service.deliverOutgoing(ctx, b.home, b.record, b.contact, b.now)
	if err != nil {
		return transport.SendResult{}, err
	}
	return transport.SendResult{
		Route:       route,
		Delivered:   false,
		Retryable:   true,
		Description: updated.Status,
	}, nil
}

func (b legacyStoreForwardBackend) Recover(ctx context.Context, route transport.RouteCandidate) (transport.SyncResult, error) {
	count, nextCursor, err := b.service.syncStoreForward(ctx, b.home, b.selfProfile, route.Target, b.now)
	if err != nil {
		return transport.SyncResult{}, err
	}
	return transport.SyncResult{
		Route:          route,
		Recovered:      count,
		AdvancedCursor: nextCursor,
	}, nil
}

func (b legacyStoreForwardBackend) Acknowledge(ctx context.Context, route transport.RouteCandidate, cursor string) error {
	if cursor == "" {
		return nil
	}
	return b.service.storeForwardBackend().Ack(ctx, route.Target, transportstoreforward.MailboxAckRequest{
		RecipientID: b.selfProfile.RecipientID,
		Cursor:      cursor,
	})
}

type directInboxReceiver struct {
	service     *Service
	home        string
	selfProfile selfMessagingProfile
	now         time.Time
}

func (r directInboxReceiver) ReceiveDirect(ctx context.Context, env transport.Envelope) error {
	return r.service.receiveDirectEnvelope(ctx, r.home, r.selfProfile, env, r.now)
}

func runtimeContactView(contact contactRecord) routing.ContactRuntimeView {
	caps := []string{}
	if peerIdentity, err := derivePeerIdentity(contact.CanonicalID, contact.SigningPublicKey, contact.EncryptionPublicKey); err == nil {
		caps = append(caps, string(transport.RouteTypeDirect))
		return routing.ContactRuntimeView{
			ContactID:             contact.ContactID,
			CanonicalID:           contact.CanonicalID,
			DisplayName:           contact.DisplayName,
			PeerID:                peerIdentity.PeerID,
			TransportCapabilities: caps,
		}
	}
	if contact.RelayURL != "" {
		caps = append(caps, string(transport.RouteTypeStoreForward))
	}
	return routing.ContactRuntimeView{
		ContactID:             contact.ContactID,
		CanonicalID:           contact.CanonicalID,
		DisplayName:           contact.DisplayName,
		PeerID:                contact.RecipientID,
		TransportCapabilities: caps,
	}
}

func derivePeerIdentity(canonicalID string, signingPublicKey string, encryptionPublicKey string) (discoverylibp2p.PeerIdentity, error) {
	return discoverylibp2p.DerivePeerIdentity(discoverylibp2p.IdentityInput{
		CanonicalID:         canonicalID,
		SigningPublicKey:    signingPublicKey,
		EncryptionPublicKey: encryptionPublicKey,
	})
}

func directTransportEnabled() bool {
	return discoverylibp2p.DirectEnabledFromEnv()
}

func buildSendRuntimeBoundary(selfProfile selfMessagingProfile, contact contactRecord, now time.Time) (agentdiscovery.PeerPresenceView, []transport.Transport, []transport.RouteCandidate) {
	routes := make([]transport.RouteCandidate, 0, 2)
	transports := make([]transport.Transport, 0, 2)
	view := agentdiscovery.PeerPresenceView{
		CanonicalID: contact.CanonicalID,
		ResolvedAt:  now.UTC(),
		FreshUntil:  now.UTC().Add(5 * time.Minute),
		Source:      "runtime-send",
	}

	if directTransportEnabled() {
		session, err := discoverylibp2p.BootSession(discoverylibp2p.SessionConfig{
			Enabled:             true,
			CanonicalID:         selfProfile.CanonicalID,
			SigningPublicKey:    selfProfile.SigningPublicKey,
			EncryptionPublicKey: "",
			Now:                 now,
		})
		if err == nil && session != nil && session.Enabled {
			if contactPeer, contactErr := derivePeerIdentity(contact.CanonicalID, contact.SigningPublicKey, contact.EncryptionPublicKey); contactErr == nil {
				directDiscovery := discoverylibp2p.NewService(discoverylibp2p.PresenceConfig{
					Peer:       contactPeer,
					DirectAddress: buildDirectRouteTarget(contact.DirectURL, contact.DirectToken),
					Reachable:  true,
					ResolvedAt: now.UTC(),
				})
				if directView, resolveErr := directDiscovery.ResolvePeer(context.Background(), contact.CanonicalID); resolveErr == nil {
					view.PeerID = directView.PeerID
					view.Reachable = directView.Reachable
					view.SignedPeerRecord = directView.SignedPeerRecord
					view.TransportCapabilities = append(view.TransportCapabilities, directView.TransportCapabilities...)
					view.DirectHints = append(view.DirectHints, directView.DirectHints...)
					view.RouteCandidates = append(view.RouteCandidates, directView.RouteCandidates...)
					routes = append(routes, directView.RouteCandidates...)
					transports = append(transports, transportlibp2p.New(session))
				}
			}
		}
	}

	if contact.RelayURL != "" {
		route := transport.RouteCandidate{
			Type:     transport.RouteTypeStoreForward,
			Label:    contact.RelayURL,
			Priority: 1,
			Target:   contact.RelayURL,
		}
		routes = append(routes, route)
		view.RouteCandidates = append(view.RouteCandidates, route)
		view.TransportCapabilities = appendIfMissing(view.TransportCapabilities, string(transport.RouteTypeStoreForward))
		view.StoreForwardHints = appendIfMissing(view.StoreForwardHints, contact.RelayURL)
		if view.PeerID == "" {
			view.PeerID = contact.RecipientID
		}
		view.Reachable = true
	}

	return view, transports, routes
}

func buildDirectRouteTarget(rawURL string, token string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return rawURL
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	query := parsed.Query()
	if strings.TrimSpace(query.Get("token")) == "" {
		query.Set("token", token)
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func appendIfMissing(values []string, value string) []string {
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

func (s *Service) ensureDirectRuntimeRegistration(ctx context.Context, home string, selfProfile selfMessagingProfile, now time.Time) error {
	if !directTransportEnabled() {
		return nil
	}
	session, err := discoverylibp2p.BootSession(discoverylibp2p.SessionConfig{
		Enabled:             true,
		CanonicalID:         selfProfile.CanonicalID,
		SigningPublicKey:    selfProfile.SigningPublicKey,
		EncryptionPublicKey: "",
		ListenAddress:       buildDirectRouteTarget(selfProfile.DirectURL, selfProfile.DirectToken),
		Now:                 now,
		Receiver: directInboxReceiver{
			service:     s,
			home:        home,
			selfProfile: selfProfile,
			now:         now,
		},
	})
	if err != nil || session == nil || !session.Enabled {
		return err
	}
	discoverylibp2p.RegisterSession(session)
	store, _, err := agentruntime.OpenStore(ctx, home, now)
	if err != nil {
		return err
	}
	defer store.Close()
	selfPresence := discoverylibp2p.NewService(discoverylibp2p.PresenceConfig{
		Peer:          session.Peer,
		DirectAddress: session.ListenAddress,
		Reachable:     true,
		ResolvedAt:    now.UTC(),
	})
	if err := selfPresence.PublishSelf(ctx); err != nil {
		return err
	}
	view, err := selfPresence.ResolvePeer(ctx, selfProfile.CanonicalID)
	if err != nil {
		return err
	}
	if err := store.UpsertPresence(ctx, presenceRecordFromView(view)); err != nil {
		return err
	}
	return nil
}

func (s *Service) sendThroughRuntime(ctx context.Context, home string, selfProfile selfMessagingProfile, contact contactRecord, record MessageRecord, now time.Time) (agentruntime.SendResult, error) {
	store, _, err := agentruntime.OpenStore(ctx, home, now)
	if err != nil {
		return agentruntime.SendResult{}, err
	}
	defer store.Close()

	view, extraTransports, routes := buildSendRuntimeBoundary(selfProfile, contact, now)
	if view.CanonicalID != "" {
		if err := store.UpsertPresence(ctx, presenceRecordFromView(view)); err != nil {
			return agentruntime.SendResult{}, err
		}
	}
	runtimeSvc := agentruntime.NewService(
		staticPlanner{
			sendRoutes: routes,
			record: func(ctx context.Context, outcome routing.RouteOutcome) error {
				return store.RecordRouteAttempt(ctx, outcome, record.ConversationID, "")
			},
		},
		staticDiscoveryService{view: view},
		transportstoreforward.New(legacyStoreForwardBackend{
			service: s,
			home:    home,
			now:     now,
			record:  record,
			contact: contact,
		}),
	)
	runtimeSvc.Transports = append(extraTransports, runtimeSvc.Transports...)
	return runtimeSvc.Send(ctx, runtimeContactView(contact), agentruntime.SendRequest{
		MessageID:   record.MessageID,
		ContactRef:  contact.ContactID,
		SenderID:    selfProfile.CanonicalID,
		RecipientID: contact.RecipientID,
		Plaintext:   record.Body,
	})
}

func presenceRecordFromView(view agentdiscovery.PeerPresenceView) agentruntime.PresenceRecord {
	return agentruntime.PresenceRecord{
		CanonicalID:           view.CanonicalID,
		PeerID:                view.PeerID,
		TransportCapabilities: append([]string(nil), view.TransportCapabilities...),
		DirectHints:           append([]string(nil), view.DirectHints...),
		StoreForwardHints:     append([]string(nil), view.StoreForwardHints...),
		SignedPeerRecord:      view.SignedPeerRecord,
		Source:                view.Source,
		Reachable:             view.Reachable,
		FreshUntil:            formatOptionalTime(view.FreshUntil),
		ResolvedAt:            formatOptionalTime(view.ResolvedAt),
		AnnouncedAt:           formatOptionalTime(view.AnnouncedAt),
	}
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func syncRuntimeSendState(ctx context.Context, home string, contact contactRecord, conversation Conversation, record MessageRecord, now time.Time) error {
	db, _, err := openStateDB(ctx, home, now)
	if err != nil {
		return err
	}
	defer db.Close()

	store := agentruntime.NewStoreWithDB(db, now)
	if err := syncRuntimeSelfIdentity(ctx, db, store); err != nil {
		return err
	}

	caps := []string{}
	directHints := []string{}
	storeForwardHints := []string{}
	peerID := contact.RecipientID
	signedPeerRecord := ""
	if peerIdentity, err := derivePeerIdentity(contact.CanonicalID, contact.SigningPublicKey, contact.EncryptionPublicKey); err == nil {
		peerID = peerIdentity.PeerID
		signedPeerRecord = peerIdentity.SignedPeerRecord
		caps = append(caps, string(transport.RouteTypeDirect))
		directHints = append(directHints, "libp2p://"+peerIdentity.PeerID)
	}
	if contact.RelayURL != "" {
		caps = append(caps, string(transport.RouteTypeStoreForward))
		storeForwardHints = append(storeForwardHints, contact.RelayURL)
	}
	if err := store.UpsertContact(ctx, agentruntime.ContactRecord{
		ContactID:             contact.ContactID,
		CanonicalID:           contact.CanonicalID,
		DisplayName:           contact.DisplayName,
		PeerID:                peerID,
		SigningPublicKey:      contact.SigningPublicKey,
		EncryptionPublicKey:   contact.EncryptionPublicKey,
		TrustState:            contact.Status,
		TransportCapabilities: caps,
		DirectHints:           directHints,
		StoreForwardHints:     storeForwardHints,
		SignedPeerRecord:      signedPeerRecord,
	}); err != nil {
		return err
	}
	if err := store.UpsertConversation(ctx, agentruntime.ConversationRecord{
		ConversationID:     conversation.ConversationID,
		ContactID:          conversation.ContactID,
		LastMessageID:      record.MessageID,
		LastMessagePreview: conversation.LastMessagePreview,
		LastMessageAt:      conversation.LastMessageAt,
		UnreadCount:        conversation.UnreadCount,
	}); err != nil {
		return err
	}
	return store.UpsertMessage(ctx, agentruntime.MessageRecord{
		MessageID:         record.MessageID,
		ConversationID:    record.ConversationID,
		SenderID:          record.SenderCanonicalID,
		RecipientID:       record.RecipientContactID,
		Direction:         record.Direction,
		PlaintextBody:     record.Body,
		PlaintextPreview:  record.Preview,
		Status:            record.Status,
		SelectedRoute:     record.SelectedRoute,
		CiphertextVersion: "v0",
		CreatedAt:         record.CreatedAt,
		DeliveredAt:       record.DeliveredAt,
	})
}

func (s *Service) syncThroughRuntime(ctx context.Context, home string, selfProfile selfMessagingProfile, relayURL string, now time.Time) (agentruntime.SyncResult, error) {
	store, _, err := agentruntime.OpenStore(ctx, home, now)
	if err != nil {
		return agentruntime.SyncResult{}, err
	}
	defer store.Close()

	route := transport.RouteCandidate{
		Type:     transport.RouteTypeRecovery,
		Label:    relayURL,
		Priority: 1,
		Target:   relayURL,
	}
	runtimeSvc := agentruntime.NewService(
		staticPlanner{
			recoverRoutes: []transport.RouteCandidate{route},
			record: func(ctx context.Context, outcome routing.RouteOutcome) error {
				return store.RecordRouteAttempt(ctx, outcome, "", "")
			},
		},
		staticDiscoveryService{
			view: agentdiscovery.PeerPresenceView{
				CanonicalID:     selfProfile.CanonicalID,
				PeerID:          selfProfile.RecipientID,
				Reachable:       selfProfile.RelayURL != "",
				RouteCandidates: []transport.RouteCandidate{route},
				ResolvedAt:      now.UTC(),
				FreshUntil:      now.UTC().Add(5 * time.Minute),
			},
		},
		transportstoreforward.New(legacyStoreForwardBackend{
			service:     s,
			home:        home,
			now:         now,
			selfProfile: selfProfile,
		}),
	)
	return runtimeSvc.Sync(ctx, routing.ContactRuntimeView{
		CanonicalID:           selfProfile.CanonicalID,
		PeerID:                selfProfile.RecipientID,
		TransportCapabilities: []string{string(transport.RouteTypeRecovery)},
	})
}

func (s *Service) receiveDirectEnvelope(ctx context.Context, home string, selfProfile selfMessagingProfile, env transport.Envelope, now time.Time) error {
	db, _, err := openStateDB(ctx, home, now)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin direct message receive transaction: %w", err)
	}
	defer tx.Rollback()

	contact, err := ensureDirectIncomingContact(ctx, tx, env, now)
	if err != nil {
		return err
	}
	conversation, err := ensureConversation(ctx, tx, contact, now)
	if err != nil {
		return err
	}
	if err := insertDirectIncomingMessage(ctx, tx, conversation, contact, selfProfile, env, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit direct message receive transaction: %w", err)
	}
	return syncRuntimeRecoveredState(ctx, db, []contactRecord{contact}, []Conversation{{
		ConversationID:     conversation.ConversationID,
		ContactID:          conversation.ContactID,
		ContactDisplayName: contact.DisplayName,
		ContactCanonicalID: contact.CanonicalID,
		ContactStatus:      contact.Status,
		LastMessageAt:      now.Format(time.RFC3339Nano),
		LastMessagePreview: makePreview(env.Plaintext),
		UnreadCount:        1,
	}}, []MessageRecord{{
		MessageID:         env.MessageID,
		ConversationID:    conversation.ConversationID,
		Direction:         DirectionIncoming,
		SenderContactID:   contact.ContactID,
		SenderCanonicalID: env.SenderID,
		RecipientRouteID:  selfProfile.RecipientID,
		Body:              env.Plaintext,
		Preview:           makePreview(env.Plaintext),
		Status:            StatusDelivered,
		CreatedAt:         now.Format(time.RFC3339Nano),
		DeliveredAt:       now.Format(time.RFC3339Nano),
	}}, now)
}

func (s *Service) syncStoreForward(ctx context.Context, home string, selfProfile selfMessagingProfile, relayURL string, now time.Time) (int, string, error) {
	db, _, err := openStateDB(ctx, home, now)
	if err != nil {
		return 0, "", err
	}
	defer db.Close()
	store := agentruntime.NewStoreWithDB(db, now)

	cursor, err := store.LoadStoreForwardCursor(ctx, selfProfile.SelfID, relayURL)
	if err != nil {
		return 0, "", err
	}
	pulled, err := s.storeForwardBackend().Pull(ctx, relayURL, selfProfile.RecipientID, cursor)
	if err != nil {
		return 0, "", err
	}
	if len(pulled.Messages) == 0 {
		if err := store.SaveStoreForwardState(ctx, agentruntime.StoreForwardStateRecord{
			SelfID:             selfProfile.SelfID,
			RouteLabel:         relayURL,
			CursorValue:        pulled.NextCursor,
			LastResult:         "success",
			LastRecoveredCount: 0,
			UpdatedAt:          now.Format(time.RFC3339Nano),
		}); err != nil {
			return 0, "", err
		}
		return 0, pulled.NextCursor, nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, "", fmt.Errorf("begin message sync transaction: %w", err)
	}
	defer tx.Rollback()

	synced := 0
	contacts := make([]contactRecord, 0, len(pulled.Messages))
	conversations := make([]Conversation, 0, len(pulled.Messages))
	messages := make([]MessageRecord, 0, len(pulled.Messages))
	for _, pulledMessage := range pulled.Messages {
		plaintext, preview, err := decryptIncomingMessage(selfProfile, pulledMessage)
		if err != nil {
			return 0, "", err
		}
		contact, err := ensureIncomingContact(ctx, tx, pulledMessage, now)
		if err != nil {
			return 0, "", err
		}
		conversation, err := ensureConversation(ctx, tx, contact, now)
		if err != nil {
			return 0, "", err
		}
		if err := insertIncomingMessage(ctx, tx, conversation, contact, pulledMessage, plaintext, preview, now); err != nil {
			return 0, "", err
		}
		contacts = append(contacts, contact)
		conversation.LastMessageAt = strings.TrimSpace(pulledMessage.SentAt)
		if conversation.LastMessageAt == "" {
			conversation.LastMessageAt = now.Format(time.RFC3339Nano)
		}
		conversation.LastMessagePreview = preview
		conversation.UnreadCount++
		conversations = append(conversations, conversation)
		messages = append(messages, MessageRecord{
			MessageID:         pulledMessage.MessageID,
			ConversationID:    conversation.ConversationID,
			Direction:         DirectionIncoming,
			SenderContactID:   contact.ContactID,
			SenderCanonicalID: pulledMessage.SenderID,
			RecipientRouteID:  pulledMessage.RecipientID,
			Body:              plaintext,
			Preview:           preview,
			Status:            StatusQueued,
			CreatedAt:         conversation.LastMessageAt,
		})
		synced++
	}
	if err := saveSyncCursor(ctx, tx, selfProfile.SelfID, relayURL, pulled.NextCursor, now); err != nil {
		return 0, "", err
	}
	if err := tx.Commit(); err != nil {
		return 0, "", fmt.Errorf("commit message sync transaction: %w", err)
	}
	if err := store.SaveStoreForwardState(ctx, agentruntime.StoreForwardStateRecord{
		SelfID:             selfProfile.SelfID,
		RouteLabel:         relayURL,
		CursorValue:        pulled.NextCursor,
		LastResult:         "success",
		LastRecoveredCount: synced,
		UpdatedAt:          now.Format(time.RFC3339Nano),
	}); err != nil {
		return 0, "", err
	}
	if err := syncRuntimeRecoveredState(ctx, db, contacts, conversations, messages, now); err != nil {
		return 0, "", err
	}
	return synced, pulled.NextCursor, nil
}

func syncRuntimeRecoveredState(
	ctx context.Context,
	db *sql.DB,
	contacts []contactRecord,
	conversations []Conversation,
	messages []MessageRecord,
	now time.Time,
) error {
	store := agentruntime.NewStoreWithDB(db, now)
	if err := syncRuntimeSelfIdentity(ctx, db, store); err != nil {
		return err
	}
	for _, contact := range contacts {
		caps := []string{}
		directHints := []string{}
		storeForwardHints := []string{}
		if contact.RecipientID != "" {
			directHints = append(directHints, contact.RecipientID)
		}
		if contact.RelayURL != "" {
			caps = append(caps, string(transport.RouteTypeStoreForward))
			storeForwardHints = append(storeForwardHints, contact.RelayURL)
		}
		if err := store.UpsertContact(ctx, agentruntime.ContactRecord{
			ContactID:             contact.ContactID,
			CanonicalID:           contact.CanonicalID,
			DisplayName:           contact.DisplayName,
			PeerID:                contact.RecipientID,
			SigningPublicKey:      contact.SigningPublicKey,
			EncryptionPublicKey:   contact.EncryptionPublicKey,
			TrustState:            contact.Status,
			TransportCapabilities: caps,
			DirectHints:           directHints,
			StoreForwardHints:     storeForwardHints,
		}); err != nil {
			return err
		}
	}
	for _, conversation := range conversations {
		if err := store.UpsertConversation(ctx, agentruntime.ConversationRecord{
			ConversationID:     conversation.ConversationID,
			ContactID:          conversation.ContactID,
			LastMessagePreview: conversation.LastMessagePreview,
			LastMessageAt:      conversation.LastMessageAt,
			UnreadCount:        conversation.UnreadCount,
		}); err != nil {
			return err
		}
	}
	for _, record := range messages {
		if err := store.UpsertMessage(ctx, agentruntime.MessageRecord{
			MessageID:         record.MessageID,
			ConversationID:    record.ConversationID,
			SenderID:          record.SenderCanonicalID,
			RecipientID:       record.RecipientRouteID,
			Direction:         record.Direction,
			PlaintextBody:     record.Body,
			PlaintextPreview:  record.Preview,
			Status:            record.Status,
			CiphertextVersion: "v0",
			CreatedAt:         record.CreatedAt,
			DeliveredAt:       now.Format(time.RFC3339Nano),
		}); err != nil {
			return err
		}
	}
	return nil
}

func syncRuntimeSelfIdentity(ctx context.Context, db *sql.DB, store *agentruntime.Store) error {
	var selfID, displayName string
	if err := db.QueryRowContext(ctx, `
		SELECT self_id, display_name
		FROM self_identities
		ORDER BY created_at ASC
		LIMIT 1
	`).Scan(&selfID, &displayName); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return fmt.Errorf("query runtime self identity snapshot: %w", err)
	}

	var peerID, signingPublicKey, signingPrivateKeyRef, encryptionPublicKey, encryptionPrivateKeyRef string
	_ = db.QueryRowContext(ctx, `
		SELECT recipient_id, relay_url, encryption_public_key, encryption_private_key_ref
		FROM self_messaging_profiles
		WHERE self_id = ?
		LIMIT 1
	`, selfID).Scan(&peerID, new(string), &encryptionPublicKey, &encryptionPrivateKeyRef)
	_ = db.QueryRowContext(ctx, `
		SELECT public_key, private_key_ref
		FROM keys
		WHERE owner_type='self' AND owner_id = ? AND status='active'
		ORDER BY created_at ASC
		LIMIT 1
	`, selfID).Scan(&signingPublicKey, &signingPrivateKeyRef)

	caps := []string{string(transport.RouteTypeStoreForward), string(transport.RouteTypeRecovery)}
	if peerIdentity, err := derivePeerIdentity(selfID, signingPublicKey, encryptionPublicKey); err == nil {
		peerID = peerIdentity.PeerID
		caps = append(caps, string(transport.RouteTypeDirect))
	}

	return store.UpsertSelfIdentity(ctx, agentruntime.SelfIdentityRecord{
		SelfID:                  selfID,
		DisplayName:             displayName,
		PeerID:                  peerID,
		SigningPublicKey:        signingPublicKey,
		EncryptionPublicKey:     encryptionPublicKey,
		SigningPrivateKeyRef:    signingPrivateKeyRef,
		EncryptionPrivateKeyRef: encryptionPrivateKeyRef,
		TransportCapabilities:   caps,
	})
}

func loadRuntimeInbox(ctx context.Context, home string, now time.Time) ([]Conversation, error) {
	store, _, err := agentruntime.OpenStore(ctx, home, now)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	records, err := store.ListConversations(ctx)
	if err != nil {
		return nil, err
	}
	conversations := make([]Conversation, 0, len(records))
	for _, record := range records {
		conversations = append(conversations, Conversation{
			ConversationID:     record.ConversationID,
			ContactID:          record.ContactID,
			ContactDisplayName: record.ContactDisplayName,
			ContactCanonicalID: record.ContactCanonicalID,
			ContactStatus:      record.ContactTrustState,
			LastMessageAt:      record.LastMessageAt,
			LastMessagePreview: record.LastMessagePreview,
			UnreadCount:        record.UnreadCount,
		})
	}
	return conversations, nil
}

func loadRuntimeOutbox(ctx context.Context, home string, now time.Time) ([]MessageRecord, error) {
	store, _, err := agentruntime.OpenStore(ctx, home, now)
	if err != nil {
		return nil, err
	}
	defer store.Close()

	records, err := store.ListOutgoingMessages(ctx)
	if err != nil {
		return nil, err
	}
	messages := make([]MessageRecord, 0, len(records))
	for _, record := range records {
		messages = append(messages, MessageRecord{
			MessageID:          record.MessageID,
			ConversationID:     record.ConversationID,
			Direction:          record.Direction,
			SenderCanonicalID:  record.SenderID,
			RecipientContactID: record.RecipientID,
			Body:               record.PlaintextBody,
			Preview:            record.PlaintextPreview,
			Status:             record.Status,
			CreatedAt:          record.CreatedAt,
		})
	}
	return messages, nil
}

func loadRuntimeThread(ctx context.Context, home string, contactRef string, limit int, markRead bool, now time.Time) (Conversation, error) {
	store, _, err := agentruntime.OpenStore(ctx, home, now)
	if err != nil {
		return Conversation{}, err
	}
	defer store.Close()

	record, err := store.LoadConversationByContactRef(ctx, contactRef, limit)
	if err != nil {
		if err == sql.ErrNoRows {
			return Conversation{}, fmt.Errorf("contact %q not found; import an identity card first", contactRef)
		}
		return Conversation{}, err
	}
	if markRead && record.ConversationID != "" {
		if err := store.MarkConversationRead(ctx, record.ConversationID); err != nil {
			return Conversation{}, err
		}
		record.UnreadCount = 0
	}

	conversation := Conversation{
		ConversationID:     record.ConversationID,
		ContactID:          record.ContactID,
		ContactDisplayName: record.ContactDisplayName,
		ContactCanonicalID: record.ContactCanonicalID,
		ContactStatus:      record.ContactTrustState,
		LastMessageAt:      record.LastMessageAt,
		LastMessagePreview: record.LastMessagePreview,
		UnreadCount:        record.UnreadCount,
	}
	for _, msg := range record.Messages {
		conversation.Messages = append(conversation.Messages, MessageRecord{
			MessageID:          msg.MessageID,
			ConversationID:     msg.ConversationID,
			Direction:          msg.Direction,
			SenderCanonicalID:  msg.SenderID,
			RecipientContactID: msg.RecipientID,
			Body:               msg.PlaintextBody,
			Preview:            msg.PlaintextPreview,
			Status:             msg.Status,
			CreatedAt:          msg.CreatedAt,
		})
	}
	return conversation, nil
}
