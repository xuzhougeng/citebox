package handler

import (
	"encoding/json"
	"github.com/xuzhougeng/citebox/internal/config"
	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
	"github.com/xuzhougeng/citebox/internal/service"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFigureJobHTTPFlowPersistsCompletedInterpretation(t *testing.T) {
	root := t.TempDir()
	cfg := &config.Config{StorageDir: root}
	repo, err := repository.NewLibraryRepository(filepath.Join(root, "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer repo.Close()
	ai := service.NewAIService(repo, cfg, nil)
	defer ai.Close()
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"output_text":"Interpretation from mock provider."}`))
	}))
	defer provider.Close()
	yes := true
	_, err = ai.UpdateSettings(model.AISettings{Models: []model.AIModelConfig{{ID: "figure", Name: "Figure", Provider: model.AIProviderOpenAI, BaseURL: provider.URL, APIKey: "test", Model: "test", SupportsImages: &yes}}, SceneModels: model.AISceneModelSelection{DefaultModelID: "figure", FigureModelID: "figure"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cfg.FiguresDir(), 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(cfg.FiguresDir(), "figure.png"))
	if err != nil {
		t.Fatal(err)
	}
	png.Encode(f, image.NewRGBA(image.Rect(0, 0, 20, 20)))
	f.Close()
	p, err := repo.CreatePaper(repository.PaperUpsertInput{Title: "Study", OriginalFilename: "study.pdf", StoredPDFName: "study.pdf", ExtractionStatus: "completed", Figures: []repository.FigureUpsertInput{{Filename: "figure.png", ContentType: "image/png", FigureIndex: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	h := NewAIHandler(ai)
	input, _ := json.Marshal(model.AIFigureJobRequest{PaperID: p.ID, Scope: "all", Mode: "append", Language: "en"})
	w := httptest.NewRecorder()
	h.FigureJobs(w, httptest.NewRequest(http.MethodPost, "/api/ai/figure-jobs", strings.NewReader(string(input))))
	if w.Code != http.StatusAccepted {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var payload struct {
		Job model.AIFigureJob `json:"job"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := ai.GetFigureAIJob(payload.Job.ID)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status == "completed" {
			paper, _ := repo.GetPaperDetail(p.ID)
			if !strings.Contains(paper.Figures[0].NotesText, "Interpretation from mock provider") {
				t.Fatal(paper.Figures[0].NotesText)
			}
			return
		}
		if job.Status == "failed" {
			t.Fatalf("job failed: %+v", job)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("job did not complete")
}
