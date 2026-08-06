package codexapp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	maxMessageBytes     = 16 * 1024 * 1024
	mcpDiscoveryTimeout = 30 * time.Second
)

var ErrMCPDiscoveryTimeout = errors.New("Codex MCP discovery timed out")

type Config struct {
	Enabled bool
	Binary  string
}

type Status struct {
	DesktopAvailable bool   `json:"desktop_available"`
	CLIAvailable     bool   `json:"cli_available"`
	Authenticated    bool   `json:"authenticated"`
	Version          string `json:"version,omitempty"`
	Binary           string `json:"binary,omitempty"`
	Message          string `json:"message,omitempty"`
}

type Model struct {
	ID                       string   `json:"id"`
	DisplayName              string   `json:"display_name"`
	DefaultReasoningEffort   string   `json:"default_reasoning_effort,omitempty"`
	SupportedReasoningEffort []string `json:"supported_reasoning_efforts,omitempty"`
	InputModalities          []string `json:"input_modalities,omitempty"`
	IsDefault                bool     `json:"is_default,omitempty"`
}

type Image struct {
	MIMEType string
	Data     string
}

type Request struct {
	Model           string
	ReasoningEffort string
	SystemPrompt    string
	UserPrompt      string
	Images          []Image
}

type Client struct {
	config Config

	opMu     sync.Mutex
	sendMu   sync.Mutex
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	scanner  *bufio.Scanner
	stderr   bytes.Buffer
	nextID   int64
	workDir  string
	binary   string
	mcpNames []string
}

func New(config Config) *Client {
	return &Client{config: config, nextID: 1}
}

func (c *Client) Status(ctx context.Context) Status {
	status := Status{DesktopAvailable: c != nil && c.config.Enabled}
	if !status.DesktopAvailable {
		status.Message = "Codex 订阅仅在 CiteBox 桌面端可用"
		return status
	}
	binary, err := c.resolveBinary()
	if err != nil {
		status.Message = "未找到 Codex CLI；请先安装 Codex，并确认桌面应用可以访问其可执行文件"
		return status
	}
	status.CLIAvailable = true
	status.Binary = binary

	if ctx == nil {
		ctx = context.Background()
	}
	versionCtx, cancelVersion := context.WithTimeout(ctx, 5*time.Second)
	versionCmd := exec.CommandContext(versionCtx, binary, "--version")
	versionCmd.Env = safeEnvironment()
	versionOutput, versionErr := versionCmd.CombinedOutput()
	cancelVersion()
	if versionErr == nil {
		status.Version = strings.TrimSpace(string(versionOutput))
	}

	message, loginErr := c.chatGPTLoginStatus(ctx, binary)
	if loginErr == nil {
		status.Authenticated = true
		status.Message = firstNonEmpty(message, "Codex 已通过 ChatGPT 登录")
		return status
	}
	status.Message = firstNonEmpty(message, "Codex 尚未通过 ChatGPT 登录；请在终端运行 codex login")
	return status
}

