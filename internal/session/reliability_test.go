package session

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func TestRewoundSubagentKeepsOwner(t *testing.T) {
	all := []Session{
		{SessionID: "rewind", ParentID: "child", CreatedReason: "rewind"},
		{SessionID: "nested", ParentID: "rewind", CreatedReason: "subagent"},
		{SessionID: "child", ParentID: "root", CreatedReason: "subagent"},
		{SessionID: "root"},
	}
	Link(all)
	if all[0].OwnerID != "root" {
		t.Fatal("rewind lost its spawning parent")
	}
	rows, children := Collapse(all, true, true)
	if len(rows) != 1 || rows[0].SessionID != "root" || len(children["root"]) != 3 {
		t.Fatalf("unexpected grouping: rows=%+v children=%+v", rows, children)
	}
	// A search result remains accessible even if its owner did not match.
	rows, _ = Collapse(all[:1], true, true)
	if len(rows) != 1 {
		t.Fatal("orphan search result disappeared")
	}
}

func TestChronologicalSortUsesOffsets(t *testing.T) {
	later := Session{UpdatedAt: "2026-11-02T13:00:00Z"}
	earlier := Session{UpdatedAt: "2026-11-02T14:00:00+05:30"}
	if !sortKey(&later).After(sortKey(&earlier)) {
		t.Fatal("timestamps sorted by wall clock instead of instant")
	}
	fallback := Session{UpdatedAt: "bad", CreatedAt: later.UpdatedAt}
	if !sortKey(&fallback).Equal(sortKey(&later)) {
		t.Fatal("invalid update time should fall back to creation")
	}
}

func TestConfigSnapshotsDuringIndexing(t *testing.T) {
	before := CurrentConfig()
	t.Cleanup(func() { SetConfig(before) })
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			cfg := DefaultConfig
			cfg.SQLiteEnabled = i%2 == 0
			SetConfig(cfg)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			BuildFullIndex(nil)
		}
	}()
	wg.Wait()
}

func TestConfigAtomicSaveAndErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings", "config.json")
	cfg := DefaultConfig
	if err := saveConfigFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	cfg.DefaultView = "list"
	if err := saveConfigFile(path, cfg); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"list"`) {
		t.Fatal("new settings not saved")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatal("temporary config file left behind")
	}
	if err := saveConfigFile(filepath.Dir(path), cfg); err == nil {
		t.Fatal("rename onto an existing directory must report an error")
	}
	if got, _ := os.ReadFile(path); string(got) != string(data) {
		t.Fatal("failed save changed previous settings")
	}
}

func TestIndexBudgetKeepsCountsAndValidUTF8(t *testing.T) {
	s := filepath.Join(t.TempDir(), "messages.jsonl")
	if err := os.WriteFile(s, []byte(
		"{\"payload\":{\"type\":\"user\",\"content\":\"hello नमस्ते\"}}\n"+
			"{\"payload\":{\"type\":\"assistant\",\"content\":\"another message\"}}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	text, count, limited := indexJSONLBounded(s, newIndexBudget(12))
	if count != 2 || !limited || len(text) > 12 || !utf8.ValidString(text) {
		t.Fatalf("text=%q count=%d limited=%v", text, count, limited)
	}
}

func TestIndexReadFailureIsReportedAsLimited(t *testing.T) {
	for _, path := range []string{t.TempDir(), filepath.Join(t.TempDir(), "missing.jsonl")} {
		_, _, limited := indexJSONLBounded(path, newIndexBudget(MaxIndexBytes))
		if !limited {
			t.Fatalf("unreadable conversation %q was reported as fully indexed", path)
		}
	}
}

func TestIndexBudgetAccountsForExpandingUnicode(t *testing.T) {
	index := indexText{budget: newIndexBudget(MaxIndexBytes)}
	// U+023A uses two UTF-8 bytes; its lowercase U+2C65 uses three.
	index.add(strings.Repeat("\u023a", MaxSessionIndexBytes/3+1))
	if index.text.Len() > MaxSessionIndexBytes || !index.limited || !utf8.ValidString(index.text.String()) {
		t.Fatalf("index bytes=%d limited=%v", index.text.Len(), index.limited)
	}
}
