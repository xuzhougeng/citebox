package ai_assistant

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

type stubPaperStore struct {
	papers map[int64]*model.Paper
	ids    []int64
	err    error
}

func (s stubPaperStore) GetPaperDetail(id int64) (*model.Paper, error) {
	return s.papers[id], nil
}

func (s stubPaperStore) ListEvidenceCandidatePaperIDs(terms []string, limit int) ([]int64, error) {
	if s.err != nil {
		return nil, s.err
	}
	return append([]int64(nil), s.ids...), nil
}

func TestLibrarySearchToolReturnsPaperHitCards(t *testing.T) {
	tool := NewLibrarySearchTool(stubPaperStore{
		ids: []int64{1, 2},
		papers: map[int64]*model.Paper{
			1: {
				ID:           1,
				Title:        "ATAC Atlas",
				DOI:          "10.1/atac",
				AbstractText: "The atlas studies chromatin accessibility.",
				PDFText:      "ATAC-seq identifies chromatin accessibility changes.",
			},
			2: {ID: 2, Title: "Unrelated", PDFText: "Protein localization only."},
		},
	})
	res, err := tool.Run(context.Background(), ToolInput{Query: "帮我查找包括 ATAC 数据的文章"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 1 || res.Cards[0].Type != "paper_hit" {
		t.Fatalf("cards = %+v", res.Cards)
	}
	if len(res.Citations) != 3 {
		t.Fatalf("citations = %+v", res.Citations)
	}
	for _, citation := range res.Citations {
		if citation.PaperID != 1 {
			t.Fatalf("citation = %+v, want PaperID 1", citation)
		}
	}
	card, ok := res.Cards[0].Payload.(PaperHitCard)
	if !ok {
		t.Fatalf("card payload = %T, want PaperHitCard", res.Cards[0].Payload)
	}
	if len(card.Snippets) != 3 {
		t.Fatalf("snippets = %+v", card.Snippets)
	}
	encoded, err := json.Marshal(card)
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	encodedCard := string(encoded)
	if !strings.Contains(encodedCard, `"highlight_terms"`) || !strings.Contains(encodedCard, "ATAC-seq") {
		t.Fatalf("card json = %s, want highlight_terms with search terms", encodedCard)
	}
	if !strings.Contains(res.AnswerContext, "ATAC Atlas") || !strings.Contains(res.AnswerContext, "chromatin accessibility") {
		t.Fatalf("answer context = %s", res.AnswerContext)
	}
	if len(res.Process.Stages) == 0 || res.Process.Stages[0].Label != "全文扫描" {
		t.Fatalf("process = %+v", res.Process)
	}
}

func TestLibrarySearchToolShowsZeroCountsInProcessLabels(t *testing.T) {
	res, err := NewLibrarySearchTool(stubPaperStore{}).Run(context.Background(), ToolInput{Query: "查找不存在的内容"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Process.Stages) != 2 {
		t.Fatalf("process = %+v", res.Process)
	}
	if res.Process.Stages[0].Label != "全文扫描 0篇" || res.Process.Stages[1].Label != "命中 0篇" {
		t.Fatalf("process = %+v, want explicit zero-count labels", res.Process)
	}
	if !strings.Contains(res.AnswerContext, "没有命中") {
		t.Fatalf("answer context = %q, want explicit no-hit reason", res.AnswerContext)
	}
}

type termSensitivePaperStore struct {
	paper *model.Paper
	terms []string
}

func (s *termSensitivePaperStore) GetPaperDetail(id int64) (*model.Paper, error) {
	if s.paper != nil && s.paper.ID == id {
		return s.paper, nil
	}
	return nil, nil
}

func (s *termSensitivePaperStore) ListEvidenceCandidatePaperIDs(terms []string, limit int) ([]int64, error) {
	s.terms = append([]string(nil), terms...)
	for _, term := range terms {
		if term == "叶绿体定位" {
			return []int64{s.paper.ID}, nil
		}
	}
	return nil, nil
}

func TestLibrarySearchToolUsesChineseLiteralFallback(t *testing.T) {
	store := &termSensitivePaperStore{
		paper: &model.Paper{
			ID:      9,
			Title:   "叶绿体定位研究",
			PDFText: "本研究分析叶绿体定位信号及相关调控机制。",
		},
	}
	tool := NewLibrarySearchTool(store)

	res, err := tool.Run(context.Background(), ToolInput{Query: "帮我查找叶绿体定位的文章"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 1 {
		t.Fatalf("cards = %+v, terms = %v, want one Chinese literal hit", res.Cards, store.terms)
	}
	if !containsTestTerm(store.terms, "叶绿体定位") {
		t.Fatalf("terms = %v, want Chinese literal term", store.terms)
	}
	if len(res.Citations) == 0 || !strings.Contains(res.Citations[0].Snippet.Text, "叶绿体定位") {
		t.Fatalf("citations = %+v, want Chinese literal snippet", res.Citations)
	}
}

func TestEvidenceSearchTermsKeepsAssayQuerySpecific(t *testing.T) {
	terms := EvidenceSearchTerms("查找包括ChiP-seq数据的文章")
	if !containsTestTermFold(terms, "chip-seq") {
		t.Fatalf("terms = %v, want ChIP-seq assay term", terms)
	}
	if containsTestTerm(terms, "数据") {
		t.Fatalf("terms = %v, should not include generic Chinese data term", terms)
	}
}

type stubLibraryPlanner struct {
	query string
	plan  LibrarySearchPlan
}

func (s *stubLibraryPlanner) PlanLibrarySearch(ctx context.Context, query string) (LibrarySearchPlan, error) {
	s.query = query
	return s.plan, nil
}

type stubLibraryClassifier struct {
	seen    []LibraryPaperClassificationInput
	results map[int64]LibraryPaperClassificationResult
	err     error
}

func (s *stubLibraryClassifier) ClassifyLibraryPaper(ctx context.Context, in LibraryPaperClassificationInput) (LibraryPaperClassificationResult, error) {
	s.seen = append(s.seen, in)
	if s.err != nil {
		return LibraryPaperClassificationResult{}, s.err
	}
	if s.results != nil {
		if res, ok := s.results[in.Paper.ID]; ok {
			return res, nil
		}
	}
	return LibraryPaperClassificationResult{Relevant: true, Reason: "accepted"}, nil
}

type planningPaperStore struct {
	papers map[int64]*model.Paper
	terms  []string
}

func (s *planningPaperStore) GetPaperDetail(id int64) (*model.Paper, error) {
	return s.papers[id], nil
}

func (s *planningPaperStore) ListEvidenceCandidatePaperIDs(terms []string, limit int) ([]int64, error) {
	s.terms = append([]string(nil), terms...)
	return []int64{1, 2}, nil
}

func TestLibrarySearchToolUsesPlannerAndClassifier(t *testing.T) {
	store := &planningPaperStore{
		papers: map[int64]*model.Paper{
			1: {ID: 1, Title: "ChIP-seq Paper", PDFText: "This study uses H3K27ac ChIP-seq data."},
			2: {ID: 2, Title: "Mention Only", PDFText: "The supplement mentions ChIP-seq only as a comparison window."},
		},
	}
	planner := &stubLibraryPlanner{plan: LibrarySearchPlan{
		SearchTerms: []string{"ChIP-seq"},
		Rationale:   "exact assay query",
	}}
	classifier := &stubLibraryClassifier{results: map[int64]LibraryPaperClassificationResult{
		1: {Relevant: true, Reason: "uses ChIP-seq data"},
		2: {Relevant: false, Reason: "only compares against prior data"},
	}}
	tool := NewLibrarySearchToolWithAgents(store, planner, classifier)

	res, err := tool.Run(context.Background(), ToolInput{Query: "查找包括ChiP-seq数据的文章"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if planner.query == "" {
		t.Fatalf("planner was not called")
	}
	if !containsTestTerm(store.terms, "ChIP-seq") || containsTestTerm(store.terms, "数据") {
		t.Fatalf("candidate terms = %v, want planner terms without generic data term", store.terms)
	}
	if len(classifier.seen) != 2 {
		t.Fatalf("classifier calls = %d, want 2", len(classifier.seen))
	}
	if len(res.Cards) != 1 {
		t.Fatalf("cards = %+v, want only classifier-approved hit", res.Cards)
	}
	card, ok := res.Cards[0].Payload.(PaperHitCard)
	if !ok || card.PaperID != 1 || !strings.Contains(card.Reason, "Sub-Agent") {
		t.Fatalf("card = %+v, want Sub-Agent accepted paper 1", res.Cards[0].Payload)
	}
	labels := make([]string, 0, len(res.Process.Stages))
	for _, stage := range res.Process.Stages {
		labels = append(labels, stage.Label)
	}
	for _, want := range []string{"Master规划", "全文扫描", "Sub-Agent判定", "命中"} {
		if !containsTestTerm(labels, want) {
			t.Fatalf("process labels = %v, missing %s", labels, want)
		}
	}
}

func TestLibrarySearchToolReportsCandidateListerFailure(t *testing.T) {
	tool := NewLibrarySearchTool(stubPaperStore{err: errors.New("candidate query failed")})

	res, err := tool.Run(context.Background(), ToolInput{Query: "ATAC"})
	if err != nil {
		t.Fatalf("Run error = %v, want nil", err)
	}
	if res.Process.Intent != IntentLibrarySearch {
		t.Fatalf("process intent = %q", res.Process.Intent)
	}
	if len(res.Process.Stages) == 0 || res.Process.Stages[0].Label != "全文扫描" || res.Process.Stages[0].Status != "failed" {
		t.Fatalf("process = %+v", res.Process)
	}
	if len(res.ToolCalls) != 1 || res.ToolCalls[0].ToolName != "library_search" || res.ToolCalls[0].Status != "failed" {
		t.Fatalf("tool calls = %+v", res.ToolCalls)
	}
	if !strings.Contains(res.ToolCalls[0].Error, "candidate query failed") {
		t.Fatalf("tool call error = %q", res.ToolCalls[0].Error)
	}
	if !strings.Contains(res.Process.Note, "全文扫描失败") || !strings.Contains(res.AnswerContext, "全文扫描失败") {
		t.Fatalf("process note=%q answer context=%q, want explicit scan failure", res.Process.Note, res.AnswerContext)
	}
	if len(res.Cards) != 0 || len(res.Citations) != 0 {
		t.Fatalf("cards=%+v citations=%+v, want empty", res.Cards, res.Citations)
	}
}

func TestLibrarySearchToolReportsClassifierFailure(t *testing.T) {
	store := &planningPaperStore{
		papers: map[int64]*model.Paper{
			1: {ID: 1, Title: "ChIP-seq Paper", PDFText: "This study uses H3K27ac ChIP-seq data."},
			2: {ID: 2, Title: "Second ChIP-seq Paper", PDFText: "This study uses H3K4me3 ChIP-seq data."},
		},
	}
	planner := &stubLibraryPlanner{plan: LibrarySearchPlan{SearchTerms: []string{"ChIP-seq"}}}
	classifier := &stubLibraryClassifier{err: errors.New("classifier unavailable")}
	tool := NewLibrarySearchToolWithAgents(store, planner, classifier)

	res, err := tool.Run(context.Background(), ToolInput{Query: "查找包括 ChIP-seq 数据的文章"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Process.Note, "Sub-Agent判定失败") {
		t.Fatalf("process note = %q, want classifier failure note", res.Process.Note)
	}
	if got := stageByLabel(res.Process.Stages, "Sub-Agent判定").Detail; !strings.Contains(got, "失败 2篇") {
		t.Fatalf("Sub-Agent stage detail = %q, want failed classifier count", got)
	}
	if len(res.Cards) != 2 {
		t.Fatalf("cards = %+v, want fallback to local full-text hits", res.Cards)
	}
}

func containsTestTerm(terms []string, want string) bool {
	for _, term := range terms {
		if term == want {
			return true
		}
	}
	return false
}

func containsTestTermFold(terms []string, want string) bool {
	want = strings.ToLower(want)
	for _, term := range terms {
		if strings.ToLower(term) == want {
			return true
		}
	}
	return false
}

func TestLibrarySearchToolClampsLargeLimit(t *testing.T) {
	store := stubPaperStore{
		papers: make(map[int64]*model.Paper),
	}
	for i := int64(1); i <= 60; i++ {
		store.ids = append(store.ids, i)
		store.papers[i] = &model.Paper{
			ID:      i,
			Title:   fmt.Sprintf("Paper %d", i),
			PDFText: "This paper discusses chromatin accessibility.",
		}
	}
	tool := NewLibrarySearchTool(store)

	res, err := tool.Run(context.Background(), ToolInput{Query: "ATAC", Limit: 1000})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Cards) != 50 {
		t.Fatalf("card count = %d, want 50", len(res.Cards))
	}
	if len(res.Citations) != 50 {
		t.Fatalf("citation count = %d, want 50", len(res.Citations))
	}
	if len(res.Process.Stages) < 2 || res.Process.Stages[1].Count != 50 {
		t.Fatalf("process = %+v", res.Process)
	}
}
