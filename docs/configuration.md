# Configuration

Workbench reads these layers, lowest to highest:

1. Built-in defaults.
2. `<PiG agent directory>/Workbench.toml`.
3. `<original session cwd>/<PiG config directory>/Workbench.toml`, only while that project is trusted by PiG.
4. Session-only runtime overrides.

The agent/config directories come from PiG, including its alternate/shared-directory configuration. `cd` never moves either file. There are no environment-variable overrides.

## Example

```toml
version = 1

[read]
max-bytes = 16384
max-lines = 400
```

`version` may be omitted for version 1; saves add it. Unknown versions are refused. Documents use TOML 1.0 syntax and are limited to 1 MiB. Unknown keys are reported, retained and inactive.

Currently, `read.max-bytes` (1024–51200) and `read.max-lines` (5–2000) are live consumers. Each read captures one immutable configuration revision and reports it in Details. The complete result, including framing, must fit the selected budget.

Other scalar settings from the specifications are retained and validated, but catalog entries marked `unavailable` have no wired consumer yet. Saving one does not activate its feature. Complex instruction classes, producer-specific settings and trust-rule editing are not implemented yet.

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

Values use TOML syntax: quote strings, use `true`/`false`, and write arrays as `["one", "two"]`. Arrays replace lower-layer arrays; they do not append. Explicit `false`, `0` and `[]` remain explicit overrides when valid for that key.

Reset deletes the chosen scope's key, revealing the lower-layer value. Runtime changes never touch a file and disappear on session replacement or extension reload.

Inspection shows value, source scope, source file, configuration revision and application status. A save separately reports `saved`, `published` and `pending`; unavailable consumers remain pending rather than being described as applied.

## Project restrictions

PiG's current trust decision controls whether Workbench reads project settings. Workbench checks it again before committing a save; it does not manufacture a trust grant.

Even a trusted project cannot set `trust.rules`, secret settings, session messaging policy or other user/runtime-only settings. Read budgets can only be reduced relative to the user/default layer. Projects may require seen-line enforcement, never disable it.

## Save and recovery guarantees

- Saves re-read both permitted files, validate the whole candidate and serialize writers across sessions/processes.
- Disjoint key changes merge. A changed-key conflict or changed canonical symlink target requires reloading and reconciling; it is never silently overwritten.
- Symlinked settings files remain symlinks: the resolved target receives the update.
- Structural edits retain comments, key ordering, string-value quoting and unknown values; the editor may normalize whitespace and key quoting. A second parser checks the result, including that unrelated settings did not change.
- Files are replaced through a same-directory, private (0600) temporary file, flushed before rename. The parent directory is flushed where supported. Windows compilation is checked; directory flush and live platform behavior are not verified there.
- Lock acquisition is bounded by cancellation and a three-second save budget.
- Malformed files are never replaced with defaults. A failed reload retains the last valid snapshot as visibly stale and blocks reads that depend on it. Fix the file and reload.
- If persistence succeeds but consumer reconciliation fails, the result says saved but not published, and admission stays blocked until recovery. It does not pretend to undo the file write.

Invalid-value diagnostics report the file/key/type and remediation, not the rejected value or its source line.
