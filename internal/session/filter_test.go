package session

import (
	"os"
	"path/filepath"
	"testing"
)

func setupSessions(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("KIRO_DEMO_DIR", dir)
	cli := filepath.Join(dir, "kiro", "sessions", "cli")
	if err := os.MkdirAll(cli, 0o755); err != nil {
		t.Fatal(err)
	}
	mk := func(id, cwd, ts string) {
		writeFile(t, cli, id+".json",
			`{"session_id":"`+id+`","cwd":"`+cwd+`","title":"`+id+`","updated_at":"`+ts+`"}`)
		writeFile(t, cli, id+".jsonl",
			`{"version":"v1","kind":"Prompt","data":{"content":[{"kind":"text","data":"hi"}]}}`+"\n")
	}
	mk("a", "/home/x/proj", "2026-01-03T00:00:00Z")
	mk("b", "/home/x/proj/", "2026-01-02T00:00:00Z") // trailing slash, same dir
	mk("c", "/home/x/other", "2026-01-01T00:00:00Z")
	return cli
}

func TestLoadAllFilteredByCwd(t *testing.T) {
	setupSessions(t)
	before := CurrentConfig()
	t.Cleanup(func() { SetConfig(before) })
	cfg := DefaultConfig
	cfg.SQLiteEnabled = false
	SetConfig(cfg)

	all := LoadAllFiltered(LoadFilter{})
	if len(all) != 3 {
		t.Fatalf("unfiltered = %d, want 3", len(all))
	}

	got := LoadAllFiltered(LoadFilter{Cwd: "/home/x/proj"})
	ids := map[string]bool{}
	for _, s := range got {
		ids[s.SessionID] = true
	}
	if len(got) != 2 || !ids["a"] || !ids["b"] {
		t.Fatalf("filtered = %v, want a and b (trailing slash tolerant)", ids)
	}

	// Trailing slash on the filter itself also matches.
	if n := len(LoadAllFiltered(LoadFilter{Cwd: "/home/x/proj/"})); n != 2 {
		t.Fatalf("filter with trailing slash = %d, want 2", n)
	}

	if n := len(LoadAllFiltered(LoadFilter{Cwd: "/nowhere"})); n != 0 {
		t.Fatalf("non-matching filter = %d, want 0", n)
	}
}

func TestWorkspaces(t *testing.T) {
	s := Session{
		Cwd:            "/a/project/",
		WorkspacePaths: []string{"", "/a/project", "/b/project", "/b/project/"},
	}
	got := s.Workspaces()
	if len(got) != 2 || got[0] != "/a/project" || got[1] != "/b/project" {
		t.Fatalf("unexpected workspaces: %v", got)
	}
	if paths := (&Session{}).Workspaces(); len(paths) != 0 {
		t.Fatalf("empty session has workspaces: %v", paths)
	}
	for _, root := range []string{"/", `C:\`, "C:/"} {
		paths := (&Session{Cwd: root}).Workspaces()
		if len(paths) != 1 || paths[0] != root {
			t.Errorf("workspace root %q became %v", root, paths)
		}
	}
}
