package initflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/didkey"
	"github.com/xiewanpeng/claw-identity/internal/ids"
	"github.com/xiewanpeng/claw-identity/internal/keys"
	"github.com/xiewanpeng/claw-identity/internal/layout"
	"github.com/xiewanpeng/claw-identity/internal/messagingprofile"
	"github.com/xiewanpeng/claw-identity/internal/migrate"

	_ "modernc.org/sqlite"
)

type Options struct {
	Home        string
	CanonicalID string
	DisplayName string
}

type DirectoryStatus struct {
	Path    string `json:"path"`
	Created bool   `json:"created"`
}

type IdentityStatus struct {
	SelfID      string `json:"self_id"`
	CanonicalID string `json:"canonical_id"`
	DisplayName string `json:"display_name"`
	Created     bool   `json:"created"`
}

type MessagingStatus struct {
	Transport   string `json:"transport"`
	RecipientID string `json:"recipient_id"`
	Created     bool   `json:"created"`
	Ready       bool   `json:"ready"`
}

type Result struct {
	Home        string            `json:"home"`
	DBPath      string            `json:"db_path"`
	Directories []DirectoryStatus `json:"directories"`
	Migrations  []migrate.Step    `json:"migrations"`
	Identity    IdentityStatus    `json:"identity"`
	Key         keys.Result       `json:"key"`
	Messaging   MessagingStatus   `json:"messaging"`
	GeneratedAt string            `json:"generated_at"`
}

type Service struct {
	KeyBackend keys.Backend
	Now        func() time.Time
}

func NewService() *Service {
	return &Service{
		KeyBackend: keys.NewFileBackend(),
		Now:        time.Now,
	}
}

