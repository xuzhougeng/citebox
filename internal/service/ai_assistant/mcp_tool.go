package ai_assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/xuzhougeng/citebox/internal/mcp"
)

type RemoteMCPClient interface {
	ListTools(ctx context.Context) ([]mcp.Tool, error)
	CallTool(ctx context.Context, name string, arguments map[string]any) (mcp.CallResult, error)
}

type MCPTool struct {
	client   RemoteMCPClient
	settings AISettingsProvider
	caller   NonStreamCaller
}

func NewMCPTool(client RemoteMCPClient, settings AISettingsProvider, caller NonStreamCaller) *MCPTool {
	return &MCPTool{client: client, settings: settings, caller: caller}
}

type mcpToolPlan struct {
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments"`
}

func (t *MCPTool) Run(ctx context.Context, in ToolInput) (ToolResult, error) {
	if t == nil || t.client == nil || t.settings == nil || t.caller == nil {
		return ToolResult{}, fmt.Errorf("remote MCP tool is not configured")
	}
	started := time.Now()
	tools, err := t.client.ListTools(ctx)
	if err != nil {
		return ToolResult{}, err
	}
	if len(tools) == 0 {
		return ToolResult{}, fmt.Errorf("remote MCP server returned no tools")
	}
	settings, err := t.settings.GetSettings()
	if err != nil {
		return ToolResult{}, err
	}
	catalog, _ := json.Marshal(tools)
	systemPrompt := `你是 CiteBox 的 MCP 工具规划器。根据用户请求，从提供的 MCP 工具目录中只选择一个最合适的工具，并严格按该工具 inputSchema 生成参数。
只输出 JSON，不要 Markdown，不要解释。格式：{"tool_name":"目录中的精确工具名","arguments":{}}。不得编造工具名或参数。`
	userPrompt := "MCP 工具目录：\n" + trimRunes(string(catalog), 24000) + "\n\n用户请求：\n" + strings.TrimSpace(in.Query)
	out, _, err := t.caller.CallProviderGeneric(ctx, assistantMasterSettings(*settings), systemPrompt, userPrompt)
	if err != nil {
		return ToolResult{}, err
	}
	var plan mcpToolPlan
	if err := decodeFirstJSONObject(out, &plan); err != nil {
		return ToolResult{}, fmt.Errorf("decode MCP tool plan: %w", err)
	}
	var selected *mcp.Tool
	for i := range tools {
		if tools[i].Name == plan.ToolName {
			selected = &tools[i]
			break
		}
	}
	if selected == nil {
		return ToolResult{}, fmt.Errorf("MCP planner selected unknown tool %q", plan.ToolName)
	}
	if plan.Arguments == nil {
		plan.Arguments = map[string]any{}
	}
	result, err := t.client.CallTool(ctx, selected.Name, plan.Arguments)
	if err != nil {
		return ToolResult{}, err
	}
	var texts []string
	for _, content := range result.Content {
		if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
			texts = append(texts, content.Text)
		}
	}
	answer := strings.Join(texts, "\n")
	if answer == "" {
		raw, _ := json.Marshal(result)
		answer = string(raw)
	}
	inputJSON, _ := json.Marshal(plan.Arguments)
	outputJSON, _ := json.Marshal(map[string]any{"content_blocks": len(result.Content)})
	duration := int(time.Since(started).Milliseconds())
	return ToolResult{
		Process: ProcessSummary{Intent: IntentRemoteMCP, Stages: []ProcessStage{
			{Label: "发现 MCP 工具", Count: len(tools), Unit: "个工具", Status: "completed"},
			{Label: "调用 MCP 工具", Count: 1, Unit: "次", Status: "completed", Detail: selected.Name, DurationMS: duration},
		}},
		AnswerContext: "MCP tool: " + selected.Name + "\nMCP result:\n" + trimRunes(answer, 30000),
		ToolCalls:     []ToolCallSummary{{ToolName: selected.Name, InputJSON: string(inputJSON), OutputSummaryJSON: string(outputJSON), Status: "completed", DurationMS: duration}},
	}, nil
}
