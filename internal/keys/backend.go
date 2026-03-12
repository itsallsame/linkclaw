package keys

import (
	"context"
	"database/sql"
)

type Backend interface {
	EnsureDefaultKey(ctx context.Context, db *sql.DB, ownerID, keysDir string) (Result, error)
}

type Result struct {
	KeyID          string `json:"key_id"`
	Algorithm      string `json:"algorithm"`
	PublicKey      string `json:"public_key"`
	PrivateKeyPath string `json:"private_key_path"`
	PublicKeyPath  string `json:"public_key_path"`
	Created        bool   `json:"created"`
}
