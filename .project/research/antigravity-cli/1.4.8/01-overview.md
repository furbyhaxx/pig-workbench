---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.8-01-overview"
  title: "agy 1.4.8 identity and architecture"
  description: "What the artifact is, its Codeium heritage, component map, three-layer architecture and child-process model"
  status: "published"
  revision: 2
  created_at: "2026-10-27T09:59:34+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "pi"
  updated_by: "orchestrator"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/antigravity-cli/1.4.8/01-overview.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 1
      at: "2026-10-27T09:59:34+01:00"
      note: "Initial analysis of agy 1.4.8."
    - revision: 2
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/antigravity-cli/1.4.8/01-overview.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---

# 01 — Identity and architecture

## 1. What the artifact is

| field | value |
|---|---|
| name | Google Antigravity CLI |
| command | `agy` |
| version analysed | **1.4.8** |
| path | `~/.local/bin/agy` |
| size | 228,897,168 bytes |
| sha256 | `70b1d27f1812f5ebccc8f13daf2209112447413437dd1ab6d87c58b8a04fe35b` |
| format | ELF64 x86-64, PIE, dynamically linked, **stripped** |
| toolchain | `go1.28-20261015-RC02 cl/1000353293 +18a2e1f59d`, flags `X:fieldtrack,boringcrypto,simd,nogolistexportnewformat` |
| optimisation | BOLT post-link (`.text.hot`, `.text.split`, `.text.unlikely`, `.lrodata`, `google_malloc` sections) |
| internal codename | **jetski** |

Two things follow from the build metadata.

**It is an internal Google build.** `cl/1000353293` is a Google-internal changelist,
`X:fieldtrack`/`nogolistexportnewformat` are internal compiler flags, and the package
paths are `google3/third_party/jetski/...` rather than a public module path. There is no
usable `.go.module` dependency list, so third-party versions cannot be read off the
binary directly.

**It is Codeium-derived.** Witness strings: `codeium_common_go_proto.*` (3,550
functions), `codeium.yaml`, `/google/cog/`, `codeiumconverter`. Antigravity's agent
engine descends from the Codeium/Windsurf codebase, rebranded and extended inside
Google. This explains the `exa.*` protobuf package prefix (`exa` is the Codeium-era
namespace) sitting alongside newer `google.gca.aicode.*` packages.

## 2. Scale

| metric | value |
|---|---|
| functions (pclntab) | 171,928 |
| distinct source `.go` files named | 6,825 |
| functions under `google3/third_party/jetski/` | 51,750 (≈30%) |
| gRPC/Connect services (≥4 methods) | 44 |
| distinct (service, method) pairs | 1,036 |
| RPC implementation files (`language_server/rpcs_*.go`) | 51 |

Recovered from generated client/server method names in pclntab (`data/rpc-services.txt`,
full list in `data/rpc_methods.txt`), which is reliable. Largest services:

| methods | service |
|---|---|
| 359 | `LanguageServerService` |
| 115 | `ApiServerService` |
| 97 | `SeatManagementService` |
| 80 | `JetskiRemoteAgentProxyService` |
| 54 | `ExtensionServerService` |
| 37 | `PredictionService` |
| 22 | `JetskiService` |
| 21 | `KnowledgeBaseService` |
| 20 | `IndexManagementService`, `ApiService` |
| 15 | `TaskService` |
| 10 | `SecretService` |
| 9 | `InteractionService` |
| 8 | `AnalyticsService`, `ConversationService`, `ManagementService`, `RemotingService` |

These are the **vendored** surface. Most are never called by the CLI — 04-api separates
live from dead. `LanguageServerService` is the local one (the CLI talks to it); the
`…Service` entries are what the language server talks upstream.

The remaining ~70% is Go stdlib, `boringcrypto`, vendored third-party
(`connectrpc.com/connect`, `charmbracelet/lipgloss`, `openai/openai-go/v3`,
`google.golang.org/genai`, `zalando/go-keyring`, `hashicorp/retryablehttp`,
`go-yaml/yaml`, `bluemonday`, `glamour`, `cron`, `dbus`, `pion/webrtc`,
`quic-go`, `gogo/protobuf`, `gorm`/`sqlite`, `opentelemetry`) and generated protobuf
code.

