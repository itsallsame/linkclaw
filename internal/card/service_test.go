package card

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
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
