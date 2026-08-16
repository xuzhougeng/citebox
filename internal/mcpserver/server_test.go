package mcpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/integration"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
)

type testRig struct {
	server   *Server
	tokens   *integration.TokenService
	settings *integration.Service
	repo     *repository.LibraryRepository
	cfg      *config.Config
	token    string
	baseURL  string
}

func newTestRig(t *testing.T) *testRig {
	t.Helper()

	root := t.TempDir()
	cfg := &config.Config{
		StorageDir:    filepath.Join(root, "storage"),
		DatabasePath:  filepath.Join(root, "library.db"),
		MaxUploadSize: 10 << 20,
	}
	repo, err := repository.NewLibraryRepository(cfg.DatabasePath)
	if err != nil {
		t.Fatalf("NewLibraryRepository() error = %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := service.NewLibraryService(repo, cfg, service.WithLogger(logger), service.WithoutBackgroundJobs())
	if err != nil {
		t.Fatalf("NewLibraryService() error = %v", err)
	}

	settings := integration.NewService(repo.Setting)
	tokens := integration.NewTokenService(repo.Integration)
	assets := integration.NewAssetStore()
	facade := integration.NewFacade(svc, repo, assets, settings)
	server := New(facade, tokens, assets, settings, logger)

	// 选一个空闲端口并启用集成
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve free port error = %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("close probe listener error = %v", err)
	}
	if err := settings.Update(true, port); err != nil {
		t.Fatalf("settings.Update() error = %v", err)
	}

	plaintext, _, err := tokens.Rotate()
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}

	if err := server.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Stop(ctx)
	})

	if !server.Running() {
		t.Fatal("Running() = false after Start")
	}
	if server.URL() != fmt.Sprintf("http://127.0.0.1:%d", port) {
		t.Fatalf("URL() = %q", server.URL())
	}

	return &testRig{
		server:   server,
		tokens:   tokens,
		settings: settings,
		repo:     repo,
		cfg:      cfg,
		token:    plaintext,
		baseURL:  fmt.Sprintf("http://127.0.0.1:%d", port),
	}
}

func (rig *testRig) rpc(t *testing.T, token, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, rig.baseURL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp error = %v", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body error = %v", err)
	}
	var decoded map[string]any
	if len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &decoded); err != nil {
			t.Fatalf("response not JSON: %s", raw)
		}
	}
	return resp.StatusCode, decoded
}

func rpcErrorCode(t *testing.T, resp map[string]any) float64 {
	t.Helper()
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("response has no error object: %v", resp)
	}
	code, ok := errObj["code"].(float64)
	if !ok {
		t.Fatalf("error.code missing: %v", errObj)
	}
	return code
}

func TestMCPInitializeAndToolsList(t *testing.T) {
	rig := newTestRig(t)

	status, resp := rig.rpc(t, rig.token, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`)
	if status != http.StatusOK {
		t.Fatalf("initialize status = %d, want 200", status)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("initialize result = %v", resp)
	}
	if result["protocolVersion"] != "2025-06-18" {
		t.Fatalf("protocolVersion = %v", result["protocolVersion"])
	}
	serverInfo, ok := result["serverInfo"].(map[string]any)
	if !ok || serverInfo["name"] != "citebox" {
		t.Fatalf("serverInfo = %v", result["serverInfo"])
	}
	if _, ok := result["capabilities"].(map[string]any)["tools"]; !ok {
		t.Fatalf("capabilities = %v, want tools key", result["capabilities"])
	}

	status, resp = rig.rpc(t, rig.token, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if status != http.StatusOK {
		t.Fatalf("tools/list status = %d", status)
	}
	tools, ok := resp["result"].(map[string]any)["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list result = %v", resp)
	}
	if len(tools) != 8 {
		t.Fatalf("tools count = %d, want 8", len(tools))
	}
	names := map[string]bool{}
	for _, tool := range tools {
		entry, ok := tool.(map[string]any)
		if !ok {
			t.Fatalf("tool entry = %v", tool)
		}
		name, _ := entry["name"].(string)
		names[name] = true
		if _, ok := entry["inputSchema"].(map[string]any); !ok {
			t.Fatalf("tool %s missing inputSchema", name)
		}
		if desc, _ := entry["description"].(string); desc == "" {
			t.Fatalf("tool %s missing description", name)
		}
	}
	for _, want := range integration.ToolNames() {
		if !names[want] {
			t.Fatalf("tool %s missing from tools/list", want)
		}
	}

	// ping
	status, resp = rig.rpc(t, rig.token, `{"jsonrpc":"2.0","id":3,"method":"ping"}`)
	if status != http.StatusOK {
		t.Fatalf("ping status = %d", status)
	}
	if _, ok := resp["result"]; !ok {
		t.Fatalf("ping response = %v", resp)
	}

	// notifications/* → 202 空响应
	status, resp = rig.rpc(t, rig.token, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	if status != http.StatusAccepted {
		t.Fatalf("notification status = %d, want 202", status)
	}
	if resp != nil {
		t.Fatalf("notification body = %v, want empty", resp)
	}
}

func TestMCPCallGetCapabilities(t *testing.T) {
	rig := newTestRig(t)

	status, resp := rig.rpc(t, rig.token, `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"citebox_get_capabilities","arguments":{}}}`)
	if status != http.StatusOK {
		t.Fatalf("tools/call status = %d", status)
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/call result = %v", resp)
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("structuredContent missing: %v", result)
	}
	if structured["research_context_schema"] != integration.ResearchContextSchema {
		t.Fatalf("structuredContent = %v", structured)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("content = %v", result["content"])
	}
	textEntry, ok := content[0].(map[string]any)
	if !ok || textEntry["type"] != "text" {
		t.Fatalf("content[0] = %v", content[0])
	}
	text, _ := textEntry["text"].(string)
	if !strings.Contains(text, "research_context_schema") {
		t.Fatalf("content text = %q", text)
	}
}

func TestMCPUnauthorized(t *testing.T) {
	rig := newTestRig(t)

	// 缺少令牌
	status, resp := rig.rpc(t, "", `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("missing token status = %d, want 401", status)
	}
	if resp["error"] != "unauthorized" {
		t.Fatalf("missing token body = %v", resp)
	}

	// 错误令牌
	status, _ = rig.rpc(t, "cbx_wrong", `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("bad token status = %d, want 401", status)
	}

	// 查询参数传令牌不接受
	req, err := http.NewRequest(http.MethodPost, rig.baseURL+"/mcp?token="+rig.token, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	respHTTP, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /mcp?token error = %v", err)
	}
	respHTTP.Body.Close()
	if respHTTP.StatusCode != http.StatusUnauthorized {
		t.Fatalf("query token status = %d, want 401", respHTTP.StatusCode)
	}

	// 已吊销的令牌
	if err := rig.tokens.Revoke(); err != nil {
		t.Fatalf("Revoke() error = %v", err)
	}
	status, _ = rig.rpc(t, rig.token, `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	if status != http.StatusUnauthorized {
		t.Fatalf("revoked token status = %d, want 401", status)
	}
}

