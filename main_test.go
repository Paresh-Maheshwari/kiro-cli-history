package main

import (
	"reflect"
	"testing"

	"kiro-cli-history/internal/session"
)

func TestResumeArgs(t *testing.T) {
	for _, source := range []string{"jsonl_v3", "jsonl", "sqlite_v1", "sqlite_v2"} {
		t.Run(source, func(t *testing.T) {
			id := "cli_12345678-1234-1234-1234-123456789abc_suffix"
			want := []string{"kiro-cli", "chat"}
			if source == "jsonl_v3" {
				want = append(want, "--v3")
			}
			want = append(want, "--resume-id", id)
			if got := resumeArgs(&session.Session{Source: source, SessionID: id}); !reflect.DeepEqual(got, want) {
				t.Fatalf("got %v, want %v", got, want)
			}
		})
	}
}
