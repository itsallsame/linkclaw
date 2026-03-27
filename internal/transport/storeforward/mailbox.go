package storeforward

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type MailboxSendRequest struct {
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

type MailboxSendResponse struct {
	RemoteMessageID string
}

type MailboxPullMessage struct {
	MessageID          string `json:"message_id"`
	RelayMessageID     string `json:"relay_message_id"`
	SenderID           string `json:"sender_id"`
	SenderPubKey       string `json:"sender_pubkey,omitempty"`
	SenderSigningKey   string `json:"sender_signing_key"`
	RecipientID        string `json:"recipient_id"`
	RecipientPubKey    string `json:"recipient_pubkey,omitempty"`
	EphemeralPublicKey string `json:"ephemeral_public_key"`
	Nonce              string `json:"nonce"`
	Ciphertext         string `json:"ciphertext"`
	Signature          string `json:"signature"`
	SentAt             string `json:"sent_at"`
}

type MailboxPullResponse struct {
	Messages   []MailboxPullMessage `json:"messages"`
	NextCursor string               `json:"next_cursor"`
}

type MailboxAckRequest struct {
	RecipientID string `json:"recipient_id"`
	Cursor      string `json:"cursor"`
}

type MailboxBackend interface {
	Send(ctx context.Context, routeLabel string, req MailboxSendRequest) (MailboxSendResponse, error)
	Pull(ctx context.Context, routeLabel string, recipientID string, cursor string) (MailboxPullResponse, error)
	Ack(ctx context.Context, routeLabel string, req MailboxAckRequest) error
}

type LegacyHTTPMailboxBackend struct {
	HTTPClient *http.Client
}

func (b LegacyHTTPMailboxBackend) httpClient() *http.Client {
	if b.HTTPClient == nil {
		return &http.Client{Timeout: 10 * time.Second}
	}
	return b.HTTPClient
}

func (b LegacyHTTPMailboxBackend) Send(ctx context.Context, routeLabel string, req MailboxSendRequest) (MailboxSendResponse, error) {
	endpoint, err := joinURL(routeLabel, "/messages")
	if err != nil {
		return MailboxSendResponse{}, err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return MailboxSendResponse{}, fmt.Errorf("encode store-forward send request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return MailboxSendResponse{}, fmt.Errorf("build store-forward send request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := b.httpClient().Do(httpReq)
	if err != nil {
		return MailboxSendResponse{}, fmt.Errorf("send store-forward request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return MailboxSendResponse{}, fmt.Errorf("store-forward send returned http %d", resp.StatusCode)
	}
	var parsed struct {
		RelayMessageID string `json:"relay_message_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return MailboxSendResponse{}, fmt.Errorf("decode store-forward send response: %w", err)
	}
	return MailboxSendResponse{RemoteMessageID: parsed.RelayMessageID}, nil
}

func (b LegacyHTTPMailboxBackend) Pull(ctx context.Context, routeLabel string, recipientID string, cursor string) (MailboxPullResponse, error) {
	endpoint, err := joinURL(routeLabel, "/messages")
	if err != nil {
		return MailboxPullResponse{}, err
	}
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return MailboxPullResponse{}, fmt.Errorf("parse store-forward pull url: %w", err)
	}
	query := parsedURL.Query()
	query.Set("recipient_id", recipientID)
	if strings.TrimSpace(cursor) != "" {
		query.Set("after", cursor)
	}
	parsedURL.RawQuery = query.Encode()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return MailboxPullResponse{}, fmt.Errorf("build store-forward pull request: %w", err)
	}
	resp, err := b.httpClient().Do(httpReq)
	if err != nil {
		return MailboxPullResponse{}, fmt.Errorf("send store-forward pull request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return MailboxPullResponse{}, fmt.Errorf("store-forward pull returned http %d", resp.StatusCode)
	}
	var parsed MailboxPullResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return MailboxPullResponse{}, fmt.Errorf("decode store-forward pull response: %w", err)
	}
	return MailboxPullResponse{
		Messages:   parsed.Messages,
		NextCursor: parsed.NextCursor,
	}, nil
}

func (b LegacyHTTPMailboxBackend) Ack(ctx context.Context, routeLabel string, req MailboxAckRequest) error {
	endpoint, err := joinURL(routeLabel, "/messages/ack")
	if err != nil {
		return err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encode store-forward ack request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build store-forward ack request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := b.httpClient().Do(httpReq)
	if err != nil {
		return fmt.Errorf("send store-forward ack request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("store-forward ack returned http %d", resp.StatusCode)
	}
	return nil
}

func joinURL(baseURL, p string) (string, error) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return "", fmt.Errorf("store-forward route label is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse store-forward route label: %w", err)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + p
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