func TestMCPUnknownMethodToolAndBadParams(t *testing.T) {
	rig := newTestRig(t)

	// 未知方法 → -32601
	_, resp := rig.rpc(t, rig.token, `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`)
	if got := rpcErrorCode(t, resp); got != -32601 {
		t.Fatalf("unknown method code = %v, want -32601", got)
	}

	// 未知工具 → -32601
	_, resp = rig.rpc(t, rig.token, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"nope","arguments":{}}}`)
	if got := rpcErrorCode(t, resp); got != -32601 {
		t.Fatalf("unknown tool code = %v, want -32601", got)
	}

	// 参数缺失 → -32602
	_, resp = rig.rpc(t, rig.token, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"citebox_get_paper_context","arguments":{}}}`)
	if got := rpcErrorCode(t, resp); got != -32602 {
		t.Fatalf("missing paper_id code = %v, want -32602", got)
	}

	_, resp = rig.rpc(t, rig.token, `{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"citebox_get_figure_handoff","arguments":{}}}`)
	if got := rpcErrorCode(t, resp); got != -32602 {
		t.Fatalf("missing figure_id code = %v, want -32602", got)
	}

	// 参数类型错误 → -32602
	_, resp = rig.rpc(t, rig.token, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"citebox_get_paper_context","arguments":{"paper_id":"abc"}}}`)
	if got := rpcErrorCode(t, resp); got != -32602 {
		t.Fatalf("bad paper_id type code = %v, want -32602", got)
	}

	// JSON 解析失败 → -32700，id 为 null
	_, resp = rig.rpc(t, rig.token, `{not json`)
	if got := rpcErrorCode(t, resp); got != -32700 {
		t.Fatalf("parse error code = %v, want -32700", got)
	}
	if id, present := resp["id"]; !present || id != nil {
		t.Fatalf("parse error id = %v, want null", resp["id"])
	}

	// 域错误（实体不存在）→ isError 工具结果
	_, resp = rig.rpc(t, rig.token, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"citebox_get_entity","arguments":{"source_id":"citebox:paper:99999"}}}`)
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("domain error response = %v", resp)
	}
	if result["isError"] != true {
		t.Fatalf("domain error isError = %v, want true", result)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("domain error content = %v", result["content"])
	}
}

