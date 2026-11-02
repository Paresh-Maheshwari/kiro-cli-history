# Diff and performance review — 2 November 2026

Reviewed the working-tree changes against `HEAD`, including the new Go files,
tests, V3 format documentation, installers and CI/release changes. The existing
V3/workspace work was preserved. No commit or release was created.

## Corrections made in this review

- Removed the total-file-size cutoff from JSONL message reading, indexing,
  title fallback, copy and export. Large files are read as streams.
- Bounded each read to the file length observed when opened, so a conversation
  being appended to cannot keep an operation running indefinitely.
- Discarded recognized legacy tool events in small chunks without allocating
  their full records. Cancellation is checked during hidden traffic too.
- Continued reading after an oversized record, reporting the partial read
  instead of losing all subsequent messages.
- Stopped allocating and lowercasing text after the search-index budget is
  exhausted. The per-session budget now also accounts for Unicode characters
  whose lowercase spelling occupies more UTF-8 bytes.
- Initialized Markdown rendering only when needed. A message larger than the
  preview budget now shows its beginning and a limit notice.
- Made save errors visible while the settings overlay remains open.
- Preserved expanded child groups inside collapsed workspaces during tree
  rebuilds, and included formatting overhead in the clipboard size budget.

JSONL has no total-size cap. Individual undecoded records retain a 64 MiB cap;
known non-message records can be discarded even above that size. Metadata and
SQLite values retain their 100 MiB whole-object decoding limit. Preview, cache
and search-index memory budgets remain separate from file size.

## Measured comparison

These benchmarks compare the working tree immediately before this review with
the reviewed implementation, using the same synthetic inputs, Go toolchain and
machine. They do not compare against the original 1.4.0 release. Three
100-millisecond benchmark runs were collected for each version.

| Workload | Before | After |
| --- | --- | --- |
| Scan 10 MiB of legacy tool events plus one visible message | 4.10–4.26 ms | 1.80–1.84 ms |
| Allocations for that scan | 3.93 MB/op | 67.2 KB/op |
| Add a 2 MB message after the index budget is exhausted | 6.99–8.93 ms, 2.01 MB allocated | 25–32 ns, 8 bytes allocated |
| Preview a roughly 114 KB assistant message | 3.63–4.08 ms | 3.25–4.04 ms |

The tool-event workload uses roughly 98% fewer allocated bytes and runs about
2.3 times faster in this measurement. The exhausted-budget case demonstrates
avoided work, not an equivalent speedup for ordinary full-session indexing.
Preview timing is similar; no large timing improvement is claimed for it.
Allocated bytes are not peak RSS, and these local samples are not latency
guarantees.

Reproduce the current benchmarks with:

```sh
go test ./internal/session ./internal/ui -run '^$' \
  -bench 'Benchmark(JSONLToolTraffic|ExhaustedIndexBudget|LargeAssistantPreview)$' \
  -benchtime=100ms -count=3 -benchmem
```

## Existing-history validation

A read-only run found:

- 222 conversations, including 88 V3 sessions.
- 25,338 indexed visible messages, with zero sessions marked search-limited.
- 67 root chats across 40 workspaces; all 222 conversations remained reachable.
- V3 extracted messages and counts matching the shared index decoder.

The previously excluded, approximately 105 MiB JSONL history was included.
Discovery took 101 ms, indexing 2.13 seconds, average search 2.07 ms, and tree
construction 0.30 ms. Retained search text was 15.6 MB. Heap allocation at the
sample was 21.0 MB; this is not a peak-memory measurement. No conversation text
was copied into this repository or printed in the measurements.

## Checks and remaining validation

Passed:

- `go test -race -cover ./...`, including large-file streaming, cancellation,
  oversized-record recovery, growing-file snapshots, Unicode budgets and
  existing terminal/export regression tests.
- `go vet ./...`, formatting, `git diff --check`, and Bash syntax checks.
- Native Linux CGO build and version smoke test.
- Linux, macOS and Windows amd64/arm64 cross-compilation with CGO disabled.

No additional blocking defect remains in the reviewed Linux paths. Native
macOS/Windows execution, native CGO combinations, and PowerShell installer
recovery still require the configured CI matrix before publication.
