package keys

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/ids"
)

const AlgorithmEd25519 = "Ed25519"

type FileBackend struct {
	Now func() time.Time
}

func NewFileBackend() *FileBackend {
	return &FileBackend{Now: time.Now}
}

func (b *FileBackend) EnsureDefaultKey(ctx context.Context, db *sql.DB, ownerID, keysDir string) (Result, error) {
	const selectSQL = `
		SELECT key_id, algorithm, public_key, private_key_ref
		FROM keys
		WHERE owner_type = 'self' AND owner_id = ? AND status = 'active'
		ORDER BY created_at ASC
		LIMIT 1
	`

	var out Result
	var privateRef string
	err := db.QueryRowContext(ctx, selectSQL, ownerID).Scan(&out.KeyID, &out.Algorithm, &out.PublicKey, &privateRef)
	switch {
	case err == nil:
		out.PrivateKeyPath = privateRef
		if !filepath.IsAbs(out.PrivateKeyPath) {
			out.PrivateKeyPath = filepath.Join(keysDir, privateRef)
		}
		out.PublicKeyPath = out.PrivateKeyPath + ".pub"
		out.Created = false
		return out, nil
	case !errors.Is(err, sql.ErrNoRows):
		return Result{}, fmt.Errorf("query existing key: %w", err)
	}

	keyID, err := ids.New("key")
	if err != nil {
		return Result{}, err
	}
	nowFn := b.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	createdAt := nowFn().UTC().Format(time.RFC3339Nano)

	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Result{}, fmt.Errorf("generate ed25519 key: %w", err)
	}

	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return Result{}, fmt.Errorf("marshal private key: %w", err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})

	publicDER, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return Result{}, fmt.Errorf("marshal public key: %w", err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})

	privateName := keyID + ".pem"
	privatePath := filepath.Join(keysDir, privateName)
	publicPath := privatePath + ".pub"

	if err := os.WriteFile(privatePath, privatePEM, 0o600); err != nil {
		return Result{}, fmt.Errorf("write private key file: %w", err)
	}
	if err := os.WriteFile(publicPath, publicPEM, 0o644); err != nil {
		cleanupFiles(privatePath)
		return Result{}, fmt.Errorf("write public key file: %w", err)
	}

	const insertSQL = `
		INSERT INTO keys (
			key_id, owner_type, owner_id, algorithm, public_key,
			private_key_ref, status, published_status, valid_from, created_at
		) VALUES (?, 'self', ?, ?, ?, ?, 'active', 'local_only', ?, ?)
	`
	publicEncoded := base64.RawStdEncoding.EncodeToString(publicKey)
	if _, err := db.ExecContext(ctx, insertSQL, keyID, ownerID, AlgorithmEd25519, publicEncoded, privateName, createdAt, createdAt); err != nil {
		cleanupFiles(privatePath, publicPath)
		return Result{}, fmt.Errorf("insert key metadata: %w", err)
	}

	return Result{
		KeyID:          keyID,
		Algorithm:      AlgorithmEd25519,
		PublicKey:      publicEncoded,
		PrivateKeyPath: privatePath,
		PublicKeyPath:  publicPath,
		Created:        true,
	}, nil
}

func cleanupFiles(paths ...string) {
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			// Best-effort cleanup. The original operation error remains authoritative.
			continue
		}
	}
}
