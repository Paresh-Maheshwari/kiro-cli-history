package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"kiro-cli-history/internal/session"
	"kiro-cli-history/internal/terminaltext"
)

// ViewMode controls left sidebar display.
type ViewMode int

const (
	ViewList ViewMode = iota
	ViewTree
)

// TreeNode is a directory, a session row, or a session nested under a row
// (older rewind version or subagent).
type TreeNode struct {
	Name     string // directory name or session title
	Path     string // full directory path
	Session  *session.Session
	Children []*TreeNode
	Parent   *TreeNode
	Depth    int
	Expanded bool
	IsDir    bool
	Rel      string // "version" or "subagent" for nested sessions
}

// Key identifies a node across rebuilds (to keep expand state and cursor).
func (n *TreeNode) Key() string {
	if n.IsDir {
		return "d:" + n.Path
	}
	if n.Session != nil {
		return "s:" + n.Session.SessionID + "|" + n.Session.Cwd
	}
	return ""
}

// BuildTree groups rows by workspace, keeping child sessions under their
// parents. autoExpand reveals matching descendants while searching.
func BuildTree(rows []session.Session, children map[string][]session.Session, autoExpand bool) []*TreeNode {
	return buildWorkspaceTree(rows, children, autoExpand, "")
}

func buildWorkspaceTree(rows []session.Session, children map[string][]session.Session, autoExpand bool, cwdFilter string) []*TreeNode {
	groups := make(map[string][]session.Session)
	for _, row := range rows {
		paths := row.Workspaces()
		if cwdFilter != "" {
			paths = []string{row.Cwd}
		}
		if len(paths) == 0 || len(paths) == 1 && paths[0] == "" {
			paths = []string{"(unknown)"}
		}
		for _, path := range paths {
			s := row
			if path != "(unknown)" {
				s.Cwd = path
			}
			groups[path] = append(groups[path], s)
		}
	}
	dirs := make([]string, 0, len(groups))
	names := make(map[string]int)
	for d := range groups {
		dirs = append(dirs, d)
		names[filepath.Base(d)]++
	}
	sort.Strings(dirs)

	tree := make([]*TreeNode, 0, len(dirs))
	for _, dir := range dirs {
		name := filepath.Base(dir)
		if names[name] > 1 {
			name = dir // same folder name in different workspaces
		}
		dn := &TreeNode{Name: name, Path: dir, IsDir: true, Expanded: autoExpand}
		for _, row := range groups[dir] {
			sn := buildSessionTree(row, children[row.SessionID], autoExpand)
			sn.Parent = dn
			dn.Children = append(dn.Children, sn)
		}
		tree = append(tree, dn)
	}
	return tree
}

func buildSessionTree(row session.Session, children []session.Session, autoExpand bool) *TreeNode {
	root := &TreeNode{Name: row.Title, Session: &row, Depth: 1, Expanded: autoExpand}
	if row.SessionID == "" {
		return root
	}
	byID := map[string]*TreeNode{row.SessionID: root}
	newest := map[string]*TreeNode{row.GroupID: root}
	var nodes []*TreeNode
	for _, child := range children {
		c := child
		for _, path := range c.Workspaces() {
			if path == row.Cwd {
				c.Cwd = path
				break
			}
		}
		rel := "subagent"
		if session.IsVersionOf(&c, &row) {
			rel = "version"
		}
		n := &TreeNode{Name: c.Title, Session: &c, Rel: rel, Expanded: autoExpand}
		nodes = append(nodes, n)
		byID[c.SessionID] = n
		if c.GroupID != "" && newest[c.GroupID] == nil {
			newest[c.GroupID] = n
		}
	}
	for _, n := range nodes {
		n.Parent = root
		if representative := newest[n.Session.GroupID]; n.Session.GroupID != "" && representative != nil && representative != n {
			n.Parent = representative
			n.Rel = "version"
		} else {
			parentID := n.Session.ParentID
			if n.Session.IsSubagent() {
				parentID = n.Session.SubagentOwnerID()
			}
			if parent := byID[parentID]; parent != nil && parent != n {
				n.Parent = parent
			}
		}
	}
	// Malformed parent cycles must not make sessions disappear or recurse forever.
	for _, n := range nodes {
		for p, depth := n.Parent, 0; p != root && p != nil; p, depth = p.Parent, depth+1 {
			if p == n || depth > len(nodes) {
				n.Parent = root
				break
			}
		}
	}
	for _, n := range nodes {
		n.Parent.Children = append(n.Parent.Children, n)
	}
	var setDepth func(*TreeNode)
	setDepth = func(n *TreeNode) {
		for _, c := range n.Children {
			c.Depth = n.Depth + 1
			setDepth(c)
		}
	}
	setDepth(root)
	return root
}

// FlattenTree returns visible nodes (respecting expanded/collapsed state).
func FlattenTree(tree []*TreeNode) []*TreeNode {
	var flat []*TreeNode
	var walk func([]*TreeNode)
	walk = func(nodes []*TreeNode) {
		for _, n := range nodes {
			flat = append(flat, n)
			if n.Expanded {
				walk(n.Children)
			}
		}
	}
	walk(tree)
	return flat
}

// fit truncates s (which may contain ANSI styles) to w terminal cells.
func fit(s string, w int) string {
	if w < 1 {
		w = 1
	}
	return ansi.Truncate(s, w, "…")
}

// relBadges renders "↺3 ⑂6" style counts for a row with nested sessions.
func relBadges(s *session.Session) string {
	var b []string
	if s.Versions > 0 {
		b = append(b, fmt.Sprintf("↺%d", s.Versions))
	}
	if s.Subagents > 0 {
		b = append(b, fmt.Sprintf("⑂%d", s.Subagents))
	}
	return strings.Join(b, " ")
}

// RenderTreeNode renders one node for display in the sidebar.
func RenderTreeNode(node *TreeNode, width int, selected bool) string {
	if width < 10 {
		width = 10
	}
	indent := strings.Repeat("  ", node.Depth)
	icon := " "
	if len(node.Children) > 0 {
		icon = "▸"
		if node.Expanded {
			icon = "▾"
		}
	}

	if node.IsDir {
		count := formatCount(len(node.Children))
		if selected {
			return SelectedStyle.Width(width).Render(fit(fmt.Sprintf("%s%s %s (%s)", indent, icon, terminaltext.Line(node.Name), count), width))
		}
		head := indent + icon + " 📁 "
		tail := " " + count
		name := fit(terminaltext.Line(node.Name), width-ansi.StringWidth(head)-ansi.StringWidth(tail))
		return fit(head+name+DimStyle.Render(tail), width)
	}

	prefix := ""
	switch node.Rel {
	case "version":
		prefix = "↺ "
	case "subagent":
		prefix = "⑂ "
	}
	head := indent + icon + " " + prefix
	badge := ""
	if node.Session != nil {
		if b := relBadges(node.Session); b != "" {
			badge = " " + b
		}
	}
	title := fit(terminaltext.Line(node.Name), width-ansi.StringWidth(head)-ansi.StringWidth(badge))
	if selected {
		return SelectedStyle.Width(width).Render(fit(head+title+badge, width))
	}
	return fit(head+DimStyle.Render(title)+CyanStyle.Render(badge), width)
}

func formatCount(n int) string {
	if n == 1 {
		return "1 chat"
	}
	return fmt.Sprintf("%d chats", n)
}
