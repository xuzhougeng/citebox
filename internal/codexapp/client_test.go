package codexapp

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStatusUsesCodexLoginStatus(t *testing.T) {
	binary := writeFakeCodex(t)
	client := New(Config{Enabled: true, Binary: binary})
	t.Cleanup(func() { _ = client.Close() })

	status := client.Status(context.Background())
	if !status.DesktopAvailable || !status.CLIAvailable || !status.Authenticated {
		t.Fatalf("Status() = %+v", status)
	}
	if status.Version != "codex-cli 1.2.3" {
		t.Fatalf("Status().Version = %q", status.Version)
	}
}

func TestModelsParsesAppServerCatalog(t *testing.T) {
	binary := writeFakeCodex(t)
	client := New(Config{Enabled: true, Binary: binary})
	t.Cleanup(func() { _ = client.Close() })

	models, err := client.Models(context.Background())
	if err != nil {
		t.Fatalf("Models() error = %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("Models() = %+v", models)
	}
	got := models[0]
	if got.ID != "codex-test" || got.DisplayName != "Codex Test" || !got.IsDefault {
		t.Fatalf("Models()[0] = %+v", got)
	}
	if !reflect.DeepEqual(got.SupportedReasoningEffort, []string{"low", "high"}) {
		t.Fatalf("SupportedReasoningEffort = %#v", got.SupportedReasoningEffort)
	}
}

func TestCompleteStreamsAndAssemblesAgentMessage(t *testing.T) {
	binary := writeFakeCodex(t)
	client := New(Config{Enabled: true, Binary: binary})
	t.Cleanup(func() { _ = client.Close() })

	var deltas []string
	text, err := client.Complete(context.Background(), Request{
		Model:           "codex-test",
		ReasoningEffort: "high",
		SystemPrompt:    "Only answer the question.",
		UserPrompt:      "Say hello.",
	}, func(delta string) error {
		deltas = append(deltas, delta)
		return nil
	})
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if text != "hello world" {
		t.Fatalf("Complete() = %q", text)
	}
	if strings.Join(deltas, "") != text {
		t.Fatalf("deltas = %#v", deltas)
	}
}

func TestDisabledClientDoesNotResolveCredentials(t *testing.T) {
	client := New(Config{Enabled: false})
	status := client.Status(context.Background())
	if status.DesktopAvailable || status.CLIAvailable || status.Authenticated {
		t.Fatalf("Status() = %+v", status)
	}
	if _, err := client.Models(context.Background()); err == nil {
		t.Fatal("Models() error = nil")
	}
}

func TestResolveBinaryFindsUserLocalInstallOutsidePATH(t *testing.T) {
	homeDir := t.TempDir()
	pathDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	t.Setenv("PATH", pathDir)

	binaryName := "codex"
	if runtime.GOOS == "windows" {
		binaryName = "codex.exe"
	}
	binary := filepath.Join(homeDir, ".local", "bin", binaryName)
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatalf("create user-local bin: %v", err)
	}
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write user-local Codex: %v", err)
	}

	client := New(Config{Enabled: true})
	resolved, err := client.resolveBinary()
	if err != nil {
		t.Fatalf("resolveBinary() error = %v", err)
	}
	if resolved != binary {
		t.Fatalf("resolveBinary() = %q, want %q", resolved, binary)
	}
}

func TestStatusRejectsAPIKeyLogin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codex")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "codex-cli 1.2.3"
  exit 0
fi
if [ "$1" = "login" ] && [ "$2" = "status" ]; then
  echo "Logged in using an API key"
  exit 0
fi
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake Codex: %v", err)
	}
	client := New(Config{Enabled: true, Binary: path})
	status := client.Status(context.Background())
	if status.Authenticated {
		t.Fatalf("Status() = %+v, API key login must not be treated as subscription auth", status)
	}
	if _, err := client.Models(context.Background()); err == nil {
		t.Fatal("Models() error = nil for API key login")
	}
}

