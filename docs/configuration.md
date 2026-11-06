# Configuration

Workbench reads these layers, lowest to highest:

1. Built-in defaults.
2. `<PiG agent directory>/Workbench.toml`.
3. `<original session cwd>/<PiG config directory>/Workbench.toml`, only while that project is trusted by PiG.
4. Session-only runtime overrides.

The agent/config directories come from PiG, including its alternate/shared-directory configuration. `cd` never moves either file. There are no environment-variable overrides.

Secret detection policy lives in its own `Secrets.toml` beside each `Workbench.toml` (agent directory and, while trusted, the project's config directory), in the nukii schema: `version = 2`, `[env] names`, `[[patterns]]` (`name` plus one of `regex`, `prefix`, `suffix`; `min-length` ≥ 8 for prefix) and `[exclusions]` (`values`, `hashes`, `patterns`, `env-names`). Patterns and exclusions **append** across layers, so a trusted project can add to the user's policy but never drop it; only the user file may set `[env] names`. The file is read on session start and by `/workbench settings reload`; a malformed file withholds model requests (fail closed) until it is repaired. It never holds secret values.

## Example

```toml
version = 1

[read]
max-bytes = 16384
max-lines = 400
```

`version` may be omitted for version 1; saves add it. Unknown versions are refused. Documents use TOML 1.0 syntax and are limited to 1 MiB. Unknown keys are reported, retained and inactive.

`read.max-bytes` (1024–51200) and `read.max-lines` (5–2000) bound each complete read result, including framing. Each read captures one immutable configuration revision and reports it in Details. `read.summarize.enabled` and `read.summarize.min-total-lines` control declaration summaries for eligible local code reads.

`trust.rules` (user/runtime only; entries `allow <absolute dir>` or `deny <absolute dir>`, managed with `/workbench trust`) and `projects.session-storage` (`link`, the default, or `off`) are live too. With `link`, a session start in a directory that belongs to a recognised project converts PiG's per-directory session bucket into a symlink to `<agent>/projects/<id>/sessions/`, so `/resume` lists every checkout's sessions. The conversion is journaled and resumes after a crash, never moves a file a running session has open, and waits (with a notice) while another live session uses the same directory or the bucket is on another filesystem. `/workbench projects unlink-storage` restores real directories. A custom `--session-dir` is never touched.

The catalog identifies the other live consumers, including edit policy, instruction delivery, the default job timeout, ask dialog timeout, session messaging policy, secret protection, prompt delivery, the skills command budget and activity placement. Inspect each description for its scope and timing: for example, activity placement and session endpoint enablement are captured at session start, so publishing a new snapshot does not immediately re-register them. Prompt takeover currently also requires `instructions.enabled`; `prompt.delivery = "auto"` retains PiG's prompt pending user-authorized A/B evaluation.

Settings marked `unavailable` are retained and validated but do not control their intended runtime feature. In particular, `prompt.template` is displayed but rendering always uses `builtin/workbench`, and `activity.shortcut` is not applied: `alt+w` is registered statically before settings load. Complex instruction classes and producer-specific settings are not implemented.

## Inspect and change

```text
/workbench settings show read.max-lines
/workbench settings catalog read.
/workbench settings set user read.max-lines 800
/workbench settings set project read.max-lines 400
/workbench settings set runtime read.max-lines 200
/workbench settings reset runtime read.max-lines
/workbench settings reload
```

Values use TOML syntax: quote strings, use `true`/`false`, and write arrays as `["one", "two"]`. Arrays replace lower-layer arrays, except `Secrets.toml` detection policy, which appends; they do not append otherwise. This command writes `Workbench.toml` only; edit `Secrets.toml` directly for detection policy. Explicit `false`, `0` and `[]` remain explicit overrides when valid for that key.

Reset deletes the chosen scope's key, revealing the lower-layer value. Runtime changes never touch a file and disappear on session replacement or extension reload.

Inspection shows value, source scope, source file, configuration revision and application status. The same data is readable through `cfg://`: `cfg://workbench/status` for readiness and the effective read policy, `cfg://<group>/<key>` for one value with provenance, and `cfg://` for every entry. A save separately reports `saved`, `published` and `pending`; unavailable consumers remain pending rather than being described as applied.

## Project restrictions

PiG's current trust decision controls whether Workbench reads project settings. Workbench checks it again before committing a save; it does not manufacture a trust grant.

Even a trusted project cannot set `trust.rules`, `secrets.redact-owners`, session messaging policy or other user/runtime-only settings. Project detection policy is limited to adding patterns and exclusions in `Secrets.toml`. Read budgets can only be reduced relative to the user/default layer. Projects may require seen-line enforcement, never disable it.

## Save and recovery guarantees

- Saves re-read every permitted file (both `Workbench.toml` layers and both `Secrets.toml` layers), validate the whole candidate and serialize writers across sessions/processes.
- Disjoint key changes merge. A changed-key conflict or changed canonical symlink target requires reloading and reconciling; it is never silently overwritten.
- Symlinked settings files remain symlinks: the resolved target receives the update.
- Structural edits retain comments, key ordering, string-value quoting and unknown values; the editor may normalize whitespace and key quoting. A second parser checks the result, including that unrelated settings did not change.
- Files are replaced through a same-directory, private (0600) temporary file, flushed before rename. The parent directory is flushed where supported. Windows compilation is checked; directory flush and live platform behavior are not verified there.
- Lock acquisition is bounded by cancellation and a three-second save budget.
- Malformed files are never replaced with defaults. A failed reload retains the last valid snapshot as visibly stale and blocks reads that depend on it. Fix the file and reload.
- If persistence succeeds but consumer reconciliation fails, the result says saved but not published, and admission stays blocked until recovery. It does not pretend to undo the file write.

Invalid-value diagnostics report the file/key/type and remediation, not the rejected value or its source line.
