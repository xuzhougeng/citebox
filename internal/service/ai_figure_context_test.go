package service

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFigureContextResolvesImagesAndSummaries(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	res, err := repo.DB().Exec(
		`INSERT INTO papers (title, original_filename, stored_pdf_name) VALUES ('Ctx Paper', 'ctx.pdf', 'ctx.pdf')`)
	if err != nil {
		t.Fatalf("insert paper: %v", err)
	}
	paperID, _ := res.LastInsertId()

	if err := os.MkdirAll(cfg.FiguresDir(), 0o755); err != nil {
		t.Fatalf("MkdirAll figures dir: %v", err)
	}
	figIDs := make([]int64, 0, 2)
	for i, name := range []string{"ctx_fig_a.png", "ctx_fig_b.png"} {
		writeFigureFixture(t, filepath.Join(cfg.FiguresDir(), name), 32, 32)
		figRes, err := repo.DB().Exec(
			`INSERT INTO paper_figures (paper_id, filename, content_type, page_number, figure_index, caption)
			 VALUES (?, ?, 'image/png', ?, ?, ?)`,
			paperID, name, i+1, i+1, "caption "+name)
		if err != nil {
			t.Fatalf("insert figure %s: %v", name, err)
		}
		id, _ := figRes.LastInsertId()
		figIDs = append(figIDs, id)
	}

	images, summaries, err := aiSvc.LoadFigureContext(context.Background(),
		[]int64{figIDs[0], figIDs[1], figIDs[0], 99999})
	if err != nil {
		t.Fatalf("LoadFigureContext: %v", err)
	}
	if len(images) != 2 {
		t.Fatalf("images = %d, want 2 (duplicate + missing skipped)", len(images))
	}
	for i, img := range images {
		if img.MIMEType == "" || img.Data == "" {
			t.Fatalf("image %d incomplete: %+v", i, img)
		}
		if _, err := base64.StdEncoding.DecodeString(img.Data); err != nil {
			t.Fatalf("image %d data not base64: %v", i, err)
		}
	}
	if len(summaries) != 2 || !strings.Contains(summaries[0], "figure_id=") || !strings.Contains(summaries[0], "caption ctx_fig_a.png") {
		t.Fatalf("summaries = %+v", summaries)
	}
}

func TestLoadFigureContextEmptyInput(t *testing.T) {
	_, repo, cfg := newTestService(t)
	aiSvc := NewAIService(repo, cfg, nil)

	images, summaries, err := aiSvc.LoadFigureContext(context.Background(), nil)
	if err != nil || images != nil || summaries != nil {
		t.Fatalf("empty input = %v, %v, %v", images, summaries, err)
	}
}
