package importer

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/initflow"
	"github.com/xiewanpeng/claw-identity/internal/resolver"
	"github.com/xiewanpeng/claw-identity/internal/transport"

	_ "modernc.org/sqlite"
)

func TestServiceImportPersistsTrustBookRecords(t *testing.T) {
	t.Parallel()

	server := newFixtureServer(t, filepath.Join("..", "resolver", "testdata", "consistent"))
	defer server.Close()

	home := seedHome(t)
	service := NewService()
	service.Now = func() time.Time { return time.Date(2026, time.March, 13, 9, 30, 0, 0, time.UTC) }
	service.Resolver = resolver.NewService()
	service.Resolver.Client = server.Client()
	service.Resolver.Now = service.Now

	result, err := service.Import(context.Background(), Options{
		Home:  home,
		Input: server.URL + "/profile/",
	})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}

	if !result.Created {
		t.Fatalf("expected first import to create a contact")
	}
	if result.Inspection.Status != resolver.StatusConsistent {
		t.Fatalf("status = %q, want %q", result.Inspection.Status, resolver.StatusConsistent)
	}
	if result.SnapshotCount != 4 {
		t.Fatalf("snapshot count = %d, want 4", result.SnapshotCount)
	}
	if result.ProofCount != 5 {
		t.Fatalf("proof count = %d, want 5", result.ProofCount)
	}

	db := openDB(t, home)
	defer db.Close()

	var canonicalID, status string
	if err := db.QueryRow(`SELECT canonical_id, status FROM contacts WHERE contact_id = ?`, result.ContactID).Scan(&canonicalID, &status); err != nil {
		t.Fatalf("query contact: %v", err)
	}
	if canonicalID != "did:web:fixture.example" {
		t.Fatalf("canonical id = %q", canonicalID)
	}
	if status != resolver.StatusConsistent {
		t.Fatalf("contact status = %q", status)
	}

	var verificationState string
	if err := db.QueryRow(`SELECT verification_state FROM trust_records WHERE contact_id = ?`, result.ContactID).Scan(&verificationState); err != nil {
		t.Fatalf("query trust record: %v", err)
	}
	if verificationState != resolver.StatusConsistent {
		t.Fatalf("verification state = %q", verificationState)
	}
	var runtimeTrustVerification string
	if err := db.QueryRow(
		`SELECT verification_state FROM runtime_trust_records WHERE canonical_id = ?`,
		result.Inspection.CanonicalID,
	).Scan(&runtimeTrustVerification); err != nil {
		t.Fatalf("query runtime trust record: %v", err)
	}
	if runtimeTrustVerification != resolver.StatusConsistent {
		t.Fatalf("runtime trust verification_state = %q", runtimeTrustVerification)
	}
	assertCount(t, db, `SELECT COUNT(*) FROM runtime_discovery_records WHERE canonical_id = ?`, result.Inspection.CanonicalID, 1)

	assertCount(t, db, `SELECT COUNT(*) FROM artifact_snapshots WHERE contact_id = ?`, result.ContactID, 4)
	assertCount(t, db, `SELECT COUNT(*) FROM proofs WHERE contact_id = ?`, result.ContactID, 5)
	assertCount(t, db, `SELECT COUNT(*) FROM interaction_events WHERE contact_id = ? AND event_type = 'import'`, result.ContactID, 1)
}

func TestServiceImportAllowsResolvedDidOnly(t *testing.T) {
	t.Parallel()

	server := newFixtureServer(t, filepath.Join("..", "resolver", "testdata", "did-only"))
	defer server.Close()

	home := seedHome(t)
	service := NewService()
	service.Now = func() time.Time { return time.Date(2026, time.March, 13, 10, 0, 0, 0, time.UTC) }
	service.Resolver = resolver.NewService()
	service.Resolver.Client = server.Client()
	service.Resolver.Now = service.Now

	result, err := service.Import(context.Background(), Options{
		Home:  home,
		Input: server.URL,
	})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if result.Inspection.Status != resolver.StatusResolved {
		t.Fatalf("status = %q, want %q", result.Inspection.Status, resolver.StatusResolved)
	}
	if result.SnapshotCount != 1 {
		t.Fatalf("snapshot count = %d, want 1", result.SnapshotCount)
	}
}

