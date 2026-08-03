// Package mcpserver 实现绑定回环地址的嵌入式 HTTP MCP 服务，
// 把 integration 门面的研究上下文能力以 MCP 工具的形式暴露给外部工具（如 Wisp）
package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
	"github.com/xuzhougeng/citebox/internal/buildinfo"
	"github.com/xuzhougeng/citebox/internal/integration"
	"github.com/xuzhougeng/citebox/internal/model"
)

// protocolVersion 是服务器声明的 MCP 协议版本
const protocolVersion = "2025-06-18"

const (
	rpcCodeParseError     = -32700
	rpcCodeInvalidRequest = -32600
	rpcCodeMethodNotFound = -32601
	rpcCodeInvalidParams  = -32602
	rpcCodeInternal       = -32603
)

// tokenContextKey 把已认证的集成令牌挂到请求上下文
type tokenContextKey struct{}

// Server 是嵌入式 MCP 服务，只监听 127.0.0.1，默认关闭
type Server struct {
	facade   *integration.Facade
	tokens   *integration.TokenService
	assets   *integration.AssetStore
	settings *integration.Service
	logger   *slog.Logger

	mu         sync.Mutex
	httpServer *http.Server
	running    bool
	url        string
}

// New 创建 MCP 服务
func New(facade *integration.Facade, tokens *integration.TokenService, assets *integration.AssetStore, settings *integration.Service, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		facade:   facade,
		tokens:   tokens,
		assets:   assets,
		settings: settings,
		logger:   logger.With("component", "mcp_server"),
	}
}

// Start 按设置绑定 127.0.0.1:<port> 并启动服务；重复调用是安全的
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return nil
	}
	settings := s.settings.Get()
	// 只绑定回环地址，MCP 服务不暴露到局域网
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", settings.Port))
	if err != nil {
		return apperr.Wrap(apperr.CodeUnavailable, "MCP 服务监听失败", err)
	}
	httpServer := &http.Server{
		Handler:           s.routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	s.httpServer = httpServer
	s.running = true
	s.url = fmt.Sprintf("http://127.0.0.1:%d", settings.Port)
	go func() {
		if err := httpServer.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.logger.Error("MCP 服务异常停止", "error", err)
		}
	}()
	s.logger.Info("MCP 服务已启动", "url", s.url)
	return nil
}

// Stop 优雅停止服务
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || s.httpServer == nil {
		return nil
	}
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return apperr.Wrap(apperr.CodeInternal, "停止 MCP 服务失败", err)
	}
	s.httpServer = nil
	s.running = false
	s.url = ""
	s.logger.Info("MCP 服务已停止")
	return nil
}

// Restart 按当前设置重启服务：关闭时不启动，端口变化时重新绑定
func (s *Server) Restart() error {
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Stop(stopCtx); err != nil {
		return err
	}
	if !s.settings.Get().Enabled {
		return nil
	}
	return s.Start()
}

// Running 返回服务是否在运行
func (s *Server) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// URL 返回服务地址（未运行为空串）
func (s *Server) URL() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.url
}

// routes 构建独立的 ServeMux：不复用主 mux 的 Cookie 认证和 CORS 中间件
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.withAuth("", s.handleMCP))
	mux.HandleFunc("/assets/", s.withAuth(integration.ScopeAssetsRead, s.handleAsset))
	return mux
}

// withAuth 校验 Authorization: Bearer <token>；令牌只从请求头读取，绝不接受查询参数
func (s *Server) withAuth(requiredScope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plaintext := bearerToken(r)
		if plaintext == "" {
			writeUnauthorized(w)
			return
		}
		token, err := s.tokens.Authenticate(plaintext)
		if err != nil {
			writeUnauthorized(w)
			return
		}
		if requiredScope != "" && !integration.HasScope(token, requiredScope) {
			writeUnauthorized(w)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), tokenContextKey{}, token)))
	}
}

func bearerToken(r *http.Request) string {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
}

// ========== /mcp JSON-RPC ==========

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// 单请求模式：不支持批量请求和 SSE，统一返回 application/json
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		s.writeRPCError(w, nil, rpcCodeParseError, "Parse error")
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(body, &req); err != nil {
		s.writeRPCError(w, nil, rpcCodeParseError, "Parse error")
		return
	}
	// 通知（notifications/*）不需要响应体
	if strings.HasPrefix(req.Method, "notifications/") {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if req.JSONRPC != "2.0" {
		s.writeRPCError(w, req.ID, rpcCodeInvalidRequest, "Invalid Request")
		return
	}
	token, _ := r.Context().Value(tokenContextKey{}).(*model.IntegrationToken)
	result, callErr := s.dispatch(req, token)
	if callErr != nil {
		s.writeRPCError(w, req.ID, callErr.Code, callErr.Message)
		return
	}
	s.writeRPCResult(w, req.ID, result)
}

func (s *Server) dispatch(req rpcRequest, token *model.IntegrationToken) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "citebox", "version": buildinfo.CurrentVersion()},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolDefinitions()}, nil
	case "tools/call":
		return s.callTool(req.Params, token)
	default:
		return nil, &rpcError{Code: rpcCodeMethodNotFound, Message: "Method not found"}
	}
}

func (s *Server) writeRPCResult(w http.ResponseWriter, id json.RawMessage, result any) {
	s.writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: rpcIDOrNull(id), Result: result})
}

func (s *Server) writeRPCError(w http.ResponseWriter, id json.RawMessage, code int, message string) {
	s.writeRPC(w, rpcResponse{JSONRPC: "2.0", ID: rpcIDOrNull(id), Error: &rpcError{Code: code, Message: message}})
}

func (s *Server) writeRPC(w http.ResponseWriter, resp rpcResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func rpcIDOrNull(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return json.RawMessage("null")
	}
	return id
}

// ========== /assets/{id} ==========

func (s *Server) handleAsset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/assets/"), "/")
	if id == "" || strings.Contains(id, "/") {
		s.writeAssetError(w, http.StatusNotFound, "not found")
		return
	}
	data, mediaType, filename, ok := s.assets.Get(id)
	if !ok {
		s.writeAssetError(w, http.StatusNotFound, "not found")
		return
	}
	w.Header().Set("Content-Type", mediaType)
	if filename != "" {
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (s *Server) writeAssetError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
