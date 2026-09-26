package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"kiro-cli-history/internal/session"
)

func TestRenderTreeNodeNeverExceedsWidth(t *testing.T) {
	long := strings.Repeat("नमस्ते 日本語 emoji 🚀 ", 10)
	s := &session.Session{SessionID: "x", Title: long, Versions: 3, Subagents: 12}
	nodes := []*TreeNode{
		{Name: long, Path: "/p", IsDir: true, Children: make([]*TreeNode, 3)},
		{Name: long, Session: s, Depth: 1, Children: make([]*TreeNode, 2)},
		{Name: long, Session: s, Depth: 2, Rel: "version"},
		{Name: "", Session: &session.Session{}, Depth: 2, Rel: "subagent"},
	}
	for _, w := range []int{0, 5, 10, 20, 28, 60, 200} {
		for _, n := range nodes {
			for _, sel := range []bool{false, true} {
				out := RenderTreeNode(n, w, sel)
				limit := w
				if limit < 10 {
					limit = 10
				}
				if got := ansi.StringWidth(out); got > limit {
					t.Fatalf("width=%d sel=%v depth=%d: rendered %d cells: %q", w, sel, n.Depth, got, out)
				}
			}
		}
	}
}

func TestBuildTreeNestsChildren(t *testing.T) {
	rows := []session.Session{
		{SessionID: "r2", Cwd: "/b", GroupID: "orig", Versions: 1, Subagents: 1},
		{SessionID: "m", Cwd: "/a"},
	}
	children := map[string][]session.Session{
		"r2": {
			{SessionID: "orig", Cwd: "/b", GroupID: "orig"},
			{SessionID: "sa", Cwd: "/b", GroupID: "sa", CreatedReason: "subagent", ParentID: "orig"},
		},
	}
	tree := BuildTree(rows, children, false)
	if len(tree) != 2 || tree[0].Path != "/a" || tree[1].Path != "/b" {
		t.Fatalf("dirs not sorted: %+v", tree)
	}
	row := tree[1].Children[0]
	if len(row.Children) != 2 || row.Children[0].Rel != "version" || row.Children[1].Rel != "subagent" {
		t.Fatalf("children wrong: %+v", row.Children)
	}
	if row.Children[0].Parent != row || row.Parent != tree[1] {
		t.Fatal("parent links wrong")
	}

	if n := len(FlattenTree(tree)); n != 2 {
		t.Fatalf("collapsed flatten = %d, want 2", n)
	}
	tree[1].Expanded, row.Expanded = true, true
	if n := len(FlattenTree(tree)); n != 5 {
		t.Fatalf("expanded flatten = %d, want 5", n)
	}
}

func TestModelTreeNavigationAndSelection(t *testing.T) {
	session.AppConfig = session.DefaultConfig
	m := NewModel()
	m.W, m.H = 120, 40
	all := []session.Session{
		{SessionID: "r1", Cwd: "/w", Title: "new", CreatedReason: "rewind", ParentID: "o"},
		{SessionID: "o", Cwd: "/w", Title: "old"},
		{SessionID: "z", Cwd: "/z", Title: "zed"},
	}
	session.Link(all)
	m.All = all
	m.Loading = false
	m.refilter(true)
	if len(m.Filtered) != 2 || m.Filtered[0].Versions != 1 {
		t.Fatalf("rows=%d", len(m.Filtered))
	}

	m.ViewMode = ViewTree
	m.rebuildTree(true)
	if m.current() != nil {
		t.Fatal("directory node should have no current session")
	}
	m.selectInTree(&m.Filtered[0])
	if s := m.current(); s == nil || s.SessionID != "r1" {
		t.Fatalf("selectInTree -> %+v", s)
	}

	// Expand the row, move to the nested old version: actions must target it.
	m.FlatTree[m.TreeCursor].Expanded = true
	m.FlatTree = FlattenTree(m.Tree)
	m.moveCursor(m.TreeCursor + 1)
	if s := m.current(); s == nil || s.SessionID != "o" {
		t.Fatalf("nested selection -> %+v", s)
	}

	// Rebuilding (e.g. after indexing) keeps expansion and selection.
	m.rebuildTree(false)
	if s := m.current(); s == nil || s.SessionID != "o" {
		t.Fatalf("after rebuild -> %+v", s)
	}

	// Clamp at both ends without panicking.
	m.moveCursor(-5)
	m.moveCursor(1 << 30)
	if m.TreeCursor != len(m.FlatTree)-1 {
		t.Fatalf("clamp: %d", m.TreeCursor)
	}

	// Empty data must not panic in any view.
	m.All = nil
	m.refilter(true)
	_ = m.View()
	m.ViewMode = ViewList
	_ = m.View()
}