func TestAppServerArgsDisableExternalTools(t *testing.T) {
	args := appServerArgs()
	joined := strings.Join(args, " ")
	for _, expected := range []string{
		"--disable apps",
		"--disable plugins",
		"--disable browser_use",
		"--disable computer_use",
		"--disable shell_tool",
		"--disable unified_exec",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("appServerArgs() = %q, missing %q", joined, expected)
		}
	}
}

func TestReadOnlySandboxPolicyMatchesCurrentAppServerProtocol(t *testing.T) {
	policy := readOnlySandboxPolicy()
	if policy["type"] != "readOnly" || policy["networkAccess"] != false {
		t.Fatalf("readOnlySandboxPolicy() = %#v", policy)
	}
	if _, exists := policy["access"]; exists {
		t.Fatalf("readOnlySandboxPolicy() includes deprecated access field: %#v", policy)
	}
}

func TestMCPDiscoveryTimeoutAllowsColdCodexStartup(t *testing.T) {
	if mcpDiscoveryTimeout < 20*time.Second {
		t.Fatalf("mcpDiscoveryTimeout = %s, want at least 20s", mcpDiscoveryTimeout)
	}
}

func TestIsolatedThreadConfigDisablesConfiguredMCPServers(t *testing.T) {
	config := isolatedThreadConfig([]string{"z-server", "server-with-dash"})
	servers, ok := config["mcp_servers"].(map[string]any)
	if !ok || len(servers) != 2 {
		t.Fatalf("isolatedThreadConfig() = %#v", config)
	}
	for _, name := range []string{"z-server", "server-with-dash"} {
		entry, ok := servers[name].(map[string]any)
		if !ok || entry["enabled"] != false {
			t.Fatalf("MCP server %q config = %#v", name, servers[name])
		}
	}
}

func writeFakeCodex(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "codex")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "codex-cli 1.2.3"
  exit 0
fi
if [ "$1" = "login" ] && [ "$2" = "status" ]; then
  echo "Logged in using ChatGPT"
  exit 0
fi
if [ "$1" = "mcp" ] && [ "$2" = "list" ] && [ "$3" = "--json" ]; then
  printf '[{"name":"fake-mcp"}]\n'
  exit 0
fi
while IFS= read -r line; do
  case "$line" in
    *'"method":"initialize"'*)
      id=$(printf '%s' "$line" | sed -E 's/.*"id":([0-9]+).*/\1/')
      printf '{"id":%s,"result":{"userAgent":"fake"}}\n' "$id"
      ;;
    *'"method":"model/list"'*)
      id=$(printf '%s' "$line" | sed -E 's/.*"id":([0-9]+).*/\1/')
      printf '{"id":%s,"result":{"data":[{"id":"codex-test","model":"codex-test","displayName":"Codex Test","defaultReasoningEffort":"low","supportedReasoningEfforts":[{"reasoningEffort":"low"},{"reasoningEffort":"high"}],"inputModalities":["text","image"],"isDefault":true}]}}\n' "$id"
      ;;
    *'"method":"thread/start"'*)
      id=$(printf '%s' "$line" | sed -E 's/.*"id":([0-9]+).*/\1/')
      printf '{"id":%s,"result":{"thread":{"id":"thr_test"}}}\n' "$id"
      ;;
    *'"method":"turn/start"'*)
      id=$(printf '%s' "$line" | sed -E 's/.*"id":([0-9]+).*/\1/')
      printf '{"id":%s,"result":{"turn":{"id":"turn_test","status":"inProgress"}}}\n' "$id"
      printf '{"method":"item/agentMessage/delta","params":{"delta":"hello "}}\n'
      printf '{"method":"item/agentMessage/delta","params":{"delta":"world"}}\n'
      printf '{"method":"turn/completed","params":{"turn":{"id":"turn_test","status":"completed"}}}\n'
      ;;
  esac
done
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake Codex: %v", err)
	}
	return path
}
