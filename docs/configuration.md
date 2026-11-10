# Configuration

Workbench reads these layers, lowest to highest:

1. Built-in defaults.
2. `<PiG agent directory>/Workbench.toml`.
3. `<original session cwd>/<PiG config directory>/Workbench.toml`, only while that project is trusted by PiG.
4. Session-only runtime overrides.

The agent/config directories come from PiG, including its alternate/shared-directory configuration. `cd` never moves either file. There are no environment-variable overrides.

Secret detection policy lives in its own `Secrets.toml` beside each `Workbench.toml` (agent directory and, while trusted, the project's config directory), in the nukii schema: `version = 2`, `[env] names`, `[[patterns]]` (`name` plus one of `regex`, `prefix`, `suffix`; `min-length` ≥ 8 for prefix) and `[exclusions]` (`values`, `hashes`, `patterns`, `env-names`). Patterns and exclusions **append** across layers, so a trusted project can add to the user's policy but never drop it; only the user file may set `[env] names`. The file is read on session start and by `/workbench settings reload`; a malformed file withholds model requests (fail closed) until it is repaired, and the prompt you submitted is returned to the editor, never dropped. Vault records the current key cannot decrypt are ignored with a one-time notice rather than blocking requests, and a `secret.key` you own that is readable by others is narrowed to `0600` (also with a one-time notice). It never holds secret values.

Tool-result protection covers content plus retained `details` and `structuredContent` before session persistence. Retained JSON string values and keys are redacted; a guard failure, unsupported shape or redacted-key collision withholds all those fields atomically rather than saving partial plaintext. Native image/audio content blocks retain their existing opaque-binary handling. Presentation update captures are protected before entering Workbench's retained caches.

## Editing settings in the TUI

`/workbench settings` opens a panel listing every setting with its effective value and source. `enter` toggles a boolean, cycles a choice or opens a typed prompt for integers and strings; typing in the list searches. `tab` switches between user, runtime and (when trusted) project scope. Edits stay staged and marked `[unsaved]` until `ctrl+s`; a failed save shows the exact reason and keeps your edits, and `escape` with unsaved edits asks first. `ctrl+r` stages a reset of the selected setting at the current scope. `enter` on an array-valued setting opens a list editor: `enter` changes an item, the last row adds one, `d` removes the selected item and `escape` stages the whole replacement array (arrays are never appended to). Outside the TUI, `/workbench settings` prints the same data as JSON.

## Example

```toml
version = 1

[read]
max-bytes = 16384
max-lines = 400
```

`version` may be omitted for version 1; saves add it. Unknown versions are refused. Documents use TOML 1.0 syntax and are limited to 1 MiB. Unknown keys are reported, retained and inactive.

`read.max-bytes` (1024–51200) and `read.max-lines` (5–2000) bound each complete read result, including framing. Each read captures one immutable configuration revision and reports it in Details. `read.summarize.enabled` and `read.summarize.min-total-lines` (default 200) control summaries for eligible local code reads; `read.summarize.min-body-lines` (default 12) and `read.summarize.min-comment-lines` (default 8) set how long a body or comment block must be to fold, and `read.summarize.prose` (default off) extends summaries to Markdown headings. `read.loop-hint-after` (1–10000, default 3) sets when repeated identical canonical/revision/view/selection reads gain a hint; content is never suppressed.

`trust.rules` (user/runtime only; entries `allow <absolute dir>` or `deny <absolute dir>`, managed with `/workbench trust`; in the TUI a panel where `a` adds and `d` removes a rule; it only governs foreign nested repositories, because a linked `git worktree` of a checkout inside the project, such as `.worktrees/<branch>`, is part of the project: its instruction files apply without a rule and a file identical to an already delivered one is not sent again) and `projects.session-storage` (`link`, the default, or `off`) are live too. With `link`, a session start in a directory that belongs to a recognised project converts PiG's per-directory session bucket into a symlink to `<agent>/projects/<id>/sessions/`, so `/resume` lists every checkout's sessions. The conversion is journaled and resumes after a crash, never moves a file a running session has open, and waits (with a notice) while another live session uses the same directory or the bucket is on another filesystem. `/workbench projects unlink-storage` restores real directories. A custom `--session-dir` is never touched.

`frames.display.default-visibility` (default `collapsed`) decides how context frames appear in the transcript: one row each, `shown` with the body open, or `hidden` without a row. `frames.display.style` (`rail`, `row`, `card`) frames an opened body. `frames.display.sources` overrides the visibility per source: each entry is `shown|collapsed|hidden <source prefix>` (for example `"hidden session.date"`), the longest matching prefix wins, and `/workbench frames show|collapse|hide <source>` edits it. Neither changes what the model receives.

`context.timezone` (default `system`) sets the zone of the `session.date` frame: `system` or an IANA name such as `Europe/Zurich`. An unknown zone delivers `Date is unavailable.`, never a guessed date. In a running session a changed date or zone reaches the model before the next prompt as a short change notice, not as a repeated frame.

`jobs.wake-on-completion` (default `true`) lets a finished background job start a turn on an idle session, and `frames.wake-budget` (default 10 per producer and hour; zero disables it) bounds how often a frame producer registered with `allowWake` may do the same. Both are user/runtime settings.

`models.knowledge-cutoffs` (user/runtime only) overrides the knowledge cutoff the `session.model` frame shows. Entries are `<model-id>=<YYYY-MM[-DD]>`, optionally `provider/<model-id>=…`, for example `["claude-sonnet-5-5=2026-06"]`. Without an entry Workbench uses a cutoff a registered extension contributed, then a bundled models.dev snapshot, then an estimate of the model's earliest listed release date minus one year (shown with basis `estimated` and its release date as the source), and shows no cutoff when the model has no release date either. The frame always adds a short note that the model's training data is stale and should be verified. A malformed entry is named in the frame and ignored.

`fetch.disabled-backends` (user/runtime; entries `owner/key` as shown by `/workbench integrations`) turns contributed fetch backends off for `read https://`; the builtin reader then serves those URLs and the producer's own tools keep working.

`instructions.system-dir` (default `/etc/agents`; user/runtime; empty disables) names the host-level instruction directory. `AGENTS.md` there is steering for every session on the machine, delivered before the user's and the project's `AGENTS.md`; `HOST.md` carries static facts about the host (setup, tooling, paths, quirks) and is always delivered as context, never as an instruction and never withdrawn. Both are optional, apply to untrusted projects too, share the usual caps (`HOST.md` 400 lines/32 KiB, `AGENTS.md` 200 lines/16 KiB) and arrive again as a changed view when edited. A file or directory writable by any user (world-writable; group-writable is accepted), or owned by neither root nor you, is refused with a visible notice. Remote hosts (SSH, planned) will supply their own copies that replace the local ones per file name; that needs the SSH backend and is not available yet. `/workbench instructions` lists what was found.

`instructions.boundary` (default `project`; user/runtime) decides how far up instruction discovery looks. `project` stops at the project root. `filesystem` also applies `AGENTS.md` and `BACKLOG.md` files in the directories above the project, up to the filesystem root, for trusted projects.

`instructions.updates.self-edits` (default `notice`; user/project/runtime) decides how a change the agent made itself, through `edit` or `write`, to an instruction file it already holds comes back: `notice` sends one line, `diff` the same diff an outside change produces, `silent` nothing. If the bytes on disk differ from what the agent wrote, the change always arrives as a diff. The agent's own edit does not block its next mutation.

`skills.listing` (`prompt`, the default, or `frame`) chooses where Workbench's own system prompt lists skills: with `frame` the `<skills>` section is left out and the budgeted `skill://` listing rides the `session.capabilities` frame instead, so a skill change does not rewrite the prompt. It matters only while `prompt.delivery` takes the prompt over.

The catalog identifies the other live consumers, including edit policy, instruction delivery, the default job timeout, ask dialog timeout, session messaging policy, secret protection, prompt delivery, the skills command budget and activity placement. Inspect each description for its scope and timing: for example, activity placement and session endpoint enablement are captured at session start, so publishing a new snapshot does not immediately re-register them. Prompt takeover also requires `instructions.enabled`. `prompt.delivery = "auto"` (the default) resolves to `force` on stock PiG, so `builtin/workbench` replaces PiG's system prompt; `prompt.enabled = false` keeps PiG's. `sections` needs the fork host and leaves PiG's prompt in place on stock PiG.

`prompt.template` (default `builtin/workbench`) names the entry template of the system prompt. Any other name is a file, such as `system.md`, in `system-prompts/` under the agent directory or, while the host trusts the project, under the project's PiG configuration directory; the project file wins. A template can build on the builtin one with `{% include "builtin/workbench" %}` and read `model.provider`, `model.id`, `model.name`, `model.reasoning`, `model.context_window` and `model.max_output`. A template that fails to render is reported once with its name and cause; the last good render stays in use, or PiG's prompt when there is none. `session://self/system-prompt` names the template that produced the served prompt.

Settings marked `unavailable` are retained and validated but do not control their intended runtime feature. In particular, `activity.shortcut` is read from the user layer at load because PiG registers shortcuts before a session's settings exist: set it in `Workbench.toml` and `/reload`; an invalid key keeps `alt+w` and `/workbench show` says so. Complex instruction classes and producer-specific settings are not implemented.

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
