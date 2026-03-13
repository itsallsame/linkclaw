package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/importer"
	"github.com/xiewanpeng/claw-identity/internal/indexer"
	"github.com/xiewanpeng/claw-identity/internal/initflow"
	"github.com/xiewanpeng/claw-identity/internal/known"
	"github.com/xiewanpeng/claw-identity/internal/publish"
	"github.com/xiewanpeng/claw-identity/internal/resolver"
)

const cliSchemaVersion = "linkclaw.cli.v1"

var envelopeNow = func() time.Time {
	return time.Now().UTC()
}

type jsonEnvelope struct {
	SchemaVersion string             `json:"schema_version"`
	Command       string             `json:"command"`
	Subcommand    *string            `json:"subcommand"`
	OK            bool               `json:"ok"`
	Timestamp     string             `json:"timestamp"`
	Warnings      []string           `json:"warnings"`
	Result        any                `json:"result,omitempty"`
	Error         *jsonEnvelopeError `json:"error,omitempty"`
}

type jsonEnvelopeError struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details"`
}

type inspectJSONResult struct {
	Input             string              `json:"input"`
	NormalizedOrigin  string              `json:"normalized_origin,omitempty"`
	Resource          string              `json:"resource,omitempty"`
	VerificationState string              `json:"verification_state"`
	CanImport         bool                `json:"can_import"`
	CanonicalID       string              `json:"canonical_id,omitempty"`
	DisplayName       string              `json:"display_name,omitempty"`
	ProfileURL        string              `json:"profile_url,omitempty"`
	Artifacts         []resolver.Artifact `json:"artifacts"`
	Proofs            []resolver.Proof    `json:"proofs,omitempty"`
	Mismatches        []string            `json:"mismatches,omitempty"`
	Warnings          []string            `json:"warnings,omitempty"`
	ResolvedAt        string              `json:"resolved_at"`
}

type initOutput struct {
	SchemaVersion string             `json:"schema_version"`
	Command       string             `json:"command"`
	Subcommand    *string            `json:"subcommand"`
	OK            bool               `json:"ok"`
	Timestamp     string             `json:"timestamp"`
	Warnings      []string           `json:"warnings"`
	Result        initflow.Result    `json:"result"`
	Error         *jsonEnvelopeError `json:"error,omitempty"`
}

type publishOutput struct {
	SchemaVersion string             `json:"schema_version"`
	Command       string             `json:"command"`
	Subcommand    *string            `json:"subcommand"`
	OK            bool               `json:"ok"`
	Timestamp     string             `json:"timestamp"`
	Warnings      []string           `json:"warnings"`
	Result        publish.Result     `json:"result"`
	Error         *jsonEnvelopeError `json:"error,omitempty"`
}

type inspectOutput struct {
	SchemaVersion string             `json:"schema_version"`
	Command       string             `json:"command"`
	Subcommand    *string            `json:"subcommand"`
	OK            bool               `json:"ok"`
	Timestamp     string             `json:"timestamp"`
	Warnings      []string           `json:"warnings"`
	Result        inspectJSONResult  `json:"result"`
	Error         *jsonEnvelopeError `json:"error,omitempty"`
}

type importOutput struct {
	SchemaVersion string             `json:"schema_version"`
	Command       string             `json:"command"`
	Subcommand    *string            `json:"subcommand"`
	OK            bool               `json:"ok"`
	Timestamp     string             `json:"timestamp"`
	Warnings      []string           `json:"warnings"`
	Result        importer.Result    `json:"result"`
	Error         *jsonEnvelopeError `json:"error,omitempty"`
}

type knownOutput[T any] struct {
	SchemaVersion string             `json:"schema_version"`
	Command       string             `json:"command"`
	Subcommand    *string            `json:"subcommand"`
	OK            bool               `json:"ok"`
	Timestamp     string             `json:"timestamp"`
	Warnings      []string           `json:"warnings"`
	Result        T                  `json:"result"`
	Error         *jsonEnvelopeError `json:"error,omitempty"`
}

