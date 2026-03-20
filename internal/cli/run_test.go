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
	"time"

	"github.com/xiewanpeng/claw-identity/internal/buildinfo"
	"github.com/xiewanpeng/claw-identity/internal/card"
	"github.com/xiewanpeng/claw-identity/internal/indexer"
	"github.com/xiewanpeng/claw-identity/internal/known"
	"github.com/xiewanpeng/claw-identity/internal/relayserver"

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
	assertEnvelopeMetadata(t, first.SchemaVersion, first.Command, first.Subcommand, first.Timestamp, first.Warnings, "init", nil)
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
	assertEnvelopeMetadata(t, second.SchemaVersion, second.Command, second.Subcommand, second.Timestamp, second.Warnings, "init", nil)
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
	code, stdout, stderr := runForTest(t, args, "Interactive Example\n")
	if code != 0 {
		t.Fatalf("interactive init exit code = %d, stderr=%s", code, stderr)
	}
	if strings.Contains(stderr, "Canonical ID:") {
		t.Fatalf("did not expect canonical id prompt, got %q", stderr)
	}
	if !strings.Contains(stderr, "Display Name (optional):") {
		t.Fatalf("expected display name prompt in stderr, got %q", stderr)
	}

	var out initOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("unmarshal stdout: %v, stdout=%s", err, stdout)
	}
	assertEnvelopeMetadata(t, out.SchemaVersion, out.Command, out.Subcommand, out.Timestamp, out.Warnings, "init", nil)
	if !strings.HasPrefix(out.Result.Identity.CanonicalID, "did:key:z") {
		t.Fatalf("unexpected canonical id: %s", out.Result.Identity.CanonicalID)
	}
	if out.Result.Identity.DisplayName != "Interactive Example" {
		t.Fatalf("unexpected display name: %s", out.Result.Identity.DisplayName)
	}
}

func TestRunInitNonInteractiveAutoGeneratesCanonicalID(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "linkclaw-home")
	code, stdout, stderr := runForTest(t, []string{"init", "--home", home, "--display-name", "Auto", "--non-interactive", "--json"}, "")
	if code != 0 {
		t.Fatalf("expected init success, exit code = %d, stderr=%s", code, stderr)
	}

	var out initOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	assertEnvelopeMetadata(t, out.SchemaVersion, out.Command, out.Subcommand, out.Timestamp, out.Warnings, "init", nil)
	if !out.OK {
		t.Fatalf("expected ok=true output: %+v", out)
	}
	if !strings.HasPrefix(out.Result.Identity.CanonicalID, "did:key:z") {
		t.Fatalf("unexpected canonical id: %q", out.Result.Identity.CanonicalID)
	}
	if out.Result.Identity.DisplayName != "Auto" {
		t.Fatalf("display name = %q", out.Result.Identity.DisplayName)
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
	assertEnvelopeMetadata(t, out.SchemaVersion, out.Command, out.Subcommand, out.Timestamp, out.Warnings, "publish", nil)
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
	if out.Result.HeadersPath != filepath.Join(outputDir, "_headers") {
		t.Fatalf("headers path = %q", out.Result.HeadersPath)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "manifest.json")); err != nil {
		t.Fatalf("manifest missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outputDir, "_headers")); err != nil {
		t.Fatalf("_headers missing: %v", err)
	}
}

