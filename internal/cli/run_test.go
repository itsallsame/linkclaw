package cli

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xiewanpeng/claw-identity/internal/indexer"
	"github.com/xiewanpeng/claw-identity/internal/known"

	_ "modernc.org/sqlite"
)

func TestRunInitJSONIdempotent(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "linkclaw-home")
	args := []string{
		"init",
		"--home", home,
		"--canonical-id", "did:web:example.com",
		"--display-name", "Example",
		"--non-interactive",
		"--json",
	}

	firstCode, firstOut, firstErr := runForTest(t, args, "")
	if firstCode != 0 {
		t.Fatalf("first init exit code = %d, stderr = %s", firstCode, firstErr)
	}

	var first initOutput
	if err := json.Unmarshal([]byte(firstOut), &first); err != nil {
		t.Fatalf("unmarshal first output: %v, output=%s", err, firstOut)
	}
	if !first.OK {
		t.Fatalf("first init returned not ok: %+v", first)
	}
	if !first.Result.Identity.Created {
		t.Fatalf("expected first run to create identity")
	}
	if !first.Result.Key.Created {
		t.Fatalf("expected first run to create key")
	}
	if len(first.Result.Migrations) < 2 {
		t.Fatalf("expected at least two migrations, got %+v", first.Result.Migrations)
	}
	for _, step := range first.Result.Migrations {
		if !step.Applied {
			t.Fatalf("expected first migration run to apply all migrations, got %+v", first.Result.Migrations)
		}
	}

	secondCode, secondOut, secondErr := runForTest(t, args, "")
	if secondCode != 0 {
		t.Fatalf("second init exit code = %d, stderr = %s", secondCode, secondErr)
	}

	var second initOutput
	if err := json.Unmarshal([]byte(secondOut), &second); err != nil {
		t.Fatalf("unmarshal second output: %v, output=%s", err, secondOut)
	}
	if second.Result.Identity.Created {
		t.Fatalf("expected second run not to create identity")
	}
	if second.Result.Key.Created {
		t.Fatalf("expected second run not to create key")
	}
	if len(second.Result.Migrations) < 2 {
		t.Fatalf("expected at least two migrations on second run, got %+v", second.Result.Migrations)
	}
	for _, step := range second.Result.Migrations {
		if step.Applied {
			t.Fatalf("expected second migration run to be idempotent, got %+v", second.Result.Migrations)
		}
	}

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	var migrationCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM schema_migrations").Scan(&migrationCount); err != nil {
		t.Fatalf("count schema migrations: %v", err)
	}
	if migrationCount != len(first.Result.Migrations) {
		t.Fatalf("expected %d applied migrations, got %d", len(first.Result.Migrations), migrationCount)
	}

	var selfCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM self_identities").Scan(&selfCount); err != nil {
		t.Fatalf("count self identities: %v", err)
	}
	if selfCount != 1 {
		t.Fatalf("expected exactly one self identity, got %d", selfCount)
	}
}

func TestRunInitInteractiveJSON(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "linkclaw-home")
	args := []string{"init", "--home", home, "--json"}
	code, stdout, stderr := runForTest(t, args, "did:web:interactive.example\nInteractive Example\n")
	if code != 0 {
		t.Fatalf("interactive init exit code = %d, stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "Canonical ID:") {
		t.Fatalf("expected prompt in stderr, got %q", stderr)
	}

	var out initOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("unmarshal stdout: %v, stdout=%s", err, stdout)
	}
	if out.Result.Identity.CanonicalID != "did:web:interactive.example" {
		t.Fatalf("unexpected canonical id: %s", out.Result.Identity.CanonicalID)
	}
	if out.Result.Identity.DisplayName != "Interactive Example" {
		t.Fatalf("unexpected display name: %s", out.Result.Identity.DisplayName)
	}
}

