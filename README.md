# pig-workbench

A [PiG](https://github.com/MichaelKinsy/PiG) extension for coding tools, resource URIs, working locations, instruction frames, secret redaction, questionnaires and supervised shell jobs.

**Status:** Slice 1 is in progress and is not frozen. The extension runs on Linux, but some Slice 1 behaviors and acceptance checks remain open; see [current limitations](#current-limitations).

## Get started

Install it into PiG as a Package:

```sh
pig install git:https://github.com/furbyhaxx/pig-workbench
```

To develop, install Go, [PiG](https://github.com/MichaelKinsy/PiG) and `just`, then from this repository:

```sh
just check   # format check, vet, tests and PiG validation
pig -e ./extensions/pig-workbench   # load the extension in a session
```

In the TUI, `/workbench` opens a menu of every Workbench view; typing narrows it, Enter opens one and Esc returns to the menu. `/workbench settings` opens an editable settings panel. In RPC and print mode the same commands print plain text. Use `/workbench show` for a text overview, `/tools` to inspect tool schemas and `/workbench settings catalog` to see which settings are applied. See [tools](docs/tools.md), [configuration](docs/configuration.md), [projects](docs/projects.md) (with a fully commented `Project.toml`) and [extension services](docs/extension-services.md) for contracts and examples.

## What works

- **Read and edit:** `read` supports bounded multi-read, selectors, same-revision repetition hints controlled by `read.loop-hint-after`, safe context-only supersession and registered `scheme://` providers with budgeted guide links; local source snapshots supply hashline tags and seen-line guards. `edit` supports tagged batches, structural blocks, registers and exact-text replacement. `write` atomically creates or replaces local files with optional revision checks and writes the session scratch at `local://`; it also dispatches supported `job://`, `skill://`, `agent://` and `session://` control writes. `ls`, `grep` and `find` support local paths and bounded provider-directory traversal, or call the provider's own `search` when it declares one (its ranking is kept; an `unsupported` answer falls back to the walk and Details report `via`); `ls` exposes hidden-entry control and resumable bounded output; local searches honor nested `.gitignore` rules, and globs support brace alternatives, character classes and recursive `**` components.
- **Resources:** Builtin trees include `file://`, `http(s)://`, `local://`, `artifact://`, `cfg://`, `session://`, `job://`, `skill://`, `agent://` and registered `docs://` resources. HTTP can use a contributed fetch backend (switch one off with `fetch.disabled-backends`) or the bounded builtin static reader. Other extensions can declare revision-checked writes, mkdir, remove, copy and move with outcome receipts; Workbench adds no permission prompts.
- **Locations and jobs:** `cd`, `/cd` and `/wb:cd` change Workbench's captured local location without changing PiG's cwd. `bash` and the single `jobs` tool supervise foreground and background processes, stdin, output and cancellation; a background or promoted job that finishes wakes an idle model with one notice (`jobs.wake-on-completion`); `/workbench jobs` and `/wb:jobs` expose the same jobs; `/workbench codemode` lists observed nested codemode calls. Session-storage links and `/workbench projects unlink-storage` are documented in [configuration](docs/configuration.md).
- **Instructions and frames:** Applicable steering, backlog and trusted-project private notes arrive as attributed frames. A host's `/etc/agents/AGENTS.md` (steering one level above the user's `AGENTS.md`) and `/etc/agents/HOST.md` (always-on facts about the machine) are delivered first when they exist; world-writable files are refused (`instructions.system-dir`, see [configuration](docs/configuration.md)). `/workbench instructions` manages inspection and permitted pins (in the TUI an interactive panel: `p` pins a file, `u` unpins, `v` shows the class rules); `/workbench frames [ls [id|#sequence]]` lists this session's frames or shows one; `/workbench frames show|collapse|hide [source]` sets how frames (all, or one source such as `session.date`) appear in the transcript, never what the model receives. The `session.model` frame states the model's knowledge cutoff when known (`models.knowledge-cutoffs`, a contributed value or a bundled models.dev snapshot) and why remembered specifics need verifying. Every frame of a session, its forks and branches carries one nonce. Producers can publish with `wake: true` to start a turn on an idle session (bounded by `frames.wake-budget`). Truncated instruction views block applicable mutations until the full current file is read, including all tagged edit sources and move destinations; files over 4 MiB must be reduced. Changes are reconciled at prompt or tool boundaries, not by native file watchers.
- **Questions and secrets:** `ask_user` uses native interactive dialogs; headless use reports `interactionUnavailable`. Unanswered forms can be inspected, resumed or discarded through `/workbench ask`. The secrets boundary redacts known values from model-bound content and retained result metadata, and supports encrypted secret answers and controlled substitution.
- **Sessions:** `agent://` and `/workbench sessions` provide authenticated same-user session discovery and messaging, including idle wake and busy steering. `/workbench sessions` can also trust, block, hide or show peers; in the TUI it is a panel where `m` messages the selected peer, `t` and `b` toggle trust and block and `h` hides or shows this session. Session trees and titles are accessible through `session://`.
- **Inspection:** In the TUI these open scrollable panels and the settings panel stages edits per scope (`enter` changes, `ctrl+s` saves, `ctrl+r` stages a reset, `tab` switches scope, typing searches). `/workbench settings`, `prompt`, `skills`, `integrations`, `resources`, `trust`, `projects` and `show` expose their respective state; `/workbench projects` also creates a project (and its `Project.toml`) or joins a registered one, and `list`, `create`, `join` and `example` do the same as plain commands. The activity line displays live counts and contributed items; fixed `alt+w` opens its inspection shortcut.
- **Tool display:** All ten own tools share one transcript appearance with contributed presentations: a call row `label · target · fact` and a result row `label · status · metrics` (status running, succeeded, failed, or for `ask_user` cancelled, timedOut, answered N), then a bounded body (`bash` shows its tail, `read` shows none until expanded or failed). It is derived from the call arguments and result text, so tool Details are unchanged and print/RPC output is not affected. It is a unified plain style, not the final chrome: it has no display settings yet, and the designed layouts remain PLAN-0004. Captures at 36, 57, 80 and 140 columns: `.project/analyses/2026-11-05-slice-1-inventory/evidence/own-tool-rows-tui/`.
- **Extensions:** Protocol v1 exposes location, presentation, resources, fetch, status, docs, frames and snapshots; sessions are advertised when an endpoint is available, and `secrets.redact` only to explicitly granted owners. See [extension services](docs/extension-services.md).

## Current limitations

**Unfinished Slice 1:**

- Typed presentation preferences persist by owner/stable key and survive withdrawal. `/workbench settings tools` groups and edits them; enabled/density/preview/metrics controls are live. Target/nested/custom controls and the new plain view's layout review remain open.

- Long grep matching lines use bounded windows; oversized context is reduced and marked incomplete. Searches still stop at their output and source limits.
- Scheme paths in `bash` and `!` commands: a standalone `ls`, `cat`, `rm`, `mkdir`, `mv`, `cp` or `word > path` on a non-locatable scheme runs in-process, with the same confirmation rules as the tools. In any other command a readable scheme path is passed as a read-only copy (`cat job://<id>/log | wc -l`); writes there are refused. Commands that only mention a scheme, and all `http(s)` URLs, run unchanged. Paths the shell builds at run time are not recognized. `bash.route-to-tools = false` switches this off. Cross-provider transfers and recursive operations are refused; contributed hashline editing remains unavailable.
- `prompt.delivery = "auto"` (the default) resolves to `force` on stock PiG: `builtin/workbench` replaces PiG's system prompt while instructions are enabled and rendering succeeds. Set `prompt.enabled = false` to keep PiG's prompt. A live deepseek-flash A/B (16 tasks, 5 repetitions per arm) passed 79/80 under each prompt, with 0 tool errors against 4 and 23% fewer input tokens for `builtin/workbench`; one model, so model-family adapters remain Slice 3. Evidence: `.project/analyses/2026-11-05-slice-1-inventory/evidence/live-eval/ab-round3/`. The `prompt.template` setting is not applied.
- All fourteen specifications are approved, but implementation and acceptance remain incomplete. The [acceptance inventory](.project/analyses/2026-11-05-slice-1-inventory/README.md) distinguishes observed tests from incomplete rows.
- Windows is not a supported platform yet (SPEC-0008 release matrix). The whole module and its integration tests cross-compile and vet (`just windows`); under wine the `api`, `ask`, `contribution`, `frames`, `hashline`, `htmlread`, `jobs`, `lifecycle`, `location`, `pathdisplay`, `presentation`, `project`, `prompt` and `skills` tests pass. Windows paths in `file://`, `resource`, `search` and `fileedit`, the endpoint and project registries and the secret key/vault privacy checks (they need ACL validation) still fail or fail closed there, so messaging, secrets and settings do not work on Windows. Stored-session title confirmation has been tested through PiG's RPC dialog path, not visually in a TUI. Real session-reply hop limiting has been exercised with a local scripted provider.

**Deferred depth:**

- Broadcast, coordinator and by-project agent/session trees belong to Slice 2. The rich questionnaire panel is later UI work.
- Activity-item click needs a fork host change. `activity.shortcut` (default `alt+w`) is read from the user layer when the extension loads, so a change applies after `/reload`; runtime and project values are not consulted. `/workbench show` names the active key.
- Workbench cannot be fused into a Piglet Binary (`build.extensionRealization: fused`): PiG's builder merges `go.mod` without the member's indirect requirements and stops with `go: updates to go.mod needed` (reproduced on PiG 0.7.0, [log](.project/analyses/2026-11-05-slice-1-inventory/evidence/fused-reprobe.log)). Install it as a normal extension.
- Additional resource formats, SSH locations, PTY jobs, extended recovery and other depth are listed in the [Slice 2 plan](.project/plans/0003-slice-2-feature-complete.md).

## Development

```sh
just race         # race-enabled unit tests
just integration  # real PiG with a local scripted provider and tmux; no billed inference
```

The Go SDK is staged from the installed `pig`; use the Justfile recipes rather than bare `go test`. See the [Slice 1 plan](.project/plans/0002-slice-1-foundation.md) for the remaining acceptance gates.
