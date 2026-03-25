package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) Publish(ctx context.Context, req PublishRequest) (AgentRecord, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return AgentRecord{}, fmt.Errorf("encode registry publish request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/api/agents/publish", bytes.NewReader(body))
	if err != nil {
		return AgentRecord{}, fmt.Errorf("build registry publish request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	var record AgentRecord
	if err := c.doJSON(httpReq, &record); err != nil {
		return AgentRecord{}, err
	}
	return record, nil
}

func (c *Client) Search(ctx context.Context, opts SearchOptions) (SearchResult, error) {
	values := url.Values{}
	if strings.TrimSpace(opts.Query) != "" {
		values.Set("q", strings.TrimSpace(opts.Query))
	}
	if strings.TrimSpace(opts.Capability) != "" {
		values.Set("capability", strings.TrimSpace(opts.Capability))
	}
	if strings.TrimSpace(opts.Tag) != "" {
		values.Set("tag", strings.TrimSpace(opts.Tag))
	}
	if opts.Limit > 0 {
		values.Set("limit", fmt.Sprintf("%d", opts.Limit))
	}
	target := c.BaseURL + "/api/agents/search"
	if encoded := values.Encode(); encoded != "" {
		target += "?" + encoded
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return SearchResult{}, fmt.Errorf("build registry search request: %w", err)
	}
	var result SearchResult
	if err := c.doJSON(httpReq, &result); err != nil {
		return SearchResult{}, err
	}
	return result, nil
}

func (c *Client) Show(ctx context.Context, agentID string) (AgentRecord, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/api/agents/"+url.PathEscape(strings.TrimSpace(agentID)), nil)
	if err != nil {
		return AgentRecord{}, fmt.Errorf("build registry show request: %w", err)
	}
	var record AgentRecord
	if err := c.doJSON(httpReq, &record); err != nil {
		return AgentRecord{}, err
	}
	return record, nil
}

func (c *Client) doJSON(req *http.Request, target any) error {
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send registry request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		trimmed := strings.TrimSpace(string(body))
		if trimmed == "" {
			return fmt.Errorf("registry returned http %d", resp.StatusCode)
		}
		return fmt.Errorf("registry returned http %d: %s", resp.StatusCode, trimmed)
	}
	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return fmt.Errorf("decode registry response: %w", err)
	}
	return nil
}
