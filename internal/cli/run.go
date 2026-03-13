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
	"github.com/xiewanpeng/claw-identity/internal/initflow"
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
}
