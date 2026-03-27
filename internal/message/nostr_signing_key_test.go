package message

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/initflow"
	transportnostr "github.com/xiewanpeng/claw-identity/internal/transport/nostr"
)

func TestEnsureSelfNostrSigningKeyCreatesAndReusesKey(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "home")
	if _, err := initflow.NewService().Init(ctx, initflow.Options{
		Home:        home,
		CanonicalID: "did:key:z6MkSelfNostrKey",
		DisplayName: "Self Nostr Key",
	}); err != nil {
		t.Fatalf("init home: %v", err)
	}

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	var selfID string
	if err := db.QueryRowContext(ctx, `SELECT self_id FROM self_identities LIMIT 1`).Scan(&selfID); err != nil {
		t.Fatalf("query self id: %v", err)
	}

	now := time.Date(2026, 3, 27, 3, 0, 0, 0, time.UTC)
	first, err := ensureSelfNostrSigningKey(ctx, db, home, selfID, now)
	if err != nil {
		t.Fatalf("ensureSelfNostrSigningKey() first call error = %v", err)
	}
	if got := len(strings.TrimSpace(first.PublicKey)); got != 64 {
		t.Fatalf("first public key len = %d, want 64", got)
	}
	if _, err := os.Stat(first.PrivateKeyPath); err != nil {
		t.Fatalf("nostr private key file missing: %v", err)
	}

	second, err := ensureSelfNostrSigningKey(ctx, db, home, selfID, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("ensureSelfNostrSigningKey() second call error = %v", err)
	}
	if got, want := second.PublicKey, first.PublicKey; got != want {
		t.Fatalf("reused public key = %q, want %q", got, want)
	}
	if got, want := second.PrivateKeyPath, first.PrivateKeyPath; got != want {
		t.Fatalf("reused private key path = %q, want %q", got, want)
	}

	var keyCount int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM keys WHERE owner_type = ? AND owner_id = ? AND status = 'active'`,
		selfNostrKeyOwnerType,
		selfID,
	).Scan(&keyCount); err != nil {
		t.Fatalf("count self nostr keys: %v", err)
	}
	if got, want := keyCount, 1; got != want {
		t.Fatalf("self nostr active key count = %d, want %d", got, want)
	}

	signer, err := transportnostr.NewSchnorrSignerFromPrivateKeyFile(first.PrivateKeyPath)
	if err != nil {
		t.Fatalf("load signer from private key file: %v", err)
	}
	if got, want := signer.PublicKey(), first.PublicKey; got != want {
		t.Fatalf("signer public key = %q, want %q", got, want)
	}
}
