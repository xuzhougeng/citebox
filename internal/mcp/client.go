package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

const protocolVersion = "2025-06-18"

type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type CallResult struct {
	Content           []Content      `json:"content"`
	StructuredContent map[string]any `json:"structuredContent,omitempty"`
	IsError           bool           `json:"isError,omitempty"`
}

type Client struct {
	url        string
	token      string
	httpClient *http.Client
	mu         sync.Mutex
	nextID     int64
	sessionID  string
}

func NewClient(url, token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Client{url: strings.TrimSpace(url), token: token, httpClient: httpClient, nextID: 1}
}

func (c *Client) Initialize(ctx context.Context) error {
	var result struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "CiteBox", "version": "0.1"},
	}, &result)
	if err != nil {
		return err
	}
	return c.notify(ctx, "notifications/initialized", map[string]any{})
}

func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	var result struct {
		Tools []Tool `json:"tools"`
	}
	if err := c.call(ctx, "tools/list", map[string]any{}, &result); err != nil {
		return nil, err
	}
	return result.Tools, nil
}

func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]any) (CallResult, error) {
	var result CallResult
	if err := c.call(ctx, "tools/call", map[string]any{"name": name, "arguments": arguments}, &result); err != nil {
		return CallResult{}, err
	}
	if result.IsError {
		var messages []string
		for _, content := range result.Content {
			if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
				messages = append(messages, strings.TrimSpace(content.Text))
			}
		}
		if len(messages) > 0 {
			return result, fmt.Errorf("MCP tool %q returned an error: %s", name, strings.Join(messages, "\n"))
		}
		return result, fmt.Errorf("MCP tool %q returned an error", name)
	}
	return result, nil
}

func (c *Client) call(ctx context.Context, method string, params any, result any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	id := c.nextID
	c.nextID++
	var response struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int64           `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error,omitempty"`
	}
	if err := c.send(ctx, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}, &response); err != nil {
		return err
	}
	if response.Error != nil {
		return fmt.Errorf("MCP %s failed (%d): %s", method, response.Error.Code, response.Error.Message)
	}
	if result != nil && len(response.Result) > 0 {
		if err := json.Unmarshal(response.Result, result); err != nil {
			return fmt.Errorf("decode MCP %s result: %w", method, err)
		}
	}
	return nil
}

func (c *Client) notify(ctx context.Context, method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.send(ctx, map[string]any{"jsonrpc": "2.0", "method": method, "params": params}, nil)
}

func (c *Client) send(ctx context.Context, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	if c.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", c.sessionID)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send MCP request: %w", err)
	}
	defer resp.Body.Close()
	if sid := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id")); sid != "" {
		c.sessionID = sid
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("read MCP response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("MCP server returned %s: %s", resp.Status, strings.TrimSpace(string(data)))
	}
	if target == nil || resp.StatusCode == http.StatusAccepted || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		data, err = sseResponseForRequest(data, requestID(payload))
		if err != nil {
			return fmt.Errorf("decode MCP SSE response: %w", err)
		}
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("decode MCP response: %w", err)
	}
	return nil
}

func requestID(payload any) int64 {
	message, ok := payload.(map[string]any)
	if !ok {
		return 0
	}
	id, _ := message["id"].(int64)
	return id
}

func sseResponseForRequest(body []byte, expectedID int64) ([]byte, error) {
	events, err := sseDataEvents(body)
	if err != nil {
		return nil, err
	}
	var lastMessage []byte
	for _, data := range events {
		data = bytes.TrimSpace(data)
		if len(data) == 0 {
			continue
		}
		var envelope struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal(data, &envelope); err != nil {
			continue
		}
		lastMessage = data
		var id int64
		if expectedID > 0 && json.Unmarshal(envelope.ID, &id) == nil && id == expectedID {
			return data, nil
		}
	}
	if expectedID == 0 && len(lastMessage) > 0 {
		return lastMessage, nil
	}
	return nil, fmt.Errorf("SSE stream did not contain the JSON-RPC response for request %d", expectedID)
}

func sseDataEvents(body []byte) ([][]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 64*1024), 8<<20)
	var events [][]byte
	var dataLines [][]byte
	flush := func() {
		if len(dataLines) == 0 {
			return
		}
		events = append(events, bytes.Join(dataLines, []byte("\n")))
		dataLines = nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			value := strings.TrimPrefix(line, "data:")
			value = strings.TrimPrefix(value, " ")
			dataLines = append(dataLines, []byte(value))
		}
	}
	flush()
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}
