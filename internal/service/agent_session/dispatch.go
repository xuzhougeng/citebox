package agent_session

import (
	"regexp"
	"strings"
)

// parseSlash returns (command, arg, ok). Both half-width "/" and full-width "／"
// are accepted as the command prefix (mirrors the legacy bridge).
func parseSlash(text string) (string, string, bool) {
	s := strings.TrimSpace(text)
	if strings.HasPrefix(s, "／") {
		s = "/" + strings.TrimPrefix(s, "／")
	}
	if !strings.HasPrefix(s, "/") {
		return "", "", false
	}
	parts := strings.SplitN(s, " ", 2)
	cmd := strings.ToLower(parts[0])
	arg := ""
	if len(parts) == 2 {
		arg = strings.TrimSpace(parts[1])
	}
	return cmd, arg, true
}

// doiPattern matches the standard DOI shape (10.NNNN/...) anywhere in the
// text. The character class follows the CrossRef recommendation.
var doiPattern = regexp.MustCompile(`(?i)\b10\.\d{4,9}/[-._;()/:A-Z0-9]+\b`)

// looksLikeDOI reports whether the trimmed text contains something that should
// be treated as a bare DOI (so the agent can route it to the import command
// before slash-parsing). Accepts both raw DOIs and the common `doi:` /
// `doi.org/` prefix forms.
func looksLikeDOI(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// Reject anything containing internal whitespace — matches the legacy
	// looksLikeWeixinDOIText behavior and prevents sentences like
	// "请解释 10.1038/x 的结论" from being misclassified as bare DOIs.
	if strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	if doiPattern.MatchString(s) {
		return true
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "doi:") {
		return true
	}
	if strings.Contains(lower, "doi.org/") {
		return true
	}
	return false
}
