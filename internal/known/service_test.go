package known

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/importer"
	"github.com/xiewanpeng/claw-identity/internal/initflow"
	"github.com/xiewanpeng/claw-identity/internal/resolver"

	_ "modernc.org/sqlite"
)

func TestServiceRefreshMarksMismatchWithoutDuplicatingContact(t *testing.T) {
	t.Parallel()

	server, setRoot := newSwitchableFixtureServer(t, filepath.Join("..", "resolver", "testdata", "consistent"))
	t.Cleanup(server.Close)

	home := seedKnownHome(t)
	importerService := importer.NewService()
	importerService.Now = func() time.Time { return time.Date(2026, time.March, 13, 11, 0, 0, 0, time.UTC) }
	importerService.Resolver = resolver.NewService()
	importerService.Resolver.Client = server.Client()
	importerService.Resolver.Now = importerService.Now

	imported, err := importerService.Import(context.Background(), importer.Options{
		Home:  home,
		Input: server.URL + "/profile/",
	})
	if err != nil {
		t.Fatalf("initial import: %v", err)
	}

	db := openKnownDB(t, home)
	defer db.Close()

	if _, err := db.Exec(`UPDATE contacts SET profile_url = '' WHERE contact_id = ?`, imported.ContactID); err != nil {
		t.Fatalf("clear profile_url for refresh fallback: %v", err)
	}

	setRoot(filepath.Join("..", "resolver", "testdata", "mismatch-card"))

	service := &Service{
		Importer: importerService,
		Now:      func() time.Time { return time.Date(2026, time.March, 13, 11, 5, 0, 0, time.UTC) },
	}
	refreshed, err := service.Refresh(context.Background(), RefreshOptions{
		Home:       home,
		Identifier: imported.ContactID,
	})
	if err != nil {
		t.Fatalf("refresh known contact: %v", err)
	}

	if refreshed.Contact.ContactID != imported.ContactID {
		t.Fatalf("refreshed contact_id = %q, want %q", refreshed.Contact.ContactID, imported.ContactID)
	}
	if refreshed.Inspection.Status != resolver.StatusMismatch {
		t.Fatalf("inspection status = %q, want %q", refreshed.Inspection.Status, resolver.StatusMismatch)
	}
	if refreshed.Contact.Status != resolver.StatusMismatch {
		t.Fatalf("contact status = %q, want %q", refreshed.Contact.Status, resolver.StatusMismatch)
	}
	if len(refreshed.Inspection.Mismatches) == 0 {
		t.Fatalf("expected mismatch details after refresh drift")
	}

	var contactCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM contacts`).Scan(&contactCount); err != nil {
		t.Fatalf("count contacts: %v", err)
	}
	if contactCount != 1 {
		t.Fatalf("contact count = %d, want 1", contactCount)
	}

	var canonicalID string
	if err := db.QueryRow(`SELECT canonical_id FROM contacts WHERE contact_id = ?`, imported.ContactID).Scan(&canonicalID); err != nil {
		t.Fatalf("query canonical_id: %v", err)
	}
	if canonicalID != "did:web:fixture.example" {
		t.Fatalf("stored canonical_id = %q", canonicalID)
	}

	var verificationState string
	if err := db.QueryRow(`SELECT verification_state FROM trust_records WHERE contact_id = ?`, imported.ContactID).Scan(&verificationState); err != nil {
		t.Fatalf("query trust verification_state: %v", err)
	}
	if verificationState != resolver.StatusMismatch {
		t.Fatalf("verification_state = %q, want %q", verificationState, resolver.StatusMismatch)
	}

	var refreshEvents int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM interaction_events WHERE contact_id = ? AND event_type = 'refresh'`,
		imported.ContactID,
	).Scan(&refreshEvents); err != nil {
		t.Fatalf("count refresh events: %v", err)
	}
	if refreshEvents != 1 {
		t.Fatalf("refresh event count = %d, want 1", refreshEvents)
	}
}