func (c *Client) Models(ctx context.Context) ([]Model, error) {
	if c == nil || !c.config.Enabled {
		return nil, errors.New("Codex subscription runtime is only available in the desktop app")
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureStarted(); err != nil {
		return nil, err
	}

	id := c.newID()
	if err := c.send(map[string]any{"method": "model/list", "id": id, "params": map[string]any{"limit": 100, "includeHidden": false}}); err != nil {
		c.stopProcess()
		return nil, err
	}
	message, err := c.readResponse(id)
	if err != nil {
		c.stopProcess()
		return nil, err
	}
	if rpcErr := responseError(message); rpcErr != nil {
		return nil, rpcErr
	}

	var payload struct {
		Result struct {
			Data []struct {
				ID                       string `json:"id"`
				Model                    string `json:"model"`
				DisplayName              string `json:"displayName"`
				DefaultReasoningEffort   string `json:"defaultReasoningEffort"`
				SupportedReasoningEffort []struct {
					ReasoningEffort string `json:"reasoningEffort"`
				} `json:"supportedReasoningEfforts"`
				InputModalities []string `json:"inputModalities"`
				IsDefault       bool     `json:"isDefault"`
			} `json:"data"`
		} `json:"result"`
	}
	if err := json.Unmarshal(message, &payload); err != nil {
		return nil, fmt.Errorf("decode Codex model list: %w", err)
	}
	models := make([]Model, 0, len(payload.Result.Data))
	for _, item := range payload.Result.Data {
		modelID := firstNonEmpty(item.Model, item.ID)
		if modelID == "" {
			continue
		}
		efforts := make([]string, 0, len(item.SupportedReasoningEffort))
		for _, effort := range item.SupportedReasoningEffort {
			if value := strings.TrimSpace(effort.ReasoningEffort); value != "" {
				efforts = append(efforts, value)
			}
		}
		models = append(models, Model{
			ID:                       modelID,
			DisplayName:              firstNonEmpty(item.DisplayName, modelID),
			DefaultReasoningEffort:   item.DefaultReasoningEffort,
			SupportedReasoningEffort: efforts,
			InputModalities:          item.InputModalities,
			IsDefault:                item.IsDefault,
		})
	}
	return models, nil
}

func (c *Client) Complete(ctx context.Context, request Request, onDelta func(string) error) (string, error) {
	if c == nil || !c.config.Enabled {
		return "", errors.New("Codex subscription runtime is only available in the desktop app")
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	if err := c.ensureStarted(); err != nil {
		return "", err
	}

	imageInputs, cleanup, err := c.prepareImages(request.Images)
	if err != nil {
		return "", err
	}
	defer cleanup()

	threadParams := map[string]any{
		"ephemeral":      true,
		"cwd":            c.workDir,
		"approvalPolicy": "never",
		"sandbox":        "read-only",
		"config":         isolatedThreadConfig(c.mcpNames),
	}
	if modelName := strings.TrimSpace(request.Model); modelName != "" {
		threadParams["model"] = modelName
	}
	threadIDRequest := c.newID()
	if err := c.send(map[string]any{"method": "thread/start", "id": threadIDRequest, "params": threadParams}); err != nil {
		c.stopProcess()
		return "", err
	}
	threadMessage, err := c.readResponse(threadIDRequest)
	if err != nil {
		c.stopProcess()
		return "", err
	}
	if rpcErr := responseError(threadMessage); rpcErr != nil {
		return "", rpcErr
	}
	var threadResponse struct {
		Result struct {
			Thread struct {
				ID string `json:"id"`
			} `json:"thread"`
		} `json:"result"`
	}
	if err := json.Unmarshal(threadMessage, &threadResponse); err != nil || strings.TrimSpace(threadResponse.Result.Thread.ID) == "" {
		return "", fmt.Errorf("Codex app-server did not return a thread id")
	}
	threadID := threadResponse.Result.Thread.ID

	input := []map[string]any{{"type": "text", "text": buildPrompt(request.SystemPrompt, request.UserPrompt)}}
	input = append(input, imageInputs...)
	turnParams := map[string]any{
		"threadId":       threadID,
		"input":          input,
		"approvalPolicy": "never",
		"sandboxPolicy":  readOnlySandboxPolicy(),
	}
	if effort := strings.TrimSpace(request.ReasoningEffort); effort != "" {
		turnParams["effort"] = effort
	}
	turnRequestID := c.newID()
	if err := c.send(map[string]any{"method": "turn/start", "id": turnRequestID, "params": turnParams}); err != nil {
		c.stopProcess()
		return "", err
	}
	turnMessage, err := c.readResponse(turnRequestID)
	if err != nil {
		c.stopProcess()
		return "", err
	}
	if rpcErr := responseError(turnMessage); rpcErr != nil {
		return "", rpcErr
	}
	var turnResponse struct {
		Result struct {
			Turn struct {
				ID string `json:"id"`
			} `json:"turn"`
		} `json:"result"`
	}
	_ = json.Unmarshal(turnMessage, &turnResponse)
	turnID := turnResponse.Result.Turn.ID

	done := make(chan struct{})
	if ctx != nil {
		go func() {
			select {
			case <-ctx.Done():
				if turnID != "" {
					_ = c.send(map[string]any{"method": "turn/interrupt", "id": c.newID(), "params": map[string]any{"threadId": threadID, "turnId": turnID}})
				}
			case <-done:
			}
		}()
	}
	defer close(done)

	var assembled strings.Builder
	for {
		message, readErr := c.readMessage()
		if readErr != nil {
			c.stopProcess()
			return "", readErr
		}
		var event struct {
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
			Error  *rpcError       `json:"error"`
			ID     json.RawMessage `json:"id"`
		}
		if err := json.Unmarshal(message, &event); err != nil {
			continue
		}
		if event.Error != nil {
			return "", event.Error
		}
		switch event.Method {
		case "item/agentMessage/delta":
			delta := eventDelta(event.Params)
			if delta == "" {
				continue
			}
			assembled.WriteString(delta)
			if onDelta != nil {
				if err := onDelta(delta); err != nil {
					c.stopProcess()
					return "", err
				}
			}
		case "item/completed":
			if assembled.Len() == 0 {
				if text := completedAgentText(event.Params); text != "" {
					assembled.WriteString(text)
					if onDelta != nil {
						if err := onDelta(text); err != nil {
							c.stopProcess()
							return "", err
						}
					}
				}
			}
		case "turn/completed":
			if ctx != nil && ctx.Err() != nil {
				return "", ctx.Err()
			}
			if eventErr := completedTurnError(event.Params); eventErr != nil {
				return "", eventErr
			}
			text := strings.TrimSpace(assembled.String())
			if text == "" {
				return "", errors.New("Codex completed without an assistant message")
			}
			return text, nil
		}
	}
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()
	c.stopProcess()
	if c.workDir != "" {
		err := os.RemoveAll(c.workDir)
		c.workDir = ""
		return err
	}
	return nil
}

func (c *Client) ensureStarted() error {
	binary, err := c.resolveBinary()
	if err != nil {
		return err
	}
	if _, err := c.chatGPTLoginStatus(context.Background(), binary); err != nil {
		return err
	}
	if c.cmd != nil && c.cmd.Process != nil {
		return nil
	}
	if c.workDir == "" {
		c.workDir, err = os.MkdirTemp("", "citebox-codex-")
		if err != nil {
			return fmt.Errorf("create Codex runtime directory: %w", err)
		}
	}

	c.mcpNames, err = c.configuredMCPNames(binary)
	if err != nil {
		return err
	}
	cmd := exec.Command(binary, appServerArgs()...)
	cmd.Dir = c.workDir
	cmd.Env = safeEnvironment()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	c.stderr.Reset()
	cmd.Stderr = &c.stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Codex app-server: %w", err)
	}
	c.cmd = cmd
	c.stdin = stdin
	c.scanner = bufio.NewScanner(stdout)
	c.scanner.Buffer(make([]byte, 64*1024), maxMessageBytes)

	id := c.newID()
	if err := c.send(map[string]any{
		"method": "initialize",
		"id":     id,
		"params": map[string]any{"clientInfo": map[string]any{
			"name": "citebox", "title": "CiteBox", "version": "1",
		}},
	}); err != nil {
		c.stopProcess()
		return err
	}
	message, err := c.readResponse(id)
	if err != nil {
		c.stopProcess()
		return err
	}
	if rpcErr := responseError(message); rpcErr != nil {
		c.stopProcess()
		return rpcErr
	}
	if err := c.send(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		c.stopProcess()
		return err
	}
	return nil
}

func (c *Client) configuredMCPNames(binary string) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), mcpDiscoveryTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "mcp", "list", "--json")
	cmd.Dir = c.workDir
	cmd.Env = safeEnvironment()
	output, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("%w after %s", ErrMCPDiscoveryTimeout, mcpDiscoveryTimeout)
		}
		return nil, fmt.Errorf("inspect Codex MCP configuration: %w", err)
	}
	var servers []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(output, &servers); err != nil {
		return nil, fmt.Errorf("decode Codex MCP configuration: %w", err)
	}
	names := make([]string, 0, len(servers))
	seen := make(map[string]struct{}, len(servers))
	for _, server := range servers {
		name := strings.TrimSpace(server.Name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (c *Client) chatGPTLoginStatus(ctx context.Context, binary string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	loginCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(loginCtx, binary, "login", "status")
	cmd.Env = safeEnvironment()
	output, err := cmd.CombinedOutput()
	message := strings.TrimSpace(string(output))
	if err != nil {
		return message, fmt.Errorf("Codex login status: %w", err)
	}
	if !strings.Contains(strings.ToLower(message), "using chatgpt") {
		return "Codex CLI 当前未通过 ChatGPT 登录；为避免 API 计费，请运行 codex logout 后重新执行 codex login", errors.New("Codex login is not authenticated with ChatGPT")
	}
	return message, nil
}

func appServerArgs() []string {
	return []string{
		"app-server", "--listen", "stdio://",
		"--disable", "apps",
		"--disable", "plugins",
		"--disable", "browser_use",
		"--disable", "browser_use_external",
		"--disable", "computer_use",
		"--disable", "code_mode_host",
		"--disable", "image_generation",
		"--disable", "in_app_browser",
		"--disable", "multi_agent",
		"--disable", "skill_search",
		"--disable", "shell_tool",
		"--disable", "unified_exec",
		"--disable", "hooks",
	}
}

func isolatedThreadConfig(mcpNames []string) map[string]any {
	mcpServers := make(map[string]any, len(mcpNames))
	for _, name := range mcpNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		mcpServers[name] = map[string]any{"enabled": false}
	}
	return map[string]any{"mcp_servers": mcpServers}
}

func readOnlySandboxPolicy() map[string]any {
	return map[string]any{"type": "readOnly", "networkAccess": false}
}

func (c *Client) resolveBinary() (string, error) {
	if c.binary != "" {
		return c.binary, nil
	}
	candidates := make([]string, 0, 12)
	if configured := strings.TrimSpace(c.config.Binary); configured != "" {
		candidates = append(candidates, configured)
	}
	candidates = append(candidates, "codex")
	if homeDir, err := os.UserHomeDir(); err == nil && strings.TrimSpace(homeDir) != "" {
		binaryName := "codex"
		if runtime.GOOS == "windows" {
			binaryName = "codex.exe"
		}
		candidates = append(candidates,
			filepath.Join(homeDir, ".local", "bin", binaryName),
			filepath.Join(homeDir, ".npm-global", "bin", binaryName),
			filepath.Join(homeDir, ".volta", "bin", binaryName),
		)
	}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, "/opt/homebrew/bin/codex", "/usr/local/bin/codex")
	}
	if runtime.GOOS == "windows" {
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			candidates = append(candidates, filepath.Join(appData, "npm", "codex.exe"))
		}
	}
	for _, candidate := range candidates {
		if filepath.IsAbs(candidate) {
			if isExecutableFile(candidate) {
				c.binary = candidate
				return candidate, nil
			}
			continue
		}
		if resolved, err := exec.LookPath(candidate); err == nil {
			c.binary = resolved
			return resolved, nil
		}
	}
	return "", errors.New("Codex CLI executable not found")
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return runtime.GOOS == "windows" || info.Mode()&0o111 != 0
}

