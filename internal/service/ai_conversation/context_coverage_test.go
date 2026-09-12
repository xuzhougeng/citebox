package ai_conversation

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service/ai_assistant"
)

func TestPinnedContextReachesLateMethodsWithinBudget(t *testing.T) {
	paper := model.Paper{ID: 1, Title: "Long paper", AbstractText: "Abstract", PDFText: strings.Repeat("Front matter ", 4000) + "\nSTAR METHODS\nTAIL_METHODS_SENTINEL\n" +
		strings.Repeat("protocol details ", 2000) + "\nData and code availability\nTAIL_CODE_SENTINEL"}
	for _, budget := range []int{1000, 3000, 32000} {
		block, usage := budgetedPinnedPaperBlock(paper, budget)
		if estimateTokens(block) > budget {
			t.Fatalf("budget %d exceeded", budget)
		}
		if !strings.Contains(block, "TAIL_METHODS_SENTINEL") || !strings.Contains(block, "TAIL_CODE_SENTINEL") {
			t.Fatalf("late sections omitted at budget %d", budget)
		}
		if usage.TotalBodyRunes != len([]rune(paper.PDFText)) || usage.IncludedBodyRunes > maxPinnedBodyRunes || usage.ExcerptCount < 2 {
			t.Fatalf("usage = %+v", usage)
		}
	}
}

