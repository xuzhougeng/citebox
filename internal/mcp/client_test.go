package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientStreamableHTTPJSONAndSSE(t *testing.T) {
	var sessionSeen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("Authorization = %q", got)
		}
		var request struct {
			ID     int64  `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		switch request.Method {
		case "initialize":
			w.Header().Set("Mcp-Session-Id", "session-1")
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"protocolVersion": protocolVersion}})
		case "notifications/initialized":
			if r.Header.Get("Mcp-Session-Id") == "session-1" {
				sessionSeen = true
			}
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(": keepalive\r\nevent: message\r\ndata: {\"jsonrpc\":\"2.0\",\"id\":91,\"method\":\"ping\"}\r\n\r\nevent: message\r\ndata: {\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"tools\":[{\"name\":\"search\"}]}}\r\n\r\nevent: keepalive\r\ndata:\r\n\r\n"))
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": map[string]any{"content": []map[string]any{{"type": "text", "text": "ok"}}}})
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "token", server.Client())
	if err := client.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	tools, err := client.ListTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "search" || !sessionSeen {
		t.Fatalf("tools=%v sessionSeen=%v", tools, sessionSeen)
	}
	result, err := client.CallTool(context.Background(), "search", map[string]any{"query": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Content) != 1 || result.Content[0].Text != "ok" {
		t.Fatalf("result = %#v", result)
	}
}

func TestSSEResponseJoinsDataLinesAndMatchesRequestID(t *testing.T) {
	body := []byte("event: message\ndata: {\"jsonrpc\":\"2.0\",\ndata: \"id\":7,\"result\":{}}\n\nevent: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":8,\"result\":{}}\n\n")
	response, err := sseResponseForRequest(body, 7)
	if err != nil {
		t.Fatal(err)
	}
	var envelope struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.ID != 7 {
		t.Fatalf("response id = %d", envelope.ID)
	}
}
