package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func NewHandler(service *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/agents/publish", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}
		var req PublishRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("decode publish request: %v", err))
			return
		}
		record, err := service.Publish(r.Context(), req)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		record = withURLs(r, record)
		writeJSON(w, http.StatusOK, record)
	})
	mux.HandleFunc("/api/agents/search", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w)
			return
		}
		limit := 20
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			fmt.Sscanf(raw, "%d", &limit)
		}
		result, err := service.Search(r.Context(), SearchOptions{
			Query:      r.URL.Query().Get("q"),
			Capability: r.URL.Query().Get("capability"),
			Tag:        r.URL.Query().Get("tag"),
			Limit:      limit,
		})
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for i := range result.Records {
			result.Records[i] = withURLs(r, result.Records[i])
		}
		writeJSON(w, http.StatusOK, result)
	})
	mux.HandleFunc("/api/agents/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
		path = strings.Trim(path, "/")
		if path == "" {
			writeJSONError(w, http.StatusNotFound, "agent id is required")
			return
		}
		parts := strings.Split(path, "/")
		record, ok, err := service.Get(r.Context(), parts[0])
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			writeJSONError(w, http.StatusNotFound, "agent not found")
			return
		}
		record = withURLs(r, record)
		if len(parts) == 2 && parts[1] == "card" {
			writeJSON(w, http.StatusOK, record.IdentityCard)
			return
		}
		if len(parts) > 1 {
			writeJSONError(w, http.StatusNotFound, "agent route not found")
			return
		}
		writeJSON(w, http.StatusOK, record)
	})
	return mux
}

func Serve(ctx context.Context, addr string, service *Service) error {
	server := &http.Server{Addr: addr, Handler: NewHandler(service)}
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()
	err := server.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func withURLs(r *http.Request, record AgentRecord) AgentRecord {
	base := "http://" + r.Host
	if r.TLS != nil {
		base = "https://" + r.Host
	}
	record.ProfileURL = base + "/api/agents/" + record.AgentID
	record.CardURL = base + "/api/agents/" + record.AgentID + "/card"
	return record
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]any{
		"ok":    false,
		"error": strings.TrimSpace(message),
	})
}
