package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/buildinfo"
	"github.com/xiewanpeng/claw-identity/internal/cloudflare"
	"github.com/xiewanpeng/claw-identity/internal/devserver"
	"github.com/xiewanpeng/claw-identity/internal/layout"
	"github.com/xiewanpeng/claw-identity/internal/publish"
)

type versionOutput struct {
	SchemaVersion string             `json:"schema_version"`
	Command       string             `json:"command"`
	Subcommand    *string            `json:"subcommand"`
	OK            bool               `json:"ok"`
	Timestamp     string             `json:"timestamp"`
	Warnings      []string           `json:"warnings"`
	Result        buildinfo.Info     `json:"result"`
	Error         *jsonEnvelopeError `json:"error,omitempty"`
}

func runVersion(args []string, out, errOut io.Writer) int {
	jsonRequested := hasJSONFlag(args)
	fs := newFlagSet("version", errOut, jsonRequested)
	jsonOutput := fs.Bool("json", false, "emit JSON result")
	fs.BoolVar(jsonOutput, "j", false, "emit JSON result")

	if err := fs.Parse(args); err != nil {
		if jsonRequested {
			return writeJSONCommandError(errOut, out, "version", nil, newFlagParseError(err))
		}
		return 1
	}
	if len(fs.Args()) > 0 {
		return writeValidationFailure(
			errOut,
			out,
			*jsonOutput,
			"version",
			nil,
			fmt.Sprintf("version does not accept positional arguments: %s", strings.Join(fs.Args(), " ")),
		)
	}

	info := buildinfo.Current()
	if *jsonOutput {
		return writeJSONCommandResult(errOut, out, "version", nil, nil, info)
	}

	fmt.Fprintln(out, "linkclaw version")
	fmt.Fprintf(out, "version: %s\n", info.Version)
	fmt.Fprintf(out, "commit: %s\n", info.Commit)
	fmt.Fprintf(out, "build time: %s\n", info.BuildTime)
	if info.Dirty {
		fmt.Fprintln(out, "dirty: true")
	}
	return 0
}

func runServe(ctx context.Context, args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(errOut)

	home := fs.String("home", "", "set LINKCLAW_HOME explicitly")
	dir := fs.String("dir", "", "directory to serve (defaults to <home>/publish)")
	addr := fs.String("addr", devserver.DefaultAddress, "listen address")

	if err := fs.Parse(args); err != nil {
		return 1
	}
	if len(fs.Args()) > 0 {
		fmt.Fprintf(errOut, "serve does not accept positional arguments: %s\n", strings.Join(fs.Args(), " "))
		return 1
	}

	root := strings.TrimSpace(*dir)
	if root == "" {
		resolvedHome, err := layout.ResolveHome(*home)
		if err != nil {
			fmt.Fprintf(errOut, "serve failed: %v\n", err)
			return 1
		}
		root = filepath.Join(resolvedHome, "publish")
	}

	server, err := devserver.Start(root, *addr)
	if err != nil {
		fmt.Fprintf(errOut, "serve failed: %v\n", err)
		return 1
	}

	serveCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Fprintln(out, "linkclaw serve running")
	fmt.Fprintf(out, "root: %s\n", server.Result.RootDir)
	fmt.Fprintf(out, "address: %s\n", server.Result.Address)
	fmt.Fprintf(out, "url: %s\n", server.Result.URL)
	fmt.Fprintf(out, "webfinger: %s/.well-known/webfinger\n", strings.TrimRight(server.Result.URL, "/"))

	select {
	case err := <-server.Done():
		if err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(errOut, "serve failed: %v\n", err)
			return 1
		}
		return 0
	case <-serveCtx.Done():
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(errOut, "serve shutdown failed: %v\n", err)
		return 1
	}

	if err := <-server.Done(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(errOut, "serve failed: %v\n", err)
		return 1
	}
	return 0
}

type publishDeployOptions struct {
	Target  string
	Project string
}

func resolvePublishDeploy(target, project string) (publishDeployOptions, error) {
	deployTarget := strings.TrimSpace(target)
	projectName := strings.TrimSpace(project)

	if deployTarget == "" {
		if projectName != "" {
			return publishDeployOptions{}, fmt.Errorf("publish --project requires --deploy cloudflare")
		}
		return publishDeployOptions{}, nil
	}
	if deployTarget != "cloudflare" {
		return publishDeployOptions{}, fmt.Errorf("unsupported publish deploy target %q", deployTarget)
	}
	if projectName == "" {
		return publishDeployOptions{}, fmt.Errorf("publish --deploy cloudflare requires --project")
	}

	return publishDeployOptions{
		Target:  deployTarget,
		Project: projectName,
	}, nil
}

func deployPublishBundle(ctx context.Context, errOut io.Writer, jsonOutput bool, result *publish.Result, deploy publishDeployOptions) error {
	if deploy.Target == "" {
		return nil
	}

	deployer := cloudflare.NewPagesDeployer()
	if !jsonOutput {
		fmt.Fprintf(errOut, "deploying publish bundle to Cloudflare Pages project %q\n", deploy.Project)
	}

	deployResult, err := deployer.Deploy(ctx, cloudflare.DeployOptions{
		Directory:   result.OutputDir,
		ProjectName: deploy.Project,
	})
	if err != nil {
		return fmt.Errorf("cloudflare deploy failed after bundle generation at %q: %w", result.OutputDir, err)
	}

	result.Deployment = &publish.DeploymentResult{
		Provider:  "cloudflare",
		Project:   deployResult.ProjectName,
		Directory: deployResult.Directory,
		Tool:      deployResult.Tool,
	}

	if text := strings.TrimSpace(deployResult.Stdout); text != "" && !jsonOutput {
		fmt.Fprintln(errOut, text)
	}
	if text := strings.TrimSpace(deployResult.Stderr); text != "" {
		fmt.Fprintln(errOut, text)
	}

	return nil
}
