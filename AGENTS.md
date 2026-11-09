# pig-workbench

Go factory extension for PiG: enhanced builtin tools, working locations, harness frames, questionnaires and shell jobs. Exclude subagents, agent definitions and task/todo tracking.

## Layout
- `extensions/pig-workbench/extension.go` — `Extension()` factory, package `workbench`, its own module (requires the root module via `go.work`). Registration and wiring only; feature logic goes in `internal/<feature>/`.
- `package.json` + `go.work` — PiG Package manifest (`pi.extensions`), so `pig install git:https://github.com/furbyhaxx/pig-workbench` works. Do not move the factory back to the root.
- `internal/` — feature packages; each gets its own AGENTS.md when created. `internal/host/` owns SDK adapters and composition.
- `docs/` — human-facing product documentation; settings changes cross-update `docs/configuration.md`.
- `api/` — public v1 protocol types; keep wire changes aligned with `.project/specs/0001-extension-services.md` and consumer fixtures.
- `.project/` — development artifacts; never in `docs/`; committed. Load skill `dot-project` before writing one. Every Markdown artifact needs `document:` frontmatter (`.project/AGENTS.md`). Open work: `BACKLOG.md` pointers + `.project/backlog/`; live claims: project-scope `tasks`. Never put drift/TODOs in AGENTS.md.
- `.tmp/` — generated modfile for direct `go` commands; git-ignored.

## Commands
- Gate: `just check` (gofmt, vet, test, PiG build of `./extensions/pig-workbench`, `pig package validate .`). Concurrency changes: `just race`.
- Don't run bare `go build`/`go test`; the SDK resolves only through the staged copy. Use the Justfile recipes or `-modfile=.tmp/go.mod` after `just _modfile`.
- Load live: `pig -e ./extensions/pig-workbench`; isolated: `pig --no-session -ne -e ./extensions/pig-workbench`. Install: `pig install git:https://github.com/furbyhaxx/pig-workbench`.

## Identity
- Extension name `pig-workbench` = directory `extensions/pig-workbench` = `Name` in its `extension.go` = package.json member; registration fails on mismatch.
- Command family is `/workbench`, never `/pig-workbench`. Companion top-level commands (`/tools`, `/remote`, `/cd`, `/answer`, `/tldr`, …) call the same handlers as their `/workbench` subcommand.
- Tools keep builtin names (`read`, `edit`, `write`, `grep`, `find`, `ls`, `bash`) plus `cd`, `jobs`, `ask_user`. One `jobs` tool; never `jobs_*` siblings.

## Rules
- Load skill `extending-pig` for extension work. Verify SDK signatures against `pig reload --sdk-path`; do not copy TypeScript runtime assumptions.
- Scope, milestones and acceptance gates: `.project/plans/0001-pig-workbench.md`. UI layouts: `.project/sketches/workbench-ui/README.md`; new surfaces or layout changes need user review and captures.
- Proposals in `.project/drafts/` are not approved implementation contracts.
- Never stage or commit a package while its delegated subagent is still running: a green test at check time can be mid-edit a minute later. Stage explicit paths you own, and commit a delegated package only after its agent reports.
- Subagents share this repository's git index. Never use a bare `git commit`; always `git commit -- <explicit paths>` (or `git commit --only`) so an agent's staged files cannot ride along in your commit.
- Live integration tests (`just integration`) need a quiet tree: PiG packs the whole module, so a subagent writing any file mid-build fails the run with "the inputs of cell … changed while it was being built". Run them when no agent is writing, or treat that message as an environment race, not a defect.
- Runtime dependencies have standing user approval after checking the latest available version and whether the project is maintained or superseded. Prefer its maintained replacement when applicable; record licenses in `THIRD_PARTY_NOTICES.md` and the check in the producing plan.
- Code adapted from donors (pi-workbench, oh-my-pi, nugent) → keep its license notice and add an entry to `THIRD_PARTY_NOTICES.md` in the same change.
- New tool, command, setting or scheme → update `README.md` in the same change.

## Donor sources (read-only)
| Path | Use for |
|---|---|
| `host:/mnt/Projects/furbyhaxx/pi-workbench` | locations, jobs, frames, instructions, ask, tldr, renderers, commands |
| `host:/mnt/Projects/nukular/nukular-ai/.references/oh-my-pi` | hashline (`crates/pi-edit/`), read pipeline, `internal-urls/` schemes |
| `host:/mnt/Projects/nukular/nukular-ai/crates/` | nugent scheme router, selectors, pager, instruction ledger |

Don't edit donor trees; they carry unrelated local changes. Port behavior and tests, not TypeScript structure.
