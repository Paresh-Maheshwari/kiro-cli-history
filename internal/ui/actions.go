package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	tea "github.com/charmbracelet/bubbletea"

	"kiro-cli-history/internal/session"
)

type actionDoneMsg struct{ note string }

// DoResume sets the session to resume and quits the TUI.
func (m *Model) DoResume() (Model, tea.Cmd) {
	cur := m.current()
	if cur == nil {
		return *m, nil
	}
	s := *cur
	if s.SessionID == "" {
		m.SetNote("This legacy conversation has no resumable session ID")
		return *m, nil
	}
	if s.Cwd == "" {
		m.SetNote("No directory for this session")
		return *m, nil
	}
	m.ResumeResult = &s
	return *m, tea.Quit
}

// DoCopy copies the current session's conversation to clipboard.
func (m *Model) DoCopy() tea.Cmd {
	cur := m.current()
	if cur == nil || m.ActionBusy {
		return nil
	}
	s := *cur
	m.ActionBusy = true
	m.SetNote("Copying conversation…")
	return func() tea.Msg {
		var sb strings.Builder
		count := 0
		tooLarge := false
		err := session.WalkMessages(s, func(msg session.Msg) bool {
			label := "[YOU]"
			if msg.Role == "kiro" {
				label = "[KIRO]"
			}
			if sb.Len()+len(label)+len(msg.Text)+4 > 32*1024*1024 {
				tooLarge = true
				return false
			}
			fmt.Fprintf(&sb, "%s:\n%s\n\n", label, msg.Text)
			count++
			return true
		})
		if err != nil {
			return actionDoneMsg{fmt.Sprintf("Copy failed: %v", err)}
		}
		if tooLarge {
			return actionDoneMsg{"Conversation exceeds the 32 MiB clipboard limit; use export"}
		}
		if count == 0 {
			return actionDoneMsg{"No messages to copy"}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := copyText(ctx, sb.String()); err != nil {
			return actionDoneMsg{fmt.Sprintf("Copy failed: %v", err)}
		}
		return actionDoneMsg{fmt.Sprintf("Copied %d messages", count)}
	}
}

// copyText puts text on the system clipboard.
func copyText(ctx context.Context, text string) error {
	if runtime.GOOS == "windows" {
		return clipboard.WriteAll(text) // native Win32 clipboard API
	}
	for _, name := range []string{"pbcopy", "wl-copy", "xclip", "xsel"} {
		bin, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		var c *exec.Cmd
		switch name {
		case "xclip":
			c = exec.CommandContext(ctx, bin, "-selection", "clipboard")
		case "xsel":
			c = exec.CommandContext(ctx, bin, "--clipboard", "--input")
		default:
			c = exec.CommandContext(ctx, bin)
		}
		c.WaitDelay = 200 * time.Millisecond
		c.Stdin = strings.NewReader(text)
		if c.Run() == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
	return errors.New("no clipboard tool found")
}

// DoExport saves the current session as a markdown file.
func (m *Model) DoExport() tea.Cmd {
	cur := m.current()
	if cur == nil || m.ActionBusy {
		return nil
	}
	s := *cur
	m.ActionBusy = true
	m.SetNote("Exporting conversation…")
	return func() tea.Msg {
		home, err := os.UserHomeDir()
		if err != nil {
			return actionDoneMsg{fmt.Sprintf("Export failed: %v", err)}
		}
		path, err := exportConversation(filepath.Join(home, "kiro-exports"), s)
		if err != nil {
			return actionDoneMsg{fmt.Sprintf("Export failed: %v", err)}
		}
		return actionDoneMsg{"Exported → " + path}
	}
}

func exportConversation(dir string, s session.Session) (path string, err error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("export directory must be a real directory, not a symlink")
	}
	// Tighten old installations too. On Windows access remains governed by ACLs.
	if err := os.Chmod(dir, 0700); err != nil {
		return "", err
	}
	name := sanitizeFilename(s.Title)
	if len(name) > 50 {
		name = name[:50]
	}
	if name == "" {
		name = "conversation"
	}
	f, err := os.CreateTemp(dir, time.Now().Format("2006-01-02_15-04-05")+"_"+name+"-*.md")
	if err != nil {
		return "", err
	}
	path = f.Name()
	tempPath := path
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(tempPath)
		}
	}()
	if _, err = fmt.Fprintf(f, "# %s\n\n- **Directory:** %s\n- **Date:** %s\n- **Duration:** %s\n\n---\n\n",
		s.Title, s.Cwd, FmtDate(s.UpdatedAt), FmtDur(s.DurationMin)); err != nil {
		return "", err
	}
	count := 0
	var writeErr error
	err = session.WalkMessages(s, func(msg session.Msg) bool {
		label := "## 👤 You"
		if msg.Role == "kiro" {
			label = "## 🤖 Kiro"
		}
		_, writeErr = io.WriteString(f, label+"\n\n"+msg.Text+"\n\n")
		count++
		return writeErr == nil
	})
	if err != nil {
		return "", err
	}
	if writeErr != nil {
		return "", writeErr
	}
	if count == 0 {
		return "", errors.New("no messages to export")
	}
	if _, err = fmt.Fprintf(f, "\n---\n\nMessages: %d\n", count); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	ok = true
	return path, nil
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
