package session

import (
	"sort"
	"testing"
)

// mk builds a session; list order in tests is newest first (as LoadAll sorts).
func mk(id, reason, parent string) Session {
	return Session{SessionID: id, Title: "t-" + id, Cwd: "/w", CreatedReason: reason, ParentID: parent}
}

func rowIDs(rows []Session) []string {
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.SessionID)
	}
	return ids
}

func childIDs(ch map[string][]Session, id string) []string {
	var ids []string
	for _, c := range ch[id] {
		ids = append(ids, c.SessionID)
	}
	sort.Strings(ids)
	return ids
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestLinkRewindChain(t *testing.T) {
	// orig <- r1 <- r2, plus r3 rewound from orig (tree, not a chain).
	all := []Session{
		mk("r2", "rewind", "r1"),
		mk("r3", "rewind", "orig"),
		mk("r1", "rewind", "orig"),
		mk("orig", "subagent", ""), // top-level sessions also say "subagent"
		mk("other", "subagent", ""),
	}
	Link(all)
	for _, s := range all[:4] {
		if s.GroupID != "orig" {
			t.Fatalf("%s GroupID = %q, want orig", s.SessionID, s.GroupID)
		}
	}
	if all[4].GroupID != "other" {
		t.Fatalf("unrelated session grouped: %q", all[4].GroupID)
	}
	if all[0].ParentTitle != "t-r1" {
		t.Fatalf("ParentTitle = %q", all[0].ParentTitle)
	}
}

func TestLinkMissingOriginalAndCycle(t *testing.T) {
	all := []Session{
		mk("b", "rewind", "a"),
		mk("a", "rewind", "deleted"), // original no longer on disk
		mk("x", "rewind", "y"),
		mk("y", "rewind", "x"), // malformed cycle must not hang
	}
	Link(all)
	if all[0].GroupID != "deleted" || all[1].GroupID != "deleted" {
		t.Fatalf("got %q %q, want deleted", all[0].GroupID, all[1].GroupID)
	}
	if all[2].GroupID == "" || all[3].GroupID == "" {
		t.Fatal("cycle produced empty group")
	}
}

func TestCollapse(t *testing.T) {
	all := []Session{
		mk("r2", "rewind", "r1"),
		mk("sa1", "subagent", "main"),
		mk("r1", "rewind", "orig"),
		mk("main", "subagent", ""),
		mk("orig", "subagent", ""),
		mk("sa2", "subagent", "r1"), // subagent of an older rewind version
		mk("lonely", "subagent", "gone"),
	}
	Link(all)

	t.Run("both on", func(t *testing.T) {
		rows, ch := Collapse(all, true, true)
		if want := []string{"r2", "main", "lonely"}; !eq(rowIDs(rows), want) {
			t.Fatalf("rows = %v, want %v", rowIDs(rows), want)
		}
		if got := childIDs(ch, "r2"); !eq(got, []string{"orig", "r1", "sa2"}) {
			t.Fatalf("r2 children = %v", got)
		}
		if got := childIDs(ch, "main"); !eq(got, []string{"sa1"}) {
			t.Fatalf("main children = %v", got)
		}
		if rows[0].Versions != 2 || rows[0].Subagents != 1 {
			t.Fatalf("r2 counts v=%d sa=%d", rows[0].Versions, rows[0].Subagents)
		}
		if rows[1].Subagents != 1 || rows[1].Versions != 0 {
			t.Fatalf("main counts v=%d sa=%d", rows[1].Versions, rows[1].Subagents)
		}
	})

	t.Run("both off", func(t *testing.T) {
		rows, ch := Collapse(all, false, false)
		if len(rows) != len(all) || len(ch) != 0 {
			t.Fatalf("rows=%d children=%d", len(rows), len(ch))
		}
		for _, r := range rows {
			if r.Versions != 0 || r.Subagents != 0 {
				t.Fatalf("%s has counts with grouping off", r.SessionID)
			}
		}
	})

	t.Run("only rewinds", func(t *testing.T) {
		rows, _ := Collapse(all, true, false)
		if want := []string{"r2", "sa1", "main", "sa2", "lonely"}; !eq(rowIDs(rows), want) {
			t.Fatalf("rows = %v, want %v", rowIDs(rows), want)
		}
	})

	t.Run("search hit is never hidden without its owner", func(t *testing.T) {
		// Only the subagent matched the search: it must be a visible row.
		rows, ch := Collapse([]Session{all[1]}, true, true)
		if !eq(rowIDs(rows), []string{"sa1"}) || len(ch) != 0 {
			t.Fatalf("rows=%v children=%d", rowIDs(rows), len(ch))
		}
		// Only an old rewind version matched: it becomes the row for its family.
		rows, _ = Collapse([]Session{all[2], all[4]}, true, true)
		if !eq(rowIDs(rows), []string{"r1"}) {
			t.Fatalf("rows=%v", rowIDs(rows))
		}
	})

	t.Run("does not mutate input", func(t *testing.T) {
		Collapse(all, true, true)
		for _, s := range all {
			if s.Versions != 0 || s.Subagents != 0 {
				t.Fatalf("input mutated: %s", s.SessionID)
			}
		}
	})
}

func TestCollapseSQLiteSessionsWithoutID(t *testing.T) {
	// sqlite_v1 sessions have no SessionID; they must stay as separate rows.
	all := []Session{{Cwd: "/a", Source: "sqlite_v1"}, {Cwd: "/b", Source: "sqlite_v1"}}
	Link(all)
	rows, ch := Collapse(all, true, true)
	if len(rows) != 2 || len(ch) != 0 {
		t.Fatalf("rows=%d children=%d", len(rows), len(ch))
	}
}
