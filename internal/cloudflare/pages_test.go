package cloudflare

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPagesDeployerUsesWranglerWhenAvailable(t *testing.T) {
	tempDir := t.TempDir()
	captureFile := filepath.Join(tempDir, "wrangler-args.txt")
	t.Setenv("CAPTURE_FILE", captureFile)

	wranglerPath := writeExecutable(t, filepath.Join(tempDir, "wrangler"))
	deployer := NewPagesDeployer()
	deployer.LookPath = func(name string) (string, error) {
		switch name {
		case "wrangler":
			return wranglerPath, nil
		default:
			return "", exec.ErrNotFound
		}
	}

	result, err := deployer.Deploy(context.Background(), DeployOptions{
		Directory:   tempDir,
		ProjectName: "agent-example",
	})
	if err != nil {
		t.Fatalf("Deploy returned error: %v", err)
	}
	if result.Tool != "wrangler" {
		t.Fatalf("tool = %q", result.Tool)
	}

	args := readCapturedLines(t, captureFile)
	wantDir, err := filepath.Abs(tempDir)
	if err != nil {
		t.Fatalf("abs tempDir: %v", err)
	}
	want := []string{"pages", "deploy", wantDir, "--project-name", "agent-example"}
	if strings.Join(args, "\n") != strings.Join(want, "\n") {
		t.Fatalf("args = %v, want %v", args, want)
	}
}

func TestPagesDeployerFallsBackToNpxWrangler(t *testing.T) {
	tempDir := t.TempDir()
	captureFile := filepath.Join(tempDir, "npx-args.txt")
	t.Setenv("CAPTURE_FILE", captureFile)

	npxPath := writeExecutable(t, filepath.Join(tempDir, "npx"))
	deployer := NewPagesDeployer()
	deployer.LookPath = func(name string) (string, error) {
		switch name {
		case "wrangler":
			return "", exec.ErrNotFound
		case "npx":
			return npxPath, nil
		default:
			return "", exec.ErrNotFound
		}
	}

	result, err := deployer.Deploy(context.Background(), DeployOptions{
		Directory:   tempDir,
		ProjectName: "agent-example",
	})
	if err != nil {
		t.Fatalf("Deploy returned error: %v", err)
	}
	if result.Tool != "npx wrangler@latest" {
		t.Fatalf("tool = %q", result.Tool)
	}

	args := readCapturedLines(t, captureFile)
	wantDir, err := filepath.Abs(tempDir)
	if err != nil {
		t.Fatalf("abs tempDir: %v", err)
	}
	want := []string{"--yes", "wrangler@latest", "pages", "deploy", wantDir, "--project-name", "agent-example"}
	if strings.Join(args, "\n") != strings.Join(want, "\n") {
		t.Fatalf("args = %v, want %v", args, want)
	}
}

func writeExecutable(t *testing.T, path string) string {
	t.Helper()

	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CAPTURE_FILE\"\necho deployed\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write executable: %v", err)
	}
	return path
}

func readCapturedLines(t *testing.T, path string) []string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read captured lines: %v", err)
	}
	return strings.Split(strings.TrimSpace(string(content)), "\n")
}
