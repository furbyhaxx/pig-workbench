---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.8-03-models"
  title: "agy 1.4.8 model catalogue and selection"
  description: "Model ids, thinking-level grammar, dynamic versus static catalogue, capabilities and context-window mechanism"
  status: "published"
  revision: 3
  created_at: "2026-10-27T09:59:34+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "pi"
  updated_by: "orchestrator"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/antigravity-cli/1.4.8/03-models.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 1
      at: "2026-10-27T09:59:34+01:00"
      note: "Initial analysis of agy 1.4.8."
    - revision: 2
      at: "2026-10-27T10:14:00+01:00"
      note: "Added the observed live catalogue: real context windows, output caps and thinking budgets for all 13 agent-facing models, server-reported apiProvider values, the 26-returned-vs-13-offered split, and the finding that an untouched bucket's resetTime re-anchors on every read."
    - revision: 3
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/antigravity-cli/1.4.8/03-models.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---

# 03 — Model catalogue and selection

## 1. Live catalogue (observed)

`agy models` performs an authenticated fetch and prints the catalogue. Output at the
time of analysis (streams to stderr: `Fetching available models...`):

| model id | display name |
|---|---|
| `gemini-4-argon-low` | Gemini 4 Argon (Low) |
| `gemini-4-argon-medium` | Gemini 4 Argon (Medium) |
| `gemini-4-argon-high` | Gemini 4 Argon (High) |
| `gemini-3.8-flash-low` | Gemini 3.8 Flash (Low) |
| `gemini-3.8-flash-medium` | Gemini 3.8 Flash (Medium) |
| `gemini-3.8-flash-high` | Gemini 3.8 Flash (High) |
| `claude-opus-5-5-low` | Claude Opus 5.5 (Low) |
| `claude-opus-5-5-medium` | Claude Opus 5.5 (Medium) |
| `claude-opus-5-5-high` | Claude Opus 5.5 (High) |
| `claude-sonnet-5-5-low` | Claude Sonnet 5.5 (Low) |
| `claude-sonnet-5-5-medium` | Claude Sonnet 5.5 (Medium) |
| `claude-sonnet-5-5-high` | Claude Sonnet 5.5 (High) |
| `gpt-oss-120b-medium` | GPT-OSS 120B (Medium) |

Three families, four vendors' worth of models, all served under one vendor-neutral id
scheme. Note that GPT-OSS 120B exposes only `medium`.

## 2. Identifier grammar

```
<family>-<thinking-level>
   │            └─ low | medium | high           (what the live catalogue exposes)
   └─ gemini-4-argon | gemini-3.8-flash | claude-opus-5-5 | claude-sonnet-5-5 | gpt-oss-120b
```

The suffix is a **thinking level baked into the model id**, not a separate request
parameter the caller sets. The wire enum behind it (`THINKING_LEVEL_*`) has more
values than the catalogue currently surfaces:

```
THINKING_LEVEL_UNSPECIFIED
THINKING_LEVEL_MINIMAL
THINKING_LEVEL_LOW
THINKING_LEVEL_MEDIUM
THINKING_LEVEL_HIGH
THINKING_LEVEL_MODEL_DEFAULT
```

So a model id can in principle carry `minimal` or `model-default`, and the catalogue
is a server-side subset. Display labels are abbreviated internally (`Lo`, `Med`,
`High`).

`--effort low|medium|high|xhigh|max` is the session-level control from `agy --help`.
It reaches the wire as a **generation-config** value, not as its own request field:
`thinkingLevelToEffort` sits inside the Anthropic converter's `applyGenerationConfig`
(04-api §7.3, listed alongside `setJSONModeIfResponseSchema` and `fixSchemaInt64Strings`).

`xhigh` and `max` have no matching `THINKING_LEVEL_*` value, so **derived**: they are a
CLI-side scale that `thinkingLevelToEffort` maps down onto the wire enum rather than being
forwarded verbatim. The concrete value table is still unrecovered.

Two axes, easy to conflate: the **model id** already carries a thinking level
(`gemini-4-argon-medium`), while `--effort` is a separate per-session dial applied through
generation config — it can change without renaming the model.

