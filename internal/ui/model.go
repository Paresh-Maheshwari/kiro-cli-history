package ui

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"kiro-cli-history/internal/search"
	"kiro-cli-history/internal/session"
)

// Version is shown on the splash screen; set by main.
var Version = "dev"

// Filter narrows which sessions the browser loads (e.g. only the current
// directory). The zero value loads everything.
var Filter session.LoadFilter

type sessionsLoadedMsg struct{ Sessions []session.Session }
type debounceMsg struct{ Query string }
type indexDoneMsg struct{ Results []session.IndexResult }

// Focus tracks which pane has focus.
type Focus int

const (
	FocusSearch Focus = iota
	FocusList
	FocusPreview
)

type Model struct {
	All          []session.Session            // every loaded session
	Filtered     []session.Session            // visible rows (search + grouping)
	Children     map[string][]session.Session // sessions nested under a row, by row SessionID
	TotalRows    int                          // rows with no search query (status bar)
	Cursor       int
	Input        textinput.Model
	Preview      viewport.Model
	Spinner      spinner.Model
	W, H         int
	Focus        Focus
	ViewMode     ViewMode
	Tree         []*TreeNode
	FlatTree     []*TreeNode
	TreeCursor   int
	Loading      bool
	Indexing     bool
	ShowHelp     bool
	HelpScroll   int
	ShowSettings bool
	Fullscreen   bool
	ResumeResult *session.Session
	Note         string
	NoteExpiry   time.Time
	PrevCache    map[string]string
	PrevWidth    int

	// Lazy preview: the selected session's full render runs in the background.
	pendingKey  string
	pendingSess session.Session
	pendingGen  uint64
	pendingNew  bool // a full render needs scheduling
	prevGen     *atomic.Uint64
}

// InputFocused reports whether the search bar has focus.
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
		prevGen:   new(atomic.Uint64),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(textinput.Blink, m.Spinner.Tick, func() tea.Msg {
		return sessionsLoadedMsg{session.LoadAllFiltered(Filter)}
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

// current returns the selected session, or nil (e.g. a directory in tree view).
func (m *Model) current() *session.Session {
	if m.ViewMode == ViewTree {
		if m.TreeCursor >= 0 && m.TreeCursor < len(m.FlatTree) {
			return m.FlatTree[m.TreeCursor].Session
		}
		return nil
	}
	if m.Cursor >= 0 && m.Cursor < len(m.Filtered) {
		return &m.Filtered[m.Cursor]
	}
	return nil
}

// refilter recomputes visible rows from All using the search query and the
// grouping settings, then rebuilds the tree and preview.
func (m *Model) refilter(resetCursor bool) {
	cfg := session.AppConfig
	cands := search.Sessions(m.Input.Value(), m.All)
	m.Filtered, m.Children = session.Collapse(cands, cfg.CollapseRewinds, cfg.HideSubagents)
	if m.Input.Value() == "" {
		m.TotalRows = len(m.Filtered)
	} else {
		all, _ := session.Collapse(m.All, cfg.CollapseRewinds, cfg.HideSubagents)
		m.TotalRows = len(all)
	}
	if resetCursor || m.Cursor >= len(m.Filtered) {
		m.Cursor = 0
	}
	if m.ViewMode == ViewTree {
		m.rebuildTree(resetCursor)
	}
	m.RefreshPreview()
}

// rebuildTree rebuilds the tree from Filtered, keeping expanded nodes and the
// selected node when possible.
func (m *Model) rebuildTree(resetCursor bool) {
	expanded := make(map[string]bool)
	for _, n := range FlattenTree(m.Tree) {
		if n.Expanded {
			expanded[n.Key()] = true
		}
	}
	selected := ""
	if !resetCursor && m.TreeCursor < len(m.FlatTree) {
		selected = m.FlatTree[m.TreeCursor].Key()
	}

	m.Tree = BuildTree(m.Filtered, m.Children, m.Input.Value() != "")
	var restore func([]*TreeNode)
	restore = func(nodes []*TreeNode) {
		for _, n := range nodes {
			if expanded[n.Key()] {
				n.Expanded = true
			}
			restore(n.Children)
		}
	}
	restore(m.Tree)
	m.FlatTree = FlattenTree(m.Tree)

	m.TreeCursor = 0
	for i, n := range m.FlatTree {
		if selected != "" && n.Key() == selected {
			m.TreeCursor = i
			break
		}
	}
}

// Update handles a message, then schedules the full preview render if the
// selection changed to a session that was only partially rendered.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case previewTickMsg:
		return m, m.startFullRender(msg.gen)
	case previewDoneMsg:
		m.applyFullRender(msg)
		return m, nil
	}
	next, cmd := m.update(msg)
	nm := next.(Model)
	if pc := nm.previewCmd(); pc != nil {
		cmd = tea.Batch(cmd, pc)
	}
	return nm, cmd
}

