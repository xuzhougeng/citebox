package ai_external

import "testing"

func TestMergePapersDedupesByDOIAndMergesSources(t *testing.T) {
	in := []SourceResult{
		{Source: SourcePubMed, Papers: []Paper{{Source: SourcePubMed, SourcePaperID: "pmid-1", PMID: "1", DOI: "https://doi.org/10.1/ABC", Title: "Short", Abstract: "short"}}},
		{Source: SourceSemanticScholar, Papers: []Paper{{Source: SourceSemanticScholar, SourcePaperID: "s2-1", DOI: "10.1/abc", Title: "Short", Abstract: "a much longer abstract", Year: 2026, OnlineYear: 2025, IssueYear: 2026, YearLabel: "2025 online / 2026 issue"}}},
	}
	out := MergePapers(in, []SourceID{SourcePubMed, SourceSemanticScholar}, 10)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(out), out)
	}
	p := out[0]
	if len(p.Sources) != 2 || p.SourcePaperIDs[SourcePubMed] != "pmid-1" || p.SourcePaperIDs[SourceSemanticScholar] != "s2-1" {
		t.Fatalf("merged source metadata = %+v", p)
	}
	if p.Abstract != "a much longer abstract" {
		t.Fatalf("abstract = %q", p.Abstract)
	}
	if p.Year != 2026 || p.OnlineYear != 2025 || p.IssueYear != 2026 || p.YearLabel != "2025 online / 2026 issue" {
		t.Fatalf("year metadata = %+v", p)
	}
}

func TestMergePapersDedupesByPMID(t *testing.T) {
	out := MergePapers([]SourceResult{
		{Source: SourcePubMed, Papers: []Paper{{Source: SourcePubMed, SourcePaperID: "123", PMID: "123", Title: "A"}}},
		{Source: SourceSemanticScholar, Papers: []Paper{{Source: SourceSemanticScholar, SourcePaperID: "s2", PMID: "123", Title: "A"}}},
	}, []SourceID{SourcePubMed, SourceSemanticScholar}, 10)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
}

func TestMergePapersDerivesSourceAndMatchedQueryFromResult(t *testing.T) {
	out := MergePapers([]SourceResult{
		{Source: SourcePubMed, Query: "pub query", Papers: []Paper{{SourcePaperID: "1", Title: "A"}}},
	}, []SourceID{SourcePubMed}, 10)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(out), out)
	}
	if out[0].Source != SourcePubMed || out[0].MatchedQuery != "pub query" {
		t.Fatalf("paper metadata = %+v", out[0])
	}
}

func TestMergePapersUsesResultSourceOverInnerPaperSource(t *testing.T) {
	out := MergePapers([]SourceResult{
		{Source: SourcePubMed, Papers: []Paper{{Source: SourceSemanticScholar, SourcePaperID: "1", Title: "A"}}},
	}, []SourceID{SourcePubMed}, 10)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(out), out)
	}
	if out[0].Source != SourcePubMed || len(out[0].Sources) != 1 || out[0].Sources[0] != SourcePubMed {
		t.Fatalf("source metadata = %+v", out[0])
	}
	if out[0].SourcePaperIDs[SourcePubMed] != "1" || out[0].SourcePaperIDs[SourceSemanticScholar] != "" {
		t.Fatalf("source ids = %+v", out[0].SourcePaperIDs)
	}
}

func TestMergePapersDoesNotMutateInputSourcePaperIDs(t *testing.T) {
	sourcePaperIDs := map[SourceID]string{SourceSemanticScholar: "s2"}
	in := []SourceResult{
		{Source: SourcePubMed, Papers: []Paper{{
			Source:         SourcePubMed,
			SourcePaperID:  "pmid",
			SourcePaperIDs: sourcePaperIDs,
			Title:          "A",
		}}},
	}

	out := MergePapers(in, []SourceID{SourcePubMed}, 10)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1: %+v", len(out), out)
	}
	if sourcePaperIDs[SourcePubMed] != "" || len(sourcePaperIDs) != 1 {
		t.Fatalf("input source ids mutated = %+v", sourcePaperIDs)
	}
	if out[0].SourcePaperIDs[SourcePubMed] != "pmid" || out[0].SourcePaperIDs[SourceSemanticScholar] != "s2" {
		t.Fatalf("output source ids = %+v", out[0].SourcePaperIDs)
	}
}

