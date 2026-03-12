package keys

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestEnsureDefaultKeyWithNilNow(t *testing.T) {
	t.Parallel()

	db := openTestDB(t, `
		CREATE TABLE keys (
			key_id TEXT PRIMARY KEY,
			owner_type TEXT NOT NULL,
			owner_id TEXT NOT NULL,
			algorithm TEXT NOT NULL,
			public_key TEXT NOT NULL,
			private_key_ref TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			published_status TEXT NOT NULL DEFAULT 'local_only',
			valid_from TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
	`)

	keysDir := t.TempDir()
	backend := &FileBackend{}

	result, err := backend.EnsureDefaultKey(context.Background(), db, "self_123", keysDir)
	if err != nil {
		t.Fatalf("EnsureDefaultKey returned error: %v", err)
	}
	if !result.Created {
		t.Fatalf("expected Created=true, got false")
	}
	if result.KeyID == "" {
		t.Fatalf("expected non-empty key id")
	}
	if _, err := os.Stat(result.PrivateKeyPath); err != nil {
		t.Fatalf("private key file missing: %v", err)
	}
	if _, err := os.Stat(result.PublicKeyPath); err != nil {
		t.Fatalf("public key file missing: %v", err)
	}
}

func TestEnsureDefaultKeyCleanupWhenInsertFails(t *testing.T) {
	t.Parallel()

	db := openTestDB(t, `
		CREATE TABLE keys (
			key_id TEXT PRIMARY KEY,
			owner_type TEXT NOT NULL,
			owner_id TEXT NOT NULL,
			algorithm TEXT NOT NULL,
			public_key TEXT NOT NULL,
			private_key_ref TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			valid_from TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
	`)

	keysDir := t.TempDir()
	backend := NewFileBackend()

	_, err := backend.EnsureDefaultKey(context.Background(), db, "self_456", keysDir)
	if err == nil {
		t.Fatalf("expected insert failure, got nil error")
	}
	if !strings.Contains(err.Error(), "insert key metadata") {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, readErr := os.ReadDir(keysDir)
	if readErr != nil {
		t.Fatalf("read keys dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("expected cleanup to remove generated key files, found %d entries", len(entries))
	}
}

func openTestDB(t *testing.T, schema string) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	return db
}
