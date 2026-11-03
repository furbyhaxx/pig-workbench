---
document:
  type: "research"
  id: "RES-claude-code-cli-2.1.317-11-tool-declarations"
  title: "Claude Code 2.1.317 model-facing tool declarations"
  description: "Captured ordered tool declarations and static evidence for built-in, deferred and MCP tools."
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:53:02+01:00"
  updated_at: "2026-11-03T11:53:02+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["record tool declaration evidence and unresolved wire surface"]
---

# Model-facing tool declarations

[OBS] `C<line>:<offset>` indexes `/tmp/cc_code.txt` with 1-based lines and
0-based character offsets; neither is an executable file offset. The corpus
protocol is `../2.1.310/tools/CORPUS.md`. Two actual 2.1.317 outbound request
bodies were captured to an offline loopback server, not Anthropic: see
[`evidence/commands.txt`](evidence/commands.txt) and
[`evidence/capture-local.py`](evidence/capture-local.py). The setup used an
empty temporary home and workspace, a dummy API key, the binary's default model,
and disabled nonessential traffic. A private network namespace prevented any
outside connection. The interactive run explicitly accepted its own empty
workspace and dummy key. These are concrete isolated scenarios, not proof of
what an existing user account or provider receives.

## Complete captured lists

[OBS] The complete ordered tool arrays are
[`print-tools.json`](evidence/tools/print-tools.json) and
[`interactive-tools.json`](evidence/tools/interactive-tools.json). Each file
linked in the table is a complete declaration, including its exact description
string, input schema and any flags. The source bodies are
[`print-request.json`](evidence/wire/print-request.json) and
[`interactive-request.json`](evidence/wire/interactive-request.json). They
were parsed and reserialized as JSON, so JSON whitespace and escaping may differ
from transmitted bytes; the string and JSON values are preserved. The first
request used `-p 'Say hello.'`; the second entered `Say hello.` in the TUI. The
local server returned an error and the client was stopped after capture; no
upstream acceptance or model response is claimed.

| Tool | Print index / declaration | Interactive index / declaration |
|---|---|---|
| `Agent` | [0](evidence/tools/print/00-Agent.json) | [0](evidence/tools/interactive/00-Agent.json) |
| `AskUserQuestion` | absent | [1](evidence/tools/interactive/01-AskUserQuestion.json) |
| `Bash` | [1](evidence/tools/print/01-Bash.json) | [2](evidence/tools/interactive/02-Bash.json) |
| `Edit` | [2](evidence/tools/print/02-Edit.json) | [3](evidence/tools/interactive/03-Edit.json) |
| `ListAgents` | [3](evidence/tools/print/03-ListAgents.json) | [4](evidence/tools/interactive/04-ListAgents.json) |
| `Read` | [4](evidence/tools/print/04-Read.json) | [5](evidence/tools/interactive/05-Read.json) |
| `ReportFindings` | [5](evidence/tools/print/05-ReportFindings.json) | [6](evidence/tools/interactive/06-ReportFindings.json) |
| `ScheduleWakeup` | [6](evidence/tools/print/06-ScheduleWakeup.json) | [7](evidence/tools/interactive/07-ScheduleWakeup.json) |
| `Skill` | [7](evidence/tools/print/07-Skill.json) | [8](evidence/tools/interactive/08-Skill.json) |
| `ToolSearch` | [8](evidence/tools/print/08-ToolSearch.json) | [9](evidence/tools/interactive/09-ToolSearch.json) |
| `Workflow` | [9](evidence/tools/print/09-Workflow.json) | [10](evidence/tools/interactive/10-Workflow.json) |
| `DeferredToolPlaceholder` | [10](evidence/tools/print/10-DeferredToolPlaceholder.json) | [11](evidence/tools/interactive/11-DeferredToolPlaceholder.json) |
| `Write` | [11](evidence/tools/print/11-Write.json) | [12](evidence/tools/interactive/12-Write.json) |

[OBS] The requests share 12 names; 10 corresponding declarations are equal, and
`Agent` and `Bash` differ. `AskUserQuestion` appears only in the interactive
capture. [UNK] The two runs also differ in onboarding and runtime context, so
these comparisons do not isolate mode as the only cause of any difference.

## Built-in names

[OBS] The following bare, case-sensitive names appear as constants in tool-related
modules; their actual inclusion varies with runtime configuration and permissions.
This is an inventory of located constants, not an emission-order list:

| Name | Evidence |
|---|---|
| `Read` | C14600:20 |
| `Edit`, `Write`, `Bash`, `PowerShell` | C14607:48, C14607:2139–2154 |
| `NotebookEdit` | C14807:72056 |
| `Glob`, `Grep` | C14808:67828–67842 |
| `Agent` | C17808:73 |
| `EnterPlanMode`, `ExitPlanMode` | C17863:95, C18034:0 |
| `Skill` | C17877:422 |
| `WebSearch`, `WebFetch` | C17884:7806, C18862:0 |
| `TodoWrite` | C18006:0 |
| `ToolSearch` | C18027:0 |
| `AskUserQuestion` | C19601:94 |
| `TaskStop`, `TaskCreate`, `TaskUpdate`, `TaskList`, `TaskGet` | C20069:479, C20165:99–119, C20804:215, C21002:133 |