## 3. Static fallback vs. dynamic catalogue

Two sources, in priority order:

1. **`v1internal:fetchAvailableModels`** — the authenticated dynamic list that `agy
   models` prints. Gated on auth mode: klog shows
   `model_configs.go:350] Auth mode is unspecified, skipping fetchAvailableModels and
   returning empty response`, i.e. no auth ⇒ no catalogue.
2. **Static compiled catalogue** — `models/defaults` (`defaultmodels.init`,
   `GetModelInfosWithOverride`) and `models/geminimodels` (`LookupModelInfo`,
   `extractSlug`, `map.init.0/1`). `gemini-4-argon-low` appears as a literal here, so
   the binary ships a usable fallback.

`models/model_config` provides the resolution layer:

| function | role |
|---|---|
| `GlobalCatalog` / `globalCatalog.GetModelInfo` | process-wide resolved catalogue |
| `NewLegacyStaticCatalog` / `staticCatalog.*` | compiled-in catalogue |
| `GetModelInfoFromKey` / `GetModelInfoFromKeyWithFallback` | id → info, with fallback |
| `GetModelInfoFromNameWithContext` | name → info, experiment-aware |
| `GetModelInfoMapWithContext` | whole map for the UI |
| `ModelDetailsToModelInfo` | wire details → internal info |
| `InferAPIProviderFromModelName` | id → API provider key (e.g. `claude` → `/anthropic/`) |
| `DefaultChatModelConfig` | default model when none is selected |
| `cloneModelInfos` | deep copy for per-context overrides |

Legacy slugs still present as literals, i.e. recognised but not currently listed:
`gemini-3.1-pro-*`, `gemini-3.1-flash-*`, `gemini-3.1-flash-lite-preview`,
`gemini-2.5-flash-*`. Treat these as backward-compatible parsing, not available models.

## 4. Capabilities

| function | meaning |
|---|---|
| `SupportsImages` / `SupportsVideo` | modality gates |
| `IsSupportedMimeType` | accepted attachment types |
| `ModelExperiments` | per-model experiment assignment |
| `WithAllowBYOM` | bring-your-own-model opt-in flag |

MIME types accepted by the model layer (recovered literals): `image/png`, `image/jpeg`,
`image/webp`, `image/heic`, `image/heif`, `image/avif`, `image/apng`, `image/tiff`,
`audio/flac`, `audio/mpeg`, `audio/opus`, `audio/wave`, `video/webm`, `video/mp4`,
`application/pdf`, and the Office set — `.docx`, `.xlsx`, `.pptx`, `.doc`, `.xls`, `.ppt`.

## 5. Context window

Context sizing is dynamic, not a constant per model:

- `models/model_config/context_window.go`, `ContextWindowOptions`,
  `GetModelInfoMapWithContext`.
- Driven by experiment keys `context-window-options` and `context-window_<…>` — the
  observed literals are `context-window-options` and a `context-window_` prefix, so
  windows are experiment-assigned per model.
- The CLI renders a context indicator (`cli/model/context.go`,
  `model_selector_context_window.go`) and can auto-compact
  (`AGY_CLI_AUTO_COMPACT_OVERRIDE`, `cli/model/compaction.go`).

**Consequence for a reimplementation:** do not hardcode a context window per model id.
Read it from the model catalogue response, and honour the experiment override if you
want parity.

## 6. Model routing and grouping

- `models/model_config/model_groups.go` groups models for the selector UI.
- `InferAPIProviderFromModelName` maps a model id to an API-provider key; `claude*`
  maps to `/anthropic/`. This key is what selects the request/response converter —
  the Code Assist path carries
  `cloud/developer_experience/codeassist/shared/converters/anthropic/anthropicconverter`.
  See 04-api for whether that is a client-side shape adapter or a proxy.
- `cortex/modelutils/model_tier.go` and `MODEL_TIER_INHERIT` implement model *tiers*
  (a cheap/strong split) so a subagent can inherit its parent's tier rather than a
  concrete id.
