---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.8-04-api"
  title: "agy 1.4.8 network API surface and inference plumbing"
  description: "Endpoint taxonomy, transport, base-URL selection, request envelope, multi-vendor routing, telemetry and feature flags"
  status: "published"
  revision: 5
  created_at: "2026-10-27T09:47:48+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "pi"
  updated_by: "orchestrator"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/antigravity-cli/1.4.8/04-api.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 1
      at: "2026-10-27T09:47:48+01:00"
      note: "Initial extraction."
    - revision: 2
      at: "2026-10-27T09:52:10+01:00"
      note: "Resolved the telemetry opt-out question: data_collection_optin exists at three layers plus two admin force fields (§9.4, open question 6)."
    - revision: 3
      at: "2026-10-27T10:16:00+01:00"
      note: "Added the observed live header capture: exact User-Agent with its five parameters (including the release-side cl that differs from the build), the real response header set, the confirmation that the wire is protojson over an ESF front end, and the finding that no quota or rate-limit headers exist."
    - revision: 4
      at: "2026-10-27T10:24:00+01:00"
      note: "Observed the inference call behind a local TLS proxy: Authorization is Bearer, the complete request head is seven headers, and every x-goog-* header the code suggested is absent. Also established that a turn sends Clearcut to play.googleapis.com and that aicode.googleapis.com serves fetchConfig. Closes open questions 5 and most of 2."
    - revision: 5
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/antigravity-cli/1.4.8/04-api.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---

# agy 1.4.8 — network API surface and model-inference plumbing

status: partially explained
confidence: high (inference path, transports, routing switch); medium (base-URL channel selection, telemetry wiring); low (Sentry)

Everything below is grounded in recovered bytes (`tools/goresym.py` `strs`/`disas`,
`tools/xwalk.py` string→function cross-reference over `.text`), in the embedded protobuf
descriptors, or in klog output under `~/.gemini/antigravity-cli/log/`. Claims I could not
ground are marked **UNRESOLVED** and collected in the last section.

---

## 1. Process model: the CLI is not the thing that talks to Google

The `agy` binary contains both halves of the product. It spawns itself as a child language
server and drives it over a loopback gRPC socket.

Observed (klog):

```
server.go:305]  Creating CLI server backend: product=antigravity workspaceDirs=[...]
                appDataDir=/home/furbyhaxx/.gemini/antigravity-cli languageServer=true codeAssist=true
server.go:751]  Language server will attempt to listen on host localhost
server.go:766]  Language server listening on random port at 37009 for HTTPS (gRPC)
server.go:774]  Language server listening on random port at 40897 for HTTP
server.go:1836] Starting language server process with pid 2749386
server.go:1892] Language server version: 1.4.8
server.go:3005] initialized server successfully in 224ms
```

Two loopback listeners per language-server child:

| listener | protocol | what it carries |
|---|---|---|
| `HTTPS (gRPC)`, random port | TLS + gRPC/Connect | the full `exa.language_server_pb.LanguageServerService` surface (prompts, cascade streams, conversations, terminal, MCP, auth) |
| `HTTP`, random port | plain HTTP | unauthenticated-ish sidecar endpoints: `/healthz`, `/metrics`, `/debug/pprof/*`, `/static/*`, `/artifacts`, `/proxy/unleash`, `/clearcut_proxy` |

Evidence for the routes: rodata at `0x58f8361` (`/healthz`, `/metrics`, `/debug/pprof/trace`,
`/static/`, `/artifacts`), `0x59a4547` (`/proxy/unleash`, `/clearcut_proxy`),
`language_server.(*Server).handleUnleashProxy` (LEA to `/proxy/unleash`).

The CLI is a **gRPC client** of the child: `language_server_go_grpc.(*languageServerServiceClient)`
has 358 methods (`AcceptTermsOfService` … `UpdateCascadeMemory`). The child serves the same
surface through connect-go handlers (`language_server_go_proto_connect.NewLanguageServerServiceHandler`,
358 methods), whose gRPC wire mode is why the port is labelled "HTTPS (gRPC)".

`ANTIGRAVITY_LS_ADDRESS` is **not** how the CLI learns the port. It is the *inverse*: the CLI
exports it, together with `ANTIGRAVITY_CSRF_TOKEN` and `ANTIGRAVITY_LS_VERSION`, into child
processes and shell snippets that want to call the language server directly. Evidence: the
three names are adjacent in one literal (`0x5a29f65`) and are referenced by
`cmd/agentapi.HandleAgentAPI`, `cortex/utils.GetAgentAPIShellInjection` and
`plugincli.withSignInLink`; the embedded shell templates hardcode the *fallback*
(`0x5ec3c8a` `ANTIGRAVITY_LS_ADDRESS:=localhost:5387}`, `0x5ec3de4` same,
`0x5ec3e63` `$ANTIGRAVITY_LS_ADDRESS/exa.language_server_pb.LanguageServerService/$1`). So
`localhost:5387` is a fixed port for the daemonised (`agy` daemon) mode; the ephemeral ports
above are the per-session child.

---

## 2. Endpoint taxonomy — 30 services, reconciled

`data/rpc-services.txt` (regenerated by another agent at 09:45) lists 1036 `Service/Method`
entries over 30 real service names. The naive reading — "the CLI calls 30 services" — is wrong
for a structural reason: this is one binary that is simultaneously the *client* (CLI), the
*server* (language server, `LanguageServerService` + `RemotingService` + `ConversationService`
+ `HostBridgeService` + `DevService` + `InteractionService`), and it links Google's generated
`*_go_proto` / `*_go_grpc` / `*_proto_connect` packages for a whole product family, many of
which are only needed by the IDE/webview or by the server side of the protocol.

Evidence that the bulk is server-side generated code and not client code: for every service
except `LanguageServerService`, `ExtensionServerService` and `HostBridgeService` the only
functions in the binary are `_<Service>_<Method>_Handler` / `_Handler.func1` (connect handler
adapters), `Unimplemented<Service>Server` method sets, and message codecs. There is no
`*ServiceClient` type and no `New*ServiceClient` constructor.

### 2.1 What actually crosses the network, and from where

