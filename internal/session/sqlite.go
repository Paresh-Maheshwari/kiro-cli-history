package session

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var sqliteDB *sql.DB

func openDB() *sql.DB {
	if sqliteDB != nil {
		return sqliteDB
	}
	_, dbPath := Paths()
	if dbPath == "" {
		return nil
	}
	conn, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro", dbPath))
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

// CloseDB closes the shared SQLite connection.
func CloseDB() {
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
			CreatedAt:   time.UnixMilli(cMs).Format("2006-01-02T15:04:05"),
			UpdatedAt:   time.UnixMilli(uMs).Format("2006-01-02T15:04:05"),
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
	conn := openDB()
	if conn == nil {
		return nil
	}

	var value string
	var err error

	switch s.Source {
	case "sqlite_v2":
		err = conn.QueryRow(
			"SELECT value FROM conversations_v2 WHERE conversation_id = ?", s.SessionID,
		).Scan(&value)
	case "sqlite_v1":
		err = conn.QueryRow(
			"SELECT value FROM conversations WHERE key = ?", s.Cwd,
		).Scan(&value)
	default:
		return nil
	}

	if err != nil {
		return nil
	}

	var d struct {
		History []json.RawMessage `json:"history"`
	}
	if json.Unmarshal([]byte(value), &d) != nil {
		return nil
	}
	return d.History
}
