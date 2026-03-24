package wolai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(status int, body any) *http.Response {
	data, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(data)),
	}
}

func TestGetBlockUsesAuthorizationHeader(t *testing.T) {
	client, err := NewClient(Config{
		Token:   "wolai-token",
		BaseURL: "https://mock.wolai.test",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	client.httpClient = &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/v1/blocks/parent-1" {
				t.Fatalf("path = %q, want %q", req.URL.Path, "/v1/blocks/parent-1")
			}
			if got := req.Header.Get("authorization"); got != "wolai-token" {
				t.Fatalf("authorization = %q, want %q", got, "wolai-token")
			}
			return jsonResponse(http.StatusOK, map[string]any{
				"data": map[string]any{
					"id":   "parent-1",
					"type": "page",
				},
			}), nil
		}),
	}

	block, err := client.GetBlock("parent-1")
	if err != nil {
		t.Fatalf("GetBlock() error = %v", err)
	}
	if got := block["id"]; got != "parent-1" {
		t.Fatalf("block.id = %v, want %q", got, "parent-1")
	}
}

func TestCreateBlocksPostsParentAndBlocks(t *testing.T) {
	client, err := NewClient(Config{
		Token:   "wolai-token",
		BaseURL: "https://mock.wolai.test",
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	client.httpClient = &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/v1/blocks" {
				t.Fatalf("path = %q, want %q", req.URL.Path, "/v1/blocks")
			}
			if got := req.Header.Get("authorization"); got != "wolai-token" {
				t.Fatalf("authorization = %q, want %q", got, "wolai-token")
			}

			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if got := payload["parent_id"]; got != "parent-1" {
				t.Fatalf("parent_id = %v, want %q", got, "parent-1")
			}
			blocks, ok := payload["blocks"].([]any)
			if !ok || len(blocks) != 1 {
				t.Fatalf("blocks = %#v, want len 1", payload["blocks"])
			}

			return jsonResponse(http.StatusOK, map[string]any{
				"data": map[string]any{
					"blocks": []map[string]any{
						{"id": "block-1"},
					},
				},
			}), nil
		}),
	}

	err = client.CreateBlocks("parent-1", []map[string]any{{
		"type":    "text",
		"content": "hello",
	}})
	if err != nil {
		t.Fatalf("CreateBlocks() error = %v", err)
	}
}
