package relayserver

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xiewanpeng/claw-identity/internal/ids"

	_ "modernc.org/sqlite"
)

type Server struct {
	db     *sql.DB
	server *http.Server
	done   chan error
}

type Result struct {
	DBPath  string `json:"db_path"`
	Address string `json:"address"`
	URL     string `json:"url"`
}

type sendRequest struct {
	MessageID          string `json:"message_id"`
	SenderID           string `json:"sender_id"`
	SenderSigningKey   string `json:"sender_signing_key"`
	RecipientID        string `json:"recipient_id"`
	EphemeralPublicKey string `json:"ephemeral_public_key"`
	Nonce              string `json:"nonce"`
	Ciphertext         string `json:"ciphertext"`
	Signature          string `json:"signature"`
	SentAt             string `json:"sent_at"`
}

type sendResponse struct {
	RelayMessageID string `json:"relay_message_id"`
	Cursor         string `json:"cursor"`
	AcceptedAt     string `json:"accepted_at"`
}

type relayMessage struct {
	RelayMessageID     string `json:"relay_message_id"`
	Cursor             string `json:"cursor"`
	MessageID          string `json:"message_id"`
	SenderID           string `json:"sender_id"`
	SenderSigningKey   string `json:"sender_signing_key"`
	RecipientID        string `json:"recipient_id"`
	EphemeralPublicKey string `json:"ephemeral_public_key"`
	Nonce              string `json:"nonce"`
	Ciphertext         string `json:"ciphertext"`
	Signature          string `json:"signature"`
	SentAt             string `json:"sent_at"`
}

type pullResponse struct {
	Messages   []relayMessage `json:"messages"`
	NextCursor string         `json:"next_cursor"`
}

type ackRequest struct {
	RecipientID string `json:"recipient_id"`
	Cursor      string `json:"cursor"`
}

func Start(dbPath, address string) (*Server, Result, error) {
	resolvedDB, db, err := openDB(dbPath)
	if err != nil {
		return nil, Result{}, err
	}
	if err := ensureSchema(db); err != nil {
		db.Close()
		return nil, Result{}, err
	}
	listenAddr := strings.TrimSpace(address)
	if listenAddr == "" {
		listenAddr = "127.0.0.1:0"
	}
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		db.Close()
		return nil, Result{}, fmt.Errorf("listen on %q: %w", listenAddr, err)
	}

	s := &Server{
		db:     db,
		server: &http.Server{Handler: handler{db: db}},
		done:   make(chan error, 1),
	}
	go func() {
		s.done <- s.server.Serve(listener)
	}()

	return s, Result{
		DBPath:  resolvedDB,
		Address: listener.Addr().String(),
		URL:     listenerURL(listener.Addr()),
	}, nil
}

func (s *Server) Done() <-chan error {
	return s.done
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if s.server != nil {
		if err := s.server.Shutdown(ctx); err != nil {
			return err
		}
	}
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}

type handler struct {
	db *sql.DB
}

