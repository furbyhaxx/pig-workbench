# pig-workbench

A [PiG](https://github.com/MichaelKinsy/PiG) extension that upgrades the builtin coding tools: hashline editing, selector-driven reads, `scheme://` resources, local and SSH working locations, harness frames with instruction and backlog injection, questionnaires and background jobs.

**Status: Slice 1 implementation in progress, not feature-complete.** Local selector-driven `read`, revisioned `cd`, session shutdown draining and the `location` extension service are wired. Layered TOML configuration and safe saves are also wired. Native questionnaire dialogs and encrypted secret-answer storage are wired. Full hashline editing, jobs tools, the general secret boundary, prompt takeover, rich questionnaire panels and other Slice 1 requirements remain incomplete. Scope and acceptance gates are tracked in [the slice plan](.project/plans/0002-slice-1-foundation.md).

## Available now

- `read`: exactly one of `path` or `paths` (1–32 files). Select with `:N`, `:A-B`, `:A+C`, `:-N`, comma ranges or `:raw`. Literal filenames take precedence. Local source reads carry hashline tags and epoch-bound delivered visibility; raw views preserve bytes. The resource router also exposes local scratch, configuration, session and artifact trees. One 50 KiB/2000-line batch budget; Details retain actual coverage, exact-byte revision and continuation.
- `write`: create or fully replace one local file with optional `expectedRevision` (a stale revision is refused, never overwritten), atomic replace and preserved mode. No directory creation, no non-local schemes yet.
- `ls`: list one directory through the resource router (`path`, `file://`, `local://`, `out://`) with typed entries, directories first, a continuation cursor and honest completeness.
- `session://` resources (read-only): `session://self/meta`, `title`, `transcript`, `entries`, each pageable with the same selectors. `cfg://` exposes effective settings with provenance (`cfg://workbench/status`, `cfg://<group>/<key>`).
- `grep`: search file contents under the location with regex or literal OR-patterns, glob/exclusion filters and honest `complete` reporting.
- `find`: walk for file-name globs; symlinked directories are not followed.
- `bash` and `jobs`: local foreground/background execution, delayed promotion, deadlines, output cursors, stdin/EOF and process-group cancellation. Linux requires identity-bound pidfd group signalling; crash-watchdog, scheme shell commands and persistent job resources remain pending.
- `ask_user`: native single/multi/preview/freeform/secret dialogs with notes and custom answers. Cancellation discards partial answers; headless modes report unavailable. Secret answers are encrypted in the vault and returned as placeholders. The rich TUI panel and replay command are still pending.
- `cd`: `{}` reports status, `{"path":"…"}` changes the Workbench location, `{"reset":true}` returns to the origin. It never changes PiG’s cwd. Until the remaining overrides land, other builtin tools still use PiG’s cwd.
- `/workbench show`, `/workbench cd`, `/cd`, `/wb:cd`: inspect or change that location. `/tools [name]` and `/workbench tools [name]` inspect the live host registry.
- `/workbench settings show|catalog|set|reset|reload`: inspect scoped values and change user/project/runtime settings. Read byte/line ceilings apply live; other catalog entries are explicitly marked unavailable. See [configuration](docs/configuration.md) for paths, syntax, trust restrictions and save guarantees, and [tools](docs/tools.md) for every tool contract, selector and result fact.
- Extension consumers can discover and call protocol v1 through `api.Client`; see [extension services](docs/extension-services.md). Discovery advertises only the wired `location` capability; other service capabilities are not available yet.

## Develop

Requires Go and a `pig` binary on `PATH`; the SDK is the staged copy that matches the running `pig`.

```sh
just check            # gofmt, vet, tests and PiG validation
just race             # concurrent domain checks
just integration      # real PiG + localhost scripted provider; no billed inference
pig -e .              # load into a session; /reload after edits
```