func TestMergePapersDoesNotDedupeByTitleWhenDOIsConflict(t *testing.T) {
	out := MergePapers([]SourceResult{
		{Source: SourcePubMed, Papers: []Paper{{Source: SourcePubMed, SourcePaperID: "pmid", DOI: "10.1/abc", Title: "Cell Fate Control"}}},
		{Source: SourceSemanticScholar, Papers: []Paper{{Source: SourceSemanticScholar, SourcePaperID: "s2", DOI: "10.1/def", Title: "Cell Fate Control!"}}},
	}, []SourceID{SourcePubMed, SourceSemanticScholar}, 10)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(out), out)
	}
}

func TestMergePapersDoesNotDedupeByTitleWhenPMIDsConflict(t *testing.T) {
	out := MergePapers([]SourceResult{
		{Source: SourcePubMed, Papers: []Paper{{Source: SourcePubMed, SourcePaperID: "123", PMID: "123", Title: "Cell Fate Control"}}},
		{Source: SourceSemanticScholar, Papers: []Paper{{Source: SourceSemanticScholar, SourcePaperID: "s2", PMID: "456", Title: "Cell Fate Control!"}}},
	}, []SourceID{SourcePubMed, SourceSemanticScholar}, 10)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(out), out)
	}
}

func TestMergePapersDoesNotDedupeWhenUnicodeTitleLetterDiffers(t *testing.T) {
	out := MergePapers([]SourceResult{
		{Source: SourcePubMed, Papers: []Paper{{Source: SourcePubMed, SourcePaperID: "pmid", Title: "β cell"}}},
		{Source: SourceSemanticScholar, Papers: []Paper{{Source: SourceSemanticScholar, SourcePaperID: "s2", Title: "cell"}}},
	}, []SourceID{SourcePubMed, SourceSemanticScholar}, 10)
	if len(out) != 2 {
		t.Fatalf("len = %d, want 2: %+v", len(out), out)
	}
}

func TestMergePapersDedupesByNormalizedTitleAndPreservesSourceOrder(t *testing.T) {
	out := MergePapers([]SourceResult{
		{Source: SourceSemanticScholar, Papers: []Paper{{Source: SourceSemanticScholar, SourcePaperID: "s2", Title: "Cell Fate Control!"}}},
		{Source: SourcePubMed, Papers: []Paper{{Source: SourcePubMed, SourcePaperID: "pmid", Title: "cell fate control"}}},
	}, []SourceID{SourcePubMed, SourceSemanticScholar}, 10)
	if len(out) != 1 {
		t.Fatalf("len = %d, want 1", len(out))
	}
	if out[0].SourcePaperIDs[SourcePubMed] != "pmid" || out[0].SourcePaperIDs[SourceSemanticScholar] != "s2" {
		t.Fatalf("source ids = %+v", out[0].SourcePaperIDs)
	}
	if out[0].Source != SourcePubMed || out[0].SourcePaperID != "pmid" {
		t.Fatalf("primary source metadata = %+v", out[0])
	}
	if len(out[0].Sources) != 2 || out[0].Sources[0] != SourcePubMed || out[0].Sources[1] != SourceSemanticScholar {
		t.Fatalf("source order = %+v", out[0].Sources)
	}
}

func TestMergePapersTruncatesAfterMerge(t *testing.T) {
	out := MergePapers([]SourceResult{
		{Source: SourcePubMed, Papers: []Paper{
			{Source: SourcePubMed, SourcePaperID: "1", PMID: "1", Title: "One"},
			{Source: SourcePubMed, SourcePaperID: "2", PMID: "2", Title: "Two"},
		}},
	}, []SourceID{SourcePubMed}, 1)
	if len(out) != 1 || out[0].PMID != "1" {
		t.Fatalf("out = %+v", out)
	}
}
