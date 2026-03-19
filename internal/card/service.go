package card

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/ids"
	"github.com/xiewanpeng/claw-identity/internal/layout"
	"github.com/xiewanpeng/claw-identity/internal/messagecrypto"
	"github.com/xiewanpeng/claw-identity/internal/migrate"

	_ "modernc.org/sqlite"
)

const SchemaVersion = "linkclaw.identity_card.v1"
const EnvRelayURL = "LINKCLAW_RELAY_URL"

type Options struct {
	Home string
}

type VerifyOptions struct {
	Input string
}

type ImportOptions struct {
	Home  string
	Input string
}

type MessagingProfile struct {
	Transport   string `json:"transport"`
	RelayURL    string `json:"relay_url,omitempty"`
	RecipientID string `json:"recipient_id"`
}

type Card struct {
	SchemaVersion string           `json:"schema_version"`
	ID            string           `json:"id"`
	DisplayName   string           `json:"display_name"`
	CreatedAt     string           `json:"created_at"`
	SigningKey    string           `json:"signing_public_key"`
	EncryptionKey string           `json:"encryption_public_key"`
	Messaging     MessagingProfile `json:"messaging"`
	Signature     string           `json:"signature"`
}

type ExportResult struct {
	Home string `json:"home"`
	Card Card   `json:"card"`
}

type VerifyResult struct {
	Verified bool   `json:"verified"`
	Card     Card   `json:"card"`
	Source   string `json:"source"`
}

type ImportResult struct {
	Home      string `json:"home"`
	ContactID string `json:"contact_id"`
	Created   bool   `json:"created"`
	Card      Card   `json:"card"`
	Source    string `json:"source"`
}

type Service struct {
	Now func() time.Time
}

func NewService() *Service {
	return &Service{Now: time.Now}
}

func (s *Service) Export(ctx context.Context, opts Options) (ExportResult, error) {
	db, home, now, err := s.openStateDB(ctx, opts.Home)
	if err != nil {
		return ExportResult{}, err
	}
	defer db.Close()

	selfID, canonicalID, displayName, createdAt, err := loadSelfIdentity(ctx, db)
	if err != nil {
		return ExportResult{}, err
	}
	signingKeyID, signingPublicKey, privateKeyPath, err := loadSigningKey(ctx, db, selfID, home)
	if err != nil {
		return ExportResult{}, err
	}
	profile, encryptionPublicKey, err := ensureMessagingProfile(ctx, db, selfID, signingKeyID, home, now)
	if err != nil {
		return ExportResult{}, err
	}

	card := Card{
		SchemaVersion: SchemaVersion,
		ID:            canonicalID,
		DisplayName:   displayName,
		CreatedAt:     createdAt,
		SigningKey:    signingPublicKey,
		EncryptionKey: encryptionPublicKey,
		Messaging:     profile,
	}
	signature, err := signCard(card, privateKeyPath)
	if err != nil {
		return ExportResult{}, err
	}
	card.Signature = signature

	return ExportResult{
		Home: home,
		Card: card,
	}, nil
}

func (s *Service) Verify(_ context.Context, opts VerifyOptions) (VerifyResult, error) {
	source := strings.TrimSpace(opts.Input)
	if source == "" {
		return VerifyResult{}, fmt.Errorf("identity card input is required")
	}
	card, resolvedSource, err := parseCardInput(source)
	if err != nil {
		return VerifyResult{}, err
	}
	if err := verifyCard(card); err != nil {
		return VerifyResult{}, err
	}
	return VerifyResult{
		Verified: true,
		Card:     card,
		Source:   resolvedSource,
	}, nil
}