| # | service (proto package) | methods | client code in binary | verdict |
|---|---|---|---|---|
| 1 | `exa.language_server_pb.LanguageServerService` | 359 | `language_server_go_grpc.(*languageServerServiceClient)`, 358 methods | **LIVE** — CLI → child LS, loopback gRPC |
| 2 | `google.internal.cloud.code.v1internal.JetskiService` | 22 | hand-rolled REST: `(*CodeAssistClient).constructJetskiEndpoint*` (`%s/%s:%s`), `jetskiServiceEndpointURL` | **LIVE** — LS → Cloud Code PA |
| 3 | `google.internal.cloud.code.v1internal.PredictionService` | 5 (+ the same routes surface in the 37-name bucket) | `(*CodeAssistClient).streamGenerateContentFromCCPA`, `fetchAvailableModels`, `retrieveUserQuotaSummary` | **LIVE** — LS → Cloud Code PA |
| 4 | `google.gca.aicode.v1main.*` (`JetskiRemoteAgentProxyService` 80, `PredictionService`, `ManagementService`) | 80+ | `(*CodeAssistClient).StreamGenerateContentFromAICode`, `ListRemoteAgent*`, `AICODE_ENDPOINT_URL` | **LIVE (conditional)** — consumer entitlement path, `https://aicode.googleapis.com` |
| 5 | `google.cloud.businessaicode.v1beta.*` (`PredictionService`, `ManagementService`, `TelemetryService`) | 8+ | `StreamGenerateContentFromBusinessAICode`, `SendBAICTelemetry`, `BAICODE_*_ENDPOINT_URL` | **LIVE (conditional)** — enterprise entitlement path |
| 6 | `google.cloud.aiplatform.v1beta1.PredictionService` (+ `.master.PredictionService`) | 17+13 | `(*CodeAssistClient).StreamGenerateContentFromVertexAI`, `vertexURLPath`, `modelEndpoint` | **LIVE (conditional)** — Vertex AI 3P publisher path |
| 7 | `google.cloud.aiplatform.v1beta1.TaskService` | 12 | `enterprise.(*SobiHarness).TaskServiceClient` (`task_service_grpc_go_proto.(*taskServiceClient)`, 14 methods) | **LIVE** — enterprise harness, gRPC to aiplatform |
| 8 | `google.cloud.aiplatform.v1beta1.AgentService` | 6 | `agent_service_grpc_go_proto.(*agentServiceClient)` | **LIVE (derived)** — client type present with methods, no caller found |
| 9 | `exa.extension_server_pb.ExtensionServerService` | 54 | `extension_server_go_proto_connect.(*extensionServerServiceClient)`, 53 methods | **LIVE for the IDE**, unused by the CLI path |
| 10 | `exa.host_bridge_pb.HostBridgeService` | 3 (4 in list) | `host_bridge_go_proto_connect.(*hostBridgeServiceClient)` | **LIVE for the IDE** (`GetOAuthToken`, `GetUpdateStatus`, `ApplyUpdate`) |
| 11 | `exa.plugin_context_pb.ContextProviderService` | 3 | `plugin_context_go_proto_stdiorpc.(*contextProviderServiceClient)` | **LIVE, not network** — stdio JSON-RPC to plugin child processes |
| 12 | `exa.remoting.RemotingService` | 8 | connect server only (`NewRemotingServiceHandler`) | **LIVE as server** (child LS serves it); no client |
| 13 | `jetski.product.v1.ConversationService` | 6 | `product_api.(*ConversationService)` implementation + connect server | **LIVE as server**; no client |
| 14 | `exa.dev_pb.DevService` | 2 | connect server (`NewDevServiceHandler`) | **LIVE as server** (debug surface) |
| 15 | `learning.genai.api.interactions.InteractionService` | 9 | connect server (`InteractionServiceHandler`) | **LIVE as server** inside the vendored genai SDK; not called by the CLI |
| 16 | `exa.api_server_pb.ApiServerService` | 115 | **no client, no caller**; only `UnimplementedApiServerServiceServer` (229 methods) and message types | **message types LIVE as internal DTOs, service DEAD** |
| 17 | `exa.seat_management_pb.SeatManagementService` | 97 | no client stub; named `(*CodeAssistClient)` entries (`GetUserStatus`, `RegisterGdmUser`, `SelfAssignLicense`, `MigrateApiKey`) go through hand-rolled REST | **partially live** — the LS-side helpers are named but I did not recover their routes (**UNRESOLVED**) |
| 18 | `exa.analytics_pb.AnalyticsService` | 8 | no client stub; handlers only. Actual analytics egress uses `v1internal:recordClientEvent` / `:recordTrajectoryAnalytics` | **DEAD as a service**; the *concept* is live via v1internal |
| 19 | `exa.opensearch_clients_pb.KnowledgeBaseService` / `CodeIndexService` | 21 / 5 | handlers only | **DEAD surface** (enterprise/self-hosted SKU) |
| 20 | `jetski.index_pb.IndexManagementService` / `IndexService` | 20 / 5 | handlers only | **DEAD surface** (indexing SKU) |
| 21 | `exa.cascade_plugins_pb.CascadePluginsService` | 4 | handlers only | **DEAD surface** |
| 22 | `devtools.jetski.boq.api.proto.ApiService` | 20 | handlers only | **DEAD surface** (boq = remote sandbox mesh proxy) |
| 23 | `google.cloud.businessaicode.v1beta.TelemetryService` | 1 (`SendTelemetry`) | handler only, but the *caller* exists as `(*CodeAssistClient).SendBAICTelemetry` building `/%s/projects/%s/locations/%s:sendTelemetry` | **LIVE** (hand-rolled, not the generated stub) |
| 24 | `exa.api_server_pb.EvalApiServerService`, `exa.chat_pb.ChatClientServerService` | 3 / 3 | `Unimplemented*` only | **DEAD surface** |
| 25 | `com.google.cloud.aiplatform.v1beta1.RagRetrievalConfig_Ranking_RankService`, `ServiceTier`, `SecretService` | small | `SecretService` is `zalando/go-keyring` D-Bus, not HTTP; the rest are enum holders | **not network** |

### 2.2 The `mockdata` package is *not* the dead-surface signal the brief assumed

`language_server/interceptor/mockdata` (454 functions, 12 `mock_*_rpcs.go` files) is a
**connect interceptor over `LanguageServerService`**, not over any remote service. Its
handlers are LS RPCs: `handleGetUserStatus`, `handleGetAuthStatus`, `handleListTerminals`,
`handleGetCascadeTrajectory`, `handleGetCodeActionDiff`, `handleGetMarketplaceFeatured`,
`handleGetMendelFlags`, `handleListContextProviders`, `mockListCloudPlugins`,
`mockListPluginAuthProviders`, `applyMockSubagent`, plus a `TestControl` handler. Its purpose
is to serve synthetic sessions (demo / recorded fixtures / `TestControl`) instead of a live
agent, so it says nothing about which *remote* service is dead. Use it as evidence for which
LS RPCs a demo session must satisfy, not for remote liveness.

The reliable dead-surface signal is instead: (a) no `*ServiceClient` type, (b) no
application-package function referencing a route string, (c) presence only as
`_Handler`/`Unimplemented*Server`.

---

## 3. Transport

Three transports, cleanly separated:

1. **gRPC over TLS, loopback** — CLI → child language server. Client:
   `language_server_go_grpc`. Wire: gRPC (`connect` on the server side accepts gRPC too).
   Interceptors: `requestIDInterceptor`, `traceTaskInterceptor`, `rpcTelemetryInterceptor`,
   `CsrfInterceptor`, `hostClientIdentityInterceptor`.
2. **HTTPS + JSON, hand-rolled `net/http`** — LS → Google. There are **no generated gRPC
   clients** for the Cloud Code / AICode / BAICode APIs; `*_go_proto` is used for *messages*
   only, and the request is `json.Marshal`ed and POSTed. Evidence:
   `(*CodeAssistClient).postCodeAssistBackendRequest`, `postHTTPRequest`,
   `postVertexAIRequest`, `doPostAndUnmarshal[T]`, `doJetskiRequestAndUnmarshal[T]`,
   `doGetAndUnmarshal[T]`, and the header constant `Content-Type: application/json` in
   `(*CodeAssistClient).setHeaders`.
3. **gRPC, remote** — only the Vertex aiplatform clients (`taskServiceClient`,
   `agentServiceClient`) and `s2a_go_grpc_proto.(*s2AServiceClient)` (S2A transport security).

Streaming responses on transport 2 use **SSE**, not length-prefixed gRPC frames:
`?alt=sse` is appended (`0x58afd45` `alt`, `0x58afd48` `sse`) and decoded by a
`bufio.Scanner` over `io.ReadCloser` (`deserializeStreamResponse[struct { r *bufio.Scanner;
rc io.ReadCloser; h net/http.Header }]`), with in-band error detection
(`decodeInBandSSEError`, error text `decoding in-band SSE JSON error: %w` next to
`cloudcode-pa.googleapis.com` at `0x5abd126`).

### 3.1 `v1internal` namespace

`google.internal.cloud.code.v1internal` is the Cloud Code "internal" API: the same product
surface as the public `cloudcode-pa` API, under a version string that keeps it out of any
public client library. Every route in the embedded descriptor is a
`google.api.http` POST with `body: "*"` on `/v1internal:<method>` — i.e. Google's
REST-transcoding shape, not RPC-over-JSON:

```
12 :\x01*"\x1b/v1internal:listExperiments
12 \xef\x01\n\x0eLoadCodeAssist ... :\x01*"\x1a/v1internal:loadCodeAssist
```

(`0x5fabf18`, `0x5fab973`, …; the `\x82\xd3\xe4\x93\x02` prefix is the `google.api.http`
extension tag 72295728). Extracted route table (68 distinct strings):

