package discovery

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/migrate"
	"github.com/xiewanpeng/claw-identity/internal/transport"

	_ "modernc.org/sqlite"
)

func TestStoreCRUD(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openStoreDB(t)
	defer db.Close()

	store := NewStoreWithDB(db, time.Date(2026, 3, 23, 6, 30, 0, 0, time.UTC))
	if err := store.Upsert(ctx, Record{
		CanonicalID: "did:key:z6MkBob",
		PeerID:      "lcpeer:bob",
		RouteCandidates: []transport.RouteCandidate{
			{
				Type:     transport.RouteTypeDirect,
				Label:    "direct-bob",
				Priority: 100,
				Target:   "libp2p://lcpeer:bob",
			},
			{
				Type:     transport.RouteTypeStoreForward,
				Label:    "sf-bob",
				Priority: 30,
				Target:   "sf://bob",
			},
		},
		TransportCapabilities: []string{"store_forward", "direct", "direct"},
		DirectHints:           []string{"libp2p://lcpeer:bob", "libp2p://lcpeer:bob", " "},
		StoreForwardHints:     []string{"sf://bob", "sf://bob-alt"},
		SignedPeerRecord:      `{"peer_id":"lcpeer:bob"}`,
		Source:                "dht",
		Reachable:             true,
		ResolvedAt:            "2026-03-23T06:20:00Z",
		FreshUntil:            "2026-03-23T06:40:00Z",
		AnnouncedAt:           "2026-03-23T06:21:00Z",
	}); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	record, ok, err := store.Get(ctx, "did:key:z6MkBob")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !ok {
		t.Fatal("Get() ok = false, want true")
	}
	if !record.Reachable {
		t.Fatal("record.Reachable = false, want true")
	}
	if len(record.RouteCandidates) != 2 {
		t.Fatalf("len(record.RouteCandidates) = %d, want 2", len(record.RouteCandidates))
	}
	if got, want := strings.Join(record.TransportCapabilities, ","), "direct,store_forward"; got != want {
		t.Fatalf("record.TransportCapabilities = %q, want %q", got, want)
	}
	if got, want := strings.Join(record.DirectHints, ","), "libp2p://lcpeer:bob"; got != want {
		t.Fatalf("record.DirectHints = %q, want %q", got, want)
	}

	if err := store.Upsert(ctx, Record{
		CanonicalID:           "did:key:z6MkBob",
		PeerID:                "lcpeer:bob-v2",
		RouteCandidates:       []transport.RouteCandidate{{Type: transport.RouteTypeDirect, Label: "direct-bob-v2", Priority: 90, Target: "libp2p://lcpeer:bob-v2"}},
		TransportCapabilities: []string{"direct"},
		DirectHints:           []string{"libp2p://lcpeer:bob-v2"},
		StoreForwardHints:     []string{"sf://bob"},
		SignedPeerRecord:      `{"peer_id":"lcpeer:bob-v2"}`,
		Source:                "libp2p",
		Reachable:             true,
		ResolvedAt:            "2026-03-23T06:35:00Z",
		FreshUntil:            "2026-03-23T06:55:00Z",
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
	if records[0].PeerID != "lcpeer:bob-v2" {
		t.Fatalf("records[0].PeerID = %q, want lcpeer:bob-v2", records[0].PeerID)
	}
	if records[0].Source != "libp2p" {
		t.Fatalf("records[0].Source = %q, want libp2p", records[0].Source)
	}

	deleted, err := store.Delete(ctx, "did:key:z6MkBob")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if !deleted {
		t.Fatal("Delete() deleted = false, want true")
	}
	_, ok, err = store.Get(ctx, "did:key:z6MkBob")
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
