package repository

import (
	"reflect"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

func TestListFiguresFiltersByMultipleTags(t *testing.T) {
	repo := newTestRepository(t)

	paper, err := repo.CreatePaper(PaperUpsertInput{
		Title:            "Multi Tag Figures",
		OriginalFilename: "multi-tag.pdf",
		StoredPDFName:    "paper_multi_tag.pdf",
		FileSize:         256,
		ContentType:      "application/pdf",
		ExtractionStatus: "completed",
		Figures: []FigureUpsertInput{
			{Filename: "both.png", PageNumber: 1, FigureIndex: 1, Caption: "Both"},
			{Filename: "chart_only.png", PageNumber: 2, FigureIndex: 2, Caption: "Chart only"},
			{Filename: "method_only.png", PageNumber: 3, FigureIndex: 3, Caption: "Method only"},
			{Filename: "untagged.png", PageNumber: 4, FigureIndex: 4, Caption: "Untagged"},
		},
	})
	if err != nil {
		t.Fatalf("CreatePaper() error = %v", err)
	}
	both, chartOnly, methodOnly := paper.Figures[0].ID, paper.Figures[1].ID, paper.Figures[2].ID

	for figureID, names := range map[int64][]string{
		both:       {"stacked-bar", "diff-abundance", "extra"},
		chartOnly:  {"stacked-bar"},
		methodOnly: {"diff-abundance"},
	} {
		inputs := make([]TagUpsertInput, 0, len(names))
		for _, name := range names {
			inputs = append(inputs, TagUpsertInput{Name: name, Color: "#336699"})
		}
		if _, err := repo.UpdateFigureTags(figureID, inputs); err != nil {
			t.Fatalf("UpdateFigureTags(%d) error = %v", figureID, err)
		}
	}

	tags, err := repo.ListTags(model.TagScopeFigure)
	if err != nil {
		t.Fatalf("ListTags() error = %v", err)
	}
	tagID := map[string]int64{}
	for _, tag := range tags {
		tagID[tag.Name] = tag.ID
	}
	chart, method := tagID["stacked-bar"], tagID["diff-abundance"]
	if chart == 0 || method == 0 {
		t.Fatalf("ListTags() missing expected tags: %+v", tags)
	}

	figureIDs := func(filter model.FigureFilter) []int64 {
		t.Helper()
		filter.SortBy = "paper_created_at_figure_index"
		figures, total, err := repo.ListFigures(filter)
		if err != nil {
			t.Fatalf("ListFigures(%+v) error = %v", filter, err)
		}
		if total != len(figures) {
			t.Fatalf("ListFigures(%+v) total = %d, len = %d", filter, total, len(figures))
		}
		result := make([]int64, 0, len(figures))
		for _, figure := range figures {
			result = append(result, figure.ID)
		}
		return result
	}

	cases := []struct {
		name   string
		filter model.FigureFilter
		want   []int64
	}{
		{name: "default mode is intersection", filter: model.FigureFilter{TagIDs: []int64{chart, method}}, want: []int64{both}},
		{name: "explicit all", filter: model.FigureFilter{TagIDs: []int64{chart, method}, TagMode: model.FigureTagModeAll}, want: []int64{both}},
		{name: "duplicates do not break all", filter: model.FigureFilter{TagIDs: []int64{chart, chart, method}}, want: []int64{both}},
		{name: "any is union", filter: model.FigureFilter{TagIDs: []int64{chart, method}, TagMode: model.FigureTagModeAny}, want: []int64{both, chartOnly, methodOnly}},
		{name: "single tag", filter: model.FigureFilter{TagIDs: []int64{chart}}, want: []int64{both, chartOnly}},
		{name: "unknown tag under all matches nothing", filter: model.FigureFilter{TagIDs: []int64{chart, 999999}}, want: []int64{}},
		{name: "combines with other filters", filter: model.FigureFilter{TagIDs: []int64{chart, method}, TagMode: model.FigureTagModeAny, Keyword: "Chart only"}, want: []int64{chartOnly}},
	}
	for _, tc := range cases {
		if got := figureIDs(tc.filter); !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s: figure ids = %v, want %v", tc.name, got, tc.want)
		}
	}
}