func (c *Client) prepareImages(images []Image) ([]map[string]any, func(), error) {
	if len(images) == 0 {
		return nil, func() {}, nil
	}
	dir, err := os.MkdirTemp(c.workDir, "images-")
	if err != nil {
		return nil, func() {}, fmt.Errorf("create Codex image directory: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	inputs := make([]map[string]any, 0, len(images))
	for index, image := range images {
		data, err := base64.StdEncoding.DecodeString(image.Data)
		if err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("decode Codex image input: %w", err)
		}
		extension := ".png"
		switch strings.ToLower(strings.TrimSpace(image.MIMEType)) {
		case "image/jpeg", "image/jpg":
			extension = ".jpg"
		case "image/webp":
			extension = ".webp"
		}
		path := filepath.Join(dir, fmt.Sprintf("image-%d%s", index+1, extension))
		if err := os.WriteFile(path, data, 0o600); err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("write Codex image input: %w", err)
		}
		inputs = append(inputs, map[string]any{"type": "localImage", "path": path})
	}
	return inputs, cleanup, nil
}

func (c *Client) send(message any) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	if c.stdin == nil {
		return errors.New("Codex app-server is not running")
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if _, err := c.stdin.Write(payload); err != nil {
		return fmt.Errorf("write Codex app-server message: %w", err)
	}
	return nil
}

