package session

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// JSONL wire types (unexported)
type jsonlMeta struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
	Cwd       string `json:"cwd"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type jsonlLine struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

type jsonlData struct {
	Content json.RawMessage `json:"content"`
}

type jsonlBlock struct {
	Kind string `json:"kind"`
	Data string `json:"data"`
}

// LoadJSONL reads sessions from ~/.kiro/sessions/cli/*.json.
func LoadJSONL() []Session {
	dir, _ := Paths()
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil
	}

	out := make([]Session, 0, len(files))
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil || info.Size() > MaxFileSize {
			continue
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var m jsonlMeta
		if json.Unmarshal(data, &m) != nil {
			continue
		}

		jpath := strings.TrimSuffix(f, ".json") + ".jsonl"
		title := m.Title
		if title == "" {
			title = "(untitled)"
		}

		// Estimate msg count from jsonl file size (avoids parsing)
		msgCount := 0
		if ji, err := os.Stat(jpath); err == nil {
			msgCount = int(ji.Size() / 2000) // ~2KB per message avg
			if msgCount < 1 && ji.Size() > 0 {
				msgCount = 1
			}
		}

		out = append(out, Session{
			SessionID:   m.SessionID,
			Title:       title,
			Cwd:         m.Cwd,
			CreatedAt:   m.CreatedAt,
			UpdatedAt:   m.UpdatedAt,
			Source:      "jsonl",
			DurationMin: computeDuration(m.CreatedAt, m.UpdatedAt),
			JSONLPath:   jpath,
		})
	}
	return out
}

// countFast counts messages via string scan — no JSON parsing.
func countFast(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if strings.Contains(line, `"kind":"Prompt"`) || strings.Contains(line, `"kind":"AssistantMessage"`) ||
			strings.Contains(line, `"kind": "Prompt"`) || strings.Contains(line, `"kind": "AssistantMessage"`) {
			n++
		}
	}
	return n
}

// extractJSONLMessages reads messages from a .jsonl file.
func extractJSONLMessages(path string, limit int) []Msg {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 {
		return nil
	}
	if info.Size() > MaxFileSize {
		return []Msg{{Role: "system", Text: "(File too large to preview)"}}
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var msgs []Msg
	dec := json.NewDecoder(f)
	for dec.More() {
		var line jsonlLine
		if dec.Decode(&line) != nil {
			break
		}
		if line.Kind != "Prompt" && line.Kind != "AssistantMessage" {
			continue
		}
		var d jsonlData
		if json.Unmarshal(line.Data, &d) != nil {
			continue
		}
		var blocks []jsonlBlock
		if json.Unmarshal(d.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			if b.Kind == "text" && b.Data != "" {
				role := "you"
				if line.Kind == "AssistantMessage" {
					role = "kiro"
				}
				msgs = append(msgs, Msg{Role: role, Text: b.Data})
				if limit > 0 && len(msgs) >= limit {
					return msgs
				}
				break
			}
		}
	}
	return msgs
}
