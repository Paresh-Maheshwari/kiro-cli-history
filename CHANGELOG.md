# Changelog

## 1.5.1

- Include the V3 support, V2 compatibility, workspace grouping, and reliability improvements listed below.
- Fix the Windows command installer when launched from PowerShell 7: reset the child module path so Windows PowerShell can load its own checksum commands.
- Exercise both PowerShell and command installers in regular Windows CI, including checksum rejection and failed-update recovery.

## 1.5.0 (not published)

Publication was blocked by the Windows installer check; these changes are included in 1.5.1.

- Support Kiro's V3 session metadata and message envelopes alongside legacy JSONL and SQLite.
- Group conversations by workspace and nest workflow/subagent sessions, including rewound subagents. Deduplicate migrated histories and preserve search/expansion behavior.
- Neutralize terminal commands in metadata, messages and rendered Markdown while preserving original copy/export content.
- Export privately on Unix with unique filenames; stream export and run preview/copy/export work in the background.
- Bound preview/cache/index memory, provide explicit limit notices and time out external clipboard helpers.
- Stream JSONL conversations without a total-file-size cutoff; skip hidden tool traffic with bounded buffers, cancel stale reads promptly, and keep later messages readable after an oversized record.
- Preserve hidden tree expansion choices, show save errors inside settings, and preview the beginning of messages that exceed the display budget.
- Synchronize settings and SQLite connection access; save settings atomically with visible errors.
- Sort timestamps chronologically across timezones, guard unsupported legacy resume, and fix quitting/small-terminal behavior.
- Require installer checksums and binary version validation, preserve existing installations on failure, and test staged installers before release publication.
- Update vulnerable dependencies and require Go 1.26.10 or newer.