func TestRunPublishDeployCloudflareJSON(t *testing.T) {
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

	scriptDir := t.TempDir()
	captureFile := filepath.Join(scriptDir, "wrangler-args.txt")
	t.Setenv("CAPTURE_FILE", captureFile)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	writeExecutableForTest(t, filepath.Join(scriptDir, "wrangler"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CAPTURE_FILE\"\necho deployed\n")

	outputDir := filepath.Join(t.TempDir(), "bundle")
	args := []string{
		"publish",
		"--home", home,
		"--origin", "https://agent.example",
		"--output", outputDir,
		"--deploy", "cloudflare",
		"--project", "agent-pages",
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
	if out.Result.Deployment == nil {
		t.Fatalf("expected deployment result")
	}
	if out.Result.Deployment.Provider != "cloudflare" {
		t.Fatalf("provider = %q", out.Result.Deployment.Provider)
	}
	if out.Result.Deployment.Project != "agent-pages" {
		t.Fatalf("project = %q", out.Result.Deployment.Project)
	}

	captured, err := os.ReadFile(captureFile)
	if err != nil {
		t.Fatalf("read captured args: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(captured)), "\n")
	if len(lines) != 5 {
		t.Fatalf("captured args = %v", lines)
	}
	if lines[0] != "pages" || lines[1] != "deploy" || lines[3] != "--project-name" || lines[4] != "agent-pages" {
		t.Fatalf("unexpected deploy args: %v", lines)
	}
}

func TestRunCardExportAndVerifyJSON(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "linkclaw-home")
	initArgs := []string{
		"init",
		"--home", home,
		"--canonical-id", "did:key:z6MkAlice",
		"--display-name", "Alice",
		"--non-interactive",
		"--json",
	}
	initCode, _, initErr := runForTest(t, initArgs, "")
	if initCode != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", initCode, initErr)
	}

	exportCode, exportOut, exportErr := runForTest(t, []string{"card", "export", "--home", home, "--json"}, "")
	if exportCode != 0 {
		t.Fatalf("card export exit code = %d, stderr = %s", exportCode, exportErr)
	}

	var exported struct {
		SchemaVersion string   `json:"schema_version"`
		Command       string   `json:"command"`
		Subcommand    *string  `json:"subcommand"`
		Timestamp     string   `json:"timestamp"`
		Warnings      []string `json:"warnings"`
		OK            bool     `json:"ok"`
		Result        struct {
			Card card.Card `json:"card"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(exportOut), &exported); err != nil {
		t.Fatalf("unmarshal card export output: %v, output=%s", err, exportOut)
	}
	assertEnvelopeMetadata(t, exported.SchemaVersion, exported.Command, exported.Subcommand, exported.Timestamp, exported.Warnings, "card", stringPtr("export"))
	if !exported.OK {
		t.Fatalf("expected ok=true output")
	}
	if exported.Result.Card.ID != "did:key:z6MkAlice" {
		t.Fatalf("unexpected exported card id: %s", exported.Result.Card.ID)
	}
	if exported.Result.Card.Signature == "" {
		t.Fatalf("expected exported card signature")
	}

	cardJSON, err := json.Marshal(exported.Result.Card)
	if err != nil {
		t.Fatalf("marshal exported card for verify: %v", err)
	}

	verifyCode, verifyOut, verifyErr := runForTest(t, []string{"card", "verify", "--json", string(cardJSON)}, "")
	if verifyCode != 0 {
		t.Fatalf("card verify exit code = %d, stderr = %s", verifyCode, verifyErr)
	}

	var verified struct {
		SchemaVersion string   `json:"schema_version"`
		Command       string   `json:"command"`
		Subcommand    *string  `json:"subcommand"`
		Timestamp     string   `json:"timestamp"`
		Warnings      []string `json:"warnings"`
		OK            bool     `json:"ok"`
		Result        struct {
			Verified bool      `json:"verified"`
			Card     card.Card `json:"card"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(verifyOut), &verified); err != nil {
		t.Fatalf("unmarshal card verify output: %v, output=%s", err, verifyOut)
	}
	assertEnvelopeMetadata(t, verified.SchemaVersion, verified.Command, verified.Subcommand, verified.Timestamp, verified.Warnings, "card", stringPtr("verify"))
	if !verified.OK {
		t.Fatalf("expected ok=true output for verify")
	}
	if !verified.Result.Verified {
		t.Fatalf("expected verify result to be true")
	}
	if verified.Result.Card.ID != "did:key:z6MkAlice" {
		t.Fatalf("unexpected verified card id: %s", verified.Result.Card.ID)
	}
}

func TestRunInitSeedsRelayIntoExportedCard(t *testing.T) {
	home := filepath.Join(t.TempDir(), "linkclaw-home")
	t.Setenv(card.EnvRelayURL, "http://relay.example:8788")

	initCode, _, initErr := runForTest(
		t,
		[]string{
			"init",
			"--home", home,
			"--display-name", "Relay Seeded",
			"--non-interactive",
			"--json",
		},
		"",
	)
	if initCode != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", initCode, initErr)
	}

	exportCode, exportOut, exportErr := runForTest(t, []string{"card", "export", "--home", home, "--json"}, "")
	if exportCode != 0 {
		t.Fatalf("card export exit code = %d, stderr = %s", exportCode, exportErr)
	}

	var exported struct {
		Result struct {
			Card card.Card `json:"card"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(exportOut), &exported); err != nil {
		t.Fatalf("unmarshal card export output: %v, output=%s", err, exportOut)
	}
	if exported.Result.Card.Messaging.RelayURL != "http://relay.example:8788" {
		t.Fatalf("relay url = %q", exported.Result.Card.Messaging.RelayURL)
	}
	if exported.Result.Card.Messaging.RecipientID == "" {
		t.Fatalf("expected recipient id to be seeded during init")
	}
}

func TestRunCardImportJSON(t *testing.T) {
	t.Parallel()

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	initArgs := []string{
		"init",
		"--home", aliceHome,
		"--canonical-id", "did:key:z6MkAlice",
		"--display-name", "Alice",
		"--non-interactive",
		"--json",
	}
	initCode, _, initErr := runForTest(t, initArgs, "")
	if initCode != 0 {
		t.Fatalf("alice init exit code = %d, stderr = %s", initCode, initErr)
	}

	exportCode, exportOut, exportErr := runForTest(t, []string{"card", "export", "--home", aliceHome, "--json"}, "")
	if exportCode != 0 {
		t.Fatalf("card export exit code = %d, stderr = %s", exportCode, exportErr)
	}

	var exported struct {
		Result struct {
			Card card.Card `json:"card"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(exportOut), &exported); err != nil {
		t.Fatalf("unmarshal exported card: %v", err)
	}
	cardJSON, err := json.Marshal(exported.Result.Card)
	if err != nil {
		t.Fatalf("marshal exported card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	bobInitCode, _, bobInitErr := runForTest(t, []string{
		"init",
		"--home", bobHome,
		"--canonical-id", "did:key:z6MkBob",
		"--display-name", "Bob",
		"--non-interactive",
		"--json",
	}, "")
	if bobInitCode != 0 {
		t.Fatalf("bob init exit code = %d, stderr = %s", bobInitCode, bobInitErr)
	}

	importCode, importOut, importErr := runForTest(t, []string{"card", "import", "--home", bobHome, "--json", string(cardJSON)}, "")
	if importCode != 0 {
		t.Fatalf("card import exit code = %d, stderr = %s, stdout = %s", importCode, importErr, importOut)
	}

	var imported struct {
		SchemaVersion string   `json:"schema_version"`
		Command       string   `json:"command"`
		Subcommand    *string  `json:"subcommand"`
		Timestamp     string   `json:"timestamp"`
		Warnings      []string `json:"warnings"`
		OK            bool     `json:"ok"`
		Result        struct {
			ContactID string    `json:"contact_id"`
			Created   bool      `json:"created"`
			Card      card.Card `json:"card"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(importOut), &imported); err != nil {
		t.Fatalf("unmarshal imported card output: %v, output=%s", err, importOut)
	}
	assertEnvelopeMetadata(t, imported.SchemaVersion, imported.Command, imported.Subcommand, imported.Timestamp, imported.Warnings, "card", stringPtr("import"))
	if !imported.OK {
		t.Fatalf("expected ok=true output for import")
	}
	if !imported.Result.Created {
		t.Fatalf("expected imported contact to be created")
	}
	if imported.Result.ContactID == "" {
		t.Fatalf("expected contact id to be populated")
	}
	if imported.Result.Card.ID != "did:key:z6MkAlice" {
		t.Fatalf("unexpected imported card id: %s", imported.Result.Card.ID)
	}
}

func TestRunMessageSendAndOutboxJSON(t *testing.T) {
	t.Parallel()

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	aliceInitCode, _, aliceInitErr := runForTest(t, []string{
		"init",
		"--home", aliceHome,
		"--canonical-id", "did:key:z6MkAlice",
		"--display-name", "Alice",
		"--non-interactive",
		"--json",
	}, "")
	if aliceInitCode != 0 {
		t.Fatalf("alice init exit code = %d, stderr = %s", aliceInitCode, aliceInitErr)
	}
	exportCode, exportOut, exportErr := runForTest(t, []string{"card", "export", "--home", aliceHome, "--json"}, "")
	if exportCode != 0 {
		t.Fatalf("card export exit code = %d, stderr = %s", exportCode, exportErr)
	}
	var exported struct {
		Result struct {
			Card card.Card `json:"card"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(exportOut), &exported); err != nil {
		t.Fatalf("unmarshal exported card: %v", err)
	}
	cardJSON, err := json.Marshal(exported.Result.Card)
	if err != nil {
		t.Fatalf("marshal exported card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	bobInitCode, _, bobInitErr := runForTest(t, []string{
		"init",
		"--home", bobHome,
		"--canonical-id", "did:key:z6MkBob",
		"--display-name", "Bob",
		"--non-interactive",
		"--json",
	}, "")
	if bobInitCode != 0 {
		t.Fatalf("bob init exit code = %d, stderr = %s", bobInitCode, bobInitErr)
	}
	importCode, importOut, importErr := runForTest(t, []string{"card", "import", "--home", bobHome, "--json", string(cardJSON)}, "")
	if importCode != 0 {
		t.Fatalf("card import exit code = %d, stderr = %s", importCode, importErr)
	}
	var imported struct {
		Result struct {
			ContactID string `json:"contact_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(importOut), &imported); err != nil {
		t.Fatalf("unmarshal imported contact: %v", err)
	}

	sendCode, sendOut, sendErr := runForTest(t, []string{
		"message", "send",
		"--home", bobHome,
		"--body", "hello alice",
		"--json",
		imported.Result.ContactID,
	}, "")
	if sendCode != 0 {
		t.Fatalf("message send exit code = %d, stderr = %s, stdout = %s", sendCode, sendErr, sendOut)
	}
	var sent struct {
		SchemaVersion string   `json:"schema_version"`
		Command       string   `json:"command"`
		Subcommand    *string  `json:"subcommand"`
		Timestamp     string   `json:"timestamp"`
		Warnings      []string `json:"warnings"`
		OK            bool     `json:"ok"`
		Result        struct {
			Message struct {
				Status string `json:"status"`
				Body   string `json:"body"`
			} `json:"message"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(sendOut), &sent); err != nil {
		t.Fatalf("unmarshal send output: %v", err)
	}
	assertEnvelopeMetadata(t, sent.SchemaVersion, sent.Command, sent.Subcommand, sent.Timestamp, sent.Warnings, "message", stringPtr("send"))
	if !sent.OK {
		t.Fatalf("expected message send ok=true")
	}
	if sent.Result.Message.Status != "pending" {
		t.Fatalf("message status = %q, want pending", sent.Result.Message.Status)
	}

	outboxCode, outboxOut, outboxErr := runForTest(t, []string{"message", "outbox", "--home", bobHome, "--json"}, "")
	if outboxCode != 0 {
		t.Fatalf("message outbox exit code = %d, stderr = %s", outboxCode, outboxErr)
	}
	var outbox struct {
		SchemaVersion string   `json:"schema_version"`
		Command       string   `json:"command"`
		Subcommand    *string  `json:"subcommand"`
		Timestamp     string   `json:"timestamp"`
		Warnings      []string `json:"warnings"`
		OK            bool     `json:"ok"`
		Result        struct {
			Messages []struct {
				Body   string `json:"body"`
				Status string `json:"status"`
			} `json:"messages"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(outboxOut), &outbox); err != nil {
		t.Fatalf("unmarshal outbox output: %v", err)
	}
	assertEnvelopeMetadata(t, outbox.SchemaVersion, outbox.Command, outbox.Subcommand, outbox.Timestamp, outbox.Warnings, "message", stringPtr("outbox"))
	if len(outbox.Result.Messages) != 1 {
		t.Fatalf("outbox messages = %d, want 1", len(outbox.Result.Messages))
	}
	if outbox.Result.Messages[0].Body != "hello alice" {
		t.Fatalf("outbox body = %q, want hello alice", outbox.Result.Messages[0].Body)
	}
}

func TestRunMessageSyncJSONWithRelay(t *testing.T) {
	relay, relayResult, err := relayserver.Start(filepath.Join(t.TempDir(), "relay.db"), "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start relay: %v", err)
	}
	defer relay.Shutdown(context.Background())

	t.Setenv(card.EnvRelayURL, relayResult.URL)

	aliceHome := filepath.Join(t.TempDir(), "alice-home")
	aliceInitCode, _, aliceInitErr := runForTest(t, []string{
		"init",
		"--home", aliceHome,
		"--canonical-id", "did:key:z6MkAlice",
		"--display-name", "Alice",
		"--non-interactive",
		"--json",
	}, "")
	if aliceInitCode != 0 {
		t.Fatalf("alice init exit code = %d, stderr = %s", aliceInitCode, aliceInitErr)
	}
	aliceExportCode, aliceExportOut, aliceExportErr := runForTest(t, []string{"card", "export", "--home", aliceHome, "--json"}, "")
	if aliceExportCode != 0 {
		t.Fatalf("alice card export exit code = %d, stderr = %s", aliceExportCode, aliceExportErr)
	}
	var aliceExported struct {
		Result struct {
			Card card.Card `json:"card"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(aliceExportOut), &aliceExported); err != nil {
		t.Fatalf("unmarshal alice card: %v", err)
	}
	aliceCardJSON, err := json.Marshal(aliceExported.Result.Card)
	if err != nil {
		t.Fatalf("marshal alice card: %v", err)
	}

	bobHome := filepath.Join(t.TempDir(), "bob-home")
	bobInitCode, _, bobInitErr := runForTest(t, []string{
		"init",
		"--home", bobHome,
		"--canonical-id", "did:key:z6MkBob",
		"--display-name", "Bob",
		"--non-interactive",
		"--json",
	}, "")
	if bobInitCode != 0 {
		t.Fatalf("bob init exit code = %d, stderr = %s", bobInitCode, bobInitErr)
	}
	bobExportCode, bobExportOut, bobExportErr := runForTest(t, []string{"card", "export", "--home", bobHome, "--json"}, "")
	if bobExportCode != 0 {
		t.Fatalf("bob card export exit code = %d, stderr = %s", bobExportCode, bobExportErr)
	}
	var bobExported struct {
		Result struct {
			Card card.Card `json:"card"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(bobExportOut), &bobExported); err != nil {
		t.Fatalf("unmarshal bob card: %v", err)
	}
	bobCardJSON, err := json.Marshal(bobExported.Result.Card)
	if err != nil {
		t.Fatalf("marshal bob card: %v", err)
	}

	importAliceCode, importAliceOut, importAliceErr := runForTest(t, []string{"card", "import", "--home", bobHome, "--json", string(aliceCardJSON)}, "")
	if importAliceCode != 0 {
		t.Fatalf("import alice into bob exit code = %d, stderr = %s, stdout = %s", importAliceCode, importAliceErr, importAliceOut)
	}
	var importedAlice struct {
		Result struct {
			ContactID string `json:"contact_id"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(importAliceOut), &importedAlice); err != nil {
		t.Fatalf("unmarshal alice import: %v", err)
	}
	importBobCode, importBobOut, importBobErr := runForTest(t, []string{"card", "import", "--home", aliceHome, "--json", string(bobCardJSON)}, "")
	if importBobCode != 0 {
		t.Fatalf("import bob into alice exit code = %d, stderr = %s, stdout = %s", importBobCode, importBobErr, importBobOut)
	}

	sendCode, sendOut, sendErr := runForTest(t, []string{
		"message", "send",
		"--home", bobHome,
		"--body", "hello from bob",
		"--json",
		importedAlice.Result.ContactID,
	}, "")
	if sendCode != 0 {
		t.Fatalf("message send exit code = %d, stderr = %s, stdout = %s", sendCode, sendErr, sendOut)
	}

	syncCode, syncOut, syncErr := runForTest(t, []string{"message", "sync", "--home", aliceHome, "--json"}, "")
	if syncCode != 0 {
		t.Fatalf("message sync exit code = %d, stderr = %s, stdout = %s", syncCode, syncErr, syncOut)
	}
	var synced struct {
		OK     bool `json:"ok"`
		Result struct {
			Synced int `json:"synced"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(syncOut), &synced); err != nil {
		t.Fatalf("unmarshal sync output: %v", err)
	}
	if !synced.OK || synced.Result.Synced != 1 {
		t.Fatalf("sync result = %+v, want ok with synced=1", synced)
	}
}

func TestRunMessageStatusJSON(t *testing.T) {
	home := filepath.Join(t.TempDir(), "linkclaw-status-home")
	initCode, _, initErr := runForTest(t, []string{
		"init",
		"--home", home,
		"--canonical-id", "did:key:z6MkMessageStatus",
		"--display-name", "MessageStatus",
		"--non-interactive",
		"--json",
	}, "")
	if initCode != 0 {
		t.Fatalf("init exit code = %d, stderr = %s", initCode, initErr)
	}

	statusCode, statusOut, statusErr := runForTest(t, []string{"message", "status", "--home", home, "--json"}, "")
	if statusCode != 0 {
		t.Fatalf("message status exit code = %d, stderr = %s, stdout = %s", statusCode, statusErr, statusOut)
	}
	var out struct {
		OK     bool `json:"ok"`
		Result struct {
			DisplayName        string `json:"display_name"`
			Contacts           int    `json:"contacts"`
			Conversations      int    `json:"conversations"`
			PendingOutbox      int    `json:"pending_outbox"`
			StoreForwardRoutes int    `json:"store_forward_routes"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(statusOut), &out); err != nil {
		t.Fatalf("unmarshal status output: %v", err)
	}
	if !out.OK {
		t.Fatalf("expected message status ok=true")
	}
	if out.Result.DisplayName != "MessageStatus" {
		t.Fatalf("display name = %q, want MessageStatus", out.Result.DisplayName)
	}
	if out.Result.Contacts != 0 || out.Result.Conversations != 0 || out.Result.PendingOutbox != 0 {
		t.Fatalf("unexpected status counts: %+v", out.Result)
	}
	if out.Result.StoreForwardRoutes != 0 {
		t.Fatalf("store forward routes = %d, want 0", out.Result.StoreForwardRoutes)
	}
}

func TestRunVersionJSON(t *testing.T) {
	previousVersion := buildinfo.Version
	previousCommit := buildinfo.Commit
	previousBuildTime := buildinfo.BuildTime
	t.Cleanup(func() {
		buildinfo.Version = previousVersion
		buildinfo.Commit = previousCommit
		buildinfo.BuildTime = previousBuildTime
	})

	buildinfo.Version = "0.1.0"
	buildinfo.Commit = "abc123"
	buildinfo.BuildTime = "2026-03-14T00:00:00Z"

	code, stdout, stderr := runForTest(t, []string{"version", "--json"}, "")
	if code != 0 {
		t.Fatalf("version exit code = %d, stderr = %s", code, stderr)
	}

	var out versionOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("unmarshal stdout: %v, stdout=%s", err, stdout)
	}
	assertEnvelopeMetadata(t, out.SchemaVersion, out.Command, out.Subcommand, out.Timestamp, out.Warnings, "version", nil)
	if !out.OK {
		t.Fatalf("expected ok=true output: %+v", out)
	}
	if out.Result.Version != "0.1.0" {
		t.Fatalf("version = %q", out.Result.Version)
	}
	if out.Result.Commit != "abc123" {
		t.Fatalf("commit = %q", out.Result.Commit)
	}
	if out.Result.BuildTime != "2026-03-14T00:00:00Z" {
		t.Fatalf("build time = %q", out.Result.BuildTime)
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
	assertEnvelopeMetadata(t, out.SchemaVersion, out.Command, out.Subcommand, out.Timestamp, out.Warnings, "inspect", nil)
	if !out.OK {
		t.Fatalf("expected ok=true output: %+v", out)
	}
	if out.Result.VerificationState != "consistent" {
		t.Fatalf("verification_state = %q", out.Result.VerificationState)
	}
	if !out.Result.CanImport {
		t.Fatalf("expected can_import=true for consistent inspection")
	}
	if out.Result.CanonicalID != "did:web:fixture.example" {
		t.Fatalf("canonical id = %q", out.Result.CanonicalID)
	}
}

func TestRunInspectJSONRequiresInput(t *testing.T) {
	t.Parallel()

	code, stdout, stderr := runForTest(t, []string{"inspect", "--json"}, "")
	if code == 0 {
		t.Fatalf("expected inspect to fail without input")
	}
	if stderr != "" {
		t.Fatalf("expected stderr to stay empty for JSON validation error, got %q", stderr)
	}

	var out inspectOutput
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("unmarshal stdout: %v, stdout=%s", err, stdout)
	}
	assertEnvelopeMetadata(t, out.SchemaVersion, out.Command, out.Subcommand, out.Timestamp, out.Warnings, "inspect", nil)
	if out.OK {
		t.Fatalf("expected ok=false output")
	}
	if out.Error == nil {
		t.Fatalf("expected structured error")
	}
	if out.Error.Code != "invalid_input" {
		t.Fatalf("error code = %q", out.Error.Code)
	}
	if out.Error.Details["kind"] != "validation" {
		t.Fatalf("error details kind = %v", out.Error.Details["kind"])
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
	assertEnvelopeMetadata(t, out.SchemaVersion, out.Command, out.Subcommand, out.Timestamp, out.Warnings, "import", nil)
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
	assertEnvelopeMetadata(t, out.SchemaVersion, out.Command, out.Subcommand, out.Timestamp, out.Warnings, "known", stringPtr("trust"))
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
	assertEnvelopeMetadata(t, out.SchemaVersion, out.Command, out.Subcommand, out.Timestamp, out.Warnings, "known", stringPtr("note"))
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
	assertEnvelopeMetadata(t, out.SchemaVersion, out.Command, out.Subcommand, out.Timestamp, out.Warnings, "known", stringPtr("refresh"))
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
	assertEnvelopeMetadata(t, lsOut.SchemaVersion, lsOut.Command, lsOut.Subcommand, lsOut.Timestamp, lsOut.Warnings, "known", stringPtr("ls"))
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
	assertEnvelopeMetadata(t, showOut.SchemaVersion, showOut.Command, showOut.Subcommand, showOut.Timestamp, showOut.Warnings, "known", stringPtr("show"))
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
	assertEnvelopeMetadata(t, rmOut.SchemaVersion, rmOut.Command, rmOut.Subcommand, rmOut.Timestamp, rmOut.Warnings, "known", stringPtr("rm"))
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

func writeExecutableForTest(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
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

func assertEnvelopeMetadata(
	t *testing.T,
	schemaVersion string,
	command string,
	subcommand *string,
	timestamp string,
	warnings []string,
	wantCommand string,
	wantSubcommand *string,
) {
	t.Helper()

	if schemaVersion != cliSchemaVersion {
		t.Fatalf("schema_version = %q, want %q", schemaVersion, cliSchemaVersion)
	}
	if command != wantCommand {
		t.Fatalf("command = %q, want %q", command, wantCommand)
	}
	if wantSubcommand == nil {
		if subcommand != nil {
			t.Fatalf("subcommand = %q, want null", *subcommand)
		}
	} else {
		if subcommand == nil {
			t.Fatalf("subcommand = nil, want %q", *wantSubcommand)
		}
		if *subcommand != *wantSubcommand {
			t.Fatalf("subcommand = %q, want %q", *subcommand, *wantSubcommand)
		}
	}
	if warnings == nil {
		t.Fatalf("warnings should decode as an array")
	}
	if _, err := time.Parse(time.RFC3339, timestamp); err != nil {
		t.Fatalf("timestamp %q is not RFC3339: %v", timestamp, err)
	}
}