- `MODEL_PRICING_TYPE_BYOK` + `WithAllowBYOM` indicate a bring-your-own-key pricing
  mode where the model is billed to the user's own credentials instead of the
  subscription. See 05-quota for how that interacts with entitlement.

### Vendor routing, resolved

Every family is served by Google; nothing is called vendor-direct (04-api §7.3):

| family | wire | destination | id on the wire |
|---|---|---|---|
| Gemini 4 Argon / 3.8 Flash | GenAI `GenerateContentRequest` | Cloud Code PA `v1internal` | same as the CLI id |
| Claude Opus / Sonnet 5.5 | Anthropic Messages (`anthropic_version vertex-2023-10-16`) | Vertex AI `publishers/anthropic/models/{model}:streamRawPredict` | publisher model name |
| GPT-OSS 120B | OpenAI chat-completions | Vertex MaaS `endpoints/openapi/chat/completions` | `gpt-oss-120b-maas` |

The `-maas` suffix is Vertex's model-as-a-service naming; the CLI's `gpt-oss-120b-medium`
is mapped onto it. All three carry the user's Google OAuth token, and
`SetCacheControlForJetski` sets Anthropic `cache_control` markers so the *Google* backend
does the prompt caching — which is what rules out a direct Anthropic call.

## 7. Session-level selection

| surface | effect |
|---|---|
| `--model <id>` | pin the model for the session |
| `--effort <level>` | reasoning effort for the session |
| `--agent <name>` | pick an agent definition (which may bundle its own model) |
| `/model` | interactive switcher (per help text) |
| `REMEMBER_LAST_MODEL_SELECTION_*` | setting that persists the last choice |

Model choice is stored as a setting (`REMEMBER_LAST_MODEL_SELECTION_DISABLED` is a
recognised value) and surfaced in the statusline.

## Observed live: the real catalogue (2026-10-27)

Captured from `v1internal:fetchAvailableModels` — 26 models, 201,647 bytes, HTTP 200. Full
body in [`evidence/live-2026-10-27/fetchAvailableModels.json`](evidence/live-2026-10-27/fetchAvailableModels.json).
This replaces the "server-assigned, unknown" placeholders with real numbers.

### The 13 agent-facing models

| model id | displayName | ctx | maxOut | minThink | apiProvider |
|---|---|---|---|---|---|
| `gemini-4-argon-low` | Gemini 4 Argon (Low) | 1048576 | 65536 | 32 | `API_PROVIDER_GOOGLE_GEMINI` |
| `gemini-4-argon-medium` | Gemini 4 Argon (Medium) | 1048576 | 65536 | 32 | `API_PROVIDER_GOOGLE_GEMINI` |
| `gemini-4-argon-high` | Gemini 4 Argon (High) | 1048576 | 65536 | 32 | `API_PROVIDER_GOOGLE_GEMINI` |
| `gemini-3.8-flash-low` | Gemini 3.8 Flash (Low) | 1048576 | 65536 | 32 | `API_PROVIDER_GOOGLE_GEMINI` |
| `gemini-3.8-flash-medium` | Gemini 3.8 Flash (Medium) | 1048576 | 65536 | 32 | `API_PROVIDER_GOOGLE_GEMINI` |
| `gemini-3.8-flash-high` | Gemini 3.8 Flash (High) | 1048576 | 65536 | 32 | `API_PROVIDER_GOOGLE_GEMINI` |
| `claude-opus-5-5-low` | Claude Opus 5.5 (Low) | 1000000 | 128000 | — | `API_PROVIDER_ANTHROPIC_VERTEX` |
| `claude-opus-5-5-medium` | Claude Opus 5.5 (Medium) | 1000000 | 128000 | — | `API_PROVIDER_ANTHROPIC_VERTEX` |
| `claude-opus-5-5-high` | Claude Opus 5.5 (High) | 1000000 | 128000 | — | `API_PROVIDER_ANTHROPIC_VERTEX` |
| `claude-sonnet-5-5-low` | Claude Sonnet 5.5 (Low) | 1000000 | 128000 | — | `API_PROVIDER_ANTHROPIC_VERTEX` |
| `claude-sonnet-5-5-medium` | Claude Sonnet 5.5 (Medium) | 1000000 | 128000 | — | `API_PROVIDER_ANTHROPIC_VERTEX` |
| `claude-sonnet-5-5-high` | Claude Sonnet 5.5 (High) | 1000000 | 128000 | — | `API_PROVIDER_ANTHROPIC_VERTEX` |
| `gpt-oss-120b-medium` | GPT-OSS 120B (Medium) | 131072 | 32768 | — | `API_PROVIDER_OPENAI_VERTEX` |

