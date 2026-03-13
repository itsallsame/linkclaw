package publish

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/initflow"

	_ "modernc.org/sqlite"
)

func TestServicePublishTierBundles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fixedNow := time.Date(2026, time.March, 13, 8, 30, 0, 0, time.UTC)

	testCases := []struct {
		name          string
		tier          string
		expectedPaths []string
		missingPaths  []string
	}{
		{
			name:          "minimum",
			tier:          TierMinimum,
			expectedPaths: []string{".well-known/did.json"},
			missingPaths:  []string{".well-known/webfinger", ".well-known/agent-card.json", "profile/index.html"},
		},
		{
			name:          "recommended",
			tier:          TierRecommended,
			expectedPaths: []string{".well-known/did.json", ".well-known/webfinger", ".well-known/agent-card.json"},
			missingPaths:  []string{"profile/index.html"},
		},
		{
			name:          "full",
			tier:          TierFull,
			expectedPaths: []string{".well-known/did.json", ".well-known/webfinger", ".well-known/agent-card.json", "profile/index.html"},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			home := seedPublishHome(t, ctx)
			outputDir := filepath.Join(t.TempDir(), tc.name)
			service := NewService()
			service.Now = func() time.Time { return fixedNow }

			result, err := service.Publish(ctx, Options{
				Home:   home,
				Origin: "https://agent.example",
				Output: outputDir,
				Tier:   tc.tier,
			})
			if err != nil {
				t.Fatalf("Publish returned error: %v", err)
			}

			if result.Tier != tc.tier {
				t.Fatalf("tier = %q, want %q", result.Tier, tc.tier)
			}
			if result.CanonicalID != "did:web:agent.example" {
				t.Fatalf("canonical id = %q", result.CanonicalID)
			}
			if result.HomeOrigin != "https://agent.example" {
				t.Fatalf("home origin = %q", result.HomeOrigin)
			}
			if result.HeadersPath != filepath.Join(outputDir, HeadersFilePath) {
				t.Fatalf("headers path = %q", result.HeadersPath)
			}
			if len(result.Artifacts) != len(tc.expectedPaths) {
				t.Fatalf("artifact count = %d, want %d", len(result.Artifacts), len(tc.expectedPaths))
			}

			for _, check := range result.Checks {
				if !check.OK {
					t.Fatalf("expected check %q to pass: %+v", check.Name, check)
				}
			}

			for _, relPath := range tc.expectedPaths {
				if _, err := os.Stat(filepath.Join(outputDir, filepath.FromSlash(relPath))); err != nil {
					t.Fatalf("expected artifact %q to exist: %v", relPath, err)
				}
			}
			for _, relPath := range tc.missingPaths {
				if _, err := os.Stat(filepath.Join(outputDir, filepath.FromSlash(relPath))); !os.IsNotExist(err) {
					t.Fatalf("expected artifact %q to be absent, err=%v", relPath, err)
				}
			}

			manifest := readManifest(t, filepath.Join(outputDir, "manifest.json"))
			if manifest.Tier != tc.tier {
				t.Fatalf("manifest tier = %q, want %q", manifest.Tier, tc.tier)
			}
			if manifest.CanonicalID != "did:web:agent.example" {
				t.Fatalf("manifest canonical id = %q", manifest.CanonicalID)
			}
			if manifest.HomeOrigin != "https://agent.example" {
				t.Fatalf("manifest home origin = %q", manifest.HomeOrigin)
			}
			if len(manifest.Artifacts) != len(tc.expectedPaths) {
				t.Fatalf("manifest artifact count = %d, want %d", len(manifest.Artifacts), len(tc.expectedPaths))
			}
			if len(manifest.ContentHashes) != len(tc.expectedPaths) {
				t.Fatalf("manifest hash count = %d, want %d", len(manifest.ContentHashes), len(tc.expectedPaths))
			}
			for _, relPath := range tc.expectedPaths {
				hash := manifest.ContentHashes[relPath]
				if hash == "" {
					t.Fatalf("expected content hash for %q", relPath)
				}
				if len(hash) < len("sha256:")+64 || hash[:7] != "sha256:" {
					t.Fatalf("unexpected hash format for %q: %q", relPath, hash)
				}
			}

			headersContent, err := os.ReadFile(filepath.Join(outputDir, HeadersFilePath))
			if err != nil {
				t.Fatalf("read _headers: %v", err)
			}
			headersText := string(headersContent)
			if !strings.Contains(headersText, "/.well-known/webfinger") {
				t.Fatalf("expected webfinger route in _headers: %s", headersText)
			}
			if !strings.Contains(headersText, "Content-Type: application/json") {
				t.Fatalf("expected application/json content type in _headers: %s", headersText)
			}

			didContent, err := os.ReadFile(filepath.Join(outputDir, ".well-known", "did.json"))
			if err != nil {
				t.Fatalf("read did.json: %v", err)
			}
			var did didDocument
			if err := json.Unmarshal(didContent, &did); err != nil {
				t.Fatalf("unmarshal did.json: %v", err)
			}
			if did.ID != "did:web:agent.example" {
				t.Fatalf("did id = %q", did.ID)
			}
			if len(did.Authentication) == 0 {
				t.Fatalf("expected at least one active authentication key")
			}

			if tc.tier != TierMinimum {
				webFingerContent, err := os.ReadFile(filepath.Join(outputDir, ".well-known", "webfinger"))
				if err != nil {
					t.Fatalf("read webfinger: %v", err)
				}
				var webFinger webFingerDocument
				if err := json.Unmarshal(webFingerContent, &webFinger); err != nil {
					t.Fatalf("unmarshal webfinger: %v", err)
				}
				if webFinger.Subject != "https://agent.example/" {
					t.Fatalf("webfinger subject = %q", webFinger.Subject)
				}

				agentCardContent, err := os.ReadFile(filepath.Join(outputDir, ".well-known", "agent-card.json"))
				if err != nil {
					t.Fatalf("read agent-card.json: %v", err)
				}
				var card agentCardDocument
				if err := json.Unmarshal(agentCardContent, &card); err != nil {
					t.Fatalf("unmarshal agent-card.json: %v", err)
				}
				if card.CanonicalID != "did:web:agent.example" {
					t.Fatalf("agent card canonical id = %q", card.CanonicalID)
				}
				if card.DIDURL != "https://agent.example/.well-known/did.json" {
					t.Fatalf("agent card did url = %q", card.DIDURL)
				}
			}

			if tc.tier == TierFull {
				profileContent, err := os.ReadFile(filepath.Join(outputDir, "profile", "index.html"))
				if err != nil {
					t.Fatalf("read profile/index.html: %v", err)
				}
				if string(profileContent) == "" {
					t.Fatalf("expected non-empty profile page")
				}
			}

			db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
			if err != nil {
				t.Fatalf("open sqlite db: %v", err)
			}
			defer db.Close()

			var homeOrigin, profileURL string
			if err := db.QueryRow(`SELECT home_origin, default_profile_url FROM self_identities LIMIT 1`).Scan(&homeOrigin, &profileURL); err != nil {
				t.Fatalf("query self identity urls: %v", err)
			}
			if homeOrigin != "https://agent.example" {
				t.Fatalf("persisted home_origin = %q", homeOrigin)
			}
			if profileURL != "https://agent.example/profile/" {
				t.Fatalf("persisted default_profile_url = %q", profileURL)
			}
		})
	}
}

func seedPublishHome(t *testing.T, ctx context.Context) string {
	t.Helper()

	home := filepath.Join(t.TempDir(), "linkclaw-home")
	service := initflow.NewService()
	if _, err := service.Init(ctx, initflow.Options{
		Home:        home,
		CanonicalID: "did:web:agent.example",
		DisplayName: "Agent Example",
	}); err != nil {
		t.Fatalf("init home: %v", err)
	}
	return home
}

func readManifest(t *testing.T, path string) Manifest {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}

	var manifest Manifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	return manifest
}
