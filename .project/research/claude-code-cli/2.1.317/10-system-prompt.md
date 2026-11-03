---
document:
  type: "research"
  id: "RES-claude-code-cli-2.1.317-10-system-prompt"
  title: "Claude Code 2.1.317 system prompt assembly"
  description: "Captured system arrays and static identity, output style, coordinator and cache evidence."
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:53:02+01:00"
  updated_at: "2026-11-03T11:53:02+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["record system prompt variants, composition and unresolved wire details"]
---

# System prompts

[OBS] Static citations use 1-based lines and 0-based character offsets into
`/tmp/cc_code.txt`; they are not executable offsets. The two full system arrays
below were captured from the installed 2.1.317 process to a local server in a
private network namespace. They are real outbound payloads for the specified
isolated scenarios, not captures of an Anthropic service or proof of upstream
acceptance. Prompt snapshots, settings, model, tools, extensions, provider and
per-session context affect other requests.

## Two concrete request arrays

[OBS] Both examples used the binary's default model, a fresh temporary home and
workspace, disabled nonessential traffic and an explicit dummy API key. The
print run used `-p 'Say hello.'`; the interactive run used a pseudo-terminal,
accepted its own empty workspace and dummy API key, and typed `Say hello.`.
The local server returned an error and the process was stopped after request
capture. The exact setup and commands are
[`evidence/capture-local.py`](evidence/capture-local.py) and
[`evidence/commands.txt`](evidence/commands.txt). No user account or remote
endpoint was used. Temporary paths and timestamps below are values of those
specific runs.