```
v1internal: loadCodeAssist, listExperiments, fetchAvailableModels, fetchAdminControls,
  fetchUserInfo, setUserSettings, getCodeAssistGlobalUserSetting, setCodeAssistGlobalUserSetting,
  listModelConfigs, listAgents, listCloudAICompanionProjects, listRemoteRepositories,
  onboardUser, onboardUserBackgroundTasks, migrateDatabaseCode, rewriteUri, checkUrlDenylist,
  checkUrlAntivirus, recordClientEvent, recordCodeAssistMetrics, recordSmartchoicesFeedback,
  recordTrajectoryAnalytics, writeTrajectoryAcls, uploadPerformanceProfile, registerInteraction,
  sendSignalingMessage, pollSignalingMessages, completeCode, generateCode, generateChat,
  streamGenerateChat, streamGenerateContent, generateContent, countTokens, tabChat,
  searchSnippets, transformCode, internalAtomicAgenticChat, battleModeAutoTrigger,
  battleModeOverrides, fetchCodeCustomizationState, fetchFromTrawlerCache, connectInstance,
  connectInstanceV2, deleteInstance, listInstances, updateInstance, proxyCommand,
  streamProxyCommand, initiateMeshSession, registerWebPush, listAuthorizedDevices,
  revokeAuthorizedDevice, notifyUser, getBundleWriteMint, provisionConversationBundleDir
v1beta: fetchLicenses
```

The two endpoints actually observed on the wire:
`https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist` and
`…/v1internal:fetchAvailableModels` (`http_helpers.go:349`), both POST.

---

## 4. Base-URL selection

Four independent origins, selected by *entitlement/auth mode*, not by model:

### 4.1 Cloud Code PA (default origin)

`(*CodeAssistClient).jetskiServiceEndpointURL` (0x83eeca0), disassembled:

```
if v := os.LookupEnv("JETSKI_SERVICE_ENDPOINT_URL"); v != "" { return v }
base := c.authProvider.GetEndpointURL()          // interface call [rsi+0x28]
if <pkg-level flag @0xe18e90c> != 0 {
    switch len(base) {
    case 41: base=="https://daily-cloudcode-pa.googleapis.com"                    -> "https://aicode.googleapis.com"
    case 35: base=="https://cloudcode-pa.googleapis.com"                          -> "https://aicode.googleapis.com"
    case 57: base=="https://preprod-daily-cloudcode-pa.sandbox.googleapis.com"    -> "https://aicode.googleapis.com"
    case 52: base=="https://autopush-aicode.sandbox.googleapis.com"               -> (sandbox aicode, 0x5b02df6/46)
    }
}
return base
```

Route composition: `constructJetskiEndpointWithParams` and `constructCodeAssistEndpointWithParams`
both use `fmt.Sprintf("%s/%s:%s", host, version, method)` (format string `%s/%s:%s` at
`0x58f8739`), `version = "v1internal"`.

Where `GetEndpointURL()` gets its value: `(*CLIAuthProvider).UpdateEndpointURL` (0x83d6a60)
reads `CLOUD_CODE_URL` and otherwise picks between `https://cloudcode-pa.googleapis.com` and
`https://daily-cloudcode-pa.googleapis.com`; it logs
`[CLIAuthProvider] UpdateEndpointURL: preserving custom endpoint %q` when `CLOUD_CODE_URL`
is set. **UNRESOLVED:** the discriminator between prod and `daily-` inside `UpdateEndpointURL`
was not recovered. Observed behaviour on this machine (consumer OAuth) is `daily-`.

`store.FetchCLIFeatureProvider` carries the same three hosts plus
`https://preprod-daily-cloudcode-pa.sandbox.googleapis.com`, so the channel choice is also
visible to the CLI-side feature provider.

`CreateCloudCodeHTTPClient` (0x83ee620) collects the whole override set in one place:
`CLOUD_CODE_URL` (default `https://localhost`), `BAICODE_ENDPOINT_URL`,
`BAICODE_PREDICTION_ENDPOINT_URL`, `BAICODE_MANAGEMENT_ENDPOINT_URL`,
`BAICODE_TELEMETRY_ENDPOINT_URL`, `AICODE_ENDPOINT_URL`, `JETSKI_SERVICE_ENDPOINT_URL`.

### 4.2 AI Code

`codeassistclient.getAICodeURL` / `getAICodeRemoteURL`: `AICODE_ENDPOINT_URL`, default
`https://aicode.googleapis.com`. `constructJetskiEndpoint` also knows
`https://autopush-aicode.sandbox.googleapis.com`. Version segment `v1alpha` (`0x58e1812`).

### 4.3 Business AI Code

`codeassistclient.getBusinessAICodeURL` (0x83da500): `BAICODE_*` env vars, default host
`businessaicode.googleapis.com`, version `v1beta`. Path form for management RPCs:
`/%s/projects/%s/locations/%s:%s` (0x5a9ada5 region).

### 4.4 Vertex AI

`(*CodeAssistClient).vertexURL`: `%s.%s.rep.googleapis.com` with `aiplatform`, i.e.
`{location}-aiplatform.rep.googleapis.com`, `?alt=sse`. `location` from
`GOOGLE_CLOUD_LOCATION` / `(*AuthProvider).GetGCPLocation`, project from `GetCAICProject`.

### 4.5 Gateway (bring-your-own OpenAI-compatible endpoint)

`StreamGenerateContentFromGateway` (0x83ec840) supports three shapes in one function:
`/v1/chat/completions`, `/chat/completions`, and
`%s/v1beta/models/%s:streamGenerateContent?alt=sse`; error string
`could not determine model for Gateway request: model name is empty`. Configured entirely by
env (via `gateway.ParseEnvOverrides`, referenced from
`cli/backend.(*ServerBackendConfig).chainedAuthOrDefault` and `store.FetchCLIFeatureProvider`):

```
AGY_LLM_GATEWAY_URL  AGY_LLM_GATEWAY_API_KEY  AGY_LLM_GATEWAY_MODELS
AGY_LLM_GATEWAY_HEADERS  AGY_LLM_GATEWAY_CA_CERT  AGY_LLM_GATEWAY_PROXY_URL
AGY_LLM_GATEWAY_WIRE_PROTOCOL
AGY_LLM_GATEWAY_TIER_PRO  AGY_LLM_GATEWAY_TIER_FLASH  AGY_LLM_GATEWAY_TIER_FLASH_LITE
AGY_BUSINESS_PAYGO_TIER
```

### 4.6 The `google-genai` env vars belong to the vendored SDK, not to the CLI

`GOOGLE_GEMINI_BASE_URL`, `GOOGLE_VERTEX_BASE_URL`, `GOOGLE_GENAI_USE_VERTEXAI` and
`GOOGLE_GENAI_USE_ENTERPRISE` are referenced **only** by
`google_golang_org/genai/v0/google/genai.{NewInternalAPIClient,defaultEnvVarProvider,getAPIKeyFromEnv}`
and by `model_api/backends/geminiapi.(*Client).newGenAIClient`. They are the stock google-genai
SDK knobs, reached only on the `geminiapi` backend (Gemini API key / Vertex direct), not on
the Cloud Code path. The SDK's own conflict message is embedded:
`GOOGLE_GENAI_USE_ENTERPRISE and GOOGLE_GENAI_USE_VERTEXAI are set with conflicting values. The value of GOOGLE_GENAI_USE_ENTERPRISE will be used.`

---

## 5. Diagram

