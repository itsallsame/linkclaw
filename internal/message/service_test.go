package message

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/card"
	agentdiscovery "github.com/xiewanpeng/claw-identity/internal/discovery"
	discoverylibp2p "github.com/xiewanpeng/claw-identity/internal/discovery/libp2p"
	"github.com/xiewanpeng/claw-identity/internal/initflow"
	"github.com/xiewanpeng/claw-identity/internal/relayserver"
	"github.com/xiewanpeng/claw-identity/internal/transport"
)

func TestSendAndOutbox(t *testing.T) {
	t.Parallel()

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	initService := initflow.NewService()
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAlice",
		DisplayName: "Alice",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}

	cardService := card.NewService()
	exported, err := cardService.Export(context.Background(), card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	cardJSON, err := json.Marshal(exported.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBob",
		DisplayName: "Bob",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	imported, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  bobHome,
		Input: string(cardJSON),
	})
	if err != nil {
		t.Fatalf("import alice card into bob home: %v", err)
	}

	service := NewService()
	sent, err := service.Send(context.Background(), SendOptions{
		Home:       bobHome,
		ContactRef: imported.ContactID,
		Body:       "hello alice",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if sent.Message.Status != StatusPending {
		t.Fatalf("message status = %q, want %q", sent.Message.Status, StatusPending)
	}
	if sent.Message.TransportStatus != TransportStatusDeferred {
		t.Fatalf("transport status = %q, want %q", sent.Message.TransportStatus, TransportStatusDeferred)
	}

	outbox, err := service.Outbox(context.Background(), ListOptions{Home: bobHome})
	if err != nil {
		t.Fatalf("load outbox: %v", err)
	}
	if len(outbox.Messages) != 1 {
		t.Fatalf("outbox messages = %d, want 1", len(outbox.Messages))
	}
	if outbox.Messages[0].Body != "hello alice" {
		t.Fatalf("outbox body = %q", outbox.Messages[0].Body)
	}
	if outbox.Messages[0].TransportStatus != TransportStatusDeferred {
		t.Fatalf("outbox transport status = %q, want %q", outbox.Messages[0].TransportStatus, TransportStatusDeferred)
	}

	inbox, err := service.Inbox(context.Background(), ListOptions{Home: bobHome})
	if err != nil {
		t.Fatalf("load inbox conversations: %v", err)
	}
	if len(inbox.Conversations) != 1 {
		t.Fatalf("inbox conversations = %d, want 1", len(inbox.Conversations))
	}
	if inbox.Conversations[0].ContactDisplayName != "Alice" {
		t.Fatalf("conversation display name = %q, want Alice", inbox.Conversations[0].ContactDisplayName)
	}
}

func TestSendAndSyncWithRelay(t *testing.T) {
	relay, relayResult, err := relayserver.Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Shutdown(context.Background())

	t.Setenv(card.EnvRelayURL, relayResult.URL)

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	initService := initflow.NewService()
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAlice",
		DisplayName: "Alice",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}
	cardService := card.NewService()
	aliceCard, err := cardService.Export(context.Background(), card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	aliceCardJSON, err := json.Marshal(aliceCard.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBob",
		DisplayName: "Bob",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	bobCard, err := cardService.Export(context.Background(), card.Options{Home: bobHome})
	if err != nil {
		t.Fatalf("export bob card: %v", err)
	}
	bobCardJSON, err := json.Marshal(bobCard.Card)
	if err != nil {
		t.Fatalf("marshal bob card: %v", err)
	}

	aliceImportedIntoBob, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  bobHome,
		Input: string(aliceCardJSON),
	})
	if err != nil {
		t.Fatalf("import alice into bob: %v", err)
	}
	if _, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  aliceHome,
		Input: string(bobCardJSON),
	}); err != nil {
		t.Fatalf("import bob into alice: %v", err)
	}

	service := NewService()
	sent, err := service.Send(context.Background(), SendOptions{
		Home:       bobHome,
		ContactRef: aliceImportedIntoBob.ContactID,
		Body:       "hello from bob",
	})
	if err != nil {
		t.Fatalf("send message through relay: %v", err)
	}
	if sent.Message.Status != StatusQueued {
		t.Fatalf("message status = %q, want %q", sent.Message.Status, StatusQueued)
	}
	if sent.Message.TransportStatus != TransportStatusDeferred {
		t.Fatalf("transport status = %q, want %q", sent.Message.TransportStatus, TransportStatusDeferred)
	}

	synced, err := service.Sync(context.Background(), SyncOptions{Home: aliceHome})
	if err != nil {
		t.Fatalf("sync alice inbox: %v", err)
	}
	if synced.Synced != 1 {
		t.Fatalf("synced count = %d, want 1", synced.Synced)
	}

	inbox, err := service.Inbox(context.Background(), ListOptions{Home: aliceHome})
	if err != nil {
		t.Fatalf("load alice inbox: %v", err)
	}
	if len(inbox.Conversations) != 1 {
		t.Fatalf("inbox conversations = %d, want 1", len(inbox.Conversations))
	}
	if inbox.Conversations[0].ContactCanonicalID != "did:key:z6MkBob" {
		t.Fatalf("incoming contact canonical id = %q, want bob", inbox.Conversations[0].ContactCanonicalID)
	}
	if inbox.Conversations[0].LastMessagePreview != "hello from bob" {
		t.Fatalf("incoming preview = %q", inbox.Conversations[0].LastMessagePreview)
	}

	thread, err := service.Thread(context.Background(), ThreadOptions{
		Home:       aliceHome,
		ContactRef: "did:key:z6MkBob",
		Limit:      10,
		MarkRead:   true,
	})
	if err != nil {
		t.Fatalf("load alice thread: %v", err)
	}
	if len(thread.Conversation.Messages) != 1 {
		t.Fatalf("thread messages = %d, want 1", len(thread.Conversation.Messages))
	}
	if thread.Conversation.Messages[0].Body != "hello from bob" {
		t.Fatalf("thread body = %q, want %q", thread.Conversation.Messages[0].Body, "hello from bob")
	}
	if thread.Conversation.Messages[0].TransportStatus != TransportStatusRecovered {
		t.Fatalf("thread transport status = %q, want %q", thread.Conversation.Messages[0].TransportStatus, TransportStatusRecovered)
	}
	if thread.Conversation.UnreadCount != 0 {
		t.Fatalf("thread unread count = %d, want 0", thread.Conversation.UnreadCount)
	}

	inboxAfterThread, err := service.Inbox(context.Background(), ListOptions{Home: aliceHome})
	if err != nil {
		t.Fatalf("load alice inbox after thread: %v", err)
	}
	if inboxAfterThread.Conversations[0].UnreadCount != 0 {
		t.Fatalf("inbox unread after thread = %d, want 0", inboxAfterThread.Conversations[0].UnreadCount)
	}
}