func TestServiceImportRejectsDiscoveredByDefault(t *testing.T) {
	t.Parallel()

	server := newFixtureServer(t, filepath.Join("..", "resolver", "testdata", "card-only"))
	defer server.Close()

	home := seedHome(t)
	service := NewService()
	service.Now = func() time.Time { return time.Date(2026, time.March, 13, 10, 30, 0, 0, time.UTC) }
	service.Resolver = resolver.NewService()
	service.Resolver.Client = server.Client()
	service.Resolver.Now = service.Now

	_, err := service.Import(context.Background(), Options{
		Home:  home,
		Input: server.URL,
	})
	if err == nil {
		t.Fatalf("expected discovered import to fail by default")
	}
	if !strings.Contains(err.Error(), "resolved or consistent") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServiceImportPersistsCapabilityDiscoveryFacts(t *testing.T) {
	t.Parallel()

	server := newFixtureServer(t, filepath.Join("..", "resolver", "testdata", "with-capabilities"))
	defer server.Close()

	home := seedHome(t)
	service := NewService()
	service.Now = func() time.Time { return time.Date(2026, time.March, 13, 11, 0, 0, 0, time.UTC) }
	service.Resolver = resolver.NewService()
	service.Resolver.Client = server.Client()
	service.Resolver.Now = service.Now

	result, err := service.Import(context.Background(), Options{
		Home:  home,
		Input: server.URL + "/profile/",
	})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}

	db := openDB(t, home)
	defer db.Close()

	var (
		peerID            string
		routeCandidates   string
		transportCapsJSON string
		directHintsJSON   string
		storeHintsJSON    string
		signedPeerRecord  string
		reachable         int
	)
	if err := db.QueryRow(
		`SELECT peer_id, route_candidates_json, transport_capabilities_json, direct_hints_json,
		        store_forward_hints_json, signed_peer_record, reachable
		   FROM runtime_discovery_records
		  WHERE canonical_id = ?`,
		result.Inspection.CanonicalID,
	).Scan(
		&peerID,
		&routeCandidates,
		&transportCapsJSON,
		&directHintsJSON,
		&storeHintsJSON,
		&signedPeerRecord,
		&reachable,
	); err != nil {
		t.Fatalf("query runtime discovery record: %v", err)
	}
	if peerID != "lcpeer:fixture-cap" {
		t.Fatalf("peer_id = %q, want lcpeer:fixture-cap", peerID)
	}
	if !strings.Contains(signedPeerRecord, "fixture-cap") {
		t.Fatalf("signed_peer_record = %q, want fixture-cap marker", signedPeerRecord)
	}
	if reachable != 1 {
		t.Fatalf("reachable = %d, want 1", reachable)
	}

	var transportCaps []string
	if err := json.Unmarshal([]byte(transportCapsJSON), &transportCaps); err != nil {
		t.Fatalf("decode transport_capabilities_json: %v", err)
	}
	slices.Sort(transportCaps)
	if got, want := strings.Join(transportCaps, ","), "direct,store_forward"; got != want {
		t.Fatalf("transport capabilities = %q, want %q", got, want)
	}

	var directHints []string
	if err := json.Unmarshal([]byte(directHintsJSON), &directHints); err != nil {
		t.Fatalf("decode direct_hints_json: %v", err)
	}
	if got, want := strings.Join(directHints, ","), server.URL+"/direct?token=fixture-token"; got != want {
		t.Fatalf("direct hints = %q, want %q", got, want)
	}

	var storeHints []string
	if err := json.Unmarshal([]byte(storeHintsJSON), &storeHints); err != nil {
		t.Fatalf("decode store_forward_hints_json: %v", err)
	}
	if got, want := strings.Join(storeHints, ","), server.URL+"/relay"; got != want {
		t.Fatalf("store-forward hints = %q, want %q", got, want)
	}

	var candidates []transport.RouteCandidate
	if err := json.Unmarshal([]byte(routeCandidates), &candidates); err != nil {
		t.Fatalf("decode route_candidates_json: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("route candidates len = %d, want 2", len(candidates))
	}
}

func seedHome(t *testing.T) string {
	t.Helper()

	home := filepath.Join(t.TempDir(), "linkclaw-home")
	service := initflow.NewService()
	if _, err := service.Init(context.Background(), initflow.Options{
		Home:        home,
		CanonicalID: "did:web:self.example",
		DisplayName: "Self Example",
	}); err != nil {
		t.Fatalf("init home: %v", err)
	}
	return home
}

func openDB(t *testing.T, home string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	return db
}

func assertCount(t *testing.T, db *sql.DB, query string, arg any, want int) {
	t.Helper()

	var got int
	if err := db.QueryRow(query, arg).Scan(&got); err != nil {
		t.Fatalf("query count: %v", err)
	}
	if got != want {
		t.Fatalf("count = %d, want %d", got, want)
	}
}

func newFixtureServer(t *testing.T, root string) *httptest.Server {
	t.Helper()

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