## 3. Component map

Function counts per subtree, derived from pclntab names. Counts are a proxy for
surface area, not importance.

```
┌──────────────────────────────────────────────────────────────────────────┐
│ cli/                                                          6,186 fns  │
│   TUI (bubbletea-style model/), render, layout, theme, viewport,         │
│   commands, entrypoints, headless, printmode, editing, keybindings,      │
│   installer, updater, store, backend, analytics, plugins, mcp,           │
│   remotecontrol, changelog, audio/micserve, latex, mermaid, logo         │
│   └─ backend/auth  ← login, token storage, keyring, ADC, gateway         │
│   └─ store/quota_manager  ← quota refresh + throttling                   │
├──────────────────────────────────────────────────────────────────────────┤
│ language_server/                                              4,871 fns  │
│   server, rpcs_* (51 RPC implementation files), auth_client,            │
│   code_assist_client, modelapiccpa, modelapieval, backend/{local,        │
│   cloudproxy}, state_sync, search, interceptor/mockdata, vcsdriver,      │
│   tokenizer, plugin_data, enterprise, admin_controls                     │
├──────────────────────────────────────────────────────────────────────────┤
│ cortex/                                                       9,052 fns  │
│   CascadeManager, core/contrib/variants/* (model_router,                 │
│   model_router_plan, model_router_advisor, model_router_alt,             │
│   native_swarm), policyguardian/{vetting,telemetry},                     │
│   battlemode, battleworkspace, customizations, workspaces,               │
│   command/sandboxproxy, rehydration, modelapi, utils                     │
├──────────────────────────────────────────────────────────────────────────┤
│ generated protobuf                                            31,000 fns │
│   language_server_pb 9,438 · cortex_pb 5,959 · codeium_common_pb 3,550   │
│   api_server_pb 2,077 · seat_management_pb 1,833 · extension_server_pb    │
│   941 · opensearch_clients_pb 613 · index_pb 595 · chat_pb 394 ·          │
│   jetbox_state_pb 325 · project_pb 282 · hooks_pb 209 · product_api_pb    │
│   183 · context_module_pb 178 · vcs_pb 175 · analytics_pb 155 ·           │
│   remoting_pb 146 · plugin_context_pb 140 · …                            │
├──────────────────────────────────────────────────────────────────────────┤
│ fs 465 · go_utils 274 · code_edit 228 · enterprise 186 · terminal 181 ·   │
│ remoting 133 · radix 341 · models/{model_config,defaults,geminimodels}    │
│ model_api/ · gateway/ · tokenizer/ · scm_utils/ · admin_controls/         │
└──────────────────────────────────────────────────────────────────────────┘
```

The important structural fact: there are **three concentric layers**, and the CLI is
not the thing that talks to Google.

```
  ┌─────────────────────────────────────────────────────────────────┐
  │  agy (single binary, PIE)                                       │
  │                                                                 │
  │  ┌──────────────┐   Connect/HTTP over loopback                  │
  │  │  cli/        │───────────────────────────┐                   │
  │  │  TUI+store   │                           │                   │
  │  │  backend/    │◄──────────────────────────┤                   │
  │  └──────────────┘                           ▼                   │
  │                                    ┌──────────────────┐         │
  │         (separate process)         │ language_server/ │         │
  │                                    │  server + rpcs   │         │
  │                                    │  backend/{local, │         │
  │                                    │   cloudproxy}    │         │
  │                                    └────────┬─────────┘         │
  │                                             │                   │
  │                                    ┌────────▼─────────┐         │
  │                                    │ cortex/          │         │
  │                                    │  CascadeManager  │         │
  │                                    └────────┬─────────┘         │
  │                                             │                   │
  │                                    ┌────────▼─────────┐         │
  │                                    │ model_api/       │         │
  │                                    │  code_assist_    │         │
  │                                    │  client          │         │
  │                                    └────────┬─────────┘         │
  └─────────────────────────────────────────────┼───────────────────┘
                                                │ TLS + OAuth
                                                ▼
                       daily-cloudcode-pa.googleapis.com
                       antigravity-unleash.goog
                       businessaicode / aiplatform (enterprise modes)
```

