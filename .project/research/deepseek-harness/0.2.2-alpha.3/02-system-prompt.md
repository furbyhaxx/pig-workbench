---
document:
  type: "research"
  id: "RES-deepseek-harness-0.2.2-alpha.3-02-system-prompt"
  title: "DeepSeek Harness 0.2.2-alpha.3: system prompt"
  description: "Prompt assembly, source fragments, scoping and instruction hierarchy"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:52:59+01:00"
  updated_at: "2026-11-03T11:52:59+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["Document prompt construction and verified source fragments"]
---

# System prompt

## What can be called exact

[OBS] There is no single fixed full prompt in the assembler: it joins contributions from the active deployment and scope, evaluated against the current model and tools. With only the default `SystemPrompt` plugin, the entire nonempty system prompt is `You are an AI agent powered by DeepSeek Harness.` (`packages/core/system-prompt/src/index.ts:436-474`; expected in `packages/core/system-prompt/tests/system-prompt.spec.ts:230-240`). The tests were read, not run. The exact quoted identity, shipped coding persona template, selected tool guidance, context wrapper and base plan-mode guidance are in [evidence/prompt-fragments.txt](evidence/prompt-fragments.txt), each with its source location. These are source fragments, not an observed full request.

[OBS] Headless, Web, ACP and SDK-app overlays provide the template `You are a coding agent powered by the {{model}} model.` (`packages/bundle/headless/cordis.patch.yml:9-12`, `packages/bundle/web-app/cordis.patch.yml:17-20`, `packages/bundle/acp-app/cordis.patch.yml:3-6`, `packages/bundle/sdk-app/cordis.patch.yml:3-6`). The loop supplies `model` and `provider` from the selected route (`packages/core/agent-loop/src/index.ts:336-337`). Web selects the `standard` agent preset by default (`packages/bundle/web-app/cordis.patch.yml:598-601`); a scoped persona can override that deployment template (`packages/bundle/web-app/presets/standard.patch.yml:11-14`). The SDK-minimal bundle instead disables the identity and optional context and obtains its persona from `DSH_SYSTEM_PROMPT`, with a source default (`packages/bundle/sdk-minimal/cordis.patch.yml:113-118`).

[OBS] The full expected prompt for one repository SDK scenario is copied verbatim as [evidence/sdk-desktop-system-prompt.txt](evidence/sdk-desktop-system-prompt.txt), from `snapshots/sdk/desktop-context/system-prompt.expected.md:1-37`; its authored scenario selects the SDK profile (`snapshots/sdk/desktop-context/snapshot.yml:1-6`). It explicitly enables the optional `apply_patch` tool and configures deterministic time and other test settings (`snapshots/sdk/desktop-context/cordis.yml:1-41`), so this text is not the unmodified profile's exact prompt. A second, legacy SDK scenario is copied as [evidence/sdk-bash-tool-system-prompt.txt](evidence/sdk-bash-tool-system-prompt.txt); the legacy patch disables several desktop additions and enables `apply_patch` (`snapshots/sdk/legacy-context.patch.yml:1-28`; its selection is in `snapshots/sdk/sdk.snapshot.ts:704-726,775-779`). The complete PTC scenario prompt, including its generated SDK, is copied as [evidence/sdk-desktop-ptc-system-prompt.txt](evidence/sdk-desktop-ptc-system-prompt.txt) (`snapshots/sdk/desktop-ptc-context/system-prompt.expected.md:1-1637`); that scenario selects the PTC executor and enables `apply_patch` (`snapshots/sdk/desktop-ptc-context/cordis.yml:38-47`). These are checked-in test expectations, not executions performed for this dossier.

[UNK] The evidence does not contain every conditional plugin contribution or a fully rendered unmodified shipped-profile prompt. Reconstruction also needs the selected profile and preset, active plugins and their configuration, model, tool set, scope and waterfall listeners. No runtime assembly was captured. In particular the base plan-mode text and the Web preset's plan-mode text are not byte-identical (`packages/bundle/base/cordis.patch.yml:340-350`, `packages/bundle/web-app/presets/cordis.patch.yml:57-67`).

## Assembly and order

[OBS] `assemble()` merges global and scope-chain registrations by name, resolves providers, orders sections numerically and then by code-unit name, clones and orders tool schemas, resolves contexts, and runs a scope-filtered presentation waterfall (`packages/core/system-prompt/src/index.ts:620-695`). `renderPrompt` interpolates registered `{{name}}` variables, drops empty text, then joins sections with two newlines (`packages/core/system-prompt/src/index.ts:264-300`). Malformed, unknown or undefined complete references throw; `interpolate: false` preserves literal text (`packages/core/system-prompt/src/index.ts:338-377`). Tool declarations remain provider-owned and are not part of the waterfall presentation (`packages/core/system-prompt/tests/tool-order.spec.ts:90-97`).

[OBS] The built-in positions, in ascending order, are allocated by `packages/core/system-prompt/src/index.ts:136-174`:

