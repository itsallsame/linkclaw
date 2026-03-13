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

	"github.com/xiewanpeng/claw-identity/internal/importer"
	"github.com/xiewanpeng/claw-identity/internal/indexer"
	"github.com/xiewanpeng/claw-identity/internal/initflow"
	"github.com/xiewanpeng/claw-identity/internal/known"
	"github.com/xiewanpeng/claw-identity/internal/publish"
	"github.com/xiewanpeng/claw-identity/internal/resolver"
)

type initOutput struct {
	OK      bool            `json:"ok"`
	Command string          `json:"command"`
	Result  initflow.Result `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type publishOutput struct {
	OK      bool           `json:"ok"`
	Command string         `json:"command"`
	Result  publish.Result `json:"result,omitempty"`
	Error   string         `json:"error,omitempty"`
}

type inspectOutput struct {
	OK      bool            `json:"ok"`
	Command string          `json:"command"`
	Result  resolver.Result `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type importOutput struct {
	OK      bool            `json:"ok"`
	Command string          `json:"command"`
	Result  importer.Result `json:"result,omitempty"`
	Error   string          `json:"error,omitempty"`
}

type knownOutput[T any] struct {
	OK         bool   `json:"ok"`
	Command    string `json:"command"`
	Subcommand string `json:"subcommand"`
	Result     T      `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
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
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(errOut)

	home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
	canonicalID := fs.String("canonical-id", "", "canonical identity id (required in non-interactive mode)")
	displayName := fs.String("display-name", "", "human-readable display name")
	nonInteractive := fs.Bool("non-interactive", false, "disable prompts; requires --canonical-id")
	jsonOutput := fs.Bool("json", false, "emit JSON result")
	fs.BoolVar(jsonOutput, "j", false, "emit JSON result")

	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(errOut, "unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 1
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
		enc := json.NewEncoder(out)
		if err := enc.Encode(initOutput{OK: true, Command: "init", Result: result}); err != nil {
			fmt.Fprintf(errOut, "encode JSON output: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Fprintln(out, "linkclaw init completed")
	fmt.Fprintf(out, "home: %s\n", result.Home)
	fmt.Fprintf(out, "state db: %s\n", result.DBPath)
	fmt.Fprintf(out, "self: %s (%s)\n", result.Identity.DisplayName, result.Identity.CanonicalID)
	fmt.Fprintf(out, "key: %s (%s)\n", result.Key.KeyID, result.Key.Algorithm)
	return 0
}

func runPublish(ctx context.Context, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("publish", flag.ContinueOnError)
	fs.SetOutput(errOut)

	home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
	origin := fs.String("origin", "", "public home origin (for example https://agent.example)")
	outputDir := fs.String("output", "", "bundle output directory (defaults to <home>/publish)")
	tier := fs.String("tier", publish.TierRecommended, "publish tier: minimum|recommended|full")
	jsonOutput := fs.Bool("json", false, "emit JSON result")
	fs.BoolVar(jsonOutput, "j", false, "emit JSON result")

	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(errOut, "unexpected arguments: %s\n", strings.Join(fs.Args(), " "))
		return 1
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

	if *jsonOutput {
		enc := json.NewEncoder(out)
		if err := enc.Encode(publishOutput{OK: true, Command: "publish", Result: result}); err != nil {
			fmt.Fprintf(errOut, "encode JSON output: %v\n", err)
			return 1
		}
		return 0
	}

	fmt.Fprintln(out, "linkclaw publish completed")
	fmt.Fprintf(out, "home: %s\n", result.Home)
	fmt.Fprintf(out, "output: %s\n", result.OutputDir)
	fmt.Fprintf(out, "tier: %s\n", result.Tier)
	fmt.Fprintf(out, "origin: %s\n", result.HomeOrigin)
	fmt.Fprintf(out, "manifest: %s\n", result.ManifestPath)
	fmt.Fprintf(out, "artifacts: %d\n", len(result.Artifacts))
	return 0
}

func runInspect(ctx context.Context, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("inspect", flag.ContinueOnError)
	fs.SetOutput(errOut)

	jsonOutput := fs.Bool("json", false, "emit JSON result")
	fs.BoolVar(jsonOutput, "j", false, "emit JSON result")

	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) != 1 {
		fmt.Fprintln(errOut, "inspect requires exactly one input (domain or URL)")
		return 1
	}

	service := resolver.NewService()
	result, err := service.Inspect(ctx, fs.Args()[0])
	if err != nil {
		return writeInspectError(errOut, out, *jsonOutput, err)
	}

	if *jsonOutput {
		enc := json.NewEncoder(out)
		if err := enc.Encode(inspectOutput{OK: true, Command: "inspect", Result: result}); err != nil {
			fmt.Fprintf(errOut, "encode JSON output: %v\n", err)
			return 1
		}
		return 0
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
	fs := flag.NewFlagSet("import", flag.ContinueOnError)
	fs.SetOutput(errOut)

	home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
	allowDiscovered := fs.Bool("allow-discovered", false, "allow importing discovered identities")
	allowMismatch := fs.Bool("allow-mismatch", false, "allow importing mismatched identities")
	jsonOutput := fs.Bool("json", false, "emit JSON result")
	fs.BoolVar(jsonOutput, "j", false, "emit JSON result")

	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) != 1 {
		fmt.Fprintln(errOut, "import requires exactly one input (domain or URL)")
		return 1
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
		enc := json.NewEncoder(out)
		if err := enc.Encode(importOutput{OK: true, Command: "import", Result: result}); err != nil {
			fmt.Fprintf(errOut, "encode JSON output: %v\n", err)
			return 1
		}
		return 0
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
	if len(args) == 0 {
		printKnownUsage(out)
		return 0
	}

	switch args[0] {
	case "help", "-h", "--help":
		printKnownUsage(out)
		return 0
	case "ls":
		fs := flag.NewFlagSet("known ls", flag.ContinueOnError)
		fs.SetOutput(errOut)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if len(fs.Args()) > 0 {
			fmt.Fprintf(errOut, "known ls does not accept positional arguments: %s\n", strings.Join(fs.Args(), " "))
			return 1
		}
		service := known.NewService()
		result, err := service.List(ctx, known.ListOptions{Home: *home})
		if err != nil {
			return writeKnownError[known.ListResult](errOut, out, *jsonOutput, "ls", err)
		}
		if *jsonOutput {
			return writeKnownJSON(errOut, out, "ls", result)
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
		fs := flag.NewFlagSet("known show", flag.ContinueOnError)
		fs.SetOutput(errOut)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(errOut, "known show requires exactly one contact reference")
			return 1
		}
		service := known.NewService()
		result, err := service.Show(ctx, known.LookupOptions{Home: *home, Identifier: fs.Args()[0]})
		if err != nil {
			return writeKnownError[known.ShowResult](errOut, out, *jsonOutput, "show", err)
		}
		if *jsonOutput {
			return writeKnownJSON(errOut, out, "show", result)
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
		fs := flag.NewFlagSet("known trust", flag.ContinueOnError)
		fs.SetOutput(errOut)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		level := fs.String("level", "", "set trust level: unknown|seen|verified|trusted|pinned")
		risk := fs.String("risk", "", "comma-separated risk flags; pass empty string to clear")
		reason := fs.String("reason", "", "optional note for the trust update")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(errOut, "known trust requires exactly one contact reference")
			return 1
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
			return writeKnownJSON(errOut, out, "trust", result)
		}
		fmt.Fprintln(out, "linkclaw known trust updated")
		printKnownContactSummary(out, result.Contact)
		return 0
	case "note":
		fs := flag.NewFlagSet("known note", flag.ContinueOnError)
		fs.SetOutput(errOut)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		body := fs.String("body", "", "note body")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(errOut, "known note requires exactly one contact reference")
			return 1
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
			return writeKnownJSON(errOut, out, "note", result)
		}
		fmt.Fprintln(out, "linkclaw known note added")
		printKnownContactSummary(out, result.Contact)
		fmt.Fprintf(out, "note: %s\n", result.Note.Body)
		return 0
	case "refresh":
		fs := flag.NewFlagSet("known refresh", flag.ContinueOnError)
		fs.SetOutput(errOut)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(errOut, "known refresh requires exactly one contact reference")
			return 1
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
			return writeKnownJSON(errOut, out, "refresh", result)
		}
		fmt.Fprintln(out, "linkclaw known refresh completed")
		printKnownContactSummary(out, result.Contact)
		fmt.Fprintf(out, "snapshots: %d\n", result.SnapshotCount)
		fmt.Fprintf(out, "proofs: %d\n", result.ProofCount)
		fmt.Fprintf(out, "status: %s\n", result.Inspection.Status)
		return 0
	case "rm":
		fs := flag.NewFlagSet("known rm", flag.ContinueOnError)
		fs.SetOutput(errOut)
		home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
		jsonOutput := fs.Bool("json", false, "emit JSON result")
		fs.BoolVar(jsonOutput, "j", false, "emit JSON result")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if len(fs.Args()) != 1 {
			fmt.Fprintln(errOut, "known rm requires exactly one contact reference")
			return 1
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
			return writeKnownJSON(errOut, out, "rm", result)
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
		enc := json.NewEncoder(out)
		if encodeErr := enc.Encode(initOutput{OK: false, Command: "init", Error: err.Error()}); encodeErr != nil {
			fmt.Fprintf(errOut, "encode error JSON output: %v\n", encodeErr)
			return 1
		}
		return 1
	}
	fmt.Fprintf(errOut, "init failed: %v\n", err)
	return 1
}

func writePublishError(errOut, out io.Writer, jsonOutput bool, err error) int {
	if jsonOutput {
		enc := json.NewEncoder(out)
		if encodeErr := enc.Encode(publishOutput{OK: false, Command: "publish", Error: err.Error()}); encodeErr != nil {
			fmt.Fprintf(errOut, "encode error JSON output: %v\n", encodeErr)
			return 1
		}
		return 1
	}
	fmt.Fprintf(errOut, "publish failed: %v\n", err)
	return 1
}

func writeInspectError(errOut, out io.Writer, jsonOutput bool, err error) int {
	if jsonOutput {
		enc := json.NewEncoder(out)
		if encodeErr := enc.Encode(inspectOutput{OK: false, Command: "inspect", Error: err.Error()}); encodeErr != nil {
			fmt.Fprintf(errOut, "encode error JSON output: %v\n", encodeErr)
			return 1
		}
		return 1
	}
	fmt.Fprintf(errOut, "inspect failed: %v\n", err)
	return 1
}

func writeImportError(errOut, out io.Writer, jsonOutput bool, err error) int {
	if jsonOutput {
		enc := json.NewEncoder(out)
		if encodeErr := enc.Encode(importOutput{OK: false, Command: "import", Error: err.Error()}); encodeErr != nil {
			fmt.Fprintf(errOut, "encode error JSON output: %v\n", encodeErr)
			return 1
		}
		return 1
	}
	fmt.Fprintf(errOut, "import failed: %v\n", err)
	return 1
}

func writeKnownJSON[T any](errOut, out io.Writer, subcommand string, result T) int {
	enc := json.NewEncoder(out)
	if err := enc.Encode(knownOutput[T]{OK: true, Command: "known", Subcommand: subcommand, Result: result}); err != nil {
		fmt.Fprintf(errOut, "encode JSON output: %v\n", err)
		return 1
	}
	return 0
}

func writeKnownError[T any](errOut, out io.Writer, jsonOutput bool, subcommand string, err error) int {
	if jsonOutput {
		enc := json.NewEncoder(out)
		if encodeErr := enc.Encode(knownOutput[T]{OK: false, Command: "known", Subcommand: subcommand, Error: err.Error()}); encodeErr != nil {
			fmt.Fprintf(errOut, "encode error JSON output: %v\n", encodeErr)
			return 1
		}
		return 1
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
