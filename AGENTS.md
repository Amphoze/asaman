# asaman — Agent Notes

Go CLI that parses an atomic memory-fact store and renders a canonical rules index.
Key packages: `internal/memory` (fact parsing + index render), `internal/config`
(asaman.toml loader), `internal/cli` (commands).

Build/install: `make build` / `make install` (static binary → `~/.local/bin/asaman`,
ldflags `-s -w`, CGO off). Test: `make test`.

## Changelog

- 2026-09-27: Added configurable index density. `Config.IndexDensity` (`index_density`
  in asaman.toml) accepts `full` | `hybrid` | `names`, defaults to `hybrid`; unknown
  non-empty values error at load. `memory.RenderIndex(facts, density)` now takes the
  density: `full` = `- **Name** — Desc`; `names` = `- Name`; `hybrid` = behavioral
  categories (User/Feedback/Incidents/Projects) render `- Name — <60-rune hook…>`
  (name-only when desc empty), all other categories render name-only. Unrecognized
  density is treated as `hybrid`.

## 2026-09-27 — Asaman closing verification

- Independently verified T1–T6 context topology and race-enabled Go suite; recorded coordinator fresh Codex T7 proof and two isolated metadata edge failures in `AGENTS/specs/2026-09-27-asaman-closing-tests.md`. Documentation only; no implementation or live-context edits.

## 2026-10-06 — Search denoise

- **Why:** `asaman search "sketch store"` returned a dozen sessions whose only match was injected rules text, and missed the session that built it.
- **Injected context is not searchable.** `adapters/denoise.go`: Codex `# AGENTS.md instructions`/developer/system messages, Claude `isMeta` records, skill bodies and `<system-reminder>`/`<INSTRUCTIONS>` spans get role `context` (kept in `events`, excluded from FTS).
- **Provenance columns.** FTS is now `title, text, aux`: prose (user/assistant/archive) in `text`; tool output and compaction summaries in `aux`. Ranked by `bm25` weights 10 / 4 / 0.5, newest first on ties. Claude tool results (which arrive under the user role) are now classed `tool_result`.
- **Query.** Exact phrase, then `NEAR(terms, 40)`, then (only if <5 strong hits) up to 5 terms-anywhere hits flagged `Loose`. Hits with identical matched text collapse into one with a `+N more` count. Default limit 20; `--limit N`.
- **Codex identity bug.** Sessions were keyed on `session_meta.session_id`, which every subagent shares with its root thread, and forked subagent files replay the parent's `session_meta`. 87 of 160 Codex rollouts were overwriting each other. Now keyed on the filename UUID; `parent_thread_id` sets kind `subagent`; `guardian_review` threads (replays of the parent) are not searchable. Titles now read `thread_name` from `session_index.jsonl`.
- **Archive notes** now index their body (previously title only) and take their date from the filename.
- **Cache versioning.** `core.schemaVer` (state key `schema_ver`); bump it whenever the derived layout or indexing rules change and the cache rebuilds itself on next run.
- Output: `date kind agent title` / snippet / source path.
