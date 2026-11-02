package session

// MaxFileSize bounds metadata and SQLite values decoded as whole objects.
// JSONL conversation files are streamed without a total-size cap.
const MaxFileSize = 100 * 1024 * 1024

// Session represents a Kiro CLI conversation from any source.
type Session struct {
	SessionID      string
	Title          string
	Cwd            string
	CreatedAt      string
	UpdatedAt      string
	Source         string // "jsonl", "jsonl_v3", "sqlite_v1", "sqlite_v2"
	MsgCount       int
	DurationMin    int
	JSONLPath      string   // JSONL sessions
	SearchText     string   // pre-built lowercase index
	WorkspacePaths []string // V3 sessions may span multiple directories

	// Relationships (JSONL only).
	ParentID      string // parent_session_id
	CreatedReason string // session_created_reason: "subagent", "rewind", ...
	GroupID       string // rewind family key (see Link)
	ParentTitle   string // title of ParentID, if loaded
	OwnerID       string // spawning parent of a subagent's entire rewind family

	// Set by Collapse for visible rows: how many sessions are nested under it.
	Versions  int // older rewind versions
	Subagents int // subagent sessions
}

// Workspaces returns the session's directories without empty or duplicate paths.
// Cwd is first because it is the active directory when --here/--cwd is used.
func (s *Session) Workspaces() []string {
	var paths []string
	seen := make(map[string]bool)
	for _, path := range append([]string{s.Cwd}, s.WorkspacePaths...) {
		path = normPath(path)
		if path != "" && !seen[path] {
			paths = append(paths, path)
			seen[path] = true
		}
	}
	return paths
}

// IsSubagent reports whether the session was spawned by another session.
func (s *Session) IsSubagent() bool {
	return s.OwnerID != "" || s.CreatedReason == "subagent" && s.ParentID != ""
}

func (s *Session) SubagentOwnerID() string {
	if s.OwnerID != "" {
		return s.OwnerID
	}
	if s.CreatedReason == "subagent" {
		return s.ParentID
	}
	return ""
}

// IsRewind reports whether the session is a rewound copy of another session.
func (s *Session) IsRewind() bool {
	return s.CreatedReason == "rewind" && s.ParentID != ""
}

// Msg is a single conversation message.
type Msg struct {
	Role string // "you" or "kiro"
	Text string
}
