package ai_assistant

import (
	"context"
	"strings"
)

type Tool interface {
	Run(ctx context.Context, in ToolInput) (ToolResult, error)
}

type ToolSet struct {
	LibrarySearch  Tool
	ExternalSearch Tool
	PaperRead      Tool
	FigureLookup   Tool
}

type Orchestrator struct {
	tools ToolSet
}

func NewOrchestrator(tools ToolSet) *Orchestrator {
	return &Orchestrator{tools: tools}
}

type RunInput struct {
	Content    string
	IntentHint string
	Context    RequestContext
}

type RunOutput struct {
	Intent        string
	IntentHint    string
	Process       ProcessSummary
	Cards         []ResultCard
	Citations     []Citation
	AnswerContext string
	ToolCalls     []ToolCallSummary
}

func (o *Orchestrator) Run(ctx context.Context, in RunInput) (RunOutput, error) {
	route := RouteIntent(RouteInput{Content: in.Content, IntentHint: in.IntentHint, Context: in.Context})
	tool := o.toolForIntent(route.Intent)
	if tool == nil {
		return RunOutput{
			Intent:     route.Intent,
			IntentHint: in.IntentHint,
			Process:    ProcessSummary{Intent: route.Intent, Note: route.Reason},
			AnswerContext: "用户问题：\n" +
				strings.TrimSpace(in.Content),
		}, nil
	}

	res, err := tool.Run(ctx, ToolInput{Query: in.Content, Context: in.Context, IntentHint: in.IntentHint})
	if err != nil {
		return RunOutput{}, err
	}

	return RunOutput{
		Intent:        route.Intent,
		IntentHint:    in.IntentHint,
		Process:       res.Process,
		Cards:         res.Cards,
		Citations:     res.Citations,
		AnswerContext: buildFinalAnswerContext(in.Content, res),
		ToolCalls:     res.ToolCalls,
	}, nil
}

func (o *Orchestrator) toolForIntent(intent string) Tool {
	switch intent {
	case IntentLibrarySearch:
		return o.tools.LibrarySearch
	case IntentExternalSearch:
		return o.tools.ExternalSearch
	case IntentPaperRead:
		return o.tools.PaperRead
	case IntentFigureLookup:
		return o.tools.FigureLookup
	default:
		return nil
	}
}

func buildFinalAnswerContext(userText string, res ToolResult) string {
	var b strings.Builder
	b.WriteString("你正在基于工具检索结果回答。只使用下列证据和结果卡片支持结论；证据不足时明确说明。\n\n")
	if res.AnswerContext != "" {
		b.WriteString("工具结果：\n")
		b.WriteString(res.AnswerContext)
		b.WriteString("\n\n")
	}
	b.WriteString("用户问题：\n")
	b.WriteString(strings.TrimSpace(userText))
	return b.String()
}
