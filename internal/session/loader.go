package session

import (
	"encoding/json"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// LoadAll loads all sessions, deduplicates, sorts, and builds a quick index (title+cwd only).
func LoadAll() []Session {
	jsonl := LoadJSONL()

	var sqlite []Session
	if AppConfig.SQLiteEnabled {
		sqlite = LoadSQLite()
	}

	seen := make(map[string]bool, len(jsonl))
	for _, s := range jsonl {
		if s.SessionID != "" {
			seen[s.SessionID] = true
		}
	}
	for _, s := range sqlite {
		if s.SessionID != "" && !seen[s.SessionID] {
			jsonl = append(jsonl, s)
			seen[s.SessionID] = true
		}
	}

	sort.Slice(jsonl, func(i, j int) bool {
		a, b := jsonl[i].UpdatedAt, jsonl[j].UpdatedAt
		if a == "" {
			a = jsonl[i].CreatedAt
		}
		if b == "" {
			b = jsonl[j].CreatedAt
		}
		return a > b
	})

	for i := range jsonl {
		jsonl[i].SearchText = strings.ToLower(jsonl[i].Title) + "\n" + strings.ToLower(jsonl[i].Cwd) + "\n"
	}
	return jsonl
}

// BuildFullIndex adds message content to the search index in the background.
func BuildFullIndex(sessions []Session, mu *sync.RWMutex, done func()) {
	// Index JSONL sessions in parallel (file I/O, no shared state)
	var wg sync.WaitGroup
	workers := runtime.NumCPU()
	if workers > 1 {
		workers = workers - 1 // leave 1 core for UI
	}
	sem := make(chan struct{}, workers)

	for i := range sessions {
		if sessions[i].JSONLPath == "" {
			continue
		}
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			text := indexJSONLFast(sessions[idx].JSONLPath)
			count := countFast(sessions[idx].JSONLPath)

			mu.Lock()
			if text != "" {
				sessions[idx].SearchText += text
			}
			sessions[idx].MsgCount = count
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	// Index SQLite sessions (if enabled)
	if !AppConfig.SQLiteEnabled {
		if done != nil {
			done()
		}
		return
	}
	conn := openDB()
	if conn != nil {
		// Build index: sessionID/cwd → slice index
		sqlIdx := make(map[string]int)
		for i := range sessions {
			s := &sessions[i]
			if s.Source == "sqlite_v2" {
				sqlIdx[s.SessionID] = i
			} else if s.Source == "sqlite_v1" {
				sqlIdx["v1:"+s.Cwd] = i
			}
		}

		// Full content indexing (only if enabled — slow on large DBs)
		if AppConfig.SQLiteIndex && len(sqlIdx) > 0 {
			rows, err := conn.Query("SELECT conversation_id, value FROM conversations_v2")
			if err == nil {
				for rows.Next() {
					var id, value string
					if rows.Scan(&id, &value) != nil {
						continue
					}
					idx, ok := sqlIdx[id]
					if !ok {
						value = ""
						continue
					}
					processSQLiteValue(sessions, idx, value, mu)
					value = ""
				}
				rows.Close()
			}
		}

		// Single scan of conversations (v1)
		if AppConfig.SQLiteIndex {
			rows, err := conn.Query("SELECT key, value FROM conversations")
			if err == nil {
				for rows.Next() {
					var key, value string
					if rows.Scan(&key, &value) != nil {
						continue
					}
					idx, ok := sqlIdx["v1:"+key]
					if !ok {
						continue
					}
					processSQLiteValue(sessions, idx, value, mu)
					value = ""
				}
				rows.Close()
			}
		}
	}

	if done != nil {
		done()
	}
}

// indexJSONLFast extracts text content from JSONL using lightweight JSON parsing.
// Only parses the "data" field of Prompt/AssistantMessage lines.
func indexJSONLFast(path string) string {
	info, err := os.Stat(path)
	if err != nil || info.Size() == 0 || info.Size() > MaxFileSize {
		return ""
	}

	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var b strings.Builder
	dec := json.NewDecoder(f)
	for dec.More() {
		var line struct {
			Kind string          `json:"kind"`
			Data json.RawMessage `json:"data"`
		}
		if dec.Decode(&line) != nil {
			break
		}
		if line.Kind != "Prompt" && line.Kind != "AssistantMessage" {
			continue
		}
		// Extract text blocks only
		var d struct {
			Content []struct {
				Kind string `json:"kind"`
				Data string `json:"data"`
			} `json:"content"`
		}
		if json.Unmarshal(line.Data, &d) != nil {
			continue
		}
		for _, block := range d.Content {
			if block.Kind == "text" && block.Data != "" {
				b.WriteString(strings.ToLower(block.Data))
				b.WriteByte('\n')
				break
			}
		}
	}
	return b.String()
}

// indexSQLiteBulk loads all sessions of a given source type in one query.
// processSQLiteValue parses one conversation blob, extracts title + search text + msg count.
func processSQLiteValue(sessions []Session, idx int, value string, mu *sync.RWMutex) {
	var d struct {
		History []json.RawMessage `json:"history"`
	}
	if json.Unmarshal([]byte(value), &d) != nil {
		return
	}

	// Title from first prompt
	title := ""
	if len(d.History) > 0 {
		title = FirstPrompt(d.History[:1])
	}

	// Extract all message text for search
	msgs := extractHistoryMsgs(d.History, 0)
	d.History = nil

	var b strings.Builder
	if title != "" && title != "(untitled)" {
		b.WriteString(strings.ToLower(title))
		b.WriteByte('\n')
	}
	b.WriteString(strings.ToLower(sessions[idx].Cwd))
	b.WriteByte('\n')
	for _, m := range msgs {
		b.WriteString(strings.ToLower(m.Text))
		b.WriteByte('\n')
	}
	count := len(msgs)
	msgs = nil

	mu.Lock()
	if title != "" && title != "(untitled)" {
		sessions[idx].Title = title
	}
	sessions[idx].SearchText = b.String()
	sessions[idx].MsgCount = count
	mu.Unlock()
}

// findPromptInPeek extracts the first user prompt from a truncated JSON string.
func findPromptInPeek(peek string) string {
	key := `"prompt":"`
	pos := strings.Index(peek, key)
	if pos < 0 {
		return ""
	}
	start := pos + len(key)
	end := strings.Index(peek[start:], `"`)
	if end <= 0 {
		return ""
	}
	txt := peek[start : start+end]
	if len(txt) > 60 {
		return txt[:60]
	}
	return txt
}