`apiProvider` is server-reported and independently confirms the routing in
[`04-api.md`](04-api.md) §7.3: Claude is `ANTHROPIC_VERTEX`, GPT-OSS is `OPENAI_VERTEX`, and
neither is a vendor-direct endpoint. `modelProvider` is `MODEL_PROVIDER_GOOGLE` throughout,
even for Anthropic and OpenAI weights.

### What the payload says that the picker hides

| field | value | meaning |
|---|---|---|
| `defaultAgentModelId` | `gemini-4-argon-medium` | the default model, not merely "first in list" |
| `commandModelIds` | `[gemini-3-flash]` | slash-command generation |
| `commitMessageModelIds` | `[gemini-3.5-flash-lite]` | commit messages |
| `audioTranscriptionModelIds` | `[models/proactive-observer-v10]` | audio → text |
| `tabModelIds` | `tab_flash_lite_preview`, `tab_jump_flash_lite_preview` | 16384 ctx / 4096 out — editor completion |
| `imageGenerationModelIds`, `webSearchModelIds`, `mqueryModelIds`, `tieredModelIds` | model id lists | auxiliary roles |
| `experimentIds` | list | experiment assignment |

The catalogue also ships 13 models the agent picker never shows: `gemini-2.5-flash`,
`gemini-2.5-pro`, `gemini-2.5-flash-thinking`, `gemini-2.5-flash-lite`, `gemini-3-flash`,
`gemini-3.5-flash-lite`, `gemini-3.1-flash-lite`, `gemini-3.1-flash-image`,
`gemini-3.8-flash-tiered`, the two `tab_*` previews, and two `API_PROVIDER_INTERNAL`
entries (`chat_20706` 16384 ctx, `chat_23310` 32768 ctx). **26 returned, 13 offered** — a
client that renders the raw list will expose internal and auxiliary models as if they were
user-selectable.

### Per-model `quotaInfo` and a surprise about untouched buckets

Every agent-facing model carries `quotaInfo { remainingFraction, resetTime }`, and the value
matches its bucket from [`05-quota.md`](05-quota.md) §4.1 — Gemini models share the
`gemini-5h` reset, Claude and GPT-OSS share `3p-5h`, Argon has its own. So the bucket → model
mapping is server-side and needs no client table.

The surprise: the Argon `resetTime` **moved between two reads 102 seconds apart**.

| read | at | Argon `resetTime` | delta from read |
|---|---|---|---|
| `retrieveUserQuotaSummary` | `09:06:53Z` | `2026-11-03T09:06:53Z` | exactly 7 days |
| `fetchAvailableModels` | `09:08:36Z` | `2026-11-03T09:08:36Z` | exactly 7 days |

With `remainingFraction: 1` in both, this is a bucket with no usage yet whose window is
anchored to the read. Do not cache it as a deadline and do not derive a window start from it:
for an untouched bucket the countdown restarts every time you look. Render the returned value
only while it is fresh.

## Open questions

- The concrete value table behind `thinkingLevelToEffort` (which CLI level maps to which
  wire level). Function and call site are established (04-api §7.3); only the mapping is
  missing. `xhigh`/`max` have no `THINKING_LEVEL_*` counterpart, so they are certainly
  translated rather than forwarded.
- Real context-window values per family — **resolved live**, see the catalogue table above:
  Gemini 1,048,576 / 65,536 out, Claude 1,000,000 / 128,000 out, GPT-OSS 131,072 / 32,768 out.