func (s *Service) Import(ctx context.Context, opts ImportOptions) (ImportResult, error) {
	db, home, now, err := s.openStateDB(ctx, opts.Home)
	if err != nil {
		return ImportResult{}, err
	}
	defer db.Close()

	card, source, err := parseCardInput(strings.TrimSpace(opts.Input))
	if err != nil {
		return ImportResult{}, err
	}
	if err := verifyCard(card); err != nil {
		return ImportResult{}, err
	}

	rawCard, err := json.Marshal(card)
	if err != nil {
		return ImportResult{}, fmt.Errorf("encode verified identity card: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ImportResult{}, fmt.Errorf("begin contact import transaction: %w", err)
	}
	defer tx.Rollback()

	contactID, created, err := upsertContactFromCard(ctx, tx, card, string(rawCard), now)
	if err != nil {
		return ImportResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ImportResult{}, fmt.Errorf("commit contact import transaction: %w", err)
	}

	return ImportResult{
		Home:      home,
		ContactID: contactID,
		Created:   created,
		Card:      card,
		Source:    source,
	}, nil
}

func (s *Service) openStateDB(ctx context.Context, rawHome string) (*sql.DB, string, time.Time, error) {
	nowFn := s.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	now := nowFn().UTC()

	home, err := layout.ResolveHome(rawHome)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	if _, err := layout.Ensure(home); err != nil {
		return nil, "", time.Time{}, err
	}
	paths := layout.BuildPaths(home)

	db, err := sql.Open("sqlite", paths.DB)
	if err != nil {
		return nil, "", time.Time{}, fmt.Errorf("open sqlite database: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, "", time.Time{}, fmt.Errorf("ping sqlite database: %w", err)
	}
	if _, err := migrate.Apply(ctx, db, now); err != nil {
		db.Close()
		return nil, "", time.Time{}, fmt.Errorf("apply migrations: %w", err)
	}
	return db, home, now, nil
}

func loadSelfIdentity(ctx context.Context, db *sql.DB) (selfID, canonicalID, displayName, createdAt string, err error) {
	err = db.QueryRowContext(
		ctx,
		`SELECT self_id, canonical_id, display_name, created_at
		 FROM self_identities
		 ORDER BY created_at ASC
		 LIMIT 1`,
	).Scan(&selfID, &canonicalID, &displayName, &createdAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", "", "", fmt.Errorf("self identity not found; run linkclaw init first")
		}
		return "", "", "", "", fmt.Errorf("query self identity: %w", err)
	}
	return selfID, canonicalID, displayName, createdAt, nil
}

func loadSigningKey(ctx context.Context, db *sql.DB, selfID, home string) (keyID, publicKey, privateKeyPath string, err error) {
	err = db.QueryRowContext(
		ctx,
		`SELECT key_id, public_key, private_key_ref
		 FROM keys
		 WHERE owner_type = 'self' AND owner_id = ? AND status = 'active'
		 ORDER BY created_at ASC
		 LIMIT 1`,
		selfID,
	).Scan(&keyID, &publicKey, &privateKeyPath)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", "", "", fmt.Errorf("active signing key not found; run linkclaw init first")
		}
		return "", "", "", fmt.Errorf("query active signing key: %w", err)
	}
	if !filepath.IsAbs(privateKeyPath) {
		privateKeyPath = filepath.Join(layout.BuildPaths(home).KeysDir, privateKeyPath)
	}
	return keyID, publicKey, privateKeyPath, nil
}

