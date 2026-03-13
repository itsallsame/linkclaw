package devserver

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandlerServesManagedBundlePathsWithCorrectContentTypes(t *testing.T) {
	t.Parallel()

	root := seedBundle(t)
	handler, _, err := NewHandler(root)
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	testCases := []struct {
		path            string
		wantContentType string
		wantBody        string
	}{
		{
			path:            "/.well-known/webfinger",
			wantContentType: "application/json",
			wantBody:        `"subject": "https://agent.example/"`,
		},
		{
			path:            "/.well-known/did.json",
			wantContentType: "application/did+json",
			wantBody:        `"id": "did:web:agent.example"`,
		},
		{
			path:            "/profile/",
			wantContentType: "text/html; charset=utf-8",
			wantBody:        "<h1>Agent Example</h1>",
		},
		{
			path:            "/profile",
			wantContentType: "text/html; charset=utf-8",
			wantBody:        "<h1>Agent Example</h1>",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.path, func(t *testing.T) {
			t.Parallel()

			response, err := http.Get(server.URL + tc.path)
			if err != nil {
				t.Fatalf("GET %s: %v", tc.path, err)
			}
			defer response.Body.Close()

			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d", response.StatusCode)
			}
			if got := response.Header.Get("Content-Type"); got != tc.wantContentType {
				t.Fatalf("content-type = %q, want %q", got, tc.wantContentType)
			}
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if !strings.Contains(string(body), tc.wantBody) {
				t.Fatalf("body %q does not contain %q", string(body), tc.wantBody)
			}
		})
	}
}

func TestStartAndShutdown(t *testing.T) {
	t.Parallel()

	root := seedBundle(t)
	server, err := Start(root, "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	response, err := http.Get(server.Result.URL + "/.well-known/webfinger")
	if err != nil {
		t.Fatalf("GET webfinger: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
	if err := <-server.Done(); err != nil && err != http.ErrServerClosed {
		t.Fatalf("server done error: %v", err)
	}
}

func seedBundle(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".well-known", "webfinger"), `{"subject": "https://agent.example/"}`+"\n")
	writeTestFile(t, filepath.Join(root, ".well-known", "did.json"), `{"id": "did:web:agent.example"}`+"\n")
	writeTestFile(t, filepath.Join(root, "profile", "index.html"), "<h1>Agent Example</h1>\n")
	return root
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create dir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
