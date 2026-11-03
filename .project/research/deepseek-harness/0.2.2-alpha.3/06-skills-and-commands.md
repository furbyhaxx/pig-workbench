---
document:
  type: "research"
  id: "RES-deepseek-harness-0.2.2-alpha.3-06-skills-and-commands"
  title: "DeepSeek Harness 0.2.2-alpha.3 skills and commands"
  description: "Skill discovery and prompting, commands, extension and protocol surfaces"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:54:02+01:00"
  updated_at: "2026-11-03T11:54:02+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["document source-observed skill and command surfaces"]
---

# Skills and commands

[OBS] Source is the pinned checkout at `/tmp/re-recon/dsh`, commit `8bd073392eef6f54e921a093b95076248eb08cbb`. Paths below are relative to that checkout. The target was not installed or executed.

## Skill discovery and selection

[OBS] `ctx.skills` merges independent providers; lower numeric rank wins duplicate names before provider registration order. The filesystem provider constructs roots in this order: project `.dsh/skills` (100), project `.agents/skills` (200), configured `customSkillDirs` (300), DSH-home `skills` (400), agents-home `skills` (500), and an optional bundled root (600). Runtime-registered skills use rank 250. DSH and shared home defaults are resolved from `$DSH_HOME`/`~/.dsh` and `$DSH_AGENTS_HOME`/`~/.agents`. The bundled directory is `bundledSkillDir`, else `$DSH_BUNDLED_SKILL_DIR` only when `includeDefaultRoots` is true; no fallback path is asserted here (`packages/skill/skill/src/index.ts:20`, `:30`, `:79`; `packages/skill/skill-filesystem/src/index.ts:33`, `:47`, `:164`, `:251`).

[OBS] Each immediate root entry can be a directory containing `SKILL.md` or a flat `.md` file. A skill requires YAML frontmatter with nonempty `name` and `description`; `name` must match `^[a-z0-9]+(?:-[a-z0-9]+)*$`. Optional `whenToUse` and object `metadata` are retained. `disable-model-invocation` and `user-invocable` control two independent surfaces; both default to permitted, and legacy camel-case invocation keys are rejected. The body is trimmed. Invalid or unreadable skills are skipped with warnings (`packages/skill/skill-filesystem/src/index.ts:737`, `:811`, `:1014`; `packages/skill/skill/src/index.ts:20`).

[OBS] The provider's `watch` defaults true; configuration includes `watchUsePolling`, `watchStabilityThresholdMs`, `watchPollIntervalMs`, `watchMaxProjects`, and `watchFollowSymlinks`. Root discovery failure makes an observation incomplete, rather than proving that missing skills do not exist (`packages/skill/skill-filesystem/src/index.ts:39`, `:75`, `:183`).

## How skills reach the model

[OBS] When the exact `skill` tool registration is visible, `tool-skill` snapshots model-invocable skills into a durable user-role `skill-catalog` context. The first catalog and replacement catalogs are framed in `<system-reminder>` and `<available_skills>`; unchanged catalogs are suppressed. Descriptions default to a maximum of 500 normalized characters. Incomplete discovery is explicitly narrated. The exact builders are captured in `evidence/injection-templates.txt` (`packages/skill/tool-skill/src/index.ts:27`, `:59`, `:217`, `:258`).

[OBS] The model calls `skill` with an exact name. The tool loads and returns the full body via `renderSkillContent`; the body is surrounded by `<skill_content>`, `<skill_resources>`, and `<skill_instructions>`, with a provider-specific base-path or URL hint. A direct user's first-line `/<name>` can instead load a user-invocable skill as a user-role `skill-invocation` instructions message appended after the other step injections; unknown or user-disabled names stay ordinary input. A user-invoked skill must not be loaded again via the tool (`packages/skill/skill/src/index.ts:165`; `packages/skill/tool-skill/src/index.ts:77`, `:154`, `:181`, `:258`).

