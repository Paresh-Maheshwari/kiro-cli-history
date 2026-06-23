package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	activeBorder  = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, true, false, false).BorderForeground(lipgloss.Color("62"))
	previewActive = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, false, false, true).BorderForeground(lipgloss.Color("62"))
)

func (m Model) View() string {
	if m.W == 0 || m.Loading {
		return RenderSplash(m.W, m.H, m.Spinner.View())
	}

	if m.ShowHelp {
		return fillScreen(RenderHelp(m.W, m.H, m.HelpScroll), m.W, m.H)
	}

	if m.ShowSettings {
		return fillScreen(RenderSettings(m.W, m.H, 0), m.W, m.H)
	}

	// Fullscreen preview mode — no sidebar
	if m.Fullscreen {
		body := m.Preview.View()

		var modeFull = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("3")).Background(lipgloss.Color("236")).Padding(0, 1)
		fsep := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(" │ ")
		fkey := lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
		fdesc := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
		fh := func(k, d string) string { return fkey.Render(k) + fdesc.Render(d) }
		st := " " + modeFull.Render(" FULLSCREEN ") + " " +
			fh("j/k", " scroll") + fsep + fh("d/u", " page") + fsep + fh("g/G", " top/bottom") + fsep + fh("f", "/") + fh("Esc", " exit")
		if m.Note != "" && time.Now().Before(m.NoteExpiry) {
			st = " " + m.Note
		}
		status := StatusStyle.Width(m.W).Render(st)
		return body + "\n" + status
	}

	lw := m.LeftW()
	lh := m.ListH()

	// Search bar
	m.Input.Width = lw - 8 // account for icon + border + padding
	icon := SearchIcon.Render(" ") + " "
	inputView := icon + m.Input.View()
	box := SearchBox.Width(lw - 2)
	if m.Focus == FocusSearch {
		box = SearchBoxActive.Width(lw - 2)
	}
	searchBar := box.Render(inputView)

	// Sidebar content (list or tree view)
	var lines []string
	if m.ViewMode == ViewTree && len(m.FlatTree) > 0 {
		// Tree view
		start := 0
		maxVis := lh
		if m.TreeCursor >= maxVis {
			start = m.TreeCursor - maxVis + 1
		}
		for i := start; i < len(m.FlatTree) && len(lines) < lh; i++ {
			node := m.FlatTree[i]
			lines = append(lines, RenderTreeNode(node, lw-2, i == m.TreeCursor))
		}
	} else {
		// List view
		perItem := 3
		maxVis := lh / perItem
		if maxVis < 1 {
			maxVis = 1
		}
		start := 0
		if m.Cursor >= maxVis {
			start = m.Cursor - maxVis + 1
		}
		for i := start; i < len(m.Filtered) && len(lines) < lh; i++ {
			s := m.Filtered[i]
			title := s.Title
			if len(title) > lw-4 {
				title = title[:lw-7] + "..."
			}
			cwd := filepath.Base(s.Cwd)
			date := FmtDate(s.UpdatedAt)
			msgs := fmt.Sprintf("%d msgs", s.MsgCount)
			dur := FmtDur(s.DurationMin)
			meta := cwd + "  " + date + "  " + msgs + "  " + dur

			if i == m.Cursor {
				lines = append(lines,
					SelectedStyle.Width(lw-2).Render(title),
					SelectedStyle.Width(lw-2).Render(meta),
					"")
			} else {
				lines = append(lines,
					TitleStyle.Render(title),
					DimStyle.Render(cwd)+"  "+DimStyle.Render(date)+"  "+CyanStyle.Render(msgs)+"  "+GreenStyle.Render(dur),
					"")
			}
		}
	}
	for len(lines) < lh {
		lines = append(lines, "")
	}
	lines = lines[:lh]

	// Use active border style when list/search is focused
	leftBorder := BorderStyle
	if m.Focus == FocusList || m.Focus == FocusSearch {
		leftBorder = activeBorder
	}
	left := leftBorder.Width(lw).Render(searchBar + "\n" + strings.Join(lines, "\n"))

	// Preview pane
	rw := m.RightW()
	prevContent := m.Preview.View()
	rightStyle := lipgloss.NewStyle().Width(rw)
	if m.Focus == FocusPreview {
		rightStyle = previewActive.Width(rw)
	}
	right := rightStyle.Render(prevContent)

	body := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	// Status bar
	var (
		modeSearch  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Background(lipgloss.Color("236")).Padding(0, 1)
		modeList    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2")).Background(lipgloss.Color("236")).Padding(0, 1)
		modePreview = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("5")).Background(lipgloss.Color("236")).Padding(0, 1)
		sep         = lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render(" │ ")
		hintKey     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
		hintDesc    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	)
	hint := func(k, d string) string { return hintKey.Render(k) + hintDesc.Render(d) }

	count := fmt.Sprintf(" %d/%d", len(m.Filtered), len(m.All))
	if m.Indexing {
		count += " ⟳"
	}
	if q := m.Input.Value(); q != "" {
		count += DimStyle.Render(fmt.Sprintf(" '%s'", q))
	}

	var mode, hints string
	switch m.Focus {
	case FocusSearch:
		mode = modeSearch.Render(" SEARCH ")
		hints = hint("Enter", " list") + sep + hint("Tab", " preview") + sep + hint("Ctrl+R", " resume") + sep + hint("?", " help")
	case FocusList:
		mode = modeList.Render(" LIST ")
		viewHint := "v tree"
		if m.ViewMode == ViewTree {
			viewHint = "v list"
		}
		hints = hint("/", " search") + sep + hint("l", " preview") + sep + hint("f", " full") + sep + hint(viewHint, "") + sep + hint("s", " settings") + sep + hint("?", " help")
	case FocusPreview:
		mode = modePreview.Render(" PREVIEW ")
		hints = hint("j/k", " scroll") + sep + hint("d/u", " page") + sep + hint("f", " full") + sep + hint("h", " back") + sep + hint("s", " settings") + sep + hint("?", " help")
	}

	st := count + " " + mode + " " + hints
	if m.Note != "" && time.Now().Before(m.NoteExpiry) {
		st = " " + m.Note
	}
	status := StatusStyle.Width(m.W).Render(st)

	return body + "\n" + status
}

func FmtDate(s string) string {
	if len(s) < 10 {
		return s
	}
	if t, err := time.Parse("2006-01-02", s[:10]); err == nil {
		return t.Format("2 Jan 2006")
	}
	return s[:10]
}

func FmtDur(m int) string {
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh %dm", m/60, m%60)
}

// fillScreen pads or trims content to exactly h lines of width w.
func fillScreen(content string, w, h int) string {
	lines := strings.Split(content, "\n")
	blank := strings.Repeat(" ", w)
	for len(lines) < h {
		lines = append(lines, blank)
	}
	return strings.Join(lines[:h], "\n")
}
