package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Synthetic fixtures follow the on-disk V3 schema; no private chats are used.
func writeV3Session(t *testing.T, root, workspace string, meta map[string]any, lines ...string) string {
	t.Helper()
	dir := filepath.Join(root, "kiro", "sessions", workspace, meta["id"].(string))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "session.json", string(data))
	return writeFile(t, dir, "messages.jsonl", strings.Join(lines, "\n"))
}

func TestDecodeV3Msg(t *testing.T) {
	tests := []struct {
		name, payload, role, text string
	}{
		{"user", `{"type":"user","content":"  hello  ","contextItems":[{"content":"hidden context"}]}`, "you", "hello"},
		{"assistant", `{"type":"assistant","content":"answer","operationType":"Say"}`, "kiro", "answer"},
		{"assistant without operation", `{"type":"assistant","content":"older answer"}`, "kiro", "older answer"},
		{"reasoning", `{"type":"assistant","content":"private reasoning","operationType":"Reasoning"}`, "", ""},
		{"tool result", `{"type":"tool_result","content":"tool output"}`, "", ""},
		{"agent note", `{"type":"agent_note","content":"internal summary"}`, "", ""},
		{"system prompt", `{"type":"session_start","content":"system prompt"}`, "", ""},
		{"metadata", `{"type":"session_metadata","key":"title","value":"new title"}`, "", ""},
		{"empty", `{"type":"user","content":" \n "}`, "", ""},
		{"null", `{"type":"user","content":null}`, "", ""},
		{"bad content", `{"type":"user","content":{"text":"unexpected"}}`, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Exercise the shared decoder, including the envelope and key order.
			line := `{"payload":` + tt.payload + `,"id":"message-1","timestamp":"2026-11-01T12:00:00Z"}`
			got, ok := decodeMsg([]byte(line))
			if ok != (tt.role != "") {
				t.Fatalf("visible=%v, want role %q", ok, tt.role)
			}
			if ok && (got.Role != tt.role || got.Text != tt.text) {
				t.Fatalf("got %+v, want %q/%q", got, tt.role, tt.text)
			}
		})
	}
	if _, ok := decodeMsg([]byte(`{"id":"unfinished","payload":{"type":"user"`)); ok {
		t.Fatal("partially written line should be skipped")
	}
}

func TestLoadV3MetadataAndDiscovery(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KIRO_DEMO_DIR", root)
	prompt := `{"id":"p","payload":{"type":"user","content":"  fix   the\tbug\nDetails"}}`
	writeV3Session(t, root, "workspace-hash", map[string]any{
		"schemaVersion": "1.0.0", "id": "sess-main", "title": nil,
		"workspacePaths": []string{"/work/main", "/work/second"},
		"createdAt":      "2026-11-01T10:00:00Z", "lastModifiedAt": "2026-11-01T10:45:00Z",
	}, prompt)
	writeV3Session(t, root, "workspace-hash", map[string]any{
		"id": "sess-child", "title": "Workflow\nstep", "rootPaths": []string{"/work/main"},
		"_meta": map[string]any{"kiro": map[string]any{
			"serverSession": map[string]any{"role": "workflow-step", "ownerSessionId": "sess-main"},
		}},
	}, prompt)
	writeV3Session(t, root, "workspace-hash", map[string]any{
		"id": "sess-rewind", "workspacePaths": []string{"/work/main"},
		"createdReason": "rewind", "parentSessionId": "sess-main", "rootConversationId": "sess-main",
	}, prompt)
	writeV3Session(t, root, "workspace-hash", map[string]any{"id": "empty"}, "")
	missing := writeV3Session(t, root, "workspace-hash", map[string]any{"id": "missing"}, prompt)
	if err := os.Remove(missing); err != nil {
		t.Fatal(err)
	}
	bad := writeV3Session(t, root, "workspace-hash", map[string]any{"id": "bad"}, prompt)
	writeFile(t, filepath.Dir(bad), "session.json", "{broken")
	noID := writeV3Session(t, root, "workspace-hash", map[string]any{"id": "no-id"}, prompt)
	writeFile(t, filepath.Dir(noID), "session.json", `{"title":"no id"}`)
	// A snapshot can contain session.json too; it must not become a chat.
	nested := filepath.Join(root, "kiro", "sessions", "workspace-hash", "sess-main", "snapshots", "copy")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, nested, "session.json", `{"id":"snapshot"}`)
	writeFile(t, nested, "messages.jsonl", prompt)

	all := LoadV3()
	if len(all) != 3 {
		t.Fatalf("loaded %d, want 3: %+v", len(all), all)
	}
	byID := make(map[string]Session)
	for _, s := range all {
		byID[s.SessionID] = s
	}
	main := byID["sess-main"]
	if main.Title != "fix the bug" || main.Cwd != "/work/main" || main.Source != "jsonl_v3" ||
		main.DurationMin != 45 || len(main.WorkspacePaths) != 2 {
		t.Fatalf("bad main metadata: %+v", main)
	}
	child := byID["sess-child"]
	if child.Title != "Workflow step" || child.Cwd != "/work/main" || !child.IsSubagent() || child.ParentID != main.SessionID {
		t.Fatalf("bad child metadata: %+v", child)
	}
	rewind := byID["sess-rewind"]
	if !rewind.IsRewind() || rewind.ParentID != main.SessionID {
		t.Fatalf("bad rewind metadata: %+v", rewind)
	}
	Link(all)
	rows, children := Collapse(all, true, true)
	if len(rows) != 1 || len(children[rows[0].SessionID]) != 2 {
		t.Fatalf("V3 grouping failed: rows=%+v children=%+v", rows, children)
	}
}