| Order | Section slot |
|---:|---|
| -1000 | harness identity |
| 0 | deployment persona prefix |
| 500, 600, 800, 900 | plan policy, team policy, PTC-only, file reference |
| 1000–1700 | bash, PowerShell, read, write, edit, apply-patch, glob, grep, jobs, terminal guidance |
| 2000–3100 | web search/fetch, LSP, session query, goal, schedule, workflow, ralph, subagent, report, computer use, MCP servers |
| 5000 | generated tools SDK |
| 9000, 9900 | deliverable file references, structured output |
| 10000, 10100, 10200 | harness source, Web surface, deployment persona suffix |

[OBS] Slot allocation does not guarantee a section is registered or nonempty. For example read and edit guidance test visibility of their tools (`packages/fs/tool-fs/src/read.ts:69-75`, `packages/fs/tool-fs/src/edit.ts:77-83`); plan guidance is present only when selected plan state is active (`packages/plan/plan-mode/src/index.ts:197-204`). Base configuration owns its plan-mode prose (`packages/bundle/base/cordis.patch.yml:336-350`). Other registrations occur in `packages/fs/tool-fs-search/src/glob.ts:312`, `packages/fs/tool-fs-search/src/grep.ts:277`, `packages/jobs/tool-jobs/src/index.ts:316`, `packages/web/tool-web/src/search.ts:352`, `packages/web/tool-web/src/fetch.ts:450`, `packages/subagent/tool-subagent/src/index.ts:359`, `packages/team/tool-agent-team/src/index.ts:107`, `packages/workflow/tool-workflow/src/index.ts:326`, and optional packages. [UNK] Their full emitted text and activation combinations are not exhaustively transcribed here.

[OBS] Dynamic runtime contexts are separate from system sections: ordered context text gets the exact wrapper recorded in the evidence file (`packages/core/system-prompt/src/index.ts:177-184,309-340`). The loop projects that snapshot as a `user/message`, while the rendered prompt is logged as a `system/message` (`packages/core/agent-loop/src/agent.ts:442-476`). Working directory is registered as required context, with literal interpolation disabled (`packages/session/working-directory/src/index.ts:90-96`). Suppressing optional runtime context does not remove required contributions (`packages/core/system-prompt/src/index.ts:620-628,687-695`).

## Scoping and persona

[OBS] A scope-specific section or variable shadows the same global name before section evaluation; a scoped-only entry is invisible globally and its disposal removes it. The tests cover shadowing, provider evaluation and disposal (`packages/core/system-prompt/tests/scoped.spec.ts:33-81,84-102`). Scope-specific tool providers add to global providers rather than replacing them (`packages/core/system-prompt/tests/scoped.spec.ts:165-175`). Scoped assembly listeners affect only the matching scope (`packages/core/system-prompt/tests/scoped.spec.ts:229-245`).

[OBS] `dsh-persona` installs scoped `deployment:persona-prefix` and `deployment:persona-suffix` entries, replacing the same-name deployment entries; it requires a prefix, defaults the suffix to empty, defaults `complete` false and defaults runtime context enabled (`packages/preset/persona/src/index.ts:27-74`). An active `complete` section becomes the sole system section after the waterfall; more than one fails assembly (`packages/core/system-prompt/src/index.ts:659-695`; tests `packages/core/system-prompt/tests/system-prompt.spec.ts:497-522`). Tool schemas and contexts remain separately assembled; the persona can separately suppress optional runtime context (`packages/preset/persona/src/index.ts:63-74`). Presets declare child plugins and the registry mounts definitions in scopes (`packages/preset/agent-preset/src/index.ts:10-27`, `packages/preset/agent-preset-registry/src/index.ts:108-145`).

## Locale and instruction hierarchy

[OBS] The assembler's identity and its section names are inlined in TypeScript (`packages/core/system-prompt/src/index.ts:136-174,451-473`); profile persona text is inlined in YAML. English and Chinese locale files contain UI metadata rather than the quoted prompt (`packages/core/system-prompt/locale/en.json:1-5`, `packages/core/system-prompt/locale/zh.json:1-6`). [INF] There is no locale key to choose as a canonical translation of these fragments; their canonical source is the inlined string. [UNK] This does not establish whether every third-party plugin supplies locale-independent prompt text.

[OBS] Workspace instructions are distinct from the system prompt. Their model-facing introduction says more-specific instructions take precedence over broader ones and explicitly places them below system, developer and direct user instructions (`packages/context/agent-instructions/src/render.ts:16-20`). Discovery defaults to `.git` as the root marker, `AGENTS.md` then `CLAUDE.md`, followed by their `.local.md` variants; the default per-source maximum is 1,048,576 bytes (`packages/context/agent-instructions/src/config.ts:11-14,18-37`); the base profile configures a 65,536-byte chain budget (`packages/bundle/base/cordis.patch.yml:307-310`). The plugin produces user messages (`packages/context/agent-instructions/src/index.ts:194-195,233-234`), so the phrase `<system-reminder>` in its rendering is not itself a system-role message (`packages/context/agent-instructions/src/render.ts:16-17`; `packages/context/agent-instructions/src/state.ts:94-102`).
