package trust

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/migrate"

	_ "modernc.org/sqlite"
)

func TestStoreCRUD(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openStoreDB(t)
	defer db.Close()

	store := NewStoreWithDB(db, time.Date(2026, 3, 23, 6, 0, 0, 0, time.UTC))
	if err := store.Upsert(ctx, Record{
		CanonicalID:       "did:key:z6MkAlice",
		ContactID:         "contact_1",
		TrustLevel:        "seen",
		RiskFlags:         []string{"manual", "fixture", "manual", " "},
		VerificationState: "discovered",
		DecisionReason:    "imported from fixtures",
		Source:            "import",
		DecidedAt:         "2026-03-23T05:59:00Z",
	}); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	record, ok, err := store.Get(ctx, "did:key:z6MkAlice")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if record.TrustLevel != "seen" {
		t.Fatalf("record.TrustLevel = %q, want seen", record.TrustLevel)
	}
	if got, want := strings.Join(record.RiskFlags, ","), "fixture,manual"; got != want {
		t.Fatalf("record.RiskFlags = %q, want %q", got, want)
	}

	if err := store.Upsert(ctx, Record{
		CanonicalID:       "did:key:z6MkAlice",
		ContactID:         "contact_1",
		TrustLevel:        "trusted",
		RiskFlags:         []string{"fixture"},
		VerificationState: "verified",
		DecisionReason:    "manual review",
		Source:            "known-trust",
		DecidedAt:         "2026-03-23T06:05:00Z",
	}); err != nil {
		t.Fatalf("Upsert(update) error = %v", err)
	}

	records, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len(records) = %d, want 1", len(records))
	}
	if records[0].TrustLevel != "trusted" {
		t.Fatalf("records[0].TrustLevel = %q, want trusted", records[0].TrustLevel)
	}
	if records[0].Source != "known-trust" {
		t.Fatalf("records[0].Source = %q, want known-trust", records[0].Source)
	}

	deleted, err := store.Delete(ctx, "did:key:z6MkAlice")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !deleted {
		t.Fatal("Delete() deleted = false, want true")
	}

	_, ok, err = store.Get(ctx, "did:key:z6MkAlice")
	if err != nil {
		t.Fatalf("Get(after delete) error = %v", err)
	}
	if ok {
		t.Fatal("Get(after delete) ok = true, want false")
	}
}

func TestStoreUpsertRequiresCanonicalID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openStoreDB(t)
	defer db.Close()

	store := NewStoreWithDB(db, time.Now().UTC())
	if err := store.Upsert(ctx, Record{}); err == nil {
		t.Fatal("Upsert() error = nil, want canonical_id validation error")
	}
}

func openStoreDB(t *testing.T) *sql.DB {
	t.Helper()

	path := filepath.Join(t.TempDir(), "state.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := migrate.Apply(context.Background(), db, time.Now().UTC()); err != nil {
		db.Close()
		t.Fatalf("migrate.Apply() error = %v", err)
	}
	return db
}
