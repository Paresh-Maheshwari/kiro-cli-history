package session

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// v3Meta is the shared session.json format used by the V3 agent engine.
// Workflow steps use serverSession.ownerSessionId instead of parentSessionId.
type v3Meta struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	WorkspacePaths     []string `json:"workspacePaths"`
	RootPaths          []string `json:"rootPaths"`
	CreatedAt          string   `json:"createdAt"`
	LastModifiedAt     string   `json:"lastModifiedAt"`
	ParentSessionID    string   `json:"parentSessionId"`
	CreatedReason      string   `json:"createdReason"`
	RootConversationID string   `json:"rootConversationId"`
	Meta               struct {
		Kiro struct {
			ServerSession struct {
				OwnerSessionID string `json:"ownerSessionId"`
			} `json:"serverSession"`
		} `json:"kiro"`
	} `json:"_meta"`
}

// LoadV3 reads ~/.kiro/sessions/<workspace>/<session>/session.json.
// Match only that depth: snapshots and the old cli/ store are not V3 sessions.
func LoadV3() []Session {
	legacyDir, _ := Paths()
	if legacyDir == "" {
		return nil
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(legacyDir), "*", "*", "session.json"))
	if err != nil {
		return nil
	}
	var out []Session
	for _, path := range files {
		m, err := readV3Meta(path)
		if err != nil || m.ID == "" {
			continue
		}
		messages := filepath.Join(filepath.Dir(path), "messages.jsonl")
		if info, err := os.Stat(messages); err != nil || info.Size() == 0 || info.IsDir() {
			continue
		}
		workspaces := m.WorkspacePaths
		if len(workspaces) == 0 {
			workspaces = m.RootPaths
		}
		var cwd string
		for _, p := range workspaces {
			if p != "" {
				cwd = p
				break
			}
		}
		title := cleanTitle(m.Title)
		if title == "" {
			title = Truncate(cleanTitle(firstPromptJSONL(messages)), 80)
		}
		if title == "" {
			title = "(untitled)"
		}
		parent, reason := m.ParentSessionID, m.CreatedReason
		if parent == "" && m.Meta.Kiro.ServerSession.OwnerSessionID != "" {
			parent = m.Meta.Kiro.ServerSession.OwnerSessionID
			reason = "subagent"
		}
		if parent == "" && reason == "subagent" {
			parent = m.RootConversationID
		}
		if parent == m.ID {
			parent = ""
		}
		out = append(out, Session{
			SessionID:      m.ID,
			Title:          title,
			Cwd:            cwd,
			WorkspacePaths: workspaces,
			CreatedAt:      m.CreatedAt,
			UpdatedAt:      m.LastModifiedAt,
			Source:         "jsonl_v3",
			DurationMin:    computeDuration(m.CreatedAt, m.LastModifiedAt),
			JSONLPath:      messages,
			ParentID:       parent,
			CreatedReason:  reason,
		})
	}
	return out
}

func readV3Meta(path string) (v3Meta, error) {
	var m v3Meta
	f, err := os.Open(path)
	if err != nil {
		return m, err
	}
	defer f.Close()
	err = json.NewDecoder(io.LimitReader(f, MaxFileSize)).Decode(&m)
	return m, err
}

// V3 imports a V2 session as cli_<original UUID>_<suffix>. Keep the V3 ID
// for resume, but recognize the old ID when deduplicating and linking parents.
var migratedID = regexp.MustCompile(`^cli_([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})_.+$`)

func legacyV3ID(s Session) string {
	if s.Source == "jsonl_v3" {
		if parts := migratedID.FindStringSubmatch(s.SessionID); parts != nil {
			return parts[1]
		}
	}
	return ""
}

// decodeV3Msg excludes reasoning, tool traffic, and injected context, just as
// the V2 decoder includes only visible user and assistant text blocks.
func decodeV3Msg(raw json.RawMessage) (Msg, bool) {
	var p struct {
		Type          string          `json:"type"`
		Content       json.RawMessage `json:"content"`
		OperationType string          `json:"operationType"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return Msg{}, false
	}
	var role string
	switch p.Type {
	case "user":
		role = "you"
	case "assistant":
		if p.OperationType != "" && p.OperationType != "Say" {
			return Msg{}, false
		}
		role = "kiro"
	default:
		return Msg{}, false
	}
	var text string
	if json.Unmarshal(p.Content, &text) != nil {
		return Msg{}, false
	}
	text = strings.TrimSpace(text)
	return Msg{Role: role, Text: text}, text != ""
}
