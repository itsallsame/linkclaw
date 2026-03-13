package cloudflare

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type DeployOptions struct {
	Directory   string
	ProjectName string
}

type DeployResult struct {
	Directory   string
	ProjectName string
	Tool        string
	Stdout      string
	Stderr      string
}

type PagesDeployer struct {
	LookPath       func(string) (string, error)
	CommandContext func(context.Context, string, ...string) *exec.Cmd
}

func NewPagesDeployer() *PagesDeployer {
	return &PagesDeployer{
		LookPath:       exec.LookPath,
		CommandContext: exec.CommandContext,
	}
}

func (d *PagesDeployer) Deploy(ctx context.Context, opts DeployOptions) (DeployResult, error) {
	projectName := strings.TrimSpace(opts.ProjectName)
	if projectName == "" {
		return DeployResult{}, fmt.Errorf("cloudflare deploy requires --project")
	}

	directory, err := filepath.Abs(strings.TrimSpace(opts.Directory))
	if err != nil {
		return DeployResult{}, fmt.Errorf("resolve deploy directory: %w", err)
	}

	cmdName, cmdArgs, toolLabel, err := d.commandSpec()
	if err != nil {
		return DeployResult{}, err
	}
	args := append(cmdArgs, "pages", "deploy", directory, "--project-name", projectName)

	commandContext := d.CommandContext
	if commandContext == nil {
		commandContext = exec.CommandContext
	}

	cmd := commandContext(ctx, cmdName, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = strings.TrimSpace(stdout.String())
		}
		if message == "" {
			message = err.Error()
		}
		return DeployResult{}, fmt.Errorf("cloudflare deploy failed: %s", message)
	}

	return DeployResult{
		Directory:   directory,
		ProjectName: projectName,
		Tool:        toolLabel,
		Stdout:      stdout.String(),
		Stderr:      stderr.String(),
	}, nil
}

func (d *PagesDeployer) commandSpec() (string, []string, string, error) {
	lookPath := d.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	if wranglerPath, err := lookPath("wrangler"); err == nil {
		return wranglerPath, nil, "wrangler", nil
	}
	if npxPath, err := lookPath("npx"); err == nil {
		return npxPath, []string{"--yes", "wrangler@latest"}, "npx wrangler@latest", nil
	}

	return "", nil, "", fmt.Errorf("cloudflare deploy requires Wrangler; install wrangler or make npx available")
}