func TestServiceTrustWritesTrustEventHistory(t *testing.T) {
	t.Parallel()

	server, _ := newSwitchableFixtureServer(t, filepath.Join("..", "resolver", "testdata", "consistent"))
	t.Cleanup(server.Close)

	home := seedKnownHome(t)
	importerService := importer.NewService()
	importerService.Now = func() time.Time { return time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC) }
	importerService.Resolver = resolver.NewService()
	importerService.Resolver.Client = server.Client()
	importerService.Resolver.Now = importerService.Now

	imported, err := importerService.Import(context.Background(), importer.Options{
		Home:  home,
		Input: server.URL + "/profile/",
	})
	if err != nil {
		t.Fatalf("initial import: %v", err)
	}

	service := &Service{
		Importer: importerService,
		Now:      func() time.Time { return time.Date(2026, time.March, 13, 12, 5, 0, 0, time.UTC) },
	}
	if _, err := service.Trust(context.Background(), TrustOptions{
		Home:       home,
		Identifier: imported.ContactID,
		Level:      "seen",
		Reason:     "first manual pass",
	}); err != nil {
		t.Fatalf("first trust update: %v", err)
	}
	if _, err := service.Trust(context.Background(), TrustOptions{
		Home:         home,
		Identifier:   imported.ContactID,
		Level:        "trusted",
		RiskFlags:    []string{"manual-review"},
		HasRiskFlags: true,
		Reason:       "manual review approved",
	}); err != nil {
		t.Fatalf("second trust update: %v", err)
	}

	db := openKnownDB(t, home)
	defer db.Close()

	var trustLevel string
	var riskFlags string
	var decisionReason string
	if err := db.QueryRow(
		`SELECT trust_level, risk_flags, decision_reason FROM trust_records WHERE contact_id = ?`,
		imported.ContactID,
	).Scan(&trustLevel, &riskFlags, &decisionReason); err != nil {
		t.Fatalf("query trust record: %v", err)
	}
	if trustLevel != "trusted" {
		t.Fatalf("stored trust level = %q, want trusted", trustLevel)
	}
	if riskFlags != `["manual-review"]` {
		t.Fatalf("stored risk flags = %q, want [\"manual-review\"]", riskFlags)
	}
	if decisionReason != "manual review approved" {
		t.Fatalf("stored reason = %q, want manual review approved", decisionReason)
	}

	var trustEventCount int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM trust_events WHERE contact_id = ?`,
		imported.ContactID,
	).Scan(&trustEventCount); err != nil {
		t.Fatalf("count trust events: %v", err)
	}
	if trustEventCount != 2 {
		t.Fatalf("trust event count = %d, want 2", trustEventCount)
	}

	rows, err := db.Query(
		`SELECT decision_reason, source FROM trust_events WHERE contact_id = ?`,
		imported.ContactID,
	)
	if err != nil {
		t.Fatalf("query trust event history: %v", err)
	}
	defer rows.Close()

	reasons := map[string]bool{}
	for rows.Next() {
		var reason string
		var source string
		if err := rows.Scan(&reason, &source); err != nil {
			t.Fatalf("scan trust event: %v", err)
		}
		reasons[reason] = true
		if source != "known-trust" {
			t.Fatalf("trust event source = %q, want known-trust", source)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate trust events: %v", err)
	}
	if !reasons["first manual pass"] || !reasons["manual review approved"] {
		t.Fatalf("unexpected trust event reasons: %+v", reasons)
	}
}

func seedKnownHome(t *testing.T) string {
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

func openKnownDB(t *testing.T, home string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	return db
}

func newSwitchableFixtureServer(t *testing.T, initialRoot string) (*httptest.Server, func(string)) {
	t.Helper()

	var mu sync.RWMutex
	currentRoot := initialRoot
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		root := currentRoot
		mu.RUnlock()

		filePath := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if strings.HasSuffix(r.URL.Path, "/") {
			filePath = filepath.Join(filePath, "index.html")
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		replaced := strings.ReplaceAll(string(content), "{{ORIGIN}}", knownServerOrigin(r))
		replaced = strings.ReplaceAll(replaced, "{{RESOURCE}}", knownServerOrigin(r)+"/")
		switch filepath.Ext(filePath) {
		case ".html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		default:
			w.Header().Set("Content-Type", "application/json")
		}
		_, _ = w.Write([]byte(replaced))
	}))

	return server, func(nextRoot string) {
		mu.Lock()
		currentRoot = nextRoot
		mu.Unlock()
	}
}

func knownServerOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
