package session

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// maxLineSize bounds one decoded record, not the conversation file. Known
// non-message records are discarded while streaming, regardless of their size.
const maxLineSize = 64 * 1024 * 1024

var errJSONLRecordTooLarge = errors.New("one or more JSONL records exceed the 64 MiB decoding limit")

// jsonlMeta holds the fields we need from <id>.json.
type jsonlMeta struct {
	SessionID     string
	Title         string
	Cwd           string
	CreatedAt     string
	UpdatedAt     string
	ParentID      string
	CreatedReason string
}

type jsonlLine struct {
	Kind    string          `json:"kind"`
	Data    json.RawMessage `json:"data"`
	Payload json.RawMessage `json:"payload"` // V3 message envelope
}

// jsonlBlock is one content block. Data is a string for "text" blocks and an
// object for "toolUse", "thinking", "image", "toolResult", ...
type jsonlBlock struct {
	Kind string          `json:"kind"`
	Data json.RawMessage `json:"data"`
}

var errNotObject = errors.New("not a JSON object")

// metaHeadSize is how much of <id>.json we read for the list. The metadata
// fields come first and are well under 1 KB; the rest is session_state.
const metaHeadSize = 16 * 1024

// readMeta returns the metadata fields of <id>.json, reading only the first
// metaHeadSize bytes. Kiro CLI writes a large "session_state" object (per-turn
// metrics, often several MB) after the metadata, so decoding stops there. If
// the head doesn't contain a session id (unexpected field order), it falls
// back to reading the whole file.
func readMeta(path string) (jsonlMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return jsonlMeta{}, err
	}
	defer f.Close()

	m, err := decodeMeta(io.LimitReader(f, metaHeadSize))
	if m.SessionID != "" {
		return m, nil
	}
	if _, serr := f.Seek(0, io.SeekStart); serr != nil {
		return m, err
	}
	return decodeMeta(io.LimitReader(f, MaxFileSize))
}

func decodeMeta(r io.Reader) (jsonlMeta, error) {
	var m jsonlMeta
	dec := json.NewDecoder(r)
	tok, err := dec.Token()
	if err != nil {
		return m, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return m, errNotObject
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return m, err
		}
		key, _ := t.(string)

		var dst *string
		switch key {
		case "session_id":
			dst = &m.SessionID
		case "title":
			dst = &m.Title
		case "cwd":
			dst = &m.Cwd
		case "created_at":
			dst = &m.CreatedAt
		case "updated_at":
			dst = &m.UpdatedAt
		case "parent_session_id":
			dst = &m.ParentID
		case "session_created_reason":
			dst = &m.CreatedReason
		case "session_state":
			if m.SessionID != "" {
				return m, nil
			}
		}

		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return m, err
		}
		if dst != nil {
			// Tolerate null or unexpected types: leave the field empty.
			var s string
			if json.Unmarshal(raw, &s) == nil {
				*dst = s
			}
		}
	}
	return m, nil
}

// LoadJSONL reads session metadata from ~/.kiro/sessions/cli/*.json.
func LoadJSONL() []Session {
	dir, _ := Paths()
	if dir == "" {
		return nil
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil
	}

	out := make([]Session, 0, len(files))
	for _, f := range files {
		m, err := readMeta(f)
		if err != nil {
			continue
		}
		jpath := strings.TrimSuffix(f, ".json") + ".jsonl"
		// Sessions opened but never used have an empty (or no) .jsonl.
		if ji, err := os.Stat(jpath); err != nil || ji.Size() == 0 {
			continue
		}
		title := cleanTitle(m.Title)
		if title == "" {
			// Kiro CLI leaves title null for some sessions; use the first prompt.
			title = Truncate(cleanTitle(firstPromptJSONL(jpath)), 80)
		}
		if title == "" {
			title = "(untitled)"
		}

		out = append(out, Session{
			SessionID:     m.SessionID,
			Title:         title,
			Cwd:           m.Cwd,
			CreatedAt:     m.CreatedAt,
			UpdatedAt:     m.UpdatedAt,
			Source:        "jsonl",
			DurationMin:   computeDuration(m.CreatedAt, m.UpdatedAt),
			JSONLPath:     jpath,
			ParentID:      m.ParentID,
			CreatedReason: m.CreatedReason,
		})
	}
	return out
}