func TestRunInitNonInteractiveRequiresCanonicalID(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "linkclaw-home")
	code, stdout, _ := runForTest(t, []string{"init", "--home", home, "--non-interactive", "--json"}, "")
	if code == 0 {
		t.Fatalf("expected failure when canonical id is missing")
	}

	var out initOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if out.OK {
		t.Fatalf("expected ok=false output")
	}
	if !strings.Contains(out.Error, "canonical id") {
		t.Fatalf("expected canonical id error, got %q", out.Error)
	}
}

func TestRunPublishJSON(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "linkclaw-home")
	initArgs := []string{
		"init",
		"--home", home,
		"--canonical-id", "did:web:agent.example",
		"--display-name", "Agent Example",
		"--non-interactive",
		"--json",
	}
	initCode, _, initErr := runForTest(t, initArgs, "")
	if initCode != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", initCode, initErr)
	}

	outputDir := filepath.Join(t.TempDir(), "bundle")
	args := []string{
		"publish",
		"--home", home,
		"--origin", "https://agent.example",
		"--output", outputDir,
		"--tier", "full",
		"--json",
	}
	code, stdout, stderr := runForTest(t, args, "")
	if code != 0 {
		t.Fatalf("publish exit code = %d, stderr = %s", code, stderr)
	}

	var out publishOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("unmarshal stdout: %v, stdout=%s", err, stdout)
	}
	if !out.OK {
		t.Fatalf("expected ok=true output: %+v", out)
	}
	if out.Result.Tier != "full" {
		t.Fatalf("tier = %q", out.Result.Tier)
	}
	if out.Result.HomeOrigin != "https://agent.example" {
		t.Fatalf("home origin = %q", out.Result.HomeOrigin)
	}
	if len(out.Result.Artifacts) != 4 {
		t.Fatalf("artifact count = %d, want 4", len(out.Result.Artifacts))
	}
	if _, err := os.Stat(filepath.Join(outputDir, "manifest.json")); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
}

func TestRunInspectJSON(t *testing.T) {
	t.Parallel()

	server := newFixtureServer(t, filepath.Join("..", "resolver", "testdata", "consistent"))
	defer server.Close()

	code, stdout, stderr := runForTest(t, []string{"inspect", "--json", server.URL + "/profile/"}, "")
	if code != 0 {
		t.Fatalf("inspect exit code = %d, stderr = %s", code, stderr)
	}

	var out inspectOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("unmarshal stdout: %v, stdout=%s", err, stdout)
	}
	if !out.OK {
		t.Fatalf("expected ok=true output: %+v", out)
	}
	if out.Result.Status != "consistent" {
		t.Fatalf("status = %q", out.Result.Status)
	}
	if out.Result.CanonicalID != "did:web:fixture.example" {
		t.Fatalf("canonical id = %q", out.Result.CanonicalID)
	}
}

func TestRunImportJSON(t *testing.T) {
	t.Parallel()

	server := newFixtureServer(t, filepath.Join("..", "resolver", "testdata", "consistent"))
	defer server.Close()

	home := filepath.Join(t.TempDir(), "linkclaw-home")
	initArgs := []string{
		"init",
		"--home", home,
		"--canonical-id", "did:web:self.example",
		"--display-name", "Self Example",
		"--non-interactive",
		"--json",
	}
	initCode, _, initErr := runForTest(t, initArgs, "")
	if initCode != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", initCode, initErr)
	}

	code, stdout, stderr := runForTest(t, []string{"import", "--home", home, "--json", server.URL + "/profile/"}, "")
	if code != 0 {
		t.Fatalf("import exit code = %d, stderr = %s", code, stderr)
	}

	var out importOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("unmarshal stdout: %v, stdout=%s", err, stdout)
	}
	if !out.OK {
		t.Fatalf("expected ok=true output: %+v", out)
	}
	if out.Result.Inspection.Status != "consistent" {
		t.Fatalf("status = %q", out.Result.Inspection.Status)
	}
	if out.Result.SnapshotCount != 4 {
		t.Fatalf("snapshot count = %d", out.Result.SnapshotCount)
	}
}

