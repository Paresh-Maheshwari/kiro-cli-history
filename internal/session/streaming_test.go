package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRepeated(t *testing.T, f *os.File, chunk string, size int) {
	t.Helper()
	for size > 0 {
		part := chunk[:min(size, len(chunk))]
		if _, err := f.WriteString(part); err != nil {
			t.Fatal(err)
		}
		size -= len(part)
	}
}

func TestJSONLStreamsBeyondWholeFileLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	// A valid tool record larger than both old limits is ignored without
	// buffering its contents. Visible messages after it must remain readable.
	if _, err := f.WriteString(`{"version":"v1","kind":"ToolResults","data":"`); err != nil {
		t.Fatal(err)
	}
	writeRepeated(t, f, strings.Repeat("x", 1024*1024), MaxFileSize+1)
	if _, err := f.WriteString("\"}\n" +
		"{\"payload\":{\"type\":\"user\",\"content\":\"first prompt\"}}\n" +
		"{\"payload\":{\"type\":\"assistant\",\"content\":\"last reply\"}}\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	s := Session{JSONLPath: path}
	count := 0
	if err := WalkMessages(s, func(Msg) bool { count++; return true }); err != nil || count != 2 {
		t.Fatalf("count=%d error=%v", count, err)
	}
	if first := firstPromptJSONL(path); first != "first prompt" {
		t.Fatalf("large conversation lost title: %q", first)
	}
	text, count, limited := indexJSONLBounded(path, newIndexBudget(100))
	if count != 2 || limited || !strings.Contains(text, "last reply") {
		t.Fatalf("count=%d limited=%v text=%q", count, limited, text)
	}
}

func TestOversizedRecordDoesNotHideLaterMessages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	if _, err := f.WriteString(`{"payload":{"type":"user","content":"`); err != nil {
		t.Fatal(err)
	}
	writeRepeated(t, f, strings.Repeat("x", 1024*1024), maxLineSize+1)
	if _, err := f.WriteString("\"}}\n{\"payload\":{\"type\":\"user\",\"content\":\"after oversized record\"}}\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	count := 0
	err = WalkMessages(Session{JSONLPath: path}, func(m Msg) bool {
		count++
		if m.Text != "after oversized record" {
			t.Fatalf("unexpected message %q", m.Text)
		}
		return true
	})
	if !errors.Is(err, errJSONLRecordTooLarge) || count != 1 {
		t.Fatalf("count=%d error=%v", count, err)
	}
}

func TestJSONLCancellationDuringHiddenTraffic(t *testing.T) {
	path := writeFile(t, t.TempDir(), "tools.jsonl",
		`{"version":"v1","kind":"ToolResults","data":"`+strings.Repeat("x", 1024*1024)+"\"}\n")
	checks := 0
	err := WalkMessagesWithCancel(Session{JSONLPath: path}, func(Msg) bool {
		t.Fatal("tool event should not be visible")
		return true
	}, func() bool { checks++; return checks == 4 })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("hidden traffic ignored cancellation: %v", err)
	}
}

func TestJSONLReadUsesInitialFileSize(t *testing.T) {
	path := writeFile(t, t.TempDir(), "active.jsonl", "{\"payload\":{\"type\":\"user\",\"content\":\"original\"}}\n")
	count := 0
	err := WalkMessages(Session{JSONLPath: path}, func(Msg) bool {
		count++
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, err = f.WriteString("{\"payload\":{\"type\":\"user\",\"content\":\"appended\"}}\n")
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		return true
	})
	if err != nil || count != 1 {
		t.Fatalf("reading chased a growing conversation: count=%d error=%v", count, err)
	}
}