func (c *Client) readResponse(id int64) ([]byte, error) {
	for {
		message, err := c.readMessage()
		if err != nil {
			return nil, err
		}
		var envelope struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal(message, &envelope) != nil || len(envelope.ID) == 0 {
			continue
		}
		var responseID int64
		if json.Unmarshal(envelope.ID, &responseID) == nil && responseID == id {
			return message, nil
		}
	}
}

func (c *Client) readMessage() ([]byte, error) {
	if c.scanner == nil || !c.scanner.Scan() {
		if c.scanner != nil && c.scanner.Err() != nil {
			return nil, fmt.Errorf("read Codex app-server message: %w", c.scanner.Err())
		}
		message := strings.TrimSpace(c.stderr.String())
		return nil, errors.New(firstNonEmpty(message, "Codex app-server stopped unexpectedly"))
	}
	message := append([]byte(nil), c.scanner.Bytes()...)
	return message, nil
}

func (c *Client) stopProcess() {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_, _ = c.cmd.Process.Wait()
	}
	c.cmd = nil
	c.stdin = nil
	c.scanner = nil
}

func (c *Client) newID() int64 {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	id := c.nextID
	c.nextID++
	return id
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	if e == nil {
		return ""
	}
	return firstNonEmpty(e.Message, fmt.Sprintf("Codex app-server error %d", e.Code))
}

