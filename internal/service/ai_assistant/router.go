package ai_assistant

import "strings"

func RouteIntent(in RouteInput) RouteDecision {
	if isKnownIntent(in.IntentHint) {
		return RouteDecision{Intent: in.IntentHint, Confidence: "hint", Reason: "user selected shortcut"}
	}
	q := strings.ToLower(strings.TrimSpace(in.Content))
	if q == "" {
		return RouteDecision{Intent: IntentChat, Confidence: "low", Reason: "empty content"}
	}
	if containsAny(q, "图", "figure", "fig.", "fig ") {
		return RouteDecision{Intent: IntentFigureLookup, Confidence: "rule", Reason: "figure terms"}
	}
	if in.Context.FigureID > 0 {
		return RouteDecision{Intent: IntentFigureLookup, Confidence: "rule", Reason: "figure context"}
	}
	hasLibraryTerms := containsAny(q, "查找", "找", "检索", "哪些文章", "哪些文献", "相关文献", "相关的文章", "articles", "papers")
	hasExplicitExternalTerms := containsAny(q, "外部", "semantic scholar", "pubmed", "web")
	hasReviewTerms := containsAny(q, "综述", "review")
	if hasExplicitExternalTerms || (hasReviewTerms && !hasLibraryTerms) {
		return RouteDecision{Intent: IntentExternalSearch, Confidence: "rule", Reason: "external search terms"}
	}
	if containsAny(q, "对比", "比较", "compare", "异同", "差异") || len(in.Context.PaperIDs) > 1 {
		return RouteDecision{Intent: IntentPaperRead, Confidence: "rule", Reason: "compare terms or multiple papers"}
	}
	if hasLibraryTerms {
		return RouteDecision{Intent: IntentLibrarySearch, Confidence: "rule", Reason: "library search terms"}
	}
	if in.Context.PaperID > 0 {
		return RouteDecision{Intent: IntentPaperRead, Confidence: "rule", Reason: "paper context"}
	}
	return RouteDecision{Intent: IntentChat, Confidence: "low", Reason: "default chat"}
}

func isKnownIntent(intent string) bool {
	switch intent {
	case IntentLibrarySearch, IntentExternalSearch, IntentPaperRead, IntentFigureLookup:
		return true
	default:
		return false
	}
}

func containsAny(s string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(s, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
