package search

import (
	"strings"
	"sync"

	"kiro-cli-history/internal/session"
)

// Sessions filters by fuzzy-matching against the pre-built SearchText index.
func Sessions(query string, sessions []session.Session, mu *sync.RWMutex) []session.Session {
	if query == "" {
		return sessions
	}
	q := strings.ToLower(query)
	mu.RLock()
	defer mu.RUnlock()
	var out []session.Session
	for i := range sessions {
		if match(q, sessions[i].SearchText) {
			out = append(out, sessions[i])
		}
	}
	return out
}

func match(query, text string) bool {
	for _, tok := range strings.Fields(query) {
		if !strings.Contains(text, tok) {
			return false
		}
	}
	return true
}
