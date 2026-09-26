package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDecodeMsg(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantOK   bool
		wantRole string
		wantText string
	}{
		{
			name:     "prompt text",
			line:     `{"version":"v1","kind":"Prompt","data":{"content":[{"kind":"text","data":"hello"}]}}`,
			wantOK:   true,
			wantRole: "you",
			wantText: "hello",
		},
		{
			// Regression: object-valued blocks used to make the whole message fail to decode.
			name:     "assistant text with toolUse and thinking blocks",
			line:     `{"version":"v1","kind":"AssistantMessage","data":{"content":[{"kind":"thinking","data":{"text":"hmm"}},{"kind":"text","data":"answer"},{"kind":"toolUse","data":{"name":"shell","input":{}}}]}}`,
			wantOK:   true,
			wantRole: "kiro",
			wantText: "answer",
		},
		{
			name:     "prompt with image block",
			line:     `{"version":"v1","kind":"Prompt","data":{"content":[{"kind":"text","data":"see this"},{"kind":"image","data":{"format":"png"}}]}}`,
			wantOK:   true,
			wantRole: "you",
			wantText: "see this",
		},
		{
			name:     "multiple text blocks are joined",
			line:     `{"version":"v1","kind":"AssistantMessage","data":{"content":[{"kind":"text","data":"a"},{"kind":"text","data":"b"}]}}`,
			wantOK:   true,
			wantRole: "kiro",
			wantText: "a\n\nb",
		},
		{
			name: "tool-only assistant turn is hidden",
			line: `{"version":"v1","kind":"AssistantMessage","data":{"content":[{"kind":"text","data":"  "},{"kind":"toolUse","data":{}}]}}`,
		},
		{
			name: "tool results skipped",
			line: `{"version":"v1","kind":"ToolResults","data":{"content":[{"kind":"text","data":"\"kind\":\"Prompt\""}]}}`,
		},
		{
			name: "clear has no data",
			line: `{"version":"v1","kind":"Clear"}`,
		},
		{
			name:     "different key order still decodes",
			line:     `{"kind":"Prompt","version":"v2","data":{"content":[{"kind":"text","data":"x"}]}}`,
			wantOK:   true,
			wantRole: "you",
			wantText: "x",
		},
		{
			name: "truncated line",
			line: `{"version":"v1","kind":"Prompt","data":{"content":[{"kind":"te`,
		},
		{name: "empty", line: ``},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, ok := decodeMsg([]byte(tt.line))
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && (m.Role != tt.wantRole || m.Text != tt.wantText) {
				t.Fatalf("got %q/%q, want %q/%q", m.Role, m.Text, tt.wantRole, tt.wantText)
			}
		})
	}
}

func TestPeekKind(t *testing.T) {
	if k := peekKind([]byte(`{"version":"v1","kind":"ToolResults","data":{}}`)); k != "ToolResults" {
		t.Fatalf("got %q", k)
	}
	if k := peekKind([]byte(`{"kind":"Prompt"}`)); k != "" {
		t.Fatalf("unexpected prefix should return empty, got %q", k)
	}
}

func TestScanJSONLCountsMatchPreview(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "a.jsonl", strings.Join([]string{
		`{"version":"v1","kind":"Prompt","data":{"content":[{"kind":"text","data":"Q1"}]}}`,
		`{"version":"v1","kind":"AssistantMessage","data":{"content":[{"kind":"toolUse","data":{}}]}}`,
		`{"version":"v1","kind":"ToolResults","data":{"content":[]}}`,
		`{"version":"v1","kind":"AssistantMessage","data":{"content":[{"kind":"text","data":"A1"},{"kind":"toolUse","data":{}}]}}`,
		`not json`,
		`{"version":"v1","kind":"Prompt","data":{"content":[{"kind":"text","data":"Q2 Deploy"}]}}`,
		`{"version":"v1","kind":"Prompt","data":{"conte`, // partially written last line
	}, "\n"))

	msgs := extractJSONLMessages(p, 0)
	text, count := indexJSONL(p)
	if len(msgs) != 3 || count != 3 {
		t.Fatalf("preview=%d index=%d, want 3/3", len(msgs), count)
	}
	if !strings.Contains(text, "q2 deploy") {
		t.Fatalf("index not lowercased/complete: %q", text)
	}
	if got := extractJSONLMessages(p, 2); len(got) != 2 {
		t.Fatalf("limit ignored: %d", len(got))
	}
	if fp := firstPromptJSONL(p); fp != "Q1" {
		t.Fatalf("first prompt = %q", fp)
	}
}

