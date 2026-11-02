package ui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"kiro-cli-history/internal/session"
	"kiro-cli-history/internal/terminaltext"
)

// Header card styles
var (
	headerBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)

	headerTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("62")).
			Padding(0, 1)

	headerVal = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	headerDim = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	headerIcon = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62"))

	youBubble = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("6")).
			Padding(0, 1)

	kiroBubble = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("2")).
			Padding(0, 1)

	msgCounter = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240")).
			Italic(true)
)

const (
	maxPreviewText     = 512 * 1024
	maxMarkdownMessage = 8 * 1024
	maxPreviewOutput   = 4 * 1024 * 1024
	maxPreviewCache    = 20 * 1024 * 1024
)

// previewKey identifies a rendered preview in PrevCache.
func previewKey(s *session.Session, width int) string {
	// Header counts and width change the output, so they are part of the key.
	return fmt.Sprintf("%s|%s|%s|%s|%d|%d|%d|%d", s.Source, s.SessionID, s.JSONLPath, s.Cwd, s.Versions, s.Subagents, s.MsgCount, width)
}

// RefreshPreview shows the selected session. Cached previews are shown as is;
// otherwise all file access and Markdown work runs in a cancellable command.
func (m *Model) RefreshPreview() {
	gen := m.prevGen.Add(1) // cancels any running full render
	m.pendingKey = ""
	m.pendingNew = false
	s := m.current()
	if s == nil {
		if m.ViewMode == ViewTree && m.TreeCursor < len(m.FlatTree) && m.FlatTree[m.TreeCursor].IsDir {
			m.Preview.SetContent(renderDirPreview(m.FlatTree[m.TreeCursor], m.RightW()))
		} else if len(m.All) == 0 && Filter.Cwd != "" {
			m.Preview.SetContent("No sessions found in this directory:\n  " + terminaltext.Line(Filter.Cwd) + "\n\nRun without --here to browse all sessions.")
		} else {
			m.Preview.SetContent("No sessions found.")
		}
		m.Preview.GotoTop()
		return
	}

	key := previewKey(s, m.RightW())
	if c, ok := m.PrevCache[key]; ok && key != "" {
		m.Preview.SetContent(c)
		m.Preview.GotoTop()
		return
	}

	m.pendingKey = key
	m.pendingSess = *s
	m.pendingGen = gen
	m.pendingNew = true
	m.Preview.SetContent("Loading " + terminaltext.Line(s.Title) + "…")
	m.Preview.GotoTop()
}

func (m *Model) cachePreview(key, content string) {
	if key == "" || len(content) > maxPreviewOutput {
		return
	}
	delete(m.PrevCache, key)
	size := len(content)
	for _, c := range m.PrevCache {
		size += len(c)
	}
	for len(m.PrevCache) >= 30 || size > maxPreviewCache {
		for k := range m.PrevCache {
			size -= len(m.PrevCache[k])
			delete(m.PrevCache, k)
			break
		}
	}
	m.PrevCache[key] = content
}

type previewTickMsg struct {
	gen uint64
}

type previewDoneMsg struct {
	gen     uint64
	key     string
	content string
}

// previewCmd schedules the full render of a partially shown preview. It
// waits briefly so moving through the list doesn't start a render for every
// row; a newer selection cancels an older render (see RefreshPreview).
func (m *Model) previewCmd() tea.Cmd {
	if !m.pendingNew {
		return nil
	}
	m.pendingNew = false
	gen := m.pendingGen
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg {
		return previewTickMsg{gen}
	})
}

func (m *Model) startFullRender(gen uint64) tea.Cmd {
	if gen != m.prevGen.Load() || m.pendingKey == "" {
		return nil
	}
	s, w, key, cur := m.pendingSess, m.RightW(), m.pendingKey, m.prevGen
	return func() tea.Msg {
		content, complete := RenderPreviewN(s, w, 0, func() bool { return cur.Load() != gen })
		if !complete {
			return nil // cancelled
		}
		return previewDoneMsg{gen: gen, key: key, content: content}
	}
}

// applyFullRender swaps in a finished full render, keeping the scroll offset.
func (m *Model) applyFullRender(msg previewDoneMsg) {
	if msg.gen != m.prevGen.Load() || msg.key != m.pendingKey {
		return
	}
	m.cachePreview(msg.key, msg.content)
	m.pendingKey = ""
	y := m.Preview.YOffset
	m.Preview.SetContent(msg.content)
	m.Preview.SetYOffset(y)
}

// renderDirPreview summarizes a directory node in the tree view.
func renderDirPreview(n *TreeNode, width int) string {
	var sb strings.Builder
	head := headerTitle.Render(fit(terminaltext.Line(n.Name), width-6)) + "\n\n" +
		headerIcon.Render("📁 ") + headerDim.Render(fit(terminaltext.Line(n.Path), width-8)) + "\n" +
		headerIcon.Render("💬 ") + headerVal.Render(formatCount(len(n.Children)))
	sb.WriteString(headerBox.Width(width - 2).Render(head))
	sb.WriteString("\n\n")
	for _, c := range n.Children {
		if c.Session == nil {
			continue
		}
		line := "  " + FmtDate(c.Session.UpdatedAt) + "  " + terminaltext.Line(c.Session.Title)
		if b := relBadges(c.Session); b != "" {
			line += "  " + b
		}
		sb.WriteString(headerDim.Render(fit(line, width-2)) + "\n")
	}
	sb.WriteString("\n" + headerDim.Render("  l / Enter to expand"))
	return sb.String()
}