```
  ┌──────────────────────────── agy (pid N) ────────────────────────────┐
  │  CLI (cobra)                                                        │
  │   ├─ store/quota_manager, backend.(*ServerBackend)                  │
  │   └─ language_server_go_grpc.(*languageServerServiceClient)         │
  │        358 methods, gRPC over TLS                                   │
  └───────────────────────────────┬─────────────────────────────────────┘
                                  │  127.0.0.1:<rand>  "HTTPS (gRPC)"
  ┌───────────────────────────────▼─────────────────────────────────────┐
  │  agy (pid M) — child language server                                │
  │   connect handlers: LanguageServerService / RemotingService /       │
  │     ConversationService / HostBridgeService / DevService            │
  │   interceptors: requestID → traceTask → rpcTelemetry → Csrf →       │
  │                 hostClientIdentity                                   │
  │   ── cortex ── modelapiinterface                                    │
  │        ├─ modelapiccpa.(*Client).GetChatMessage   (Cloud Code PA)   │
  │        └─ model_api/backends/geminiapi.(*Client)  (genai SDK)       │
  │   ── code_assist_client.(*CodeAssistClient) ──                      │
  │        entitlement switch → 5 HTTP backends                         │
  └──┬────────────┬────────────┬────────────┬────────────┬──────────────┘
     │            │            │            │            │
     │ HTTPS+JSON │ HTTPS+JSON │ HTTPS+JSON │ HTTPS+JSON │ HTTPS+JSON (SSE)
     ▼            ▼            ▼            ▼            ▼
 Cloud Code    AI Code     Business      Vertex AI    Gateway
 PA            aicode.     AI Code       {loc}-       AGY_LLM_GATEWAY_URL
 daily-        googleapis  business      aiplatform   /v1/chat/completions
 cloudcode-pa  .com        aicode        .rep.        or :streamGenerateContent
 .googleapis               .googleapis   googleapis
     │                                                  │
     └── /v1internal:<method>   ?alt=sse ───────────────┘
                                  │
                       telemetry: v1internal:recordClientEvent,
                       :recordTrajectoryAnalytics, :uploadPerformanceProfile,
                       BAICode :sendTelemetry, Clearcut (/clearcut_proxy),
                       Sentry (only if SENTRY_DSN), Unleash
```

---

## 6. Inference path

### 6.1 Which RPC carries the prompt

The prompt never goes straight from the CLI to a model. Two hops:

1. **CLI → child LS**, `exa.language_server_pb.LanguageServerService`:
   `StartCascade` (creates the conversation/agent) then `SendUserCascadeMessage` (carries the
   user turn) — both unary; the assistant side is streamed back over
   `StreamAgentStateUpdates` / `StreamCascadeReactiveUpdates` /
   `StreamCascadePanelReactiveUpdates` / `StreamUserTrajectoryReactiveUpdates`, all
   server-streaming. `HandleCascadeUserInteraction` returns tool approvals; `CancelCascadeInvocation`
   aborts. This matches the CLI-side logs (`conversation_manager.go:1083] Streaming conversation …`).
2. **LS → Google**, one POST to `/v1internal:streamGenerateContent?alt=sse` on the base URL
   selected in §4, made by `(*CodeAssistClient).streamGenerateContentFromCCPA`
   (0x83e9000). Its log line is `Cortex API Request: %s`; the sibling
   `StreamGenerateContentFromAICode` logs `Making AICode request to: %s`, Vertex logs
   `Making Vertex AI request to: %s`, BAICode logs `Making Business AI Code request to: %s`,
   Gateway logs `Making Gateway streaming request to: %s`.

### 6.2 The model-layer request envelope

