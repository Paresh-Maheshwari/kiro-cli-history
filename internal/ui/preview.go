package ui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"kiro-cli-history/internal/session"
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

// previewFirst is how many messages are rendered synchronously when the
// selection changes; the rest is rendered in the background (see previewCmd).
const previewFirst = 30

// previewKey identifies a rendered preview in PrevCache.
func previewKey(s *session.Session, width int) string {
	if s.SessionID == "" {
		return ""
	}
	// Header counts and width change the output, so they are part of the key.
	return fmt.Sprintf("%s|%d|%d|%d|%d", s.SessionID, s.Versions, s.Subagents, s.MsgCount, width)
}

// RefreshPreview shows the selected session. Cached previews are shown as is;
// otherwise the first previewFirst messages are rendered now and a full render
// is scheduled (picked up by Update via previewCmd).
func (m *Model) RefreshPreview() {
	gen := m.prevGen.Add(1) // cancels any running full render
	m.pendingKey = ""
	m.pendingNew = false
	s := m.current()
	if s == nil {
		if m.ViewMode == ViewTree && m.TreeCursor < len(m.FlatTree) && m.FlatTree[m.TreeCursor].IsDir {
			m.Preview.SetContent(renderDirPreview(m.FlatTree[m.TreeCursor], m.RightW()))
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

	content, complete := RenderPreviewN(*s, m.RightW(), previewFirst, nil)
	if complete {
		m.cachePreview(key, content)
	} else {
		m.pendingKey = key
		m.pendingSess = *s
		m.pendingGen = gen
		m.pendingNew = true
	}
	m.Preview.SetContent(content)
	m.Preview.GotoTop()
}

func (m *Model) cachePreview(key, content string) {
	if key == "" {
		return
	}
	// Cap cache at 30 entries to bound memory
	if len(m.PrevCache) >= 30 {
		// Evict a random entry (map iteration is random in Go)
		for k := range m.PrevCache {
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
	m.cachePreview(msg.key, msg.content)
	if msg.gen != m.prevGen.Load() || msg.key != m.pendingKey {
		return
	}
	m.pendingKey = ""
	y := m.Preview.YOffset
	m.Preview.SetContent(msg.content)
	m.Preview.SetYOffset(y)
}

// renderDirPreview summarizes a directory node in the tree view.
func renderDirPreview(n *TreeNode, width int) string {
	var sb strings.Builder
	head := headerTitle.Render(fit(n.Name, width-6)) + "\n\n" +
		headerIcon.Render("📁 ") + headerDim.Render(fit(n.Path, width-8)) + "\n" +
		headerIcon.Render("💬 ") + headerVal.Render(formatCount(len(n.Children)))
	sb.WriteString(headerBox.Width(width - 2).Render(head))
	sb.WriteString("\n\n")
	for _, c := range n.Children {
		if c.Session == nil {
			continue
		}
		line := "  " + FmtDate(c.Session.UpdatedAt) + "  " + c.Session.Title
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
	return strings.Join(parts, " · ")
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
	var sb strings.Builder
	innerW := width - 4 // account for box border + padding

	// Header card
	dir := s.Cwd
	if w := innerW - 5; ansi.StringWidth(dir) > w && w > 1 {
		dir = "…" + ansi.TruncateLeft(dir, ansi.StringWidth(dir)-(w-1), "")
	}
	dirBase := filepath.Base(s.Cwd)

	date := FmtDate(s.UpdatedAt)
	dur := FmtDur(s.DurationMin)
	source := s.Source
	switch source {
	case "jsonl":
		source = "JSONL"
	case "sqlite_v1":
		source = "SQLite v1"
	case "sqlite_v2":
		source = "SQLite v2"
	}

	title := fit(s.Title, innerW-2)

	header := headerTitle.Render(title) + "\n\n" +
		headerIcon.Render("📁 ") + headerVal.Render(dirBase) + headerDim.Render("  "+dir) + "\n" +
		headerIcon.Render("📅 ") + headerVal.Render(date) + headerDim.Render("  ⏱ "+dur) + "\n" +
		headerIcon.Render("💬 ") + headerVal.Render(fmt.Sprintf("%d messages", s.MsgCount)) + headerDim.Render("  "+source) + "\n"
	if rel := relationLine(s); rel != "" {
		header += headerDim.Render(rel) + "\n"
	}
	header += headerDim.Render("ID: " + s.SessionID)

	sb.WriteString(headerBox.Width(width - 2).Render(header))
	sb.WriteString("\n\n")

	// Messages. Read one extra to know whether the view is complete.
	limit := 0
	if n > 0 {
		limit = n + 1
	}
	msgs := session.ExtractMessages(s, limit)
	if len(msgs) == 0 {
		sb.WriteString(headerDim.Render("  (no conversation data)\n"))
		return sb.String(), true
	}
	complete = n <= 0 || len(msgs) <= n
	if !complete {
		msgs = msgs[:n]
	}

	renderer, err := newRenderer(innerW - 2)

	for i, msg := range msgs {
		if abort != nil && abort() {
			return "", false
		}
		num := msgCounter.Render(fmt.Sprintf("#%d", i+1))

		if msg.Role == "you" {
			sb.WriteString(YouLabel.Render("▶ YOU") + " " + num + "\n")
			sb.WriteString(msg.Text + "\n")
		} else {
			sb.WriteString(KiroLabel.Render("● KIRO") + " " + num + "\n")
			if err == nil {
				if rendered, rerr := renderer.Render(msg.Text); rerr == nil {
					sb.WriteString(strings.TrimRight(rendered, "\n") + "\n")
					sb.WriteString("\n")
					continue
				}
			}
			sb.WriteString(msg.Text + "\n")
		}
		sb.WriteString("\n")
	}
	if !complete {
		sb.WriteString(msgCounter.Render(fmt.Sprintf("  … loading the remaining %d messages", max(s.MsgCount-n, 1))) + "\n")
	}
	return sb.String(), complete
}

// RenderPreview renders the whole conversation.
func RenderPreview(s session.Session, width int) string {
	out, _ := RenderPreviewN(s, width, 0, nil)
	return out
}