[OBS] The complete ordered arrays, including every `type`, `text` and
`cache_control` value, are
[`print-system.json`](evidence/system-prompt/print-system.json) and
[`interactive-system.json`](evidence/system-prompt/interactive-system.json).
The associated full tool lists are in
[`11-tool-declarations.md`](11-tool-declarations.md#complete-captured-lists).
Each individual text is also preserved without an added newline:

| Index | Print text | Interactive text | Captured cache control |
|---|---|---|---|
| 0 | [Billing](evidence/system-prompt/print-00.txt) | [Billing](evidence/system-prompt/interactive-00.txt) | absent |
| 1 | [SDK-agent identity](evidence/system-prompt/print-01.txt) | [CLI identity](evidence/system-prompt/interactive-01.txt) | `type: ephemeral` |
| 2 | [Composed prompt](evidence/system-prompt/print-02.txt) | [Composed prompt](evidence/system-prompt/interactive-02.txt) | `type: ephemeral` |

[OBS] The captured billing entrypoints are `sdk-cli` and `cli`, respectively,
and neither block has a cache marker. The two captured identity and composed
blocks have ephemeral cache control with no TTL field. The `messages` array also
contains context, and is preserved in the full captured request bodies; it is not
part of the `system` array. JSON was reserialized after capture, so values are
exact but JSON whitespace and escape spelling are not claimed byte-identical.

## Identity strings and selection

[OBS] Three complete identity strings are preserved byte-for-byte as
[`identity-cli.txt`](evidence/system-prompt/identity-cli.txt),
[`identity-sdk-cli.txt`](evidence/system-prompt/identity-sdk-cli.txt), and
[`identity-agent-sdk.txt`](evidence/system-prompt/identity-agent-sdk.txt)
(C19772:691). The first says the official CLI; the second adds Agent SDK; the
third identifies a Claude agent built on the SDK.

[INF] The selector gives the CLI identity for provider `vertex`, before looking
at a recorded identity. Otherwise a recorded identity takes precedence; if
`isNonInteractive` is true, it selects the SDK-CLI identity when
`hasAppendSystemPrompt` is true and the generic SDK-agent identity when false;
otherwise it selects the CLI identity (C19772:980). [OBS] The main request
passes `isNonInteractiveSession`, `hasAppendSystemPrompt` and recorded prefix
into that selector (C24990:41265). [UNK] This is a branch decision, not proof
that an arbitrary `-p` invocation actually supplies a particular combination;
the two isolated captures above agree with these respective branches but do
not prove all `-p` or interactive configurations. The historical 2.1.311 print and
interactive capture is documented at
`host:/mnt/Projects/furbyhaxx/pi-auth-plus/.project/analyses/claude-code-cli/tracking.md`
§“Logged-in CLI captures”; do not substitute it for a 2.1.317 capture.

## Built-in output styles

[OBS] The built-in style table has `default: null` and four non-null entries:
`Proactive`, `Concise`, `Explanatory` and `Learning`, all marked to retain coding
instructions (C22685:260, C22688:188, C22691:155, C22696:146). Their different
opening tails are followed by substantial style instructions, not just a single
sentence. Full resolved Proactive and Concise *style strings* are in
[`proactive.txt`](evidence/system-prompt/proactive.txt) and
[`concise.txt`](evidence/system-prompt/concise.txt). Their component source
expressions, preserving JavaScript escapes, sit beside them as `.source.txt`.
The other full resolved style strings are
[`explanatory-unicode.txt`](evidence/system-prompt/explanatory-unicode.txt),
[`learning-unicode.txt`](evidence/system-prompt/learning-unicode.txt), and the
[`explanatory-fallback.txt`](evidence/system-prompt/explanatory-fallback.txt) /
[`learning-fallback.txt`](evidence/system-prompt/learning-fallback.txt)
variants. Their source templates and shared insights fragment also sit beside
them. [OBS] The interpolated bullet is U+25CF in both branches; the star is
U+2605 or U+2736. On non-Windows the fallback is selected when `TERM` is
`linux`; Windows uses a terminal capability predicate (C21668:0–7456).
These are resolved style strings, not complete system prompts.

[INF] The asynchronous style resolver prefers a plugin forced style, else looks up
`outputStyle` in settings and defaults to `default` (C22767:1179). [OBS] The
composer uses the selected style's existence to choose a different intro and its
`keepCodingInstructions` value to gate the task section (C24578:5786, C24578:7639).
[OBS] The common prompt-layering path attempts to add `ohr()?.prompt`
(C20354:2774); its imported `ohr` function in this build is an empty getter
(C17978:2692). [UNK] A runtime path that injects a selected style's actual
prompt text was not established; the existence of the style table does not prove
that text is sent. There is also a separate non-software-specific intro, not a
fifth variant of the software-engineering opening: see
[`intro-output-style.txt`](evidence/system-prompt/intro-output-style.txt)
and C24532:260–583.

## Main composer and agent override

[OBS] The non-lean `Ppr` intro variants are
[`intro-default.txt`](evidence/system-prompt/intro-default.txt) for the default
opening and [`intro-output-style.txt`](evidence/system-prompt/intro-output-style.txt)
for the styled opening. These resolve the static `Ppr` and `cpr` source
fragments (C24532:251–583, C18545:31); they are sections, not complete prompts.

[OBS] The `Vpr` builder branches on `e.traits.includes("lean")` (C24579:25)
and uses `Ldn` for the lean body (C24552:3), which has a further `leanRewrite`
branch. The non-rewritten template is preserved as
[`lean-body.source.txt`](evidence/system-prompt/lean-body.source.txt). The two
captured bodies start with its default opening, security text and `# Harness`
section; they do not contain the full non-lean `Ppr` intro. [INF] Their observed
prefix is consistent with the non-rewritten lean path; the internal selector was
not instrumented.

[OBS] For the non-lean full composer the shared section IDs are ordered
`intro`, `system`, `doing_tasks`, `actions`, `tools`, `tone`; `doing_tasks`
is conditional on the selected output style retaining coding instructions.
The lean branch substitutes `lean_body`. Session sections then include, in
source order, communication, pronouns, action caution, task continuity, identity,
research limits, tool JSON rules, guidance, memory, surface and environment,
background session, context management, system messages and further optional
sections (C24578:7639). A bare branch emits either nothing or dynamically
formatted working directory and date (C24578:7639). [OBS] `W8n` groups shared
sections before the optional boundary marker and session sections afterwards
(C22385:2843); `G4e` adds memory, tool, style, and appended prompt layers with
shared/session placement determined by the boundary (C20354:2774,
C19772:0). [UNK] Every conditional section and its complete resolved text were
not extracted.

[OBS] If coordinator mode is active and there is no main-thread agent definition,
the prompt builder starts with the coordinator template, followed by available
style, other guidance and append layers (C24973:689). The source template is
[`coordinator.source.txt`](evidence/system-prompt/coordinator.source.txt)
(C21123:581); it contains dynamic interpolations and is not a full resolved
prompt. [OBS] `CLAUDE_CODE_COORDINATOR_MODE` is a gate, with additional checks
(C18458:143). A main-thread agent definition can supply its own system prompt,
either replacing or, when configured to append, joining the default
(C24973:689). [UNK] The precise settings, model and agent combination for every
branch remains unresolved.

## Billing, packing and caching

[OBS] The main request constructs a logical sequence `[billing, identity,
...prompt]`, dropping falsey entries (C19772:1157, C24990:41556). The billing
text template is at C17548:2662. [OBS] Its version suffix, entrypoint and
optional attribution fields depend on request/runtime values; its entrypoint
uses a resolved override or `CLAUDE_CODE_ENTRYPOINT`, with `unknown` as this
builder's fallback (C17548:700). [OBS] The exact billing values for the two
captured examples are in the linked text files above. [UNK] Values for other
entrypoints and configurations are not established.

[OBS] The packing function treats billing as uncached. Depending on global-cache
boundary and tool-based-cache branches, it can assign the identity an `org`
cache scope or no cache scope; shared text can receive `global` scope, and the
remaining session text may receive `org` or no scope (C19772:1835–C19778:580).
[OBS] If caching is enabled, each non-null scope becomes `cache_control` on its
text block (C24992:104544); the object uses `type: "ephemeral"`, optional TTL,
and emits `scope` only for `global` (C24990:2733). [OBS] TTL selection can
choose five minutes or one hour depending on query and runtime state
(C24990:2733–3500). [OBS] Cache placement for the two concrete local captures
is shown above.
[UNK] Other provider, account and feature-flag combinations remain unobserved;
do not assume the historical 2.1.311 identity-block setting applies in every
current branch.
