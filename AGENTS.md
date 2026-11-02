# pig-workbench

Go factory extension for PiG: enhanced builtin tools, working locations, harness frames, questionnaires and shell jobs. Exclude subagents, agent definitions and task/todo tracking.

## Layout
- `extension.go` — `Extension()` factory, package `workbench`. Registration and wiring only; feature logic goes in `internal/<feature>/`.
- `internal/` — feature packages; each gets its own AGENTS.md when created.
- `.project/` — development artifacts; never in `docs/`; committed. Load skill `dot-project` before writing one. Every Markdown artifact needs `document:` frontmatter (`.project/AGENTS.md`). Open work: `BACKLOG.md` pointers + `.project/backlog/`; live claims: project-scope `tasks`. Never put drift/TODOs in AGENTS.md.
- `.tmp/` — generated modfile for direct `go` commands; git-ignored.

## Commands
- Gate: `just check` (gofmt, vet, test, `pig install . --validate-only --json`). Concurrency changes: `just race`.
- Don't run bare `go build`/`go test`; the SDK resolves only through the staged copy. Use the Justfile recipes or `-modfile=.tmp/go.mod` after `just _modfile`.
- Load live: `pig -e .`; isolated: `pig --no-session -ne -e .`.

## Identity
- Extension name `pig-workbench` = directory name = `Name` in `extension.go`; registration fails on mismatch.
- Command family is `/workbench`, never `/pig-workbench`. Companion top-level commands (`/tools`, `/remote`, `/cd`, `/answer`, `/tldr`, …) call the same handlers as their `/workbench` subcommand.
- Tools keep builtin names (`read`, `edit`, `write`, `grep`, `find`, `ls`, `bash`) plus `cd`, `jobs`, `ask_user`. One `jobs` tool; never `jobs_*` siblings.

## Rules
- Load skill `extending-pig` for extension work. Verify SDK signatures against `pig reload --sdk-path`; do not copy TypeScript runtime assumptions.
- Scope, milestones and acceptance gates: `.project/plans/0001-pig-workbench.md`. UI layouts: `.project/sketches/workbench-ui/README.md`; new surfaces or layout changes need user review and captures.
- Proposals in `.project/drafts/` are not approved implementation contracts.
- New runtime dependency → user approval first (plan decisions D1/D2), then record its license in `THIRD_PARTY_NOTICES.md` (create on first entry).
- Code adapted from donors (pi-workbench, oh-my-pi, nugent) → keep its license notice and add an entry to `THIRD_PARTY_NOTICES.md` in the same change.
- New tool, command, setting or scheme → update `README.md` in the same change.

## Donor sources (read-only)
| Path | Use for |
|---|---|
| `host:/mnt/Projects/furbyhaxx/pi-workbench` | locations, jobs, frames, instructions, ask, tldr, renderers, commands |
| `host:/mnt/Projects/nukular/nukular-ai/.references/oh-my-pi` | hashline (`crates/pi-edit/`), read pipeline, `internal-urls/` schemes |
| `host:/mnt/Projects/nukular/nukular-ai/crates/` | nugent scheme router, selectors, pager, instruction ledger |

Don't edit donor trees; they carry unrelated local changes. Port behavior and tests, not TypeScript structure.
