package migrate

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestApplyCreatesTrustDiscoveryFoundationTablesOnCleanDB(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openMigrationTestDB(t)
	defer db.Close()

	steps, err := Apply(ctx, db, time.Now().UTC())
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(steps) == 0 {
		t.Fatal("Apply() steps = 0, want migrations to run")
	}
	if !tableExists(t, db, "runtime_trust_records") {
		t.Fatal("runtime_trust_records table missing")
	}
	if !tableExists(t, db, "runtime_discovery_records") {
		t.Fatal("runtime_discovery_records table missing")
	}
	if !tableExists(t, db, "trust_events") {
		t.Fatal("trust_events table missing")
	}
	for _, table := range []string{
		"runtime_transport_bindings",
		"runtime_transport_relays",
		"runtime_relay_sync_state",
		"runtime_relay_delivery_attempts",
		"runtime_recovered_event_observations",
	} {
		if !tableExists(t, db, table) {
			t.Fatalf("%s table missing", table)
		}
	}
}

func TestApplyMigratesExistingDBToTrustDiscoveryFoundation(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openMigrationTestDB(t)
	defer db.Close()

	if _, err := Apply(ctx, db, time.Date(2026, 3, 23, 6, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Apply(initial) error = %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO runtime_contacts (contact_id, canonical_id, created_at, updated_at) VALUES (?, ?, ?, ?)`,
		"contact-existing",
		"did:key:z6MkExisting",
		"2026-03-23T06:00:00Z",
		"2026-03-23T06:00:00Z",
	); err != nil {
		t.Fatalf("insert runtime contact fixture: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE runtime_trust_records`); err != nil {
		t.Fatalf("drop runtime_trust_records: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE runtime_discovery_records`); err != nil {
		t.Fatalf("drop runtime_discovery_records: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DROP TABLE trust_events`); err != nil {
		t.Fatalf("drop trust_events: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = ?`, "0013_trust_discovery_store_foundation"); err != nil {
		t.Fatalf("delete 0013 migration marker: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = ?`, "0014_trust_events"); err != nil {
		t.Fatalf("delete 0014 migration marker: %v", err)
	}

	steps, err := Apply(ctx, db, time.Date(2026, 3, 23, 7, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Apply(re-run) error = %v", err)
	}
	found := false
	for _, step := range steps {
		if step.Version == "0013_trust_discovery_store_foundation" {
			found = true
			if !step.Applied {
				t.Fatalf("step %q Applied = false, want true", step.Version)
			}
		}
	}
	if !found {
		t.Fatal("0013_trust_discovery_store_foundation step not found")
	}
	if !tableExists(t, db, "runtime_trust_records") {
		t.Fatal("runtime_trust_records table missing after re-run")
	}
	if !tableExists(t, db, "runtime_discovery_records") {
		t.Fatal("runtime_discovery_records table missing after re-run")
	}
	if !tableExists(t, db, "trust_events") {
		t.Fatal("trust_events table missing after re-run")
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM runtime_contacts`).Scan(&count); err != nil {
		t.Fatalf("count runtime_contacts: %v", err)
	}
	if count != 1 {
		t.Fatalf("runtime_contacts count = %d, want 1", count)
	}
}

func TestApplyNormalizesLegacyDiscoverySourceEnums(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openMigrationTestDB(t)
	defer db.Close()

	initialNow := time.Date(2026, 3, 23, 8, 0, 0, 0, time.UTC)
	if _, err := Apply(ctx, db, initialNow); err != nil {
		t.Fatalf("Apply(initial) error = %v", err)
	}

	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO runtime_discovery_records (canonical_id, source, resolved_at, updated_at, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		"did:key:legacy-discovery",
		"stale-cache",
		"2026-03-23T07:00:00Z",
		"2026-03-23T07:00:00Z",
		"2026-03-23T07:00:00Z",
	); err != nil {
		t.Fatalf("insert legacy runtime_discovery_records row: %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO runtime_presence_cache (canonical_id, source, resolved_at)
		 VALUES (?, ?, ?)`,
		"did:key:legacy-presence",
		"refresh-peer",
		"2026-03-23T07:00:00Z",
	); err != nil {
		t.Fatalf("insert legacy runtime_presence_cache row: %v", err)
	}
	if _, err := db.ExecContext(
		ctx,
		`INSERT INTO runtime_presence_cache (canonical_id, source, resolved_at)
		 VALUES (?, ?, ?)`,
		"did:key:legacy-presence-unknown",
		"legacy-custom-source",
		"2026-03-23T07:00:00Z",
	); err != nil {
		t.Fatalf("insert unknown runtime_presence_cache row: %v", err)
	}

	if _, err := db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = ?`, "0015_discovery_source_enum_normalization"); err != nil {
		t.Fatalf("delete 0015 migration marker: %v", err)
	}

	steps, err := Apply(ctx, db, time.Date(2026, 3, 23, 8, 30, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Apply(re-run) error = %v", err)
	}

	found := false
	for _, step := range steps {
		if step.Version == "0015_discovery_source_enum_normalization" {
			found = true
			if !step.Applied {
				t.Fatalf("step %q Applied = false, want true", step.Version)
			}
		}
	}
	if !found {
		t.Fatal("0015_discovery_source_enum_normalization step not found")
	}

	var discoverySource string
	if err := db.QueryRowContext(
		ctx,
		`SELECT source FROM runtime_discovery_records WHERE canonical_id = ?`,
		"did:key:legacy-discovery",
	).Scan(&discoverySource); err != nil {
		t.Fatalf("query discovery source: %v", err)
	}
	if discoverySource != "cache" {
		t.Fatalf("runtime_discovery_records source = %q, want %q", discoverySource, "cache")
	}

	var presenceSource string
	if err := db.QueryRowContext(
		ctx,
		`SELECT source FROM runtime_presence_cache WHERE canonical_id = ?`,
		"did:key:legacy-presence",
	).Scan(&presenceSource); err != nil {
		t.Fatalf("query presence source: %v", err)
	}
	if presenceSource != "refresh" {
		t.Fatalf("runtime_presence_cache source = %q, want %q", presenceSource, "refresh")
	}

	var unknownPresenceSource string
	if err := db.QueryRowContext(
		ctx,
		`SELECT source FROM runtime_presence_cache WHERE canonical_id = ?`,
		"did:key:legacy-presence-unknown",
	).Scan(&unknownPresenceSource); err != nil {
		t.Fatalf("query unknown presence source: %v", err)
	}
	if unknownPresenceSource != "unknown" {
		t.Fatalf("runtime_presence_cache unknown source = %q, want %q", unknownPresenceSource, "unknown")
	}
}

func openMigrationTestDB(t *testing.T) *sql.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	return db
}

func tableExists(t *testing.T, db *sql.DB, tableName string) bool {
	t.Helper()

	var name string
	err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tableName).Scan(&name)
	if err != nil {
		return false
	}
	return name == tableName
}