[OBS] The Office provider registers `office`, `office-docx`, `office-pptx`, `office-xlsx`, `office-render`, `office-recalculate`, and `office-export-pdf` from packaged assets with bundled rank. The final three can append configured LibreOffice Kit paths or an explicit disabled notice (`packages/skill/skill-office/src/index.ts:13`, `:46`, `:69`).

[UNK] No user-facing skill installer workflow was established. Copying or creating files under a configured discovery root is inferable as a discovery mechanism, not evidence of a built-in install command. Search boot/packaging and UI code to determine installation workflows and where `$DSH_BUNDLED_SKILL_DIR` is set (`packages/skill/skill-filesystem/src/index.ts:170`).

## Commands

[OBS] The first-party command identities and English tokens are `goal`, `plan`, `feedback`, `compact`, `permission`, and `export`; Chinese aliases are `目标`, `计划`, `反馈`, `压缩`, `权限`, and `导出`. An exact scoped command name wins before an alias lookup. These are registry identities, not proof that every plugin is mounted in every profile (`packages/interaction/commands/src/builtin.ts:4`, `:16`, `:40`).

[OBS] `parseCommand` recognizes `/` followed by a lowercase ASCII letter, then lowercase ASCII letters, digits, `_` or `-`, terminated by whitespace or end of input. The command executes on the receiving agent without automatically sending the slash line to the model. The registry appends `command/run` before the handler and `command/done` after settlement; handlers may independently inject messages. Command registrations are scoped and may shadow other layers (`packages/interaction/commands/src/index.ts:31`, `:61`, `:125`, `:335`, `:361`).

[OBS] `/plan` and `/permission` are described in `05-modes-and-approvals.md`. `goal`, `feedback`, `compact`, and `export` are registered by their owning plugins (`packages/goal/command-goal/src/index.ts:191`; `packages/feedback/command-feedback/src/index.ts:119`; `packages/compaction/command-compact/src/index.ts:101`; `packages/session-query/session-log-export/src/index.ts:81`).

## Extensions, MCP and ACP

[OBS] Dynamic Cordis integrations have host and client runners. The model-visible inspection tools include `cordis_inspect_list` and `cordis_inspect_query`; runtime host/client failures may be steered to the agent as `cordis-host-runner` source messages (`packages/extensions/tool-cordis/src/index.ts:22`; `packages/extensions/cordis-host-runner/src/index.ts:1062`, `:1075`, `:1099`, `:1124`). [UNK] Complete extension installation, execution security and lifecycle behavior require a focused read of `packages/extensions/cordis-host-runner/src/` and its composition.

[OBS] Each `mcp-client` instance connects to one server using `stdio` or `streamable-http`. `serverName` must match `[A-Za-z0-9_-]{1,32}` and be unique within its registration scope. Tool names are normally `mcp__<serverName>__<rawName>`; lossy normalization or a name exceeding 64 characters appends a 12-hex SHA-256 identity suffix. The raw tool name, rather than the public name, is used for the wire call. Tool-list fetch builds a candidate generation before replacing registrations; a failed swap removes partial registrations (`packages/mcp/mcp-client/src/index.ts:1`, `:51`, `:75`, `:109`, `:163`; `packages/mcp/mcp-client/src/tools.ts:54`, `:79`, `:103`).

[OBS] MCP server instructions are prefixed `### MCP server: ${serverName}\n\n` and rejected if their complete UTF-8 byte length exceeds `maxInstructionBytes`, whose source default is 32,768. They are contributed as literal system-prompt sections `mcp:${server}` at `MCP_SERVERS` order (`packages/mcp/mcp-client/src/connection.ts:55`, `:317`; `packages/mcp/mcp-client/src/server-context.ts:30`).

[OBS] ACP is an automation-only JSON-RPC stdio server. Its initialization advertises HTTP MCP, conditional image prompts, and session close/list/resume. For bridge-owned tool calls with a call id, it sends ACP permission choices `allow-once` and `reject-once`; unknown options reject. It does not infer durable grants (`packages/acp/acp/src/index.ts:1`, `:151`, `:178`). [UNK] Detailed ACP session negotiation, cancellation and MCP-mount error cases remain untraced (`packages/acp/acp/src/session.ts`).