func responseError(message []byte) error {
	var envelope struct {
		Error *rpcError `json:"error"`
	}
	if err := json.Unmarshal(message, &envelope); err != nil {
		return err
	}
	if envelope.Error != nil {
		return envelope.Error
	}
	return nil
}

func eventDelta(raw json.RawMessage) string {
	var params struct {
		Delta any `json:"delta"`
	}
	if json.Unmarshal(raw, &params) != nil {
		return ""
	}
	switch delta := params.Delta.(type) {
	case string:
		return delta
	case map[string]any:
		if text, ok := delta["text"].(string); ok {
			return text
		}
	}
	return ""
}

func completedAgentText(raw json.RawMessage) string {
	var params struct {
		Item struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"item"`
	}
	if json.Unmarshal(raw, &params) != nil || params.Item.Type != "agentMessage" {
		return ""
	}
	return params.Item.Text
}

func completedTurnError(raw json.RawMessage) error {
	var params struct {
		Turn struct {
			Status string `json:"status"`
			Error  any    `json:"error"`
		} `json:"turn"`
	}
	if json.Unmarshal(raw, &params) != nil {
		return nil
	}
	if params.Turn.Status == "completed" || params.Turn.Status == "" {
		return nil
	}
	if params.Turn.Error != nil {
		payload, _ := json.Marshal(params.Turn.Error)
		return fmt.Errorf("Codex turn %s: %s", params.Turn.Status, strings.TrimSpace(string(payload)))
	}
	return fmt.Errorf("Codex turn ended with status %s", params.Turn.Status)
}

func buildPrompt(systemPrompt, userPrompt string) string {
	return "You are the language-model backend for CiteBox. Do not inspect files, run commands, or modify the environment. " +
		"Answer only from the supplied instructions and content.\n\nSYSTEM INSTRUCTIONS:\n" + strings.TrimSpace(systemPrompt) +
		"\n\nUSER INPUT:\n" + strings.TrimSpace(userPrompt)
}

func safeEnvironment() []string {
	allowed := map[string]struct{}{
		"HOME": {}, "PATH": {}, "TMPDIR": {}, "TEMP": {}, "TMP": {},
		"USER": {}, "LOGNAME": {}, "SHELL": {}, "LANG": {}, "LC_ALL": {}, "LC_CTYPE": {}, "TERM": {},
		"CODEX_HOME": {}, "CODEX_CA_CERTIFICATE": {}, "SSL_CERT_FILE": {},
		"HTTP_PROXY": {}, "HTTPS_PROXY": {}, "ALL_PROXY": {}, "NO_PROXY": {},
		"http_proxy": {}, "https_proxy": {}, "all_proxy": {}, "no_proxy": {},
	}
	environment := make([]string, 0, len(allowed))
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if _, keep := allowed[name]; keep {
			environment = append(environment, entry)
		}
	}
	return environment
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
