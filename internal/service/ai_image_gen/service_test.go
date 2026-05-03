package ai_image_gen

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/model"
	"github.com/xuzhougeng/citebox/internal/repository"
)

type fakeVision struct {
	prompt string
	err    error
}

func (f *fakeVision) CallVision(ctx context.Context, _ model.AISettings, sys, user string,
	_ []model.AIImageInput) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.prompt, nil
}

type fakeAPI struct {
	bytes []byte
	err   error
}

func (f *fakeAPI) Generate(_ context.Context, _ model.AIImageGenSettings, _ string) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.bytes, nil
}

type fakeInputs struct {
	papers  []PaperContext
	figures []FigureContext
	images  []model.AIImageInput
}

func (f *fakeInputs) Load(ctx context.Context, paperIDs, figureIDs []int64) (VisionInputContext, []model.AIImageInput, error) {
	return VisionInputContext{Papers: f.papers, IsolatedFigures: f.figures}, f.images, nil
}

type captureRepo struct {
	mu        sync.Mutex
	rows      []repository.AIGeneratedImage
	cards     []repository.AIResultCard
	cardErr   error
	deleteErr error
}

func (c *captureRepo) InsertImage(img repository.AIGeneratedImage) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	img.ID = int64(len(c.rows) + 1)
	c.rows = append(c.rows, img)
	return img.ID, nil
}

func (c *captureRepo) AddResultCard(card repository.AIResultCard) (int64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cardErr != nil {
		return 0, c.cardErr
	}
	card.ID = int64(len(c.cards) + 1)
	c.cards = append(c.cards, card)
	return card.ID, nil
}

func (c *captureRepo) DeleteImage(id int64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.deleteErr != nil {
		return c.deleteErr
	}
	out := c.rows[:0]
	for _, r := range c.rows {
		if r.ID != id {
			out = append(out, r)
		}
	}
	c.rows = out
	return nil
}

func newServiceWithFakes(t *testing.T, vision VisionCaller, api APIClient) (*Service, *captureRepo, string) {
	t.Helper()
	tmp := t.TempDir()
	repo := &captureRepo{}
	settingsEnabled := func() (model.AIImageGenSettings, model.AISettings, error) {
		gs := model.DefaultAISettings()
		gs.ImageGen.Enabled = true
		gs.ImageGen.APIKey = "sk-test"
		return gs.ImageGen, gs, nil
	}
	svc := NewService(ServiceDeps{
		Repo:        repo,
		Storage:     NewStorage(tmp),
		Vision:      vision,
		API:         api,
		LoadInputs:  (&fakeInputs{papers: []PaperContext{{ID: 1, Title: "P", Abstract: "A"}}}).Load,
		GetSettings: settingsEnabled,
	})
	return svc, repo, tmp
}

func TestNewService_DefaultImageTimeoutExceedsThreeMinutes(t *testing.T) {
	svc := NewService(ServiceDeps{})
	if svc.deps.ImageTimeout <= 180*time.Second {
		t.Fatalf("ImageTimeout = %s, want > 180s", svc.deps.ImageTimeout)
	}
}

func TestService_HappyPath_EmitsAllEventsAndPersists(t *testing.T) {
	vision := &fakeVision{prompt: "draw an abstract showing X"}
	api := &fakeAPI{bytes: []byte("\x89PNGfake")}
	svc, repo, tmp := newServiceWithFakes(t, vision, api)

	var events []string
	err := svc.Generate(context.Background(), GenerateInput{
		ConversationID: 7, TurnRunID: 99,
		PaperIDs: []int64{1}, UserText: "summarize",
		OnEvent: func(e StreamEvent) error { events = append(events, e.Type); return nil },
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	wantSeq := []string{"image_prompt_drafting", "image_prompt_drafted", "image_generating", "image_generated"}
	if strings.Join(events, ",") != strings.Join(wantSeq, ",") {
		t.Fatalf("events: got %v want %v", events, wantSeq)
	}
	if len(repo.rows) != 1 || repo.rows[0].Prompt != "draw an abstract showing X" {
		t.Fatalf("row not persisted: %+v", repo.rows)
	}
	abs := filepath.Join(tmp, repo.rows[0].FilePath)
	if _, err := os.Stat(abs); err != nil {
		t.Fatalf("file not on disk: %v", err)
	}
	if len(repo.cards) != 1 || repo.cards[0].CardType != "generated_image" {
		t.Fatalf("card not persisted: %+v", repo.cards)
	}
}

func TestService_VisionFailure_DoesNotCallImageAPI(t *testing.T) {
	vision := &fakeVision{err: errors.New("vision boom")}
	api := &fakeAPI{} // would panic if called; the test asserts it isn't
	svc, repo, _ := newServiceWithFakes(t, vision, api)

	var failedStage string
	err := svc.Generate(context.Background(), GenerateInput{
		ConversationID: 7, TurnRunID: 99, PaperIDs: []int64{1}, UserText: "x",
		OnEvent: func(e StreamEvent) error {
			if e.Type == "image_failed" {
				failedStage = e.Data.(FailedEvent).Stage
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if failedStage != "vision" {
		t.Fatalf("stage = %q want vision", failedStage)
	}
	if len(repo.rows) != 0 {
		t.Fatalf("expected no DB row on vision fail")
	}
}

func TestService_ImageAPIFailure_NoFileNoRow(t *testing.T) {
	vision := &fakeVision{prompt: "x"}
	api := &fakeAPI{err: errors.New("policy reject")}
	svc, repo, tmp := newServiceWithFakes(t, vision, api)

	var failedStage string
	_ = svc.Generate(context.Background(), GenerateInput{
		ConversationID: 7, TurnRunID: 99, PaperIDs: []int64{1}, UserText: "x",
		OnEvent: func(e StreamEvent) error {
			if e.Type == "image_failed" {
				failedStage = e.Data.(FailedEvent).Stage
			}
			return nil
		},
	})
	if failedStage != "image_api" {
		t.Fatalf("stage = %q want image_api", failedStage)
	}
	if len(repo.rows) != 0 {
		t.Fatal("no DB row expected")
	}
	entries, _ := os.ReadDir(tmp)
	if len(entries) != 0 {
		t.Fatalf("no files expected, got %d", len(entries))
	}
}

func TestService_AddResultCardFailure_CleansUpFileAndRow(t *testing.T) {
	vision := &fakeVision{prompt: "x"}
	api := &fakeAPI{bytes: []byte("\x89PNGfake")}
	svc, repo, tmp := newServiceWithFakes(t, vision, api)
	repo.cardErr = errors.New("card boom")

	var failedStage string
	err := svc.Generate(context.Background(), GenerateInput{
		ConversationID: 7, TurnRunID: 99, PaperIDs: []int64{1}, UserText: "x",
		OnEvent: func(e StreamEvent) error {
			if e.Type == "image_failed" {
				failedStage = e.Data.(FailedEvent).Stage
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if failedStage != "save" {
		t.Fatalf("stage = %q want save", failedStage)
	}
	// After cleanup: no DB row, no file on disk.
	if len(repo.rows) != 0 {
		t.Fatalf("expected 0 rows after cleanup, got %d", len(repo.rows))
	}
	entries, _ := os.ReadDir(filepath.Join(tmp, "7"))
	if len(entries) != 0 {
		t.Fatalf("expected 0 files after cleanup, got %d (dir contents: %v)", len(entries), entries)
	}
}
