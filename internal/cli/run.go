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

	"github.com/xiewanpeng/claw-identity/internal/initflow"
)

type initOutput struct {
	OK      bool            `json:"ok"`
	Command string          `json:"command"`
	Result  initflow.Result `json:"result,omitempty"`
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
}
