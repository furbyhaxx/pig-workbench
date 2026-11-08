# Configuration

Workbench reads these layers, lowest to highest:

1. Built-in defaults.
2. `<PiG agent directory>/Workbench.toml`.
3. `<original session cwd>/<PiG config directory>/Workbench.toml`, only while that project is trusted by PiG.
4. Session-only runtime overrides.

The agent/config directories come from PiG, including its alternate/shared-directory configuration. `cd` never moves either file. There are no environment-variable overrides.

Secret detection policy lives in its own `Secrets.toml` beside each `Workbench.toml` (agent directory and, while trusted, the project's config directory), in the nukii schema: `version = 2`, `[env] names`, `[[patterns]]` (`name` plus one of `regex`, `prefix`, `suffix`; `min-length` ≥ 8 for prefix) and `[exclusions]` (`values`, `hashes`, `patterns`, `env-names`). Patterns and exclusions **append** across layers, so a trusted project can add to the user's policy but never drop it; only the user file may set `[env] names`. The file is read on session start and by `/workbench settings reload`; a malformed file withholds model requests (fail closed) until it is repaired. It never holds secret values.

Tool-result protection covers content plus retained `details` and `structuredContent` before session persistence. Retained JSON string values and keys are redacted; a guard failure, unsupported shape or redacted-key collision withholds all those fields atomically rather than saving partial plaintext. Native image/audio content blocks retain their existing opaque-binary handling. Presentation update captures are protected before entering Workbench's retained caches.

## Editing settings in the TUI

`/workbench settings` opens a panel listing every setting with its effective value and source. `enter` toggles a boolean, cycles a choice or opens a typed prompt for integers and strings; typing in the list searches. `tab` switches between user, runtime and (when trusted) project scope. Edits stay staged and marked `[unsaved]` until `ctrl+s`; a failed save shows the exact reason and keeps your edits, and `escape` with unsaved edits asks first. `ctrl+r` stages a reset of the selected setting at the current scope. Array-valued settings are not editable in the panel; use `/workbench settings set`. Outside the TUI, `/workbench settings` prints the same data as JSON.

## Example

```toml
version = 1

[read]
max-bytes = 16384
max-lines = 400
```

`version` may be omitted for version 1; saves add it. Unknown versions are refused. Documents use TOML 1.0 syntax and are limited to 1 MiB. Unknown keys are reported, retained and inactive.

`read.max-bytes` (1024–51200) and `read.max-lines` (5–2000) bound each complete read result, including framing. Each read captures one immutable configuration revision and reports it in Details. `read.summarize.enabled` and `read.summarize.min-total-lines` control declaration summaries for eligible local code reads. `read.loop-hint-after` (1–10000, default 3) sets when repeated identical canonical/revision/view/selection reads gain a hint; content is never suppressed.

`trust.rules` (user/runtime only; entries `allow <absolute dir>` or `deny <absolute dir>`, managed with `/workbench trust`) and `projects.session-storage` (`link`, the default, or `off`) are live too. With `link`, a session start in a directory that belongs to a recognised project converts PiG's per-directory session bucket into a symlink to `<agent>/projects/<id>/sessions/`, so `/resume` lists every checkout's sessions. The conversion is journaled and resumes after a crash, never moves a file a running session has open, and waits (with a notice) while another live session uses the same directory or the bucket is on another filesystem. `/workbench projects unlink-storage` restores real directories. A custom `--session-dir` is never touched.

The catalog identifies the other live consumers, including edit policy, instruction delivery, the default job timeout, ask dialog timeout, session messaging policy, secret protection, prompt delivery, the skills command budget and activity placement. Inspect each description for its scope and timing: for example, activity placement and session endpoint enablement are captured at session start, so publishing a new snapshot does not immediately re-register them. Prompt takeover currently also requires `instructions.enabled`; `prompt.delivery = "auto"` retains PiG's prompt: the live A/B showed no regression for `force` but is too small to change the default.

Settings marked `unavailable` are retained and validated but do not control their intended runtime feature. In particular, `prompt.template` is displayed but rendering always uses `builtin/workbench`, and `activity.shortcut` is not applied: `alt+w` is registered statically before settings load. Complex instruction classes and producer-specific settings are not implemented.

## Presentation display namespaces

Presentation descriptors register typed values under `tools.<owner>.<stable-key>.<field>`; TOML quoting preserves dots in owner/key names as literal segments. Producer defaults sit below user, trusted-project and runtime overrides. Withdrawal keeps overrides and marks the schema unavailable; re-registration reuses the stable identity. Descriptor type changes must use a new key or an explicit migration.

`enabled`, `density`, `preview-lines`, `metrics` and `target-shortening` report `live` and affect the shared renderer; `target-shortening = false` turns off fish abbreviation of parent folders (the shortest relative/absolute form and elision still apply). Compact density is header-only; an explicit zero preview is retained, and `metrics = []` hides metrics. Changing these values never changes tool execution. If rendering was already cached, disabling shows a bounded raw fallback and reports `appliesAfterReload` for delegation to the producer/host; before first resolution it delegates directly.

`child-density` and producer-specific controls still report `registered` (the panel says "saved, but nothing reads it yet"): Workbench draws no child rows, and producer-declared controls need the producer to read them. Do not infer applied rendering from a successful save of a registered-only control.

```text
/workbench settings tools
/workbench settings tools reference-producer web_search
/workbench settings tools set runtime reference-producer web_search density "compact"
/workbench settings tools reset runtime reference-producer web_search density
```

Owner/key identifiers may be quoted; the TOML value tail is parsed separately. Inspection includes live and dormant schemas with effective value/source. TUI uses a compact plain view; RPC uses structured notification data. Saves and reset reuse the normal conflict-safe/scoped configuration path and recheck project trust; no confirmation dialog is added. Registration-bound `settings.changed` events contain display values only.

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