The CLI does **not** hold a Google credential per request. It drives a language server
that owns the model API client, which owns the token source. This is why the klog line
`error getting token source: You are not logged into Antigravity` appears in
`loadCodeAssistResponse`, `userInfo`, `peopleInfo` and `ListExperiments` refreshes
simultaneously — all four are downstream of one shared token source.

Evidence for the child-process model (confirmed from klog, not inferred):

```
server.go:1836] Starting language server process with pid 2749386
server.go:1892] Language server version: 1.4.8
server.go:751 ] Language server will attempt to listen on host localhost
server.go:766 ] Language server listening on random port at 37009 for HTTPS (gRPC)
server.go:774 ] Language server listening on random port at 40897 for HTTP
server.go:3005] initialized server successfully in 224.339966ms
```

So the `agy` process is a supervisor + TUI that spawns a **separate language-server
process** on two ephemeral loopback ports — one gRPC-over-TLS, one plain HTTP — and
connects to it over Connect. `ANTIGRAVITY_LS_ADDRESS` carries that address. Supporting
symbols: `cli/backend/language_server.go`, `Exit requested on language server process %d`,
`LS Config: %+v`, the literals `127.0.0.1:%d` / `localhost:%d` / `inprocess`, and the
`--headless` mode which presumably runs the server half alone.

## 4. Data flow of one user turn (high level)

Detailed in 06-mechanics; the shape is:

```
prompt
  → cli/model  (TUI input, slash commands, at-mentions, skills)
  → cli/backend  (Connect call to local LanguageServer)
  → language_server/rpcs_*  (CreateInteraction / SendUserCascadeMessage)
  → cortex.CascadeManager  (trajectory steps, tool loop)
  → model_api / code_assist_client  (prompt assembly, model call)
  → daily-cloudcode-pa.googleapis.com
  → streaming response → steps → CLI render
```

A "cascade" is a conversation run; a "trajectory" is its persisted step list. Steps are
typed (`CORTEX_STEP_TYPE_*`: CODE_ACTION, GREP_SEARCH, RUN_COMMAND, CODE_SEARCH,
NOTIFY_USER, TOOL_SEARCH, FILE_CHANGE, LINT_APPLET, CLIPBOARD, LINT_DIFF, VIEW_FILE,
BROWSER_*, MOMA, FIND, WAIT, MOVE, CAPTURE_BROWSER_CONSOLE_LOGS, …).

## 5. Surface inventory

- **CLI**: full-screen TUI (`--alt-screen`-style by default; `settings.json`
  `altScreenMode: always`), plus `--print` headless mode with
  `text|json|stream-json` output and NDJSON input.
- **Agents**: `--agent`, custom agents under `cortex/customizations/builtin/agents/`.
- **Modes**: `plan`, `auto`, `accept-edits`, plus `--sandbox` and
  `--dangerously-skip-permissions`.
- **Extensibility**: MCP servers, plugins (including **Claude Code and Gemini CLI
  importers** — `cli/plugins/claude`, `cli/plugins/gemini`, and the literal
  `this is Claude Code's file name. Move the servers`), skills, hooks, sidecars,
  marketplaces.
- **Remote**: `--remote-control` daemon, `mic-serve`, `remote_agent_proxy_pb`.
- **Enterprise**: `enterprise/` package, `gateway/` (custom CA, LLM gateway URL),
  admin controls, WIF/ADC auth.

## 6. Document map

| file | contents |
|---|---|
| `00-context.md` | analysis metadata, symbol-recovery toolkit, established facts |
| `01-overview.md` | this file — identity, architecture, component map |
| `02-auth.md` | login, OAuth flow, auth methods, token storage, refresh |
| `03-models.md` | model catalogue, id grammar, thinking levels, capabilities |
| `04-api.md` | endpoints, transports, inference path, telemetry, feature flags |
| `05-quota.md` | quota model, limits, resets, credits, tiers, seats |
| `06-mechanics.md` | turn loop, tools, permissions, sandbox, subagents, extensions, persistence |
| `07-install-and-config.md` | installer, updater, config/state layout, env vars |
| `08-integration-notes.md` | what a reimplementation must reproduce, risks, open questions |
| `data/` | recovered inventories (function names, RPCs, packages, source files) |
| `tools/goresym.py` | the symbol/string recovery tool used throughout |