// relationLine describes how a session relates to others, or "".
func relationLine(s session.Session) string {
	var parts []string
	switch {
	case s.IsSubagent():
		p := s.ParentTitle
		if p == "" {
			p = shortID(s.ParentID)
		}
		parts = append(parts, "⑂ subagent of: "+p)
	case s.IsRewind():
		parts = append(parts, "↺ rewound from "+shortID(s.ParentID))
	}
	if s.Versions > 0 {
		parts = append(parts, fmt.Sprintf("↺ %d older version(s)", s.Versions))
	}
	if s.Subagents > 0 {
		parts = append(parts, fmt.Sprintf("⑂ %d subagent(s)", s.Subagents))
	}
	if s.Versions+s.Subagents > 0 {
		parts = append(parts, "v to browse")
	}
	return terminaltext.Line(strings.Join(parts, " · "))
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func newRenderer(width int) (*glamour.TermRenderer, error) {
	style := styles.DarkStyleConfig
	zero := uint(0)
	style.Document.Margin = &zero
	return glamour.NewTermRenderer(
		glamour.WithStyles(style),
		glamour.WithWordWrap(width),
		glamour.WithTableWrap(false),
	)
}

// RenderPreviewN renders the header and the first n messages (n <= 0: all).
// complete is false when messages were left out or abort returned true.
func RenderPreviewN(s session.Session, width, n int, abort func() bool) (out string, complete bool) {
	width = max(width, 12)
	var sb strings.Builder
	innerW := width - 4 // account for box border + padding

	// Header card
	dir := terminaltext.Line(s.Cwd)
	if w := innerW - 5; ansi.StringWidth(dir) > w && w > 1 {
		dir = "…" + ansi.TruncateLeft(dir, ansi.StringWidth(dir)-(w-1), "")
	}
	dirBase := fit(terminaltext.Line(filepath.Base(s.Cwd)), innerW-2)

	date := FmtDate(s.UpdatedAt)
	dur := FmtDur(s.DurationMin)
	source := terminaltext.Line(s.Source)
	switch source {
	case "jsonl":
		source = "JSONL"
	case "jsonl_v3":
		source = "JSONL v3"
	case "sqlite_v1":
		source = "SQLite v1"
	case "sqlite_v2":
		source = "SQLite v2"
	}

	title := fit(terminaltext.Line(s.Title), innerW-2)

	header := headerTitle.Render(title) + "\n\n" +
		headerIcon.Render("📁 ") + headerVal.Render(dirBase) + headerDim.Render("  "+dir) + "\n" +
		headerIcon.Render("📅 ") + headerVal.Render(date) + headerDim.Render("  ⏱ "+dur) + "\n" +
		headerIcon.Render("💬 ") + headerVal.Render(fmt.Sprintf("%d messages", s.MsgCount)) + headerDim.Render("  "+source) + "\n"
	if rel := relationLine(s); rel != "" {
		header += headerDim.Render(fit(rel, innerW-2)) + "\n"
	}
	header += headerDim.Render(fit("ID: "+terminaltext.Line(s.SessionID), innerW-2))

	sb.WriteString(headerBox.Width(width - 2).Render(header))
	sb.WriteString("\n\n")

	var renderer *glamour.TermRenderer
	var rendererErr error
	count, textBytes := 0, 0
	complete, limited, cancelled := true, false, false
	readErr := session.WalkMessagesWithCancel(s, func(msg session.Msg) bool {
		if abort != nil && abort() {
			cancelled = true
			return false
		}
		if n > 0 && count >= n {
			complete = false
			return false
		}
		remaining := maxPreviewText - textBytes
		if remaining <= 0 {
			limited = true
			return false
		}
		text := msg.Text
		if len(text) > remaining {
			end := remaining
			for end > 0 && !utf8.RuneStart(text[end]) {
				end--
			}
			text = text[:end]
			limited = true
		}
		textBytes += len(text)
		text = terminaltext.Plain(text)
		num := msgCounter.Render(fmt.Sprintf("#%d", count+1))
		var block string

		if msg.Role == "you" {
			block = YouLabel.Render("▶ YOU") + " " + num + "\n" + text + "\n\n"
		} else {
			if len(text) <= maxMarkdownMessage {
				if renderer == nil && rendererErr == nil {
					renderer, rendererErr = newRenderer(innerW - 2)
				}
				if renderer != nil {
					if rendered, rerr := renderer.Render(text); rerr == nil {
						text = strings.TrimRight(terminaltext.Styled(rendered), "\n")
					}
				}
			}
			block = KiroLabel.Render("● KIRO") + " " + num + "\n" + text + "\n\n"
		}
		// Check after rendering too: Markdown and control escaping can expand
		// a short message. Reserve space for the size-limit notice.
		if sb.Len()+len(block) > maxPreviewOutput-512 {
			limited = true
			return false
		}
		sb.WriteString(block)
		count++
		return !limited
	}, abort)
	if cancelled || errors.Is(readErr, context.Canceled) {
		return "", false
	}
	if readErr != nil {
		sb.WriteString("Unable to read conversation: " + terminaltext.Line(readErr.Error()) + "\n")
	} else if count == 0 && !limited {
		sb.WriteString(headerDim.Render("  (no conversation data)\n"))
	}
	if limited {
		sb.WriteString("\nPreview size limit reached. Copy or export to read the full conversation.\n")
	}
	if !complete {
		sb.WriteString(msgCounter.Render("  … more messages available") + "\n")
	}
	return sb.String(), complete
}

// RenderPreview renders the whole conversation.
func RenderPreview(s session.Session, width int) string {
	out, _ := RenderPreviewN(s, width, 0, nil)
	return out
}