func TestScanJSONLLongLine(t *testing.T) {
	dir := t.TempDir()
	big := strings.Repeat("x", 3*1024*1024) // larger than bufio's default 64KB
	p := writeFile(t, dir, "big.jsonl",
		`{"version":"v1","kind":"ToolResults","data":{"content":[{"kind":"text","data":"`+big+`"}]}}`+"\n"+
			`{"version":"v1","kind":"Prompt","data":{"content":[{"kind":"text","data":"after"}]}}`+"\n")
	if msgs := extractJSONLMessages(p, 0); len(msgs) != 1 || msgs[0].Text != "after" {
		t.Fatalf("got %+v", msgs)
	}
}

func TestReadMeta(t *testing.T) {
	dir := t.TempDir()
	state := `{"conversation_metadata":{"x":"` + strings.Repeat("y", 100_000) + `"}}`

	t.Run("stops at session_state", func(t *testing.T) {
		p := writeFile(t, dir, "a.json", `{"session_id":"id1","cwd":"/w","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T01:00:00Z","title":null,"parent_session_id":"p1","session_created_reason":"rewind","session_state":`+state+`}`)
		m, err := readMeta(p)
		if err != nil {
			t.Fatal(err)
		}
		if m.SessionID != "id1" || m.Cwd != "/w" || m.Title != "" || m.ParentID != "p1" || m.CreatedReason != "rewind" {
			t.Fatalf("got %+v", m)
		}
	})

	t.Run("session_id after session_state falls back to full read", func(t *testing.T) {
		p := writeFile(t, dir, "b.json", `{"title":"T","session_state":`+state+`,"session_id":"id2","cwd":"/w"}`)
		m, err := readMeta(p)
		if err != nil || m.SessionID != "id2" || m.Title != "T" || m.Cwd != "/w" {
			t.Fatalf("got %+v err=%v", m, err)
		}
	})

	t.Run("old format without new fields", func(t *testing.T) {
		p := writeFile(t, dir, "c.json", `{"session_id":"id3","title":"Old","cwd":"/o","created_at":"","updated_at":""}`)
		m, err := readMeta(p)
		if err != nil || m.SessionID != "id3" || m.Title != "Old" || m.CreatedReason != "" {
			t.Fatalf("got %+v err=%v", m, err)
		}
	})

	t.Run("not an object", func(t *testing.T) {
		p := writeFile(t, dir, "d.json", `[1,2]`)
		if _, err := readMeta(p); err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestLoadJSONL(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KIRO_DEMO_DIR", dir)
	cli := filepath.Join(dir, "kiro", "sessions", "cli")
	if err := os.MkdirAll(filepath.Join(cli, "sub-dir", "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	// null title -> first prompt (first line only, whitespace collapsed)
	writeFile(t, cli, "a.json", `{"session_id":"a","cwd":"/w","title":null,"updated_at":"2026-01-02T00:00:00Z"}`)
	writeFile(t, cli, "a.jsonl", `{"version":"v1","kind":"Prompt","data":{"content":[{"kind":"text","data":"  fix   the\tbug\nmore detail"}]}}`+"\n")
	// title with newline is flattened
	writeFile(t, cli, "b.json", `{"session_id":"b","cwd":"/w","title":"line1\nline2"}`)
	writeFile(t, cli, "b.jsonl", `{"version":"v1","kind":"Prompt","data":{"content":[{"kind":"text","data":"x"}]}}`+"\n")
	// opened but never used: empty .jsonl -> skipped
	writeFile(t, cli, "c.json", `{"session_id":"c","cwd":"/w","title":null}`)
	writeFile(t, cli, "c.jsonl", ``)
	// no .jsonl at all -> skipped
	writeFile(t, cli, "d.json", `{"session_id":"d","cwd":"/w","title":"D"}`)
	// garbage .json -> skipped
	writeFile(t, cli, "e.json", `{{{`)
	writeFile(t, cli, "e.jsonl", `x`)
	// unrelated files are ignored
	writeFile(t, cli, "a.history", "fix the bug\n")
	writeFile(t, cli, "a.lock", `{"pid":1}`)

	got := map[string]Session{}
	for _, s := range LoadJSONL() {
		got[s.SessionID] = s
	}
	if len(got) != 2 {
		t.Fatalf("loaded %d sessions, want 2: %+v", len(got), got)
	}
	if got["a"].Title != "fix the bug" {
		t.Fatalf("null-title fallback = %q", got["a"].Title)
	}
	if got["b"].Title != "line1 line2" {
		t.Fatalf("title not flattened: %q", got["b"].Title)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in   string
		n    int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 5, "hello…"},
		{"hello world", 6, "hello…"}, // trailing space trimmed before ellipsis
		{"नमस्ते दुनिया", 3, "नमस…"}, // never splits a multi-byte rune
		{"日本語テキスト", 2, "日本…"},
		{"abc", 0, ""},
	}
	for _, tt := range tests {
		if got := Truncate(tt.in, tt.n); got != tt.want {
			t.Errorf("Truncate(%q,%d) = %q, want %q", tt.in, tt.n, got, tt.want)
		}
	}
}