`modelapiccpa` is the Cloud Code model client. It does **not** use `api_server_pb` on the
wire; it *receives* an `api_server_pb.GetChatMessageRequest` from the cortex layer, converts
it (`modelRequestToGetChatMessageRequest`, then a converter's `ToRequestPayload`) and posts a
Gemini `GenerateContentRequest`. Recovered `exa.chat_pb.GetChatMessageRequest`
(descriptor at `0x5e18e20`):

```proto
message GetChatMessageRequest {
  exa.codeium_common_pb.Metadata metadata        = 1;
  reserved 2;                                    // was `prompt`
  repeated exa.chat_pb.ChatMessage chat_messages = 3;
  exa.codeium_common_pb.ExperimentConfig experiment_config = 4 [deprecated];
  exa.codeium_common_pb.Document active_document = 5;
  reserved 6, 7;                                 // *_paths_migrate_me_to_uris
  exa.codeium_common_pb.ContextInclusionType context_inclusion_type = 8;
  exa.codeium_common_pb.Model chat_model          = 9;   // enum, legacy selector
  string system_prompt_override                  = 10;
  string active_selection                        = 11;
  repeated string open_document_uris             = 12;
  repeated string workspace_uris                 = 13;
  string chat_model_name                         = 14;  // current selector, e.g. "gemini-4-argon-medium"
  EnterpriseExternalModelConfig enterprise_chat_model_config = 15;  // max_input/output_tokens
}
```

`exa.codeium_common_pb.Metadata` (descriptor at `0x61ce7e0`) is the attribution/telemetry
payload attached to every model call — this is the privacy-relevant part:

```
1 ide_name        2 extension_version   3 api_key         4 locale
5 os              6 disable_telemetry   7 ide_version     8 hardware
10 session_id     12 extension_name     16 ls_timestamp    17 extension_path
18 user_tags      19 trigger_id         24 device_fingerprint
27 id             29 user_tier_id       30 region_code     31 product_name
32 environment_type                   33 runtime_environment
```

### 6.3 Streaming decode

`deserializeStreamResponse[T]` is instantiated eight times, one per response type, all over
`struct { r *bufio.Scanner; rc io.ReadCloser; h net/http.Header }` — a line scanner over the
SSE body. Then:

- `ProcessStreamChunks` + `decodeInBandSSEError` handle Gemini-style `data:` frames and the
  `{"error": …}` in-band failure case.
- `(*channelModelResponseStream)` (in `codeassistclient`, the "channel_model_response_stream.go"
  of the brief) implements `Receive/Msg/Err/Close` over a Go `chan`, bridging the scanner
  goroutine to the `GetChatMessageServerStreamForClientInterface` of `modelapiinterface`.
- `anthropicconverter.(*StreamDecoder).DecodeEvent` + per-block processors
  (`textContentBlockProcessor`, `thinkingContentBlockProcessor`,
  `toolUseContextContentBlockProcessor`, `jsonModeContentBlockProcessor`, each with
  `HandleContentBlockStart/Delta/Stop/LastContentBlockStop`) decode Anthropic
  `content_block_start` / `content_block_delta` / `content_block_stop` events.
- `openaiconverter.ProcessSSEEvents` + `mergeChunkResponses` + `functionCallAccumulator` decode
  OpenAI `chat.completion.chunk` frames.

**Mid-stream continuation.** `model_api/backends/components/continuation` implements resuming a
truncated stream: `CloneCCPARequestWithContinuationToken`,
`cloneMasterGenerateContentRequestWithContinuationToken`,
`(*Orchestrator).RecordCandidate`, `EvaluateLegEnd`, `EvaluateDialError`,
`canResumeOnError`, `isResourceExhaustedError`, `isContinuationBudgetExhaustedError`,
`SleepForMidStreamRetry`, `MasterStreamTracker.NewSyntheticMaxTokensResponse`, and
`(*CodeAssistClient).openNextCCPAContinuationLeg`. Error text: `stream idle timeout (%v): %w`
(0x5a7c6a4 region), `MODEL_CAPACITY_EXHAUSTED`, `individual quota …`.

**Keepalive.** `modelapiccpa.keepAlivePings` / `(*keepAlives)` — `beforeRequest`,
`onRequestDone`, `scheduleNextPingLocked`, `ping`, `sendPing`, `newKeepAlives` — sends a ping
while a long stream is open so the HTTP/2 or proxy path does not idle out. **UNRESOLVED:** the
interval constants (they are immediates inside a 160-byte BOLT fragment; not recovered).

### 6.4 The model-layer interface

`modelapiinterface` is the seam: `GetChatMessage`, `GenerateNonStreaming`, `GetModelInfos`,
`GetCascadeModelConfigData`, `GetDeprecatedModelReroute`, `GetCommitMessageModelEnum`,
`GetVideoSummarization`, `GetImageGeneration`, `GetWebSearchResults` — all typed with
`api_server_pb` messages. Two backends implement it: `modelapiccpa` (Cloud Code) and
`model_api/backends/geminiapi` (genai SDK, `newGenAIClient`, `convertGeminiResponseToApiServerResponse`).

---

## 7. Multi-vendor routing

**Non-Gemini models are proxied through Google. There is no vendor-direct path.**

The routing happens twice, at two different layers.

### 7.1 Entitlement / auth-mode layer (which *host*)

`(*CodeAssistClient).StreamGenerateContent` (0x83e87e0), disassembled. After
`getCAICProject()`, `computeRequestID()`, `time.Now()` it dispatches:

```
if isOpenAIGateway(...)                        -> StreamGenerateContentFromGateway
if tier == "personal_consumer"                 -> StreamGenerateContentFromAICode
default / consumer fallback                    -> streamGenerateContentWithContinuationFromCCPA
if tier == "business_agent_platform_paygo"     -> StreamGenerateContentFromVertexAI
if tier == "business_licensed_enterprise"      -> StreamGenerateContentFromBusinessAICode
```

(`0x83e8c90`, `0x83e8d11`, `0x83e8d40`, `0x83e8d91`, `0x83e8e4b`; the three tier strings sit at
`0x59cd673`, `0x5a874e8`, `0x5a7c6a4` and are also used by every auth provider's
`ResolveAuthMode`, `IsBusinessEnterprise` and `SetHTTPHeaders`.) `ListExperiments` has the same
split with an explicit fallback log: `AICode FetchConfig failed, falling back to CCPA ListExperiments: %v`.

**Direction caveat on the `daily-` observation:** `loadCodeAssist` and `fetchAvailableModels`
were observed on `daily-cloudcode-pa` for this consumer account, while consumer *inference*
would take the AICode branch. The two are different helpers (`constructCodeAssistEndpoint` for
management, `streamGenerateContentFromCCPA` for inference), so this is consistent — but I did
not capture a live inference request to confirm the host on the wire. **UNRESOLVED.**

### 7.2 API-provider layer (which *wire shape*)

`vertexURLPath` (0x840be00) and `modelEndpoint` (0x840c2a0) switch on
`codeium_common_pb.APIProvider`:

| provider | path builder | streaming method | non-streaming method |
|---|---|---|---|
| Anthropic | `publishers/anthropic/models/` | `streamRawPredict` | `rawPredict` |
| Google / Gemini | `publishers/google/models/` | `streamGenerateContent` | `generateContent` |
| OpenAI | `endpoints/openapi/` | `chat/completions` | — |
| anything else | — | `unsupported API provider for streaming method: %v` | `unsupported API provider for non-streaming method: %v` |

`modelEndpoint` additionally emits `unsupported API provider: %v`.

### 7.3 So: which converter runs, and in which direction (the lead's question)

The `google3/cloud/developer_experience/codeassist/shared/converters/` tree contains three
converter families, and the naming is unambiguous about direction:

**`anthropic/anthropicconverter` — request out, response back, Anthropic wire on the Google hop.**

- `ToAnthropicRequest` — `GenerateContentRequest` → Anthropic Messages request. This converts
  *the request we send*. Sub-steps: `applySystemInstruction`, `applyToolDeclarations`,
  `applyToolConfig`, `applyGenerationConfig`, `convertContents`, `convertParts`,
  `mapContentRole`, `toToolInputSchema`, `fixSchemaInt64Strings`, `fixInt64StringFields`,
  `setJSONModeIfResponseSchema`, `thinkingLevelToEffort`.
- `ToVertexGenerateContentResponse` + `fromAnthropicContent` + `Usage.ToTokenUsage` — Anthropic
  response → Vertex/GenAI `GenerateContentResponse`. This converts *the response we receive*.
- `(*StreamDecoder).DecodeEvent` + the four content-block processors — decodes Anthropic SSE.
- `SetCacheControlForJetski` — sets Anthropic `cache_control` markers "for Jetski", i.e. tuned
  so the *Google* backend does the prompt caching. That string alone rules out vendor-direct.
- The wire version string is `vertex-2023-10-16` (`0x59ceeb0`), which is Vertex AI's
  `anthropic_version` for Anthropic-on-Vertex, and the model default in the same function is
  `claude-sonnet-4`.

Conclusion: Claude traffic is an **Anthropic-format request POSTed to Vertex AI's
`publishers/anthropic/models/{model}:streamRawPredict`**, authenticated with the user's Google
OAuth token, then decoded from Anthropic SSE back into GenAI types.
`(*CodeAssistClient).filterUnavailableVertex3PModels`, `probeVertex3PModelAccess`,
`GetVertexServiceTier` / `SetVertexServiceTier` and `addVertexAILabelsHeader` /
`addInternalVertexMetadataHeader` are the surrounding 3P-on-Vertex plumbing.

**`openai/openaiconverter` — same two directions, OpenAI wire.**

`ToChatCompletionsRequest`, `ToOpenAIChatCompletionChunk`, `ChatCompletionToGenerateContentResponse`,
`ToGenerateContentResponse`, `ProcessSSEEvents`, `mergeChunkResponses`,
`oaiCompletionUsageToTokenUsage`, `toChatCompletionsContentPartUnion`,
`handleFunctionResponsePart`. Matches `modelEndpoint`'s `endpoints/openapi/` +
`chat/completions` pair, i.e. **Vertex MaaS**. The model ids confirm it:
`gpt-oss-120b-maas` and `gpt-oss-20b-maas` (`0x5a4ccd1`, `0x5a3eada`) — `-maas`
(Model-as-a-Service) is Vertex's third-party publisher naming, and the CLI's own model id is
`gpt-oss-120b-medium` mapped onto it.

**`baicode/baicodeconverter` and `aicode/generators/aicode_converters`** —
`ToBusinessAICodeGenerateContentRequest`, `VertexToAICodeGenerateContentRequest/Response`,
`VertexToBAICodeGenerateContentRequest/Response`, plus generated `Forward`/`Backward` field
mappers: pure GenAI⇄protobuf translation for the two Google-hosted sibling APIs.

**`geminiConverter` / `ccpaConverter` / `aicodeConverter` / `baicodeConverter`** (the
`codeassistclient`-local ones, all with `ToRequestPayload`,
`ToGenerateContentResponse`, `ToStreamGenerateContentResponse`) are the per-backend adapters
behind the §7.1 dispatch.

`cli/backend.extractAPIProvider` is a *different*, CLI-side switch keyed on model-name
prefixes: `blade:beyond-api-gateway-`, `beyond-api:`, `gemini-api:`, `evergreen:`, plus
`Custom Model: %s`. `models/model_config.InferAPIProviderFromModelName` maps the substring
`claude` → API-provider key `/anthropic/` (only two string literals in its span), which is the
name→provider switch the lead spotted; it feeds §7.2, not §7.1.

### 7.4 What `MODEL_PRICING_TYPE_BYOK` means here

`MODEL_PRICING_TYPE_BYOK` is an enum value of `codeium_common_pb.ModelPricingType`
(`0x5a38e1b`), regenerated into `codeium_common_go_proto.init`. It marks a model whose cost is
borne by the *user's own* credential (the Gemini API key path, `geminiAPIKeyAuth` /
`GeminiAPIKeyAuthProvider`, or the gateway path) rather than by the subscription. It does not
create a vendor-direct socket: on that path the CLI still uses a Google-shaped endpoint
(`generativelanguage.googleapis.com` via the genai SDK, or `AGY_LLM_GATEWAY_URL`) — the change
is who is billed and which credential is attached, not which wire format or which company's
servers. **Derived**, from the enum's position next to the other `MODEL_PRICING_TYPE_*` values
and from `GeminiAPIKeyAuthProvider` being one of the `ChainedAuth` methods; no BYOK request was
captured.

---

## 8. Headers and attribution

### 8.1 Outbound to Google

From `(*CodeAssistClient).setHeaders` (0x83efbe0), `addServiceTierHeader` (0x840a840),
`addVertexAILabelsHeader`, `addInternalVertexMetadataHeader`, `computeRequestID`,
`getSessionID`, `getStreamID`:

```
User-Agent: %s/%s (%s)                  // "<product>/<version> (<os or channel>)"
Content-Type: application/json
X-Goog-Request-Reason: <reason>
X-Aicode-Request-Id: <request id>       // getStreamingTextCompletion
x-cloudaicompanion-trace-id: <trace>    // used by ProcessStreamChunks,
                                        //   deserializeStreamResponse[], doGet/doPost,
                                        //   FetchLicensesFromAICode, readRemoteAgentResponse,
                                        //   printDebugInfo
x-goog-sherlog-Link: <…>                // CreateCloudCodeHTTPClient
X-Goog-User-Project: <quota project>
goog-originating-service-id: <…>
X-Vertex-AI-LLM-Shared-Request-Type: priority     // Vertex 3P service tier
```

