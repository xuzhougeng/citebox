package ai_assistant

import "testing"

func TestRouteIntentHonorsHint(t *testing.T) {
	got := RouteIntent(RouteInput{Content: "普通问题", IntentHint: IntentFigureLookup})
	if got.Intent != IntentFigureLookup || got.Confidence != "hint" {
		t.Fatalf("route = %+v", got)
	}
}

func TestRouteIntentDetectsLibrarySearch(t *testing.T) {
	got := RouteIntent(RouteInput{Content: "帮我查找包括 ATAC 数据的文章"})
	if got.Intent != IntentLibrarySearch {
		t.Fatalf("intent = %q", got.Intent)
	}
}

func TestRouteIntentDetectsExternalSearch(t *testing.T) {
	got := RouteIntent(RouteInput{Content: "查一下外部有没有 single-cell ATAC 综述"})
	if got.Intent != IntentExternalSearch {
		t.Fatalf("intent = %q", got.Intent)
	}
}

func TestRouteIntentDetectsPaperCompare(t *testing.T) {
	got := RouteIntent(RouteInput{Content: "对比这两篇文献的结论差异", Context: RequestContext{PaperIDs: []int64{1, 2}}})
	if got.Intent != IntentPaperRead {
		t.Fatalf("intent = %q", got.Intent)
	}
}

func TestRouteIntentDetectsFigureLookup(t *testing.T) {
	for _, q := range []string{"看图 1", "找所有 ATAC 相关的图"} {
		got := RouteIntent(RouteInput{Content: q})
		if got.Intent != IntentFigureLookup {
			t.Fatalf("RouteIntent(%q) = %q", q, got.Intent)
		}
	}
}

func TestRouteIntentDetectsFigureContext(t *testing.T) {
	got := RouteIntent(RouteInput{Content: "解释一下", Context: RequestContext{FigureID: 1}})
	if got.Intent != IntentFigureLookup {
		t.Fatalf("intent = %q", got.Intent)
	}
}

func TestRouteIntentDetectsPaperContext(t *testing.T) {
	got := RouteIntent(RouteInput{Content: "解释一下", Context: RequestContext{PaperID: 1}})
	if got.Intent != IntentPaperRead {
		t.Fatalf("intent = %q", got.Intent)
	}
}

func TestRouteIntentEmptyContentRoutesChat(t *testing.T) {
	got := RouteIntent(RouteInput{})
	if got.Intent != IntentChat || got.Confidence != "low" {
		t.Fatalf("route = %+v", got)
	}
}

func TestRouteIntentInvalidHintFallsBackToRules(t *testing.T) {
	got := RouteIntent(RouteInput{Content: "看图 1", IntentHint: "unknown"})
	if got.Intent != IntentFigureLookup || got.Confidence == "hint" {
		t.Fatalf("route = %+v", got)
	}
}

func TestRouteIntentLocalReviewSearchStaysLibrary(t *testing.T) {
	got := RouteIntent(RouteInput{Content: "查找库里的 ATAC 综述文章"})
	if got.Intent != IntentLibrarySearch {
		t.Fatalf("intent = %q", got.Intent)
	}
}

func TestRouteIntentExplicitExternalReviewSearch(t *testing.T) {
	got := RouteIntent(RouteInput{Content: "查一下外部有没有 single-cell ATAC 综述"})
	if got.Intent != IntentExternalSearch {
		t.Fatalf("intent = %q", got.Intent)
	}
}
