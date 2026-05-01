package model

import "strings"

const (
	AIExternalSourcePubMed          = "pubmed"
	AIExternalSourceSemanticScholar = "semantic_scholar"
)

type AIExternalSearchSettings struct {
	Sources      []string `json:"sources"`
	PubMedAPIKey string   `json:"pubmed_api_key,omitempty"`
	PubMedEmail  string   `json:"pubmed_email,omitempty"`
	PubMedTool   string   `json:"pubmed_tool,omitempty"`
}

func NormalizeAIExternalSearchSettings(input AIExternalSearchSettings) AIExternalSearchSettings {
	seen := map[string]bool{}
	sources := make([]string, 0, len(input.Sources))
	for _, source := range input.Sources {
		source = strings.TrimSpace(strings.ToLower(source))
		switch source {
		case AIExternalSourcePubMed, AIExternalSourceSemanticScholar:
		default:
			continue
		}
		if seen[source] {
			continue
		}
		seen[source] = true
		sources = append(sources, source)
	}
	if input.Sources == nil && len(sources) == 0 {
		sources = []string{AIExternalSourcePubMed}
	}
	return AIExternalSearchSettings{
		Sources:      sources,
		PubMedAPIKey: strings.TrimSpace(input.PubMedAPIKey),
		PubMedEmail:  strings.TrimSpace(input.PubMedEmail),
		PubMedTool:   strings.TrimSpace(input.PubMedTool),
	}
}
