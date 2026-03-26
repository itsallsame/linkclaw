package card

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/xiewanpeng/claw-identity/internal/initflow"

	_ "modernc.org/sqlite"
)

func TestExportAndVerify(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "home")
	initService := initflow.NewService()
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        home,
		CanonicalID: "did:key:z6MkAlice",
		DisplayName: "Alice",
	}); err != nil {
		t.Fatalf("init home: %v", err)
	}

	service := NewService()
	exported, err := service.Export(context.Background(), Options{Home: home})
	if err != nil {
		t.Fatalf("export card: %v", err)
	}
	if exported.Card.Signature == "" {
		t.Fatalf("expected signature to be populated")
	}
	if exported.Card.Messaging.RecipientID == "" {
		t.Fatalf("expected recipient id to be populated")
	}

	raw, err := json.Marshal(exported.Card)
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	verified, err := service.Verify(context.Background(), VerifyOptions{Input: string(raw)})
	if err != nil {
		t.Fatalf("verify card: %v", err)
	}
	if !verified.Verified {
		t.Fatalf("expected verified result")
	}
	if verified.Card.ID != exported.Card.ID {
		t.Fatalf("verified id = %q, want %q", verified.Card.ID, exported.Card.ID)
	}
}

func TestExportIncludesNostrBindingsWhenRuntimeMetadataExists(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "home")
	initService := initflow.NewService()
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        home,
		CanonicalID: "did:key:z6MkAlice",
		DisplayName: "Alice",
	}); err != nil {
		t.Fatalf("init home: %v", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	var selfID string
	if err := db.QueryRow(`SELECT self_id FROM self_identities LIMIT 1`).Scan(&selfID); err != nil {
		t.Fatalf("query self_id: %v", err)
	}

	stamp := "2026-03-26T09:00:00Z"
	if _, err := db.Exec(
		`INSERT INTO runtime_transport_bindings (
			binding_id, self_id, canonical_id, transport, relay_url, route_label, route_type,
			direction, enabled, metadata_json, created_at, updated_at
		) VALUES (?, ?, ?, 'nostr', ?, 'relay-main', 'nostr', 'both', 1, ?, ?, ?)`,
		"binding_1",
		selfID,
		"did:key:z6MkAlice",
		"wss://relay.binding.example",
		`{"nostr_public_keys":["npub1","npub2"],"nostr_primary_public_key":"npub2"}`,
		stamp,
		stamp,
	); err != nil {
		t.Fatalf("insert runtime_transport_bindings: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO runtime_transport_relays (
			relay_id, transport, relay_url, read_enabled, write_enabled, priority, source, status,
			last_error, metadata_json, created_at, updated_at
		) VALUES (?, 'nostr', ?, 1, 1, 9, 'config', 'active', '', '{}', ?, ?)`,
		"relay_1",
		"wss://relay.table.example",
		stamp,
		stamp,
	); err != nil {
		t.Fatalf("insert runtime_transport_relays: %v", err)
	}

	service := NewService()
	exported, err := service.Export(context.Background(), Options{Home: home})
	if err != nil {
		t.Fatalf("export card: %v", err)
	}
	if !slices.Contains(exported.Card.TransportCapabilities, "nostr") {
		t.Fatalf("transport capabilities = %v, want nostr", exported.Card.TransportCapabilities)
	}
	if !slices.Contains(exported.Card.RelayURLs, "wss://relay.binding.example") {
		t.Fatalf("relay_urls = %v, want binding relay", exported.Card.RelayURLs)
	}
	if !slices.Contains(exported.Card.RelayURLs, "wss://relay.table.example") {
		t.Fatalf("relay_urls = %v, want table relay", exported.Card.RelayURLs)
	}
	if !slices.Contains(exported.Card.NostrPublicKeys, "npub1") || !slices.Contains(exported.Card.NostrPublicKeys, "npub2") {
		t.Fatalf("nostr_public_keys = %v, want npub1 and npub2", exported.Card.NostrPublicKeys)
	}
	if exported.Card.NostrPrimaryPublicKey != "npub2" {
		t.Fatalf("nostr_primary_public_key = %q, want npub2", exported.Card.NostrPrimaryPublicKey)
	}

	raw, err := json.Marshal(exported.Card)
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	verified, err := service.Verify(context.Background(), VerifyOptions{Input: string(raw)})
	if err != nil {
		t.Fatalf("verify card: %v", err)
	}
	if !verified.Verified {
		t.Fatal("verified = false, want true")
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "home")
	initService := initflow.NewService()
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        home,
		CanonicalID: "did:key:z6MkAlice",
		DisplayName: "Alice",
	}); err != nil {
		t.Fatalf("init home: %v", err)
	}

	service := NewService()
	exported, err := service.Export(context.Background(), Options{Home: home})
	if err != nil {
		t.Fatalf("export card: %v", err)
	}
	exported.Card.DisplayName = "Mallory"

	raw, err := json.Marshal(exported.Card)
	if err != nil {
		t.Fatalf("marshal tampered card: %v", err)
	}
	if _, err := service.Verify(context.Background(), VerifyOptions{Input: string(raw)}); err == nil {
		t.Fatalf("expected tampered card verification to fail")
	}
}

func TestImportCreatesContactFromCard(t *testing.T) {
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

	service := NewService()
	exported, err := service.Export(context.Background(), Options{Home: aliceHome})
	if err != nil {
		t.Fatalf("export card: %v", err)
	}
	raw, err := json.Marshal(exported.Card)
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	if _, err := initService.Init(context.Background(), initflow.Options{
		Home:        bobHome,
		CanonicalID: "did:key:z6MkBob",
		DisplayName: "Bob",
	}); err != nil {
		t.Fatalf("init bob home: %v", err)
	}

	imported, err := service.Import(context.Background(), ImportOptions{
		Home:  bobHome,
		Input: string(raw),
	})
	if err != nil {
		t.Fatalf("import card: %v", err)
	}
	if !imported.Created {
		t.Fatalf("expected new contact to be created")
	}

	db, err := sql.Open("sqlite", filepath.Join(bobHome, "state.db"))
	if err != nil {
		t.Fatalf("open bob db: %v", err)
	}
	defer db.Close()

	var canonicalID, displayName, status, recipientID string
	if err := db.QueryRow(
		`SELECT canonical_id, display_name, status, recipient_id
		 FROM contacts
		 WHERE contact_id = ?`,
		imported.ContactID,
	).Scan(&canonicalID, &displayName, &status, &recipientID); err != nil {
		t.Fatalf("query imported contact: %v", err)
	}
	if canonicalID != exported.Card.ID {
		t.Fatalf("canonical id = %q, want %q", canonicalID, exported.Card.ID)
	}
	if displayName != exported.Card.DisplayName {
		t.Fatalf("display name = %q, want %q", displayName, exported.Card.DisplayName)
	}
	if status != "imported" {
		t.Fatalf("status = %q, want imported", status)
	}
	if recipientID != exported.Card.Messaging.RecipientID {
		t.Fatalf("recipient id = %q, want %q", recipientID, exported.Card.Messaging.RecipientID)
	}
}
