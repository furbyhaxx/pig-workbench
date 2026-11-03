---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.12-11-tool-declarations"
  title: "agy 1.4.12 tool declarations"
  description: "Recovered tool names, description literals and schema-generation evidence, with unresolved wire ordering"
  status: "published"
  revision: 1
  created_at: "2026-11-03T12:02:33+01:00"
  updated_at: "2026-11-03T12:02:33+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["recover converter and oneharness tool literals", "record schema evidence and unresolved declaration order"]
---

# agy 1.4.12 — tool declarations

## Limits and reproduction

[UNK] The ordered tool list and complete serialized parameter schemas sent by a
default session were not recovered. This is **not** a complete reconstructed
request. [OBS] The names below are strings assigned by compiled tool-definition
constructors or referenced by oneharness `Name` methods; their presence does
not prove that a given session enables them. [INF] Dynamic tool overrides,
mode, provider, MCP servers and configuration can affect the offered surface.
[UNK] Their actual effects and the default combination have not been established.

[OBS] Exact directly stored names, descriptions, lengths, offsets, constructor
symbols, and schema-generation call targets are in
[`evidence/tools/constructor-scan.json`](evidence/tools/constructor-scan.json).
Run `python3 evidence/extract-tool-literals.py "$HOME/.local/bin/agy"` from
this version directory to reproduce it. The extractor disassembles the cache
entry plus `0x80`: selected actual function prologues and pointer/length stores
are preserved in [`selected-disassembly.txt`](evidence/tools/selected-disassembly.txt).
This correction is evidenced for the inspected constructors and must not be
assumed for unrelated binaries. The raw description strings in JSON are exact;
JSON escaping represents the original newlines and tabs.

[OBS] Additional oneharness direct-return names and descriptions are in
[`oneharness-scan.json`](evidence/tools/oneharness-scan.json), reproduced with
`python3 evidence/extract-oneharness.py "$HOME/.local/bin/agy"`. Other methods
can return computed or conditional text and are not represented as complete
literals by that scan.

## Statically assigned converter names

[OBS] The literals below and their offsets occur in `GetToolDefinition`
constructor instructions. [INF] The inspected stores associate each name and
description with the corresponding definition. Where shown, `offset + bytes`
identifies a directly stored description literal in `constructor-scan.json`;
`[UNK]` means this scan could not recover a complete description. This is an
inventory, **not** the order sent to a model.

| Name | Description literal |
|---|---|
| `ask_custom_permission` | [UNK] |
| `ask_permission` | [UNK] |
| `ask_question` | `0x5e6ab1f` + 1265 |
| `call_mcp_tool` | `0x5df3b50` + 102 |
| `command_status` | `0x5e2067a` + 260 |
| `define_subagent` | `0x5e3996e` + 533 |
| `find_by_name` | `0x5e363c8` + 490 |
| `finish` | `0x5e17725` + 208 |
| `generate_image` | `0x5e38703` + 518 |
| `grep_search` | [UNK] |
| `invoke_subagent` | `0x5e4e4ef` + 843 |
| `list_dir` | `0x5e354b4` + 479 |
| `list_permissions` | `0x5e0316c` + 127 |
| `list_resources` | `0x5d813d9` + 49 |
| `manage_inbox` | `0x5e0b7f5` + 152 |
| `manage_subagents` | [UNK] rendered text; see conditional templates below |
| `manage_task` | `0x5e3260a` + 440 |
| `notebook_edit` | `0x5e3bdb4` + 561 |
| `notebook_execution` | `0x5e347c4` + 468 |
| `read_resource` | `0x5d5baab` + 42 |
| `run_command` | [UNK] |
| `schedule` | [UNK] |
| `search_web` | `0x5df76d5` + 108 |
| `sed_file` | `0x5e86527` + 1691 |
| `send_command_input` | `0x5e272ee` + 312 |
| `send_message` | [UNK] |
| `view_file` | [UNK] |
| `wait` | `0x5d1c50c` + 33 |
| `wait_5_seconds` | `0x5d7794e` + 47 |
| `write_to_file` | [UNK] selector; two literal alternatives below |

[INF] For example, the `list_dir` constructor sets its name from
`0x5b62e97` and its 479-byte description from `0x5e354b4`. [OBS] The exact
description literal is:

