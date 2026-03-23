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

	"github.com/xiewanpeng/claw-identity/internal/card"
	"github.com/xiewanpeng/claw-identity/internal/importer"
	"github.com/xiewanpeng/claw-identity/internal/indexer"
	"github.com/xiewanpeng/claw-identity/internal/initflow"
	"github.com/xiewanpeng/claw-identity/internal/known"
	"github.com/xiewanpeng/claw-identity/internal/message"
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

type cardOutput[T any] struct {
	SchemaVersion string             `json:"schema_version"`
	Command       string             `json:"command"`
	Subcommand    *string            `json:"subcommand"`
	OK            bool               `json:"ok"`
	Timestamp     string             `json:"timestamp"`
	Warnings      []string           `json:"warnings"`
	Result        T                  `json:"result"`
	Error         *jsonEnvelopeError `json:"error,omitempty"`
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
	case "card":
		return runCard(ctx, args[1:], out, errOut)
	case "message":
		return runMessage(ctx, args[1:], out, errOut)
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
	canonicalID := fs.String("canonical-id", "", "canonical identity id (optional; auto-generates did:key when omitted)")
	displayName := fs.String("display-name", "", "human-readable display name")
	nonInteractive := fs.Bool("non-interactive", false, "disable prompts")
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

	if display == "" && !*nonInteractive {
		value, err := prompt(reader, errOut, "Display Name (optional): ")
		if err != nil {
			return writeInitError(errOut, out, *jsonOutput, err)
		}
		display = strings.TrimSpace(value)
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
	fmt.Fprintf(out, "messaging: %s | ready=%t | recipient=%s\n", humanMessagingTransportLabel(result.Messaging.Transport), result.Messaging.Ready, result.Messaging.RecipientID)
	if strings.TrimSpace(result.Messaging.RelayURL) != "" {
		fmt.Fprintf(out, "offline recovery endpoint: %s\n", result.Messaging.RelayURL)
	}
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

func runMessage(ctx context.Context, args []string, out, errOut io.Writer) int {
	jsonRequested := hasJSONFlag(args)
	if len(args) == 0 {
		if jsonRequested {
			return writeValidationFailure(errOut, out, true, "message", nil, "message requires a subcommand")
		}
		printMessageUsage(out)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		printMessageUsage(out)
		return 0
	case "send":
		fs := newFlagSet("message send", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		body := fs.String("body", "", "message body")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "message", stringPtr("send"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) != 1 {
			return writeValidationFailure(errOut, out, *jsonOutput, "message", stringPtr("send"), "message send requires exactly one contact reference")
		}
		service := message.NewService()
		result, err := service.Send(ctx, message.SendOptions{
			Home:       *home,
			ContactRef: fs.Args()[0],
			Body:       *body,
		})
		if err != nil {
			return writeMessageError[message.SendResult](errOut, out, *jsonOutput, "send", err)
		}
		if *jsonOutput {
			return writeMessageJSON(errOut, out, "send", result)
		}
		headline := "Message queued for delivery."
		switch result.Message.TransportStatus {
		case message.TransportStatusDirect:
			headline = "Message delivered via direct transport."
		case message.TransportStatusDeferred:
			headline = "Message deferred for recovery delivery."
		case message.TransportStatusFailed:
			headline = "Message delivery failed."
		}
		fmt.Fprintln(out, headline)
		fmt.Fprintf(out, "conversation: %s\n", result.Conversation.ConversationID)
		fmt.Fprintf(out, "message: %s\n", result.Message.MessageID)
		fmt.Fprintf(out, "status: %s\n", result.Message.Status)
		if strings.TrimSpace(result.Message.TransportStatus) != "" {
			fmt.Fprintf(out, "transport status: %s\n", result.Message.TransportStatus)
		}
		fmt.Fprintln(out, "Next:")
		if result.Message.TransportStatus == message.TransportStatusDirect {
			fmt.Fprintln(out, "- the recipient can open `linkclaw message thread <contact>` immediately")
		} else {
			fmt.Fprintln(out, "- the recipient needs to run `linkclaw message sync` to receive it")
		}
		fmt.Fprintln(out, "- run `linkclaw message inbox` to review local conversation state")
		return 0
	case "inbox":
		fs := newFlagSet("message inbox", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "message", stringPtr("inbox"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) > 0 {
			return writeValidationFailure(errOut, out, *jsonOutput, "message", stringPtr("inbox"), "message inbox does not accept positional arguments")
		}
		service := message.NewService()
		result, err := service.Inbox(ctx, message.ListOptions{Home: *home})
		if err != nil {
			return writeMessageError[message.InboxResult](errOut, out, *jsonOutput, "inbox", err)
		}
		if *jsonOutput {
			return writeMessageJSON(errOut, out, "inbox", result)
		}
		fmt.Fprintln(out, "LinkClaw inbox")
		fmt.Fprintf(out, "conversations: %d\n", len(result.Conversations))
		hasUnknownSender := false
		for _, conversation := range result.Conversations {
			label := "known"
			if strings.TrimSpace(conversation.ContactStatus) == "discovered" {
				label = "new sender"
				hasUnknownSender = true
			}
			fmt.Fprintf(out, "- %s | %s | %s | unread=%d | last=%s\n", conversation.ContactDisplayName, conversation.ContactCanonicalID, label, conversation.UnreadCount, conversation.LastMessagePreview)
		}
		if hasUnknownSender {
			fmt.Fprintln(out, "Next:")
			fmt.Fprintln(out, "- ask the sender for an identity card")
			fmt.Fprintln(out, "- then run `linkclaw card import <card>` if you want to keep them")
		} else if len(result.Conversations) > 0 {
			fmt.Fprintln(out, "Next:")
			fmt.Fprintln(out, "- reply with `linkclaw message send <contact> --body \"...\"`")
		}
		return 0
	case "thread":
		fs := newFlagSet("message thread", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		limit := fs.Int("limit", 20, "maximum number of messages to show")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "message", stringPtr("thread"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) != 1 {
			return writeValidationFailure(errOut, out, *jsonOutput, "message", stringPtr("thread"), "message thread requires exactly one contact reference")
		}
		service := message.NewService()
		result, err := service.Thread(ctx, message.ThreadOptions{
			Home:       *home,
			ContactRef: fs.Args()[0],
			Limit:      *limit,
			MarkRead:   true,
		})
		if err != nil {
			return writeMessageError[message.ThreadResult](errOut, out, *jsonOutput, "thread", err)
		}
		if *jsonOutput {
			return writeMessageJSON(errOut, out, "thread", result)
		}
		fmt.Fprintln(out, "LinkClaw thread")
		fmt.Fprintf(out, "contact: %s\n", result.Conversation.ContactDisplayName)
		fmt.Fprintf(out, "canonical id: %s\n", result.Conversation.ContactCanonicalID)
		fmt.Fprintf(out, "messages: %d\n", len(result.Conversation.Messages))
		for _, msg := range result.Conversation.Messages {
			transportStatus := strings.TrimSpace(msg.TransportStatus)
			if transportStatus != "" {
				fmt.Fprintf(out, "- [%s] %s | status=%s | transport=%s | %s\n", msg.Direction, msg.CreatedAt, msg.Status, transportStatus, msg.Body)
				continue
			}
			fmt.Fprintf(out, "- [%s] %s | %s\n", msg.Direction, msg.CreatedAt, msg.Body)
		}
		if len(result.Conversation.Messages) == 0 {
			fmt.Fprintln(out, "No messages in this thread yet.")
		}
		return 0
	case "outbox":
		fs := newFlagSet("message outbox", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "message", stringPtr("outbox"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) > 0 {
			return writeValidationFailure(errOut, out, *jsonOutput, "message", stringPtr("outbox"), "message outbox does not accept positional arguments")
		}
		service := message.NewService()
		result, err := service.Outbox(ctx, message.ListOptions{Home: *home})
		if err != nil {
			return writeMessageError[message.OutboxResult](errOut, out, *jsonOutput, "outbox", err)
		}
		if *jsonOutput {
			return writeMessageJSON(errOut, out, "outbox", result)
		}
		fmt.Fprintln(out, "LinkClaw outbox")
		fmt.Fprintf(out, "messages: %d\n", len(result.Messages))
		for _, msg := range result.Messages {
			transportStatus := strings.TrimSpace(msg.TransportStatus)
			if transportStatus != "" {
				fmt.Fprintf(out, "- %s | %s | transport=%s | %s\n", msg.MessageID, msg.Status, transportStatus, msg.Preview)
				continue
			}
			fmt.Fprintf(out, "- %s | %s | %s\n", msg.MessageID, msg.Status, msg.Preview)
		}
		if len(result.Messages) > 0 {
			fmt.Fprintln(out, "Next:")
			fmt.Fprintln(out, "- wait for the recipient to run `linkclaw message sync`")
		}
		return 0
	case "sync":
		fs := newFlagSet("message sync", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "message", stringPtr("sync"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) > 0 {
			return writeValidationFailure(errOut, out, *jsonOutput, "message", stringPtr("sync"), "message sync does not accept positional arguments")
		}
		service := message.NewService()
		result, err := service.Sync(ctx, message.SyncOptions{Home: *home})
		if err != nil {
			return writeMessageError[message.SyncResult](errOut, out, *jsonOutput, "sync", err)
		}
		if *jsonOutput {
			return writeMessageJSON(errOut, out, "sync", result)
		}
		fmt.Fprintln(out, "LinkClaw sync completed")
		fmt.Fprintf(out, "synced: %d\n", result.Synced)
		fmt.Fprintf(out, "recovery checks: %d\n", result.RelayCalls)
		fmt.Fprintln(out, "Next:")
		if result.Synced > 0 {
			fmt.Fprintln(out, "- run `linkclaw message inbox` to read new messages")
		} else {
			fmt.Fprintln(out, "- no new messages yet; run `linkclaw message send <contact> --body \"...\"` to start a conversation")
		}
		return 0
	case "status":
		fs := newFlagSet("message status", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "message", stringPtr("status"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) > 0 {
			return writeValidationFailure(errOut, out, *jsonOutput, "message", stringPtr("status"), "message status does not accept positional arguments")
		}
		service := message.NewService()
		result, err := service.Status(ctx, message.ListOptions{Home: *home})
		if err != nil {
			return writeMessageError[message.StatusResult](errOut, out, *jsonOutput, "status", err)
		}
		if *jsonOutput {
			return writeMessageJSON(errOut, out, "status", result)
		}
		fmt.Fprintln(out, "LinkClaw message status")
		if result.DisplayName != "" {
			fmt.Fprintf(out, "identity: %s\n", result.DisplayName)
		}
		if result.SelfID != "" {
			fmt.Fprintf(out, "self id: %s\n", result.SelfID)
		}
		if result.PeerID != "" {
			fmt.Fprintf(out, "peer id: %s\n", result.PeerID)
		}
		fmt.Fprintf(out, "identity ready: %t\n", result.IdentityReady)
		fmt.Fprintf(out, "transport ready: %t\n", result.TransportReady)
		fmt.Fprintf(out, "discovery ready: %t\n", result.DiscoveryReady)
		fmt.Fprintf(out, "contacts: %d\n", result.Contacts)
		fmt.Fprintf(out, "conversations: %d\n", result.Conversations)
		fmt.Fprintf(out, "unread: %d\n", result.Unread)
		fmt.Fprintf(out, "pending outbox: %d\n", result.PendingOutbox)
		fmt.Fprintf(out, "message transport status: direct=%d deferred=%d recovered=%d\n", result.MessageStatusDirect, result.MessageStatusDeferred, result.MessageStatusRecovered)
		fmt.Fprintf(out, "presence cache: %d\n", result.PresenceEntries)
		fmt.Fprintf(out, "offline recovery paths: %d\n", result.StoreForwardRoutes)
		fmt.Fprintf(out, "direct enabled: %t\n", result.DirectEnabled)
		if result.LastStoreForwardSyncAt != "" {
			fmt.Fprintf(out, "last recovery check: %s | result=%s | recovered=%d\n", result.LastStoreForwardSyncAt, result.LastStoreForwardResult, result.LastRecoveredCount)
		}
		if result.LastStoreForwardError != "" {
			fmt.Fprintf(out, "last recovery issue: %s\n", result.LastStoreForwardError)
		}
		if len(result.RecentRouteOutcomes) > 0 {
			fmt.Fprintln(out, "recent delivery outcomes:")
			for _, item := range result.RecentRouteOutcomes {
				fmt.Fprintf(out, "- %s | %s | %s\n", humanRouteOutcomeLabel(item.RouteType), item.Outcome, item.AttemptedAt)
			}
		}
		fmt.Fprintln(out, "Next:")
		fmt.Fprintln(out, "- run `linkclaw message inbox` to inspect recent conversations")
		fmt.Fprintln(out, "- run `linkclaw message sync` to recover new messages")
		return 0
	case "inspect-trust":
		fs := newFlagSet("message inspect-trust", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "message", stringPtr("inspect-trust"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) != 1 {
			return writeValidationFailure(errOut, out, *jsonOutput, "message", stringPtr("inspect-trust"), "message inspect-trust requires exactly one contact reference")
		}
		service := message.NewService()
		result, err := service.InspectTrust(ctx, message.InspectTrustOptions{
			Home:       *home,
			Identifier: fs.Args()[0],
		})
		if err != nil {
			return writeMessageError[map[string]any](errOut, out, *jsonOutput, "inspect-trust", err)
		}
		if *jsonOutput {
			return writeMessageJSON(errOut, out, "inspect-trust", result)
		}
		fmt.Fprintln(out, "LinkClaw trust inspection")
		fmt.Fprintf(out, "canonical id: %s\n", result.CanonicalID)
		fmt.Fprintf(out, "trust level: %s\n", result.Summary.TrustLevel)
		if result.Summary.VerificationState != "" {
			fmt.Fprintf(out, "verification: %s\n", result.Summary.VerificationState)
		}
		fmt.Fprintf(out, "confidence: %s (%.2f)\n", result.Summary.ConfidenceLevel, result.Summary.ConfidenceScore)
		fmt.Fprintf(out, "reachability: %s\n", result.Summary.Reachability)
		return 0
	case "list-discovery":
		fs := newFlagSet("message list-discovery", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		capability := fs.String("capability", "", "filter by one capability (for example direct)")
		capabilities := fs.String("capabilities", "", "comma-separated capability filters")
		source := fs.String("source", "", "filter by discovery source")
		freshOnly := fs.Bool("fresh-only", false, "include only fresh discovery records")
		limit := fs.Int("limit", 0, "maximum records to return; 0 means no limit")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "message", stringPtr("list-discovery"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) > 0 {
			return writeValidationFailure(errOut, out, *jsonOutput, "message", stringPtr("list-discovery"), "message list-discovery does not accept positional arguments")
		}
		service := message.NewService()
		result, err := service.ListDiscovery(ctx, message.ListDiscoveryOptions{
			Home:         *home,
			Capability:   *capability,
			Capabilities: splitCSV(*capabilities),
			Source:       *source,
			FreshOnly:    *freshOnly,
			Limit:        *limit,
		})
		if err != nil {
			return writeMessageError[map[string]any](errOut, out, *jsonOutput, "list-discovery", err)
		}
		if *jsonOutput {
			return writeMessageJSON(errOut, out, "list-discovery", result)
		}
		fmt.Fprintln(out, "LinkClaw discovery list")
		fmt.Fprintf(out, "records: %d\n", len(result.Records))
		for _, record := range result.Records {
			fmt.Fprintf(out, "- %s | reachable=%t | source=%s\n", record.CanonicalID, record.Reachable, record.Source)
		}
		return 0
	case "connect-peer":
		fs := newFlagSet("message connect-peer", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		refresh := fs.Bool("refresh", false, "refresh discovery view before connecting")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "message", stringPtr("connect-peer"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) != 1 {
			return writeValidationFailure(errOut, out, *jsonOutput, "message", stringPtr("connect-peer"), "message connect-peer requires exactly one contact reference")
		}
		service := message.NewService()
		result, err := service.ConnectPeer(ctx, message.ConnectPeerOptions{
			Home:       *home,
			ContactRef: fs.Args()[0],
			Refresh:    *refresh,
		})
		if err != nil {
			return writeMessageError[map[string]any](errOut, out, *jsonOutput, "connect-peer", err)
		}
		if *jsonOutput {
			return writeMessageJSON(errOut, out, "connect-peer", result)
		}
		fmt.Fprintln(out, "LinkClaw connect peer")
		fmt.Fprintf(out, "canonical id: %s\n", result.CanonicalID)
		fmt.Fprintf(out, "connected: %t\n", result.Connected)
		if result.Transport != "" {
			fmt.Fprintf(out, "transport: %s\n", result.Transport)
		}
		if result.Reason != "" {
			fmt.Fprintf(out, "reason: %s\n", result.Reason)
		}
		return 0
	case "receive-direct":
		fs := newFlagSet("message receive-direct", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		input := fs.String("input", "", "direct envelope JSON payload")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "message", stringPtr("receive-direct"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) > 0 {
			return writeValidationFailure(errOut, out, *jsonOutput, "message", stringPtr("receive-direct"), "message receive-direct does not accept positional arguments")
		}
		if strings.TrimSpace(*input) == "" {
			return writeValidationFailure(errOut, out, *jsonOutput, "message", stringPtr("receive-direct"), "message receive-direct requires --input")
		}
		service := message.NewService()
		err := service.ReceiveDirect(ctx, message.ReceiveDirectOptions{
			Home:    *home,
			Payload: *input,
		})
		if err != nil {
			return writeMessageError[map[string]any](errOut, out, *jsonOutput, "receive-direct", err)
		}
		if *jsonOutput {
			return writeMessageJSON(errOut, out, "receive-direct", map[string]any{"accepted": true})
		}
		fmt.Fprintln(out, "LinkClaw direct message accepted")
		return 0
	default:
		if jsonRequested {
			return writeValidationFailure(errOut, out, true, "message", stringPtr(args[0]), fmt.Sprintf("unknown message subcommand %q", args[0]))
		}
		fmt.Fprintf(errOut, "unknown message subcommand %q\n", args[0])
		printMessageUsage(errOut)
		return 1
	}
}

func runCard(ctx context.Context, args []string, out, errOut io.Writer) int {
	jsonRequested := hasJSONFlag(args)
	if len(args) == 0 {
		if jsonRequested {
			return writeValidationFailure(errOut, out, true, "card", nil, "card requires a subcommand")
		}
		printCardUsage(out)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		printCardUsage(out)
		return 0
	case "export":
		fs := newFlagSet("card export", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "card", stringPtr("export"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) > 0 {
			return writeValidationFailure(errOut, out, *jsonOutput, "card", stringPtr("export"), "card export does not accept positional arguments")
		}
		service := card.NewService()
		result, err := service.Export(ctx, card.Options{Home: *home})
		if err != nil {
			return writeCardError[card.ExportResult](errOut, out, *jsonOutput, "export", err)
		}
		if *jsonOutput {
			return writeCardJSON(errOut, out, "export", result)
		}
		encoded, err := json.MarshalIndent(result.Card, "", "  ")
		if err != nil {
			fmt.Fprintf(errOut, "encode card export output: %v\n", err)
			return 1
		}
		fmt.Fprintln(out, string(encoded))
		return 0
	case "verify":
		fs := newFlagSet("card verify", errOut, jsonRequested)
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "card", stringPtr("verify"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) != 1 {
			return writeValidationFailure(errOut, out, *jsonOutput, "card", stringPtr("verify"), "card verify requires exactly one input (file path or inline JSON)")
		}
		service := card.NewService()
		result, err := service.Verify(ctx, card.VerifyOptions{Input: fs.Args()[0]})
		if err != nil {
			return writeCardError[card.VerifyResult](errOut, out, *jsonOutput, "verify", err)
		}
		if *jsonOutput {
			return writeCardJSON(errOut, out, "verify", result)
		}
		fmt.Fprintln(out, "Identity card verified.")
		fmt.Fprintf(out, "verified: %t\n", result.Verified)
		fmt.Fprintf(out, "source: %s\n", result.Source)
		fmt.Fprintf(out, "id: %s\n", result.Card.ID)
		fmt.Fprintf(out, "name: %s\n", result.Card.DisplayName)
		fmt.Fprintf(out, "transport: %s\n", result.Card.Messaging.Transport)
		fmt.Fprintf(out, "recipient: %s\n", result.Card.Messaging.RecipientID)
		fmt.Fprintln(out, "Next:")
		fmt.Fprintln(out, "- import it with `linkclaw card import <card>` if you want to save this contact")
		return 0
	case "import":
		fs := newFlagSet("card import", errOut, jsonRequested)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			if jsonRequested {
				return writeJSONCommandError(errOut, out, "card", stringPtr("import"), newFlagParseError(err))
			}
			return 1
		}
		if len(fs.Args()) != 1 {
			return writeValidationFailure(errOut, out, *jsonOutput, "card", stringPtr("import"), "card import requires exactly one input (file path or inline JSON)")
		}
		service := card.NewService()
		result, err := service.Import(ctx, card.ImportOptions{
			Home:  *home,
			Input: fs.Args()[0],
		})
		if err != nil {
			return writeCardError[card.ImportResult](errOut, out, *jsonOutput, "import", err)
		}
		if *jsonOutput {
			return writeCardJSON(errOut, out, "import", result)
		}
		fmt.Fprintln(out, "Contact added to your LinkClaw contacts.")
		fmt.Fprintf(out, "contact: %s\n", result.ContactID)
		fmt.Fprintf(out, "created: %t\n", result.Created)
		fmt.Fprintf(out, "id: %s\n", result.Card.ID)
		fmt.Fprintf(out, "name: %s\n", result.Card.DisplayName)
		fmt.Fprintln(out, "Next:")
		fmt.Fprintln(out, "- send a message with `linkclaw message send <contact> --body \"...\"`")
		fmt.Fprintln(out, "- or run `linkclaw message inbox` to review conversations")
		return 0
	default:
		if jsonRequested {
			return writeValidationFailure(errOut, out, true, "card", stringPtr(args[0]), fmt.Sprintf("unknown card subcommand %q", args[0]))
		}
		fmt.Fprintf(errOut, "unknown card subcommand %q\n", args[0])
		printCardUsage(errOut)
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

func writeCardJSON[T any](errOut, out io.Writer, subcommand string, result T) int {
	return writeJSONCommandResult(errOut, out, "card", stringPtr(subcommand), nil, result)
}

func writeCardError[T any](errOut, out io.Writer, jsonOutput bool, subcommand string, err error) int {
	if jsonOutput {
		return writeJSONCommandError(errOut, out, "card", stringPtr(subcommand), newCommandError(err))
	}
	fmt.Fprintf(errOut, "card %s failed: %v\n", subcommand, err)
	return 1
}

func writeMessageJSON[T any](errOut, out io.Writer, subcommand string, result T) int {
	return writeJSONCommandResult(errOut, out, "message", stringPtr(subcommand), nil, result)
}

func writeMessageError[T any](errOut, out io.Writer, jsonOutput bool, subcommand string, err error) int {
	if jsonOutput {
		return writeJSONCommandError(errOut, out, "message", stringPtr(subcommand), newCommandError(err))
	}
	fmt.Fprintf(errOut, "message %s failed: %v\n", subcommand, err)
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

func humanMessagingTransportLabel(_ string) string {
	return "runtime-managed"
}

func humanRouteOutcomeLabel(routeType string) string {
	switch strings.ToLower(strings.TrimSpace(routeType)) {
	case "direct":
		return "direct transport"
	case "store_forward":
		return "offline recovery"
	default:
		return "delivery path"
	}
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
	fmt.Fprintln(out, "  card    Export and verify local identity cards")
	fmt.Fprintln(out, "  message Queue and inspect local direct messages")
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

func printCardUsage(out io.Writer) {
	fmt.Fprintln(out, "LinkClaw card")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  linkclaw card <subcommand> [flags] [input]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  export  Export the local signed identity card")
	fmt.Fprintln(out, "  import  Import a verified identity card as a local contact")
	fmt.Fprintln(out, "  verify  Verify an identity card from a file path or inline JSON")
}

func printMessageUsage(out io.Writer) {
	fmt.Fprintln(out, "LinkClaw message")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  linkclaw message <subcommand> [flags] [contact]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "Subcommands:")
	fmt.Fprintln(out, "  send    Queue a message for one imported contact")
	fmt.Fprintln(out, "  inbox   List local conversations")
	fmt.Fprintln(out, "  thread  Show recent messages for one contact")
	fmt.Fprintln(out, "  outbox  List queued outgoing messages")
	fmt.Fprintln(out, "  sync    Recover new messages through the runtime")
	fmt.Fprintln(out, "  status  Show runtime messaging readiness and counters")
	fmt.Fprintln(out, "  inspect-trust   Inspect trust summary for one peer")
	fmt.Fprintln(out, "  list-discovery  List cached discovery records")
	fmt.Fprintln(out, "  connect-peer    Evaluate runtime connect readiness for one peer")
	fmt.Fprintln(out, "  receive-direct  Accept one direct-envelope payload")
}
