---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.12-14-skills-and-commands"
  title: "agy 1.4.12 skills and commands"
  description: "Skill discovery and prompt loading, slash expansion, plugins and subagent definitions"
  status: "published"
  revision: 1
  created_at: "2026-11-03T12:02:40+01:00"
  updated_at: "2026-11-03T12:02:40+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["document skill, slash command and customization contracts"]
---

# agy 1.4.12 — skills and commands

`[OBS]` identifies installed documentation, static binary bytes, descriptors, or CLI
output, not necessarily executed behavior. `[INF]` marks interpretation; `[UNK]`
marks what remains unresolved. No plugin, skill, configuration or live agent state was
modified. The binary identity is recorded in
[cli-and-settings.txt](evidence/context-recovery/cli-and-settings.txt).

## 1. Discovery and metadata

[OBS] The installed `agy-customizations` skill says the ordinary project roots are
`.agents/`, `.agent/`, `_agents/`, `_agent/`, searched from CWD up to the repository
root. Skills use `<root>/skills/<name>/SKILL.md`. Global discovery is under
`~/.gemini/config/`; built-ins are mounted by agent configuration rather than found by
that traversal. `skills.json` can declare additional paths and inherited manifests.
Its schema has `entries[]` and `inherits[]`, each with `path`, optional `include_only`
and `exclude`; entry directories are scanned one level deep. Relative paths resolve
against the directory containing the declaring dot-directory when it exists, falling
back to the project root. Config files are honored in each applicable project
customization root, not just at the repository root. A directly configured manifest is
expanded, while a configured directory retains its normal one-level scan.
([installed guide](evidence/context-recovery/builtin-customization-docs.txt)).

[OBS] For same-name customization collisions, the guide lists priority as workspace
project, workspace-declared configs, global discovery, built-ins, then globally
declared configs. This is an installed documentation claim, not an independently
executed precedence test. The guide also says resolved rule paths are deduplicated;
plugin skill duplicate loading was separately fixed in the embedded release notes
([seed strings](evidence/context-recovery/rodata-seeds.txt)). [UNK] Tie-breaking
within a tier and the complete scope of deduplication need controlled fixtures.

[OBS] The documented skill entry is a directory with `SKILL.md`; its YAML frontmatter
requires `name` and `description`. Optional sibling directories include `scripts/`,
`examples/`, `resources/`, and `references/`. The 1.4.12 plugin-validator release
notes describe an error for missing or invalid frontmatter, and warnings for missing
name/description, name-directory mismatch, a body over 24 KB, and a shadowed duplicate.
These are **validator** findings; they do not prove every invalid item fails loading.
The runtime protobuf `SkillMetadata` exposes `name`, `description`, `publisher`,
`version`, `icon`, `logo` ([schemas](evidence/context-recovery/proto-schemas.txt),
[validator excerpt](evidence/context-recovery/release-notes-extracts.txt)).

[OBS] `disable-slash-command: true` is an actual installed skill frontmatter field;
the release note states it hides the skill in the `/` menu and `/name` resolution
while leaving it model-discoverable and model-invocable
([field extract](evidence/context-recovery/skill-fields.txt)). The string
`disable-model-invocation` also occurs in the binary, but its source format and
behavior are [UNK]; do not assume it is the inverse switch for native skills.

## 2. How a skill enters context

[OBS] `SkillsSection.Content` is part of the system-prompt builder. The complete Go
template at `0x5e8c9be` is preserved in
[skill-template.txt](evidence/context-recovery/skill-template.txt). Its two paths are:

1. `Data.Skills`: include `- {{.GetName}} ({{.GetPath}}): {{.GetDescription}}`, tell
   the model to read the exact `SKILL.md` path when relevant, and allow skipping that
   read if the task is delegated to a child that will read it.
2. `Data.PreloadedSkills`: include the full file content verbatim under
   `<preloaded_skill name="{{.GetName}}" path="{{.GetPath}}" description="{{.GetDescription}}">`
   and `</preloaded_skill>`, telling the model it need not view those files.

An optional `.Data.TruncatedPrompt` is emitted at the end. [UNK] Selection of
preloaded skills, the numeric customization budget, and exact truncation text are not
established. The separate 20,000-token always-on rules budget and the 24,000-byte rule
limit must not be substituted for a skill budget; see
[12 §1–2](12-instructions-and-injections.md).

[OBS] The view-file payload includes `is_skill_file` and `skill_metadata` as in
[1.4.8 §6](../1.4.8/06-mechanics.md), and the current `ViewFileHandler.Handle`
references `SKILL.md` and `Failed to parse skill file: %v`
([function strings](evidence/context-recovery/additional-strings.txt)). [INF] A
normally listed skill becomes full context through a subsequent file-view tool result;
[UNK] exact role and any file-view size limit for that read require a request capture.

## 3. Slash command enumeration and expansion

