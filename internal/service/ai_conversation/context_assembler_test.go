package ai_conversation

import (
	"fmt"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

func TestAssembleForTurnIncludesFullAbstractAndTruncationHint(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)

	paperID := mustInsertPaperForTest(t, libRepo, "Spatial Omni Paper", "10.1/spatial")
	abstract := strings.Repeat("摘要句子。", 400) // 2000 runes, well above the old 800 cap
	longTail := strings.Repeat("RESULTS AND METHODS tail ", 7000)
	pdfText := "INTRODUCTION head " + longTail
	if _, err := libRepo.DB().Exec(
		`UPDATE papers SET abstract_text = ?, pdf_text = ? WHERE id = ?`,
		abstract, pdfText, paperID); err != nil {
		t.Fatalf("update paper text: %v", err)
	}

	conv := repository.AIConversation{StrictEvidence: false}
	pinned := []repository.AIPinnedPaper{{PaperID: paperID, Title: "Spatial Omni Paper"}}

	asm, err := svc.assembleForTurn(conv, pinned, nil, "这篇论文的图注说了什么？", "", model.AISettings{})
	if err != nil {
		t.Fatalf("assembleForTurn: %v", err)
	}

	if !strings.Contains(asm.userPrompt, abstract) {
		t.Fatalf("pinned block should include the full abstract")
	}
	if !strings.Contains(asm.userPrompt, "非完整全文") {
		t.Fatalf("pinned block should tell the model the body is truncated: %s", asm.userPrompt)
	}
	if !strings.Contains(asm.userPrompt, "文献检索工具") {
		t.Fatalf("pinned block should point the model at retrieval tools")
	}
}

func TestAssembleForTurnKeepsBodyWholeWithinBudget(t *testing.T) {
	svc, libRepo, _ := newServiceForTest(t)

	paperID := mustInsertPaperForTest(t, libRepo, "Short Body Paper", "10.1/short")
	pdfText := strings.Repeat("short body ", 200) // Short body fits comfortably within the turn budget
	if _, err := libRepo.DB().Exec(
		`UPDATE papers SET pdf_text = ? WHERE id = ?`, pdfText, paperID); err != nil {
		t.Fatalf("update paper text: %v", err)
	}

	conv := repository.AIConversation{StrictEvidence: false}
	pinned := []repository.AIPinnedPaper{{PaperID: paperID, Title: "Short Body Paper"}}

	asm, err := svc.assembleForTurn(conv, pinned, nil, "总结方法", "", model.AISettings{})
	if err != nil {
		t.Fatalf("assembleForTurn: %v", err)
	}

	if strings.Contains(asm.userPrompt, "非完整全文") {
		t.Fatalf("short body should not carry the truncation hint")
	}
	if !strings.Contains(asm.userPrompt, strings.TrimRight(pdfText, " ")) {
		t.Fatalf("pinned block should include the whole body within the budget")
	}
}

func TestAssembleForTurnFitsPinnedPapersWithinRemainingBudget(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		count  int
		budget int
	}{
		{"single_english", strings.Repeat("Full body text. ", 2200), 1, 4000},
		{"multiple_english", strings.Repeat("Full body text. ", 2200), 3, 8000},
		{"multiple_chinese", strings.Repeat("正文方法结果。", 5000), 3, 8000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _ := newServiceForTest(t)
			var pinned []repository.AIPinnedPaper
			for i := 0; i < tc.count; i++ {
				title := fmt.Sprintf("Budget Paper %d", i)
				id := mustInsertPaperForTest(t, repo, title, fmt.Sprintf("10.1/budget-%d", i))
				if _, err := repo.DB().Exec(`UPDATE papers SET abstract_text=?, pdf_text=? WHERE id=?`, strings.Repeat("Abstract text. ", 300), tc.body, id); err != nil {
					t.Fatal(err)
				}
				pinned = append(pinned, repository.AIPinnedPaper{PaperID: id, Title: title})
			}
			conv := repository.AIConversation{SummaryText: strings.Repeat("Earlier summary. ", 50)}
			settings := model.AISettings{ContextBudgetTokens: tc.budget, SystemPrompt: "Read the evidence."}
			attachment := "Attached excerpt: " + strings.Repeat("Excerpt text. ", 50)
			asm, err := svc.assembleForTurn(conv, pinned, []repository.AIMessage{{Role: "user", Content: strings.Repeat("Older message. ", 800)}}, "Summarize", attachment, settings)
			if err != nil {
				t.Fatal(err)
			}
			if got := estimateTokens(asm.systemPrompt) + estimateTokens(asm.userPrompt); got > tc.budget {
				t.Fatalf("estimated tokens=%d, budget=%d", got, tc.budget)
			}
			for _, pp := range pinned {
				if !strings.Contains(asm.userPrompt, pp.Title) {
					t.Fatalf("missing pinned paper %q", pp.Title)
				}
			}
			if !strings.Contains(asm.userPrompt, "文献检索工具") || !strings.Contains(asm.userPrompt, "已带入 ") {
				t.Fatal("truncated text must disclose its extent and retrieval path")
			}
			if !strings.Contains(asm.userPrompt, conv.SummaryText) || !strings.Contains(asm.userPrompt, attachment) {
				t.Fatal("mandatory context was dropped to make space for pinned papers")
			}
		})
	}
}

func TestAssembleForTurnOmitsPinnedTextWhenMandatoryInputUsesBudget(t *testing.T) {
	svc, repo, _ := newServiceForTest(t)
	id := mustInsertPaperForTest(t, repo, "Optional Paper", "10.1/optional")
	asm, err := svc.assembleForTurn(repository.AIConversation{}, []repository.AIPinnedPaper{{PaperID: id}}, nil, strings.Repeat("Question ", 1000), "", model.AISettings{ContextBudgetTokens: 2000})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(asm.userPrompt, "Optional Paper") {
		t.Fatal("optional pinned block exceeded the remaining budget")
	}
}