[OBS] Historical 2.1.311 interactive and print-mode captures document bare
PascalCase names and a deferred placeholder; see
`host:/mnt/Projects/furbyhaxx/pi-auth-plus/.project/analyses/claude-code-cli/tracking.md`
§“Logged-in CLI captures”. They are not 2.1.317 runtime captures. [UNK] The
complete set of declarations and inclusion predicates across all 2.1.317 modes
remains to be established.

## Description factories

[OBS] The code sends `prompt(...)` as the description when `cBe()` is false;
when true it prefers `searchHint`, or the first prompt paragraph if no hint exists
(C23309:8361). [OBS] A later `jNe(...)` transform, hooks, and recorded snapshots
can affect the actual description (C23311:4160–5500). The shorter
`description()` members displayed by the UI are not by themselves the
model-facing declarations.

[OBS] The complete static prompt-factory results for two tools are
[`TaskGet`](evidence/tools/task-get-description.txt) (C30242:315) and
[`TaskUpdate`](evidence/tools/task-update-description.txt) (C30264:46).
Their corresponding `.source.txt` files preserve source escapes. [UNK] Their
final wire text depends on the transformations and runtime switches above.

[OBS] For the `cBe()` branch, literal search hints include:

| Tool | Search hint | Evidence |
|---|---|---|
| `Bash` | `execute shell commands` | C25945:9029 |
| `Read` | `read files, images, PDFs, notebooks` | C25409:11261 |
| `Edit` | `modify file contents in place` | C25861:1235 |
| `Write` | `create or overwrite files` | C23146:31492 |
| `Glob` | `find files by name pattern or wildcard` | C23154:11972 |
| `Grep` | `search file contents with regex (ripgrep)` | C23158:4150 |
| `Agent` | `delegate work to a subagent` | C29574:16813 |
| `Skill` | `invoke a slash-command skill` | C29665:16010 |
| `TaskGet` | `retrieve a task by ID` | C30263:198 |
| `TaskUpdate` | `update a task` | C30338:863 |

[UNK] These hints are not asserted to be the final descriptions in the two
captured requests; their exact final descriptions are in the declaration files.

## Deferred placeholder

[OBS] The exact declaration constructed by the code is
[`evidence/tools/deferred-tool-placeholder.json`](evidence/tools/deferred-tool-placeholder.json).
Its name and description constants are at C18027:0; the object and
`defer_loading:!0` are at C24987:3970–4013. [INF] Its construction is gated,
not unconditional; the surrounding code invokes the `tengu_deferred_stub_tool`
flag and returns `null` if the computed condition is false (C24987:3970).

## Schema serialization

[OBS] The serializer constructs a declaration with `name`, `description` and
`input_schema`; it can add `strict`, `eager_input_streaming`, `defer_loading`,
and `cache_control` (C23311:4485–5973). [OBS] It obtains the schema from an
existing `inputJSONSchema` or converts `inputSchema`, then may remove or
alter fields for context (C23309:5759). Raw schema factories for `Read`, `Edit`,
`Write`, `Glob`, `ToolSearch`, `TaskCreate`, `TaskGet`, `TaskUpdate` and
`TaskList` are in
[`evidence/tools/schema-factories.source.txt`](evidence/tools/schema-factories.source.txt).
These are JavaScript schema expressions, **not** serialized JSON schemas.
[OBS] The exact transformed schemas and flags for the two captured requests are
in the declaration files above. [UNK] The schemas for other configurations and
conditional tools remain incomplete.

## MCP and emission order

[OBS] The MCP registry's name builder emits ``mcp__${yn(e)}__${n}``
(C22385:12997). [UNK] `yn`'s complete normalization and any no-prefix
exceptions were not resolved. [OBS] Remote passthrough `input_schema` is
normalized, checked to have object type and passed through a schema judge
(C18341:13158–13541); MCP servers supply their own schemas.

[OBS] The base registry has `Agent`, conditional `Bash`, `Glob`, `Grep`, then
other tools including `Read`, `Edit` and `Write` (C30372:3638). [OBS] That is
**registry order**, not final wire order: tool-pool assembly filters and sorts
its built-in group and MCP group separately by `name.localeCompare`, then
concatenates and deduplicates (C30372:5763, C18384:12928). [OBS] The request
builder maps its selected tools to schemas with `Promise.all` (C24990:30900),
and, when its dynamic-loading channel is active and the placeholder is absent,
inserts the placeholder immediately before the last current declaration, or
at index zero for an empty list (C24990:33870). [UNK] Other selection, replay and extension paths can change membership and
order beyond the two captured examples.