[OBS] `GetSlashCommandsRequest` includes cascade configuration/id, workspace URIs,
active profile, personal enablement, custom agent spec, selected agent script,
project ID and global customization directories. Its response is `commands[]` of
`SlashCommandDefinition { info, title, description, arguments[] }`; arguments have
`name`, `description`, `required`, and optional `default_value`. The language server
has `getAllSlashCommands`, `hydrateSlashCommands`, and the CLI has
`reloadSlashCommandsSync`, `parseLeadingSlashCommands` and `parseSystemSlashCommand`.
The separate builders are `GetSystemSlashCommandsDefinitions`, `GetSkillSlashCommands`
and `GetWorkflowSlashCommands` ([schema](evidence/context-recovery/proto-schemas.txt),
[function index](evidence/context-recovery/function-index.txt)). [INF] Available
commands depend on context and configuration, rather than a single static menu.

[OBS] The `GetSkillSlashCommands` model-facing format at `0x5df0bcc`, with exact Go
string length recovered from disassembly, is:

```text
<SKILL>The user requested you read and use the %q skill. The path to the skill file is:
%s</SKILL>
```

The workflow counterpart at `0x5de9f63` is:

```text
<WORKFLOW>The user requested you run the %q workflow. Call the %s tool with ScriptPath=%q.
```

This recovered fragment contains no closing `</WORKFLOW>`; do not add one unless
another builder supplies it. The explicit-subagent format at `0x5e08748` is:

```text
<SUBAGENT>The user has explicitly added the `%s` subagent. You must use this subagent via the `%s` tool to process their request.</SUBAGENT>
```

The `deepagent` and schedule instruction templates are in
[exact-templates.txt](evidence/context-recovery/exact-templates.txt); source pointer
and length evidence is in [slash-disassembly.txt](evidence/context-recovery/slash-disassembly.txt).
[UNK] Whether these expansions are inserted as a user message, scope item, or another
provider-specific role in every mode requires a captured request. Do not assume that
expansion means the full skill body was inlined.

[OBS] `--disable-slash-commands` disables slash-command **and skill expansion in print
mode** ([CLI help](evidence/context-recovery/cli-and-settings.txt)); the binary also
contains a print-mode suggestion to use that flag to send `/%s` literally
([skill fields](evidence/context-recovery/skill-fields.txt)). The release notes name
`/plan` as a mode prefix and describe removing legacy `/fast`; installed guidance
points to `/help`, and the 1.4.12 release text names `/skills` and `/config`. Those
are examples, not a complete current enumeration. [UNK] Menu ordering, collisions,
argument substitution, and exact handling of local-only commands need a live
`GetSlashCommands` response or an isolated CLI trace.

[OBS] Legacy Markdown workflows are described by the installed `migrate-workflows`
skill as deprecated, with examples in `.agents/workflows/`, `_agents/workflows/`,
`.agent/workflows/` and `~/.gemini/config/workflows/`. A separate installed
`run-workflow` skill documents the model-provided `run_workflow` tool for Python-driven
`agent`, `parallel`, `pipeline`, and `phase` orchestration; this tool is not itself
proof of a slash command ([installed skills](evidence/context-recovery/builtin-skill-examples.txt)).
The full inventory of built-in tool-provided commands and their availability by agent
are [UNK]. See [1.4.8 §6](../1.4.8/06-mechanics.md) for the existing extension
inventory.

## 4. Plugins and subagent definitions

[OBS] The installed plugin guide defines `<root>/plugins/<name>/plugin.json` with
optional `name`, `author`, `homepage`, and `repository`. A plugin can ship
`skills/`, `rules/`, root `hooks.json`, and `mcp_config.json`; user `config.json`
`plugins[<directory>].enabled` overrides the manifest's `disabled` default.
Runtime native plugin hook discovery requires `hooks.json` at plugin root. The
installer can stage a nested `hooks/hooks.json` for imported Gemini CLI or Claude Code
extensions; native plugins do not get that staging. See the complete manifest and
configuration examples in [builtin-customization-docs.txt](evidence/context-recovery/builtin-customization-docs.txt).

[OBS] The current binary's plugin validator warns that plugin `commands/` directories
and Markdown `workflows/` files are not loaded, and flags nested `hooks/hooks.json`.
The same release notes describe plugin `agents/` Markdown: invalid frontmatter or
missing `name`/`description` is an error; duplicate or reserved names receive validator
findings. `agents/<agent>.md` also appears in the installed plugin-authoring skill.
These are validation and guide observations; runtime loading and collision resolution
need separate tests ([release extracts](evidence/context-recovery/release-notes-extracts.txt),
[installed skill](evidence/context-recovery/builtin-skill-examples.txt)).

[OBS] `CustomAgentConfig`, `CustomAgentSpec`, `SubagentSpec`, and the `--agent` CLI
flag remain present. Their role/tool-list and child-trajectory context are already
covered by [1.4.8 §5.1 and §6](../1.4.8/06-mechanics.md). The current
`SubagentSpec` has `type_name`, `role`, `initial_prompt`, `inherit`, workspace and
model fields ([schema dump](evidence/context-recovery/proto-schemas.txt)). [UNK] A
complete supported agent frontmatter schema, model/tool allow-list parsing, discovery
precedence for collisions, and exact child system/user prompt assembly require
additional disassembly or an isolated fixture run.