`http_helpers.go:349] URL: https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist Trace: 0xbeaf2d2bdef18896`
— the `Trace:` value is a `uint64` printed with `%#x`; it is the *request id / trace id* the
client mints per call and echoes into the `x-cloudaicompanion-trace-id` header (correlated with
`computeRequestID` + `getSessionID` + `getStreamID`, which `getStreamingTextCompletion`
populates alongside `X-Aicode-Request-Id` and `sessionID`). It is not a credential binding —
the credential is a separate `Authorization: Bearer <oauth token>` from the token source.

#### Observed live (2026-10-27)

Captured with `agy -v=5 models`; the CLI's own `http_helpers.go:369/375/381` log sites dump the
request UA, the response headers and the response body. Verbatim block in
[`evidence/live-2026-10-27/request-response-headers.txt`](evidence/live-2026-10-27/request-response-headers.txt).

**The real User-Agent** — and it corrects the `%s/%s (%s)` shape above, because the parens
technically hold a semicolon-joined parameter list, not just an OS:

```
antigravity/cli/1.4.8 (aidev_client; os_type=linux; arch=amd64; cl=1007749967; auth_method=consumer)
```

| parameter | observed | note |
|---|---|---|
| `aidev_client` | fixed | product token |
| `os_type=linux` | `linux` | platform |
| `arch=amd64` | `amd64` | GOARCH spelling |
| `cl=1007749967` | **not** the build's changelist | the build reports `cl/1000353293`; this is a separate release-side constant, so do not derive it from the binary version |
| `auth_method=consumer` | `consumer` / `gcp` | mirrors the OAuth client choice of [`02-auth.md`](02-auth.md) §3 |

**Response headers** on every `v1internal` call:

```
Content-Type: application/json; charset=UTF-8     ← protojson, camelCase fields
X-Cloudaicompanion-Trace-Id: <trace id>           ← echoes the `Trace:` value, no 0x prefix
Server: ESF                                       ← Google's Extensible Service Proxy
Alt-Svc: h3=":443"; ma=2592000,h3-29=":443"; ma=2592000
Server-Timing: gfet5t7;dur=597, gfet4t7; dur=779
Vary: Origin, X-Origin, Referer
X-Xss-Protection: 0 / X-Frame-Options: SAMEORIGIN / X-Content-Type-Options: nosniff
Date: <rfc1123>
```

Two things a reimplementation must not assume:

1. **No quota or rate-limit headers.** There is no `X-RateLimit-*`, no `Retry-After` and no
   quota field on the metadata responses. Header ingestion therefore contributes nothing for
   this provider — every number comes from a body (see [`05-quota.md`](05-quota.md)).
2. **The wire is JSON, not protobuf.** Bodies are `protojson`: camelCase keys
   (`bucketId`, `resetTime`, `remainingFraction`), string-encoded int64. The proto field
   numbers in 05-quota describe the schema, not the wire encoding.

`Server: ESF` plus `Alt-Svc: h3` also settles transport: HTTP/1.1 or h2 over TLS to an ESF
front end that offers h3. No gRPC, no Connect for this hop — the Connect/gRPC layer is the
*loopback* boundary to the language server, not the Google boundary.

#### The inference call — observed, and smaller than the code implied

One `agy -p 'Reply with exactly: OK' --model gemini-3.8-flash-low` turn, run behind a local
TLS-terminating proxy so the request heads could be read. Verbatim (credential masked) in
[`evidence/live-2026-10-27/inference-request-headers.txt`](evidence/live-2026-10-27/inference-request-headers.txt):

```
POST /v1internal:streamGenerateContent?alt=sse HTTP/1.1
Host: daily-cloudcode-pa.googleapis.com
User-Agent: antigravity/cli/1.4.8 (aidev_client; os_type=linux; arch=amd64; cl=1007749967; auth_method=consumer)
Transfer-Encoding: chunked
Authorization: Bearer <redacted, 265 chars>
Content-Type: application/json
Accept-Encoding: gzip
```

That is the **complete** set — seven headers and nothing else. `Authorization` is `Bearer`, a
plain OAuth access token, not a signed request and not an `x-goog-*` credential header. The
265-character value is `Bearer ` plus a 258-character token, consistent with a Google `ya29.`
access token; the value itself is deliberately not recorded.

Equally important is what is **absent**, because the §8.1 list above was assembled from
`setHeaders` and its siblings and therefore overstates the wire:

| header from the code | sent on inference? |
|---|---|
| `X-Goog-Request-Reason` | **no** |
| `x-cloudaicompanion-trace-id` | **no** (response-only; the request carries no trace header) |
| `x-goog-sherlog-Link` | **no** |
| `X-Goog-User-Project` | **no** |
| `goog-originating-service-id` | **no** |
| `X-Vertex-AI-LLM-Shared-Request-Type` | **no** |
| `X-Aicode-Request-Id` | **no** |

So those headers exist in code paths that this account and this model do not take. A
reimplementation that copies the whole list sends headers the real client never sends — the
kind of difference that is harmless until an upstream starts caring. Send the seven above.

The body is **chunked JSON**: `Transfer-Encoding: chunked` with no `Content-Length`, because
the request envelope is streamed rather than buffered. `?alt=sse` on the URL, not a header, is
what selects server-sent events.

#### What else a turn contacts

Same capture, same turn — the complete outbound surface, which is broader than a models
listing:

| host | request | auth |
|---|---|---|
| `daily-cloudcode-pa.googleapis.com` | `:loadCodeAssist`, `:fetchUserInfo`, `:fetchAvailableModels`, `:retrieveUserQuotaSummary`, `:writeTrajectoryAcls`, `:streamGenerateContent?alt=sse` | `Bearer` |
| `aicode.googleapis.com` | `GET /v1alpha:fetchConfig` | `Bearer` |
| `www.googleapis.com` | `GET /oauth2/v2/userinfo` | `Bearer` |
| `play.googleapis.com` | `POST /log` ×3 (36,240 / 29,415 / 51,014 B) | `Bearer` |

The last row is the telemetry answer: **an agent turn does send Clearcut traffic** — three
batches of 36,240 / 29,415 / 51,014 bytes, ~117 KB in total for a one-word prompt — to
`play.googleapis.com/log`, and they carry the user's access token, not an anonymous or API-key
identity. Its request is visibly unscrubbed:

```
POST /log HTTP/1.1
Host: play.googleapis.com
User-Agent: Go-http-client/1.1          ← default Go UA, no product string
Content-Length: 36240
Authorization: Bearer <redacted>
Content-Type: application/octet-stream  ← binary Clearcut batch
Accept-Encoding: gzip
```

The missing product UA is the tell that this hop does **not** go through the logging
`http_helpers` client: it uses a bare `http.Client`. A `data_collection_optin=false` account
would presumably suppress it, but that is untested here and is not something to infer from
this capture (see §11.6).

`aicode.googleapis.com` being live also resolves §11.2 for the consumer path: the AICode host
serves `fetchConfig` in 1.4.8, while prompt inference stays on `daily-cloudcode-pa`.

OAuth scopes in play: `aicode`, `openid`, `cloud-platform`, `userinfo.email`,
`userinfo.profile`, `experimentsandconfigs`, `cclog` (from `getScopesByAuthMethod`).

### 8.2 Inbound to the loopback language server: CSRF and per-principal RPC authorization

`language_server/interceptor/interceptor.(*CsrfInterceptor)` is the security boundary of the
local server. It has two distinct jobs:

1. **Token check.** `validateAndAuthorize` compares the request header
   **`x-codeium-csrf-token`** (0x5a04d30) against the token the CLI generated and exported as
   `ANTIGRAVITY_CSRF_TOKEN`. Failures: `missing CSRF token`, `invalid CSRF token`
   (0x59df9c2, 0x59df86c).
