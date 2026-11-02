package session

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var (
	sqliteDB *sql.DB
	sqliteMu sync.Mutex
)

var errConversationTooLarge = errors.New("conversation exceeds the 100 MiB file limit")

func openDB() *sql.DB {
	sqliteMu.Lock()
	defer sqliteMu.Unlock()
	if sqliteDB != nil {
		return sqliteDB
	}
	_, dbPath := Paths()
	if dbPath == "" {
		return nil
	}
	if _, err := os.Stat(dbPath); err != nil {
		return nil // classic mode never used; don't let SQLite create a file
	}
	conn, err := sql.Open("sqlite3", sqliteURI(dbPath))
	if err != nil {
		return nil
	}
	if conn.Ping() != nil {
		conn.Close()
		return nil
	}
	sqliteDB = conn
	return sqliteDB
}

// sqliteURI returns a read-only SQLite URI for path. It works for paths with
// spaces (macOS "Application Support") and Windows drive letters.
func sqliteURI(path string) string {
	p := filepath.ToSlash(path)
	if len(p) > 0 && p[0] != '/' {
		p = "/" + p // C:/x -> /C:/x, as SQLite expects
	}
	u := url.URL{Scheme: "file", Path: p, RawQuery: "mode=ro"}
	return u.String()
}

// CloseDB closes the shared SQLite connection.
func CloseDB() {
	sqliteMu.Lock()
	defer sqliteMu.Unlock()
	if sqliteDB != nil {
		sqliteDB.Close()
		sqliteDB = nil
	}
}

// LoadSQLite reads session metadata only — no full history loaded.
func LoadSQLite() []Session {
	conn := openDB()
	if conn == nil {
		return nil
	}
	var out []Session
	out = append(out, loadV2Meta(conn)...)
	out = append(out, loadV1Meta(conn)...)
	return out
}

func loadV2Meta(conn *sql.DB) []Session {
	rows, err := conn.Query(
		"SELECT key, conversation_id, created_at, updated_at FROM conversations_v2 ORDER BY updated_at DESC",
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var cwd, convID string
		var cMs, uMs int64
		if rows.Scan(&cwd, &convID, &cMs, &uMs) != nil {
			continue
		}

		out = append(out, Session{
			SessionID:   convID,
			Title:       filepath.Base(cwd),
			Cwd:         cwd,
			CreatedAt:   time.UnixMilli(cMs).UTC().Format(time.RFC3339Nano),
			UpdatedAt:   time.UnixMilli(uMs).UTC().Format(time.RFC3339Nano),
			Source:      "sqlite_v2",
			DurationMin: int((uMs - cMs) / 1000 / 60),
		})
	}
	if err := rows.Err(); err != nil {
		return out
	}
	return out
}

func loadV1Meta(conn *sql.DB) []Session {
	rows, err := conn.Query("SELECT key FROM conversations")
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []Session
	for rows.Next() {
		var cwd string
		if rows.Scan(&cwd) != nil {
			continue
		}

		out = append(out, Session{
			Title:  filepath.Base(cwd),
			Cwd:    cwd,
			Source: "sqlite_v1",
		})
	}
	if err := rows.Err(); err != nil {
		return out
	}
	return out
}

// LoadSQLiteHistory loads the full history for a single session on-demand.
func LoadSQLiteHistory(s Session) []json.RawMessage {
	history, _ := loadSQLiteHistory(s)
	return history
}

func loadSQLiteHistory(s Session) ([]json.RawMessage, error) {
	conn := openDB()
	if conn == nil {
		return nil, errors.New("classic conversation storage is unavailable")
	}
	value, err := sqliteValue(conn, s)
	if err != nil {
		return nil, err
	}
	var d struct {
		History []json.RawMessage `json:"history"`
	}
	if err := json.Unmarshal([]byte(value), &d); err != nil {
		return nil, err
	}
	return d.History, nil
}

func sqliteValue(conn *sql.DB, s Session) (string, error) {
	var value sql.NullString
	var err error

	switch s.Source {
	case "sqlite_v2":
		err = conn.QueryRow(
			"SELECT CASE WHEN length(CAST(value AS BLOB)) <= ? THEN value END FROM conversations_v2 WHERE conversation_id = ?",
			MaxFileSize, s.SessionID,
		).Scan(&value)
	case "sqlite_v1":
		err = conn.QueryRow(
			"SELECT CASE WHEN length(CAST(value AS BLOB)) <= ? THEN value END FROM conversations WHERE key = ?",
			MaxFileSize, s.Cwd,
		).Scan(&value)
	default:
		return "", fmt.Errorf("unknown classic source %q", s.Source)
	}

	if err != nil {
		return "", err
	}
	if !value.Valid {
		return "", errConversationTooLarge
	}
	return value.String, nil
}
