package ai_assistant

import (
	"github.com/xuzhougeng/citebox/internal/model"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var questionFigureRE = regexp.MustCompile(`(?i)(?:figure|fig\.?|图)\s*(\d+)([a-z])?\b`)

// Rank figures within each paper; the caller retains cross-paper round-robin
// selection and the user's opt-in/image-count limits.
func RelevantFigures(figures []model.Figure, question string) []model.Figure {
	terms := EvidenceSearchTerms(question)
	references := questionFigureRE.FindAllStringSubmatch(question, -1)
	type ranked struct {
		figure model.Figure
		score  int
	}
	var candidates []ranked
	for _, f := range figures {
		score := 0
		explicitChild := false
		for _, ref := range references {
			n, _ := strconv.Atoi(ref[1])
			if n != f.FigureIndex {
				continue
			}
			if ref[2] != "" {
				if strings.EqualFold(ref[2], f.SubfigureLabel) {
					score += 2000
					explicitChild = true
				}
			} else if f.ParentFigureID == nil {
				score += 1000
			}
		}
		if f.ParentFigureID != nil && !explicitChild {
			continue
		}
		text := strings.ToLower(f.Caption + " " + f.DisplayLabel)
		for _, term := range terms {
			if strings.Contains(text, strings.ToLower(term)) {
				score++
			}
		}
		candidates = append(candidates, ranked{f, score})
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	out := make([]model.Figure, 0, len(candidates))
	for _, c := range candidates {
		out = append(out, c.figure)
	}
	return out
}
