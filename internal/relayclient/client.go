package relayclient

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

type Client struct {
	HTTPClient *http.Client
}

type SendRequest struct {
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

type SendResponse struct {
	RelayMessageID string `json:"relay_message_id"`
	Cursor         string `json:"cursor"`
	AcceptedAt     string `json:"accepted_at"`
}

type PullMessage struct {
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

type PullResponse struct {
	Messages   []PullMessage `json:"messages"`
	NextCursor string        `json:"next_cursor"`
}

type AckRequest struct {
	RecipientID string `json:"recipient_id"`
	Cursor      string `json:"cursor"`
}

func New() *Client {
	return &Client{HTTPClient: &http.Client{Timeout: 10 * time.Second}}
}

func (c *Client) Send(ctx context.Context, relayURL string, req SendRequest) (SendResponse, error) {
	endpoint, err := joinURL(relayURL, "/messages")
	if err != nil {
		return SendResponse{}, err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return SendResponse{}, fmt.Errorf("encode relay send request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return SendResponse{}, fmt.Errorf("build relay send request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return SendResponse{}, fmt.Errorf("send relay request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		return SendResponse{}, fmt.Errorf("relay send returned http %d", resp.StatusCode)
	}
	var parsed SendResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return SendResponse{}, fmt.Errorf("decode relay send response: %w", err)
	}
	return parsed, nil
}

func (c *Client) Pull(ctx context.Context, relayURL, recipientID, after string) (PullResponse, error) {
	endpoint, err := joinURL(relayURL, "/messages")
	if err != nil {
		return PullResponse{}, err
	}
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return PullResponse{}, fmt.Errorf("parse relay pull url: %w", err)
	}
	query := parsedURL.Query()
	query.Set("recipient_id", recipientID)
	if strings.TrimSpace(after) != "" {
		query.Set("after", after)
	}
	parsedURL.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return PullResponse{}, fmt.Errorf("build relay pull request: %w", err)
	}
	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return PullResponse{}, fmt.Errorf("send relay pull request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return PullResponse{}, fmt.Errorf("relay pull returned http %d", resp.StatusCode)
	}
	var parsed PullResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return PullResponse{}, fmt.Errorf("decode relay pull response: %w", err)
	}
	return parsed, nil
}

func (c *Client) Ack(ctx context.Context, relayURL string, req AckRequest) error {
	endpoint, err := joinURL(relayURL, "/messages/ack")
	if err != nil {
		return err
	}
	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("encode relay ack request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build relay ack request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return fmt.Errorf("send relay ack request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("relay ack returned http %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) httpClient() *http.Client {
	if c == nil || c.HTTPClient == nil {
		return &http.Client{Timeout: 10 * time.Second}
	}
	return c.HTTPClient
}

func joinURL(baseURL, p string) (string, error) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return "", fmt.Errorf("relay url is required")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("parse relay url: %w", err)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + p
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}