func ensureMessagingProfile(ctx context.Context, db *sql.DB, selfID, signingKeyID, home string, now time.Time) (MessagingProfile, string, error) {
	envRelayURL := strings.TrimSpace(os.Getenv(EnvRelayURL))
	var profile MessagingProfile
	var relayURL string
	var encryptionPublicKey string
	var privateKeyRef string
	err := db.QueryRowContext(
		ctx,
		`SELECT recipient_id, relay_url, encryption_public_key, encryption_private_key_ref
		 FROM self_messaging_profiles
		 WHERE self_id = ?
		 LIMIT 1`,
		selfID,
	).Scan(&profile.RecipientID, &relayURL, &encryptionPublicKey, &privateKeyRef)
	switch {
	case err == nil:
		if relayURL == "" && envRelayURL != "" {
			relayURL = envRelayURL
			if _, err := db.ExecContext(
				ctx,
				`UPDATE self_messaging_profiles
				 SET relay_url = ?, updated_at = ?
				 WHERE self_id = ?`,
				relayURL,
				now.Format(time.RFC3339Nano),
				selfID,
			); err != nil {
				return MessagingProfile{}, "", fmt.Errorf("update self messaging relay url: %w", err)
			}
		}
		if encryptionPublicKey == "" || strings.TrimSpace(privateKeyRef) == "" {
			var ensureErr error
			encryptionPublicKey, privateKeyRef, ensureErr = ensureMessagingEncryptionKey(ctx, db, selfID, home, now)
			if ensureErr != nil {
				return MessagingProfile{}, "", ensureErr
			}
		}
		profile.Transport = "linkclaw-relay"
		profile.RelayURL = relayURL
		return profile, encryptionPublicKey, nil
	case err != sql.ErrNoRows:
		return MessagingProfile{}, "", fmt.Errorf("query self messaging profile: %w", err)
	}

	recipientID, err := ids.New("rcpt")
	if err != nil {
		return MessagingProfile{}, "", err
	}
	encryptionPublicKey, privateKeyRef, err = createMessagingEncryptionKey(home, selfID)
	if err != nil {
		return MessagingProfile{}, "", err
	}
	stamp := now.Format(time.RFC3339Nano)
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO self_messaging_profiles (
			self_id, recipient_id, relay_url, signing_key_id, encryption_public_key,
			encryption_private_key_ref, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		selfID,
		recipientID,
		envRelayURL,
		signingKeyID,
		encryptionPublicKey,
		privateKeyRef,
		stamp,
		stamp,
	); err != nil {
		return MessagingProfile{}, "", fmt.Errorf("insert self messaging profile: %w", err)
	}
	return MessagingProfile{
		Transport:   "linkclaw-relay",
		RelayURL:    envRelayURL,
		RecipientID: recipientID,
	}, encryptionPublicKey, nil
}

func signCard(card Card, privateKeyPath string) (string, error) {
	payload, err := unsignedPayload(card)
	if err != nil {
		return "", err
	}
	return messagecrypto.SignPayload(privateKeyPath, payload)
}

func verifyCard(card Card) error {
	if strings.TrimSpace(card.Signature) == "" {
		return fmt.Errorf("identity card signature is required")
	}
	if strings.TrimSpace(card.SchemaVersion) != SchemaVersion {
		return fmt.Errorf("unsupported identity card schema %q", card.SchemaVersion)
	}
	payload, err := unsignedPayload(card)
	if err != nil {
		return err
	}
	if err := messagecrypto.VerifyPayload(card.SigningKey, payload, card.Signature); err != nil {
		return fmt.Errorf("identity card %w", err)
	}
	return nil
}

func unsignedPayload(card Card) ([]byte, error) {
	payload := struct {
		SchemaVersion string           `json:"schema_version"`
		ID            string           `json:"id"`
		DisplayName   string           `json:"display_name"`
		CreatedAt     string           `json:"created_at"`
		SigningKey    string           `json:"signing_public_key"`
		EncryptionKey string           `json:"encryption_public_key"`
		Messaging     MessagingProfile `json:"messaging"`
	}{
		SchemaVersion: strings.TrimSpace(card.SchemaVersion),
		ID:            strings.TrimSpace(card.ID),
		DisplayName:   strings.TrimSpace(card.DisplayName),
		CreatedAt:     strings.TrimSpace(card.CreatedAt),
		SigningKey:    strings.TrimSpace(card.SigningKey),
		EncryptionKey: strings.TrimSpace(card.EncryptionKey),
		Messaging: MessagingProfile{
			Transport:   strings.TrimSpace(card.Messaging.Transport),
			RelayURL:    strings.TrimSpace(card.Messaging.RelayURL),
			RecipientID: strings.TrimSpace(card.Messaging.RecipientID),
		},
	}
	if payload.ID == "" {
		return nil, fmt.Errorf("identity card id is required")
	}
	if payload.DisplayName == "" {
		return nil, fmt.Errorf("identity card display_name is required")
	}
	if payload.CreatedAt == "" {
		return nil, fmt.Errorf("identity card created_at is required")
	}
	if payload.SigningKey == "" {
		return nil, fmt.Errorf("identity card signing_public_key is required")
	}
	if payload.EncryptionKey == "" {
		return nil, fmt.Errorf("identity card encryption_public_key is required")
	}
	if payload.Messaging.Transport == "" {
		return nil, fmt.Errorf("identity card messaging.transport is required")
	}
	if payload.Messaging.RecipientID == "" {
		return nil, fmt.Errorf("identity card messaging.recipient_id is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal identity card payload: %w", err)
	}
	return encoded, nil
}

