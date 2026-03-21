package messagingprofile

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/ids"
	"github.com/xiewanpeng/claw-identity/internal/layout"
	"github.com/xiewanpeng/claw-identity/internal/messagecrypto"
)

const EnvRelayURL = "LINKCLAW_RELAY_URL"
const EnvDirectURL = "LINKCLAW_DIRECT_URL"
const EnvDirectToken = "LINKCLAW_DIRECT_TOKEN"

type Profile struct {
	Transport   string `json:"transport"`
	RelayURL    string `json:"relay_url,omitempty"`
	DirectURL   string `json:"direct_url,omitempty"`
	DirectToken string `json:"direct_token,omitempty"`
	RecipientID string `json:"recipient_id"`
}

func EnsureSelfProfile(
	ctx context.Context,
	db *sql.DB,
	selfID,
	signingKeyID,
	home string,
	now time.Time,
) (Profile, string, error) {
	envRelayURL := strings.TrimSpace(os.Getenv(EnvRelayURL))
	envDirectURL := strings.TrimSpace(os.Getenv(EnvDirectURL))
	envDirectToken := strings.TrimSpace(os.Getenv(EnvDirectToken))
	var profile Profile
	var relayURL string
	var directURL string
	var directToken string
	var encryptionPublicKey string
	var privateKeyRef string
	err := db.QueryRowContext(
		ctx,
		`SELECT recipient_id, relay_url, direct_url, direct_token, encryption_public_key, encryption_private_key_ref
		 FROM self_messaging_profiles
		 WHERE self_id = ?
		 LIMIT 1`,
		selfID,
	).Scan(&profile.RecipientID, &relayURL, &directURL, &directToken, &encryptionPublicKey, &privateKeyRef)
	switch {
	case err == nil:
		if (relayURL == "" && envRelayURL != "") || (directURL == "" && envDirectURL != "") || (directToken == "" && envDirectToken != "") {
			if relayURL == "" && envRelayURL != "" {
				relayURL = envRelayURL
			}
			if directURL == "" && envDirectURL != "" {
				directURL = envDirectURL
			}
			if directToken == "" && envDirectToken != "" {
				directToken = envDirectToken
			}
			if _, err := db.ExecContext(
				ctx,
				`UPDATE self_messaging_profiles
				 SET relay_url = ?, direct_url = ?, direct_token = ?, updated_at = ?
				 WHERE self_id = ?`,
				relayURL,
				directURL,
				directToken,
				now.Format(time.RFC3339Nano),
				selfID,
			); err != nil {
				return Profile{}, "", fmt.Errorf("update self messaging transport fields: %w", err)
			}
		}
		if encryptionPublicKey == "" || strings.TrimSpace(privateKeyRef) == "" {
			var ensureErr error
			encryptionPublicKey, privateKeyRef, ensureErr = ensureEncryptionKey(ctx, db, selfID, home, now)
			if ensureErr != nil {
				return Profile{}, "", ensureErr
			}
		}
		profile.Transport = "linkclaw-relay"
		profile.RelayURL = relayURL
		profile.DirectURL = directURL
		profile.DirectToken = directToken
		return profile, encryptionPublicKey, nil
	case err != sql.ErrNoRows:
		return Profile{}, "", fmt.Errorf("query self messaging profile: %w", err)
	}

	recipientID, err := ids.New("rcpt")
	if err != nil {
		return Profile{}, "", err
	}
	encryptionPublicKey, privateKeyRef, err = createEncryptionKey(home, selfID)
	if err != nil {
		return Profile{}, "", err
	}
	stamp := now.Format(time.RFC3339Nano)
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO self_messaging_profiles (
			self_id, recipient_id, relay_url, direct_url, direct_token, signing_key_id, encryption_public_key,
			encryption_private_key_ref, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		selfID,
		recipientID,
		envRelayURL,
		envDirectURL,
		envDirectToken,
		signingKeyID,
		encryptionPublicKey,
		privateKeyRef,
		stamp,
		stamp,
	); err != nil {
		return Profile{}, "", fmt.Errorf("insert self messaging profile: %w", err)
	}
	return Profile{
		Transport:   "linkclaw-relay",
		RelayURL:    envRelayURL,
		DirectURL:   envDirectURL,
		DirectToken: envDirectToken,
		RecipientID: recipientID,
	}, encryptionPublicKey, nil
}

func ensureEncryptionKey(ctx context.Context, db *sql.DB, selfID, home string, now time.Time) (string, string, error) {
	encryptionPublicKey, privateKeyRef, err := createEncryptionKey(home, selfID)
	if err != nil {
		return "", "", err
	}
	if _, err := db.ExecContext(
		ctx,
		`UPDATE self_messaging_profiles
		 SET encryption_public_key = ?, encryption_private_key_ref = ?, updated_at = ?
		 WHERE self_id = ?`,
		encryptionPublicKey,
		privateKeyRef,
		now.Format(time.RFC3339Nano),
		selfID,
	); err != nil {
		return "", "", fmt.Errorf("update messaging encryption key: %w", err)
	}
	return encryptionPublicKey, privateKeyRef, nil
}

func createEncryptionKey(home, selfID string) (string, string, error) {
	publicKeyBase64, privateKeyBase64, err := messagecrypto.GenerateX25519KeyPair()
	if err != nil {
		return "", "", err
	}
	keyFileName := fmt.Sprintf("%s.messaging.x25519", selfID)
	privateKeyPath := filepath.Join(layout.BuildPaths(home).KeysDir, keyFileName)
	if err := messagecrypto.SaveBase64File(privateKeyPath, privateKeyBase64, 0o600); err != nil {
		return "", "", fmt.Errorf("write x25519 private key file: %w", err)
	}
	return publicKeyBase64, keyFileName, nil
}