func (s *Service) Init(ctx context.Context, opts Options) (Result, error) {
	canonicalID := strings.TrimSpace(opts.CanonicalID)
	if s.KeyBackend == nil {
		return Result{}, errors.New("key backend is not configured")
	}
	if s.Now == nil {
		s.Now = time.Now
	}

	home, err := layout.ResolveHome(opts.Home)
	if err != nil {
		return Result{}, err
	}
	layoutResult, err := layout.Ensure(home)
	if err != nil {
		return Result{}, err
	}

	dirCreated := map[string]bool{}
	for _, path := range layoutResult.Created {
		dirCreated[path] = true
	}
	directories := []DirectoryStatus{
		{Path: layoutResult.Paths.Home, Created: dirCreated[layoutResult.Paths.Home]},
		{Path: layoutResult.Paths.KeysDir, Created: dirCreated[layoutResult.Paths.KeysDir]},
		{Path: layoutResult.Paths.BlobsDir, Created: dirCreated[layoutResult.Paths.BlobsDir]},
		{Path: layoutResult.Paths.CacheDir, Created: dirCreated[layoutResult.Paths.CacheDir]},
	}

	db, err := sql.Open("sqlite", layoutResult.Paths.DB)
	if err != nil {
		return Result{}, fmt.Errorf("open sqlite database: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return Result{}, fmt.Errorf("ping sqlite database: %w", err)
	}

	now := s.Now()
	migrations, err := migrate.Apply(ctx, db, now)
	if err != nil {
		return Result{}, err
	}

	displayName := strings.TrimSpace(opts.DisplayName)
	var identity IdentityStatus
	var keyResult keys.Result
	var messagingStatus MessagingStatus

	if canonicalID == "" {
		identity, err = findSelfIdentity(ctx, db)
		if err != nil {
			return Result{}, err
		}
		if identity.SelfID != "" {
			identity, err = updateDisplayName(ctx, db, now, identity, displayName)
			if err != nil {
				return Result{}, err
			}
			keyResult, err = s.KeyBackend.EnsureDefaultKey(ctx, db, identity.SelfID, layoutResult.Paths.KeysDir)
			if err != nil {
				return Result{}, err
			}
			profile, _, ensureErr := messagingprofile.EnsureSelfProfile(
				ctx,
				db,
				identity.SelfID,
				keyResult.KeyID,
				layoutResult.Paths.Home,
				now,
			)
			if ensureErr != nil {
				return Result{}, ensureErr
			}
			messagingStatus = buildMessagingStatus(profile, false)
			return Result{
				Home:        layoutResult.Paths.Home,
				DBPath:      layoutResult.Paths.DB,
				Directories: directories,
				Migrations:  migrations,
				Identity:    identity,
				Key:         keyResult,
				Messaging:   messagingStatus,
				GeneratedAt: now.UTC().Format(time.RFC3339Nano),
			}, nil
		}

		selfID, err := ids.New("self")
		if err != nil {
			return Result{}, err
		}
		keyResult, err = s.KeyBackend.EnsureDefaultKey(ctx, db, selfID, layoutResult.Paths.KeysDir)
		if err != nil {
			return Result{}, err
		}
		canonicalID, err = didkey.FromBase64PublicKey(keyResult.PublicKey)
		if err != nil {
			return Result{}, fmt.Errorf("derive did:key from default key: %w", err)
		}
		identity, err = insertSelfIdentity(ctx, db, now, selfID, canonicalID, displayName)
		if err != nil {
			return Result{}, err
		}
		messagingStatus.Created = true
	} else {
		identity, err = ensureSelfIdentity(ctx, db, now, canonicalID, displayName)
		if err != nil {
			return Result{}, err
		}
		keyResult, err = s.KeyBackend.EnsureDefaultKey(ctx, db, identity.SelfID, layoutResult.Paths.KeysDir)
		if err != nil {
			return Result{}, err
		}
	}

	profile, _, err := messagingprofile.EnsureSelfProfile(
		ctx,
		db,
		identity.SelfID,
		keyResult.KeyID,
		layoutResult.Paths.Home,
		now,
	)
	if err != nil {
		return Result{}, err
	}
	messagingStatus = buildMessagingStatus(profile, messagingStatus.Created)

	return Result{
		Home:        layoutResult.Paths.Home,
		DBPath:      layoutResult.Paths.DB,
		Directories: directories,
		Migrations:  migrations,
		Identity:    identity,
		Key:         keyResult,
		Messaging:   messagingStatus,
		GeneratedAt: now.UTC().Format(time.RFC3339Nano),
	}, nil
}

func buildMessagingStatus(profile messagingprofile.Profile, created bool) MessagingStatus {
	return MessagingStatus{
		Transport:   profile.Transport,
		RecipientID: profile.RecipientID,
		Created:     created,
		Ready:       strings.TrimSpace(profile.RecipientID) != "",
	}
}

func ensureSelfIdentity(ctx context.Context, db *sql.DB, now time.Time, canonicalID, displayName string) (IdentityStatus, error) {
	if displayName == "" {
		displayName = canonicalID
	}

	const selectSQL = `
		SELECT self_id, canonical_id, display_name
		FROM self_identities
		WHERE canonical_id = ?
		LIMIT 1
	`

	var status IdentityStatus
	err := db.QueryRowContext(ctx, selectSQL, canonicalID).Scan(&status.SelfID, &status.CanonicalID, &status.DisplayName)
	switch {
	case err == nil:
		status.Created = false
		if displayName != "" && displayName != status.DisplayName {
			if _, err := db.ExecContext(
				ctx,
				"UPDATE self_identities SET display_name = ?, updated_at = ? WHERE self_id = ?",
				displayName,
				now.UTC().Format(time.RFC3339Nano),
				status.SelfID,
			); err != nil {
				return IdentityStatus{}, fmt.Errorf("update display name: %w", err)
			}
			status.DisplayName = displayName
		}
		return status, nil
	case !errors.Is(err, sql.ErrNoRows):
		return IdentityStatus{}, fmt.Errorf("query self identity: %w", err)
	}

	selfID, err := ids.New("self")
	if err != nil {
		return IdentityStatus{}, err
	}
	return insertSelfIdentity(ctx, db, now, selfID, canonicalID, displayName)
}

func findSelfIdentity(ctx context.Context, db *sql.DB) (IdentityStatus, error) {
	const selectSQL = `
		SELECT self_id, canonical_id, display_name
		FROM self_identities
		ORDER BY created_at ASC
		LIMIT 1
	`

	var status IdentityStatus
	err := db.QueryRowContext(ctx, selectSQL).Scan(&status.SelfID, &status.CanonicalID, &status.DisplayName)
	switch {
	case err == nil:
		status.Created = false
		return status, nil
	case errors.Is(err, sql.ErrNoRows):
		return IdentityStatus{}, nil
	default:
		return IdentityStatus{}, fmt.Errorf("query self identity: %w", err)
	}
}

func updateDisplayName(ctx context.Context, db *sql.DB, now time.Time, status IdentityStatus, displayName string) (IdentityStatus, error) {
	if strings.TrimSpace(displayName) == "" || displayName == status.DisplayName {
		return status, nil
	}
	if _, err := db.ExecContext(
		ctx,
		"UPDATE self_identities SET display_name = ?, updated_at = ? WHERE self_id = ?",
		displayName,
		now.UTC().Format(time.RFC3339Nano),
		status.SelfID,
	); err != nil {
		return IdentityStatus{}, fmt.Errorf("update display name: %w", err)
	}
	status.DisplayName = displayName
	return status, nil
}

func insertSelfIdentity(ctx context.Context, db *sql.DB, now time.Time, selfID, canonicalID, displayName string) (IdentityStatus, error) {
	if displayName == "" {
		displayName = canonicalID
	}
	stamp := now.UTC().Format(time.RFC3339Nano)

	const insertSQL = `
		INSERT INTO self_identities (
			self_id, canonical_id, display_name, description,
			home_origin, default_profile_url, status, created_at, updated_at
		) VALUES (?, ?, ?, '', '', '', 'active', ?, ?)
	`
	if _, err := db.ExecContext(ctx, insertSQL, selfID, canonicalID, displayName, stamp, stamp); err != nil {
		return IdentityStatus{}, fmt.Errorf("insert self identity: %w", err)
	}

	return IdentityStatus{
		SelfID:      selfID,
		CanonicalID: canonicalID,
		DisplayName: displayName,
		Created:     true,
	}, nil
}
