---
document:
  type: "research"
  id: "RES-claude-code-cli-2.1.317-14-skills-and-subagents"
  title: "Claude Code 2.1.317 skills and subagents"
  description: "Skill and command discovery, agent definitions, plugin sources and deferred-tool search."
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:52:57+01:00"
  updated_at: "2026-11-03T11:52:57+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["document skills, agents and tool discovery"]
---
# Skills and subagents

Offline static analysis of the supplied 2.1.317 corpus. `C<line>:<offset>` addresses `host:/tmp/cc_code.txt` (local path `/tmp/cc_code.txt`) with 1-based lines and 0-based character offsets. `[OBS]` is code or a bundled literal; `[INF]` is an inference; `[UNK]` is unresolved. Supporting snippets and reproducible commands are in `evidence/14-*.txt`. Registry presence does not prove that a feature is enabled in a particular session.

## Skills: discovery and listing

[OBS] A filesystem skill scanner enumerates candidate directories and reads `<directory>/<name>/SKILL.md`, parses frontmatter and body, records source, base directory and content hash, and sorts the resulting skills by name. Invalid YAML is logged and ignored by that scanner; diskless sessions do not scan the ordinary local filesystem path (C25397:1632, C25397:3917). The settings schema names `.claude/skills/` and `.claude/commands/` as separate sources, alongside bundled skills and plugins (C13880:54461). Plugin skill loading uses its own scanner: it reads a regular size-bounded `SKILL.md`, parses frontmatter and creates a plugin-qualified name such as `plugin:skill` (C23350:4696). Shared memory stores also have a separate, more restrictive scanner (C20246:24094). [UNK] Exact search roots and collision precedence for all scopes and all clients.

[OBS] Before advertising a listing, `mLr` requires the Skill tool to be present, gathers filesystem and MCP command sources, removes model-disabled built-ins, merges/deduplicates names, and compares names and digests to previous `skill_listing` attachments. It emits only initial or changed entries (C25620:4924). The renderer emits an `isMeta` message through `ld` beginning:

> The following skills are available for use with the Skill tool:

[OBS] This listing contains selected listing text, not automatically every skill body. A `skill_mention` reminder says that if the user is asking to run a named `/skill`, call Skill with `skill` and supplied `args`; if merely mentioned, do not run it (C26136:1091). The skill invocation's body enters context separately; post-compaction restoration explicitly marks prior bodies as earlier invocations, not new requests (C26185:933). [UNK] Precise API wire placement for every listing and invocation variant.

## Frontmatter and command expansion

[OBS] The shared frontmatter schema includes `name`, `description`, `model`, `allowed-tools`, `disallowed-tools`, `argument-hint`, `arguments`, `disable-model-invocation`, `user-invocable`, `effort`, `shell`; skill-specific fields include `when_to_use`, `paths`, `hooks`, `context`, `agent`, and `background` (C22661:22446, C22661:23914). `allowed-tools` and `disallowed-tools` accept comma-separated strings or lists; the latter is described as cleared at the next user message. `disable-model-invocation:true` makes it user-invocable only; `user-invocable:false` hides the slash command from the user while retaining model invocation. `context:inline` expands in the conversation, `fork` invokes a subagent; `agent` selects its type. `paths` scopes loading to matching file touches. These are the schema's descriptions; runtime policy can add constraints.

[OBS] `skillOverrides` accepts `on`, `name-only`, `user-invocable-only`, `off`. `disableBundledSkills` or `CLAUDE_CODE_DISABLE_BUNDLED_SKILLS=1` removes bundled skills/workflows but leaves built-in slash commands typable while hiding them from the model; it does not disable plugins or filesystem skills/commands (C13880:54461).

[OBS] A plugin skill's `getPromptForCommand` prepends `Base directory for this skill: ${directory}`, substitutes arguments, optionally processes shell blocks, and returns a text block (C23348:40037). The shared `cVe` substitution code (C23067:648) does the following:

1. Splits arguments with `Wk`; if that returns no words, falls back to whitespace splitting. It recognizes named arguments, `$ARGUMENTS[N]`, `$N`, and `$ARGUMENTS`, with zero-based indexes.
2. Leaves a missing indexed placeholder intact, protects escaped dollar signs, and prevents substituted text from being recursively expanded. If no placeholder matched and appending is enabled with a nonempty argument string, it appends `\n\nARGUMENTS: ${args}`.
3. Caps the resulting skill/command/hook plus substitutions at **4,000,000** characters; named argument selection is bounded to **64** names of at most **64** characters each. The exact placeholder and escaping code is retained in `evidence/14-deep-code.txt`.

[OBS] Slash commands include prompt-backed skills and separately dispatched built-in `local`/`local-jsx` commands; the startup/init schema publishes `slash_commands`, optional `terminal_slash_commands`, and `skills` arrays (C14522:168494). The dispatcher maintains a built-in loader table including `/skills`, `/permissions`, `/sandbox`, `/plan`, and `/plugin` (C47315:46764). [UNK] Complete command enumeration, aliases, and permission treatment for every built-in. Inline `!` shell blocks are recognized by the skill/command module (C23067:242); invocation gates and permission handling are not fully traced.

