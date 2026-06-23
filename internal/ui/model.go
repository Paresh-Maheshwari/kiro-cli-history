package ui

import (
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"kiro-cli-history/internal/search"
	"kiro-cli-history/internal/session"
)

type sessionsLoadedMsg struct{ Sessions []session.Session }
type debounceMsg struct{ Query string }
type indexDoneMsg struct{}

// Focus tracks which pane has focus.
type Focus int

const (
	FocusSearch Focus = iota
	FocusList
	FocusPreview
)

type Model struct {
	All, Filtered []session.Session
	Cursor        int
	Input         textinput.Model
	Preview       viewport.Model
	Spinner       spinner.Model
	W, H          int
	Focus         Focus
	ViewMode      ViewMode
	Tree          []*TreeNode
	FlatTree      []*TreeNode
	TreeCursor    int
	Loading       bool
	Indexing      bool
	ShowHelp      bool
	HelpScroll    int
	ShowSettings  bool
	Fullscreen    bool
	ResumeResult  *session.Session
	Note          string
	NoteExpiry    time.Time
	PrevCache     map[string]string
	PrevWidth     int
	IndexMu       *sync.RWMutex
}

// Keep InputFocused for backward compat in view.go
func (m *Model) InputFocused() bool { return m.Focus == FocusSearch }

func NewModel() Model {
	ti := textinput.New()
	ti.Placeholder = "Search sessions..."
	ti.CharLimit = 200
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("62"))

	return Model{
		Input:     ti,
		Preview:   viewport.New(0, 0),
		Spinner:   sp,
		Focus:     FocusSearch,
		ViewMode:  defaultViewMode(),
		Loading:   true,
		PrevCache: make(map[string]string),
		IndexMu:   &sync.RWMutex{},
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.Spinner.Tick, func() tea.Msg {
		return sessionsLoadedMsg{session.LoadAll()}
	})
}

func (m *Model) LeftW() int {
	w := m.W * 2 / 5
	if w < 30 {
		w = 30
	}
	return w
}

func (m *Model) RightW() int {
	if m.Fullscreen {
		return m.W - 2
	}
	r := m.W - m.LeftW() - 1
	if r < 20 {
		r = 20
	}
	return r
}