2. **Scoped-token principal check.** `authorize` (0x7b4cf80) implements a second, narrower
   credential: a *scoped* token bound to a `principal` with an allowlist of `procedure`s.
   Strings: `principal`, `procedure`, `decision` ∈ {`allowed`, `denied`},
   `enforcement_enabled`, `dry_run_blocked`, flag/experiment name `ls_scoped_rpc_access`,
   and telemetry field `[LS Auth] Blocked RPC for principal %q: procedure=%q`,
   `procedure %q is not allowed for principal %q`,
   `[LS Auth Telemetry] Scoped token RPC called: principal=%q procedure=%q enforcement_enabled=%t`.
   `isScopedEnforcementEnabled` builds a `%s/%s` key, so the flag is evaluated per
   principal/procedure. `dry_run_blocked` means the scoped check can run in report-only mode
   behind the flag. So an unprivileged local caller with a valid CSRF token can still be
   confined to a subset of RPCs by principal.

There is also `isMethodAllowed` (0x7b4e2c0) gated on the `rpc-telemetry-allowlist`
(0x5a3e07e), which decides which RPCs are reported by `rpcTelemetryInterceptor`.

### 8.3 CORS / origin allowlist

`language_server/constants/constants.GetAllowedHostDomains` reads **`AGY_ALLOWED_DOMAINS`**
and defaults to `https://localhost:*` and `https://127.0.0.1:*` (0x59f18f7). The wider literal
at `0x59ccd38` shows the full set the server knows about:
`https://localhost`, `https://127.0.0.1`, `http://localhost:`, `http://127.0.0.1:`,
`vscode-webview://`, `.corp.google.com`. Note the asymmetry: the *default* allowlist accepts
only `https://` with a port wildcard, while `http://localhost:<port>` appears in the general
constant pool — worth treating as a real difference, not decoration, since the plain-HTTP
sidecar port exists.

`(*Server).handleUnleashProxy` is the one place that answers preflight explicitly:

```
Access-Control-Allow-Origin:  *          (0x5a7042b)
Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS   (0x5a7b8f8, 0x5a9a224)
Access-Control-Allow-Headers: Content-Type, Authorization       (0x5a7b914, 0x5a70446)
```

`*` + `Authorization` on a loopback endpoint that forwards to `https://antigravity-unleash.goog/api`
is the most permissive CORS in the product. It is gated: `Unleash is disabled in LLM gateway mode`.

---

## 9. Analytics and telemetry

### 9.1 What exists

`telemetryinterface` defines the whole vocabulary behind a `CompositeTelemetryClient`:
`RecordCompletions`, `RecordCommandUsage`, `RecordTrajectorySegmentAnalytics`,
`RecordFullTrajectoryAnalytics`, `RecordTrajectorySegmentEvents`, `RecordCortexTrajectory`,
`RecordCortexTrajectoryStep`, `RecordCortexGeneratorMetadata`, `RecordCortexExecutionMetadata`,
`RecordBattleModeMetadata`, `RecordStateInitializationData`, `RecordReadUrlContent`,
`RecordAnalyticsEvent(String)`, `RecordError`, `RecordObservabilityData`, `RecordRPCMetadata`,
`RecordMonitorSignal`, `RecordInputCompletion(Feedback)`, `RecordRemoteControlEvent`,
`RecordHookExecution`, `ProvideFeedback`, `Shutdown`.

Crucially, a parallel **`(*DummyTelemetryClient)`** implements every one of those as a no-op.
That is the kill switch: the client list is a composite, and the dummy can stand in for the
real one.

### 9.2 Where it goes

| sink | evidence | when |
|---|---|---|
| Cloud Code PA `POST /v1internal:recordClientEvent` | route strings; `(*CodeAssistClient).recordAnalyticsEvent`, `RecordTrajectorySegmentAnalytics` (0x8404cc0, errors `marshal trajectory: %w`, `marshal metadata: %w`, route `recordTrajectoryAnalytics`) | per trajectory segment / analytics event |
| Cloud Code PA `POST /v1internal:recordTrajectoryAnalytics` | same function; request protobuf `RecordTrajectoryAnalyticsRequest{ trajectory: Struct, start_step_index, start_generator_metadata_index, metadata: Struct, mendel_experiment_ids: []int }` (descriptor `0x5e18e20` region) | per segment; carries the *whole trajectory* as a Struct |
| Cloud Code PA `POST /v1internal:recordCodeAssistMetrics`, `:recordSmartchoicesFeedback`, `:uploadPerformanceProfile` | route table; `(*CodeAssistClient).UploadPerformanceProfile` | metrics, feedback, pprof upload |
| Business AI Code `POST /v1beta/projects/{p}/locations/{l}:sendTelemetry` | `(*CodeAssistClient).SendBAICTelemetry` (0x83dc9c0); `TelemetryService/SendTelemetry`; logs `[BAIC Telemetry] Failed to send BAIC telemetry: %v` | enterprise only |
| Clearcut, via the local `/clearcut_proxy` | `0x59a4547` `/proxy/unleash//clearcut_proxy`; `(*CodeAssistClient).recordAnalyticsEvent` | Google-internal log pipeline |
| Sentry | `SENTRY_DSN` env (0x592a8eb), `ANTIGRAVITY_SENTRY_SAMPLE_RATE` (0x5a91519, 0x61e0ba3), `SENTRY` tag (0x61e0b4c), `ExperimentSentryPayload{sample_rate, procedure_to_sample_rate, error_match_to_sample_rate}` | **only if `SENTRY_DSN` is set** — no DSN is embedded in the binary |
| pprof | `profiler.go:231] Continuous pprof profiling is disabled.`; `/debug/pprof/trace` served on the local HTTP port | off by default; served locally |

Also `(*CodeAssistClient).SkipTelemetry` and the per-request `Metadata.disable_telemetry`
(bool, field 6) and `Metadata.api_key` (field 3) — the model request itself carries a
telemetry-off flag.

### 9.3 Startup telemetry

```
analytics.go:249] CLI startup completed (took 240.397653ms, phases
  [main_entered=0ms ls_start=1ms ls_ready=226ms launch_cli=229ms backend_created=229ms
   project_resolved=230ms manager_created=232ms program_run=236ms tui_init_start=237ms
   tui_init_done=238ms])
```

Phase timings plus counts are emitted on every launch. Same on `agy models`.

### 9.4 What can be disabled

There is no `--no-telemetry` flag and no `DO_NOT_TRACK` handling. What exists instead is a
**per-user data-collection opt-in**, carried as a protobuf bool, not as a local switch:

| layer | field | setter |
|---|---|---|
| AI Code | `google.gca.aicode.v1main.UserSettings.data_collection_optin` (optional bool, field 2; resource `aicode.googleapis.com/UserSettings`) | `ManagementService/UpdateUserSettings` |
| Cloud Code PA | `SetCodeAssistGlobalUserSettingRequest.free_tier_data_collection_optin` | `POST /v1internal:setCodeAssistGlobalUserSetting` |
| CLI → LS | `exa.language_server_pb.LanguageServerService/SetUserSettings` | `(*CodeAssistClient).SetUserSettings`, `fetchUserInfo` |

(descriptors `0x5d9c099`, `0xb174344`, `0xb18c78f`, `0x5fa54a3`). Two further policy fields can
override the user: `user_data_collection_force_disabled` and
`prompt_and_response_logging_enabled` (`0xb21cbc9`, in the `google.gca.aicode.v1main` Go
struct metadata) — i.e. an enterprise admin can force data collection off, and separately
enable prompt/response logging.

So the honest picture: a user *can* opt out of data collection at the account level, and it is
an explicit boolean rather than a hidden default, but that is a different control from the
operational telemetry below.

What is definitely off or optional:

- `Metadata.disable_telemetry = true` (per model request) — client-side flag on every call.
- `SENTRY_DSN` unset (the default; no DSN is embedded) — no crash reporting.
- Continuous pprof profiling is already disabled (`profiler.go:231`); `/debug/pprof/trace` is
  only served on the loopback HTTP port.

**UNRESOLVED:** whether `data_collection_optin = false` also suppresses the
`recordClientEvent` / `recordTrajectoryAnalytics` / Clearcut channels — those are operational
telemetry and the opt-in governs content collection, so they are plausibly separate. I also
found no user-reachable route to `DummyTelemetryClient`; it looks like a test/embedded-context
implementation.

