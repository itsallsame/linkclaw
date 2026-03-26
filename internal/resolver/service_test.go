package resolver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestServiceInspectFixtures(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name            string
		fixture         string
		inputPath       string
		wantStatus      string
		wantCanonicalID string
		wantDisplayName string
		wantProofs      int
		wantSuccesses   int
	}{
		{
			name:            "consistent profile entrypoint",
			fixture:         "consistent",
			inputPath:       "/profile/",
			wantStatus:      StatusConsistent,
			wantCanonicalID: "did:web:fixture.example",
			wantDisplayName: "Fixture Agent",
			wantProofs:      5,
			wantSuccesses:   4,
		},
		{
			name:            "resolved did only",
			fixture:         "did-only",
			inputPath:       "",
			wantStatus:      StatusResolved,
			wantCanonicalID: "did:web:fixture.example",
			wantSuccesses:   1,
		},
		{
			name:            "discovered card only",
			fixture:         "card-only",
			inputPath:       "",
			wantStatus:      StatusDiscovered,
			wantCanonicalID: "",
			wantDisplayName: "Card Only Agent",
			wantSuccesses:   1,
		},
		{
			name:            "mismatch card canonical id",
			fixture:         "mismatch-card",
			inputPath:       "",
			wantStatus:      StatusMismatch,
			wantCanonicalID: "did:web:fixture.example",
			wantDisplayName: "Mismatch Agent",
			wantSuccesses:   2,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := newFixtureServer(t, tc.fixture)
			defer server.Close()

			service := NewService()
			service.Client = server.Client()
			service.Now = func() time.Time { return time.Date(2026, time.March, 13, 9, 0, 0, 0, time.UTC) }

			result, err := service.Inspect(context.Background(), server.URL+tc.inputPath)
			if err != nil {
				t.Fatalf("Inspect returned error: %v", err)
			}

			if result.Status != tc.wantStatus {
				t.Fatalf("status = %q, want %q", result.Status, tc.wantStatus)
			}
			if result.CanonicalID != tc.wantCanonicalID {
				t.Fatalf("canonical id = %q, want %q", result.CanonicalID, tc.wantCanonicalID)
			}
			if tc.wantDisplayName != "" && result.DisplayName != tc.wantDisplayName {
				t.Fatalf("display name = %q, want %q", result.DisplayName, tc.wantDisplayName)
			}
			if got := successfulArtifacts(result.Artifacts); got != tc.wantSuccesses {
				t.Fatalf("successful artifacts = %d, want %d; artifacts=%+v", got, tc.wantSuccesses, result.Artifacts)
			}
			if tc.wantProofs > 0 && len(result.Proofs) != tc.wantProofs {
				t.Fatalf("proof count = %d, want %d; proofs=%+v", len(result.Proofs), tc.wantProofs, result.Proofs)
			}
			if tc.wantStatus == StatusMismatch && len(result.Mismatches) == 0 {
				t.Fatalf("expected mismatch details, got %+v", result)
			}
			if tc.wantStatus == StatusConsistent && len(result.Warnings) != 0 {
				t.Fatalf("expected no warnings, got %+v", result.Warnings)
			}
		})
	}
}

func successfulArtifacts(artifacts []Artifact) int {
	count := 0
	for _, artifact := range artifacts {
		if artifact.OK {
			count++
		}
	}
	return count
}

func TestServiceInspectExtractsCapabilityHintsFromAgentCard(t *testing.T) {
	t.Parallel()

	server := newFixtureServer(t, "with-capabilities")
	defer server.Close()

	service := NewService()
	service.Client = server.Client()
	service.Now = func() time.Time { return time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC) }

	result, err := service.Inspect(context.Background(), server.URL+"/profile/")
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if result.PeerID != "lcpeer:fixture-cap" {
		t.Fatalf("peer_id = %q, want lcpeer:fixture-cap", result.PeerID)
	}
	if !strings.Contains(result.SignedPeerRecord, "fixture-cap") {
		t.Fatalf("signed_peer_record = %q, want fixture-cap marker", result.SignedPeerRecord)
	}
	caps := append([]string(nil), result.TransportCapabilities...)
	slices.Sort(caps)
	if got, want := strings.Join(caps, ","), "direct,store_forward"; got != want {
		t.Fatalf("transport_capabilities = %q, want %q", got, want)
	}
	if got, want := strings.Join(result.DirectHints, ","), server.URL+"/direct?token=fixture-token"; got != want {
		t.Fatalf("direct_hints = %q, want %q", got, want)
	}
	if got, want := strings.Join(result.StoreForwardHints, ","), server.URL+"/relay"; got != want {
		t.Fatalf("store_forward_hints = %q, want %q", got, want)
	}
}

func TestServiceInspectExtractsNostrBindingsFromAgentCard(t *testing.T) {
	t.Parallel()

	server := newFixtureServer(t, "with-nostr")
	defer server.Close()

	service := NewService()
	service.Client = server.Client()
	service.Now = func() time.Time { return time.Date(2026, time.March, 13, 13, 0, 0, 0, time.UTC) }

	result, err := service.Inspect(context.Background(), server.URL+"/profile/")
	if err != nil {
		t.Fatalf("Inspect returned error: %v", err)
	}
	if result.PeerID != "lcpeer:fixture-nostr" {
		t.Fatalf("peer_id = %q, want lcpeer:fixture-nostr", result.PeerID)
	}
	if !strings.Contains(result.SignedPeerRecord, "fixture-nostr") {
		t.Fatalf("signed_peer_record = %q, want fixture-nostr marker", result.SignedPeerRecord)
	}

	caps := append([]string(nil), result.TransportCapabilities...)
	slices.Sort(caps)
	if got, want := strings.Join(caps, ","), "nostr"; got != want {
		t.Fatalf("transport_capabilities = %q, want %q", got, want)
	}
	if len(result.StoreForwardHints) != 0 {
		t.Fatalf("store_forward_hints = %v, want empty for nostr relay mappings", result.StoreForwardHints)
	}

	relayURLs := append([]string(nil), result.RelayURLs...)
	slices.Sort(relayURLs)
	if got, want := strings.Join(relayURLs, ","), "wss://relay.backup.example,wss://relay.nostr.example"; got != want {
		t.Fatalf("relay_urls = %q, want %q", got, want)
	}

	publicKeys := append([]string(nil), result.NostrPublicKeys...)
	slices.Sort(publicKeys)
	if got, want := strings.Join(publicKeys, ","), "npub_fixture_1,npub_fixture_2"; got != want {
		t.Fatalf("nostr_public_keys = %q, want %q", got, want)
	}
	if result.NostrPrimaryPublicKey != "npub_fixture_2" {
		t.Fatalf("nostr_primary_public_key = %q, want npub_fixture_2", result.NostrPrimaryPublicKey)
	}
}

func newFixtureServer(t *testing.T, fixture string) *httptest.Server {
	t.Helper()

	root := filepath.Join("testdata", fixture)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filePath := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if strings.HasSuffix(r.URL.Path, "/") {
			filePath = filepath.Join(filePath, "index.html")
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		replaced := strings.ReplaceAll(string(content), "{{ORIGIN}}", serverOrigin(r))
		replaced = strings.ReplaceAll(replaced, "{{RESOURCE}}", serverOrigin(r)+"/")

		switch filepath.Ext(filePath) {
		case ".html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case ".json":
			w.Header().Set("Content-Type", "application/json")
		default:
			w.Header().Set("Content-Type", "application/json")
		}
		_, _ = w.Write([]byte(replaced))
	}))
}

func serverOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
