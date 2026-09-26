package ui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"kiro-cli-history/internal/session"
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

// BuildTree groups rows by cwd; children (from session.Collapse) are nested
// under their row. autoExpand opens all directories (used while searching).
func BuildTree(rows []session.Session, children map[string][]session.Session, autoExpand bool) []*TreeNode {
	groups := make(map[string][]int)
	for i := range rows {
		cwd := rows[i].Cwd
		if cwd == "" {
			cwd = "(unknown)"
		}
		groups[cwd] = append(groups[cwd], i)
	}
	dirs := make([]string, 0, len(groups))
	for d := range groups {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	tree := make([]*TreeNode, 0, len(dirs))
	for _, dir := range dirs {
		dn := &TreeNode{Name: filepath.Base(dir), Path: dir, IsDir: true, Expanded: autoExpand}
		for _, i := range groups[dir] {
			row := &rows[i]
			sn := &TreeNode{Name: row.Title, Session: row, Parent: dn, Depth: 1}
			if row.SessionID != "" {
				kids := children[row.SessionID]
				for j := range kids {
					c := &kids[j]
					rel := "subagent"
					if session.IsVersionOf(c, row) {
						rel = "version"
					}
					sn.Children = append(sn.Children, &TreeNode{
						Name: c.Title, Session: c, Parent: sn, Depth: 2, Rel: rel,
					})
				}
			}
			dn.Children = append(dn.Children, sn)
		}
		tree = append(tree, dn)
	}
	return tree
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
			return SelectedStyle.Width(width).Render(fit(fmt.Sprintf("%s%s %s (%s)", indent, icon, node.Name, count), width))
		}
		head := indent + icon + " 📁 "
		tail := " " + count
		name := fit(node.Name, width-ansi.StringWidth(head)-ansi.StringWidth(tail))
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
	title := fit(node.Name, width-ansi.StringWidth(head)-ansi.StringWidth(badge))
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
