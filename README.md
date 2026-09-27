# asaman — Agentic Session Assist MANagement

One deterministic Go tool: a single pane over **context · sessions · memory · feedback** for every agent on the machine (currently Claude + Codex). Successor to `csess`.

## Install
```
CGO_ENABLED=0 go build -ldflags="-s -w" -o asaman .
cp asaman ~/.local/bin/asaman
```
Pure-Go / cgo-free (static binary). Config: `AGENTS/asaman.toml` (or `--config`, `$ASAMAN_CONFIG`).

## Commands
| Command | Purpose |
|---|---|
| `asaman search <q> [--agent claude\|codex]` | unified search across sessions + memory |
| `asaman sessions` | list recent sessions (all agents) |
| `asaman mem <q>` / `feedback <q>` | search memory facts |
| `asaman index [--check]` | regenerate inline index + `MEMORY.md` stub + cache (`--check` = staleness gate) |
| `asaman doctor` | verify invariants (symlinks, size cap, freshness, meta quarantine) |
| `asaman setup --init-project <dir>` | scaffold a new `AGENTS/` (its `.gitignore` ignores `secrets/`) |
| `asaman serve [--port N]` | local "observatory" UI — **loopback only** |
| `asaman import-csess [--from db]` | import csess favourites/tags/notes (exact-equality) |

## Design highlights
- **Deterministic derivations:** sessions + memory indexed into `~/.cache/asaman/index.db` (SQLite/FTS5, pure-Go modernc), rebuilt from source via hash-inventory reconciliation.
- **Durable curation** (favourites/tags/notes) lives in `AGENTS/asaman/meta.jsonl` — append-only, crash-safe (repair-before-append under flock, fsync ack), *not* in the rebuildable cache.
- **Symlink-preserving index write:** `SyncIndex` resolves `CLAUDE.md → AGENTS.md` and writes the real file atomically, never breaking the symlink.
- **Adapters:** `internal/adapters/{claude,codex}.go`; a new agent = one adapter + one config block.
- **UI security:** loopback bind, html/template-escaped transcripts, per-instance CSRF + same-origin on writable endpoints, no remote assets (offline).

## SCOPE — this repo is the tool only
Building asaman performs **no live migration**. Symlinking `~/.claude` memory, rewriting `~/Projects/AGENTS.md`/`CLAUDE.md`, installing the `SessionStart`/pre-commit hooks, and deleting `csess` are a **separate later phase** (see spec §6, `AGENTS/specs/2026-09-27-asaman-context-reorg-design.md`). `import-csess` reads a copy; nothing here mutates the live csess store.