func parseCardInput(input string) (Card, string, error) {
	if info, err := os.Stat(input); err == nil && !info.IsDir() {
		data, err := os.ReadFile(input)
		if err != nil {
			return Card{}, "", fmt.Errorf("read identity card file: %w", err)
		}
		var card Card
		if err := json.Unmarshal(data, &card); err != nil {
			return Card{}, "", fmt.Errorf("decode identity card file: %w", err)
		}
		return card, input, nil
	}

	var card Card
	if err := json.Unmarshal([]byte(input), &card); err != nil {
		return Card{}, "", fmt.Errorf("decode identity card input: %w", err)
	}
	return card, "inline", nil
}

func upsertContactFromCard(ctx context.Context, tx *sql.Tx, card Card, rawCard string, now time.Time) (string, bool, error) {
	var contactID string
	err := tx.QueryRowContext(
		ctx,
		`SELECT contact_id
		 FROM contacts
		 WHERE canonical_id = ?
		 LIMIT 1`,
		card.ID,
	).Scan(&contactID)
	switch {
	case err == nil:
		if _, err := tx.ExecContext(
			ctx,
			`UPDATE contacts
			 SET display_name = ?,
			     status = 'imported',
			     signing_public_key = ?,
			     encryption_public_key = ?,
			     relay_url = ?,
			     recipient_id = ?,
			     raw_identity_card_json = ?
			 WHERE contact_id = ?`,
			card.DisplayName,
			card.SigningKey,
			card.EncryptionKey,
			card.Messaging.RelayURL,
			card.Messaging.RecipientID,
			rawCard,
			contactID,
		); err != nil {
			return "", false, fmt.Errorf("update imported contact: %w", err)
		}
		return contactID, false, nil
	case err != sql.ErrNoRows:
		return "", false, fmt.Errorf("query existing imported contact: %w", err)
	}

	contactID, err = ids.New("contact")
	if err != nil {
		return "", false, err
	}
	stamp := now.Format(time.RFC3339Nano)
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO contacts (
			contact_id, canonical_id, display_name, status, created_at,
			signing_public_key, encryption_public_key, relay_url, recipient_id, raw_identity_card_json
		) VALUES (?, ?, ?, 'imported', ?, ?, ?, ?, ?, ?)`,
		contactID,
		card.ID,
		card.DisplayName,
		stamp,
		card.SigningKey,
		card.EncryptionKey,
		card.Messaging.RelayURL,
		card.Messaging.RecipientID,
		rawCard,
	); err != nil {
		return "", false, fmt.Errorf("insert imported contact: %w", err)
	}
	return contactID, true, nil
}

func ensureMessagingEncryptionKey(ctx context.Context, db *sql.DB, selfID, home string, now time.Time) (string, string, error) {
	encryptionPublicKey, privateKeyRef, err := createMessagingEncryptionKey(home, selfID)
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

func createMessagingEncryptionKey(home, selfID string) (string, string, error) {
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