```text
List the contents of a directory, i.e. all files and subdirectories that are children of the directory. Directory path must be an absolute path to a directory that exists. For each child in the directory, output will have: relative path to the directory, whether it is a directory or file, size in bytes if file, and number of children (recursive) if directory. Number of children may be missing if the workspace is too large, since we are not able to track the entire workspace.
```

[INF] The `FindToolConverter` assigns the wire name `find_by_name`, rather
than establishing `find` as its emitted name. [OBS] Its description literal at
`0x5e363c8` is:

```text
Search for files and subdirectories within a specified directory using fd.
Search uses smart case and will ignore gitignored files by default.
Pattern and Excludes both use the glob format. If you are searching for Extensions, there is no need to specify both Pattern AND Extensions.
To avoid overwhelming output, the results are capped at 50 matches. Use the various arguments to filter the search scope as needed.
Results will include the type, size, modification time, and relative path.
```

[OBS] The 50-match sentence is part of this converter's description; it is not
independently verified as a runtime limit, nor as applicable to another tool.
The disassembly of both constructors is in `selected-disassembly.txt`.

## Conditional and oneharness declarations

[OBS] `write_to_file` has two directly loaded descriptions, with and without an
artifact instruction, at `0x5e2bbbf` (361 bytes) and `0x5e35872` (479 bytes).
`manage_subagents` has two format-string templates at `0x5e316b0` (434 bytes)
and `0x5e3bfe5` (562 bytes), the latter mentioning `list_peers`. The unrendered
bytes and reproducible extraction command are in
[`conditional-descriptions.json`](evidence/tools/conditional-descriptions.json)
and `evidence/extract-tool-fragments.py`. [UNK] The branch selectors and the
inserted `%s` schema for `manage_subagents` are not established.

[OBS] The same evidence file preserves individual format fragments used while
constructing `run_command`, `grep_search`, and `view_file` descriptions,
including their `%s`/`%d` placeholders. Their instruction sites include those in
[`conditional-disassembly.txt`](evidence/tools/conditional-disassembly.txt).
[UNK] These fragments have not been assembled into complete descriptions or
resolved to the defaults of a session.

[INF] Oneharness constant-return methods separately emit `ask_question`,
`define_subagent`, `edit_file`, `finish`, `generate_image`, `invoke_subagent`,
`manage_subagents`, `manage_task`, `read_file`, `run_command`, `run_workflow`,
`schedule`, `search_web`, `send_message`, and `write_file`; see
`oneharness-scan.json` for their exact byte offsets and the subset of direct
constant descriptions. [UNK] It is not established whether this implementation
or the Cortex converter implementation supplies each declaration in the
default CLI session.

## Parameter-schema evidence

[OBS] The `list_dir` constructor calls `ToJSONSchemaString` with the recovered
type shape below. This is the Go input type and its reflection tags, **not** a
capture of the final JSON schema:

```text
struct { DirectoryPath string "jsonschema:\"required\" jsonschema_description:\"Path to list contents of, should be absolute path to a directory\"" }
```

[OBS] Disassembly of the schema helper calls the `invopop/jsonschema`
`Reflector.Reflect` and `Schema.MarshalJSON` methods. `constructor-scan.json`
records other directly called shape names, including explicit shapes for
`view_file`, `notebook_execution`, `search_web`, `call_mcp_tool`,
`list_resources`, `read_resource`, `send_message`, and `manage_subagents`.
Some shapes are named only by a hash or embed other named structures. [UNK]
The exact resulting JSON, generated property names, all nested structures,
conditional branches and any later schema transformations remain unverified.
Do not substitute the execution-step payload for the declaration schema.

## Relation to the 1.4.8 inventory

[OBS] The prior inventory in [`../1.4.8/06-mechanics.md` §2.2](../1.4.8/06-mechanics.md)
lists file operations, shell and terminal operations, search, web and browser,
MCP, subagent, task, memory, knowledge, permissions and notebook steps. It
records step numbers and payloads for those candidates. It does not prove that
all are declared simultaneously or that every step label is its wire name.
[INF] The direct 1.4.12 method bodies above establish additional names including
`find_by_name`, `write_to_file`, `notebook_execution`, `sed_file`,
`run_workflow`, and `write_file`. [UNK] These observations alone do not prove
when those names were introduced or whether they are active by default.