func TestRunKnownTrustJSON(t *testing.T) {
	t.Parallel()

	home, imported := setupImportedContact(t)
	code, stdout, stderr := runForTest(
		t,
		[]string{
			"known",
			"trust",
			"--home", home,
			"--level", "trusted",
			"--risk", "manual-review,fixture",
			"--reason", "reviewed in CLI test",
			"--json",
			imported.Result.ContactID,
		},
		"",
	)
	if code != 0 {
		t.Fatalf("known trust exit code = %d, stderr = %s", code, stderr)
	}

	var out knownOutput[known.TrustResult]
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("unmarshal stdout: %v, stdout=%s", err, stdout)
	}
	if !out.OK {
		t.Fatalf("expected ok=true output: %+v", out)
	}
	if out.Result.Contact.Trust.TrustLevel != "trusted" {
		t.Fatalf("trust level = %q", out.Result.Contact.Trust.TrustLevel)
	}
	if got := strings.Join(out.Result.Contact.Trust.RiskFlags, ","); got != "fixture,manual-review" {
		t.Fatalf("risk flags = %q", got)
	}

	db := openDBForTest(t, home)
	defer db.Close()

	var level string
	var riskFlags string
	var reason string
	if err := db.QueryRow(
		`SELECT trust_level, risk_flags, decision_reason FROM trust_records WHERE contact_id = ?`,
		imported.Result.ContactID,
	).Scan(&level, &riskFlags, &reason); err != nil {
		t.Fatalf("query trust record: %v", err)
	}
	if level != "trusted" {
		t.Fatalf("stored trust level = %q", level)
	}
	if riskFlags != `["fixture","manual-review"]` {
		t.Fatalf("stored risk flags = %q", riskFlags)
	}
	if reason != "reviewed in CLI test" {
		t.Fatalf("stored reason = %q", reason)
	}
	assertCount(t, db, `SELECT COUNT(*) FROM interaction_events WHERE contact_id = ? AND event_type = 'trust'`, imported.Result.ContactID, 1)
}

func TestRunKnownNoteJSON(t *testing.T) {
	t.Parallel()

	home, imported := setupImportedContact(t)
	code, stdout, stderr := runForTest(
		t,
		[]string{
			"known",
			"note",
			"--home", home,
			"--body", "met via fixture test",
			"--json",
			imported.Result.ContactID,
		},
		"",
	)
	if code != 0 {
		t.Fatalf("known note exit code = %d, stderr = %s", code, stderr)
	}

	var out knownOutput[known.NoteResult]
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("unmarshal stdout: %v, stdout=%s", err, stdout)
	}
	if !out.OK {
		t.Fatalf("expected ok=true output: %+v", out)
	}
	if out.Result.Note.Body != "met via fixture test" {
		t.Fatalf("note body = %q", out.Result.Note.Body)
	}

	db := openDBForTest(t, home)
	defer db.Close()

	var noteBody string
	if err := db.QueryRow(`SELECT body FROM notes WHERE contact_id = ?`, imported.Result.ContactID).Scan(&noteBody); err != nil {
		t.Fatalf("query note: %v", err)
	}
	if noteBody != "met via fixture test" {
		t.Fatalf("stored note body = %q", noteBody)
	}
	assertCount(t, db, `SELECT COUNT(*) FROM interaction_events WHERE contact_id = ? AND event_type = 'note'`, imported.Result.ContactID, 1)
}

