//go:build cgo && !nocgo

package service

import (
	"context"
	"fmt"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func twoPageCheckpointPDF() []byte {
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R 4 0 R] /Count 2 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 144 144] /Resources << >> /Contents 5 0 R >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 144 144] /Resources << >> /Contents 6 0 R >>",
		"<< /Length 0 >>\nstream\n\nendstream",
		"<< /Length 0 >>\nstream\n\nendstream",
	}
	var b strings.Builder
	b.WriteString("%PDF-1.4\n")
	offsets := []int{0}
	for i, obj := range objects {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", offset)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return []byte(b.String())
}

func TestBuiltInExtractionRetryReusesCompletedPages(t *testing.T) {
	svc, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)
	svc.aiService = aiSvc
	defer aiSvc.Close()
	var first, second atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(body), "Page number: 1") {
			first.Add(1)
			io.WriteString(w, `{"output_text":"{\"figures\":[{\"bbox\":[100,100,500,500]}]}"}`)
			return
		}
		if second.Add(1) == 1 {
			http.Error(w, "temporary page failure", http.StatusBadGateway)
			return
		}
		io.WriteString(w, `{"output_text":"{\"figures\":[]}"}`)
	}))
	defer server.Close()
	_, err := aiSvc.UpdateSettings(model.AISettings{Models: []model.AIModelConfig{{ID: "figure", Name: "Figure", Provider: model.AIProviderOpenAI, APIKey: "test-key", BaseURL: server.URL, Model: "test", MaxOutputTokens: 1200}}, SceneModels: model.AISceneModelSelection{DefaultModelID: "figure", FigureModelID: "figure"}})
	if err != nil {
		t.Fatal(err)
	}
	p, err := repo.CreatePaper(repository.PaperUpsertInput{Title: "Study", OriginalFilename: "two.pdf", StoredPDFName: "two.pdf", ExtractionStatus: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	pdfPath := filepath.Join(cfg.PapersDir(), "two.pdf")
	if err := os.WriteFile(pdfPath, twoPageCheckpointPDF(), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.extractBuiltInLLMResult(context.Background(), p.ID, pdfPath, "two.pdf"); err == nil {
		t.Fatal("expected second-page failure")
	}
	result, err := svc.extractBuiltInLLMResult(context.Background(), p.ID, pdfPath, "two.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if first.Load() != 1 || second.Load() != 2 || len(result.Figures) != 1 {
		t.Fatalf("first=%d second=%d figures=%d", first.Load(), second.Load(), len(result.Figures))
	}
	if err := svc.processBuiltInLLMExtraction(model.ExtractorSettings{}, p.ID, pdfPath, "two.pdf"); err != nil {
		t.Fatal(err)
	}
	var count int
	repo.DB().QueryRow(`SELECT count(*) FROM extraction_page_checkpoints WHERE paper_id=?`, p.ID).Scan(&count)
	if count != 0 {
		t.Fatal("successful extraction left checkpoint payloads")
	}
}
