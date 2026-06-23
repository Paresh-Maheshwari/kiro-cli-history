package ui

import (
	"fmt"
	"path/filepath"
	"sort"

	"kiro-cli-history/internal/session"
)

// ViewMode controls left sidebar display.
type ViewMode int

const (
	ViewList ViewMode = iota
	ViewTree
)

// TreeNode represents a directory or session in the tree.
type TreeNode struct {
	Name     string // directory name or session title
	Path     string // full directory path
	Session  *session.Session
	Children []*TreeNode
	Expanded bool
	IsDir    bool
}

// BuildTree groups sessions by cwd into a tree.
// If expanded is true, all directories start expanded (used during search).
func BuildTree(sessions []session.Session, expanded ...bool) []*TreeNode {
	autoExpand := len(expanded) > 0 && expanded[0]
	// Group by cwd
	groups := make(map[string][]session.Session)
	for i := range sessions {
		cwd := sessions[i].Cwd
		if cwd == "" {
			cwd = "(unknown)"
		}
		groups[cwd] = append(groups[cwd], sessions[i])
	}

	// Sort directories
	dirs := make([]string, 0, len(groups))
	for d := range groups {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)

	var tree []*TreeNode
	for _, dir := range dirs {
		dirNode := &TreeNode{
			Name:     filepath.Base(dir),
			Path:     dir,
			IsDir:    true,
			Expanded: autoExpand,
		}
		for i := range groups[dir] {
			s := groups[dir][i]
			dirNode.Children = append(dirNode.Children, &TreeNode{
				Name:    s.Title,
				Session: &s,
				IsDir:   false,
			})
		}
		tree = append(tree, dirNode)
	}
	return tree
}

// FlattenTree returns visible nodes (respecting expanded/collapsed state).
func FlattenTree(tree []*TreeNode) []*TreeNode {
	var flat []*TreeNode
	for _, node := range tree {
		flat = append(flat, node)
		if node.IsDir && node.Expanded {
			for _, child := range node.Children {
				flat = append(flat, child)
			}
		}
	}
	return flat
}

// RenderTreeNode renders one node for display in the sidebar.
func RenderTreeNode(node *TreeNode, width int, selected bool) string {
	if width < 20 {
		width = 20
	}
	if node.IsDir {
		icon := "▸"
		if node.Expanded {
			icon = "▾"
		}
		name := node.Name
		count := formatCount(len(node.Children))
		maxName := width - 16 // room for icon + 📁 + count + padding
		if maxName < 4 {
			maxName = 4
		}
		if len(name) > maxName {
			name = name[:maxName] + "…"
		}
		if selected {
			return SelectedStyle.Width(width).Render(fmt.Sprintf("%s %s (%s)", icon, name, count))
		}
		return fmt.Sprintf("%s 📁 %s %s", icon, name, DimStyle.Render(count))
	}

	// Session node (indented)
	title := node.Name
	maxTitle := width - 8 // room for indent + ellipsis
	if maxTitle < 4 {
		maxTitle = 4
	}
	if len(title) > maxTitle {
		title = title[:maxTitle] + "…"
	}
	if selected {
		return SelectedStyle.Width(width).Render("    " + title)
	}
	return "    " + DimStyle.Render(title)
}

func formatCount(n int) string {
	if n == 1 {
		return "1 chat"
	}
	return fmt.Sprintf("%d chats", n)
}
