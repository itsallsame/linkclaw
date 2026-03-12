package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
)

//go:embed sql/*.sql
var migrationFS embed.FS

type Step struct {
	Version string `json:"version"`
	Applied bool   `json:"applied"`
}

func Apply(ctx context.Context, db *sql.DB, now time.Time) ([]Step, error) {
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TEXT NOT NULL
		)
	`); err != nil {
		return nil, fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrationFS, "sql")
	if err != nil {
		return nil, fmt.Errorf("read migrations directory: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	steps := make([]Step, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version := strings.TrimSuffix(entry.Name(), ".sql")

		var exists int
		err := db.QueryRowContext(ctx, "SELECT 1 FROM schema_migrations WHERE version = ?", version).Scan(&exists)
		switch {
		case err == nil:
			steps = append(steps, Step{Version: version, Applied: false})
			continue
		case err != sql.ErrNoRows:
			return nil, fmt.Errorf("check migration %q: %w", version, err)
		}

		sqlBytes, err := migrationFS.ReadFile("sql/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("begin transaction for migration %q: %w", version, err)
		}

		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("apply migration %q: %w", version, err)
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)",
			version,
			now.UTC().Format(time.RFC3339Nano),
		); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("record migration %q: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit migration %q: %w", version, err)
		}

		steps = append(steps, Step{Version: version, Applied: true})
	}

	return steps, nil
}