func TestMCPScopeEnforcement(t *testing.T) {
	rig := newTestRig(t)

	// 只给 library:read 的令牌
	limitedPlaintext := "cbx_limitedscopetoken0123456789abcdef"
	if _, err := rig.repo.Integration.Create("limited", integration.HashToken(limitedPlaintext), []string{integration.ScopeLibraryRead}); err != nil {
		t.Fatalf("Create(limited) error = %v", err)
	}

	// library:read 工具可用
	status, resp := rig.rpc(t, limitedPlaintext, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"citebox_search_library","arguments":{"limit":5}}}`)
	if status != http.StatusOK {
		t.Fatalf("search_library status = %d", status)
	}
	if _, ok := resp["result"].(map[string]any); !ok {
		t.Fatalf("search_library result = %v", resp)
	}

	// export_asset 需要 assets:read → -32600
	_, resp = rig.rpc(t, limitedPlaintext, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"citebox_export_asset","arguments":{"kind":"figure_image","id":1}}}`)
	if got := rpcErrorCode(t, resp); got != -32600 {
		t.Fatalf("export_asset with limited scope code = %v, want -32600", got)
	}

	// assets 路由需要 assets:read → 401
	req, err := http.NewRequest(http.MethodGet, rig.baseURL+"/assets/whatever", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+limitedPlaintext)
	respHTTP, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /assets error = %v", err)
	}
	respHTTP.Body.Close()
	if respHTTP.StatusCode != http.StatusUnauthorized {
		t.Fatalf("assets with limited scope status = %d, want 401", respHTTP.StatusCode)
	}
}

func TestMCPExportAssetEndToEnd(t *testing.T) {
	rig := newTestRig(t)

	// 造一篇带真实图片的文献
	imageData := testMCPPNG(t)
	paper, err := rig.repo.CreatePaper(repository.PaperUpsertInput{
		Title:            "MCP Asset Paper",
		OriginalFilename: "mcp-asset.pdf",
		StoredPDFName:    "mcp-asset.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []repository.FigureUpsertInput{
			{Filename: "mcp_fig.png", OriginalName: "mcp-fig.png", ContentType: "image/png", PageNumber: 1, FigureIndex: 1, Caption: "MCP figure"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	figure := paper.Figures[0]
	if err := os.MkdirAll(rig.cfg.FiguresDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(rig.cfg.FiguresDir(), figure.Filename), imageData, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// 调 export_asset 拿到下载描述
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"citebox_export_asset","arguments":{"kind":"figure_image","id":%d}}}`, figure.ID)
	_, resp := rig.rpc(t, rig.token, body)
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("export_asset result = %v", resp)
	}
	structured, ok := result["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("structuredContent = %v", result)
	}
	url, _ := structured["url"].(string)
	if !strings.HasPrefix(url, rig.baseURL+"/assets/") {
		t.Fatalf("asset url = %q, want prefix %s/assets/", url, rig.baseURL)
	}

	// 带令牌下载：字节和 sha256 必须一致
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+rig.token)
	downloadResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET asset error = %v", err)
	}
	defer downloadResp.Body.Close()
	if downloadResp.StatusCode != http.StatusOK {
		t.Fatalf("GET asset status = %d, want 200", downloadResp.StatusCode)
	}
	if got := downloadResp.Header.Get("Content-Type"); got != "image/png" {
		t.Fatalf("asset content type = %q, want image/png", got)
	}
	downloaded, err := io.ReadAll(downloadResp.Body)
	if err != nil {
		t.Fatalf("read asset error = %v", err)
	}
	if !bytes.Equal(downloaded, imageData) {
		t.Fatalf("downloaded %d bytes, want %d identical bytes", len(downloaded), len(imageData))
	}
	sum := sha256.Sum256(downloaded)
	if structured["sha256"] != hex.EncodeToString(sum[:]) {
		t.Fatalf("sha256 = %v, want %q", structured["sha256"], hex.EncodeToString(sum[:]))
	}
	if int(structured["byte_size"].(float64)) != len(imageData) {
		t.Fatalf("byte_size = %v, want %d", structured["byte_size"], len(imageData))
	}

	// 无令牌下载 → 401
	plainResp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET asset without token error = %v", err)
	}
	plainResp.Body.Close()
	if plainResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET asset without token status = %d, want 401", plainResp.StatusCode)
	}
}

func TestMCPServerLifecycle(t *testing.T) {
	rig := newTestRig(t)

	// 重复 Start 是安全的
	if err := rig.server.Start(); err != nil {
		t.Fatalf("second Start() error = %v", err)
	}

	// 关闭集成后 Restart 会停止服务
	if err := rig.settings.Update(false, 19831); err != nil {
		t.Fatalf("Update(false) error = %v", err)
	}
	if err := rig.server.Restart(); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	if rig.server.Running() {
		t.Fatal("Running() = true after disabling integration, want false")
	}
	if rig.server.URL() != "" {
		t.Fatalf("URL() = %q after stop, want empty", rig.server.URL())
	}

	// 重新启用后 Restart 重新绑定
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port error = %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	if err := rig.settings.Update(true, port); err != nil {
		t.Fatalf("Update(true) error = %v", err)
	}
	if err := rig.server.Restart(); err != nil {
		t.Fatalf("Restart() error = %v", err)
	}
	if !rig.server.Running() {
		t.Fatal("Running() = false after re-enable, want true")
	}
	rig.baseURL = fmt.Sprintf("http://127.0.0.1:%d", port)
	status, _ := rig.rpc(t, rig.token, `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	if status != http.StatusOK {
		t.Fatalf("ping after restart status = %d, want 200", status)
	}
}

func testMCPPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(50 + x), G: uint8(60 + y), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("png.Encode() error = %v", err)
	}
	return buf.Bytes()
}
