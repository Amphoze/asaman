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
