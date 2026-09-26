package search

import (
	"testing"

	"kiro-cli-history/internal/session"
)

func TestSessions(t *testing.T) {
	all := []session.Session{
		{SessionID: "a", SearchText: "deploy to aws\n/home/x\n"},
		{SessionID: "b", SearchText: "deploy locally\n"},
	}
	cases := []struct {
		q    string
		want int
	}{
		{"", 2},
		{"   ", 2},
		{"DEPLOY", 2},
		{"deploy aws", 1},
		{"aws deploy", 1},
		{"gcp", 0},
	}
	for _, c := range cases {
		if got := len(Sessions(c.q, all)); got != c.want {
			t.Errorf("%q: got %d, want %d", c.q, got, c.want)
		}
	}
}
