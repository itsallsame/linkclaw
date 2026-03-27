package message

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/xiewanpeng/claw-identity/internal/card"
	agentdiscovery "github.com/xiewanpeng/claw-identity/internal/discovery"
	discoverylibp2p "github.com/xiewanpeng/claw-identity/internal/discovery/libp2p"
	"github.com/xiewanpeng/claw-identity/internal/initflow"
	agentruntime "github.com/xiewanpeng/claw-identity/internal/runtime"
	"github.com/xiewanpeng/claw-identity/internal/transport"
	transportnostr "github.com/xiewanpeng/claw-identity/internal/transport/nostr"
	transportstoreforward "github.com/xiewanpeng/claw-identity/internal/transport/storeforward"
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
	if !result.Promotion.ContactCreated {
		t.Fatalf("connect promotion contact_created = false, want true; promotion=%+v", result.Promotion)
	}
	if result.Promotion.ContactID == "" {
		t.Fatalf("connect promotion contact_id is empty")
	}
	if !result.Promotion.TrustLinked {
		t.Fatalf("connect promotion trust_linked = false, want true")
	}
	if result.Promotion.NoteWritten {
		t.Fatalf("connect promotion note_written = true, want false")
	}
	if result.Promotion.PinWritten {
		t.Fatalf("connect promotion pin_written = true, want false")
	}
	if result.Promotion.EventID == "" {
		t.Fatalf("connect promotion event_id is empty")
	}

	var promotedContactID string
	var promotedStatus string
	if err := db.QueryRow(
		`SELECT contact_id, status FROM contacts WHERE canonical_id = ?`,
		peerCanonicalID,
	).Scan(&promotedContactID, &promotedStatus); err != nil {
		t.Fatalf("load promoted contact: %v", err)
	}
	if promotedContactID != result.Promotion.ContactID {
		t.Fatalf("promoted contact id = %q, want %q", promotedContactID, result.Promotion.ContactID)
	}
	if promotedStatus == "" {
		t.Fatalf("promoted contact status is empty")
	}

	var trustCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM trust_records WHERE contact_id = ?`, promotedContactID).Scan(&trustCount); err != nil {
		t.Fatalf("count trust records: %v", err)
	}
	if trustCount != 1 {
		t.Fatalf("trust_records count = %d, want 1", trustCount)
	}

	var runtimeTrustContactID string
	if err := db.QueryRow(
		`SELECT contact_id FROM runtime_trust_records WHERE canonical_id = ?`,
		peerCanonicalID,
	).Scan(&runtimeTrustContactID); err != nil {
		t.Fatalf("load runtime trust record: %v", err)
	}
	if runtimeTrustContactID != promotedContactID {
		t.Fatalf("runtime trust contact_id = %q, want %q", runtimeTrustContactID, promotedContactID)
	}

	var connectEventCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM interaction_events WHERE contact_id = ? AND event_type = 'connect'`,
		promotedContactID,
	).Scan(&connectEventCount); err != nil {
		t.Fatalf("count connect events: %v", err)
	}
	if connectEventCount != 1 {
		t.Fatalf("connect event count = %d, want 1", connectEventCount)
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
	if !staleResult.Connected {
		t.Fatalf("stale connect connected = false, want true; result=%+v", staleResult)
	}
	if got, want := staleResult.Transport, "store_forward_ready"; got != want {
		t.Fatalf("stale connect transport = %q, want %q", got, want)
	}
	if got, want := staleResult.Presence.Source, "cache"; got != want {
		t.Fatalf("stale connect source = %q, want %q", got, want)
	}
	if !hasStoreForwardRoute(staleResult.Routes, relayURL) {
		t.Fatalf("stale routes = %#v, want store-forward route to %q", staleResult.Routes, relayURL)
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

func TestDirectDeliveryWhenBothHostsAreOnline(t *testing.T) {
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

func TestRuntimeContactViewPreservesHTTPDirectHints(t *testing.T) {
	contact := contactRecord{
		ContactID:           "contact_http_direct",
		CanonicalID:         "did:key:z6MkHTTPDirect",
		DisplayName:         "HTTP Direct",
		RecipientID:         "rcpt_http_direct",
		SigningPublicKey:    "signing-key",
		EncryptionPublicKey: "enc-key",
		DirectURL:           "http://127.0.0.1:8907/plugins/linkclaw/direct",
		DirectToken:         "shared-secret",
		RelayURL:            "https://relay.example",
	}

	view := runtimeContactView(contact)

	if view.DirectURL != contact.DirectURL {
		t.Fatalf("DirectURL = %q, want %q", view.DirectURL, contact.DirectURL)
	}
	if view.DirectToken != contact.DirectToken {
		t.Fatalf("DirectToken = %q, want %q", view.DirectToken, contact.DirectToken)
	}
	if view.RecipientID != contact.RecipientID {
		t.Fatalf("RecipientID = %q, want %q", view.RecipientID, contact.RecipientID)
	}
	if !slices.Contains(view.TransportCapabilities, string(transport.RouteTypeDirect)) {
		t.Fatalf("TransportCapabilities = %#v, want direct", view.TransportCapabilities)
	}
	if !slices.Contains(view.TransportCapabilities, string(transport.RouteTypeStoreForward)) {
		t.Fatalf("TransportCapabilities = %#v, want store_forward", view.TransportCapabilities)
	}
	directTarget := buildDirectRouteTarget(contact.DirectURL, contact.DirectToken)
	if !slices.Contains(view.DirectHints, directTarget) {
		t.Fatalf("DirectHints = %#v, want %q", view.DirectHints, directTarget)
	}
	if !slices.Contains(view.StoreForwardHints, contact.RelayURL) {
		t.Fatalf("StoreForwardHints = %#v, want %q", view.StoreForwardHints, contact.RelayURL)
	}
}

func TestBuildSendRuntimeBoundaryUsesHTTPDirectWithoutExperimentalDirect(t *testing.T) {
	contact := contactRecord{
		ContactID:           "contact_http_direct",
		CanonicalID:         "did:key:z6MkHTTPDirect",
		DisplayName:         "HTTP Direct",
		RecipientID:         "rcpt_http_direct",
		SigningPublicKey:    "signing-key",
		EncryptionPublicKey: "enc-key",
		DirectURL:           "http://127.0.0.1:8907/plugins/linkclaw/direct",
		DirectToken:         "shared-secret",
	}
	selfProfile := selfMessagingProfile{
		CanonicalID:      "did:key:z6MkSelf",
		SigningPublicKey: "self-signing-key",
	}

	view, transports, routes := buildSendRuntimeBoundary(selfProfile, contact, time.Now().UTC())
	directTarget := buildDirectRouteTarget(contact.DirectURL, contact.DirectToken)

	if !slices.Contains(view.DirectHints, directTarget) {
		t.Fatalf("DirectHints = %#v, want %q", view.DirectHints, directTarget)
	}
	if !slices.Contains(view.TransportCapabilities, string(transport.RouteTypeDirect)) {
		t.Fatalf("TransportCapabilities = %#v, want direct", view.TransportCapabilities)
	}
	if !hasDirectRoute(routes, directTarget) {
		t.Fatalf("routes = %#v, want direct target %q", routes, directTarget)
	}
	if len(transports) != 1 {
		t.Fatalf("transports len = %d, want 1", len(transports))
	}
	if !view.Reachable {
		t.Fatal("view.Reachable = false, want true")
	}
}

func TestBuildSendRuntimeBoundaryIncludesNostrFallbackRoutes(t *testing.T) {
	contact := contactRecord{
		ContactID:           "contact_nostr_fallback",
		CanonicalID:         "did:key:z6MkNostrFallback",
		DisplayName:         "Nostr Fallback",
		RecipientID:         "rcpt_nostr_fallback",
		SigningPublicKey:    "signing-key",
		EncryptionPublicKey: "enc-key",
		StoreForwardHints:   []string{"https://relay.storeforward.example"},
		NostrRelayHints: []string{
			"wss://relay-primary.nostr.example",
			"wss://relay-backup.nostr.example",
		},
		NostrPublicKeys:       []string{"npub_peer_primary"},
		NostrPrimaryPublicKey: "npub_peer_primary",
	}
	selfProfile := selfMessagingProfile{
		CanonicalID:      "did:key:z6MkSelf",
		SigningPublicKey: "self-signing-key",
	}

	view, transports, routes := buildSendRuntimeBoundary(selfProfile, contact, time.Now().UTC())

	if !hasNostrRouteRecipient(routes, "wss://relay-primary.nostr.example", "npub_peer_primary") ||
		!hasNostrRouteRecipient(routes, "wss://relay-backup.nostr.example", "npub_peer_primary") {
		t.Fatalf("routes = %#v, want both nostr fallback routes", routes)
	}
	if !hasStoreForwardRoute(routes, "https://relay.storeforward.example") {
		t.Fatalf("routes = %#v, want store-forward fallback route", routes)
	}
	if !slices.Contains(view.TransportCapabilities, string(transport.RouteTypeNostr)) {
		t.Fatalf("TransportCapabilities = %#v, want nostr", view.TransportCapabilities)
	}
	if len(transports) == 0 {
		t.Fatal("transports len = 0, want nostr transport")
	}
}

func TestBuildSendRuntimeBoundarySkipsNostrRoutesWithoutRecipientPublicKeys(t *testing.T) {
	contact := contactRecord{
		ContactID:           "contact_nostr_no_pubkey",
		CanonicalID:         "did:key:z6MkNoNostrPubKey",
		DisplayName:         "No Nostr PubKey",
		RecipientID:         "rcpt_no_pubkey",
		SigningPublicKey:    "signing-key",
		EncryptionPublicKey: "enc-key",
		StoreForwardHints:   []string{"https://relay.storeforward.example"},
		NostrRelayHints:     []string{"wss://relay-only.nostr.example"},
	}
	selfProfile := selfMessagingProfile{
		CanonicalID:      "did:key:z6MkSelf",
		SigningPublicKey: "self-signing-key",
	}

	view, _, routes := buildSendRuntimeBoundary(selfProfile, contact, time.Now().UTC())

	if hasNostrRoute(routes, "wss://relay-only.nostr.example") {
		t.Fatalf("routes = %#v, want nostr route skipped when recipient pubkey is missing", routes)
	}
	if slices.Contains(view.TransportCapabilities, string(transport.RouteTypeNostr)) {
		t.Fatalf("TransportCapabilities = %#v, want nostr capability removed without recipient pubkey", view.TransportCapabilities)
	}
	if !hasStoreForwardRoute(routes, "https://relay.storeforward.example") {
		t.Fatalf("routes = %#v, want store-forward fallback retained", routes)
	}
}

func TestSendMarksMessageFailedWhenAllRuntimeRoutesFail(t *testing.T) {
	t.Setenv(discoverylibp2p.EnvExperimentalDirect, "0")

	ctx := context.Background()
	now := time.Now().UTC()
	initService := initflow.NewService()
	cardService := card.NewService()

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	if _, err := initService.Init(ctx, initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAliceAllFail",
		DisplayName: "Alice All Fail",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}
	aliceCard, err := cardService.Export(ctx, card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	aliceCardJSON, err := json.Marshal(aliceCard.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(ctx, initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBobAllFail",
		DisplayName: "Bob All Fail",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	imported, err := cardService.Import(ctx, card.ImportOptions{
		Home:  bobHome,
		Input: string(aliceCardJSON),
	})
	if err != nil {
		t.Fatalf("import alice card into bob home: %v", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(bobHome, "state.db"))
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, `UPDATE contacts SET relay_url = ? WHERE contact_id = ?`, "https://relay.storeforward.invalid", imported.ContactID); err != nil {
		t.Fatalf("set contact relay_url: %v", err)
	}
	var canonicalID string
	if err := db.QueryRowContext(ctx, `SELECT canonical_id FROM contacts WHERE contact_id = ?`, imported.ContactID).Scan(&canonicalID); err != nil {
		t.Fatalf("query contact canonical_id: %v", err)
	}
	selfProfile, err := loadSelfMessagingProfile(ctx, db, bobHome)
	if err != nil {
		t.Fatalf("load self messaging profile: %v", err)
	}

	runtimeStore := agentruntime.NewStoreWithDB(db, now)
	if err := runtimeStore.UpsertTransportBinding(ctx, agentruntime.TransportBindingRecord{
		BindingID:    "binding_nostr_all_fail",
		SelfID:       selfProfile.SelfID,
		CanonicalID:  canonicalID,
		Transport:    string(transport.RouteTypeNostr),
		RelayURL:     "wss://",
		RouteLabel:   "wss://",
		RouteType:    string(transport.RouteTypeNostr),
		Direction:    "both",
		Enabled:      true,
		MetadataJSON: `{"nostr_public_keys":["npub_all_fail"],"nostr_primary_public_key":"npub_all_fail"}`,
	}); err != nil {
		t.Fatalf("upsert runtime transport binding: %v", err)
	}

	storeForwardBackend := &stubFailingMailboxBackend{sendErr: context.DeadlineExceeded}
	service := NewService()
	service.StoreForwardBackend = storeForwardBackend

	sent, err := service.Send(ctx, SendOptions{
		Home:       bobHome,
		ContactRef: imported.ContactID,
		Body:       "hello all failed routes",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if got, want := sent.Message.Status, StatusFailed; got != want {
		t.Fatalf("message status = %q, want %q", got, want)
	}
	if got, want := sent.Message.TransportStatus, TransportStatusFailed; got != want {
		t.Fatalf("transport status = %q, want %q", got, want)
	}
	if got, want := storeForwardBackend.sendCalls, 1; got != want {
		t.Fatalf("store-forward send calls = %d, want %d", got, want)
	}

	attempts := loadRuntimeRouteAttempts(t, bobHome)
	requireRouteAttempt(t, attempts, string(transport.RouteTypeNostr), "failed", "")
	requireRouteAttempt(t, attempts, string(transport.RouteTypeStoreForward), "failed", "")
}

func TestSendPublishesNostrEventWithRealSignature(t *testing.T) {
	t.Setenv(discoverylibp2p.EnvExperimentalDirect, "0")

	ctx := context.Background()
	now := time.Now().UTC()
	initService := initflow.NewService()
	cardService := card.NewService()

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	if _, err := initService.Init(ctx, initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAliceSignedNostr",
		DisplayName: "Alice Signed Nostr",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}
	aliceCard, err := cardService.Export(ctx, card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	aliceCardJSON, err := json.Marshal(aliceCard.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(ctx, initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBobSignedNostr",
		DisplayName: "Bob Signed Nostr",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	imported, err := cardService.Import(ctx, card.ImportOptions{
		Home:  bobHome,
		Input: string(aliceCardJSON),
	})
	if err != nil {
		t.Fatalf("import alice card into bob home: %v", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(bobHome, "state.db"))
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, `UPDATE contacts SET relay_url = ? WHERE contact_id = ?`, "https://relay.storeforward.invalid", imported.ContactID); err != nil {
		t.Fatalf("set contact relay_url: %v", err)
	}
	var canonicalID string
	if err := db.QueryRowContext(ctx, `SELECT canonical_id FROM contacts WHERE contact_id = ?`, imported.ContactID).Scan(&canonicalID); err != nil {
		t.Fatalf("query contact canonical_id: %v", err)
	}
	selfProfile, err := loadSelfMessagingProfile(ctx, db, bobHome)
	if err != nil {
		t.Fatalf("load self messaging profile: %v", err)
	}

	runtimeStore := agentruntime.NewStoreWithDB(db, now)
	if err := runtimeStore.UpsertTransportBinding(ctx, agentruntime.TransportBindingRecord{
		BindingID:    "binding_nostr_signed_send",
		SelfID:       selfProfile.SelfID,
		CanonicalID:  canonicalID,
		Transport:    string(transport.RouteTypeNostr),
		RelayURL:     "wss://relay.sign.nostr.example",
		RouteLabel:   "relay-sign",
		RouteType:    string(transport.RouteTypeNostr),
		Direction:    "both",
		Enabled:      true,
		MetadataJSON: `{"nostr_public_keys":["npub_peer_sign"],"nostr_primary_public_key":"npub_peer_sign"}`,
	}); err != nil {
		t.Fatalf("upsert runtime transport binding: %v", err)
	}

	nostrClient := &stubNostrRecoveryClient{
		publishReceipt: transportnostr.PublishReceipt{
			EventID:  "evt_ack_signed",
			Accepted: true,
			Message:  "ok",
		},
	}
	service := NewService()
	service.StoreForwardBackend = &stubFailingMailboxBackend{sendErr: context.DeadlineExceeded}
	service.NostrRelayClient = nostrClient

	sent, err := service.Send(ctx, SendOptions{
		Home:       bobHome,
		ContactRef: imported.ContactID,
		Body:       "hello signed nostr",
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if got, want := sent.Message.Status, StatusQueued; got != want {
		t.Fatalf("message status = %q, want %q", got, want)
	}
	if got, want := sent.Message.SelectedRoute.Type, transport.RouteTypeNostr; got != want {
		t.Fatalf("selected route type = %q, want %q", got, want)
	}
	if got, want := len(nostrClient.publishCalls), 1; got != want {
		t.Fatalf("nostr publish calls = %d, want %d", got, want)
	}
	published := nostrClient.publishCalls[0]
	if got, want := published.RelayURL, "wss://relay.sign.nostr.example"; got != want {
		t.Fatalf("publish relay url = %q, want %q", got, want)
	}
	if got, want := published.Event.PubKey, selfProfile.NostrSigningPublicKey; got != want {
		t.Fatalf("event pubkey = %q, want self nostr key %q", got, want)
	}
	verifyNostrEventSignature(t, published.Event)

	attempts := loadRuntimeRouteAttempts(t, bobHome)
	requireRouteAttempt(t, attempts, string(transport.RouteTypeNostr), "queued", "")
}

func TestDeriveTransportStatusSupportsRecoverableAsyncStatuses(t *testing.T) {
	if got, want := deriveTransportStatus(DirectionOutgoing, StatusRecovering, transport.RouteCandidate{}), TransportStatusDeferred; got != want {
		t.Fatalf("deriveTransportStatus(outgoing,recovering) = %q, want %q", got, want)
	}
	if got, want := deriveTransportStatus(DirectionIncoming, StatusRecovered, transport.RouteCandidate{}), TransportStatusRecovered; got != want {
		t.Fatalf("deriveTransportStatus(incoming,recovered) = %q, want %q", got, want)
	}
	if got, want := deriveTransportStatus(DirectionIncoming, StatusQueued, transport.RouteCandidate{}), TransportStatusRecovered; got != want {
		t.Fatalf("deriveTransportStatus(incoming,queued) = %q, want %q", got, want)
	}
	if got, want := deriveTransportStatus(DirectionOutgoing, StatusFailed, transport.RouteCandidate{}), TransportStatusFailed; got != want {
		t.Fatalf("deriveTransportStatus(outgoing,failed) = %q, want %q", got, want)
	}
}

func TestSyncUsesRuntimeNostrRoutesWhenLegacyRelayURLEmpty(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 3, 27, 1, 0, 0, 0, time.UTC)
	home, profile, recipientEncryptionPublicKey := setupRuntimeBridgeSyncHome(t, now)
	senderSigningPublicKey, senderSigningPrivateKeyPath := writeEd25519SigningKeyPair(t)

	const relayURL = "wss://relay.sync.nostr.example"
	const recipientPubKey = "npub_self_sync_route_1"
	const senderPubKey = "npub_sender_sync_route_1"
	const sentAt = "2026-03-27T00:30:00Z"

	pulledMessage := buildSignedMailboxPullMessage(
		t,
		recipientEncryptionPublicKey,
		senderSigningPrivateKeyPath,
		senderSigningPublicKey,
		profile.RecipientID,
		"did:key:z6MkSyncSender",
		"msg_sync_nostr_1",
		"evt_sync_nostr_1",
		sentAt,
		"hello from nostr recovery",
	)
	eventPayload, err := json.Marshal(map[string]string{
		"message_id":           pulledMessage.MessageID,
		"sender_pubkey":        senderPubKey,
		"sender_signing_key":   pulledMessage.SenderSigningKey,
		"recipient_pubkey":     recipientPubKey,
		"ephemeral_public_key": pulledMessage.EphemeralPublicKey,
		"nonce":                pulledMessage.Nonce,
		"ciphertext":           pulledMessage.Ciphertext,
		"signature":            pulledMessage.Signature,
		"sent_at":              pulledMessage.SentAt,
	})
	if err != nil {
		t.Fatalf("marshal nostr event payload: %v", err)
	}
	eventCreatedAt := parseTimestamp(sentAt).Unix()
	nostrEvent := transportnostr.Event{
		ID:        pulledMessage.RelayMessageID,
		PubKey:    senderPubKey,
		CreatedAt: eventCreatedAt,
		Kind:      4,
		Tags: [][]string{
			{"p", recipientPubKey},
			{"linkclaw_message_id", pulledMessage.MessageID},
		},
		Content: string(eventPayload),
	}

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	runtimeStore := agentruntime.NewStoreWithDB(db, now)
	if err := runtimeStore.UpsertTransportBinding(ctx, agentruntime.TransportBindingRecord{
		BindingID:   "binding_peer_sync_nostr",
		SelfID:      profile.SelfID,
		CanonicalID: pulledMessage.SenderID,
		Transport:   string(transport.RouteTypeNostr),
		RelayURL:    relayURL,
		RouteLabel:  "peer-sync",
		RouteType:   string(transport.RouteTypeNostr),
		Direction:   "incoming",
		Enabled:     true,
		MetadataJSON: `{"nostr_public_keys":["` + senderPubKey + `"],` +
			`"nostr_primary_public_key":"` + senderPubKey + `"}`,
	}); err != nil {
		db.Close()
		t.Fatalf("upsert peer runtime transport binding: %v", err)
	}
	if err := runtimeStore.UpsertTransportBinding(ctx, agentruntime.TransportBindingRecord{
		BindingID:   "binding_self_sync_nostr",
		SelfID:      profile.SelfID,
		CanonicalID: profile.CanonicalID,
		Transport:   string(transport.RouteTypeNostr),
		RelayURL:    relayURL,
		RouteLabel:  "self-sync",
		RouteType:   string(transport.RouteTypeNostr),
		Direction:   "incoming",
		Enabled:     true,
		MetadataJSON: `{"nostr_public_keys":["` + recipientPubKey + `"],` +
			`"nostr_primary_public_key":"` + recipientPubKey + `"}`,
	}); err != nil {
		db.Close()
		t.Fatalf("upsert runtime transport binding: %v", err)
	}
	if err := runtimeStore.UpsertTransportRelay(ctx, agentruntime.TransportRelayRecord{
		RelayID:      "relay_self_sync_nostr",
		Transport:    string(transport.RouteTypeNostr),
		RelayURL:     relayURL,
		ReadEnabled:  true,
		WriteEnabled: true,
		Priority:     50,
		Source:       "test",
		Status:       "active",
		MetadataJSON: "{}",
	}); err != nil {
		db.Close()
		t.Fatalf("upsert runtime transport relay: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite db: %v", err)
	}

	client := &stubNostrRecoveryClient{
		queryEvents: []transportnostr.Event{nostrEvent},
	}
	service := NewService()
	service.Now = func() time.Time { return now }
	service.NostrRelayClient = client

	firstSync, err := service.Sync(ctx, SyncOptions{Home: home})
	if err != nil {
		t.Fatalf("first Sync() error = %v", err)
	}
	if got, want := firstSync.Synced, 1; got != want {
		t.Fatalf("first sync synced = %d, want %d", got, want)
	}
	if got, want := firstSync.RelayCalls, 1; got != want {
		t.Fatalf("first sync relay calls = %d, want %d", got, want)
	}
	if got := len(client.queryCalls); got != 1 {
		t.Fatalf("first sync query calls = %d, want 1", got)
	}
	firstQuery := client.queryCalls[0]
	if firstQuery.RelayURL != relayURL {
		t.Fatalf("first query relay url = %q, want %q", firstQuery.RelayURL, relayURL)
	}
	if len(firstQuery.Filter.Recipient) != 1 || firstQuery.Filter.Recipient[0] != recipientPubKey {
		t.Fatalf("first query recipient filter = %#v, want [%s]", firstQuery.Filter.Recipient, recipientPubKey)
	}
	if firstQuery.Filter.Since != nil {
		t.Fatalf("first query since = %#v, want nil", firstQuery.Filter.Since)
	}

	service.Now = func() time.Time { return now.Add(2 * time.Minute) }
	secondSync, err := service.Sync(ctx, SyncOptions{Home: home})
	if err != nil {
		t.Fatalf("second Sync() error = %v", err)
	}
	if got, want := secondSync.Synced, 0; got != want {
		t.Fatalf("second sync synced = %d, want %d", got, want)
	}
	if got, want := secondSync.RelayCalls, 1; got != want {
		t.Fatalf("second sync relay calls = %d, want %d", got, want)
	}
	if got := len(client.queryCalls); got != 2 {
		t.Fatalf("second sync query calls = %d, want 2", got)
	}
	secondQuery := client.queryCalls[1]
	if secondQuery.Filter.Since == nil {
		t.Fatal("second query since = nil, want cursor-derived since")
	}
	if got, want := *secondQuery.Filter.Since, eventCreatedAt; got != want {
		t.Fatalf("second query since = %d, want %d", got, want)
	}

	attempts := loadRuntimeRouteAttempts(t, home)
	recoveredCursor := requireRouteAttemptWithNonEmptyCursor(t, attempts, string(transport.RouteTypeNostr), "recovered")
	requireRouteAttempt(t, attempts, string(transport.RouteTypeNostr), "acked", recoveredCursor)
}

func TestNostrSendSyncRecoverUsesPubkeySchema(t *testing.T) {
	t.Setenv(discoverylibp2p.EnvExperimentalDirect, "0")

	ctx := context.Background()
	now := time.Date(2026, 3, 27, 2, 45, 0, 0, time.UTC)
	const relayURL = "wss://relay.e2e.nostr.example"
	const recipientPubKey = "npub_recipient_e2e_1"
	const body = "hello from nostr e2e"

	initService := initflow.NewService()
	cardService := card.NewService()

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	if _, err := initService.Init(ctx, initflow.Options{
		Home:        aliceHome,
		CanonicalID: "did:key:z6MkAliceNostrE2E",
		DisplayName: "Alice Nostr E2E",
	}); err != nil {
		t.Fatalf("init alice home: %v", err)
	}
	aliceCard, err := cardService.Export(ctx, card.Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export alice card: %v", err)
	}
	aliceCardJSON, err := json.Marshal(aliceCard.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(ctx, initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBobNostrE2E",
		DisplayName: "Bob Nostr E2E",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}
	bobCard, err := cardService.Export(ctx, card.Options{Home: bobHome})
	if err != nil {
		t.Fatalf("export bob card: %v", err)
	}
	bobCardJSON, err := json.Marshal(bobCard.Card)
	if err != nil {
		t.Fatalf("marshal bob card: %v", err)
	}

	importedAliceInBob, err := cardService.Import(ctx, card.ImportOptions{
		Home:  bobHome,
		Input: string(aliceCardJSON),
	})
	if err != nil {
		t.Fatalf("import alice card into bob home: %v", err)
	}
	importedBobInAlice, err := cardService.Import(ctx, card.ImportOptions{
		Home:  aliceHome,
		Input: string(bobCardJSON),
	})
	if err != nil {
		t.Fatalf("import bob card into alice home: %v", err)
	}

	bobDB, err := sql.Open("sqlite", filepath.Join(bobHome, "state.db"))
	if err != nil {
		t.Fatalf("open bob sqlite db: %v", err)
	}
	var aliceCanonicalID string
	if err := bobDB.QueryRowContext(ctx, `SELECT canonical_id FROM contacts WHERE contact_id = ?`, importedAliceInBob.ContactID).Scan(&aliceCanonicalID); err != nil {
		bobDB.Close()
		t.Fatalf("query alice canonical_id in bob db: %v", err)
	}
	bobProfile, err := loadSelfMessagingProfile(ctx, bobDB, bobHome)
	if err != nil {
		bobDB.Close()
		t.Fatalf("load bob self messaging profile: %v", err)
	}
	if _, err := bobDB.ExecContext(ctx, `UPDATE contacts SET relay_url = ? WHERE contact_id = ?`, "https://relay.storeforward.invalid", importedAliceInBob.ContactID); err != nil {
		bobDB.Close()
		t.Fatalf("set bob contact relay_url fallback: %v", err)
	}
	bobRuntimeStore := agentruntime.NewStoreWithDB(bobDB, now)
	if err := bobRuntimeStore.UpsertTransportBinding(ctx, agentruntime.TransportBindingRecord{
		BindingID:   "binding_bob_to_alice_nostr_e2e",
		SelfID:      bobProfile.SelfID,
		CanonicalID: aliceCanonicalID,
		Transport:   string(transport.RouteTypeNostr),
		RelayURL:    relayURL,
		RouteLabel:  "e2e-bob-send",
		RouteType:   string(transport.RouteTypeNostr),
		Direction:   "both",
		Enabled:     true,
		MetadataJSON: `{"nostr_public_keys":["` + recipientPubKey + `"],` +
			`"nostr_primary_public_key":"` + recipientPubKey + `"}`,
	}); err != nil {
		bobDB.Close()
		t.Fatalf("upsert bob runtime transport binding: %v", err)
	}
	if err := bobDB.Close(); err != nil {
		t.Fatalf("close bob sqlite db: %v", err)
	}

	publishClient := &stubNostrRecoveryClient{
		publishReceipt: transportnostr.PublishReceipt{
			EventID:  "evt_nostr_e2e",
			Accepted: true,
			Message:  "ok",
		},
	}
	sendService := NewService()
	sendService.Now = func() time.Time { return now }
	sendService.StoreForwardBackend = &stubFailingMailboxBackend{sendErr: context.DeadlineExceeded}
	sendService.NostrRelayClient = publishClient

	sent, err := sendService.Send(ctx, SendOptions{
		Home:       bobHome,
		ContactRef: importedAliceInBob.ContactID,
		Body:       body,
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}
	if got, want := sent.Message.Status, StatusQueued; got != want {
		t.Fatalf("send status = %q, want %q", got, want)
	}
	if got, want := sent.Message.SelectedRoute.Type, transport.RouteTypeNostr; got != want {
		t.Fatalf("send selected route type = %q, want %q", got, want)
	}
	if got, want := len(publishClient.publishCalls), 1; got != want {
		t.Fatalf("nostr publish calls = %d, want %d", got, want)
	}
	publishedEvent := publishClient.publishCalls[0].Event
	var publishedPayload map[string]string
	if err := json.Unmarshal([]byte(publishedEvent.Content), &publishedPayload); err != nil {
		t.Fatalf("decode published nostr payload: %v", err)
	}
	senderPubKey := publishedPayload["sender_pubkey"]
	if senderPubKey == "" {
		t.Fatalf("published payload sender_pubkey = %q, want non-empty", senderPubKey)
	}
	if got, want := publishedPayload["recipient_pubkey"], recipientPubKey; got != want {
		t.Fatalf("published payload recipient_pubkey = %q, want %q", got, want)
	}
	if strings.TrimSpace(publishedPayload["signature"]) == "" {
		t.Fatalf("published payload signature = %q, want non-empty", publishedPayload["signature"])
	}
	if _, ok := publishedPayload["sender_id"]; ok {
		t.Fatalf("published payload sender_id should not be present: %#v", publishedPayload)
	}
	if _, ok := publishedPayload["recipient_id"]; ok {
		t.Fatalf("published payload recipient_id should not be present: %#v", publishedPayload)
	}

	aliceDB, err := sql.Open("sqlite", filepath.Join(aliceHome, "state.db"))
	if err != nil {
		t.Fatalf("open alice sqlite db: %v", err)
	}
	aliceProfile, err := loadSelfMessagingProfile(ctx, aliceDB, aliceHome)
	if err != nil {
		aliceDB.Close()
		t.Fatalf("load alice self messaging profile: %v", err)
	}
	var bobCanonicalID string
	if err := aliceDB.QueryRowContext(ctx, `SELECT canonical_id FROM contacts WHERE contact_id = ?`, importedBobInAlice.ContactID).Scan(&bobCanonicalID); err != nil {
		aliceDB.Close()
		t.Fatalf("query bob canonical_id in alice db: %v", err)
	}
	aliceRuntimeStore := agentruntime.NewStoreWithDB(aliceDB, now)
	if err := aliceRuntimeStore.UpsertTransportBinding(ctx, agentruntime.TransportBindingRecord{
		BindingID:   "binding_alice_self_nostr_e2e",
		SelfID:      aliceProfile.SelfID,
		CanonicalID: aliceProfile.CanonicalID,
		Transport:   string(transport.RouteTypeNostr),
		RelayURL:    relayURL,
		RouteLabel:  "e2e-alice-self",
		RouteType:   string(transport.RouteTypeNostr),
		Direction:   "incoming",
		Enabled:     true,
		MetadataJSON: `{"nostr_public_keys":["` + recipientPubKey + `"],` +
			`"nostr_primary_public_key":"` + recipientPubKey + `"}`,
	}); err != nil {
		aliceDB.Close()
		t.Fatalf("upsert alice self runtime transport binding: %v", err)
	}
	if err := aliceRuntimeStore.UpsertTransportBinding(ctx, agentruntime.TransportBindingRecord{
		BindingID:   "binding_alice_peer_nostr_e2e",
		SelfID:      aliceProfile.SelfID,
		CanonicalID: bobCanonicalID,
		Transport:   string(transport.RouteTypeNostr),
		RelayURL:    relayURL,
		RouteLabel:  "e2e-alice-peer",
		RouteType:   string(transport.RouteTypeNostr),
		Direction:   "incoming",
		Enabled:     true,
		MetadataJSON: `{"nostr_public_keys":["` + senderPubKey + `"],` +
			`"nostr_primary_public_key":"` + senderPubKey + `"}`,
	}); err != nil {
		aliceDB.Close()
		t.Fatalf("upsert alice peer runtime transport binding: %v", err)
	}
	if err := aliceRuntimeStore.UpsertTransportRelay(ctx, agentruntime.TransportRelayRecord{
		RelayID:      "relay_nostr_e2e",
		Transport:    string(transport.RouteTypeNostr),
		RelayURL:     relayURL,
		ReadEnabled:  true,
		WriteEnabled: true,
		Priority:     100,
		Source:       "test",
		Status:       "active",
		MetadataJSON: "{}",
	}); err != nil {
		aliceDB.Close()
		t.Fatalf("upsert alice runtime transport relay: %v", err)
	}
	if err := aliceDB.Close(); err != nil {
		t.Fatalf("close alice sqlite db: %v", err)
	}

	syncClient := &stubNostrRecoveryClient{
		queryEvents: []transportnostr.Event{publishedEvent},
	}
	syncService := NewService()
	syncService.Now = func() time.Time { return now.Add(90 * time.Second) }
	syncService.NostrRelayClient = syncClient

	syncResult, err := syncService.Sync(ctx, SyncOptions{Home: aliceHome})
	if err != nil {
		t.Fatalf("sync message: %v", err)
	}
	if got, want := syncResult.Synced, 1; got != want {
		t.Fatalf("sync recovered count = %d, want %d", got, want)
	}
	if got, want := syncResult.RelayCalls, 1; got != want {
		t.Fatalf("sync relay calls = %d, want %d", got, want)
	}
	if got, want := len(syncClient.queryCalls), 1; got != want {
		t.Fatalf("sync query calls = %d, want %d", got, want)
	}
	if len(syncClient.queryCalls[0].Filter.Recipient) != 1 || syncClient.queryCalls[0].Filter.Recipient[0] != recipientPubKey {
		t.Fatalf("sync query recipient filter = %#v, want [%s]", syncClient.queryCalls[0].Filter.Recipient, recipientPubKey)
	}

	verifyDB, err := sql.Open("sqlite", filepath.Join(aliceHome, "state.db"))
	if err != nil {
		t.Fatalf("open verify sqlite db: %v", err)
	}
	defer verifyDB.Close()

	var recoveredSenderID, recoveredRecipientID, recoveredStatus, recoveredBody string
	if err := verifyDB.QueryRowContext(
		ctx,
		`SELECT sender_canonical_id, recipient_route_id, status, plaintext_body
		   FROM messages
		  WHERE message_id = ?
		  LIMIT 1`,
		sent.Message.MessageID,
	).Scan(&recoveredSenderID, &recoveredRecipientID, &recoveredStatus, &recoveredBody); err != nil {
		t.Fatalf("query recovered message row: %v", err)
	}
	if got, want := recoveredSenderID, bobCanonicalID; got != want {
		t.Fatalf("recovered sender_canonical_id = %q, want %q", got, want)
	}
	if got, want := recoveredRecipientID, aliceProfile.RecipientID; got != want {
		t.Fatalf("recovered recipient_route_id = %q, want %q", got, want)
	}
	if got, want := recoveredStatus, StatusRecovered; got != want {
		t.Fatalf("recovered message status = %q, want %q", got, want)
	}
	if got, want := recoveredBody, body; got != want {
		t.Fatalf("recovered plaintext_body = %q, want %q", got, want)
	}

	attempts := loadRuntimeRouteAttempts(t, aliceHome)
	recoveredCursor := requireRouteAttemptWithNonEmptyCursor(t, attempts, string(transport.RouteTypeNostr), "recovered")
	requireRouteAttempt(t, attempts, string(transport.RouteTypeNostr), "acked", recoveredCursor)
}

type stubFailingMailboxBackend struct {
	sendErr   error
	sendCalls int
}

func (b *stubFailingMailboxBackend) Send(context.Context, string, transportstoreforward.MailboxSendRequest) (transportstoreforward.MailboxSendResponse, error) {
	b.sendCalls++
	if b.sendErr != nil {
		return transportstoreforward.MailboxSendResponse{}, b.sendErr
	}
	return transportstoreforward.MailboxSendResponse{RemoteMessageID: "relay_msg"}, nil
}

func (b *stubFailingMailboxBackend) Pull(context.Context, string, string, string) (transportstoreforward.MailboxPullResponse, error) {
	return transportstoreforward.MailboxPullResponse{}, nil
}

func (b *stubFailingMailboxBackend) Ack(context.Context, string, transportstoreforward.MailboxAckRequest) error {
	return nil
}

type stubNostrRecoveryClient struct {
	publishReceipt transportnostr.PublishReceipt
	publishErr     error
	publishCalls   []stubNostrRecoveryPublishCall

	queryEvents []transportnostr.Event
	queryErr    error
	queryCalls  []stubNostrRecoveryQueryCall
}

type stubNostrRecoveryPublishCall struct {
	RelayURL string
	Event    transportnostr.Event
}

type stubNostrRecoveryQueryCall struct {
	RelayURL       string
	SubscriptionID string
	Filter         transportnostr.Filter
}

func (c *stubNostrRecoveryClient) Publish(_ context.Context, relayURL string, event transportnostr.Event) (transportnostr.PublishReceipt, error) {
	c.publishCalls = append(c.publishCalls, stubNostrRecoveryPublishCall{
		RelayURL: relayURL,
		Event:    event,
	})
	if c.publishErr != nil {
		return transportnostr.PublishReceipt{}, c.publishErr
	}
	receipt := c.publishReceipt
	if receipt.EventID == "" {
		receipt.EventID = event.ID
	}
	return receipt, nil
}

func (c *stubNostrRecoveryClient) Query(_ context.Context, relayURL string, subscriptionID string, filter transportnostr.Filter) ([]transportnostr.Event, error) {
	c.queryCalls = append(c.queryCalls, stubNostrRecoveryQueryCall{
		RelayURL:       relayURL,
		SubscriptionID: subscriptionID,
		Filter:         filter,
	})
	if c.queryErr != nil {
		return nil, c.queryErr
	}
	return append([]transportnostr.Event(nil), c.queryEvents...), nil
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

func hasDirectRoute(routes []transport.RouteCandidate, target string) bool {
	for _, route := range routes {
		if route.Type == transport.RouteTypeDirect && route.Target == target {
			return true
		}
	}
	return false
}

func hasNostrRoute(routes []transport.RouteCandidate, target string) bool {
	for _, route := range routes {
		if route.Type == transport.RouteTypeNostr && route.Target == target {
			return true
		}
	}
	return false
}

func hasNostrRouteRecipient(routes []transport.RouteCandidate, relayURL string, recipient string) bool {
	for _, route := range routes {
		if route.Type != transport.RouteTypeNostr {
			continue
		}
		parsed, err := url.Parse(route.Target)
		if err != nil {
			continue
		}
		query := parsed.Query()
		gotRecipient := query.Get("recipient")
		if gotRecipient == "" {
			gotRecipient = query.Get("p")
		}
		parsed.RawQuery = ""
		parsed.Fragment = ""
		if parsed.String() == relayURL && gotRecipient == recipient {
			return true
		}
	}
	return false
}

func verifyNostrEventSignature(t *testing.T, event transportnostr.Event) {
	t.Helper()
	eventHash, err := hex.DecodeString(event.ID)
	if err != nil {
		t.Fatalf("decode event id: %v", err)
	}
	signatureBytes, err := hex.DecodeString(event.Sig)
	if err != nil {
		t.Fatalf("decode event signature: %v", err)
	}
	signature, err := schnorr.ParseSignature(signatureBytes)
	if err != nil {
		t.Fatalf("parse event signature: %v", err)
	}
	pubKeyBytes, err := hex.DecodeString(event.PubKey)
	if err != nil {
		t.Fatalf("decode event pubkey: %v", err)
	}
	pubKey, err := schnorr.ParsePubKey(pubKeyBytes)
	if err != nil {
		t.Fatalf("parse event pubkey: %v", err)
	}
	if !signature.Verify(eventHash, pubKey) {
		t.Fatalf("event signature verification failed: id=%q sig=%q pubkey=%q", event.ID, event.Sig, event.PubKey)
	}
}
