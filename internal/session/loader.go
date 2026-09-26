package session

import (
	"encoding/json"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// LoadAll loads all sessions, deduplicates, sorts newest first, links
// rewind/subagent relationships and builds a quick index (title + cwd).
func LoadAll() []Session {
	out := LoadJSONL()

	var sqlite []Session
	if AppConfig.SQLiteEnabled {
		sqlite = LoadSQLite()
	}

	seen := make(map[string]bool, len(out))
	for _, s := range out {
		if s.SessionID != "" {
			seen[s.SessionID] = true
		}
	}
	for _, s := range sqlite {
		if s.SessionID != "" && !seen[s.SessionID] {
			out = append(out, s)
			seen[s.SessionID] = true
		}
	}

	sort.SliceStable(out, func(i, j int) bool {
		return sortKey(&out[i]) > sortKey(&out[j])
	})
	Link(out)
	for i := range out {
		out[i].SearchText = baseSearchText(&out[i])
	}
	return out
}

func sortKey(s *Session) string {
	if s.UpdatedAt != "" {
		return s.UpdatedAt
	}
	return s.CreatedAt
}

func baseSearchText(s *Session) string {
	return strings.ToLower(s.Title) + "\n" + strings.ToLower(s.Cwd) + "\n"
}

// IndexResult is the full-text data for one session. It is computed off the
// UI goroutine and applied with ApplyIndex, so sessions are never mutated
// concurrently.
type IndexResult struct {
	Done  bool
	Text  string // lowercased message text, appended to SearchText
	Count int    // visible messages (same rules as the preview)
	Title string // replacement title; "" keeps the current one
}

// BuildFullIndex reads message content for every session. It only reads
// sessions; results[i] belongs to sessions[i].
func BuildFullIndex(sessions []Session) []IndexResult {
	res := make([]IndexResult, len(sessions))

	workers := runtime.NumCPU() - 1 // leave a core for the UI
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				text, count := indexJSONL(sessions[i].JSONLPath)
				res[i] = IndexResult{Done: true, Text: text, Count: count}
			}
		}()
	}
	for i := range sessions {
		if sessions[i].JSONLPath != "" {
			jobs <- i
		}
	}
	close(jobs)
	wg.Wait()

	if AppConfig.SQLiteEnabled && AppConfig.SQLiteIndex {
		indexSQLite(sessions, res)
	}
	return res
}

// ApplyIndex copies index results into sessions. Call from the UI goroutine.
func ApplyIndex(sessions []Session, res []IndexResult) {
	if len(res) != len(sessions) {
		return
	}
	for i := range res {
		r := &res[i]
		if !r.Done {
			continue
		}
		s := &sessions[i]
		if r.Title != "" {
			s.Title = r.Title
		}
		s.MsgCount = r.Count
		s.SearchText = baseSearchText(s) + r.Text
	}
}

// indexSQLite fills results for classic-mode sessions (full content scan).
func indexSQLite(sessions []Session, res []IndexResult) {
	conn := openDB()
	if conn == nil {
		return
	}
	v2 := make(map[string]int)
	v1 := make(map[string]int)
	for i := range sessions {
		switch sessions[i].Source {
		case "sqlite_v2":
			v2[sessions[i].SessionID] = i
		case "sqlite_v1":
			v1[sessions[i].Cwd] = i
		}
	}
	scan := func(query string, idx map[string]int) {
		if len(idx) == 0 {
			return
		}
		rows, err := conn.Query(query)
		if err != nil {
			return
		}
		defer rows.Close()
		for rows.Next() {
			var key, value string
			if rows.Scan(&key, &value) != nil {
				continue
			}
			if i, ok := idx[key]; ok {
				res[i] = sqliteResult(value)
			}
		}
	}
	scan("SELECT conversation_id, value FROM conversations_v2", v2)
	scan("SELECT key, value FROM conversations", v1)
}

// sqliteResult parses one conversation blob into an IndexResult.
func sqliteResult(value string) IndexResult {
	var d struct {
		History []json.RawMessage `json:"history"`
	}
	if json.Unmarshal([]byte(value), &d) != nil {
		return IndexResult{}
	}
	r := IndexResult{Done: true}
	if len(d.History) > 0 {
		if t := FirstPrompt(d.History[:1]); t != "(untitled)" {
			r.Title = cleanTitle(t)
		}
	}
	msgs := extractHistoryMsgs(d.History, 0)
	var b strings.Builder
	for _, m := range msgs {
		b.WriteString(strings.ToLower(m.Text))
		b.WriteByte('\n')
	}
	r.Text = b.String()
	r.Count = len(msgs)
	return r
}
