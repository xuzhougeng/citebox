package integration

import (
	"strconv"
	"testing"
	"time"

	"github.com/xuzhougeng/citebox/internal/apperr"
)

func TestSourceIDFormatParseRoundTrip(t *testing.T) {
	cases := []struct {
		sourceID string
		want     SourceRef
	}{
		{PaperSourceID(42), SourceRef{Kind: EntityTypePaper, ID: 42}},
		{FigureSourceID(123), SourceRef{Kind: EntityTypeFigure, ID: 123}},
		{AnnotationSourceID(91), SourceRef{Kind: EntityTypeAnnotation, ID: 91}},
		{PaperNoteSourceID(42), SourceRef{Kind: EntityTypeNote, ID: 42, NoteParent: EntityTypePaper, NoteName: "main"}},
		{FigureNoteSourceID(123), SourceRef{Kind: EntityTypeNote, ID: 123, NoteParent: EntityTypeFigure, NoteName: "main"}},
	}
	for _, tc := range cases {
		if tc.want.Kind != EntityTypeNote && tc.sourceID != "citebox:"+tc.want.Kind+":"+strconv.FormatInt(tc.want.ID, 10) {
			t.Fatalf("unexpected source id format %q", tc.sourceID)
		}
		ref, err := ParseSourceID(tc.sourceID)
		if err != nil {
			t.Fatalf("ParseSourceID(%q) error = %v", tc.sourceID, err)
		}
		if ref != tc.want {
			t.Fatalf("ParseSourceID(%q) = %+v, want %+v", tc.sourceID, ref, tc.want)
		}
		if got := ref.String(); got != tc.sourceID {
			t.Fatalf("SourceRef.String() = %q, want %q", got, tc.sourceID)
		}
	}

	// 显式校验规格中的字面量格式
	literals := map[string]SourceRef{
		"citebox:paper:42":             {Kind: EntityTypePaper, ID: 42},
		"citebox:figure:123":           {Kind: EntityTypeFigure, ID: 123},
		"citebox:annotation:91":        {Kind: EntityTypeAnnotation, ID: 91},
		"citebox:note:paper:42:main":   {Kind: EntityTypeNote, ID: 42, NoteParent: EntityTypePaper, NoteName: "main"},
		"citebox:note:figure:123:main": {Kind: EntityTypeNote, ID: 123, NoteParent: EntityTypeFigure, NoteName: "main"},
	}
	for literal, want := range literals {
		ref, err := ParseSourceID(literal)
		if err != nil {
			t.Fatalf("ParseSourceID(%q) error = %v", literal, err)
		}
		if ref != want {
			t.Fatalf("ParseSourceID(%q) = %+v, want %+v", literal, ref, want)
		}
	}
}

func TestParseSourceIDMalformed(t *testing.T) {
	for _, raw := range []string{
		"",
		"paper:42",
		"citebox:paper",
		"citebox:paper:",
		"citebox:paper:abc",
		"citebox:paper:-1",
		"citebox:paper:0",
		"citebox:paper:42:extra",
		"citebox:unknown:42",
		"citebox:note:paper:42",
		"citebox:note:unknown:42:main",
		"citebox:note:paper:abc:main",
		"citebox:note:paper:42:",
		"other:paper:42",
	} {
		if _, err := ParseSourceID(raw); !apperr.IsCode(err, apperr.CodeInvalidArgument) {
			t.Fatalf("ParseSourceID(%q) code = %q, want %q", raw, apperr.CodeOf(err), apperr.CodeInvalidArgument)
		}
	}
}

func TestSourceRefDeepLink(t *testing.T) {
	cases := map[string]string{
		"citebox:paper:42":             "citebox://paper/42",
		"citebox:figure:123":           "citebox://figure/123",
		"citebox:annotation:91":        "citebox://annotation/91",
		"citebox:note:paper:42:main":   "citebox://paper/42",
		"citebox:note:figure:123:main": "citebox://figure/123",
	}
	for sourceID, want := range cases {
		ref, err := ParseSourceID(sourceID)
		if err != nil {
			t.Fatalf("ParseSourceID(%q) error = %v", sourceID, err)
		}
		if got := ref.DeepLink(); got != want {
			t.Fatalf("DeepLink(%q) = %q, want %q", sourceID, got, want)
		}
	}
}

func TestNewEnvelope(t *testing.T) {
	updatedAt := time.Date(2026, 2, 3, 4, 5, 6, 0, time.FixedZone("UTC+8", 8*3600))
	ref := SourceRef{Kind: EntityTypePaper, ID: 42}
	envelope := NewEnvelope(ref, updatedAt, map[string]any{"title": "x"})

	if envelope.SchemaVersion != ResearchContextSchema {
		t.Fatalf("SchemaVersion = %q, want %q", envelope.SchemaVersion, ResearchContextSchema)
	}
	if envelope.SourceID != "citebox:paper:42" || envelope.EntityType != EntityTypePaper {
		t.Fatalf("envelope source = %q type = %q", envelope.SourceID, envelope.EntityType)
	}
	// revision 统一为 UTC RFC3339
	if envelope.Revision != "2026-02-02T20:05:06Z" {
		t.Fatalf("Revision = %q, want 2026-02-02T20:05:06Z", envelope.Revision)
	}
	if len(envelope.Permissions) != 1 || envelope.Permissions[0] != "read" {
		t.Fatalf("Permissions = %v, want [read]", envelope.Permissions)
	}
	if envelope.DeepLink != "citebox://paper/42" {
		t.Fatalf("DeepLink = %q, want citebox://paper/42", envelope.DeepLink)
	}
}
