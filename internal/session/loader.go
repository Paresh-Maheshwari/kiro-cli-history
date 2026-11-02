package session

import (
	"encoding/json"
	"errors"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// LoadFilter narrows which sessions LoadAll returns.
type LoadFilter struct {
	Cwd string // if set, keep sessions with a workspace matching this path
}

// LoadAll loads all sessions, deduplicates, sorts newest first, links
// rewind/subagent relationships and builds a quick index (title + cwd).
func LoadAll() []Session {
	return LoadAllFiltered(LoadFilter{})
}

// LoadAllFiltered is LoadAll with an optional filter. Filtering happens before
// full-text indexing, so a directory filter also makes startup much faster:
// only the matching sessions' message files are read.
func LoadAllFiltered(f LoadFilter) []Session {
	// Prefer the V3 copy when a V2/SQLite conversation has been migrated.
	candidates := append(LoadV3(), LoadJSONL()...)
	if CurrentConfig().SQLiteEnabled {
		candidates = append(candidates, LoadSQLite()...)
	}
	var out []Session
	seen := make(map[string]bool, len(candidates))
	aliases := make(map[string]string)
	for _, s := range candidates {
		old := legacyV3ID(s)
		if s.SessionID != "" && (seen[s.SessionID] || old != "" && seen[old]) {
			continue
		}
		out = append(out, s)
		if s.SessionID != "" {
			seen[s.SessionID] = true
		}
		if old != "" {
			seen[old] = true
			aliases[old] = s.SessionID
		}
	}
	for i := range out {
		if id, ok := aliases[out[i].ParentID]; ok {
			out[i].ParentID = id
		}
	}

	if f.Cwd != "" {
		want := normPath(f.Cwd)
		kept := out[:0]
		for _, s := range out {
			for _, cwd := range s.Workspaces() {
				if normPath(cwd) == want {
					// Resume a multi-root conversation in the requested workspace.
					s.Cwd = cwd
					kept = append(kept, s)
					break
				}
			}
		}
		out = kept
	}

	sort.SliceStable(out, func(i, j int) bool {
		return sortKey(&out[i]).After(sortKey(&out[j]))
	})
	Link(out)
	for i := range out {
		out[i].SearchText = baseSearchText(&out[i])
	}
	return out
}

// normPath makes directory comparison tolerant of a trailing separator.
func normPath(p string) string {
	if len(p) > 1 {
		trimmed := strings.TrimRight(p, `/\`)
		if trimmed == "" {
			return p[:1] // filesystem root
		}
		if len(trimmed) == 2 && trimmed[1] == ':' && len(p) > 2 {
			return p[:3] // keep C:\ absolute instead of turning it into C:
		}
		p = trimmed
	}
	return p
}

func sortKey(s *Session) time.Time {
	if s.UpdatedAt != "" {
		if t := parseTime(s.UpdatedAt); !t.IsZero() {
			return t
		}
	}
	return parseTime(s.CreatedAt)
}

func baseSearchText(s *Session) string {
	return strings.ToLower(s.Title) + "\n" + strings.ToLower(s.Cwd) + "\n" +
		strings.ToLower(strings.Join(s.WorkspacePaths, "\n")) + "\n"
}

// IndexResult is the full-text data for one session. It is computed off the
// UI goroutine and applied with ApplyIndex, so sessions are never mutated
// concurrently.
type IndexResult struct {
	Done    bool
	Text    string // lowercased message text, appended to SearchText
	Count   int    // visible messages (same rules as the preview)
	Title   string // replacement title; "" keeps the current one
	Limited bool   // full-text content was unreadable or exceeded the indexing budget
}

// BuildFullIndex reads message content for every session. It only reads
// sessions; results[i] belongs to sessions[i].
func BuildFullIndex(sessions []Session) []IndexResult {
	return BuildFullIndexWithConfig(sessions, CurrentConfig())
}

// BuildFullIndexWithConfig uses the settings captured when indexing was requested.
func BuildFullIndexWithConfig(sessions []Session, cfg Config) []IndexResult {
	res := make([]IndexResult, len(sessions))
	budget := newIndexBudget(MaxIndexBytes)

	workers := min(4, runtime.NumCPU()-1) // bound concurrent decoder memory
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
				text, count, limited := indexJSONLBounded(sessions[i].JSONLPath, budget)
				res[i] = IndexResult{Done: true, Text: text, Count: count, Limited: limited}
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

	if cfg.SQLiteEnabled && cfg.SQLiteIndex {
		indexSQLite(sessions, res, budget)
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
func indexSQLite(sessions []Session, res []IndexResult, budget *indexBudget) {
	conn := openDB()
	if conn == nil {
		return
	}
	for i := range sessions {
		s := sessions[i]
		if s.Source != "sqlite_v1" && s.Source != "sqlite_v2" {
			continue
		}
		value, err := sqliteValue(conn, s)
		if err != nil {
			res[i].Limited = errors.Is(err, errConversationTooLarge)
			continue
		}
		res[i] = sqliteResultWithBudget(value, budget)
	}
}

// sqliteResult parses one conversation blob into an IndexResult.
func sqliteResult(value string) IndexResult {
	return sqliteResultWithBudget(value, newIndexBudget(MaxIndexBytes))
}

func sqliteResultWithBudget(value string, budget *indexBudget) IndexResult {
	if len(value) > MaxFileSize {
		return IndexResult{Limited: true}
	}
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
	b := indexText{budget: budget}
	for _, raw := range d.History {
		for _, m := range extractHistoryMsgs([]json.RawMessage{raw}, 0) {
			b.add(m.Text)
			r.Count++
		}
	}
	r.Text = b.text.String()
	r.Limited = b.limited
	return r
}
