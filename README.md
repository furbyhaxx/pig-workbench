# pig-workbench

A [PiG](https://github.com/MichaelKinsy/PiG) extension that upgrades the builtin coding tools: hashline editing, selector-driven reads, `scheme://` resources, local and SSH working locations, harness frames with instruction and backlog injection, questionnaires and background jobs.

**Status: Slice 1 implementation in progress, not feature-complete.** Local selector-driven `read`, revisioned `cd`, session shutdown draining and the `location` extension service are wired. Hashline editing, other resource providers, configuration, jobs, secrets, prompts, questionnaires and the remaining Slice 1 features are not implemented yet. Scope and acceptance gates are tracked in [the slice plan](.project/plans/0002-slice-1-foundation.md).

## Available now

- `read`: exactly one of `path` or `paths` (1–32 files). Select with `:N`, `:A-B`, `:A+C`, `:-N`, comma ranges or `:raw`. Literal filenames take precedence. Local files and `file://` only; no usable hashline tags yet. One 50 KiB/2000-line batch budget; Details retain actual coverage, exact-byte revision and continuation.
- `cd`: `{}` reports status, `{"path":"…"}` changes the Workbench location, `{"reset":true}` returns to the origin. It never changes PiG’s cwd. Until the remaining overrides land, other builtin tools still use PiG’s cwd.
- `/workbench show`, `/workbench cd`, `/cd`, `/wb:cd`: inspect or change that location. `/tools [name]` and `/workbench tools [name]` inspect the live host registry.
- Extension consumers can discover protocol v1 on `workbench:v1:request` and call `location.get`. Discovery advertises only the wired `location` capability; other service capabilities are not available yet.

## Develop

Requires Go and a `pig` binary on `PATH`; the SDK is the staged copy that matches the running `pig`.

```sh
just check            # gofmt, vet, tests and PiG validation
just race             # concurrent domain checks
just integration      # real PiG + localhost scripted provider; no billed inference
pig -e .              # load into a session; /reload after edits
```