func TestSendAndSyncWithRelayAndExperimentalDirectFallback(t *testing.T) {
	relay, relayResult, err := relayserver.Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Shutdown(context.Background())

	t.Setenv(card.EnvRelayURL, relayResult.URL)
	t.Setenv(discoverylibp2p.EnvExperimentalDirect, "1")

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	initService := initflow.NewService()
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAlice",
		DisplayName: "Alice",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}
	cardService := card.NewService()
	aliceCard, err := cardService.Export(context.Background(), card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	aliceCardJSON, err := json.Marshal(aliceCard.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBob",
		DisplayName: "Bob",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	bobCard, err := cardService.Export(context.Background(), card.Options{Home: bobHome})
	if err != nil {
		t.Fatalf("export bob card: %v", err)
	}
	bobCardJSON, err := json.Marshal(bobCard.Card)
	if err != nil {
		t.Fatalf("marshal bob card: %v", err)
	}

	aliceImportedIntoBob, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  bobHome,
		Input: string(aliceCardJSON),
	})
	if err != nil {
		t.Fatalf("import alice into bob: %v", err)
	}
	if _, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  aliceHome,
		Input: string(bobCardJSON),
	}); err != nil {
		t.Fatalf("import bob into alice: %v", err)
	}

	service := NewService()
	sent, err := service.Send(context.Background(), SendOptions{
		Home:       bobHome,
		ContactRef: aliceImportedIntoBob.ContactID,
		Body:       "hello from bob with direct fallback",
	})
	if err != nil {
		t.Fatalf("send message through relay with direct fallback: %v", err)
	}
	if sent.Message.Status != StatusQueued {
		t.Fatalf("message status = %q, want %q", sent.Message.Status, StatusQueued)
	}
	if sent.Message.TransportStatus != TransportStatusDeferred {
		t.Fatalf("transport status = %q, want %q", sent.Message.TransportStatus, TransportStatusDeferred)
	}

	synced, err := service.Sync(context.Background(), SyncOptions{Home: aliceHome})
	if err != nil {
		t.Fatalf("sync alice inbox: %v", err)
	}
	if synced.Synced != 1 {
		t.Fatalf("synced count = %d, want 1", synced.Synced)
	}

	bobAttempts := loadRuntimeRouteAttempts(t, bobHome)
	requireRouteAttempt(t, bobAttempts, "direct", "failed", "")
	requireRouteAttempt(t, bobAttempts, "store_forward", "queued", "")

	aliceAttempts := loadRuntimeRouteAttempts(t, aliceHome)
	recoveryCursor := requireRouteAttemptWithNonEmptyCursor(t, aliceAttempts, "recovery", "recovered")
	requireRouteAttempt(t, aliceAttempts, "recovery", "acked", recoveryCursor)
}