type indexOutput[T any] struct {
	OK         bool   `json:"ok"`
	Command    string `json:"command"`
	Subcommand string `json:"subcommand"`
	Result     T      `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
}

func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		printUsage(out)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		printUsage(out)
		return 0
	case "version":
		return runVersion(args[1:], out, errOut)
	case "serve":
		return runServe(ctx, args[1:], out, errOut)
	case "init":
		return runInit(ctx, args[1:], in, out, errOut)
	case "publish":
		return runPublish(ctx, args[1:], out, errOut)
	case "inspect":
		return runInspect(ctx, args[1:], out, errOut)
	case "import":
		return runImport(ctx, args[1:], out, errOut)
	case "index":
		return runIndex(ctx, args[1:], out, errOut)
	case "known":
		return runKnown(ctx, args[1:], out, errOut)
	default:
		fmt.Fprintf(errOut, "unknown command %q\n", args[0])
		printUsage(errOut)
		return 1
	}
}

func runInit(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer) int {
	jsonRequested := hasJSONFlag(args)
	fs := newFlagSet("init", errOut, jsonRequested)

	home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
	canonicalID := fs.String("canonical-id", "", "canonical identity id (required in non-interactive mode)")
	displayName := fs.String("display-name", "", "human-readable display name")
	nonInteractive := fs.Bool("non-interactive", false, "disable prompts; requires --canonical-id")
	jsonOutput := fs.Bool("json", false, "emit JSON result")
	fs.BoolVar(jsonOutput, "j", false, "emit JSON result")

	if err := fs.Parse(args); err != nil {
		if jsonRequested {
			return writeJSONCommandError(errOut, out, "init", nil, newFlagParseError(err))
		}
		return 1
	}
	if len(fs.Args()) > 0 {
		return writeValidationFailure(
			errOut,
			out,
			*jsonOutput,
			"init",
			nil,
			fmt.Sprintf("unexpected arguments: %s", strings.Join(fs.Args(), " ")),
		)
	}

	canon := strings.TrimSpace(*canonicalID)
	display := strings.TrimSpace(*displayName)
	reader := bufio.NewReader(in)

	if canon == "" && !*nonInteractive {
		value, err := prompt(reader, errOut, "Canonical ID: ")
		if err != nil {
			return writeInitError(errOut, out, *jsonOutput, err)
		}
		canon = strings.TrimSpace(value)
	}
	if display == "" && !*nonInteractive {
		value, err := prompt(reader, errOut, "Display Name (optional): ")
		if err != nil {
			return writeInitError(errOut, out, *jsonOutput, err)
		}
		display = strings.TrimSpace(value)
	}
	if canon == "" {
		return writeInitError(errOut, out, *jsonOutput, errors.New("canonical id is required (set --canonical-id or use interactive prompt)"))
	}

	service := initflow.NewService()
	result, err := service.Init(ctx, initflow.Options{
		Home:        *home,
		CanonicalID: canon,
		DisplayName: display,
	})
	if err != nil {
		return writeInitError(errOut, out, *jsonOutput, err)
	}

	if *jsonOutput {
		return writeJSONCommandResult(errOut, out, "init", nil, nil, result)
	}

	fmt.Fprintln(out, "linkclaw init completed")
	fmt.Fprintf(out, "home: %s\n", result.Home)
	fmt.Fprintf(out, "state db: %s\n", result.DBPath)
	fmt.Fprintf(out, "self: %s (%s)\n", result.Identity.DisplayName, result.Identity.CanonicalID)
	fmt.Fprintf(out, "key: %s (%s)\n", result.Key.KeyID, result.Key.Algorithm)
	return 0
}

func runPublish(ctx context.Context, args []string, out, errOut io.Writer) int {
	jsonRequested := hasJSONFlag(args)
	fs := newFlagSet("publish", errOut, jsonRequested)

	home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
	origin := fs.String("origin", "", "public home origin (for example https://agent.example)")
	outputDir := fs.String("output", "", "bundle output directory (defaults to <home>/publish)")
	tier := fs.String("tier", publish.TierRecommended, "publish tier: minimum|recommended|full")
	deployTarget := fs.String("deploy", "", "optional deployment target: cloudflare")
	projectName := fs.String("project", "", "Cloudflare Pages project name (required with --deploy cloudflare)")
	jsonOutput := fs.Bool("json", false, "emit JSON result")
	fs.BoolVar(jsonOutput, "j", false, "emit JSON result")

	if err := fs.Parse(args); err != nil {
		if jsonRequested {
			return writeJSONCommandError(errOut, out, "publish", nil, newFlagParseError(err))
		}
		return 1
	}
	if len(fs.Args()) > 0 {
		return writeValidationFailure(
			errOut,
			out,
			*jsonOutput,
			"publish",
			nil,
			fmt.Sprintf("unexpected arguments: %s", strings.Join(fs.Args(), " ")),
		)
	}
	deploy, err := resolvePublishDeploy(*deployTarget, *projectName)
	if err != nil {
		return writeValidationFailure(errOut, out, *jsonOutput, "publish", nil, err.Error())
	}

	service := publish.NewService()
	result, err := service.Publish(ctx, publish.Options{
		Home:   *home,
		Origin: *origin,
		Output: *outputDir,
		Tier:   *tier,
	})
	if err != nil {
		return writePublishError(errOut, out, *jsonOutput, err)
	}
	if err := deployPublishBundle(ctx, errOut, *jsonOutput, &result, deploy); err != nil {
		return writePublishError(errOut, out, *jsonOutput, err)
	}

	if *jsonOutput {
		return writeJSONCommandResult(errOut, out, "publish", nil, nil, result)
	}

	fmt.Fprintln(out, "linkclaw publish completed")
	fmt.Fprintf(out, "home: %s\n", result.Home)
	fmt.Fprintf(out, "output: %s\n", result.OutputDir)
	fmt.Fprintf(out, "headers: %s\n", result.HeadersPath)
	fmt.Fprintf(out, "tier: %s\n", result.Tier)
	fmt.Fprintf(out, "origin: %s\n", result.HomeOrigin)
	fmt.Fprintf(out, "manifest: %s\n", result.ManifestPath)
	fmt.Fprintf(out, "artifacts: %d\n", len(result.Artifacts))
	if result.Deployment != nil {
		fmt.Fprintf(out, "deploy: %s (%s via %s)\n", result.Deployment.Provider, result.Deployment.Project, result.Deployment.Tool)
	}
	return 0
}

func runInspect(ctx context.Context, args []string, out, errOut io.Writer) int {
	jsonRequested := hasJSONFlag(args)
	fs := newFlagSet("inspect", errOut, jsonRequested)

	jsonOutput := fs.Bool("json", false, "emit JSON result")
	fs.BoolVar(jsonOutput, "j", false, "emit JSON result")

	if err := fs.Parse(args); err != nil {
		if jsonRequested {
			return writeJSONCommandError(errOut, out, "inspect", nil, newFlagParseError(err))
		}
		return 1
	}
	if len(fs.Args()) != 1 {
		return writeValidationFailure(errOut, out, *jsonOutput, "inspect", nil, "inspect requires exactly one input (domain or URL)")
	}

	service := resolver.NewService()
	result, err := service.Inspect(ctx, fs.Args()[0])
	if err != nil {
		return writeInspectError(errOut, out, *jsonOutput, err)
	}

	if *jsonOutput {
		return writeJSONCommandResult(errOut, out, "inspect", nil, result.Warnings, makeInspectJSONResult(result))
	}

	fmt.Fprintln(out, "linkclaw inspect completed")
	fmt.Fprintf(out, "input: %s\n", result.Input)
	fmt.Fprintf(out, "origin: %s\n", result.NormalizedOrigin)
	fmt.Fprintf(out, "status: %s\n", result.Status)
	if result.CanonicalID != "" {
		fmt.Fprintf(out, "canonical id: %s\n", result.CanonicalID)
	}
	fmt.Fprintf(out, "artifacts: %d\n", len(result.Artifacts))
	fmt.Fprintf(out, "proofs: %d\n", len(result.Proofs))
	return 0
}

func runImport(ctx context.Context, args []string, out, errOut io.Writer) int {
	jsonRequested := hasJSONFlag(args)
	fs := newFlagSet("import", errOut, jsonRequested)

	home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
	allowDiscovered := fs.Bool("allow-discovered", false, "allow importing discovered identities")
	allowMismatch := fs.Bool("allow-mismatch", false, "allow importing mismatched identities")
	jsonOutput := fs.Bool("json", false, "emit JSON result")
	fs.BoolVar(jsonOutput, "j", false, "emit JSON result")

	if err := fs.Parse(args); err != nil {
		if jsonRequested {
			return writeJSONCommandError(errOut, out, "import", nil, newFlagParseError(err))
		}
		return 1
	}
	if len(fs.Args()) != 1 {
		return writeValidationFailure(errOut, out, *jsonOutput, "import", nil, "import requires exactly one input (domain or URL)")
	}

	service := importer.NewService()
	result, err := service.Import(ctx, importer.Options{
		Home:            *home,
		Input:           fs.Args()[0],
		AllowDiscovered: *allowDiscovered,
		AllowMismatch:   *allowMismatch,
	})
	if err != nil {
		return writeImportError(errOut, out, *jsonOutput, err)
	}

	if *jsonOutput {
		return writeJSONCommandResult(errOut, out, "import", nil, result.Inspection.Warnings, result)
	}

	fmt.Fprintln(out, "linkclaw import completed")
	fmt.Fprintf(out, "home: %s\n", result.Home)
	fmt.Fprintf(out, "contact: %s\n", result.ContactID)
	fmt.Fprintf(out, "status: %s\n", result.Inspection.Status)
	fmt.Fprintf(out, "snapshots: %d\n", result.SnapshotCount)
	fmt.Fprintf(out, "proofs: %d\n", result.ProofCount)
	return 0
}

func runKnown(ctx context.Context, args []string, out, errOut io.Writer) int {
	jsonRequested := hasJSONFlag(args)
	if len(args) == 0 {
		if jsonRequested {
			return writeValidationFailure(errOut, out, true, "known", nil, "known requires a subcommand")
		}
		printKnownUsage(out)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		printKnownUsage(out)
		return 0
	case "ls":
		fs := newFlagSet("known ls", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "known", stringPtr("ls"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) > 0 {
			return writeValidationFailure(
				errOut,
				out,
				*jsonOutput,
				"known",
				stringPtr("ls"),
				fmt.Sprintf("known ls does not accept positional arguments: %s", strings.Join(fs.Args(), " ")),
			)
		}
		service := known.NewService()
		result, err := service.List(ctx, known.ListOptions{Home: *home})
		if err != nil {
			return writeKnownError[known.ListResult](errOut, out, *jsonOutput, "ls", err)
		}
		if *jsonOutput {
			return writeKnownJSON(errOut, out, "ls", nil, result)
		}
		if len(result.Contacts) == 0 {
			fmt.Fprintln(out, "linkclaw known ls")
			fmt.Fprintln(out, "contacts: 0")
			return 0
		}
		fmt.Fprintln(out, "linkclaw known ls")
		fmt.Fprintf(out, "contacts: %d\n", len(result.Contacts))
		for _, contact := range result.Contacts {
			fmt.Fprintf(
				out,
				"- %s | %s | trust=%s verification=%s | notes=%d\n",
				contact.DisplayName,
				contact.CanonicalID,
				contact.Trust.TrustLevel,
				contact.Trust.VerificationState,
				contact.NoteCount,
			)
		}
		return 0
	case "show":
		fs := newFlagSet("known show", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "known", stringPtr("show"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) != 1 {
			return writeValidationFailure(errOut, out, *jsonOutput, "known", stringPtr("show"), "known show requires exactly one contact reference")
		}
		service := known.NewService()
		result, err := service.Show(ctx, known.LookupOptions{Home: *home, Identifier: fs.Args()[0]})
		if err != nil {
			return writeKnownError[known.ShowResult](errOut, out, *jsonOutput, "show", err)
		}
		if *jsonOutput {
			return writeKnownJSON(errOut, out, "show", nil, result)
		}
		printKnownContactSummary(out, result.Contact.ContactSummary)
		fmt.Fprintf(out, "handles: %d\n", len(result.Contact.Handles))
		for _, handle := range result.Contact.Handles {
			marker := " "
			if handle.IsPrimary {
				marker = "*"
			}
			fmt.Fprintf(out, "  %s %s=%s\n", marker, handle.HandleType, handle.Value)
		}
		fmt.Fprintf(out, "artifacts: %d\n", len(result.Contact.Artifacts))
		fmt.Fprintf(out, "proofs: %d\n", len(result.Contact.Proofs))
		fmt.Fprintf(out, "notes: %d\n", len(result.Contact.Notes))
		for _, note := range result.Contact.Notes {
			fmt.Fprintf(out, "  - %s (%s)\n", note.Body, note.CreatedAt)
		}
		fmt.Fprintf(out, "events: %d\n", len(result.Contact.Events))
		for _, event := range result.Contact.Events {
			fmt.Fprintf(out, "  - [%s] %s\n", event.EventType, event.Summary)
		}
		return 0
	case "trust":
		fs := newFlagSet("known trust", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		level := fs.String("level", "", "set trust level: unknown|seen|verified|trusted|pinned")
		risk := fs.String("risk", "", "comma-separated risk flags; pass empty string to clear")
		reason := fs.String("reason", "", "optional note for the trust update")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "known", stringPtr("trust"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) != 1 {
			return writeValidationFailure(errOut, out, *jsonOutput, "known", stringPtr("trust"), "known trust requires exactly one contact reference")
		}
		service := known.NewService()
		result, err := service.Trust(ctx, known.TrustOptions{
			Home:         *home,
			Identifier:   fs.Args()[0],
			Level:        *level,
			RiskFlags:    splitCSV(*risk),
			HasRiskFlags: flagProvided(fs, "risk"),
			Reason:       *reason,
		})
		if err != nil {
			return writeKnownError[known.TrustResult](errOut, out, *jsonOutput, "trust", err)
		}
		if *jsonOutput {
			return writeKnownJSON(errOut, out, "trust", nil, result)
		}
		fmt.Fprintln(out, "linkclaw known trust updated")
		printKnownContactSummary(out, result.Contact)
		return 0
	case "note":
		fs := newFlagSet("known note", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		body := fs.String("body", "", "note body")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "known", stringPtr("note"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) != 1 {
			return writeValidationFailure(errOut, out, *jsonOutput, "known", stringPtr("note"), "known note requires exactly one contact reference")
		}
		service := known.NewService()
		result, err := service.Note(ctx, known.NoteOptions{
			Home:       *home,
			Identifier: fs.Args()[0],
			Body:       *body,
		})
		if err != nil {
			return writeKnownError[known.NoteResult](errOut, out, *jsonOutput, "note", err)
		}
		if *jsonOutput {
			return writeKnownJSON(errOut, out, "note", nil, result)
		}
		fmt.Fprintln(out, "linkclaw known note added")
		printKnownContactSummary(out, result.Contact)
		fmt.Fprintf(out, "note: %s\n", result.Note.Body)
		return 0
	case "refresh":
		fs := newFlagSet("known refresh", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "known", stringPtr("refresh"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) != 1 {
			return writeValidationFailure(errOut, out, *jsonOutput, "known", stringPtr("refresh"), "known refresh requires exactly one contact reference")
		}
		service := known.NewService()
		result, err := service.Refresh(ctx, known.RefreshOptions{
			Home:       *home,
			Identifier: fs.Args()[0],
		})
		if err != nil {
			return writeKnownError[known.RefreshResult](errOut, out, *jsonOutput, "refresh", err)
		}
		if *jsonOutput {
			return writeKnownJSON(errOut, out, "refresh", result.Inspection.Warnings, result)
		}
		fmt.Fprintln(out, "linkclaw known refresh completed")
		printKnownContactSummary(out, result.Contact)
		fmt.Fprintf(out, "snapshots: %d\n", result.SnapshotCount)
		fmt.Fprintf(out, "proofs: %d\n", result.ProofCount)
		fmt.Fprintf(out, "status: %s\n", result.Inspection.Status)
		return 0
	case "rm":
		fs := newFlagSet("known rm", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "known", stringPtr("rm"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) != 1 {
			return writeValidationFailure(errOut, out, *jsonOutput, "known", stringPtr("rm"), "known rm requires exactly one contact reference")
		}
		service := known.NewService()
		result, err := service.Remove(ctx, known.RemoveOptions{
			Home:       *home,
			Identifier: fs.Args()[0],
		})
		if err != nil {
			return writeKnownError[known.RemoveResult](errOut, out, *jsonOutput, "rm", err)
		}
		if *jsonOutput {
			return writeKnownJSON(errOut, out, "rm", nil, result)
		}
		fmt.Fprintln(out, "linkclaw known rm completed")
		fmt.Fprintf(out, "contact: %s (%s)\n", result.Contact.DisplayName, result.Contact.ContactID)
		fmt.Fprintf(out, "deleted contacts: %d\n", result.Removed.Contacts)
		fmt.Fprintf(out, "deleted trust records: %d\n", result.Removed.TrustRecords)
		fmt.Fprintf(out, "deleted handles: %d\n", result.Removed.Handles)
		fmt.Fprintf(out, "deleted artifacts: %d\n", result.Removed.Artifacts)
		fmt.Fprintf(out, "deleted proofs: %d\n", result.Removed.Proofs)
		fmt.Fprintf(out, "deleted notes: %d\n", result.Removed.Notes)
		fmt.Fprintf(out, "deleted events: %d\n", result.Removed.Events)
		return 0
	default:
		if jsonRequested {
			return writeValidationFailure(errOut, out, true, "known", stringPtr(args[0]), fmt.Sprintf("unknown known subcommand %q", args[0]))
		}
		fmt.Fprintf(errOut, "unknown known subcommand %q\n", args[0])
		printKnownUsage(errOut)
		return 1
	}
}

func runIndex(ctx context.Context, args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		printIndexUsage(out)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		printIndexUsage(out)
		return 0
	case "crawl":
		fs := flag.NewFlagSet("index crawl", flag.ContinueOnError)
		fs.SetOutput(errOut)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(errOut, "index crawl requires exactly one input (domain or URL)")
			return 1
		}
		service := indexer.NewService()
		result, err := service.Crawl(ctx, indexer.CrawlOptions{
			Home:  *home,
			Input: fs.Args()[0],
		})
		if err != nil {
			return writeIndexError[indexer.CrawlResult](errOut, out, *jsonOutput, "crawl", err)
		}
		if *jsonOutput {
			return writeIndexJSON(errOut, out, "crawl", result)
		}
		fmt.Fprintln(out, "linkclaw index crawl completed")
		printIndexRecord(out, result.Record)
		return 0
	case "search":
		fs := flag.NewFlagSet("index search", flag.ContinueOnError)
		fs.SetOutput(errOut)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if len(fs.Args()) > 1 {
			fmt.Fprintln(errOut, "index search accepts at most one query")
			return 1
		}
		query := ""
		if len(fs.Args()) == 1 {
			query = fs.Args()[0]
		}
		service := indexer.NewService()
		result, err := service.Search(ctx, indexer.SearchOptions{
			Home:  *home,
			Query: query,
		})
		if err != nil {
			return writeIndexError[indexer.SearchResult](errOut, out, *jsonOutput, "search", err)
		}
		if *jsonOutput {
			return writeIndexJSON(errOut, out, "search", result)
		}
		fmt.Fprintln(out, "linkclaw index search")
		if query != "" {
			fmt.Fprintf(out, "query: %s\n", query)
		}
		fmt.Fprintf(out, "records: %d\n", len(result.Records))
		for _, record := range result.Records {
			fmt.Fprintln(out)
			printIndexRecord(out, record)
		}
		return 0
	default:
		fmt.Fprintf(errOut, "unknown index subcommand %q\n", args[0])
		printIndexUsage(errOut)
		return 1
	}
}

func writeInitError(errOut, out io.Writer, jsonOutput bool, err error) int {
	if jsonOutput {
		return writeJSONCommandError(errOut, out, "init", nil, newCommandError(err))
	}
	fmt.Fprintf(errOut, "init failed: %v\n", err)
	return 1
}

func writePublishError(errOut, out io.Writer, jsonOutput bool, err error) int {
	if jsonOutput {
		return writeJSONCommandError(errOut, out, "publish", nil, newCommandError(err))
	}
	fmt.Fprintf(errOut, "publish failed: %v\n", err)
	return 1
}

func writeInspectError(errOut, out io.Writer, jsonOutput bool, err error) int {
	if jsonOutput {
		return writeJSONCommandError(errOut, out, "inspect", nil, newCommandError(err))
	}
	fmt.Fprintf(errOut, "inspect failed: %v\n", err)
	return 1
}

func writeImportError(errOut, out io.Writer, jsonOutput bool, err error) int {
	if jsonOutput {
		return writeJSONCommandError(errOut, out, "import", nil, newCommandError(err))
	}
	fmt.Fprintf(errOut, "import failed: %v\n", err)
	return 1
}

func writeKnownJSON[T any](errOut, out io.Writer, subcommand string, warnings []string, result T) int {
	return writeJSONCommandResult(errOut, out, "known", stringPtr(subcommand), warnings, result)
}

func writeKnownError[T any](errOut, out io.Writer, jsonOutput bool, subcommand string, err error) int {
	if jsonOutput {
		return writeJSONCommandError(errOut, out, "known", stringPtr(subcommand), newCommandError(err))
	}
	fmt.Fprintf(errOut, "known %s failed: %v\n", subcommand, err)
	return 1
}

func writeIndexJSON[T any](errOut, out io.Writer, subcommand string, result T) int {
	enc := json.NewEncoder(out)
	if err := enc.Encode(indexOutput[T]{OK: true, Command: "index", Subcommand: subcommand, Result: result}); err != nil {
		fmt.Fprintf(errOut, "encode JSON output: %v\n", err)
		return 1
	}
	return 0
}

func writeIndexError[T any](errOut, out io.Writer, jsonOutput bool, subcommand string, err error) int {
	if jsonOutput {
		enc := json.NewEncoder(out)
		if encodeErr := enc.Encode(indexOutput[T]{OK: false, Command: "index", Subcommand: subcommand, Error: err.Error()}); encodeErr != nil {
			fmt.Fprintf(errOut, "encode error JSON output: %v\n", encodeErr)
			return 1
		}
		return 1
	}
	fmt.Fprintf(errOut, "index %s failed: %v\n", subcommand, err)
	return 1
}

func writeJSONCommandResult(errOut, out io.Writer, command string, subcommand *string, warnings []string, result any) int {
	return writeJSONEnvelope(errOut, out, jsonEnvelope{
		SchemaVersion: cliSchemaVersion,
		Command:       command,
		Subcommand:    subcommand,
		OK:            true,
		Timestamp:     envelopeNow().Format(time.RFC3339),
		Warnings:      normalizeWarnings(warnings),
		Result:        result,
	})
}

func writeJSONCommandError(errOut, out io.Writer, command string, subcommand *string, err *jsonEnvelopeError) int {
	return writeJSONEnvelope(errOut, out, jsonEnvelope{
		SchemaVersion: cliSchemaVersion,
		Command:       command,
		Subcommand:    subcommand,
		OK:            false,
		Timestamp:     envelopeNow().Format(time.RFC3339),
		Warnings:      []string{},
		Error:         err,
	})
}

func writeJSONEnvelope(errOut, out io.Writer, envelope jsonEnvelope) int {
	enc := json.NewEncoder(out)
	if err := enc.Encode(envelope); err != nil {
		fmt.Fprintf(errOut, "encode JSON output: %v\n", err)
		return 1
	}
	if envelope.OK {
		return 0
	}
	return 1
}

func writeValidationFailure(errOut, out io.Writer, jsonOutput bool, command string, subcommand *string, message string) int {
	if jsonOutput {
		return writeJSONCommandError(errOut, out, command, subcommand, newValidationError(message))
	}
	fmt.Fprintln(errOut, message)
	return 1
}

func newFlagSet(name string, errOut io.Writer, jsonRequested bool) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	if jsonRequested {
		fs.SetOutput(io.Discard)
		return fs
	}
	fs.SetOutput(errOut)
	return fs
}

func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--json" || arg == "-j" {
			return true
		}
	}
	return false
}

func newFlagParseError(err error) *jsonEnvelopeError {
	return &jsonEnvelopeError{
		Code:      "invalid_input",
		Message:   err.Error(),
		Retryable: false,
		Details: map[string]any{
			"kind": "flag_parse",
		},
	}
}

func newValidationError(message string) *jsonEnvelopeError {
	return &jsonEnvelopeError{
		Code:      "invalid_input",
		Message:   message,
		Retryable: false,
		Details: map[string]any{
			"kind": "validation",
		},
	}
}

func newCommandError(err error) *jsonEnvelopeError {
	return &jsonEnvelopeError{
		Code:      commandErrorCode(err),
		Message:   err.Error(),
		Retryable: isRetryableError(err),
		Details: map[string]any{
			"kind": "command",
		},
	}
}

func commandErrorCode(err error) string {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(message, "state db not found"):
		return "state_not_initialized"
	case strings.Contains(message, " not found"):
		return "not_found"
	case strings.Contains(message, "requires exactly one"),
		strings.Contains(message, "does not accept positional arguments"),
		strings.Contains(message, "unexpected arguments"),
		strings.Contains(message, "canonical id is required"),
		strings.Contains(message, "must include"),
		strings.Contains(message, "must use"),
		strings.Contains(message, "must not"),
		strings.Contains(message, "unsupported"):
		return "invalid_input"
	default:
		return "command_failed"
	}
}

func isRetryableError(err error) bool {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "timeout") ||
		strings.Contains(message, "temporar") ||
		strings.Contains(message, "connection refused") ||
		strings.Contains(message, "connection reset") ||
		strings.Contains(message, "unexpected http status 5")
}

func normalizeWarnings(warnings []string) []string {
	if len(warnings) == 0 {
		return []string{}
	}
	return warnings
}

func makeInspectJSONResult(result resolver.Result) inspectJSONResult {
	return inspectJSONResult{
		Input:             result.Input,
		NormalizedOrigin:  result.NormalizedOrigin,
		Resource:          result.Resource,
		VerificationState: result.Status,
		CanImport:         canImportInspection(result.Status),
		CanonicalID:       result.CanonicalID,
		DisplayName:       result.DisplayName,
		ProfileURL:        result.ProfileURL,
		Artifacts:         result.Artifacts,
		Proofs:            result.Proofs,
		Mismatches:        result.Mismatches,
		Warnings:          result.Warnings,
		ResolvedAt:        result.ResolvedAt,
	}
}

func canImportInspection(status string) bool {
	switch status {
	case resolver.StatusResolved, resolver.StatusConsistent:
		return true
	default:
		return false
	}
}

func stringPtr(value string) *string {
	return &value
}

func flagProvided(fs *flag.FlagSet, name string) bool {
	provided := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			provided = true
		}
	})
	return provided
}

func splitCSV(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

func printKnownContactSummary(out io.Writer, contact known.ContactSummary) {
	fmt.Fprintf(out, "contact: %s\n", contact.ContactID)
	fmt.Fprintf(out, "name: %s\n", contact.DisplayName)
	fmt.Fprintf(out, "canonical id: %s\n", contact.CanonicalID)
	if contact.HomeOrigin != "" {
		fmt.Fprintf(out, "home origin: %s\n", contact.HomeOrigin)
	}
	if contact.ProfileURL != "" {
		fmt.Fprintf(out, "profile url: %s\n", contact.ProfileURL)
	}
	fmt.Fprintf(out, "status: %s\n", contact.Status)
	fmt.Fprintf(out, "trust: %s\n", contact.Trust.TrustLevel)
	if contact.Trust.VerificationState != "" {
		fmt.Fprintf(out, "verification: %s\n", contact.Trust.VerificationState)
	}
	if len(contact.Trust.RiskFlags) > 0 {
		fmt.Fprintf(out, "risk flags: %s\n", strings.Join(contact.Trust.RiskFlags, ", "))
	}
	fmt.Fprintf(out, "notes: %d\n", contact.NoteCount)
	if contact.LastSeenAt != "" {
		fmt.Fprintf(out, "last seen: %s\n", contact.LastSeenAt)
	}
	if contact.LastEventAt != "" {
		fmt.Fprintf(out, "last event: %s\n", contact.LastEventAt)
	}
}

func printIndexRecord(out io.Writer, record indexer.Record) {
	fmt.Fprintf(out, "record: %s\n", record.RecordID)
	fmt.Fprintf(out, "name: %s\n", record.DisplayName)
	if record.CanonicalID != "" {
		fmt.Fprintf(out, "canonical id: %s\n", record.CanonicalID)
	}
	fmt.Fprintf(out, "origin: %s\n", record.NormalizedOrigin)
	if record.ProfileURL != "" {
		fmt.Fprintf(out, "profile url: %s\n", record.ProfileURL)
	}
	fmt.Fprintf(out, "status: %s\n", record.ResolverStatus)
	fmt.Fprintf(out, "conflict: %s\n", record.ConflictState)
	fmt.Fprintf(out, "freshness: %s\n", record.Freshness)
	fmt.Fprintf(out, "source urls: %d\n", len(record.SourceURLs))
	for _, sourceURL := range record.SourceURLs {
		fmt.Fprintf(out, "  - %s\n", sourceURL)
	}
}

func prompt(reader *bufio.Reader, out io.Writer, label string) (string, error) {
	fmt.Fprint(out, label)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if errors.Is(err, io.EOF) && strings.TrimSpace(line) == "" {
		return "", io.EOF
	}
	return strings.TrimSpace(line), nil
}

func printUsage(out io.Writer) {
	fmt.Fprintln(out, "LinkClaw CLI")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  linkclaw <command> [flags]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Commands:")
	fmt.Fprintln(out, "  version Show build and release metadata")
	fmt.Fprintln(out, "  serve   Serve a publish bundle locally with correct MIME types")
	fmt.Fprintln(out, "  init    Initialize LinkClaw local home and state")
	fmt.Fprintln(out, "  publish Compile and bundle publishable identity artifacts")
	fmt.Fprintln(out, "  inspect Resolve and verify public identity artifacts")
	fmt.Fprintln(out, "  import  Resolve, verify, and persist a public identity")
	fmt.Fprintln(out, "  index   Crawl and search the local read-only public index")
	fmt.Fprintln(out, "  known   Read and manage the local trust book")
}

func printKnownUsage(out io.Writer) {
	fmt.Fprintln(out, "LinkClaw known")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  linkclaw known <subcommand> [flags] <contact>")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  ls       List known contacts")
	fmt.Fprintln(out, "  show     Show one known contact")
	fmt.Fprintln(out, "  trust    Update trust level and risk flags")
	fmt.Fprintln(out, "  note     Append a note to a contact")
	fmt.Fprintln(out, "  refresh  Re-resolve and persist current public artifacts")
	fmt.Fprintln(out, "  rm       Remove a contact from the local trust book")
}

func printIndexUsage(out io.Writer) {
	fmt.Fprintln(out, "LinkClaw index")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  linkclaw index <subcommand> [flags] [query]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  crawl   Crawl one public identity surface into the local read-only index")
	fmt.Fprintln(out, "  search  Search index records and show freshness/conflict/source urls")
}
