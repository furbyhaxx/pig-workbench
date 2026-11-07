# pig-workbench

A [PiG](https://github.com/MichaelKinsy/PiG) extension for coding tools, resource URIs, working locations, instruction frames, secret redaction, questionnaires and supervised shell jobs.

**Status:** Slice 1 is in progress and is not frozen. The extension runs on Linux, but some Slice 1 behaviors and acceptance checks remain open; see [current limitations](#current-limitations).

## Get started

Install Go, [PiG](https://github.com/MichaelKinsy/PiG) and `just`, then from this repository:

```sh
just check   # format check, vet, tests and PiG validation
pig -e .     # load the extension in a session
```

Use `/workbench` for an overview, `/tools` to inspect tool schemas and `/workbench settings catalog` to see which settings are applied. See [tools](docs/tools.md), [configuration](docs/configuration.md) and [extension services](docs/extension-services.md) for contracts and examples.

## What works

- **Read and edit:** `read` supports bounded multi-read, selectors and registered `scheme://` providers; local source snapshots supply hashline tags and seen-line guards. `edit` supports tagged batches, structural blocks, registers and exact-text replacement. `write` atomically creates or replaces local files with optional revision checks; it also dispatches supported `job://`, `skill://`, `agent://` and `session://` control writes. `ls`, `grep` and `find` are available; grep/find currently search local filesystem paths only.
- **Resources:** Builtin trees include `file://`, `http(s)://`, `local://`, `out://`, `artifact://`, `cfg://`, `session://`, `job://`, `skill://`, `agent://` and registered `docs://` resources. HTTP can use a contributed fetch backend or the bounded builtin static reader. Resource providers from other extensions are read-only.
- **Locations and jobs:** `cd`, `/cd` and `/wb:cd` change Workbench's captured local location without changing PiG's cwd. `bash` and the single `jobs` tool supervise foreground and background processes, stdin, output and cancellation; `/workbench jobs` and `/wb:jobs` expose the same jobs. Session-storage links and `/workbench projects unlink-storage` are documented in [configuration](docs/configuration.md).
- **Instructions and frames:** Applicable steering and backlog files arrive as attributed frames. `/workbench instructions` manages inspection and backlog pins; `/workbench frames` inspects frames. Changes are reconciled at prompt or tool boundaries, not by native file watchers.
- **Questions and secrets:** `ask_user` uses native interactive dialogs; headless use reports `interactionUnavailable`. Unanswered forms can be inspected, resumed or discarded through `/workbench ask`. The secrets boundary redacts known values from model-bound content and supports encrypted secret answers and controlled substitution.
- **Sessions:** `agent://` and `/workbench sessions` provide authenticated same-user session discovery and messaging, including idle wake and busy steering. `/workbench sessions` can also trust, block, hide or show peers. Session trees and titles are accessible through `session://`.
- **Inspection:** `/workbench settings`, `prompt`, `skills`, `integrations`, `resources`, `trust`, `projects` and `show` expose their respective state. The activity line displays live counts and contributed items; fixed `alt+w` opens its inspection shortcut.
- **Extensions:** Protocol v1 exposes location, presentation, resources, fetch, status, docs, frames and snapshots; sessions are advertised when an endpoint is available, and `secrets.redact` only to explicitly granted owners. See [extension services](docs/extension-services.md).

## Current limitations

**Unfinished Slice 1:**

- `grep` accepts `context` but does not emit surrounding lines; `grep` and `find` do not search provider URIs. The project-private instruction class is not wired.
- For non-locatable scheme paths, standalone `ls` and `cat` are routed in-process. A single-word `value > URI` dispatches supported control writes with the same confirmation rules as `write`; it is not general shell redirection. `rm`, `mv`, `cp` and `mkdir` are rejected. Provider mutation authorization and full scheme-command routing remain open.
- `prompt.delivery = "auto"` retains PiG's prompt; `force` opts in to `builtin/workbench` only when instructions are enabled. The live prompt A/B evaluation and full real-agent transcript await authorization for provider spend. The `prompt.template` setting is not applied.
- All fourteen specifications are approved, but implementation and acceptance remain incomplete. The [acceptance inventory](.project/analyses/2026-11-05-slice-1-inventory/README.md) distinguishes observed tests from incomplete rows.
- Windows messaging, session storage and registry have compile-level coverage only; some required platform security operations fail closed. Stored-session title confirmation has been tested through PiG's RPC dialog path, not visually in a TUI. Real session-reply hop limiting has been exercised with a local scripted provider.

**Deferred depth:**

- Broadcast, coordinator and by-project agent/session trees belong to Slice 2. The rich questionnaire panel is later UI work.
- Activity-item click needs a fork host change. `activity.shortcut` is not applied: `alt+w` is registered statically.
- Additional resource formats, SSH locations, PTY jobs, extended recovery and other depth are listed in the [Slice 2 plan](.project/plans/0003-slice-2-feature-complete.md).

## Development

```sh
just race         # race-enabled unit tests
just integration  # real PiG with a local scripted provider and tmux; no billed inference
```

The Go SDK is staged from the installed `pig`; use the Justfile recipes rather than bare `go test`. See the [Slice 1 plan](.project/plans/0002-slice-1-foundation.md) for the remaining acceptance gates.
