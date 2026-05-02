package agent_session

import "strings"

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
