package handler

import (
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/xuzhougeng/citebox/internal/model"
)

func TestParseFigureTagFilterAcceptsRepeatedAndCommaSeparatedIDs(t *testing.T) {
	cases := []struct {
		name     string
		query    string
		wantIDs  []int64
		wantMode string
	}{
		{name: "absent", query: "", wantIDs: nil, wantMode: model.FigureTagModeAll},
		{name: "single", query: "tag_id=7", wantIDs: []int64{7}, wantMode: model.FigureTagModeAll},
		{name: "repeated", query: "tag_id=7&tag_id=9", wantIDs: []int64{7, 9}, wantMode: model.FigureTagModeAll},
		{name: "comma", query: "tag_id=7,9", wantIDs: []int64{7, 9}, wantMode: model.FigureTagModeAll},
		{name: "mixed and deduped", query: "tag_id=7,9&tag_id=9&tag_id=3", wantIDs: []int64{7, 9, 3}, wantMode: model.FigureTagModeAll},
		{name: "blank parts ignored", query: "tag_id=&tag_id=7,,", wantIDs: []int64{7}, wantMode: model.FigureTagModeAll},
		{name: "any mode", query: "tag_id=7,9&tag_mode=ANY", wantIDs: []int64{7, 9}, wantMode: model.FigureTagModeAny},
		{name: "explicit all", query: "tag_id=7&tag_mode=all", wantIDs: []int64{7}, wantMode: model.FigureTagModeAll},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			q, err := url.ParseQuery(tc.query)
			if err != nil {
				t.Fatalf("ParseQuery() error = %v", err)
			}
			ids, mode, err := parseFigureTagFilter(q)
			if err != nil {
				t.Fatalf("parseFigureTagFilter() error = %v", err)
			}
			if !reflect.DeepEqual(ids, tc.wantIDs) {
				t.Fatalf("ids = %v, want %v", ids, tc.wantIDs)
			}
			if mode != tc.wantMode {
				t.Fatalf("mode = %q, want %q", mode, tc.wantMode)
			}
		})
	}
}

func TestParseFigureTagFilterRejectsInvalidInput(t *testing.T) {
	tooMany := make([]string, model.MaxFigureFilterTags+1)
	for i := range tooMany {
		tooMany[i] = strconv.Itoa(i + 1)
	}
	for _, query := range []string{
		"tag_id=abc",
		"tag_id=7,abc",
		"tag_id=0",
		"tag_id=-3",
		"tag_id=7&tag_mode=xor",
		"tag_id=" + strings.Join(tooMany, ","),
	} {
		q, err := url.ParseQuery(query)
		if err != nil {
			t.Fatalf("ParseQuery(%q) error = %v", query, err)
		}
		if _, _, err := parseFigureTagFilter(q); err == nil {
			t.Fatalf("parseFigureTagFilter(%q) error = nil, want error", query)
		}
	}
}
