package indexer

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/resolver"

	_ "modernc.org/sqlite"
)

func TestServiceCrawlAndSearchPersistFreshBacklinks(t *testing.T) {
	t.Parallel()

	server := newFixtureServer(t, "consistent")
	t.Cleanup(server.Close)

	home := filepath.Join(t.TempDir(), "linkclaw-home")
	fixedNow := time.Date(2026, time.March, 13, 12, 0, 0, 0, time.UTC)

	service := NewService()
	service.Now = func() time.Time { return fixedNow }
	service.Resolver = resolver.NewService()
	service.Resolver.Client = server.Client()
	service.Resolver.Now = service.Now

	crawled, err := service.Crawl(context.Background(), CrawlOptions{
		Home:  home,
		Input: server.URL + "/profile/",
	})
	if err != nil {
		t.Fatalf("Crawl returned error: %v", err)
	}
	if crawled.Record.ResolverStatus != resolver.StatusConsistent {
		t.Fatalf("resolver status = %q", crawled.Record.ResolverStatus)
	}
	if crawled.Record.ConflictState != ConflictClear {
		t.Fatalf("conflict state = %q", crawled.Record.ConflictState)
	}
	if crawled.Record.SourceCount != 4 {
		t.Fatalf("source count = %d", crawled.Record.SourceCount)
	}
	if crawled.Record.Freshness != fixedNow.Format(time.RFC3339Nano) {
		t.Fatalf("freshness = %q", crawled.Record.Freshness)
	}

	searched, err := service.Search(context.Background(), SearchOptions{
		Home:  home,
		Query: "fixture",
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(searched.Records) != 1 {
		t.Fatalf("search record count = %d", len(searched.Records))
	}
	record := searched.Records[0]
	if record.RecordID != crawled.Record.RecordID {
		t.Fatalf("record id = %q, want %q", record.RecordID, crawled.Record.RecordID)
	}
	if record.SourceCount != 4 {
		t.Fatalf("search source count = %d", record.SourceCount)
	}
	if got := strings.Join(record.SourceURLs, ","); !strings.Contains(got, "/.well-known/did.json") || !strings.Contains(got, "/profile/") {
		t.Fatalf("search source urls missing expected backlinks: %s", got)
	}

	db := openDB(t, home)
	defer db.Close()
	assertCount(t, db, `SELECT COUNT(*) FROM index_records`, nil, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM index_sources`, nil, 4)
}

func TestServiceCrawlKeepsSeparateRecordsPerOrigin(t *testing.T) {
	t.Parallel()

	serverA := newFixtureServer(t, "consistent")
	t.Cleanup(serverA.Close)
	serverB := newFixtureServer(t, "consistent")
	t.Cleanup(serverB.Close)

	home := filepath.Join(t.TempDir(), "linkclaw-home")
	service := NewService()
	service.Now = func() time.Time { return time.Date(2026, time.March, 13, 12, 5, 0, 0, time.UTC) }

	service.Resolver = resolver.NewService()
	service.Resolver.Client = serverA.Client()
	service.Resolver.Now = service.Now
	first, err := service.Crawl(context.Background(), CrawlOptions{
		Home:  home,
		Input: serverA.URL + "/profile/",
	})
	if err != nil {
		t.Fatalf("first crawl returned error: %v", err)
	}

	service.Resolver = resolver.NewService()
	service.Resolver.Client = serverB.Client()
	service.Resolver.Now = service.Now
	second, err := service.Crawl(context.Background(), CrawlOptions{
		Home:  home,
		Input: serverB.URL + "/profile/",
	})
	if err != nil {
		t.Fatalf("second crawl returned error: %v", err)
	}
	if first.Record.RecordID == second.Record.RecordID {
		t.Fatalf("expected different record ids for separate origins")
	}
	if first.Record.NormalizedOrigin == second.Record.NormalizedOrigin {
		t.Fatalf("expected different origins for separate servers")
	}

	searched, err := service.Search(context.Background(), SearchOptions{
		Home:  home,
		Query: "did:web:fixture.example",
	})
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(searched.Records) != 2 {
		t.Fatalf("search record count = %d", len(searched.Records))
	}
}

func TestServiceRecrawlMarksConflictAndReplacesSources(t *testing.T) {
	t.Parallel()

	server, setRoot := newSwitchableFixtureServer(t, "consistent")
	t.Cleanup(server.Close)

	home := filepath.Join(t.TempDir(), "linkclaw-home")
	currentTime := time.Date(2026, time.March, 13, 12, 10, 0, 0, time.UTC)

	service := NewService()
	service.Now = func() time.Time { return currentTime }
	service.Resolver = resolver.NewService()
	service.Resolver.Client = server.Client()
	service.Resolver.Now = service.Now

	first, err := service.Crawl(context.Background(), CrawlOptions{
		Home:  home,
		Input: server.URL + "/profile/",
	})
	if err != nil {
		t.Fatalf("first crawl returned error: %v", err)
	}

	currentTime = currentTime.Add(5 * time.Minute)
	setRoot("mismatch-card")
	second, err := service.Crawl(context.Background(), CrawlOptions{
		Home:  home,
		Input: server.URL,
	})
	if err != nil {
		t.Fatalf("second crawl returned error: %v", err)
	}
	if second.Record.RecordID != first.Record.RecordID {
		t.Fatalf("record id = %q, want %q", second.Record.RecordID, first.Record.RecordID)
	}
	if second.Record.ConflictState != ConflictMarked {
		t.Fatalf("conflict state = %q", second.Record.ConflictState)
	}
	if len(second.Record.Mismatches) == 0 {
		t.Fatalf("expected mismatches after recrawl")
	}
	if second.Record.SourceCount != 2 {
		t.Fatalf("source count = %d, want 2", second.Record.SourceCount)
	}

	db := openDB(t, home)
	defer db.Close()
	assertCount(t, db, `SELECT COUNT(*) FROM index_records`, nil, 1)
	assertCount(t, db, `SELECT COUNT(*) FROM index_sources`, nil, 2)
}

func openDB(t *testing.T, home string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", filepath.Join(home, "state.db"))
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	return db
}

func assertCount(t *testing.T, db *sql.DB, query string, arg any, want int) {
	t.Helper()

	var count int
	var err error
	if arg == nil {
		err = db.QueryRow(query).Scan(&count)
	} else {
		err = db.QueryRow(query, arg).Scan(&count)
	}
	if err != nil {
		t.Fatalf("query count: %v", err)
	}
	if count != want {
		t.Fatalf("count = %d, want %d for query %q", count, want, query)
	}
}

func newFixtureServer(t *testing.T, fixture string) *httptest.Server {
	t.Helper()

	root := filepath.Join("..", "resolver", "testdata", fixture)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		filePath := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if strings.HasSuffix(r.URL.Path, "/") {
			filePath = filepath.Join(filePath, "index.html")
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		replaced := strings.ReplaceAll(string(content), "{{ORIGIN}}", indexServerOrigin(r))
		replaced = strings.ReplaceAll(replaced, "{{RESOURCE}}", indexServerOrigin(r)+"/")
		switch filepath.Ext(filePath) {
		case ".html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		default:
			w.Header().Set("Content-Type", "application/json")
		}
		_, _ = w.Write([]byte(replaced))
	}))
}

func newSwitchableFixtureServer(t *testing.T, initialFixture string) (*httptest.Server, func(string)) {
	t.Helper()

	var mu sync.RWMutex
	currentRoot := filepath.Join("..", "resolver", "testdata", initialFixture)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		root := currentRoot
		mu.RUnlock()

		filePath := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(r.URL.Path, "/")))
		if strings.HasSuffix(r.URL.Path, "/") {
			filePath = filepath.Join(filePath, "index.html")
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		replaced := strings.ReplaceAll(string(content), "{{ORIGIN}}", indexServerOrigin(r))
		replaced = strings.ReplaceAll(replaced, "{{RESOURCE}}", indexServerOrigin(r)+"/")
		switch filepath.Ext(filePath) {
		case ".html":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		default:
			w.Header().Set("Content-Type", "application/json")
		}
		_, _ = w.Write([]byte(replaced))
	}))

	return server, func(nextFixture string) {
		mu.Lock()
		currentRoot = filepath.Join("..", "resolver", "testdata", nextFixture)
		mu.Unlock()
	}
}

func indexServerOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}
