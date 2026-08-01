package ai_assistant

import (
	"context"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/mcp"
	"github.com/xuzhougeng/citebox/internal/model"
)

type stubRemoteMCP struct {
	calledName string
	calledArgs map[string]any
}

func (s *stubRemoteMCP) ListTools(context.Context) ([]mcp.Tool, error) {
	return []mcp.Tool{{Name: "notion-search", Description: "Search workspace", InputSchema: map[string]any{"type": "object"}}}, nil
}

func (s *stubRemoteMCP) CallTool(_ context.Context, name string, arguments map[string]any) (mcp.CallResult, error) {
	s.calledName = name
	s.calledArgs = arguments
	return mcp.CallResult{Content: []mcp.Content{{Type: "text", Text: "workspace result"}}}, nil
}

func TestMCPToolPlansValidatesAndCallsRemoteTool(t *testing.T) {
	remote := &stubRemoteMCP{}
	caller := &stubNonStreamCaller{output: `{"tool_name":"notion-search","arguments":{"query":"CRISPR"}}`}
	tool := NewMCPTool(remote, stubAISettingsProvider{settings: model.DefaultAISettings()}, caller)
	result, err := tool.Run(context.Background(), ToolInput{Query: "在 Notion 搜索 CRISPR"})
	if err != nil {
		t.Fatal(err)
	}
	if remote.calledName != "notion-search" || remote.calledArgs["query"] != "CRISPR" {
		t.Fatalf("call = %q %#v", remote.calledName, remote.calledArgs)
	}
	if !strings.Contains(result.AnswerContext, "workspace result") || len(result.ToolCalls) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestMCPToolRejectsPlannerHallucinatedTool(t *testing.T) {
	remote := &stubRemoteMCP{}
	caller := &stubNonStreamCaller{output: `{"tool_name":"invented","arguments":{}}`}
	tool := NewMCPTool(remote, stubAISettingsProvider{settings: model.DefaultAISettings()}, caller)
	if _, err := tool.Run(context.Background(), ToolInput{Query: "search"}); err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("error = %v", err)
	}
}
