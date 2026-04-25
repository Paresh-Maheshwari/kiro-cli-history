package ui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"kiro-cli-history/internal/session"
)

// DoResume sets the session to resume and quits the TUI.
func (m *Model) DoResume() (Model, tea.Cmd) {
	if len(m.Filtered) == 0 || m.Cursor >= len(m.Filtered) {
		return *m, nil
	}
	s := m.Filtered[m.Cursor]
	if s.Cwd == "" {
		m.SetNote("No directory for this session")
		return *m, nil
	}
	m.ResumeResult = &s
	return *m, tea.Quit
}

// DoCopy copies the current session's conversation to clipboard.
func (m *Model) DoCopy() {
	if len(m.Filtered) == 0 || m.Cursor >= len(m.Filtered) {
		return
	}
	msgs := session.ExtractMessages(m.Filtered[m.Cursor], 0)
	if len(msgs) == 0 {
		m.SetNote("No messages to copy")
		return
	}

	var sb strings.Builder
	for _, msg := range msgs {
		label := "[YOU]"
		if msg.Role == "kiro" {
			label = "[KIRO]"
		}
		fmt.Fprintf(&sb, "%s:\n%s\n\n", label, msg.Text)
	}

	for _, name := range []string{"pbcopy", "xclip", "xsel", "wl-copy"} {
		bin, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		var c *exec.Cmd
		switch name {
		case "xclip":
			c = exec.Command(bin, "-selection", "clipboard")
		case "xsel":
			c = exec.Command(bin, "--clipboard", "--input")
		default:
			c = exec.Command(bin)
		}
		c.Stdin = strings.NewReader(sb.String())
		if c.Run() == nil {
			m.SetNote(fmt.Sprintf("Copied %d messages", len(msgs)))
			return
		}
	}
	m.SetNote("No clipboard tool found")
}

// DoExport saves the current session as a markdown file.
func (m *Model) DoExport() {
	if len(m.Filtered) == 0 || m.Cursor >= len(m.Filtered) {
		return
	}
	s := m.Filtered[m.Cursor]
	msgs := session.ExtractMessages(s, 0)
	if len(msgs) == 0 {
		m.SetNote("No messages to export")
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("# %s\n\n", s.Title))
	sb.WriteString(fmt.Sprintf("- **Directory:** %s\n", s.Cwd))
	sb.WriteString(fmt.Sprintf("- **Date:** %s\n", FmtDate(s.UpdatedAt)))
	sb.WriteString(fmt.Sprintf("- **Messages:** %d\n", len(msgs)))
	sb.WriteString(fmt.Sprintf("- **Duration:** %s\n", FmtDur(s.DurationMin)))
	sb.WriteString("\n---\n\n")

	for _, msg := range msgs {
		if msg.Role == "you" {
			sb.WriteString("## 👤 You\n\n")
			sb.WriteString(msg.Text + "\n\n")
		} else {
			sb.WriteString("## 🤖 Kiro\n\n")
			sb.WriteString(msg.Text + "\n\n")
		}
	}

	// Save to ~/kiro-exports/
	dir := filepath.Join(os.Getenv("HOME"), "kiro-exports")
	os.MkdirAll(dir, 0755)

	// Filename from title (sanitized)
	name := sanitizeFilename(s.Title)
	if len(name) > 50 {
		name = name[:50]
	}
	ts := time.Now().Format("2006-01-02_15-04")
	filename := fmt.Sprintf("%s_%s.md", ts, name)
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		m.SetNote(fmt.Sprintf("Export failed: %v", err))
		return
	}
	m.SetNote(fmt.Sprintf("Exported → ~/kiro-exports/%s", filename))
}

func sanitizeFilename(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	return b.String()
}
