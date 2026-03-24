package wolai

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"strings"
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
				"data": []string{
					"https://www.wolai.com/block-1",
				},
			}), nil
		}),
	}

	blocks, err := client.CreateBlocks("parent-1", []map[string]any{{
		"type":    "text",
		"content": "hello",
	}})
	if err != nil {
		t.Fatalf("CreateBlocks() error = %v", err)
	}
	if len(blocks) != 1 || blocks[0].ID != "block-1" {
		t.Fatalf("CreateBlocks() blocks = %#v, want created block-1", blocks)
	}
	if blocks[0].URL != "https://www.wolai.com/block-1" {
		t.Fatalf("CreateBlocks() url = %q, want Wolai page URL", blocks[0].URL)
	}
}

func TestCreateBlocksDecodesObjectPayload(t *testing.T) {
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
			return jsonResponse(http.StatusOK, map[string]any{
				"data": map[string]any{
					"blocks": []map[string]any{
						{"id": "block-2", "type": "text"},
					},
				},
			}), nil
		}),
	}

	blocks, err := client.CreateBlocks("parent-1", []map[string]any{{
		"type":    "text",
		"content": "hello",
	}})
	if err != nil {
		t.Fatalf("CreateBlocks() error = %v", err)
	}
	if len(blocks) != 1 || blocks[0].ID != "block-2" || blocks[0].Type != "text" {
		t.Fatalf("CreateBlocks() blocks = %#v, want decoded object payload", blocks)
	}
}

func TestCreateBlocksUsesFragmentAsBlockID(t *testing.T) {
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
			return jsonResponse(http.StatusOK, map[string]any{
				"data": []string{
					"https://www.wolai.com/page-1#image-block-1",
				},
			}), nil
		}),
	}

	blocks, err := client.CreateBlocks("page-1", []map[string]any{{
		"type": "image",
	}})
	if err != nil {
		t.Fatalf("CreateBlocks() error = %v", err)
	}
	if len(blocks) != 1 || blocks[0].ID != "image-block-1" {
		t.Fatalf("CreateBlocks() blocks = %#v, want fragment-derived image block id", blocks)
	}
}

func TestCreateUploadSessionUsesAPIBaseURL(t *testing.T) {
	client, err := NewClient(Config{
		Token:      "wolai-token",
		BaseURL:    "https://openapi.wolai.test",
		APIBaseURL: "https://api.wolai.test",
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	client.httpClient = &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://api.wolai.test/v1/file/getSignedPostUrl" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "https://api.wolai.test/v1/file/getSignedPostUrl")
			}
			if got := req.Header.Get("authorization"); got != "wolai-token" {
				t.Fatalf("authorization = %q, want %q", got, "wolai-token")
			}

			var payload UploadSessionRequest
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if payload.SpaceID != "space-1" || payload.BlockID != "block-1" || payload.Type != "image" || payload.FileName != "demo.png" || payload.OSSPath != "static" {
				t.Fatalf("payload = %#v", payload)
			}

			return jsonResponse(http.StatusOK, map[string]any{
				"data": map[string]any{
					"fileId":  "file-1",
					"fileUrl": "static/file-1/demo.png",
					"policyData": map[string]any{
						"url":    "https://upload.wolai.test",
						"bucket": "wolai-secure",
						"formData": map[string]string{
							"policy": "policy-value",
						},
					},
				},
			}), nil
		}),
	}

	session, err := client.CreateUploadSession(UploadSessionRequest{
		SpaceID:  "space-1",
		FileSize: 123,
		BlockID:  "block-1",
		Type:     "image",
		FileName: "demo.png",
		OSSPath:  "static",
	})
	if err != nil {
		t.Fatalf("CreateUploadSession() error = %v", err)
	}
	if session.FileID != "file-1" || session.FileURL != "static/file-1/demo.png" {
		t.Fatalf("session = %#v", session)
	}
	if session.PolicyData.URL != "https://upload.wolai.test" {
		t.Fatalf("policy url = %q", session.PolicyData.URL)
	}
}

func TestUploadFilePostsMultipartForm(t *testing.T) {
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
			if req.Method != http.MethodPost {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPost)
			}
			if req.URL.String() != "https://upload.wolai.test" {
				t.Fatalf("url = %q, want %q", req.URL.String(), "https://upload.wolai.test")
			}

			mediaType, _, err := mime.ParseMediaType(req.Header.Get("Content-Type"))
			if err != nil {
				t.Fatalf("ParseMediaType() error = %v", err)
			}
			if mediaType != "multipart/form-data" {
				t.Fatalf("mediaType = %q, want %q", mediaType, "multipart/form-data")
			}

			reader, err := req.MultipartReader()
			if err != nil {
				t.Fatalf("MultipartReader() error = %v", err)
			}

			fields := map[string]string{}
			var fileContent string
			var fileType string
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("NextPart() error = %v", err)
				}

				data, err := io.ReadAll(part)
				if err != nil {
					t.Fatalf("ReadAll() error = %v", err)
				}
				if part.FormName() == "file" {
					fileContent = string(data)
					fileType = part.Header.Get("Content-Type")
					continue
				}
				fields[part.FormName()] = string(data)
			}

			if fields["policy"] != "policy-value" {
				t.Fatalf("policy = %q, want %q", fields["policy"], "policy-value")
			}
			if fields["key"] != "static/file-1/demo.png" {
				t.Fatalf("key = %q, want %q", fields["key"], "static/file-1/demo.png")
			}
			if fields["success_action_status"] != "200" {
				t.Fatalf("success_action_status = %q, want %q", fields["success_action_status"], "200")
			}
			if fileType != "image/png" {
				t.Fatalf("file content-type = %q, want %q", fileType, "image/png")
			}
			if fileContent != "hello-image" {
				t.Fatalf("file content = %q, want %q", fileContent, "hello-image")
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("ok")),
			}, nil
		}),
	}

	err = client.UploadFile(UploadSession{
		FileURL: "static/file-1/demo.png",
		PolicyData: UploadPolicy{
			URL:    "https://upload.wolai.test",
			Bucket: "wolai-secure",
			FormData: map[string]string{
				"policy": "policy-value",
			},
		},
	}, "demo.png", "image/png", strings.NewReader("hello-image"))
	if err != nil {
		t.Fatalf("UploadFile() error = %v", err)
	}
}

func TestUpdateBlockFileSendsFileID(t *testing.T) {
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
			if req.Method != http.MethodPatch {
				t.Fatalf("method = %q, want %q", req.Method, http.MethodPatch)
			}
			if req.URL.Path != "/v1/blocks/block-1" {
				t.Fatalf("path = %q, want %q", req.URL.Path, "/v1/blocks/block-1")
			}

			var payload map[string]any
			if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
				t.Fatalf("Decode() error = %v", err)
			}
			if got := payload["file_id"]; got != "file-1" {
				t.Fatalf("file_id = %v, want %q", got, "file-1")
			}

			return jsonResponse(http.StatusOK, map[string]any{
				"data": map[string]any{"id": "block-1"},
			}), nil
		}),
	}

	if err := client.UpdateBlockFile("block-1", "file-1"); err != nil {
		t.Fatalf("UpdateBlockFile() error = %v", err)
	}
}
