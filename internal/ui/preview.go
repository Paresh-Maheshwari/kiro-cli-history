package ui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/charmbracelet/lipgloss"

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

func (m *Model) RefreshPreview() {
	if len(m.Filtered) == 0 || m.Cursor >= len(m.Filtered) {
		m.Preview.SetContent("No sessions found.")
		return
	}
	s := m.Filtered[m.Cursor]

	if c, ok := m.PrevCache[s.SessionID]; ok && s.SessionID != "" {
		m.Preview.SetContent(c)
		m.Preview.GotoTop()
		return
	}

	rw := m.RightW()
	content := RenderPreview(s, rw)

	if s.SessionID != "" {
		// Cap cache at 30 entries to bound memory
		if len(m.PrevCache) >= 30 {
			// Evict a random entry (map iteration is random in Go)
			for k := range m.PrevCache {
				delete(m.PrevCache, k)
				break
			}
		}
		m.PrevCache[s.SessionID] = content
	}
	m.Preview.SetContent(content)
	m.Preview.GotoTop()
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

func RenderPreview(s session.Session, width int) string {
	var sb strings.Builder
	innerW := width - 4 // account for box border + padding

	// Header card
	dir := s.Cwd
	if len(dir) > innerW-5 {
		dir = "…" + dir[len(dir)-innerW+6:]
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

	title := s.Title
	if len(title) > innerW-2 {
		title = title[:innerW-5] + "..."
	}

	header := headerTitle.Render(title) + "\n\n" +
		headerIcon.Render("📁 ") + headerVal.Render(dirBase) + headerDim.Render("  "+dir) + "\n" +
		headerIcon.Render("📅 ") + headerVal.Render(date) + headerDim.Render("  ⏱ "+dur) + "\n" +
		headerIcon.Render("💬 ") + headerVal.Render(fmt.Sprintf("%d messages", s.MsgCount)) + headerDim.Render("  "+source) + "\n" +
		headerDim.Render("ID: "+s.SessionID)

	sb.WriteString(headerBox.Width(width - 2).Render(header))
	sb.WriteString("\n\n")

	// Messages
	msgs := session.ExtractMessages(s, 0)
	if len(msgs) == 0 {
		sb.WriteString(headerDim.Render("  (no conversation data)\n"))
		return sb.String()
	}

	renderer, err := newRenderer(innerW - 2)

	for i, msg := range msgs {
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
	return sb.String()
}