## Subagents

[OBS] Three concrete built-in definitions are:

- `general-purpose`: `tools:["*"]`, source/base `built-in`, with a system prompt for multi-step research and execution (C24760:162).
- `Explore`: read-only search agent, specified disallowed tools, `model:"inherit"`, `omitClaudeMd:true`; described as a locator rather than an audit agent (C22501:1123).
- `Plan`: read-only planning agent, specified disallowed tools, `model:"inherit"`, `omitClaudeMd:true`, and a plan-design system prompt (C22550:155).

[OBS] `KVe` builds a conditional registry: general-purpose starts the ordinary list, Explore/Plan are added under `OG()`, and other built-ins are conditional on session and entrypoint (C24966:2535). Thus the three examples are not a universal complete set. The custom agent schema has `description`, `prompt`, `tools`, `disallowedTools`, `model`, `mcpServers`, `skills`, `maxTurns`, `autoCompactWindow`, `background`, `omitClaudeMd`, `memory`, `effort`, `permissionMode` and observer fields (C14522:13482). `memory` scopes are `user`, `project`, `local`; `omitClaudeMd` retains managed policy instructions. `maxTurns` counts agentic API round trips. The frontend schema additionally lists `isolation` values `worktree` and `remote` (C24966:3610).

[OBS] An agent's parent/session permission mode constrains its effective mode: the agent-launch path can reject a requested widening or unauthorized bypass and keep the parent mode; `dontAsk` and `plan` affect inherited `bubble` (C29388:12849). Hooks include `SubagentStart` and `SubagentStop` (C14521:45480). [UNK] Full custom-agent filesystem discovery precedence, tool execution/return transcript ordering, and concurrency limits.

## Plugins and marketplaces

[OBS] The plugin loader recognizes `.claude-plugin/plugin.json`, `.claude-plugin/marketplace.json`, and component paths including `commands`, `skills`, `agents`, `hooks`, `themes`, `output-styles`, `monitors`, `workflows`, `.mcp.json` and `.lsp.json` (C19517:4928). The SDK plugin configuration accepts a local plugin path and `skipMcpDiscovery` while still loading the other components (C14522:15202). The init schema can report plugin source sentinels `@inline`, `@skills-dir`, `@synced`, and `@builtin` (C14522:168494). [UNK] Complete marketplace trust, installation, update, precedence and manifest schema. No plugin was installed or executed in this research.

## ToolSearch / deferred-tool mode

[OBS] The search-mode resolver first applies two internal overrides. Otherwise `ENABLE_TOOL_SEARCH=auto:N` parses and clamps `N` to 0–100: `0` selects `tst`, `100` selects `standard`, intermediate `auto:N` and `auto` select `tst-auto`; truthy values select `tst`, falsy values `standard`, and the shown default is `tst` (C18841:764). Eligibility, model, provider and other gates still apply. The separate context-budget code defaults the auto percentage to **10** and computes `floor(modelContext * percent/100)`; the relationship of this estimate to the final switch decision was not fully traced (C24976:8741).

[OBS] `CYe` excludes `alwaysLoad`, non-deferrable and other guarded tools; otherwise MCP tools and tools declaring `shouldDefer:true` are candidates. ToolSearch itself and selected core tools are excluded from ordinary deferral (C20874:295). An MCP server `alwaysLoad:true` normally forces its tool schemas into the initial prompt, with a per-tool metadata exception, and makes startup wait up to the standard 5-second connect timeout (C14521:32052).

[OBS] The ToolSearch input schema is `{query, max_results?: number}`, default `max_results:5`; output data includes `matches`, `query`, `total_deferred_tools`, and optional pending/failed MCP servers (C23348:8055). Its prompt describes `select:Read,Edit,Grep` for exact selection, keywords for ranked search, and `+term` for a required name term. It states that unloaded schemas appear by name in system reminders and calls can fail until fetched (C20867:416). Its actual result builder shown at C23348:19325 returns `tool_result` content as `{type:"tool_reference",tool_name}` blocks for matches. The prompt also describes an alternative `<functions>` JSON-schema rendering; do not assume both representations are identical wire formats. Pre-compaction discovered tool names are recorded in `compact_boundary` metadata for subsequent schema filtering (C14522:198349).

[OBS] ToolSearch reminder generation scans assistant messages since the last ToolSearch call and prior reminder, requires both counts to reach configured `everyNTurns`, `tst` mode, an eligible model, an available ToolSearch tool, and at least one undiscovered name; it avoids colliding with the todo/task reminder (C25622:3808). The feature is off when `marsh_lantern` is absent/false; when enabled as `true`, its defaults are `everyNTurns:15`, `maxNames:10`; object `stride` and `span` override with positive integers (C14729:1064). Its exact builder text is at C26208:50. [UNK] The active remote feature value and live request formatting in this installation.