`defaults.go:732] failed to get cs path: cs path /usr/bin/cs invalid` — the CLI probes for a
`cs` binary (Google's internal "cs" client tool) on every start; a presence check, not a
network call.

---

## 10. Feature flags (Unleash / Mendel)

### 10.1 Mechanism

- Server: **`https://antigravity-unleash.goog/api`** (0x5ac3e41, 0x5accffa). Not configurable
  in the strings I recovered except through the local proxy.
- Client: the vendored `github_com/Unleash/unleash_client_go/v4`, wrapped by
  `jetski/unleash.(*UnleashWrapper)`.
- Init: `unleash.InitializeUnleash` (0x7ff96a0) / `NewUnleashWrapper` (0x7ff9ea0);
  `DisableUnleash` has the guard string `Cannot disable Unleash, already initialized`.
- **Context** sent with every evaluation (`NewUnleashWrapper` string set):
  `ide`, `ideVersion`, `extensionVersion`, `productName`, `os`, `language`,
  `installationId`, `requestedModelId`, `userTierId`, `devMode`.
  Empty properties are rejected: `empty property in unleash context: %s`.
- Proxy: the language server exposes `/proxy/unleash` on its plain-HTTP port and forwards to
  `https://antigravity-unleash.goog/api` (`(*Server).handleUnleashProxy`, 0x90e17a0). The
  CLI-side client reaches it at `127.0.0.1:4713/proxy/unleash` (default in
  `CreateCloudCodeHTTPClient`). `UNLEASH_URL` (0x5944ec2) overrides the upstream base.
- Auth: no Unleash client token is embedded; the proxy adds the caller's Google credential
  (`Authorization` is in the allowed-headers list) and the scope `experimentsandconfigs`
  gates server-side experiments.

### 10.2 Two separate flag planes

1. **Unleash / Mendel** — kill switches and experiments, evaluated locally by
   `(*UnleashWrapper).IsEnabled(ByName)`, `GetVariant(ByName)`, `getExperiment`, over a
   snapshot refreshed by `unleash.InitializeUnleash.func1` (which builds
   `https://antigravity-unleash.goog/api/InitializeUnleash`).
2. **`ListExperiments`** — the Cloud Code RPC (`/v1internal:listExperiments`, and the AICode
   `FetchConfig` variant with `shadow-call-aicode-mendel`). `(*CodeAssistClient).ListExperiments`
   (0x83dec60) prefers AICode `FetchConfig` and falls back:
   `AICode FetchConfig failed, falling back to CCPA ListExperiments: %v`. `CompareAGYExperiments`
   diffs the two responses for the shadow evaluation.
   Refresh is triggered by login (`experiment_manager.go:66] Starting experiment refresh after login`
   → `:70] Experiments refreshed after login`) and on a poll (`Failed to poll ListExperiments`
   appears in `errorreport.go:224`).

### 10.3 Model configuration comes from experiments

`experiment_manager` + `unleash.experimentConfigToKeysMap` +
`collectExperimentConfigLegacyFields` feed `codeium_common_pb.ExperimentConfig`
(`force_enable_experiments`, `force_disable_experiments`, `force_enable_experiments_with_variants`,
`force_enable/disable_experiment_strings`, `dev_mode`). This is what makes the model list,
context windows, effort levels and routing *server-controlled*.

### 10.4 How to inject an override

Three documented-by-code levers:

1. **`AGY_CLI_MODEL_EXPERIMENT_PROFILES`** — referenced only by `main.applyExperimentModelOverride`
   (0x5aa3318 LEA). This is the CLI's own experiment-profile injection point, a JSON/paths
   value applied at startup before the backend is built.
2. **`SetMendelFlagOverrides`** — `language_server.(*Server).SetMendelFlagOverrides`
   (0x90e… region) with the guard
   `Mendel flag overrides require developer mode` (0x5af73e7). The RPC request type is
   `exa.language_server_pb.SetMendelFlagOverridesRequest{ overrides: map<string,…> }`
   (descriptor `0x618308a`). So arbitrary flag overrides *are* possible over the loopback
   socket, but only when `devMode` is set in the Unleash context — which is itself a flag.
3. **`GetUnleashData`** — `exa.language_server_pb.GetUnleashDataRequest{ properties: map }`
   (`0x6184c3b`) reads the current context, useful to observe what would be evaluated.

`Unleash is disabled in LLM gateway mode` means: with `AGY_LLM_GATEWAY_URL` set, flags come
from nowhere and defaults apply.

---

## 11. Open questions / unresolved

1. **`daily-` vs prod discriminator.** `(*CLIAuthProvider).UpdateEndpointURL` picks between
   `https://cloudcode-pa.googleapis.com` and `https://daily-cloudcode-pa.googleapis.com`, but
   the condition (build channel constant? `CLOUD_CODE_URL` suffix? an entitlement field?) was
   not recovered. Observed value is always `daily-` on this machine.
2. **The `aicode` rewrite at `0xe18e90c`.** `jetskiServiceEndpointURL` has a table mapping all
   three Cloud Code PA hosts to `https://aicode.googleapis.com` (and the sandbox host to a
   sandbox aicode host), guarded by a package-level flag. Which build or runtime state sets
   that flag is not established; the observed `loadCodeAssist`/`fetchAvailableModels` traffic
   proves it is **off** in 1.4.8 on the Cloud Code management path. Whether the consumer
   inference path reaches AICode via this rewrite or via
   `StreamGenerateContentFromAICode`'s own `getAICodeURL()` is also open — both exist.
   **Partly resolved:** a live turn shows `aicode.googleapis.com` is used for
   `GET /v1alpha:fetchConfig`, while prompt inference goes to
   `daily-cloudcode-pa.googleapis.com/v1internal:streamGenerateContent`. So the rewrite is ON
   for config and OFF for inference in 1.4.8 on the consumer path.
3. **SeatManagementService routing.** `GetUserStatus`, `GetPlanStatus`, `MigrateApiKey`,
   `RegisterGdmUser`, `SelfAssignLicense` exist as `(*CodeAssistClient)` methods but I did not
   recover the route strings they build (the `/v1internal:` table has no seat-management
   entries, so they must use a different prefix — possibly bare `/v1/…` on the same host).
4. **Keepalive and retry constants.** `modelapiccpa.(*keepAlives)` intervals and
   `continuation.SleepForMidStreamRetry` backoff are immediates inside short BOLT fragments;
   not decoded.
5. **Raw header capture — closed (§8.1 "The inference call").** Observed by running one
   `agy -p` turn behind a local TLS-terminating proxy: the complete inference request head is
   `Host`, `User-Agent`, `Transfer-Encoding: chunked`, `Authorization: Bearer <oauth token>`,
   `Content-Type: application/json`, `Accept-Encoding: gzip` — and nothing else.
   `Authorization` is `Bearer`, observed rather than derived. Every `x-goog-*` header the code
   suggested (`X-Goog-Request-Reason`, `x-goog-sherlog-Link`, `X-Goog-User-Project`,
   `goog-originating-service-id`, `X-Vertex-AI-LLM-Shared-Request-Type`, `X-Aicode-Request-Id`)
   was **absent**, and the trace id is response-only. Copy the seven observed headers, not the
   code's full list.
6. **Telemetry kill switch — half closed.** A live agent turn **does** send Clearcut:
   three `POST https://play.googleapis.com/log` batches (36,240 / 29,415 / 51,014 bytes,
   ~117 KB for a one-word prompt) of `application/octet-stream`, with
   `User-Agent: Go-http-client/1.1` and the user's `Bearer` token. So operational telemetry
   fires on a turn and is tied to the account identity, unlike the product-UA'd
   `v1internal` calls. What remains open is only the switch itself: whether
   `data_collection_optin = false` suppresses this hop. Testing that needs an account-level
   setting change, which was not made, so the answer stays unknown rather than inferred. What
   `/clearcut_proxy` forwards is also still unrecovered; note that this capture shows the
   client posting to `play.googleapis.com` directly.
7. **`SENTRY_DSN`.** No DSN is embedded; the only activation is the env var. Whether the
   shipped `agy install` wrapper or the updater sets it was not checked.
8. **`ApiService` (boq) and the mesh/proxy routes** (`connectInstanceV2`, `proxyCommand`,
   `initiateMeshSession`) suggest a remote-sandbox transport that the CLI may enable under a
   flag; not exercised here.
