package service

import (
	"strings"
	"testing"
)

func TestParseAIFigureRegionsStandardPayload(t *testing.T) {
	regions, err := parseAIFigureRegions(`{"figures":[{"bbox":[100,120,700,820],"confidence":0.93}]}`)
	if err != nil {
		t.Fatalf("parseAIFigureRegions() error = %v", err)
	}
	if len(regions) != 1 {
		t.Fatalf("regions = %+v, want 1 item", regions)
	}
	region := regions[0]
	if region.X != 0.1 || region.Y != 0.12 || region.Width != 0.6 || region.Height != 0.7 {
		t.Fatalf("region = %+v, want normalized 0-1000 scale", region)
	}
	if region.Confidence != 0.93 {
		t.Fatalf("confidence = %v, want 0.93", region.Confidence)
	}
}

func TestParseAIFigureRegionsAcceptsBBox2DAlias(t *testing.T) {
	regions, err := parseAIFigureRegions(`{"figures":[{"bbox_2d":[0,0,1000,1000],"label":"figure"}]}`)
	if err != nil {
		t.Fatalf("parseAIFigureRegions() error = %v", err)
	}
	if len(regions) != 1 || regions[0].Width != 1 || regions[0].Height != 1 {
		t.Fatalf("regions = %+v, want full-page region from bbox_2d", regions)
	}
}

func TestParseAIFigureRegionsAcceptsBareArray(t *testing.T) {
	regions, err := parseAIFigureRegions(`[{"bbox":[100,100,500,500]},{"bbox":[500,500,900,900]}]`)
	if err != nil {
		t.Fatalf("parseAIFigureRegions() error = %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("regions = %+v, want 2 items", regions)
	}
}

func TestParseAIFigureRegionsAcceptsCodeFenceAndProse(t *testing.T) {
	fenced := "```json\n{\"figures\":[{\"bbox\":[0.1,0.1,0.5,0.5]}]}\n```"
	if regions, err := parseAIFigureRegions(fenced); err != nil || len(regions) != 1 {
		t.Fatalf("fenced parse = %+v, %v; want 1 region", regions, err)
	}

	prose := "I found one figure on the page.\n{\"figures\":[{\"bbox\":[0.1,0.1,0.5,0.5]}]}\nHope this helps."
	if regions, err := parseAIFigureRegions(prose); err != nil || len(regions) != 1 {
		t.Fatalf("prose parse = %+v, %v; want 1 region", regions, err)
	}
}

func TestParseAIFigureRegionsSalvagesTruncatedJSON(t *testing.T) {
	truncated := `{"figures":[{"bbox":[100,120,700,820],"confidence":0.93},{"bbox_2d":[50,60,400,410]},{"bbox":[10,20`
	regions, err := parseAIFigureRegions(truncated)
	if err != nil {
		t.Fatalf("parseAIFigureRegions() error = %v", err)
	}
	if len(regions) != 2 {
		t.Fatalf("regions = %+v, want 2 salvaged items", regions)
	}
}

func TestParseAIFigureRegionsEmptyAndNoFigure(t *testing.T) {
	if regions, err := parseAIFigureRegions("  "); err != nil || len(regions) != 0 {
		t.Fatalf("empty input = %+v, %v; want no regions and no error", regions, err)
	}
	if regions, err := parseAIFigureRegions(`{"figures":[]}`); err != nil || len(regions) != 0 {
		t.Fatalf("empty figures = %+v, %v; want no regions and no error", regions, err)
	}
}

func TestParseAIFigureRegionsInvalidIncludesRawPreview(t *testing.T) {
	_, err := parseAIFigureRegions("The page contains a figure near the top but I will not return JSON.")
	if err == nil {
		t.Fatal("parseAIFigureRegions() error = nil, want failure")
	}
	if !strings.Contains(err.Error(), "返回片段") || !strings.Contains(err.Error(), "The page contains a figure") {
		t.Fatalf("error = %q, want raw preview snippet", err.Error())
	}
}
