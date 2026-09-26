package session

const MaxFileSize = 100 * 1024 * 1024 // 100MB

// Session represents a Kiro CLI conversation from any source.
type Session struct {
	SessionID   string
	Title       string
	Cwd         string
	CreatedAt   string
	UpdatedAt   string
	Source      string // "jsonl", "sqlite_v1", "sqlite_v2"
	MsgCount    int
	DurationMin int
	JSONLPath   string // JSONL sessions
	SearchText  string // pre-built lowercase index

	// Relationships (JSONL only).
	ParentID      string // parent_session_id
	CreatedReason string // session_created_reason: "subagent", "rewind", ...
	GroupID       string // rewind family key (see Link)
	ParentTitle   string // title of ParentID, if loaded

	// Set by Collapse for visible rows: how many sessions are nested under it.
	Versions  int // older rewind versions
	Subagents int // subagent sessions
}

// IsSubagent reports whether the session was spawned by another session.
func (s *Session) IsSubagent() bool {
	return s.CreatedReason == "subagent" && s.ParentID != ""
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
