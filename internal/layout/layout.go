package layout

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const EnvHome = "LINKCLAW_HOME"

type Paths struct {
	Home     string
	DB       string
	KeysDir  string
	BlobsDir string
	CacheDir string
}

type EnsureResult struct {
	Paths   Paths
	Created []string
}

func ResolveHome(explicit string) (string, error) {
	raw := explicit
	if raw == "" {
		raw = os.Getenv(EnvHome)
	}
	if raw == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
		raw = filepath.Join(userHome, ".linkclaw")
	}
	if raw == "" {
		return "", errors.New("resolved home is empty")
	}

	abs, err := filepath.Abs(raw)
	if err != nil {
		return "", fmt.Errorf("resolve absolute home path: %w", err)
	}
	return abs, nil
}

func BuildPaths(home string) Paths {
	return Paths{
		Home:     home,
		DB:       filepath.Join(home, "state.db"),
		KeysDir:  filepath.Join(home, "keys"),
		BlobsDir: filepath.Join(home, "blobs"),
		CacheDir: filepath.Join(home, "cache"),
	}
}

func Ensure(home string) (EnsureResult, error) {
	paths := BuildPaths(home)
	dirs := []string{paths.Home, paths.KeysDir, paths.BlobsDir, paths.CacheDir}
	created := make([]string, 0, len(dirs))

	for _, dir := range dirs {
		fi, err := os.Stat(dir)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return EnsureResult{}, fmt.Errorf("create directory %q: %w", dir, err)
			}
			created = append(created, dir)
		case err != nil:
			return EnsureResult{}, fmt.Errorf("stat directory %q: %w", dir, err)
		case !fi.IsDir():
			return EnsureResult{}, fmt.Errorf("path %q exists but is not a directory", dir)
		}
	}

	return EnsureResult{Paths: paths, Created: created}, nil
}
