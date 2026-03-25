package registry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/card"
	"github.com/xiewanpeng/claw-identity/internal/initflow"
)

func TestServicePublishAndSearchAndServer(t *testing.T) {
	ctx := context.Background()
	home := filepath.Join(t.TempDir(), "home")
	if _, err := initflow.NewService().Init(ctx, initflow.Options{Home: home, DisplayName: "FSociety"}); err != nil {
		t.Fatalf("init: %v", err)
	}
	exported, err := card.NewService().Export(ctx, card.Options{Home: home})
	if err != nil {
		t.Fatalf("card export: %v", err)
	}

	service, err := Open(ctx, filepath.Join(t.TempDir(), "registry.db"))
	if err != nil {
		t.Fatalf("open registry: %v", err)
	}
	defer service.Close()
	service.now = func() time.Time { return time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC) }

	record, err := service.Publish(ctx, PublishRequest{
		IdentityCard: exported.Card,
		Summary:      "operator coordination agent",
		Capabilities: []string{"ops", "coordination"},
		Tags:         []string{"security", "ops"},
	})
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if record.AgentID == "" || record.CanonicalID != exported.Card.ID {
		t.Fatalf("unexpected publish record: %+v", record)
	}

	result, err := service.Search(ctx, SearchOptions{Query: "coordination"})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("search records = %d, want 1", len(result.Records))
	}

	server := httptest.NewServer(NewHandler(service))
	defer server.Close()
	client := NewClient(server.URL)

	published, err := client.Publish(ctx, PublishRequest{
		IdentityCard: exported.Card,
		Summary:      "operator coordination agent",
		Capabilities: []string{"ops", "coordination"},
		Tags:         []string{"security", "ops"},
	})
	if err != nil {
		t.Fatalf("client publish: %v", err)
	}
	if published.ProfileURL == "" || published.CardURL == "" {
		t.Fatalf("publish urls missing: %+v", published)
	}

	searchResult, err := client.Search(ctx, SearchOptions{Capability: "ops"})
	if err != nil {
		t.Fatalf("client search: %v", err)
	}
	if len(searchResult.Records) != 1 {
		t.Fatalf("client search records = %d, want 1", len(searchResult.Records))
	}

	shown, err := client.Show(ctx, published.AgentID)
	if err != nil {
		t.Fatalf("client show: %v", err)
	}
	if shown.IdentityCard.ID != exported.Card.ID {
		t.Fatalf("show card id = %q, want %q", shown.IdentityCard.ID, exported.Card.ID)
	}

	resp, err := http.Get(published.CardURL)
	if err != nil {
		t.Fatalf("get card url: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("card status = %d, want 200", resp.StatusCode)
	}
	var fetched card.Card
	if err := json.NewDecoder(resp.Body).Decode(&fetched); err != nil {
		t.Fatalf("decode card: %v", err)
	}
	if fetched.ID != exported.Card.ID {
		t.Fatalf("fetched card id = %q, want %q", fetched.ID, exported.Card.ID)
	}
}