func (m *Model) ListH() int {
	h := m.H - 6
	if h < 1 {
		h = 1
	}
	return h
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case sessionsLoadedMsg:
		m.All = msg.Sessions
		m.Filtered = msg.Sessions
		m.Loading = false
		m.Indexing = true
		m.Cursor = 0
		if m.ViewMode == ViewTree {
			m.Tree = BuildTree(m.Filtered, false)
			m.FlatTree = FlattenTree(m.Tree)
		}
		m.RefreshPreview()
		// Start background full-text indexing
		return m, func() tea.Msg {
			session.BuildFullIndex(m.All, m.IndexMu, nil)
			// Release resources after indexing
			runtime.GC()
			return indexDoneMsg{}
		}

	case indexDoneMsg:
		m.Indexing = false
		total := 0
		for _, s := range m.All {
			total += s.MsgCount
		}
		m.SetNote(fmt.Sprintf("Index ready — %d sessions, %d messages", len(m.All), total))
		// Re-filter to pick up updated msg counts and search text
		q := m.Input.Value()
		if q == "" {
			m.Filtered = m.All
		} else {
			m.Filtered = search.Sessions(q, m.All, m.IndexMu)
		}
		if m.ViewMode == ViewTree {
			m.Tree = BuildTree(m.Filtered, m.Input.Value() != "")
			m.FlatTree = FlattenTree(m.Tree)
			m.TreeCursor = 0
		}
		m.PrevCache = make(map[string]string)
		m.RefreshPreview()
		return m, nil

	case debounceMsg:
		if msg.Query == m.Input.Value() {
			m.Filtered = search.Sessions(msg.Query, m.All, m.IndexMu)
			m.Cursor = 0
			if m.ViewMode == ViewTree {
				m.Tree = BuildTree(m.Filtered, m.Input.Value() != "")
				m.FlatTree = FlattenTree(m.Tree)
				m.TreeCursor = 0
			}
			m.RefreshPreview()
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.W, m.H = msg.Width, msg.Height
		rw := m.RightW()
		m.Preview.Width = rw
		m.Preview.Height = m.ListH() + 1
		if m.Fullscreen {
			m.Preview.Width = m.W - 2
			m.Preview.Height = m.H - 2
		}
		if rw != m.PrevWidth {
			m.PrevCache = make(map[string]string)
			m.PrevWidth = rw
		}
		m.RefreshPreview()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if m.Loading {
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd
	}

	if m.Focus == FocusSearch {
		var cmd tea.Cmd
		m.Input, cmd = m.Input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Help overlay — scrollable
	if m.ShowHelp {
		switch key {
		case "?", "esc", "enter":
			m.ShowHelp = false
			m.HelpScroll = 0
		case "j", "down":
			m.HelpScroll++
		case "k", "up":
			if m.HelpScroll > 0 {
				m.HelpScroll--
			}
		case "g":
			m.HelpScroll = 0
		}
		return m, nil
	}

	// Settings overlay
	if m.ShowSettings {
		switch key {
		case "s", "esc", "enter":
			m.ShowSettings = false
		case "1":
			cfg := session.AppConfig
			cfg.SQLiteEnabled = !cfg.SQLiteEnabled
			session.SaveConfig(cfg)
		case "2":
			cfg := session.AppConfig
			cfg.SQLiteIndex = !cfg.SQLiteIndex
			session.SaveConfig(cfg)
		case "3":
			cfg := session.AppConfig
			if cfg.DefaultView == "tree" {
				cfg.DefaultView = "list"
			} else {
				cfg.DefaultView = "tree"
			}
			session.SaveConfig(cfg)
		}
		return m, nil
	}

	// Fullscreen mode — most keys exit back
	if m.Fullscreen {
		switch key {
		case "f", "esc", "q":
			m.Fullscreen = false
			m.Preview.Width = m.RightW()
			m.Preview.Height = m.ListH() + 1
			m.PrevCache = make(map[string]string)
			m.RefreshPreview()
		case "j", "down":
			m.Preview.LineDown(3)
		case "k", "up":
			m.Preview.LineUp(3)
		case "d":
			m.Preview.HalfViewDown()
		case "u":
			m.Preview.HalfViewUp()
		case "g":
			m.Preview.GotoTop()
		case "G":
			m.Preview.GotoBottom()
		case "pgdown", " ":
			m.Preview.ViewDown()
		case "pgup":
			m.Preview.ViewUp()
		case "ctrl+c":
			return m, tea.Quit
		}
		return m, nil
	}

	// Global keys
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "?":
		m.ShowHelp = true
		return m, nil
	case "ctrl+r":
		return m.DoResume()
	case "ctrl+y":
		m.DoCopy()
		return m, nil
	case "ctrl+e":
		m.DoExport()
		return m, nil
	case "ctrl+f", "/":
		if m.Focus != FocusSearch {
			m.Focus = FocusSearch
			m.Input.Focus()
			return m, textinput.Blink
		}
	case "tab":
		// Cycle: search → list → preview → list
		switch m.Focus {
		case FocusSearch:
			m.Focus = FocusList
			m.Input.Blur()
		case FocusList:
			m.Focus = FocusPreview
		case FocusPreview:
			m.Focus = FocusList
		}
		return m, nil
	case "shift+tab":
		switch m.Focus {
		case FocusPreview:
			m.Focus = FocusList
		case FocusList:
			m.Focus = FocusSearch
			m.Input.Focus()
			return m, textinput.Blink
		case FocusSearch:
			m.Focus = FocusPreview
		}
		return m, nil
	}

	switch m.Focus {
	case FocusSearch:
		return m.handleSearchKey(key, msg)
	case FocusList:
		return m.handleListKey(key, msg)
	case FocusPreview:
		return m.handlePreviewKey(key, msg)
	}
	return m, nil
}

func (m Model) handleSearchKey(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		if m.Input.Value() != "" {
			m.Input.SetValue("")
			m.Filtered = m.All
			m.Cursor = 0
			m.RefreshPreview()
			return m, nil
		}
		m.Focus = FocusList
		m.Input.Blur()
		return m, nil
	case "enter", "down":
		m.Focus = FocusList
		m.Input.Blur()
		return m, nil
	case "up":
		return m, nil
	default:
		prev := m.Input.Value()
		var cmd tea.Cmd
		m.Input, cmd = m.Input.Update(msg)
		if cur := m.Input.Value(); cur != prev {
			q := cur
			return m, tea.Batch(cmd, tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
				return debounceMsg{q}
			}))
		}
		return m, cmd
	}
}

