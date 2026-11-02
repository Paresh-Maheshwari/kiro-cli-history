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
	if len(row.Children) != 1 || row.Children[0].Rel != "version" {
		t.Fatalf("children wrong: %+v", row.Children)
	}
	old := row.Children[0]
	if len(old.Children) != 1 || old.Children[0].Rel != "subagent" || old.Children[0].Depth != 3 {
		t.Fatalf("subagent must stay under its old-version parent: %+v", old.Children)
	}
	if row.Children[0].Parent != row || row.Parent != tree[1] {
		t.Fatal("parent links wrong")
	}

	if n := len(FlattenTree(tree)); n != 2 {
		t.Fatalf("collapsed flatten = %d, want 2", n)
	}
	tree[1].Expanded, row.Expanded = true, true
	if n := len(FlattenTree(tree)); n != 4 {
		t.Fatalf("child must stay collapsed with its parent: %d nodes", n)
	}
	old.Expanded = true
	if n := len(FlattenTree(tree)); n != 5 {
		t.Fatalf("expanded flatten = %d, want 5", n)
	}
}

func TestWorkspaceTreeMultiRootAndFilter(t *testing.T) {
	rows := []session.Session{
		{SessionID: "multi", Cwd: "/a/project", WorkspacePaths: []string{"/a/project/", "/b/project"}},
	}
	tree := BuildTree(rows, nil, false)
	if len(tree) != 2 || tree[0].Name != "/a/project" || tree[1].Name != "/b/project" {
		t.Fatalf("multi-root directories must be distinct: %+v", tree)
	}
	a, b := tree[0].Children[0], tree[1].Children[0]
	if a.Session.Cwd != "/a/project" || b.Session.Cwd != "/b/project" || a.Key() == b.Key() {
		t.Fatalf("row must use its displayed workspace: %+v %+v", a.Session, b.Session)
	}
	if rows[0].Cwd != "/a/project" {
		t.Fatal("building a tree mutated the input session")
	}
	if previewKey(a.Session, 80) == previewKey(b.Session, 80) {
		t.Fatal("preview cache must distinguish workspaces")
	}
	rows[0].Cwd = "/b/project"
	filtered := buildWorkspaceTree(rows, nil, false, "/b/project")
	if len(filtered) != 1 || filtered[0].Path != "/b/project" {
		t.Fatalf("directory-filtered view leaked another workspace: %+v", filtered)
	}
}

func TestWorkspaceTreeSearchRevealsNestedChildren(t *testing.T) {
	rows := []session.Session{{SessionID: "root", Cwd: "/w"}}
	children := map[string][]session.Session{
		"root": {
			{SessionID: "grandchild", ParentID: "child", Cwd: "/w"},
			{SessionID: "child", ParentID: "root", Cwd: "/w"},
		},
	}
	collapsed := BuildTree(rows, children, false)
	if len(FlattenTree(collapsed)) != 1 {
		t.Fatal("workspace should start collapsed")
	}
	collapsed[0].Expanded = true
	if len(FlattenTree(collapsed)) != 2 {
		t.Fatal("children should start collapsed under their chat")
	}
	open := BuildTree(rows, children, true)
	flat := FlattenTree(open)
	if len(flat) != 4 || flat[3].Session.SessionID != "grandchild" || flat[3].Depth != 3 {
		t.Fatalf("search must expose every matched child under its parent: %+v", flat)
	}
	if flat[3].Parent != flat[2] {
		t.Fatal("grandchild has wrong parent")
	}
}

func TestWorkspaceTreeMalformedChildLinksRemainReachable(t *testing.T) {
	rows := []session.Session{{SessionID: "root", Cwd: "/w"}}
	children := map[string][]session.Session{
		"root": {
			{SessionID: "a", ParentID: "b"},
			{SessionID: "b", ParentID: "a"},
			{SessionID: "orphan", ParentID: "missing"},
		},
	}
	flat := FlattenTree(BuildTree(rows, children, true))
	if len(flat) != 5 {
		t.Fatalf("all sessions must remain reachable: %d nodes", len(flat))
	}
}

func TestDefaultViewGroupsWorkspaces(t *testing.T) {
	before := session.CurrentConfig()
	t.Cleanup(func() { session.SetConfig(before) })
	session.SetConfig(session.DefaultConfig)
	if NewModel().ViewMode != ViewTree {
		t.Fatal("default view should group workspaces")
	}
	cfg := session.CurrentConfig()
	cfg.DefaultView = "list"
	session.SetConfig(cfg)
	if NewModel().ViewMode != ViewList {
		t.Fatal("explicit list preference must be respected")
	}
}

func TestTreeNestsRewoundSubagentFamily(t *testing.T) {
	all := []session.Session{
		{SessionID: "rewind", ParentID: "child", CreatedReason: "rewind", Cwd: "/w"},
		{SessionID: "child", ParentID: "root", CreatedReason: "subagent", Cwd: "/w"},
		{SessionID: "root", Cwd: "/w"},
	}
	session.Link(all)
	rows, children := session.Collapse(all, true, true)
	tree := BuildTree(rows, children, true)
	root := tree[0].Children[0]
	if root.Session.SessionID != "root" || len(root.Children) != 1 {
		t.Fatal("rewound subagent is outside its owner")
	}
	worker := root.Children[0]
	if worker.Session.SessionID != "rewind" || len(worker.Children) != 1 || worker.Children[0].Session.SessionID != "child" {
		t.Fatal("older subagent version is not nested under the current version")
	}
}

func TestSearchRestoresCollapsedChildren(t *testing.T) {
	before := session.CurrentConfig()
	t.Cleanup(func() { session.SetConfig(before) })
	session.SetConfig(session.DefaultConfig)
	m := NewModel()
	m.W, m.H = 120, 40
	m.Loading = false
	m.All = []session.Session{
		{SessionID: "parent", Cwd: "/w", Title: "match parent", SearchText: "match parent"},
		{SessionID: "child", Cwd: "/w", Title: "match child", SearchText: "match child", CreatedReason: "subagent", ParentID: "parent"},
	}
	session.Link(m.All)
	m.refilter(true)
	m.Tree[0].Expanded = true
	m.FlatTree = FlattenTree(m.Tree)
	if len(m.FlatTree) != 2 {
		t.Fatal("child should initially be hidden under the collapsed parent")
	}
	m.Input.SetValue("match")
	m.refilter(true)
	if len(m.FlatTree) != 3 || m.FlatTree[2].Session.SessionID != "child" {
		t.Fatal("search should reveal matching nested child")
	}
	m.Input.SetValue("")
	m.refilter(true)
	if len(m.FlatTree) != 2 || m.Tree[0].Children[0].Expanded {
		t.Fatal("clearing search should restore the user's collapsed parent")
	}
}

func TestTreeRebuildPreservesHiddenExpansion(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewTree
	m.All = []session.Session{
		{SessionID: "root", Cwd: "/w"},
		{SessionID: "child", ParentID: "root", CreatedReason: "subagent", Cwd: "/w"},
	}
	session.Link(m.All)
	m.refilter(true)
	// The child group was expanded before its entire workspace was collapsed.
	m.Tree[0].Children[0].Expanded = true
	m.Tree[0].Expanded = false
	m.rebuildTree(false)
	if !m.Tree[0].Children[0].Expanded || m.Tree[0].Expanded {
		t.Fatal("rebuild forgot expansion state inside a collapsed workspace")
	}
}

func TestModelTreeNavigationAndSelection(t *testing.T) {
	session.SetConfig(session.DefaultConfig)
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
