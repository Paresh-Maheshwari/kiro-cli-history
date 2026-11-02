package ui

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"kiro-cli-history/internal/session"
)

func v3Message(t *testing.T, role, text string) session.Session {
	t.Helper()
	path := filepath.Join(t.TempDir(), "messages.jsonl")
	data, err := json.Marshal(map[string]any{"payload": map[string]any{"type": role, "content": text}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	return session.Session{SessionID: "test", Cwd: "/work", Title: "Test", Source: "jsonl_v3", JSONLPath: path}
}

func assertSafeDisplay(t *testing.T, out string) {
	t.Helper()
	// SGR is the only permitted escape sequence in a rendered application view.
	out = regexp.MustCompile("\x1b\\[[0-9;:]*m").ReplaceAllString(out, "")
	for _, r := range out {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			t.Fatalf("unsafe control U+%04X in display", r)
		}
	}
}

func TestTerminalControlsInMessagesAndMetadata(t *testing.T) {
	for _, role := range []string{"user", "assistant"} {
		for _, text := range []string{
			"Hello 👋\nline\tend \x1b]52;c;payload\a",
			"\x1b[2J\x1bPdata\x1b\\\u009b2J\x7f\r",
			"**&#x1b;]52;c;payload&#7;**",
			"&#27;[2J &#x9b;2J",
			"**bold** and `code` and [link](https://example.test)",
		} {
			s := v3Message(t, role, text)
			s.Title, s.Cwd, s.SessionID, s.ParentTitle = "title\x1b[2J", "/dir\x1b]52;c;evil\a", "id\u009b2J", "\x1b[Hparent"
			s.CreatedReason, s.ParentID = "subagent", "parent"
			out, _ := RenderPreviewN(s, 90, 0, nil)
			assertSafeDisplay(t, out)
			msgs := session.ExtractMessages(s, 0)
			if len(msgs) != 1 || msgs[0].Text != strings.TrimSpace(text) {
				t.Fatal("display sanitization changed stored/extracted content")
			}
			m := NewModel()
			m.W, m.H, m.Loading = 120, 40, false
			m.All = []session.Session{s}
			for _, mode := range []ViewMode{ViewList, ViewTree} {
				m.ViewMode = mode
				m.refilter(true)
				assertSafeDisplay(t, m.View())
				m.SetNote(text)
				assertSafeDisplay(t, m.View())
			}
		}
	}
}

func TestExportPrivateUniqueAndPreservesRawText(t *testing.T) {
	raw := "Hello नमस्ते\n\x1b]52;c;raw\a"
	s := v3Message(t, "user", raw)
	s.Title = "नमस्ते"
	dir := filepath.Join(t.TempDir(), "exports")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan string, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			path, err := exportConversation(dir, s)
			if err != nil {
				t.Error(err)
				return
			}
			results <- path
		}()
	}
	wg.Wait()
	close(results)
	seen := map[string]bool{}
	for path := range results {
		if seen[path] {
			t.Fatal("duplicate export filename")
		}
		seen[path] = true
		data, err := os.ReadFile(path)
		if err != nil || !strings.Contains(string(data), raw) {
			t.Fatalf("export changed content: %v", err)
		}
		if runtime.GOOS != "windows" {
			info, _ := os.Stat(path)
			if info.Mode().Perm() != 0600 {
				t.Fatalf("export permissions: %v", info.Mode())
			}
		}
	}
	if len(seen) != 8 {
		t.Fatal("missing exports")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(dir)
		if info.Mode().Perm() != 0700 {
			t.Fatalf("directory permissions: %v", info.Mode())
		}
	}
}

func TestExportRejectsSymlinkAndCleansFailedFiles(t *testing.T) {
	base := t.TempDir()
	target, link := filepath.Join(base, "target"), filepath.Join(base, "link")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	s := v3Message(t, "user", "hello")
	if err := os.Symlink(target, link); err == nil {
		if _, err := exportConversation(link, s); err == nil {
			t.Fatal("accepted symlink export directory")
		}
	}
	s.JSONLPath = filepath.Join(base, "missing.jsonl")
	if _, err := exportConversation(target, s); err == nil {
		t.Fatal("missing conversation should fail")
	}
	entries, _ := os.ReadDir(target)
	if len(entries) != 0 {
		t.Fatal("failed export left a partial transcript")
	}
}

