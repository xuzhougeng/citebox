package service

import "testing"

func TestMergePaperDOIMetadataPrefersExistingFields(t *testing.T) {
	merged := mergePaperDOIMetadata(
		paperDOIMetadata{
			Title:       "Primary Title",
			AuthorsText: "Ada Lovelace",
		},
		paperDOIMetadata{
			Title:        "Fallback Title",
			AbstractText: "Fallback abstract",
			AuthorsText:  "Alan Turing",
			Journal:      "Nature Communications",
			PublishedAt:  "2023-01-18",
		},
	)

	if merged.Title != "Primary Title" {
		t.Fatalf("merged title = %q, want primary title", merged.Title)
	}
	if merged.AbstractText != "Fallback abstract" || merged.Journal != "Nature Communications" || merged.PublishedAt != "2023-01-18" {
		t.Fatalf("merged metadata = %+v, want fallback values filled", merged)
	}
	if merged.AuthorsText != "Ada Lovelace" {
		t.Fatalf("merged authors = %q, want primary authors", merged.AuthorsText)
	}
}

func TestCleanMetadataAbstractStripsJATSTags(t *testing.T) {
	abstract := cleanMetadataAbstract("<jats:title>Abstract</jats:title><jats:p>First &amp; second.</jats:p><jats:p>Third.</jats:p>")
	want := "Abstract\nFirst & second.\n\nThird."
	if abstract != want {
		t.Fatalf("cleanMetadataAbstract() = %q, want %q", abstract, want)
	}
}

func TestFormatCrossrefDatePartsSupportsPartialDates(t *testing.T) {
	cases := []struct {
		name  string
		input crossrefDateParts
		want  string
	}{
		{name: "year", input: crossrefDateParts{DateParts: [][]int{{2023}}}, want: "2023"},
		{name: "year month", input: crossrefDateParts{DateParts: [][]int{{2023, 1}}}, want: "2023-01"},
		{name: "full date", input: crossrefDateParts{DateParts: [][]int{{2023, 1, 18}}}, want: "2023-01-18"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatCrossrefDateParts(tc.input); got != tc.want {
				t.Fatalf("formatCrossrefDateParts() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestJoinCrossrefAuthorsUsesAvailableNames(t *testing.T) {
	got := joinCrossrefAuthors([]crossrefAuthor{
		{Given: "Ada", Family: "Lovelace"},
		{Name: "Alan Turing"},
		{Family: "Hopper"},
	})
	want := "Ada Lovelace, Alan Turing, Hopper"
	if got != want {
		t.Fatalf("joinCrossrefAuthors() = %q, want %q", got, want)
	}
}
