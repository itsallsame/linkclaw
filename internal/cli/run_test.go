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
	if migrationCount != 2 {
		t.Fatalf("expected exactly two applied migrations, got %d", migrationCount)
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

func runForTest(t *testing.T, args []string, stdin string) (int, string, string) {
	t.Helper()

	in := strings.NewReader(stdin)
	var out strings.Builder
	var errOut strings.Builder
	code := Run(context.Background(), args, in, &out, &errOut)
	return code, out.String(), errOut.String()
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