func (m Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case sessionsLoadedMsg:
		m.All = msg.Sessions
		m.Loading = false
		m.Indexing = true
		m.refilter(true)
		// Full-text index in the background. It only reads sessions; results
		// are applied here on the UI goroutine (no shared mutable state).
		all := m.All
		return m, func() tea.Msg {
			return indexDoneMsg{session.BuildFullIndex(all)}
		}

	case indexDoneMsg:
		session.ApplyIndex(m.All, msg.Results)
		m.Indexing = false
		total := 0
		for _, s := range m.All {
			total += s.MsgCount
		}
		m.SetNote(fmt.Sprintf("Index ready — %d sessions, %d messages", len(m.All), total))
		m.PrevCache = make(map[string]string)
		m.refilter(false)
		return m, nil

	case debounceMsg:
		if msg.Query == m.Input.Value() {
			m.refilter(true)
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
		cfg := session.AppConfig
		switch key {
		case "s", "esc", "enter":
			m.ShowSettings = false
		case "1":
			cfg.SQLiteEnabled = !cfg.SQLiteEnabled
			session.SaveConfig(cfg)
		case "2":
			cfg.SQLiteIndex = !cfg.SQLiteIndex
			session.SaveConfig(cfg)
		case "3":
			if cfg.DefaultView == "tree" {
				cfg.DefaultView = "list"
			} else {
				cfg.DefaultView = "tree"
			}
			session.SaveConfig(cfg)
		case "4":
			cfg.HideSubagents = !cfg.HideSubagents
			session.SaveConfig(cfg)
			m.PrevCache = make(map[string]string)
			m.refilter(false)
		case "5":
			cfg.CollapseRewinds = !cfg.CollapseRewinds
			session.SaveConfig(cfg)
			m.PrevCache = make(map[string]string)
			m.refilter(false)
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
			m.refilter(true)
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

// moveCursor moves the list or tree cursor to pos (clamped).
func (m *Model) moveCursor(pos int) {
	n := len(m.Filtered)
	if m.ViewMode == ViewTree {
		n = len(m.FlatTree)
	}
	if pos > n-1 {
		pos = n - 1
	}
	if pos < 0 {
		pos = 0
	}
	if m.ViewMode == ViewTree {
		m.TreeCursor = pos
	} else {
		m.Cursor = pos
	}
	m.RefreshPreview()
}

func (m *Model) cursorPos() int {
	if m.ViewMode == ViewTree {
		return m.TreeCursor
	}
	return m.Cursor
}

func (m Model) handleListKey(key string, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		return m, tea.Quit
	case "v":
		if m.ViewMode == ViewList {
			sel := m.current()
			m.ViewMode = ViewTree
			m.rebuildTree(true)
			m.selectInTree(sel)
		} else {
			// Keep the selected row when it is a top-level row.
			if s := m.current(); s != nil {
				for i := range m.Filtered {
					if m.Filtered[i].SessionID == s.SessionID && m.Filtered[i].Cwd == s.Cwd {
						m.Cursor = i
						break
					}
				}
			}
			m.ViewMode = ViewList
		}
		m.RefreshPreview()
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
	case "l", "enter", "right":
		if m.ViewMode == ViewTree {
			if m.TreeCursor >= len(m.FlatTree) {
				return m, nil
			}
			node := m.FlatTree[m.TreeCursor]
			switch {
			case node.IsDir:
				node.Expanded = !node.Expanded
				m.FlatTree = FlattenTree(m.Tree)
			case len(node.Children) > 0 && !node.Expanded:
				node.Expanded = true
				m.FlatTree = FlattenTree(m.Tree)
			case node.Session != nil:
				m.Focus = FocusPreview
			}
			return m, nil
		}
		m.Focus = FocusPreview
		return m, nil
	case "h", "left":
		if m.ViewMode == ViewTree && m.TreeCursor < len(m.FlatTree) {
			node := m.FlatTree[m.TreeCursor]
			if node.Expanded && len(node.Children) > 0 {
				node.Expanded = false
				m.FlatTree = FlattenTree(m.Tree)
			} else if node.Parent != nil {
				for i, n := range m.FlatTree {
					if n == node.Parent {
						m.TreeCursor = i
						break
					}
				}
			}
			m.RefreshPreview()
		}
		return m, nil
	case "j", "down":
		m.moveCursor(m.cursorPos() + 1)
	case "k", "up":
		m.moveCursor(m.cursorPos() - 1)
	case "g", "home":
		m.moveCursor(0)
	case "G", "end":
		m.moveCursor(1 << 30)
	case "pgdown":
		m.moveCursor(m.cursorPos() + 10)
	case "pgup":
		m.moveCursor(m.cursorPos() - 10)
	}
	return m, nil
}

// selectInTree moves the tree cursor to s, expanding its directory.
func (m *Model) selectInTree(s *session.Session) {
	if s == nil {
		return
	}
	for _, dir := range m.Tree {
		for _, row := range dir.Children {
			if row.Session != nil && row.Session.SessionID == s.SessionID && row.Session.Cwd == s.Cwd {
				dir.Expanded = true
				m.FlatTree = FlattenTree(m.Tree)
				for i, n := range m.FlatTree {
					if n == row {
						m.TreeCursor = i
						return
					}
				}
			}
		}
	}
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
