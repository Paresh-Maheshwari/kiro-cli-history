# Kiro V3 session format

These notes describe the V3 files inspected locally while implementing the
reader. Examples are synthetic; no conversation content is copied here.
V3 is the agent engine name. Its on-disk `schemaVersion` was `"1.0.0"`.

## Files

```text
$KIRO_HOME/                         # defaults to ~/.kiro
  sessions/
    <workspace-key>/
      <session-id>/
        session.json               # metadata and relationships
        messages.jsonl             # saved conversation events
        snapshots/                 # not a separate conversation store
    cli/
      <old-id>.json                 # older V2 metadata
      <old-id>.jsonl                # older V2 messages
```

The reader discovers `sessions/*/*/session.json` at that depth only. It does
not depend on Kiro's index caches or scan snapshots for conversations.

## Session metadata

```json
{
  "schemaVersion": "1.0.0",
  "id": "sess-example",
  "title": "Example conversation",
  "workspacePaths": ["/projects/example"],
  "rootPaths": ["/projects/example"],
  "createdAt": "2026-11-01T10:00:00.000Z",
  "lastModifiedAt": "2026-11-01T10:30:00.000Z"
}
```

`workspacePaths` supplies the workspace membership; `rootPaths` is a fallback
when it is absent. The reader retains every workspace and chooses a matching
directory for `--here`/`--cwd` and resume.

A rewind has `createdReason: "rewind"` and `parentSessionId`. The observed
files also contain `rootConversationId` and `forkedAtMessageId`. The parent
chain identifies the rewind family; the newest version represents that family.

Workflow steps instead record ownership in metadata:

```json
{
  "_meta": {
    "kiro": {
      "serverSession": {
        "role": "workflow-step",
        "ownerSessionId": "sess-parent"
      }
    }
  }
}
```

The reader treats these as child sessions. It does not infer ownership from
titles, filenames, or directory proximity. Children remain browsable under
their immediate parent, including a parent that is an older rewind version.
A child remains visible if its parent is unavailable. Search expands matching
groups.

## Message events

Each line is an independent JSON object with an event ID, a timestamp, and a
`payload`. The V3 format differs from V2's `kind`/`data` envelope.

```jsonl
{"id":"message-1","timestamp":"2026-11-01T10:00:00Z","payload":{"type":"user","content":"Explain the project"}}
{"id":"message-2","timestamp":"2026-11-01T10:00:01Z","payload":{"type":"assistant","operationType":"Say","content":"This project is a terminal session browser."}}
```

The text reader includes:

- `payload.type == "user"` with a nonempty string `content`.
- `payload.type == "assistant"` with `operationType == "Say"`, or no
  `operationType`, and a nonempty string `content`.

Observed separate events include assistant `Reasoning`, `tool_call`,
`tool_result`, `turn_start`, `turn_end`, `session_metadata`, `session_start`,
`agent_note`, `pending_interaction`, `interaction_resolved`, `sub_agent_start`,
`sub_agent_complete`, and `steering_inclusion`. These are not visible
conversation messages and are excluded from previews, search, copy, and export.
Malformed, incomplete, and unsupported lines are skipped.

Preview, indexing, copy, and export use the same decoder so their visible
message counts agree for fully readable sessions. JSONL files have no total-size
cutoff. Each read uses the file length observed at its start; a later read sees
newly appended messages. Individual records larger than 64 MiB that cannot be
discarded as known non-message traffic are skipped with a partial-read notice,
and reading continues at the next record.

## Migrated sessions and resume

Observed V2 imports use IDs shaped like `cli_<original-UUID>_<suffix>`. The
reader prefers that V3 copy over a legacy copy, maps legacy parent references
to it, and keeps its complete V3 ID for resume:

```text
kiro-cli chat --v3 --resume-id <complete-v3-session-id>
```

Legacy sessions continue through their existing reader and resume path.
All session data is read-only; grouping is an in-memory presentation.