func TestRunKnownRefreshJSON(t *testing.T) {
	t.Parallel()

	home, imported := setupImportedContact(t)
	code, stdout, stderr := runForTest(
		t,
		[]string{"known", "refresh", "--home", home, "--json", imported.Result.ContactID},
		"",
	)
	if code != 0 {
		t.Fatalf("known refresh exit code = %d, stderr = %s", code, stderr)
	}

	var out knownOutput[known.RefreshResult]
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("unmarshal stdout: %v, stdout=%s", err, stdout)
	}
	if !out.OK {
		t.Fatalf("expected ok=true output: %+v", out)
	}
	if out.Result.Inspection.Status != "consistent" {
		t.Fatalf("refresh status = %q", out.Result.Inspection.Status)
	}

	db := openDBForTest(t, home)
	defer db.Close()

	assertCount(t, db, `SELECT COUNT(*) FROM interaction_events WHERE contact_id = ? AND event_type = 'refresh'`, imported.Result.ContactID, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM artifact_snapshots WHERE contact_id = ?`, imported.Result.ContactID, 8)
	var handleCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM handles WHERE owner_type = 'contact' AND owner_id = ?`, imported.Result.ContactID).Scan(&handleCount); err != nil {
		t.Fatalf("count handles: %v", err)
	}
	if handleCount == 0 {
		t.Fatalf("expected refresh path to leave contact handles")
	}
}

func TestRunKnownListShowAndRemoveJSON(t *testing.T) {
	t.Parallel()

	home, imported := setupImportedContact(t)

	lsCode, lsStdout, lsStderr := runForTest(t, []string{"known", "ls", "--home", home, "--json"}, "")
	if lsCode != 0 {
		t.Fatalf("known ls exit code = %d, stderr = %s", lsCode, lsStderr)
	}
	var lsOut knownOutput[known.ListResult]
	if err := json.Unmarshal([]byte(lsStdout), &lsOut); err != nil {
		t.Fatalf("unmarshal ls output: %v, stdout=%s", err, lsStdout)
	}
	if len(lsOut.Result.Contacts) != 1 {
		t.Fatalf("known ls contacts = %d", len(lsOut.Result.Contacts))
	}

	showCode, showStdout, showStderr := runForTest(t, []string{"known", "show", "--home", home, "--json", imported.Result.ContactID}, "")
	if showCode != 0 {
		t.Fatalf("known show exit code = %d, stderr = %s", showCode, showStderr)
	}
	var showOut knownOutput[known.ShowResult]
	if err := json.Unmarshal([]byte(showStdout), &showOut); err != nil {
		t.Fatalf("unmarshal show output: %v, stdout=%s", err, showStdout)
	}
	if showOut.Result.Contact.ContactID != imported.Result.ContactID {
		t.Fatalf("shown contact = %q", showOut.Result.Contact.ContactID)
	}
	if len(showOut.Result.Contact.Handles) == 0 {
		t.Fatalf("expected known show to include handles")
	}

	rmCode, rmStdout, rmStderr := runForTest(t, []string{"known", "rm", "--home", home, "--json", imported.Result.ContactID}, "")
	if rmCode != 0 {
		t.Fatalf("known rm exit code = %d, stderr = %s", rmCode, rmStderr)
	}
	var rmOut knownOutput[known.RemoveResult]
	if err := json.Unmarshal([]byte(rmStdout), &rmOut); err != nil {
		t.Fatalf("unmarshal rm output: %v, stdout=%s", err, rmStdout)
	}
	if rmOut.Result.Removed.Contacts != 1 {
		t.Fatalf("removed contacts = %d", rmOut.Result.Removed.Contacts)
	}

	db := openDBForTest(t, home)
	defer db.Close()
	assertCount(t, db, `SELECT COUNT(*) FROM contacts`, nil, 0)
}