// cleanTitle collapses newlines/tabs/runs of spaces so a title fits one line.
func cleanTitle(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// linePrefix is how Kiro CLI starts every line; used to read "kind" cheaply.
var linePrefix = []byte(`{"version":"v1","kind":"`)

// peekKind returns the line's kind without decoding it, or "" when the line
// doesn't start with the expected prefix (caller must then decode fully).
func peekKind(line []byte) string {
	if !bytes.HasPrefix(line, linePrefix) {
		return ""
	}
	rest := line[len(linePrefix):]
	end := bytes.IndexByte(rest, '"')
	if end <= 0 || end > 64 {
		return ""
	}
	return string(rest[:end])
}

// decodeMsg turns a Prompt/AssistantMessage line into a Msg. It returns
// ok=false for other kinds, malformed lines, and messages with no visible text
// (e.g. assistant turns that only call tools).
func decodeMsg(line []byte) (Msg, bool) {
	if k := peekKind(line); k != "" && k != "Prompt" && k != "AssistantMessage" {
		return Msg{}, false // skip ToolResults etc. without parsing them
	}
	var l jsonlLine
	if json.Unmarshal(line, &l) != nil {
		return Msg{}, false
	}
	if l.Kind == "" && len(l.Payload) > 0 {
		return decodeV3Msg(l.Payload)
	}
	var role string
	switch l.Kind {
	case "Prompt":
		role = "you"
	case "AssistantMessage":
		role = "kiro"
	default:
		return Msg{}, false
	}
	var d struct {
		Content []jsonlBlock `json:"content"`
	}
	if json.Unmarshal(l.Data, &d) != nil {
		return Msg{}, false
	}
	var parts []string
	for _, b := range d.Content {
		if b.Kind != "text" {
			continue
		}
		var s string
		if json.Unmarshal(b.Data, &s) != nil {
			continue
		}
		if s = strings.TrimSpace(s); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return Msg{}, false
	}
	return Msg{Role: role, Text: strings.Join(parts, "\n\n")}, true
}

// scanJSONL calls fn for every visible message in the file, in order.
// Malformed or partially written lines are skipped. fn returns false to stop.
func scanJSONL(path string, fn func(Msg) bool) error {
	return scanJSONLWithCancel(path, fn, nil)
}

func scanJSONLWithCancel(path string, fn func(Msg) bool, abort func() bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("conversation is not a regular file")
	}
	// Read a snapshot of the current length. An actively growing conversation
	// must not keep an index/export operation running indefinitely.
	reader := bufio.NewReaderSize(io.NewSectionReader(f, 0, info.Size()), 64*1024)
	var record []byte
	var recordErr error
	discard := false
	for {
		if abort != nil && abort() {
			return context.Canceled
		}
		fragment, readErr := reader.ReadSlice('\n')
		if !discard && len(record) == 0 {
			if kind := peekKind(fragment); kind != "" && kind != "Prompt" && kind != "AssistantMessage" {
				discard = true
			}
		}
		if !discard {
			if len(record)+len(fragment) > maxLineSize {
				recordErr = errJSONLRecordTooLarge
				discard = true
				record = nil
			} else if readErr == bufio.ErrBufferFull || len(record) > 0 {
				record = append(record, fragment...)
			}
		}
		if readErr != bufio.ErrBufferFull {
			if !discard {
				line := fragment
				if len(record) > 0 {
					line = record
				}
				if m, ok := decodeMsg(line); ok && !fn(m) {
					return recordErr
				}
			}
			discard = false
			// Do not retain a very large record buffer for the rest of a file.
			if cap(record) > 1024*1024 {
				record = nil
			} else {
				record = record[:0]
			}
		}
		if readErr == io.EOF {
			return recordErr
		}
		if readErr != nil && readErr != bufio.ErrBufferFull {
			return errors.Join(recordErr, readErr)
		}
	}
}

// extractJSONLMessages reads up to limit messages (0 = all) from a .jsonl file.
func extractJSONLMessages(path string, limit int) []Msg {
	var msgs []Msg
	scanJSONL(path, func(m Msg) bool {
		msgs = append(msgs, m)
		return limit <= 0 || len(msgs) < limit
	})
	return msgs
}

// indexJSONL returns the lowercased searchable text and the message count,
// using exactly the same rules as the preview.
func indexJSONL(path string) (text string, count int) {
	text, count, _ = indexJSONLBounded(path, newIndexBudget(MaxIndexBytes))
	return text, count
}

func indexJSONLBounded(path string, budget *indexBudget) (text string, count int, limited bool) {
	b := indexText{budget: budget}
	err := scanJSONL(path, func(m Msg) bool {
		b.add(m.Text)
		count++
		return true
	})
	return b.text.String(), count, b.limited || err != nil
}

// firstPromptJSONL returns the first line of the first user prompt.
func firstPromptJSONL(path string) string {
	var first string
	scanJSONL(path, func(m Msg) bool {
		if m.Role != "you" {
			return true
		}
		first, _, _ = strings.Cut(m.Text, "\n")
		return false
	})
	return first
}