func TestConnectPeerSupportsDiscoveryRecordWithoutContact(t *testing.T) {
	initService := initflow.NewService()

	selfHome := filepath.Join(t.TempDir(), "self-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        selfHome,
		CanonicalID: "did:key:z6MkConnectSelf",
		DisplayName: "Connect Self",
	}); err != nil {
		t.Fatalf("init self home: %v", err)
	}

	peerCanonicalID := "did:key:z6MkConnectPeer"
	relayURL := "https://relay.example"

	db, err := sql.Open("sqlite", filepath.Join(selfHome, "state.db"))
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	routesJSON, err := json.Marshal([]transport.RouteCandidate{
		{
			Type:     transport.RouteTypeStoreForward,
			Label:    relayURL,
			Priority: 1,
			Target:   relayURL,
		},
	})
	if err != nil {
		t.Fatalf("marshal discovery routes: %v", err)
	}
	capsJSON, err := json.Marshal([]string{string(transport.RouteTypeStoreForward)})
	if err != nil {
		t.Fatalf("marshal discovery capabilities: %v", err)
	}
	storeForwardHintsJSON, err := json.Marshal([]string{relayURL})
	if err != nil {
		t.Fatalf("marshal discovery store-forward hints: %v", err)
	}
	stamp := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(
		`INSERT INTO runtime_discovery_records (
			canonical_id, peer_id, route_candidates_json, transport_capabilities_json, direct_hints_json,
			store_forward_hints_json, signed_peer_record, source, reachable, resolved_at, fresh_until,
			announced_at, updated_at, created_at
		) VALUES (?, '', ?, ?, '[]', ?, '', 'fixture', 1, ?, ?, '', ?, ?)`,
		peerCanonicalID,
		string(routesJSON),
		string(capsJSON),
		string(storeForwardHintsJSON),
		stamp,
		stamp,
		stamp,
		stamp,
	); err != nil {
		t.Fatalf("insert discovery record: %v", err)
	}

	var contactCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM contacts WHERE canonical_id = ?`, peerCanonicalID).Scan(&contactCount); err != nil {
		t.Fatalf("count contacts: %v", err)
	}
	if contactCount != 0 {
		t.Fatalf("contacts for canonical_id = %d, want 0", contactCount)
	}

	service := NewService()
	result, err := service.ConnectPeer(context.Background(), ConnectPeerOptions{
		Home:    selfHome,
		PeerRef: peerCanonicalID,
	})
	if err != nil {
		t.Fatalf("connect peer from discovery: %v", err)
	}
	if got, want := result.CanonicalID, peerCanonicalID; got != want {
		t.Fatalf("connect canonical id = %q, want %q", got, want)
	}
	if !result.Connected {
		t.Fatalf("connect result connected = false, want true; result=%+v", result)
	}
	if got, want := result.Transport, "store_forward_ready"; got != want {
		t.Fatalf("connect transport = %q, want %q", got, want)
	}
}

func TestConnectPeerRefreshDistinguishesStaleAndFreshPresence(t *testing.T) {
	initService := initflow.NewService()
	cardService := card.NewService()

	selfHome := filepath.Join(t.TempDir(), "self-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        selfHome,
		CanonicalID: "did:key:z6MkRefreshSelf",
		DisplayName: "Refresh Self",
	}); err != nil {
		t.Fatalf("init self home: %v", err)
	}

	peerHome := filepath.Join(t.TempDir(), "peer-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        peerHome,
		CanonicalID: "did:key:z6MkRefreshPeer",
		DisplayName: "Refresh Peer",
	}); err != nil {
		t.Fatalf("init peer home: %v", err)
	}
	exportedPeer, err := cardService.Export(context.Background(), card.Options{Home: peerHome})
	if err != nil {
		t.Fatalf("export peer card: %v", err)
	}
	peerCardJSON, err := json.Marshal(exportedPeer.Card)
	if err != nil {
		t.Fatalf("marshal peer card: %v", err)
	}
	imported, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  selfHome,
		Input: string(peerCardJSON),
	})
	if err != nil {
		t.Fatalf("import peer card: %v", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(selfHome, "state.db"))
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	relayURL := "https://relay.refresh.example"
	if _, err := db.Exec(
		`UPDATE contacts SET relay_url = ?, recipient_id = ?, direct_url = '', direct_token = '' WHERE contact_id = ?`,
		relayURL,
		"refresh-peer-recipient",
		imported.ContactID,
	); err != nil {
		t.Fatalf("update contact relay hint: %v", err)
	}

	staleNow := time.Now().UTC().Add(-2 * time.Hour)
	discoveryStore := agentdiscovery.NewStoreWithDB(db, staleNow)
	if err := discoveryStore.Upsert(context.Background(), agentdiscovery.Record{
		CanonicalID: imported.Card.ID,
		PeerID:      "stale-peer",
		Source:      "stale-cache",
		Reachable:   false,
		ResolvedAt:  staleNow.Format(time.RFC3339Nano),
		FreshUntil:  staleNow.Add(-30 * time.Minute).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("upsert stale discovery record: %v", err)
	}

	service := NewService()
	staleResult, err := service.ConnectPeer(context.Background(), ConnectPeerOptions{
		Home:    selfHome,
		PeerRef: imported.ContactID,
	})
	if err != nil {
		t.Fatalf("connect stale presence: %v", err)
	}
	if staleResult.Connected {
		t.Fatalf("stale connect connected = true, want false; result=%+v", staleResult)
	}
	if got := len(staleResult.Routes); got != 0 {
		t.Fatalf("stale connect routes = %d, want 0", got)
	}
	if got, want := staleResult.Presence.Source, "cache"; got != want {
		t.Fatalf("stale connect source = %q, want %q", got, want)
	}

	freshResult, err := service.ConnectPeer(context.Background(), ConnectPeerOptions{
		Home:    selfHome,
		PeerRef: imported.ContactID,
		Refresh: true,
	})
	if err != nil {
		t.Fatalf("connect refresh presence: %v", err)
	}
	if !freshResult.Connected {
		t.Fatalf("refresh connect connected = false, want true; result=%+v", freshResult)
	}
	if got, want := freshResult.Transport, "store_forward_ready"; got != want {
		t.Fatalf("refresh connect transport = %q, want %q", got, want)
	}
	if !freshResult.Presence.ResolvedAt.After(staleResult.Presence.ResolvedAt) {
		t.Fatalf("refresh resolved_at = %s, stale resolved_at = %s; want refresh newer", freshResult.Presence.ResolvedAt, staleResult.Presence.ResolvedAt)
	}
	foundStoreForward := false
	for _, route := range freshResult.Routes {
		if route.Type == transport.RouteTypeStoreForward && route.Target == relayURL {
			foundStoreForward = true
			break
		}
	}
	if !foundStoreForward {
		t.Fatalf("refresh routes = %#v, want store-forward route to %q", freshResult.Routes, relayURL)
	}

	updatedStore := agentdiscovery.NewStoreWithDB(db, time.Now().UTC())
	record, ok, err := updatedStore.Get(context.Background(), imported.Card.ID)
	if err != nil {
		t.Fatalf("load refreshed discovery record: %v", err)
	}
	if !ok {
		t.Fatalf("refreshed discovery record missing for %q", imported.Card.ID)
	}
	if record.Source == "stale-cache" {
		t.Fatalf("refreshed discovery source = %q, want non-stale source", record.Source)
	}
	if !hasStoreForwardRoute(record.RouteCandidates, relayURL) {
		t.Fatalf("refreshed discovery routes = %#v, want store-forward route to %q", record.RouteCandidates, relayURL)
	}
}

func TestStatusSummary(t *testing.T) {
	relay, relayResult, err := relayserver.Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Shutdown(context.Background())

	t.Setenv(card.EnvRelayURL, relayResult.URL)

	home := filepath.Join(t.TempDir(), "status-home")
	initService := initflow.NewService()
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        home,
		CanonicalID: "did:key:z6MkStatusSummary",
		DisplayName: "StatusSummary",
	}); err != nil {
		t.Fatalf("init home: %v", err)
	}

	service := NewService()
	result, err := service.Status(context.Background(), ListOptions{Home: home})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if result.DisplayName != "StatusSummary" {
		t.Fatalf("display name = %q, want StatusSummary", result.DisplayName)
	}
	if result.StoreForwardRoutes != 0 {
		t.Fatalf("store forward routes = %d, want 0", result.StoreForwardRoutes)
	}
	if !result.IdentityReady || !result.TransportReady {
		t.Fatalf("expected identity/transport ready, got identity=%t transport=%t", result.IdentityReady, result.TransportReady)
	}
	if result.Contacts != 0 || result.Conversations != 0 || result.Unread != 0 {
		t.Fatalf("unexpected counts: %+v", result)
	}
	if len(result.TransportCapabilities) == 0 {
		t.Fatalf("expected transport capabilities, got none")
	}
	if result.DiscoveryReady {
		t.Fatalf("discovery ready = %t, want false without peer presence", result.DiscoveryReady)
	}
}

func TestStatusSummaryDiscoveryReadyRequiresPeerPresence(t *testing.T) {
	relay, relayResult, err := relayserver.Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Shutdown(context.Background())

	t.Setenv(card.EnvRelayURL, relayResult.URL)
	t.Setenv(discoverylibp2p.EnvExperimentalDirect, "1")

	home := filepath.Join(t.TempDir(), "status-discovery-ready-home")
	initService := initflow.NewService()
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        home,
		CanonicalID: "did:key:z6MkStatusDiscoveryReady",
		DisplayName: "Status Discovery Ready",
	}); err != nil {
		t.Fatalf("init home: %v", err)
	}

	service := NewService()
	initial, err := service.Status(context.Background(), ListOptions{Home: home})
	if err != nil {
		t.Fatalf("initial status: %v", err)
	}
	if !initial.IdentityReady || !initial.TransportReady {
		t.Fatalf("expected identity/transport ready, got identity=%t transport=%t", initial.IdentityReady, initial.TransportReady)
	}
	if initial.DiscoveryReady {
		t.Fatalf("initial discovery ready = %t, want false without peer discovery", initial.DiscoveryReady)
	}
	if initial.PresenceEntries != 0 || initial.ReachablePresence != 0 {
		t.Fatalf("initial peer presence counters = (%d,%d), want (0,0)", initial.PresenceEntries, initial.ReachablePresence)
	}

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := db.Exec(`
		INSERT INTO runtime_presence_cache (
			canonical_id, peer_id, source, reachable, fresh_until, resolved_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`,
		"did:key:z6MkStatusPeerPresence",
		"peer-status-ready",
		"refresh",
		1,
		now,
		now,
	); err != nil {
		t.Fatalf("insert peer presence: %v", err)
	}

	withPeerPresence, err := service.Status(context.Background(), ListOptions{Home: home})
	if err != nil {
		t.Fatalf("status with peer presence: %v", err)
	}
	if !withPeerPresence.DiscoveryReady {
		t.Fatalf("discovery ready = %t, want true with peer presence", withPeerPresence.DiscoveryReady)
	}
	if withPeerPresence.PresenceEntries < 1 || withPeerPresence.ReachablePresence < 1 {
		t.Fatalf("peer presence counters = (%d,%d), want >=1", withPeerPresence.PresenceEntries, withPeerPresence.ReachablePresence)
	}
}

func TestStatusSummaryTracksRecoveryState(t *testing.T) {
	relay, relayResult, err := relayserver.Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Shutdown(context.Background())

	t.Setenv(card.EnvRelayURL, relayResult.URL)

	initService := initflow.NewService()
	cardService := card.NewService()

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAliceStatusRecovery",
		DisplayName: "Alice Status Recovery",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}
	aliceCard, err := cardService.Export(context.Background(), card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	aliceCardJSON, err := json.Marshal(aliceCard.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBobStatusRecovery",
		DisplayName: "Bob Status Recovery",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	bobCard, err := cardService.Export(context.Background(), card.Options{Home: bobHome})
	if err != nil {
		t.Fatalf("export bob card: %v", err)
	}
	bobCardJSON, err := json.Marshal(bobCard.Card)
	if err != nil {
		t.Fatalf("marshal bob card: %v", err)
	}

	aliceImportedIntoBob, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  bobHome,
		Input: string(aliceCardJSON),
	})
	if err != nil {
		t.Fatalf("import alice into bob: %v", err)
	}
	if _, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  aliceHome,
		Input: string(bobCardJSON),
	}); err != nil {
		t.Fatalf("import bob into alice: %v", err)
	}

	service := NewService()
	if _, err := service.Send(context.Background(), SendOptions{
		Home:       bobHome,
		ContactRef: aliceImportedIntoBob.ContactID,
		Body:       "hello with recovery state",
	}); err != nil {
		t.Fatalf("send: %v", err)
	}
	if _, err := service.Sync(context.Background(), SyncOptions{Home: aliceHome}); err != nil {
		t.Fatalf("sync: %v", err)
	}

	status, err := service.Status(context.Background(), ListOptions{Home: aliceHome})
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status.Conversations != 1 || status.Unread != 1 {
		t.Fatalf("unexpected conversation counters: %+v", status)
	}
	if status.StoreForwardRoutes != 1 {
		t.Fatalf("store forward routes = %d, want 1", status.StoreForwardRoutes)
	}
	if status.LastRecoveredCount != 1 {
		t.Fatalf("last recovered count = %d, want 1", status.LastRecoveredCount)
	}
	if status.LastStoreForwardResult != "success" {
		t.Fatalf("last store-forward result = %q, want success", status.LastStoreForwardResult)
	}
	if status.MessageStatusRecovered < 1 {
		t.Fatalf("message recovered counter = %d, want >= 1", status.MessageStatusRecovered)
	}
	if len(status.RecentRouteOutcomes) == 0 {
		t.Fatal("recent route outcomes should not be empty after sync")
	}
}

func TestDirectDeliveryWhenBothHostsAreOnline(t *testing.T) {
	relay, relayResult, err := relayserver.Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Shutdown(context.Background())

	t.Setenv(card.EnvRelayURL, relayResult.URL)
	t.Setenv(discoverylibp2p.EnvExperimentalDirect, "1")

	initService := initflow.NewService()
	cardService := card.NewService()

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAliceDirect",
		DisplayName: "Alice Direct",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}
	aliceCard, err := cardService.Export(context.Background(), card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	aliceCardJSON, err := json.Marshal(aliceCard.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBobDirect",
		DisplayName: "Bob Direct",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	bobCard, err := cardService.Export(context.Background(), card.Options{Home: bobHome})
	if err != nil {
		t.Fatalf("export bob card: %v", err)
	}
	bobCardJSON, err := json.Marshal(bobCard.Card)
	if err != nil {
		t.Fatalf("marshal bob card: %v", err)
	}

	aliceImportedIntoBob, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  bobHome,
		Input: string(aliceCardJSON),
	})
	if err != nil {
		t.Fatalf("import alice into bob: %v", err)
	}
	if _, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  aliceHome,
		Input: string(bobCardJSON),
	}); err != nil {
		t.Fatalf("import bob into alice: %v", err)
	}

	aliceService := NewService()
	if _, err := aliceService.Status(context.Background(), ListOptions{Home: aliceHome}); err != nil {
		t.Fatalf("prime alice direct runtime: %v", err)
	}

	bobService := NewService()
	sent, err := bobService.Send(context.Background(), SendOptions{
		Home:       bobHome,
		ContactRef: aliceImportedIntoBob.ContactID,
		Body:       "hello direct online",
	})
	if err != nil {
		t.Fatalf("direct send: %v", err)
	}
	if sent.Message.Status != StatusDelivered {
		t.Fatalf("message status = %q, want %q", sent.Message.Status, StatusDelivered)
	}
	if sent.Message.TransportStatus != TransportStatusDirect {
		t.Fatalf("transport status = %q, want %q", sent.Message.TransportStatus, TransportStatusDirect)
	}

	inbox, err := aliceService.Inbox(context.Background(), ListOptions{Home: aliceHome})
	if err != nil {
		t.Fatalf("alice inbox: %v", err)
	}
	if len(inbox.Conversations) != 1 {
		t.Fatalf("inbox conversations = %d, want 1", len(inbox.Conversations))
	}
	if inbox.Conversations[0].LastMessagePreview != "hello direct online" {
		t.Fatalf("preview = %q, want %q", inbox.Conversations[0].LastMessagePreview, "hello direct online")
	}
	if inbox.Conversations[0].UnreadCount != 1 {
		t.Fatalf("unread = %d, want 1", inbox.Conversations[0].UnreadCount)
	}
}

func TestDirectDeliveryPreservesMultipleMessages(t *testing.T) {
	relay, relayResult, err := relayserver.Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Shutdown(context.Background())

	t.Setenv(card.EnvRelayURL, relayResult.URL)
	t.Setenv(discoverylibp2p.EnvExperimentalDirect, "1")

	initService := initflow.NewService()
	cardService := card.NewService()

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAliceDirectMulti",
		DisplayName: "Alice Direct Multi",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}
	aliceCard, err := cardService.Export(context.Background(), card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	aliceCardJSON, err := json.Marshal(aliceCard.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBobDirectMulti",
		DisplayName: "Bob Direct Multi",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	bobCard, err := cardService.Export(context.Background(), card.Options{Home: bobHome})
	if err != nil {
		t.Fatalf("export bob card: %v", err)
	}
	bobCardJSON, err := json.Marshal(bobCard.Card)
	if err != nil {
		t.Fatalf("marshal bob card: %v", err)
	}

	aliceImportedIntoBob, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  bobHome,
		Input: string(aliceCardJSON),
	})
	if err != nil {
		t.Fatalf("import alice into bob: %v", err)
	}
	if _, err := cardService.Import(context.Background(), card.ImportOptions{
		Home:  aliceHome,
		Input: string(bobCardJSON),
	}); err != nil {
		t.Fatalf("import bob into alice: %v", err)
	}

	aliceService := NewService()
	if _, err := aliceService.Status(context.Background(), ListOptions{Home: aliceHome}); err != nil {
		t.Fatalf("prime alice direct runtime: %v", err)
	}

	bobService := NewService()
	first, err := bobService.Send(context.Background(), SendOptions{
		Home:       bobHome,
		ContactRef: aliceImportedIntoBob.ContactID,
		Body:       "hello direct first",
	})
	if err != nil {
		t.Fatalf("first direct send: %v", err)
	}
	second, err := bobService.Send(context.Background(), SendOptions{
		Home:       bobHome,
		ContactRef: aliceImportedIntoBob.ContactID,
		Body:       "hello direct second",
	})
	if err != nil {
		t.Fatalf("second direct send: %v", err)
	}
	if first.Message.MessageID == second.Message.MessageID {
		t.Fatalf("message ids collided: %q", first.Message.MessageID)
	}

	thread, err := aliceService.Thread(context.Background(), ThreadOptions{
		Home:       aliceHome,
		ContactRef: "did:key:z6MkBobDirectMulti",
		Limit:      10,
		MarkRead:   false,
	})
	if err != nil {
		t.Fatalf("alice thread: %v", err)
	}
	if len(thread.Conversation.Messages) != 2 {
		t.Fatalf("thread messages = %d, want 2", len(thread.Conversation.Messages))
	}
	bodies := map[string]bool{}
	for _, msg := range thread.Conversation.Messages {
		bodies[msg.Body] = true
		if msg.TransportStatus != TransportStatusDirect {
			t.Fatalf("thread message transport status = %q, want %q", msg.TransportStatus, TransportStatusDirect)
		}
	}
	if !bodies["hello direct first"] || !bodies["hello direct second"] {
		t.Fatalf("thread bodies = %#v", thread.Conversation.Messages)
	}
}

type runtimeRouteAttempt struct {
	RouteType string
	Outcome   string
	Cursor    string
}

func loadRuntimeRouteAttempts(t *testing.T, home string) []runtimeRouteAttempt {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT route_type, outcome, cursor_value FROM runtime_route_attempts`)
	if err != nil {
		t.Fatalf("query runtime_route_attempts: %v", err)
	}
	defer rows.Close()

	attempts := make([]runtimeRouteAttempt, 0)
	for rows.Next() {
		var record runtimeRouteAttempt
		if err := rows.Scan(&record.RouteType, &record.Outcome, &record.Cursor); err != nil {
			t.Fatalf("scan runtime_route_attempts: %v", err)
		}
		attempts = append(attempts, record)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate runtime_route_attempts: %v", err)
	}
	return attempts
}

func requireRouteAttempt(t *testing.T, attempts []runtimeRouteAttempt, routeType, outcome, cursor string) {
	t.Helper()
	for _, attempt := range attempts {
		if attempt.RouteType == routeType && attempt.Outcome == outcome && attempt.Cursor == cursor {
			return
		}
	}
	t.Fatalf("route attempt (%s,%s,%s) not found, got %#v", routeType, outcome, cursor, attempts)
}

func requireRouteAttemptWithNonEmptyCursor(t *testing.T, attempts []runtimeRouteAttempt, routeType, outcome string) string {
	t.Helper()
	for _, attempt := range attempts {
		if attempt.RouteType == routeType && attempt.Outcome == outcome && attempt.Cursor != "" {
			return attempt.Cursor
		}
	}
	t.Fatalf("route attempt (%s,%s,<non-empty cursor>) not found, got %#v", routeType, outcome, attempts)
	return ""
}

func hasStoreForwardRoute(routes []transport.RouteCandidate, target string) bool {
	for _, route := range routes {
		if route.Type == transport.RouteTypeStoreForward && route.Target == target {
			return true
		}
	}
	return false
}