func TestV3PreviewAndIndexAgree(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KIRO_DEMO_DIR", root)
	path := writeV3Session(t, root, "workspace", map[string]any{"id": "sess-chat"},
		`{"id":"u","payload":{"type":"user","content":"Deploy the service"}}`,
		`{"id":"think","payload":{"type":"assistant","operationType":"Reasoning","content":"reasoning-only secret"}}`,
		`{"id":"t","payload":{"type":"tool_result","content":"tool-only secret"}}`,
		`{"id":"a","payload":{"type":"assistant","operationType":"Say","content":"Deployment complete"}}`,
		`not json`,
		`{"id":"partial","payload":`,
	)
	s := LoadV3()[0]
	msgs := ExtractMessages(s, 0)
	if len(msgs) != 2 || msgs[0].Role != "you" || msgs[1].Role != "kiro" {
		t.Fatalf("unexpected visible messages: %+v", msgs)
	}
	if limited := ExtractMessages(s, 1); len(limited) != 1 {
		t.Fatalf("limit ignored: %+v", limited)
	}
	text, count := indexJSONL(path)
	if count != len(msgs) || !strings.Contains(text, "deployment complete") || strings.Contains(text, "secret") {
		t.Fatalf("index count=%d text=%q", count, text)
	}
	sessions := []Session{s}
	ApplyIndex(sessions, BuildFullIndex(sessions))
	if sessions[0].MsgCount != 2 || !strings.Contains(sessions[0].SearchText, "deploy the service") {
		t.Fatalf("full index failed: %+v", sessions[0])
	}
}

func TestLoadMixedStoresAndMigratedIDs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KIRO_DEMO_DIR", root)
	cfg := CurrentConfig()
	t.Cleanup(func() { SetConfig(cfg) })
	testCfg := DefaultConfig
	testCfg.SQLiteEnabled = false
	SetConfig(testCfg)

	oldID := "12345678-1234-1234-1234-123456789abc"
	newID := "cli_" + oldID + "_AbC12345"
	cli := filepath.Join(root, "kiro", "sessions", "cli")
	if err := os.MkdirAll(cli, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{oldID, "legacy-only", "legacy-child"} {
		parent := ""
		reason := ""
		if id == "legacy-child" {
			parent, reason = oldID, "subagent"
		}
		meta, _ := json.Marshal(map[string]any{
			"session_id": id, "title": id, "cwd": "/work/main",
			"parent_session_id": parent, "session_created_reason": reason,
		})
		writeFile(t, cli, id+".json", string(meta))
		writeFile(t, cli, id+".jsonl", `{"kind":"Prompt","data":{"content":[{"kind":"text","data":"old prompt"}]}}`)
	}
	writeV3Session(t, root, "workspace", map[string]any{
		"id": newID, "title": "Migrated", "workspacePaths": []string{"/work/main", "/work/second"},
		"lastModifiedAt": "2026-11-01T12:00:00Z",
	}, `{"payload":{"type":"user","content":"new prompt"}}`)
	writeV3Session(t, root, "workspace", map[string]any{
		"id": "sess-new", "title": "Native V3", "workspacePaths": []string{"/work/other"},
	}, `{"payload":{"type":"user","content":"native prompt"}}`)

	all := LoadAll()
	if len(all) != 4 {
		t.Fatalf("want migrated, native, legacy-only, legacy-child; got %+v", all)
	}
	for _, s := range all {
		if s.SessionID == oldID {
			t.Fatal("legacy copy of migrated session should not be shown")
		}
		if s.SessionID == "legacy-child" && (s.ParentID != newID || s.ParentTitle != "Migrated") {
			t.Fatalf("parent alias not linked: %+v", s)
		}
	}
	filtered := LoadAllFiltered(LoadFilter{Cwd: "/work/second/"})
	if len(filtered) != 1 || filtered[0].SessionID != newID || filtered[0].Cwd != "/work/second" {
		t.Fatalf("multi-workspace filter failed: %+v", filtered)
	}
	if !strings.Contains(filtered[0].SearchText, "/work/main") {
		t.Fatal("all workspace paths must remain searchable")
	}
}

func TestV3HonorsKiroHome(t *testing.T) {
	root := t.TempDir()
	t.Setenv("KIRO_DEMO_DIR", "")
	t.Setenv("KIRO_HOME", filepath.Join(root, "kiro"))
	writeV3Session(t, root, "workspace", map[string]any{"id": "sess-home"},
		`{"payload":{"type":"user","content":"hello"}}`)
	if all := LoadV3(); len(all) != 1 || all[0].SessionID != "sess-home" {
		t.Fatalf("KIRO_HOME ignored: %+v", all)
	}
}

func TestLegacyV3ID(t *testing.T) {
	id := "12345678-1234-1234-1234-123456789abc"
	for _, tt := range []struct {
		source, id, want string
	}{
		{"jsonl_v3", "cli_" + id + "_AbC12345", id},
		{"jsonl_v3", "cli_" + id + "_a-b_c", id},
		{"jsonl_v3", "sess_" + id, ""},
		{"jsonl_v3", "cli_not-a-uuid_suffix", ""},
		{"jsonl_v3", "cli_" + id + "_", ""},
		{"jsonl", "cli_" + id + "_AbC12345", ""},
	} {
		if got := legacyV3ID(Session{Source: tt.source, SessionID: tt.id}); got != tt.want {
			t.Errorf("legacyV3ID(%q, %q)=%q, want %q", tt.source, tt.id, got, tt.want)
		}
	}
}
