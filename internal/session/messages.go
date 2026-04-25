package session

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ExtractMessages returns messages from any session format.
func ExtractMessages(s Session, limit int) []Msg {
	if s.JSONLPath != "" {
		return extractJSONLMessages(s.JSONLPath, limit)
	}
	if s.Source == "sqlite_v1" || s.Source == "sqlite_v2" {
		if h := LoadSQLiteHistory(s); len(h) > 0 {
			return extractHistoryMsgs(h, limit)
		}
	}
	return nil
}

// FirstPrompt returns the first user prompt from history (for titles).
func FirstPrompt(raw []json.RawMessage) string {
	for _, r := range raw {
		user, _ := parseEntry(r)
		if txt := promptFromUser(user); txt != "" {
			if len(txt) > 60 {
				return txt[:60]
			}
			return txt
		}
	}
	return "(untitled)"
}

func extractHistoryMsgs(raw []json.RawMessage, limit int) []Msg {
	var msgs []Msg
	for _, r := range raw {
		user, asst := parseEntry(r)
		if txt := promptFromUser(user); txt != "" {
			msgs = append(msgs, Msg{Role: "you", Text: txt})
			if limit > 0 && len(msgs) >= limit {
				return msgs
			}
		}
		if txt := textFromAssistant(asst); txt != "" {
			msgs = append(msgs, Msg{Role: "kiro", Text: txt})
			if limit > 0 && len(msgs) >= limit {
				return msgs
			}
		}
	}
	return msgs
}

// parseEntry handles both history formats:
//   - Dict: {"user": {...}, "assistant": {...}}
//   - List: [user_msg, assistant_msg]
func parseEntry(raw json.RawMessage) (user, asst map[string]interface{}) {
	// Try dict format first
	var dictEntry struct {
		User      map[string]interface{} `json:"user"`
		Assistant interface{}            `json:"assistant"`
	}
	if json.Unmarshal(raw, &dictEntry) == nil && dictEntry.User != nil {
		am, _ := dictEntry.Assistant.(map[string]interface{})
		return dictEntry.User, am
	}

	// Try list format: [user_msg, assistant_msg]
	var listEntry []json.RawMessage
	if json.Unmarshal(raw, &listEntry) == nil && len(listEntry) >= 1 {
		var userMsg map[string]interface{}
		json.Unmarshal(listEntry[0], &userMsg)
		user = userMsg

		if len(listEntry) >= 2 {
			var asstMsg map[string]interface{}
			json.Unmarshal(listEntry[1], &asstMsg)
			asst = asstMsg
		}
		return user, asst
	}

	return nil, nil
}

// promptFromUser extracts the prompt text from a user message.
// Handles: {"content": {"Prompt": {"prompt": "..."}}}
func promptFromUser(user map[string]interface{}) string {
	if user == nil {
		return ""
	}
	c, _ := user["content"].(map[string]interface{})
	p, _ := c["Prompt"].(map[string]interface{})
	txt, _ := p["prompt"].(string)
	return strings.TrimSpace(txt)
}

// textFromAssistant extracts text from an assistant message.
// The assistant msg itself can be the top-level with Response/ToolUse/Text keys,
// or nested under "content".
func textFromAssistant(asst map[string]interface{}) string {
	if asst == nil {
		return ""
	}

	// Direct Response at top level: {"Response": {"content": "..."}}
	if resp, ok := asst["Response"].(map[string]interface{}); ok {
		if txt, ok := resp["content"].(string); ok && txt != "" {
			return txt
		}
	}

	// content.Text: {"content": {"Text": "..."}}
	if ac, ok := asst["content"].(map[string]interface{}); ok {
		if txt, ok := ac["Text"].(string); ok && txt != "" {
			return txt
		}
	}

	// ToolUse: {"ToolUse": {"content": "...", "tool_uses": [...]}}
	if tu, ok := asst["ToolUse"].(map[string]interface{}); ok {
		txt, _ := tu["content"].(string)
		names := toolNames(tu)
		if names != "" {
			if txt != "" {
				return fmt.Sprintf("%s\n[tools: %s]", txt, names)
			}
			return fmt.Sprintf("[tools: %s]", names)
		}
		return txt
	}

	return ""
}

func toolNames(tu map[string]interface{}) string {
	tools, ok := tu["tool_uses"].([]interface{})
	if !ok {
		return ""
	}
	var names []string
	for i, t := range tools {
		if i >= 3 {
			break
		}
		if m, ok := t.(map[string]interface{}); ok {
			if n, ok := m["name"].(string); ok {
				names = append(names, n)
			}
		}
	}
	return strings.Join(names, ", ")
}

func computeDuration(created, updated string) int {
	c, u := parseTime(created), parseTime(updated)
	if c.IsZero() || u.IsZero() {
		return 0
	}
	return int(u.Sub(c).Minutes())
}

func parseTime(s string) time.Time {
	s = strings.Replace(s, "Z", "+00:00", 1)
	for _, l := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04:05-07:00"} {
		if t, err := time.Parse(l, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
