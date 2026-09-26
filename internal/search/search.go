package search

import (
	"strings"

	"kiro-cli-history/internal/session"
)

// Sessions filters by matching every query word against the pre-built
// SearchText index. An empty query returns sessions unchanged.
func Sessions(query string, sessions []session.Session) []session.Session {
	toks := strings.Fields(strings.ToLower(query))
	if len(toks) == 0 {
		return sessions
	}
	var out []session.Session
	for i := range sessions {
		if match(toks, sessions[i].SearchText) {
			out = append(out, sessions[i])
		}
	}
	return out
}

func match(toks []string, text string) bool {
	for _, tok := range toks {
		if !strings.Contains(text, tok) {
			return false
		}
	}
	return true
}
