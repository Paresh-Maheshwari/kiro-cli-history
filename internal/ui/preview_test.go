package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kiro-cli-history/internal/session"
)

func writeSession(t *testing.T, dir, id string, n int) session.Session {
	t.Helper()
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, `{"version":"v1","kind":"Prompt","data":{"content":[{"kind":"text","data":"question %d"}]}}`+"\n", i)
	}
	p := filepath.Join(dir, id+".jsonl")
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return session.Session{SessionID: id, Title: id, Cwd: "/w", Source: "jsonl", JSONLPath: p, MsgCount: n}
}

func TestLazyPreview(t *testing.T) {
	session.SetConfig(session.DefaultConfig)
	dir := t.TempDir()
	big := writeSession(t, dir, "big", 50)
	small := writeSession(t, dir, "small", 3)

	m := NewModel()
	m.W, m.H = 120, 40
	m.Loading = false
	m.All = []session.Session{big, small}
	m.ViewMode = ViewList
	m.refilter(true)

	// Selection only schedules work; it never reads/renders on the UI thread.
	if m.pendingKey == "" || !m.pendingNew {
		t.Fatal("expected a pending full render")
	}
	if c := m.Preview.TotalLineCount(); c == 0 {
		t.Fatal("empty partial preview")
	}
	partialGen := m.pendingGen

	// Update schedules exactly one tick for it.
	next, cmd := m.Update(nil)
	m = next.(Model)
	if cmd == nil || m.pendingNew {
		t.Fatal("full render not scheduled")
	}

	// Moving to another row cancels the old render.
	m.moveCursor(1)
	if m.prevGen.Load() == partialGen {
		t.Fatal("generation not bumped on selection change")
	}
	if c := m.startFullRender(partialGen); c != nil {
		t.Fatal("stale render should not start")
	}
	if m.pendingKey == "" {
		t.Fatal("small session should also render in the background")
	}

	// Back to the big one: run the full render and apply it.
	m.moveCursor(0)
	gen := m.pendingGen
	c := m.startFullRender(gen)
	if c == nil {
		t.Fatal("render did not start")
	}
	done, ok := c().(previewDoneMsg)
	if !ok {
		t.Fatal("render returned no result")
	}
	m.applyFullRender(done)
	if m.pendingKey != "" {
		t.Fatal("pending not cleared after full render")
	}
	full, _ := RenderPreviewN(big, m.RightW(), 0, nil)
	if !strings.Contains(full, "question 50") {
		t.Fatal("full render missing last message")
	}
	if strings.Contains(full, "loading the remaining") {
		t.Fatal("full render still shows loading footer")
	}
	// It is now cached: selecting again needs no background work.
	m.moveCursor(1)
	m.moveCursor(0)
	if m.pendingKey != "" {
		t.Fatal("cached preview should not re-render")
	}
}

func TestRenderPreviewNPartial(t *testing.T) {
	dir := t.TempDir()
	s := writeSession(t, dir, "s", 5)
	out, complete := RenderPreviewN(s, 80, 2, nil)
	if complete || !strings.Contains(out, "question 2") || strings.Contains(out, "question 3") {
		t.Fatalf("partial wrong (complete=%v)", complete)
	}
	if _, complete := RenderPreviewN(s, 80, 5, nil); !complete {
		t.Fatal("exact n should be complete")
	}
	if out, complete := RenderPreviewN(s, 80, 0, func() bool { return true }); complete || out != "" {
		t.Fatal("abort ignored")
	}
}

func BenchmarkLargeAssistantPreview(b *testing.B) {
	dir := b.TempDir()
	text := strings.Repeat("**bold** and `code`\n", 6000)
	raw := fmt.Sprintf(`{"payload":{"type":"assistant","content":%q}}`+"\n", text)
	path := filepath.Join(dir, "messages.jsonl")
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		b.Fatal(err)
	}
	s := session.Session{SessionID: "large", Source: "jsonl_v3", JSONLPath: path}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		RenderPreviewN(s, 90, 0, nil)
	}
}
