package message

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/ids"
	"github.com/xiewanpeng/claw-identity/internal/layout"
	transportnostr "github.com/xiewanpeng/claw-identity/internal/transport/nostr"
)

const selfNostrKeyOwnerType = "self_nostr"

type selfNostrSigningKey struct {
	PublicKey      string
	PrivateKeyPath string
}

func ensureSelfNostrSigningKey(ctx context.Context, db *sql.DB, home string, selfID string, now time.Time) (selfNostrSigningKey, error) {
	selfID = strings.TrimSpace(selfID)
	if selfID == "" {
		return selfNostrSigningKey{}, fmt.Errorf("self id is required for nostr signing key")
	}
	if db == nil {
		return selfNostrSigningKey{}, fmt.Errorf("database handle is required for nostr signing key")
	}

	var keyID string
	var publicKey string
	var privateRef string
	err := db.QueryRowContext(
		ctx,
		`SELECT key_id, public_key, private_key_ref
		 FROM keys
		 WHERE owner_type = ? AND owner_id = ? AND status = 'active' AND algorithm = ?
		 ORDER BY created_at ASC
		 LIMIT 1`,
		selfNostrKeyOwnerType,
		selfID,
		transportnostr.AlgorithmSecp256k1Schnorr,
	).Scan(&keyID, &publicKey, &privateRef)
	switch {
	case err == nil:
		paths := layout.BuildPaths(home)
		privatePath := strings.TrimSpace(privateRef)
		if !filepath.IsAbs(privatePath) {
			privatePath = filepath.Join(paths.KeysDir, privatePath)
		}
		signer, signerErr := transportnostr.NewSchnorrSignerFromPrivateKeyFile(privatePath)
		if signerErr != nil {
			return selfNostrSigningKey{}, fmt.Errorf("load self nostr private key: %w", signerErr)
		}
		derivedPubKey := signer.PublicKey()
		if strings.ToLower(strings.TrimSpace(publicKey)) != strings.ToLower(strings.TrimSpace(derivedPubKey)) && strings.TrimSpace(keyID) != "" {
			if _, updateErr := db.ExecContext(
				ctx,
				`UPDATE keys SET public_key = ? WHERE key_id = ?`,
				derivedPubKey,
				keyID,
			); updateErr != nil {
				return selfNostrSigningKey{}, fmt.Errorf("repair self nostr public key metadata: %w", updateErr)
			}
		}
		return selfNostrSigningKey{
			PublicKey:      derivedPubKey,
			PrivateKeyPath: privatePath,
		}, nil
	case !errors.Is(err, sql.ErrNoRows):
		return selfNostrSigningKey{}, fmt.Errorf("query self nostr signing key: %w", err)
	}

	privateKeyHex, publicKeyHex, err := transportnostr.GenerateSchnorrPrivateKeyHex()
	if err != nil {
		return selfNostrSigningKey{}, fmt.Errorf("generate self nostr signing key: %w", err)
	}
	keyID, err = ids.New("key")
	if err != nil {
		return selfNostrSigningKey{}, err
	}
	paths := layout.BuildPaths(home)
	if err := os.MkdirAll(paths.KeysDir, 0o755); err != nil {
		return selfNostrSigningKey{}, fmt.Errorf("ensure keys dir: %w", err)
	}
	privateName := keyID + ".nostr.key"
	privatePath := filepath.Join(paths.KeysDir, privateName)
	if err := os.WriteFile(privatePath, []byte(privateKeyHex+"\n"), 0o600); err != nil {
		return selfNostrSigningKey{}, fmt.Errorf("write self nostr private key: %w", err)
	}
	timestamp := now.UTC().Format(time.RFC3339Nano)
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO keys (
			key_id, owner_type, owner_id, algorithm, public_key, private_key_ref,
			status, published_status, valid_from, created_at
		) VALUES (?, ?, ?, ?, ?, ?, 'active', 'local_only', ?, ?)`,
		keyID,
		selfNostrKeyOwnerType,
		selfID,
		transportnostr.AlgorithmSecp256k1Schnorr,
		publicKeyHex,
		privateName,
		timestamp,
		timestamp,
	); err != nil {
		_ = os.Remove(privatePath)
		return selfNostrSigningKey{}, fmt.Errorf("persist self nostr signing key metadata: %w", err)
	}
	return selfNostrSigningKey{
		PublicKey:      publicKeyHex,
		PrivateKeyPath: privatePath,
	}, nil
}