func (h handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodPost && r.URL.Path == "/messages":
		h.handlePostMessage(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/messages":
		h.handleGetMessages(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/messages/ack":
		h.handleAck(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h handler) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	var req sendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	req.MessageID = strings.TrimSpace(req.MessageID)
	req.SenderID = strings.TrimSpace(req.SenderID)
	req.RecipientID = strings.TrimSpace(req.RecipientID)
	req.SenderSigningKey = strings.TrimSpace(req.SenderSigningKey)
	req.EphemeralPublicKey = strings.TrimSpace(req.EphemeralPublicKey)
	req.Nonce = strings.TrimSpace(req.Nonce)
	req.Ciphertext = strings.TrimSpace(req.Ciphertext)
	req.Signature = strings.TrimSpace(req.Signature)
	if req.MessageID == "" || req.SenderID == "" || req.RecipientID == "" || req.SenderSigningKey == "" || req.EphemeralPublicKey == "" || req.Nonce == "" || req.Ciphertext == "" || req.Signature == "" {
		http.Error(w, "message_id, sender_id, sender_signing_key, recipient_id, ephemeral_public_key, nonce, ciphertext, and signature are required", http.StatusBadRequest)
		return
	}
	relayMessageID, err := ids.New("relaymsg")
	if err != nil {
		http.Error(w, "generate relay message id", http.StatusInternalServerError)
		return
	}
	acceptedAt := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := h.db.Exec(
		`INSERT INTO relay_messages (
			relay_message_id, message_id, sender_id, sender_signing_key, recipient_id,
			ephemeral_public_key, nonce, ciphertext, signature, sent_at, accepted_at, acked_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '')`,
		relayMessageID,
		req.MessageID,
		req.SenderID,
		req.SenderSigningKey,
		req.RecipientID,
		req.EphemeralPublicKey,
		req.Nonce,
		req.Ciphertext,
		req.Signature,
		req.SentAt,
		acceptedAt,
	)
	if err != nil {
		http.Error(w, "store relay message", http.StatusInternalServerError)
		return
	}
	seq, err := result.LastInsertId()
	if err != nil {
		http.Error(w, "load relay cursor", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusAccepted, sendResponse{
		RelayMessageID: relayMessageID,
		Cursor:         strconv.FormatInt(seq, 10),
		AcceptedAt:     acceptedAt,
	})
}

func (h handler) handleGetMessages(w http.ResponseWriter, r *http.Request) {
	recipientID := strings.TrimSpace(r.URL.Query().Get("recipient_id"))
	if recipientID == "" {
		http.Error(w, "recipient_id is required", http.StatusBadRequest)
		return
	}
	after := strings.TrimSpace(r.URL.Query().Get("after"))
	afterSeq := int64(0)
	if after != "" {
		value, err := strconv.ParseInt(after, 10, 64)
		if err != nil {
			http.Error(w, "after must be numeric", http.StatusBadRequest)
			return
		}
		afterSeq = value
	}
	rows, err := h.db.Query(
		`SELECT seq, relay_message_id, message_id, sender_id, sender_signing_key, recipient_id,
		        ephemeral_public_key, nonce, ciphertext, signature, sent_at
		 FROM relay_messages
		 WHERE recipient_id = ? AND acked_at = '' AND seq > ?
		 ORDER BY seq ASC`,
		recipientID,
		afterSeq,
	)
	if err != nil {
		http.Error(w, "query relay messages", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var messages []relayMessage
	nextCursor := after
	for rows.Next() {
		var seq int64
		var msg relayMessage
		if err := rows.Scan(&seq, &msg.RelayMessageID, &msg.MessageID, &msg.SenderID, &msg.SenderSigningKey, &msg.RecipientID, &msg.EphemeralPublicKey, &msg.Nonce, &msg.Ciphertext, &msg.Signature, &msg.SentAt); err != nil {
			http.Error(w, "scan relay message", http.StatusInternalServerError)
			return
		}
		msg.Cursor = strconv.FormatInt(seq, 10)
		nextCursor = msg.Cursor
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "iterate relay messages", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, pullResponse{
		Messages:   messages,
		NextCursor: nextCursor,
	})
}

func (h handler) handleAck(w http.ResponseWriter, r *http.Request) {
	var req ackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	req.RecipientID = strings.TrimSpace(req.RecipientID)
	req.Cursor = strings.TrimSpace(req.Cursor)
	if req.RecipientID == "" || req.Cursor == "" {
		http.Error(w, "recipient_id and cursor are required", http.StatusBadRequest)
		return
	}
	cursor, err := strconv.ParseInt(req.Cursor, 10, 64)
	if err != nil {
		http.Error(w, "cursor must be numeric", http.StatusBadRequest)
		return
	}
	if _, err := h.db.Exec(
		`UPDATE relay_messages
		 SET acked_at = ?
		 WHERE recipient_id = ? AND acked_at = '' AND seq <= ?`,
		time.Now().UTC().Format(time.RFC3339Nano),
		req.RecipientID,
		cursor,
	); err != nil {
		http.Error(w, "ack relay messages", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func openDB(dbPath string) (string, *sql.DB, error) {
	abs, err := filepath.Abs(strings.TrimSpace(dbPath))
	if err != nil {
		return "", nil, fmt.Errorf("resolve relay db path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", nil, fmt.Errorf("create relay db directory: %w", err)
	}
	db, err := sql.Open("sqlite", abs)
	if err != nil {
		return "", nil, fmt.Errorf("open relay sqlite db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return "", nil, fmt.Errorf("ping relay sqlite db: %w", err)
	}
	return abs, db, nil
}

func ensureSchema(db *sql.DB) error {
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS relay_messages (
			seq INTEGER PRIMARY KEY AUTOINCREMENT,
			relay_message_id TEXT NOT NULL UNIQUE,
			message_id TEXT NOT NULL,
			sender_id TEXT NOT NULL,
			sender_signing_key TEXT NOT NULL,
			recipient_id TEXT NOT NULL,
			ephemeral_public_key TEXT NOT NULL,
			nonce TEXT NOT NULL,
			ciphertext TEXT NOT NULL,
			signature TEXT NOT NULL,
			sent_at TEXT NOT NULL DEFAULT '',
			accepted_at TEXT NOT NULL,
			acked_at TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_relay_messages_recipient_seq
		  ON relay_messages(recipient_id, seq);
	`); err != nil {
		return fmt.Errorf("ensure relay schema: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func listenerURL(addr net.Addr) string {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return "http://" + addr.String()
	}
	host := tcpAddr.IP.String()
	if host == "" || host == "::" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	return fmt.Sprintf("http://%s:%d", host, tcpAddr.Port)
}