func TestRunIndexCrawlAndSearchJSON(t *testing.T) {
	t.Parallel()

	server := newFixtureServer(t, filepath.Join("..", "resolver", "testdata", "consistent"))
	defer server.Close()

	home := filepath.Join(t.TempDir(), "linkclaw-home")
	crawlCode, crawlStdout, crawlStderr := runForTest(t, []string{"index", "crawl", "--home", home, "--json", server.URL + "/profile/"}, "")
	if crawlCode != 0 {
		t.Fatalf("index crawl exit code = %d, stderr = %s", crawlCode, crawlStderr)
	}
	var crawlOut indexOutput[indexer.CrawlResult]
	if err := json.Unmarshal([]byte(crawlStdout), &crawlOut); err != nil {
		t.Fatalf("unmarshal crawl output: %v, stdout=%s", err, crawlStdout)
	}
	if !crawlOut.OK {
		t.Fatalf("expected ok=true crawl output: %+v", crawlOut)
	}
	if crawlOut.Result.Record.ConflictState != indexer.ConflictClear {
		t.Fatalf("conflict state = %q", crawlOut.Result.Record.ConflictState)
	}
	if crawlOut.Result.Record.SourceCount != 4 {
		t.Fatalf("source count = %d", crawlOut.Result.Record.SourceCount)
	}

	searchCode, searchStdout, searchStderr := runForTest(t, []string{"index", "search", "--home", home, "--json", "fixture"}, "")
	if searchCode != 0 {
		t.Fatalf("index search exit code = %d, stderr = %s", searchCode, searchStderr)
	}
	var searchOut indexOutput[indexer.SearchResult]
	if err := json.Unmarshal([]byte(searchStdout), &searchOut); err != nil {
		t.Fatalf("unmarshal search output: %v, stdout=%s", err, searchStdout)
	}
	if !searchOut.OK {
		t.Fatalf("expected ok=true search output: %+v", searchOut)
	}
	if len(searchOut.Result.Records) != 1 {
		t.Fatalf("search record count = %d", len(searchOut.Result.Records))
	}
	if got := searchOut.Result.Records[0].CanonicalID; got != "did:web:fixture.example" {
		t.Fatalf("canonical id = %q", got)
	}
	if len(searchOut.Result.Records[0].SourceURLs) != 4 {
		t.Fatalf("source urls = %d", len(searchOut.Result.Records[0].SourceURLs))
	}
}

func runForTest(t *testing.T, args []string, stdin string) (int, string, string) {
	t.Helper()

	in := strings.NewReader(stdin)
	var out strings.Builder
	var errOut strings.Builder
	code := Run(context.Background(), args, in, &out, &errOut)
	return code, out.String(), errOut.String()
}

func setupImportedContact(t *testing.T) (string, importOutput) {
	t.Helper()

	server := newFixtureServer(t, filepath.Join("..", "resolver", "testdata", "consistent"))
	t.Cleanup(server.Close)

	home := filepath.Join(t.TempDir(), "linkclaw-home")
	initArgs := []string{
		"init",
		"--home", home,
		"--canonical-id", "did:web:self.example",
		"--display-name", "Self Example",
		"--non-interactive",
		"--json",
	}
	initCode, _, initErr := runForTest(t, initArgs, "")
	if initCode != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", initCode, initErr)
	}

	importCode, importStdout, importErr := runForTest(t, []string{"import", "--home", home, "--json", server.URL + "/profile/"}, "")
	if importCode != 0 {
		t.Fatalf("import exit code = %d, stderr = %s", importCode, importErr)
	}
	var out importOutput
	if err := json.Unmarshal([]byte(importStdout), &out); err != nil {
		t.Fatalf("unmarshal import output: %v, stdout=%s", err, importStdout)
	}
	return home, out
}

func openDBForTest(t *testing.T, home string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	return db
}

func assertCount(t *testing.T, db *sql.DB, query string, arg any, want int) {
	t.Helper()

	var count int
	var err error
	if arg == nil {
		err = db.QueryRow(query).Scan(&count)
	} else {
		err = db.QueryRow(query, arg).Scan(&count)
	}
	if err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != want {
		t.Fatalf("count = %d, want %d for query %q", count, want, query)
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
