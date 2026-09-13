package ai_assistant

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Tool interface {
	Run(ctx context.Context, in ToolInput) (ToolResult, error)
}

type ToolSet struct {
	LibrarySearch  Tool
	ExternalSearch Tool
	PaperRead      Tool
	FigureLookup   Tool
	RemoteMCP      Tool
}

type Orchestrator struct {
	tools   ToolSet
	planner RetrievalPlanner
}

func NewOrchestrator(tools ToolSet) *Orchestrator {
	return &Orchestrator{tools: tools}
}

func (o *Orchestrator) WithPlanner(p RetrievalPlanner) *Orchestrator { o.planner = p; return o }

type RunInput struct {
	History            []PlanningMessage
	Summary            string
	PinnedPapers       []PlanningPaper
	ExplicitPaperScope bool
	Content            string
	IntentHint         string
	SearchGoalHint     ExternalSearchGoal
	Sources            []string
	Context            RequestContext
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
	query := in.Content
	var terms []string
	var plan RetrievalPlan
	var planningCalls []ToolCallSummary
	planningInput := RetrievalPlanningInput{Question: in.Content, IntentHint: in.IntentHint, History: in.History, Summary: in.Summary, Papers: in.PinnedPapers, Context: in.Context}
	if o.planner != nil && route.Intent != IntentRemoteMCP {
		planned, err := o.planner.PlanRetrieval(ctx, planningInput)
		if err == nil {
			plan = planned
			if isKnownIntent(in.IntentHint) {
				plan.Intent = in.IntentHint
			}
			if plan.Intent == IntentExternalSearch && plan.Scope != "external" {
				plan.Intent = route.Intent
			}
			if plan.Intent != IntentRemoteMCP && (isKnownIntent(plan.Intent) || plan.Intent == IntentChat) {
				route.Intent = plan.Intent
			}
			if plan.ResolvedQuery != "" {
				query = plan.ResolvedQuery
			}
			terms = plan.SearchTerms
			if route.Intent == IntentPaperRead || route.Intent == IntentFigureLookup {
				if in.Context.PaperID == 0 && len(in.Context.PaperIDs) == 0 {
					for _, paper := range in.PinnedPapers {
						in.Context.PaperIDs = append(in.Context.PaperIDs, paper.ID)
					}
				}
			}
			if (route.Intent == IntentLibrarySearch || route.Intent == IntentExternalSearch) && in.IntentHint == "" && !in.ExplicitPaperScope && plan.Scope != "pinned" {
				in.Context.PaperIDs = nil
				in.Context.PaperID = 0
			}
			payload, _ := json.Marshal(plan)
			planningCalls = append(planningCalls, ToolCallSummary{ToolName: "retrieval_plan", Status: "completed", OutputSummaryJSON: string(payload)})
		} else {
			if ctx.Err() != nil {
				return RunOutput{}, ctx.Err()
			}
			planningCalls = append(planningCalls, ToolCallSummary{ToolName: "retrieval_plan", Status: "failed", Error: err.Error()})
		}
	}
	tool := o.toolForIntent(route.Intent)
	if tool == nil {
		return RunOutput{
			Intent:     route.Intent,
			IntentHint: in.IntentHint,
			Process:    ProcessSummary{Intent: route.Intent, Note: route.Reason},
			ToolCalls:  planningCalls,
			AnswerContext: "用户问题：\n" +
				strings.TrimSpace(in.Content),
		}, nil
	}

	toolHint := in.IntentHint
	if route.Intent == IntentLibrarySearch && (plan.Scope == "pinned" || in.ExplicitPaperScope) {
		toolHint = IntentLibrarySearch
	}
	res, err := tool.Run(ctx, ToolInput{
		Query:          query,
		SearchTerms:    terms,
		Context:        in.Context,
		IntentHint:     toolHint,
		SearchGoalHint: in.SearchGoalHint,
		Sources:        in.Sources,
	})
	if err != nil {
		return RunOutput{}, err
	}

	// One evidence-guided follow-up for pinned reading. Both planning and tool
	// calls receive the request cancellation; follow-up has its own hard deadline.
	if o.planner != nil && len(terms) > 0 && route.Intent == IntentPaperRead && ctx.Err() == nil {
		followCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		planningInput.Context = in.Context
		planningInput.Evidence = res.AnswerContext
		follow, err := o.planner.PlanRetrieval(followCtx, planningInput)
		reviewCall := ToolCallSummary{ToolName: "retrieval_review", Status: "completed"}
		if err != nil {
			reviewCall.Status = "failed"
			reviewCall.Error = err.Error()
		} else {
			payload, _ := json.Marshal(follow)
			reviewCall.OutputSummaryJSON = string(payload)
		}
		planningCalls = append(planningCalls, reviewCall)
		if err == nil && !follow.Sufficient && len(follow.SearchTerms) > 0 && strings.Join(follow.SearchTerms, "|") != strings.Join(terms, "|") {
			extra, readErr := tool.Run(followCtx, ToolInput{Query: query, SearchTerms: follow.SearchTerms, Context: in.Context, IntentHint: in.IntentHint})
			if readErr == nil {
				res = mergeReadingResults(res, extra)
			}
		}
		cancel()
	}
	res.ToolCalls = append(planningCalls, res.ToolCalls...)
	if ctx.Err() != nil {
		return RunOutput{}, ctx.Err()
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
	case IntentRemoteMCP:
		return o.tools.RemoteMCP
	default:
		return nil
	}
}

func buildFinalAnswerContext(userText string, res ToolResult) string {
	var b strings.Builder
	b.WriteString("你正在基于本轮提供的钉住文献、原文引用、图片输入及下列工具证据回答。只使用实际提供的内容支持结论；抽样片段不等于完整全文，图片文字说明不等于看到了图片，证据不足时明确说明。\n\n")
	if res.AnswerContext != "" {
		b.WriteString("工具结果：\n")
		b.WriteString(res.AnswerContext)
		b.WriteString("\n\n")
	}
	b.WriteString("用户问题：\n")
	b.WriteString(strings.TrimSpace(userText))
	return b.String()
}

var citationNumberRE = regexp.MustCompile(`\[(\d+)\]`)

func mergeReadingResults(first, extra ToolResult) ToolResult {
	offset := len(first.Citations)
	extra.Citations = append([]Citation(nil), extra.Citations...)
	extra.Cards = append([]ResultCard(nil), extra.Cards...)
	for i := range extra.Citations {
		extra.Citations[i].I += offset
	}
	extra.AnswerContext = citationNumberRE.ReplaceAllStringFunc(extra.AnswerContext, func(match string) string {
		n, _ := strconv.Atoi(match[1 : len(match)-1])
		return "[" + strconv.Itoa(n+offset) + "]"
	})
	for i := range extra.Cards {
		if card, ok := extra.Cards[i].Payload.(PaperCompareCard); ok {
			card.Papers = append([]PaperCompareItem(nil), card.Papers...)
			for j := range card.Papers {
				card.Papers[j].Evidence = append([]PaperHitSnippet(nil), card.Papers[j].Evidence...)
				for k := range card.Papers[j].Evidence {
					card.Papers[j].Evidence[k].CitationIndex += offset
				}
			}
			extra.Cards[i].Payload = card
		}
	}
	first.Citations = append(first.Citations, extra.Citations...)
	first.Cards = append(first.Cards, extra.Cards...)
	first.ToolCalls = append(first.ToolCalls, extra.ToolCalls...)
	first.AnswerContext += "\n\n" + extra.AnswerContext
	return first
}