func (m Model) handleListKey(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		return m, tea.Quit
	case "v":
		// Toggle view mode
		if m.ViewMode == ViewList {
			m.ViewMode = ViewTree
			m.Tree = BuildTree(m.Filtered, m.Input.Value() != "")
			m.FlatTree = FlattenTree(m.Tree)
			m.TreeCursor = 0
		} else {
			m.ViewMode = ViewList
		}
		return m, nil
	case "s":
		m.ShowSettings = true
		return m, nil
	case "f":
		m.Fullscreen = true
		m.Preview.Width = m.W - 2
		m.Preview.Height = m.H - 2
		m.PrevCache = make(map[string]string)
		m.RefreshPreview()
		return m, nil
	case "l", "enter":
		if m.ViewMode == ViewTree {
			// Expand/collapse dir or select session
			if m.TreeCursor < len(m.FlatTree) {
				node := m.FlatTree[m.TreeCursor]
				if node.IsDir {
					node.Expanded = !node.Expanded
					m.FlatTree = FlattenTree(m.Tree)
				} else if node.Session != nil {
					m.Focus = FocusPreview
				}
			}
			return m, nil
		}
		// Switch to preview pane
		m.Focus = FocusPreview
		return m, nil
	case "j", "down":
		if m.ViewMode == ViewTree {
			if m.TreeCursor < len(m.FlatTree)-1 {
				m.TreeCursor++
				m.refreshTreePreview()
			}
			return m, nil
		}
		if m.Cursor < len(m.Filtered)-1 {
			m.Cursor++
			m.RefreshPreview()
		}
	case "k", "up":
		if m.ViewMode == ViewTree {
			if m.TreeCursor > 0 {
				m.TreeCursor--
				m.refreshTreePreview()
			}
			return m, nil
		}
		if m.Cursor > 0 {
			m.Cursor--
			m.RefreshPreview()
		}
	case "h":
		if m.ViewMode == ViewTree {
			// Collapse current dir or go to parent
			if m.TreeCursor < len(m.FlatTree) {
				node := m.FlatTree[m.TreeCursor]
				if node.IsDir && node.Expanded {
					node.Expanded = false
					m.FlatTree = FlattenTree(m.Tree)
				}
			}
			return m, nil
		}
	case "g":
		m.Cursor = 0
		m.RefreshPreview()
	case "G":
		if n := len(m.Filtered); n > 0 {
			m.Cursor = n - 1
			m.RefreshPreview()
		}
	case "pgdown":
		m.Cursor = min(m.Cursor+10, max(len(m.Filtered)-1, 0))
		m.RefreshPreview()
	case "pgup":
		m.Cursor = max(m.Cursor-10, 0)
		m.RefreshPreview()
	}
	return m, nil
}

func (m Model) handlePreviewKey(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "s":
		m.ShowSettings = true
		return m, nil
	case "f":
		m.Fullscreen = true
		m.Preview.Width = m.W - 2
		m.Preview.Height = m.H - 2
		m.PrevCache = make(map[string]string)
		m.RefreshPreview()
		return m, nil
	case "esc", "h", "q":
		// Back to list
		m.Focus = FocusList
		return m, nil
	case "j", "down":
		m.Preview.LineDown(3)
		return m, nil
	case "k", "up":
		m.Preview.LineUp(3)
		return m, nil
	case "d":
		m.Preview.HalfViewDown()
		return m, nil
	case "u":
		m.Preview.HalfViewUp()
		return m, nil
	case "g":
		m.Preview.GotoTop()
		return m, nil
	case "G":
		m.Preview.GotoBottom()
		return m, nil
	case "pgdown", " ":
		m.Preview.ViewDown()
		return m, nil
	case "pgup":
		m.Preview.ViewUp()
		return m, nil
	default:
		var cmd tea.Cmd
		m.Preview, cmd = m.Preview.Update(msg)
		return m, cmd
	}
}

func (m *Model) SetNote(s string) {
	m.Note = s
	m.NoteExpiry = time.Now().Add(3 * time.Second)
}

func defaultViewMode() ViewMode {
	if session.AppConfig.DefaultView == "tree" {
		return ViewTree
	}
	return ViewList
}

func (m *Model) refreshTreePreview() {
	if m.TreeCursor >= len(m.FlatTree) {
		return
	}
	node := m.FlatTree[m.TreeCursor]
	if node.Session != nil {
		// Find session in Filtered and set cursor
		for i, s := range m.Filtered {
			if s.SessionID == node.Session.SessionID {
				m.Cursor = i
				m.RefreshPreview()
				return
			}
		}
	}
}