func TestSendMessageReservesToolEvidenceAndUsesPersistentPins(t *testing.T) {
	svc, repo, caller := newServiceForTest(t)
	settings := model.DefaultAISettings()
	settings.ContextBudgetTokens = 4000
	svc.settings = &stubSettingsProvider{settings: settings}
	id := mustInsertPaperForTest(t, repo, "Pinned Study", "10.1/persistent")
	if _, err := repo.DB().Exec(`UPDATE papers SET pdf_text=? WHERE id=?`, strings.Repeat("Long body text. ", 4000), id); err != nil {
		t.Fatal(err)
	}
	convID, _ := svc.CreateDraft()
	if err := svc.PinPaper(convID, id); err != nil {
		t.Fatal(err)
	}
	orch := &stubOrchestrator{out: ai_assistant.RunOutput{AnswerContext: "工具结果：\n" + strings.Repeat("Evidence detail. ", 4000) + "\n\n用户问题：\nExplain design"}}
	svc.orchestrator = orch
	var usage ContextUsage
	_, err := svc.SendMessage(context.Background(), SendMessageInput{ConversationID: convID, Content: "Explain design", OnEvent: func(event StreamEvent) error {
		if event.Type == "context_usage" {
			if caller.calls != 0 {
				t.Fatal("usage emitted after provider call")
			}
			usage = event.Data.(ContextUsage)
		}
		return nil
	}}, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(orch.in.Context.PaperIDs, []int64{id}) {
		t.Fatalf("persistent pin missing: %+v", orch.in.Context)
	}
	if strings.Count(caller.userSeen, "用户问题：\n") != 1 || !strings.HasSuffix(caller.userSeen, "Explain design") {
		t.Fatal("question lost or duplicated")
	}
	if !strings.Contains(caller.userSeen, "Pinned Study") || !strings.Contains(caller.userSeen, "工具结果受本轮预算限制") {
		t.Fatal("missing pinned context or evidence disclosure")
	}
	if estimateTokens(caller.userSeen)+estimateTokens(caller.systemSeen) > settings.ContextBudgetTokens {
		t.Fatal("final prompt exceeded text budget")
	}
	if len(usage.Papers) != 1 || usage.Papers[0].IncludedBodyRunes == 0 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestStrictEvidenceOmitsPinnedBodyButDisclosesZeroCoverage(t *testing.T) {
	svc, repo, _ := newServiceForTest(t)
	id := mustInsertPaperForTest(t, repo, "Strict study", "10.1/strict-coverage")
	if _, err := repo.DB().Exec(`UPDATE papers SET pdf_text='PRIVATE_BODY' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}
	asm, err := svc.assembleWithEvidence(repository.AIConversation{StrictEvidence: true}, []repository.AIPinnedPaper{{PaperID: id}}, nil, "question", "", "evidence", model.DefaultAISettings())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(asm.userPrompt, "PRIVATE_BODY") || len(asm.papers) != 1 || asm.papers[0].IncludedBodyRunes != 0 {
		t.Fatalf("strict coverage = %+v", asm)
	}
}

func TestAutomaticFiguresAreOptInBoundedAndManualSelectionWins(t *testing.T) {
	svc, repo, _ := newServiceForTest(t)
	id := mustInsertPaperForTest(t, repo, "Figures", "10.1/auto-figures")
	var ids []int64
	for i := 0; i < 6; i++ {
		res, err := repo.DB().Exec(`INSERT INTO paper_figures (paper_id, filename, page_number, figure_index) VALUES (?, ?, ?, ?)`, id, fmt.Sprintf("fig-%d.png", i), i+1, i+1)
		if err != nil {
			t.Fatal(err)
		}
		figID, _ := res.LastInsertId()
		ids = append(ids, figID)
	}
	if _, err := repo.DB().Exec(`UPDATE paper_figures SET parent_figure_id=?, subfigure_label='a' WHERE id=?`, ids[0], ids[1]); err != nil {
		t.Fatal(err)
	}
	pinned := []repository.AIPinnedPaper{{PaperID: id}}
	if got := svc.turnFigureIDs(ai_assistant.RequestContext{}, pinned); len(got) != 0 {
		t.Fatalf("auto-attached without opt-in: %v", got)
	}
	if got := svc.turnFigureIDs(ai_assistant.RequestContext{AutoAttachFigures: true}, pinned); !reflect.DeepEqual(got, []int64{ids[0], ids[2], ids[3], ids[4]}) {
		t.Fatalf("auto selection = %v", got)
	}
	if got := svc.turnFigureIDs(ai_assistant.RequestContext{AutoAttachFigures: true, FigureIDs: []int64{ids[5], ids[5], -1}, FigureID: ids[5]}, pinned); !reflect.DeepEqual(got, []int64{ids[5]}) {
		t.Fatalf("manual selection = %v", got)
	}
}

func TestFigureInputUsageDescribesActualImagesAndFailures(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name       string
		capability *bool
		images     int
		loadErr    error
		reason     string
		want       int
	}{
		{"supported", &yes, 2, nil, "attached", 2},
		{"partial", &yes, 1, nil, "partial", 1},
		{"unknown", nil, 2, nil, "capability_unknown", 0},
		{"disabled", &no, 2, nil, "model_unsupported", 0},
		{"missing", &yes, 0, nil, "no_image_data", 0},
		{"failed", &yes, 0, errors.New("missing file"), "load_failed", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, caller := newServiceForTest(t)
			settings := model.DefaultAISettings()
			settings.Models[0].SupportsImages = tc.capability
			svc.settings = &stubSettingsProvider{settings: settings}
			loader := &stubFigureContextLoader{summaries: []string{"figure one", "figure two"}, err: tc.loadErr}
			for i := 0; i < tc.images; i++ {
				loader.images = append(loader.images, model.AIImageInput{MIMEType: "image/png", Data: "aW1n"})
			}
			svc.WithFigureContextLoader(loader)
			convID, _ := svc.CreateDraft()
			var usage ContextUsage
			res, err := svc.SendMessage(context.Background(), SendMessageInput{ConversationID: convID, Content: "explain", Context: ai_assistant.RequestContext{FigureIDs: []int64{1, 2}}, OnEvent: func(e StreamEvent) error {
				if e.Type == "context_usage" {
					usage = e.Data.(ContextUsage)
				}
				return nil
			}}, func(string) error { return nil })
			if err != nil {
				t.Fatal(err)
			}
			if len(caller.imagesSeen) != tc.want || usage.AttachedImages != tc.want || usage.ImageReason != tc.reason || res.AssistantMessage.IncludedFigures != tc.want {
				t.Fatalf("usage=%+v images=%d message=%+v", usage, len(caller.imagesSeen), res.AssistantMessage)
			}
			if tc.want == 0 && !strings.Contains(caller.userSeen, "未提供图片输入") {
				t.Fatal("missing zero-image boundary")
			}
			if tc.want == 1 && !strings.Contains(caller.userSeen, "共 1 张") {
				t.Fatal("summaries falsely counted as images")
			}
		})
	}
}

func TestImageCapabilityFollowsMasterSelection(t *testing.T) {
	yes, no := true, false
	settings := model.DefaultAISettings()
	settings.Models = []model.AIModelConfig{{ID: "vision", SupportsImages: &yes}, {ID: "text", SupportsImages: &no}}
	settings.SceneModels = model.AISceneModelSelection{DefaultModelID: "vision", QAModelID: "text"}
	if assistantMasterSupportsImages(settings) {
		t.Fatal("ignored QA fallback selection")
	}
	settings.SceneModels.AssistantMasterModelID = "vision"
	if !assistantMasterSupportsImages(settings) {
		t.Fatal("ignored explicit master selection")
	}
	settings.SceneModels.AssistantMasterModelID = "missing"
	if assistantMasterSupportsImages(settings) {
		t.Fatal("unmatched model assumed image-capable")
	}
}