func TestQuitAndTinyTerminals(t *testing.T) {
	for _, overlay := range []string{"help", "settings", "fullscreen"} {
		m := NewModel()
		m.ShowHelp, m.ShowSettings, m.Fullscreen = overlay == "help", overlay == "settings", overlay == "fullscreen"
		_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		if cmd == nil {
			t.Fatalf("Ctrl+C swallowed by %s", overlay)
		}
	}
	for _, size := range [][2]int{{1, 1}, {20, 5}, {40, 24}, {60, 12}, {80, 24}} {
		m := NewModel()
		m.Loading = false
		next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		out := next.(Model).View()
		for _, line := range strings.Split(out, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("view exceeds width %d: %q", size[0], line)
			}
		}
		if len(strings.Split(out, "\n")) > size[1] {
			t.Fatalf("view exceeds height %d", size[1])
		}
	}
}

func TestPreviewIsDeferredAndBounded(t *testing.T) {
	s := v3Message(t, "assistant", strings.Repeat("**large** ", 10000))
	m := NewModel()
	m.W, m.H, m.Loading, m.ViewMode = 100, 30, false, ViewList
	m.Preview.Width, m.Preview.Height = 100, 24
	m.All = []session.Session{s}
	m.refilter(true)
	if !strings.Contains(m.Preview.View(), "Loading") || m.pendingKey == "" {
		t.Fatal("preview should defer work")
	}
	out, complete := RenderPreviewN(s, 90, 0, nil)
	if !complete || len(out) > 2*len(strings.Repeat("**large** ", 10000)) {
		t.Fatal("large message should use bounded plain-text fallback")
	}
	for i := 0; i < 40; i++ {
		m.cachePreview(string(rune(i+65)), strings.Repeat("x", 1024*1024))
	}
	total := 0
	for _, cached := range m.PrevCache {
		total += len(cached)
	}
	if total > maxPreviewCache {
		t.Fatal("preview cache exceeded byte budget")
	}
}

func TestPreviewCapsExpandedMarkdown(t *testing.T) {
	// Each small message qualifies for Markdown rendering, whose styling and
	// wrapping can expand the aggregate well beyond the source text budget.
	s := v3Message(t, "assistant", strings.Repeat("**bold** ", 800))
	data, err := os.ReadFile(s.JSONLPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.JSONLPath, []byte(strings.Repeat(string(data), 80)), 0600); err != nil {
		t.Fatal(err)
	}
	out, complete := RenderPreviewN(s, 90, 0, nil)
	if !complete || len(out) > maxPreviewOutput || !strings.Contains(out, "Preview size limit reached") {
		t.Fatalf("output size=%d, complete=%v, missing size-limit notice=%v",
			len(out), complete, !strings.Contains(out, "Preview size limit reached"))
	}
	assertSafeDisplay(t, out)
}

func TestPreviewShowsPrefixOfLargeMessage(t *testing.T) {
	s := v3Message(t, "assistant", "Beginning "+strings.Repeat("नमस्ते", maxPreviewText/6)+" END-OF-MESSAGE")
	out, complete := RenderPreviewN(s, 90, 0, nil)
	if !complete || !utf8.ValidString(out) || !strings.Contains(out, "Beginning") ||
		strings.Contains(out, "END-OF-MESSAGE") || !strings.Contains(out, "Preview size limit reached") {
		t.Fatal("large message should show a valid prefix and size-limit notice")
	}
}

func TestSettingsErrorVisibleInsideOverlay(t *testing.T) {
	m := NewModel()
	m.Loading, m.ShowSettings = false, true
	m.W, m.H = 100, 40
	m.SetNote("Could not save settings: permission denied")
	if !strings.Contains(m.View(), "Could not save settings") {
		t.Fatal("settings error is hidden behind the settings overlay")
	}
}

func TestLegacyResumeIsRejected(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewList
	m.Filtered = []session.Session{{Cwd: "/work", Source: "sqlite_v1"}}
	_, cmd := m.DoResume()
	if cmd != nil || m.ResumeResult != nil {
		t.Fatal("legacy session without an ID must not launch an invalid command")
	}
}

func TestClipboardTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses native clipboard with a bounded open wait")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "pbcopy")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nwhile :; do :; done\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := copyText(ctx, "text"); err == nil || ctx.Err() == nil {
		t.Fatalf("clipboard command was not cancelled: %v", err)
	}
}
