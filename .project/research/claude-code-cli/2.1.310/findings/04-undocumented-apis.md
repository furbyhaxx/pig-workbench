---
document:
  type: "research"
  id: "RES-claude-code-cli-2.1.310-findings-04-undocumented-apis"
  title: "Claude Code 2.1.310 HTTP surfaces and URL resolution"
  description: "Offline inventory of shipped JavaScript, including account APIs, session ingress, flags, artifacts, telemetry and client identity."
  status: "published"
  revision: 3
  created_at: "2026-10-26T12:15:48+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "worker"
  updated_by: "orchestrator"
  source: "pi-quotas@78268296fed62ba14601b8cb80f1ff45a5c04796:.project/research/claude-binary-re/findings/04-undocumented-apis.md"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/claude-code-cli/binary-re/findings/04-undocumented-apis.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 2
      reason: "imported into pi-auth-plus"
      changes: ["copied from pi-quotas@78268296fed62ba14601b8cb80f1ff45a5c04796:.project/research/claude-binary-re/findings/04-undocumented-apis.md"]
    - revision: 3
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/claude-code-cli/binary-re/findings/04-undocumented-apis.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---
# Undocumented HTTP surfaces — Claude Code 2.1.310
Scope: supplied extracted corpus of the installed Bun executable; **no network requests or target execution**. O = observed literal/static call, I = inferred semantics, ? = unknown. This documents shipping code, including dormant and vendored modules, not a supported API contract or permission to reuse it.
Evidence notation: `C<line>:<character-offset>` identifies a hit in `/tmp/cc_code.txt` (1-based lines, 0-based characters). These are **not binary offsets**. Use `tools/ctx.py '<literal>' --files code --window 700 --max 4`. Table placeholders replace JavaScript interpolation expressions; IDs are positional, not a claim that their variable names match. Methods aggregated across call sites; a path hit alone does not establish invocation.

## 1. Base URL resolution
**There is no single universal API base.** Inference, account/OAuth, flags, frame routing, session ingress and telemetry have separate resolvers.
- **Provider selection (O/I):** `if(Wr()||Rit()||Pit())return"gateway"`, then `CLAUDE_CODE_USE_BEDROCK` > `CLAUDE_CODE_USE_FOUNDRY` > `CLAUDE_CODE_USE_ANTHROPIC_AWS` > `CLAUDE_CODE_USE_ANTHROPIC_GOOGLE_CLOUD` > `CLAUDE_CODE_USE_MANTLE` > `CLAUDE_CODE_USE_VERTEX` > `"firstParty"` (C13798:31130). This is provider-selection precedence, not proof that every contradictory environment combination is valid. `Rit` reads gateway-server-process state and `Pit` reads host-policy-required gateway state (C12989:105385/105540).
- **First-party inference:** WIF credential resolution says `let s=i.ANTHROPIC_BASE_URL||r.base_url,o=s||"https://api.anthropic.com"` (C18312:16531, find `resolvedBaseUrlSnapshot` / `ANTHROPIC_BASE_URL||r.base_url`). Inference SDK receives `baseURL:pe` with `pe=i.ANTHROPIC_BASE_URL||le?.baseURL` in the WIF branch (C18502:33400); standard SDK defaults are separate. Files explicitly use `process.env.ANTHROPIC_BASE_URL||process.env.CLAUDE_CODE_API_BASE_URL||"https://api.anthropic.com"` (C21489:3012). Thus `CLAUDE_CODE_API_BASE_URL` is a real Files fallback, not an established universal override.
- **Negative search:** exact `ANTHROPIC_API_URL` search over both supplied corpora returned no matches. Do not document it as supported by this build.
- **Account/OAuth (O):** `function E(){return"prod"}`; production `BASE_API_URL:"https://api.anthropic.com"`, authorization at `platform.claude.com/oauth/authorize` or `claude.com/cai/oauth/authorize`, token at `platform.claude.com/v1/oauth/token`; MCP proxy `mcp-proxy.anthropic.com/v1/mcp/{server_id}` (C13401:0–2200). Local resolver contains `CLAUDE_LOCAL_OAUTH_API_BASE`, `CLAUDE_LOCAL_OAUTH_APPS_BASE`, `CLAUDE_LOCAL_OAUTH_CONSOLE_BASE` (defaults localhost:8000/4000/3000), but its selector is compiled to prod. Staging branch is `u??a`; its presence does not mean a user env flag activates it.
- **Custom account origin:** `CLAUDE_CODE_CUSTOM_OAUTH_URL` (one trailing slash stripped) must equal `https://beacon.claude-ai.staging.ant.dev`, `https://claude.fedstart.com`, or `https://claude-staging.fedstart.com`; otherwise `throw Error("CLAUDE_CODE_CUSTOM_OAUTH_URL is not an approved endpoint.")`. It rewrites account, authorization, token, roles and key-mint URLs, uses the custom credential-file suffix and turns off registered optional scopes. `CLAUDE_CODE_OAUTH_CLIENT_ID` independently overrides the public OAuth client ID and disables those scopes (C13401:3431 onward). Public client IDs are identifiers, not bearer secrets; their values are deliberately omitted.
- **Cloud modes:** observed override variables (C13798:33280): Bedrock `ANTHROPIC_BEDROCK_BASE_URL`, `AWS_ENDPOINT_URL_BEDROCK_RUNTIME`, `AWS_ENDPOINT_URL_BEDROCK`, `AWS_ENDPOINT_URL`; Mantle `ANTHROPIC_BEDROCK_MANTLE_BASE_URL`; Anthropic AWS `ANTHROPIC_AWS_BASE_URL`; Vertex `ANTHROPIC_VERTEX_BASE_URL`; Anthropic Google Cloud `ANTHROPIC_GOOGLE_CLOUD_BASE_URL`; Foundry `ANTHROPIC_FOUNDRY_BASE_URL` > resource-derived `https://<ANTHROPIC_FOUNDRY_RESOURCE>.services.ai.azure.com` (C18502:8273). The Bedrock list is also a host-validation input; SDK-specific endpoint precedence beyond the explicit Foundry expression remains unverified.
- **Cloud auth/routing context:** `CLAUDE_CODE_SKIP_BEDROCK_AUTH`, `CLAUDE_CODE_SKIP_VERTEX_AUTH`, `CLAUDE_CODE_SKIP_FOUNDRY_AUTH`, `CLAUDE_CODE_SKIP_MANTLE_AUTH`, `CLAUDE_CODE_SKIP_ANTHROPIC_AWS_AUTH`, `CLAUDE_CODE_SKIP_ANTHROPIC_GOOGLE_CLOUD_AUTH`; `AWS_REGION` > `AWS_DEFAULT_REGION` > shared-config region > us-east-1; `AWS_PROFILE`, `AWS_CONFIG_FILE`, `AWS_SHARED_CREDENTIALS_FILE`, AWS credential/session variables, `AWS_BEARER_TOKEN_BEDROCK`, `ANTHROPIC_FOUNDRY_API_KEY`, `ANTHROPIC_AWS_API_KEY`, `ANTHROPIC_VERTEX_PROJECT_ID`, `CLOUD_ML_REGION`, per-model `VERTEX_REGION_*`, `GOOGLE_CLOUD_PROJECT` / `GCLOUD_PROJECT`, `GOOGLE_APPLICATION_CREDENTIALS`, `GOOGLE_CLOUD_QUOTA_PROJECT`, `ANTHROPIC_GOOGLE_CLOUD_PROJECT`, `ANTHROPIC_GOOGLE_CLOUD_LOCATION`, `ANTHROPIC_{AWS,GOOGLE_CLOUD}_WORKSPACE_ID` are observed context inputs (C13414 export inventory, C13777 auth families, C14743 AWS resolver, C18502 clients). This is not an exhaustive inventory of vendored AWS/Google SDK environment variables.
- **Gateway is a distinct mode:** the inference client uses `baseURL:oe.url,authToken:oe.jwt`, `Authorization: Bearer`, `apiKey:null`, after host-credential refresh (C18502:26723–27544). Host-managed selection checks `CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST`, `CLAUDE_CODE_USE_GATEWAY`, approved entrypoint or `CLAUDE_CODE_HOST_GATEWAY_LINEAGE&&!!CLAUDE_CODE_HOST_CREDS_FILE` (C13566:2787). Gateway bootstrap also emits `inferenceProvider:"gateway",inferenceGatewayBaseUrl:o,inferenceCredentialKind:"interactive"` (search `function ez(e,t,n,o)`). Desktop schema observes `provider:"gateway"` / flat key `inferenceGatewayBaseUrl`, `inferenceStreamIdleTimeoutSec` (300–1800), `inferenceCredentialKind`; external-idp uses OIDC discovery + PKCE and ID/access-token selection. On Bedrock external-idp requires a validating proxy, never direct Bedrock (search `inferenceGatewayBaseUrl`). Desktop schema is bundled code, not proof that a CLI JSON setting of that name is honored directly.
- **Standalone gateway precedence (O):** `Su()` first checks `CLAUDE_CODE_USE_GATEWAY`, requires `ANTHROPIC_BASE_URL`, then chooses `i.ANTHROPIC_AUTH_TOKEN||lAt()`; missing URL/token logs a warning and ignores the mode. It preserves already-held strict gateway credentials or the same token, otherwise normalizes the URL and records an `unpinned:!0` gateway credential, expiry from JWT or a sentinel (C15190:97626–98011). `lAt()` reads its cached/snapshot credential before `CLAUDE_CODE_GATEWAY_TOKEN_FILE_DESCRIPTOR`; `CLAUDE_BG_AUTH_SNAPSHOT_PATH` is the snapshot handoff (C14577:5981/9026). URL normalizer prepends HTTPS if absent, trims trailing slashes, and only allows plain HTTP for localhost/127.0.0.1/[::1] (C15162:114555). Thus this mode does not require desktop host lineage. Normal API-key/OAuth credential inputs also include `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_CUSTOM_HEADERS`, `CLAUDE_CODE_OAUTH_TOKEN`, `CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR`, `CLAUDE_CODE_API_KEY_FILE_DESCRIPTOR`, `CCR_OAUTH_TOKEN_FILE`, saved login and apiKeyHelper (C15168:62060 onward); they do not all route through the gateway token reader.
- **Separate surfaces:** `SESSION_INGRESS_URL`, `CLAUDE_BRIDGE_BASE_URL`, `CLAUDE_BRIDGE_SESSION_INGRESS_URL`, `CLAUDE_REMOTE_TOOLS_BRIDGE_URL`, `SELF_HOSTED_RUNNER_AGENT_PROXY_URL`; artifact/memory env families `CLAUDE_CODE_ARTIFACTS_API_BASE_URL`, `CLAUDE_CODE_ARTIFACT_ASSET_BASE_URL`, `CLAUDE_CODE_ARTIFACT_LIVE_BASE_URL`, `CLAUDE_CODE_ARTIFACT_SYNC_BASE_URL`, `CLAUDE_CODE_ARTIFACT_VIEWER_BASE_URL`, `CLAUDE_CODE_MEMORY_API_BASE_URL`; `VOICE_STREAM_BASE_URL`, `CLAUDE_CODE_GB_BASE_URL`, `CLAUDE_CODE_GB_REFRESH_INTERVAL_MS` appear in the environment policy/export lists (C13777:4309–7979). **Presence is not support:** production frame `function Vii(){return ae()??rn().BASE_API_URL}function ae(){return}` (C14698:2263); GrowthBook host override is also compiled away (§5).
- **Host boundaries:** account-origin trust is current `rn().BASE_API_URL` plus `PW=new Set(["https://api.anthropic.com","https://api-staging.anthropic.com"])`; custom OAuth disables that extra fallback. URL guard requires HTTPS, no userinfo/explicit port, trusted origin. `OW=new Set(["authorization","proxy-authorization","x-authorization","x-api-key","cookie"])` enumerates sensitive headers (C15228:112168). Cloud `ownHost` recognizers separately classify AWS regional/VPC endpoints, Google aiplatform/claude googleapis hosts, and Foundry services.ai/cognitiveservices/openai Azure hosts (C13798:33280); these are provider-origin classifiers, not a universal outbound allowlist. `Hu` only recognizes `api.anthropic.com` as first-party (C13798:32804); `_CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL` can alter eligibility, not server identity.
- **Transport is another layer:** `ANTHROPIC_UNIX_SOCKET`, `HTTP_PROXY`, `HTTPS_PROXY`, `ALL_PROXY`, `NO_PROXY` (lowercase variants), `CLAUDE_CODE_HTTP_PROXY`, `CLAUDE_CODE_HTTPS_PROXY`, `NODE_EXTRA_CA_CERTS`, `SSL_CERT_FILE`, `SSL_CERT_DIR`, `CLAUDE_CODE_CLIENT_CERT`, `CLAUDE_CODE_CLIENT_KEY`, `CLAUDE_CODE_CLIENT_KEY_PASSPHRASE` affect connection/auth boundaries, not a single common URL precedence (C13776/C13777, C15168 proxy conversion).

## 2. Endpoint inventory
Host shorthand: **A** = account resolver `rn().BASE_API_URL` (normally api.anthropic.com); **F** = frame route (A directly, or session agent-proxy relay); **S** = negotiated session/ingress host; **G** = configured gateway; **D** = configured inference SDK base (normally api.anthropic.com). Host assignments are I, not captured traffic.
Auth shorthand: **O** = OAuth bearer; **K** = API key; **J** = session-ingress JWT/bearer; **GJ** = gateway token; **W** = wrapper-resolved required/async auth (may select O/K/WIF); **N** = no caller credential; **?** = not resolved. **U** (user-facing) is inferred: Y = user action/surface, B = background machinery, SDK = bundled definition only, ? = unresolved. Unknown method/body is intentional; no missing verb is guessed from a path's name.
**Mutation warning:** `POST /api/organizations/{org}/reset_rate_limits` is an authenticated state-changing reset, not usage retrieval. Cedar body `{program:ae,grant_id:n,request_id:r}` with `ae="cedar_ember"` (C56098:8513/13567); Juniper body `{program:Sr}` with `Sr="juniper_tide"` (C56111:36227/38834). Both use `auth:"async"`, `refreshOAuth:!0`, organization from the OAuth identity, JSON and 25 s timeout. `request_id` is the Cedar idempotency/correlation field; the static client preserves unsettled claim IDs, but no server deduplication guarantee was tested. Parsed outcomes include `reset | already_used | not_limited | cooldown | ineligible | unavailable`; client failures additionally distinguish rate_limited/auth_error/error. **A third-party client must not call this implicitly**, including while refreshing quota status.
The org-scoped inventory below includes onboarding, model_selector/state, projects, MCP start-auth, plugin assets, remote devices and reset_rate_limits under `/api/organizations/...`, and separate `/api/oauth/organizations/:orgUUID/...` billing/plugin/skill/connector families. A bare `/api/organizations` prefix alone does not establish a collection-fetch endpoint.
One row per normalized route; beta/non-beta SDK query variants share a row. SDK rows prove shipped surface definitions, **not that the CLI invokes them**; SDK `?beta=true` variants occur in C18463/C18486 and stable variants in C18486/C18487. Each evidence cell points to the actual literal/template hit. Prefix-only allowlist entries are identified as prefixes, not concrete callable routes.

### Account / OAuth and organization settings
| Path | Method | Host | Auth | Purpose / request shape (I unless quoted) | U | Evidence |
|---|---|---|---|---|---|---|
| `/api/oauth/account/grove_notice_viewed` | POST | A | W | Acknowledge notice; {} | Y | C38811:1247 |
| `/api/oauth/account/settings` | GET/PATCH | A | O | Account preferences incl. Grove and artifact preferences; settings object | Y | C35091:19429 |
| `/api/oauth/claude_cli/create_api_key` | POST | A | O | Mint Console API key; body not reconstructed | Y | C13401:1364 |
| `/api/oauth/claude_cli/roles` | GET | A | O | Account/org roles | B | C13401:1438 |
| `/api/oauth/cri` | issuer literal | api.anthropic.com | ? | CRI JWT issuer constant; request verb not established | B | C50258:86291 |
| `/api/oauth/cri/jwks.json` | GET | A | ? | cri / jwks.json operation (name); body/query ? | ? | C50258:86291 |
| `/api/oauth/file_upload` | POST | A | ? | oauth / file_upload operation (name); body/query ? | ? | C55579:8287 |
| `/api/oauth/files/{id}/content` | GET | A | O/? | OAuth file content; fetch with bounded abort handling | Y | C57591:27252 |
| `/api/oauth/local_pairing` | issuer literal | account origin | ? | Local-pairing attestation issuer; base, not a proven callable route | B | C39990:6225 |
| `/api/oauth/local_pairing/attest` | ? | A | ? | local_pairing / attest operation (name); body/query ? | ? | C39990:6225 |
| `/api/oauth/local_pairing/jwks.json` | GET | A | ? | local_pairing / jwks.json operation (name); body/query ? | ? | C39990:6225 |
| `/api/oauth/organizations/:orgUUID/admin_requests` | POST | A | ? | organizations / admin_requests operation (name); body/query ? | ? | C42623:835 |
| `/api/oauth/organizations/:orgUUID/admin_requests/eligibility` | GET | A | ? | admin_requests / eligibility operation (name); body/query ? | ? | C42623:1531 |
| `/api/oauth/organizations/:orgUUID/admin_requests/me` | GET | A | ? | admin_requests / me operation (name); body/query ? | ? | C42623:1241 |
| `/api/oauth/organizations/:orgUUID/billing/tax_rate` | POST | A | O (teleport-org) | Tax preview; {product_id,price,currency} | Y | C25110:8773 |
| `/api/oauth/organizations/:orgUUID/claude_code/pro_trial` | POST | A | ? | claude_code / pro_trial operation (name); body/query ? | ? | C38868:681 |
| `/api/oauth/organizations/:orgUUID/contracts/auto_reload_settings` | PUT | A | O (teleport-org) | {enabled,threshold_in_minor_units?,reload_to_in_minor_units?,currency} | Y | C25110:4607 |
| `/api/oauth/organizations/:orgUUID/contracts/prepaid/credits` | POST | A | O (teleport-org) | Purchase credits; expected-price/amount payload; full schema unresolved | Y | C25110:8214 |
| `/api/oauth/organizations/:orgUUID/marketplaces` | GET (I) | A | O (teleport-org) | Marketplace registrations/catalog; schema not reconstructed | Y | C24427:25606 |
| `/api/oauth/organizations/:orgUUID/marketplaces/{id}/plugins` | GET | A | O (teleport-org) | Catalog pagination; ETag and cursor | Y | C24427:32751 |
| `/api/oauth/organizations/:orgUUID/mcp/connectors/list` | POST | A | O (teleport-org) | Connector listing; arguments object | Y | C55082:809 |
| `/api/oauth/organizations/:orgUUID/mcp/connectors/search` | POST | A | O (teleport-org) | Connector discovery; arguments object | Y | C55082:688 |
| `/api/oauth/organizations/:orgUUID/mcp/connectors/suggest` | POST | A | O (teleport-org) | Connector suggestions; arguments object | Y | C55082:748 |
| `/api/oauth/organizations/:orgUUID/overage_credit_grant` | GET/POST | A | ? | organizations / overage_credit_grant operation (name); body/query ? | ? | C53059:3450 |
| `/api/oauth/organizations/:orgUUID/overage_spend_limit` | PUT | A | O (teleport-org) | {is_enabled,monthly_credit_limit?,currency?}; billing mutation | Y | C25110:3371 |
| `/api/oauth/organizations/:orgUUID/payment_method` | GET | A | O (teleport-org) | Saved payment method | Y | C25110:7192 |
| `/api/oauth/organizations/:orgUUID/plugin_ratings` | POST/DELETE | A | O (teleport-org) | marketplace_name,plugin_name,rating,client_surface,client_version | Y | C57281:31155 |
| `/api/oauth/organizations/:orgUUID/plugin_ratings/appearances` | POST | A | O (teleport-org) | marketplace_name,plugin_name,client_surface,appearance_id | Y | C57281:30831 |
| `/api/oauth/organizations/:orgUUID/plugin_ratings/appearances/outcome` | POST | A | O (teleport-org) | marketplace_name,plugin_name,outcome=dismissed/unsure | Y | C57281:31899 |
| `/api/oauth/organizations/:orgUUID/plugins/list-plugins` | GET | A | O (teleport-org) | enabled_only,compact,component details/hooks/wrapped-skills options | Y | C20586:126 |
| `/api/oauth/organizations/:orgUUID/plugins/search` | ? | A | ? | plugins / search operation (name); body/query ? | ? | C46455:7044 |
| `/api/oauth/organizations/:orgUUID/plugins/{id}/download{id}` | ? | A | ? | plugins / download{id} operation (name); body/query ? | ? | C20586:2480 |
| `/api/oauth/organizations/:orgUUID/prepaid/bundles` | GET | A | O (teleport-org) | Available prepaid purchase bundles | Y | C25110:6653 |
| `/api/oauth/organizations/:orgUUID/prepaid/commits/{id}` | GET | A | O (teleport-org) | Credit purchase status | Y | C25110:9494 |
| `/api/oauth/organizations/:orgUUID/prepaid/credits` | GET | A | O (teleport-org) | Prepaid balance; amount required; currency/expiry returned | Y | C25110:5364 |
| `/api/oauth/organizations/:orgUUID/quirky-lollipop/link-offer` | GET | A | ? | quirky-lollipop / link-offer operation (name); body/query ? | ? | C56413:75967 |
| `/api/oauth/organizations/:orgUUID/referral/eligibility` | GET | A | ? | referral / eligibility operation (name); body/query ? | ? | C21503:4855 |
| `/api/oauth/organizations/:orgUUID/referral/redemptions` | GET | A | ? | referral / redemptions operation (name); body/query ? | ? | C21503:5220 |
| `/api/oauth/organizations/:orgUUID/setup_overage_billing` | POST | A | O (teleport-org) | Enable billing; {org_monthly_spend_limit} | Y | C25110:3142 |
| `/api/oauth/organizations/:orgUUID/skills/list-skills` | GET | A | O (teleport-org) | include_wiggle_skills,entrypoint,wrapped_skills_as_plugins | Y | C34832:489 |
| `/api/oauth/organizations/:orgUUID/skills/search` | ? | A | ? | skills / search operation (name); body/query ? | ? | C46455:7097 |
| `/api/oauth/organizations/:orgUUID/skills/{id}/download{id}` | GET | A | ? | skills / download{id} operation (name); body/query ? | ? | C34832:1520 |
| `/api/oauth/organizations/:orgUUID/sync/github/auth` | GET | A | ? | github / auth operation (name); body/query ? | ? | C42713:1804 |
| `/api/oauth/organizations/{id}` | ? | A | ? | oauth / organizations operation (name); body/query ? | ? | C54880:3038 |
| `/api/oauth/organizations/{id}/code/repos/resync` | ? | A | ? | repos / resync operation (name); body/query ? | ? | C21488:17947 |
| `/api/oauth/organizations/{id}/code/repos/{id}/{id}` | ? | A | ? | code / repos operation (name); body/query ? | ? | C21488:14867 |
| `/api/oauth/organizations/{id}/sync/github/auth` | ? | A | ? | github / auth operation (name); body/query ? | ? | C21488:20280 |
| `/api/oauth/profile` | GET | A | O | Account/org profile; no-cache | B | C15166:17591 |
| `/api/oauth/usage` | GET | A | O | Quota windows/spend; plain or cedar_ember=1 / at_wall=1 / starry_waterfall=<surface>, skip_spend=1 | Y | C20934:11049 |
| `/api/oauth/validate` | POST | A | O | Credential/scope validation; null body | B | C15166:19212 |
| `/v1/oauth/hello` | GET | token origin | N | OAuth connectivity probe | Y | C59621:21659 |
| `/v1/oauth/token` | POST | platform.claude.com | N | OAuth code/refresh grant; client_id, grant_type, code or refresh_token | Y | C13401:1309 |
| `/v1/oauth/token/revoke` | POST | platform.claude.com | N | Revoke; token, token_type_hint=refresh_token, client_id | Y | C15166:24848 |

### Claude Code, marketplace and desktop
| Path | Method | Host | Auth | Purpose / request shape (I unless quoted) | U | Evidence |
|---|---|---|---|---|---|---|
| `/api/artifacts/mcp` | MCP | A | ? | Artifact MCP identity/path; transport auth not established here | Y | C14456:985 |
| `/api/auth/trusted_devices` | POST | A | O | Device enrollment; display_name | Y | C20810:7966 |
| `/api/cask/{id}.json` | GET | formulae.brew.sh | N | Homebrew cask update metadata; not Anthropic | B | C29372:2926 |
| `/api/claude_cli/bootstrap` | GET | A or inference override | O/K/WIF | Startup client_data, model access/options/costs/profile; query entrypoint,model | B | C34387:6271 |
| `/api/claude_cli_feedback` | POST | A | W | Description, transcript, sanitized error log; size/policy checks | Y | C14746:443725 |
| `/api/claude_cli_profile` | GET | A | K | Profile; query account_uuid | B | C15166:16994 |
| `/api/claude_code/notification/preferences` | GET/PATCH | A | W | Push preferences; bogosort/code_requires_action enable_push | Y | C44317:1552 |
| `/api/claude_code/organizations/metrics_enabled` | GET | A | W | Organization telemetry opt-out check | B | C43820:634 |
| `/api/claude_code/policy_limits` | GET | A | W | Organization feature policy; request body absent | B | C14698:2385 |
| `/api/claude_code/settings` | GET | A | W | Managed settings; conditional If-None-Match checksum | B | C31539:2882 |
| `/api/claude_code/skills` | GET | A | W | Skill health/dashboard data; async auth | Y | C63339:7858 |
| `/api/claude_code_grove` | GET | A | W | Grove notice/config; grove_enabled, domain_excluded, grace/reminder controls | B | C38811:3175 |
| `/api/claude_code_penguin_mode` | GET | A | O/K | Fast-mode organization eligibility; no body | B | C15168:47616 |
| `/api/claude_code_shared_session_transcripts` | POST | A | W | Share session transcript; transcript payload | Y | C57274:11201 |
| `/api/desktop/darwin/universal/dmg/latest/redirect` | URL/link | claude.ai | N/? | Desktop installer redirect | Y | C64298:20554 |
| `/api/desktop/win32/x64/exe/latest/redirect` | URL/link | claude.ai | N/? | Desktop installer redirect; not an API fetch proven here | Y | C64298:20478 |
| `/api/directory/plugins` | GET (guard) | A | N (exception) | Catalog policy exception without q; does not prove actual fetch/signing | Y | C15228:74286 |
| `/api/directory/servers` | GET | A | N | Remote server directory; limit=500,visibility,cursor | Y | C15228:123379 |
| `/api/eval-authed/{id}` | POST | api.anthropic.com/ (GB) | W | Authed evaluation; same payload, fallback /api/eval | B | C15227:27254 |
| `/api/eval/{id}` | POST | api.anthropic.com/ (GB) | W | Remote evaluation; attributes,forcedVariations,forcedFeatures,url | B | C15133:11642 |
| `/api/features/{id}` | GET | api.anthropic.com/ (GB) | W/? | GrowthBook SDK feature payload; path embeds client key | B | C15133:11425 |
| `/api/graphql` | ? | caller-supplied GitHub host | ? | GitHub Enterprise URL builder; not Anthropic | Y | C13169:1323 |
| `/api/hello` | GET | A | N | Connectivity probe | Y | C31579:5098 |
| `/api/model_selector/{id}` | ? | A | O/K/WIF | Model selector API; claude-ai-oauth / required / async credential paths | Y | C37141:3275 |
| `/api/organization/claude_code_first_token_date` | GET | A | W | First-use date; async auth | B | C49157:1106 |
| `/api/organizations/:orgUUID/claude_code/onboarding` | POST/GET | A | ? | claude_code / onboarding operation (name); body/query ? | ? | C25447:14879 |
| `/api/organizations/:orgUUID/claude_code/onboarding/{id}` | PUT/DELETE | A | ? | claude_code / onboarding operation (name); body/query ? | ? | C25447:15088 |
| `/api/organizations/:orgUUID/cowork/remote_devices` | ? | A | ? | cowork / remote_devices operation (name); body/query ? | ? | C44917:6706 |
| `/api/organizations/:orgUUID/marketplace/plugin-assets/{id}` | GET | A | ? | marketplace / plugin-assets operation (name); body/query ? | ? | C27687:55082 |
| `/api/organizations/:orgUUID/model_selector/{id}` | ? | A | O (teleport-org) | Organization-bound model selector variant | Y | C37141:3169 |
| `/api/organizations/:orgUUID/model_selector_state/{id}` | PATCH | A | ? | organizations / model_selector_state operation (name); body/query ? | ? | C42889:2197 |
| `/api/organizations/:orgUUID/projects/{id}` | ? | A | ? | organizations / projects operation (name); body/query ? | ? | C33912:1321 |
| `/api/organizations/{id}/mcp/start-auth/{id}` | ? | A | ? | mcp / start-auth operation (name); body/query ? | ? | C20378:7589 |
| `/api/organizations/{id}/reset_rate_limits` | POST — MUTATION | A | async; refreshed OAuth (I) | Explicit reset: {program:cedar_ember,grant_id,request_id} or {program:juniper_tide}; NEVER implicit | Y | C56098:13544 |
| `/api/web/domain_info` | GET | api.anthropic.com | N | WebFetch preflight; domain=<hostname>; response can_fetch | B | C26758:590 |
| `/api/ws/speech_to_text/voice_stream` | WS | A (WS scheme) | O | Streaming audio bytes; KeepAlive / CloseStream JSON controls; keyterms header | Y | C59870:709 |

### Frame / artifacts
| Path | Method | Host | Auth | Purpose / request shape (I unless quoted) | U | Evidence |
|---|---|---|---|---|---|---|
| `/api/frame/blob/{id}/agent-copy` | ? | F | ? | blob / agent-copy operation (name); body/query ? | Y | C34985:9592 |
| `/api/frame/blob/{id}/agent-list` | ? | F | ? | blob / agent-list operation (name); body/query ? | Y | C34985:9473 |
| `/api/frame/blob/{id}/agent-upload` | ? | F | ? | blob / agent-upload operation (name); body/query ? | Y | C34985:9416 |
| `/api/frame/blob/{id}/{id}/agent-delete` | ? | F | ? | blob / agent-delete operation (name); body/query ? | Y | C34985:9530 |
| `/api/frame/comments/{id}` | GET | F | ? | frame / comments operation (name); body/query ? | Y | C32051:10317 |
| `/api/frame/comments/{id}/{id}` | POST | F | ? | frame / comments operation (name); body/query ? | Y | C32051:18616 |
| `/api/frame/comments/{id}/{id}/resolve` | POST | F | ? | comments / resolve operation (name); body/query ? | Y | C32051:23986 |
| `/api/frame/comments/{id}/{id}/summon-status` | POST | F | ? | comments / summon-status operation (name); body/query ? | Y | C32051:22665 |
| `/api/frame/comments/{id}/{id}/{id}` | ? | F | ? | frame / comments operation (name); body/query ? | Y | C32051:25852 |
| `/api/frame/content/{id}` | GET | F | asset token + O/J | Read content; x-frame-asset-token | Y | C32037:12908 |
| `/api/frame/contract/latest` | GET | F | O/J relay | Artifact capability contract; no body | B | C31975:10561 |
| `/api/frame/contract/{id}` | GET | F | O/J relay | Pinned contract version | B | C31975:12923 |
| `/api/frame/contract/{id}/prompt` | GET | F | O/J relay | Contract prompt; max 64 KiB | B | C31975:13590 |
| `/api/frame/contract/{id}/{id}.d.ts` | GET | F | O/J relay | Capability TypeScript definitions; max 256 KiB | B | C31975:13248 |
| `/api/frame/db/agent` | ? | F | O/J relay | Artifact data-store operations; schema not recovered | Y | C34924:1128 |
| `/api/frame/deploy/direct` | POST | F | O/J relay | Artifact deploy/body upload; attempt_id,resend,create-confirmation queries | Y | C31993:19204 |
| `/api/frame/deploy/prepare` | POST | F | O/J relay | Reserve/prepare slug; metadata builder; returns slug | Y | C31993:15677 |
| `/api/frame/diag/{id}/{id}` | GET | F | ? | frame / diag operation (name); body/query ? | Y | C34937:2259 |
| `/api/frame/favorite/{id}` | POST/DELETE | F | O/J relay | Pin/unpin artifact | Y | C35091:17396 |
| `/api/frame/frames` | GET | F | O/J relay | List published artifacts; limit | Y | C31996:41909 |
| `/api/frame/frames/external` | GET | F | O/J relay | List external artifacts | Y | C31975:9717 |
| `/api/frame/perm/{id}` | GET/POST (I) | F | O/J relay | Sharing policy; mode,access,user_uuids; optional org query | Y | C35091:7884 |
| `/api/frame/read/{id}` | GET | F | O/J relay | Read back source/metadata | Y | C31983:387 |
| `/api/frame/retitle/{id}` | POST | F | O/J relay | {title} | Y | C64468:24925 |
| `/api/frame/subscribe/{id}` | POST | F | O/J relay | {url,sealed_secret,events}; watch-trigger subscription | Y | C54010:5007 |
| `/api/frame/sync` | WS (I) | F (WS scheme) | watch token/? | slug,comment_hint,vn; artifact sync channel | B | C32371:10846 |
| `/api/frame/track` | POST | F | O/J relay | {event_name,slug?,via?,mode?} | B | C31993:16880 |
| `/api/frame/types` | GET | F | O/J relay | Artifact types directory; q/category/limit | Y | C34944:3249 |
| `/api/frame/types/{id}` | GET | F | O/J relay | Type metadata | Y | C34944:3323 |
| `/api/frame/types/{id}/create` | POST (I) | F | O/J relay | Create artifact from type; body not reconstructed | Y | C34943:1532 |
| `/api/frame/types/{id}/files/{id}` | GET | F | O/J relay | Type source file | Y | C34944:3505 |
| `/api/frame/types/{id}/instances` | GET | F | O/J relay | Type instances; limit,rel | Y | C34944:3390 |
| `/api/frame/unsubscribe/{id}` | POST | F | O/J relay | {}; release watch trigger | Y | C54010:7740 |
| `/api/frame/upload` | POST | F | O/J relay | {slug,cross_org_ok?,files:[...]} | Y | C31996:17755 |
| `/api/frame/user/profiles/{id}` | POST | F | O/J relay | {ids}; comment/profile lookup | B | C54041:10409 |
| `/api/frame/versions/{id}` | GET | F | O/J relay | Version history | Y | C35091:15015 |
| `/api/frame/watch-token/{id}` | POST | F | O/J relay | {}; vn query; live watch-token renewal | B | C32037:10077 |
| `/api/frame/{id}` | GET/DELETE | F | O/J relay | Artifact metadata/watch negotiation or delete | Y | C32037:2821 |

### Session / ingress / MCP and remote control
| Path | Method | Host | Auth | Purpose / request shape (I unless quoted) | U | Evidence |
|---|---|---|---|---|---|---|
| `/v1/code/.well-known/jwks.json` | ? | A/S/? | ? | .well-known / jwks.json operation (name); body/query ? | ? | C29283:40 |
| `/v1/code/agent-proxy` | ? | A/S/? | ? | code / agent-proxy operation (name); body/query ? | ? | C15168:27961 |
| `/v1/code/auth/refresh` | POST | A/S/? | ? | auth / refresh operation (name); body/query ? | ? | C28439:15094 |
| `/v1/code/github/get-batch-branch-status` | ? | A/S/? | ? | github / get-batch-branch-status operation (name); body/query ? | ? | C25825:2938 |
| `/v1/code/github/import-token` | POST | A/S/? | ? | github / import-token operation (name); body/query ? | ? | C42713:938 |
| `/v1/code/github/post-review-comment` | ? | A/S/? | ? | github / post-review-comment operation (name); body/query ? | ? | C25785:1169 |
| `/v1/code/github/{id}-pull-request` | ? | A/S/? | ? | github / {id}-pull-request operation (name); body/query ? | ? | C23082:939 |
| `/v1/code/local/memory/credential` | POST | A | O/? | Scoped memory access credential; stores/expires_in_seconds | B | C19984:39976 |
| `/v1/code/local/memory/mounts` | GET | A | O/? | Memory store discovery; cached org-memory-discovery.json | B | C19984:46831 |
| `/v1/code/mcp/ccr-artifacts` | MCP | S/A | J/? | Remote artifact MCP counterpart | Y | C14456:952 |
| `/v1/code/mcp/hearthbot` | ? | A/S/? | ? | mcp / hearthbot operation (name); body/query ? | ? | C14508:18232 |
| `/v1/code/runners/self-hosted/deregister` | POST | A/S/? | ? | self-hosted / deregister operation (name); body/query ? | ? | C28439:14537 |
| `/v1/code/runners/self-hosted/pools` | ? | A/S/? | ? | self-hosted / pools operation (name); body/query ? | ? | C18793:18754 |
| `/v1/code/runners/self-hosted/pools/{id}` | ? | A/S/? | ? | self-hosted / pools operation (name); body/query ? | ? | C55045:6384 |
| `/v1/code/runners/self-hosted/pools/{id}/secrets` | ? | A/S/? | ? | pools / secrets operation (name); body/query ? | ? | C55045:9258 |
| `/v1/code/runners/self-hosted/pools/{id}/sessions{id}` | ? | A/S/? | ? | pools / sessions{id} operation (name); body/query ? | ? | C55045:7428 |
| `/v1/code/runners/self-hosted/runners` | ? | A/S/? | ? | self-hosted / runners operation (name); body/query ? | ? | C55045:8347 |
| `/v1/code/runners/self-hosted/runners/register` | POST | A/S/? | ? | runners / register operation (name); body/query ? | ? | C28439:9380 |
| `/v1/code/runners/self-hosted/runners/{id}/poll` | POST | A/S/? | ? | runners / poll operation (name); body/query ? | ? | C28439:11588 |
| `/v1/code/runners/self-hosted/runners/{id}/work-hints/stream` | ? | A/S/? | ? | work-hints / stream operation (name); body/query ? | ? | C29688:4542 |
| `/v1/code/runners/self-hosted/sessions/{id}/release` | POST | A/S/? | ? | sessions / release operation (name); body/query ? | ? | C28439:14142 |
| `/v1/code/runners/self-hosted/sessions/{id}/report-failure` | POST | A/S/? | ? | sessions / report-failure operation (name); body/query ? | ? | C28439:13579 |
| `/v1/code/runners/self-hosted/sessions/{id}/requeue` | ? | A/S/? | ? | sessions / requeue operation (name); body/query ? | ? | C55046:2412 |
| `/v1/code/runners/self-hosted/sessions/{id}/token` | ? | A/S/? | ? | sessions / token operation (name); body/query ? | ? | C28439:12933 |
| `/v1/code/runners/self-hosted/spawn-hints/nack` | POST | A/S/? | ? | spawn-hints / nack operation (name); body/query ? | ? | C28439:11028 |
| `/v1/code/runners/self-hosted/spawn-hints/poll` | POST | A/S/? | ? | spawn-hints / poll operation (name); body/query ? | ? | C28439:9968 |
| `/v1/code/scm-connectors/{id}/{id}/tunnel` | ? | A/S/? | ? | scm-connectors / tunnel operation (name); body/query ? | ? | C28652:2477 |
| `/v1/code/sessions` | GET/POST | A/S | O | List/create cloud sessions; creation session context/body varies | Y | C15228:92244 |
| `/v1/code/sessions/watch` | GET/SSE | A | O | include_trigger_sessions=true,exclude_tags=-; Last-Event-ID | Y | C48361:9334 |
| `/v1/code/sessions/{id}` | GET/PATCH | A/S | O/J | Fetch/update session; body varies | Y | C15228:95355 |
| `/v1/code/sessions/{id}/agent-proxy` | ? | A/S/? | ? | sessions / agent-proxy operation (name); body/query ? | ? | C15168:28142 |
| `/v1/code/sessions/{id}/archive` | POST | A/S | O/J | Archive session; {} | Y | C15228:105741 |
| `/v1/code/sessions/{id}/bridge` | ? | A/S/? | ? | sessions / bridge operation (name); body/query ? | ? | C40500:4680 |
| `/v1/code/sessions/{id}/client/presence` | ? | A/S/? | ? | client / presence operation (name); body/query ? | ? | C56591:1186 |
| `/v1/code/sessions/{id}/device` | ? | A/S/? | ? | sessions / device operation (name); body/query ? | ? | C57956:23987 |
| `/v1/code/sessions/{id}/events` | GET/POST | A/S | O/J | Read events; append {events:[{payload,device_attestation?}]} | Y | C15228:98031 |
| `/v1/code/sessions/{id}/events/stream` | GET/SSE | A/S | O/J | Session event stream/reconnection | Y | C56552:3432 |
| `/v1/code/sessions/{id}/file` | GET | A/S/? | ? | sessions / file operation (name); body/query ? | ? | C45165:9187 |
| `/v1/code/sessions/{id}/inbox/{id}` | ? | A/S/? | ? | sessions / inbox operation (name); body/query ? | ? | C46747:1660 |
| `/v1/code/sessions/{id}/mark_read` | POST | A | O | {event_id} or {} | Y | C15228:103480 |
| `/v1/code/sessions/{id}/move-to-cloud` | ? | A/S/? | ? | sessions / move-to-cloud operation (name); body/query ? | ? | C67316:27134 |
| `/v1/code/sessions/{id}/remote` | GET | A/S/? | ? | sessions / remote operation (name); body/query ? | ? | C28439:15474 |
| `/v1/code/sessions/{id}/sign-commit` | ? | A/S/? | ? | sessions / sign-commit operation (name); body/query ? | ? | C29246:1338 |
| `/v1/code/sessions/{id}/synced_file` | PUT | A/S/? | ? | sessions / synced_file operation (name); body/query ? | ? | C45165:8748 |
| `/v1/code/sessions/{id}/teleport-events` | ? | A/S/? | ? | sessions / teleport-events operation (name); body/query ? | ? | C24337:19044 |
| `/v1/code/sessions/{id}/tool_hosts/refuse` | POST | A/S/? | ? | tool_hosts / refuse operation (name); body/query ? | ? | C48361:8010 |
| `/v1/code/sessions/{id}/worker` | GET/PUT | S | J | Worker state; epoch-scoped updates | B | C28439:17166 |
| `/v1/code/sessions/{id}/worker/agent-metadata-events/batch-get` | ? | S | ? | agent-metadata-events / batch-get operation (name); body/query ? | ? | C47817:100981 |
| `/v1/code/sessions/{id}/worker/channels/{id}/files:write` | ? | S | ? | channels / files:write operation (name); body/query ? | ? | C23082:2813 |
| `/v1/code/sessions/{id}/worker/diagnostics` | POST | S | J | {worker_epoch,lines} | B | C28439:17872 |
| `/v1/code/sessions/{id}/worker/events` | GET/POST | S | J | {worker_epoch,events:[{payload}]} | B | C28439:16736 |
| `/v1/code/sessions/{id}/worker/events/delivery` | ? | S | J | Delivery acknowledgement; body not reconstructed | B | C47817:12049 |
| `/v1/code/sessions/{id}/worker/events/stream` | GET/SSE | S | J | Worker SSE with sequence/resume handling | B | C47818:8408 |
| `/v1/code/sessions/{id}/worker/events/{id}` | ? | S | ? | worker / events operation (name); body/query ? | ? | C47817:39887 |
| `/v1/code/sessions/{id}/worker/files` | ? | S | ? | worker / files operation (name); body/query ? | ? | C65358:11985 |
| `/v1/code/sessions/{id}/worker/heartbeat` | POST | S | J | {worker_epoch} | B | C28439:17616 |
| `/v1/code/sessions/{id}/worker/internal-events` | POST | S | J | Transcript/internal rows, serialized upload | B | C14746:113469 |
| `/v1/code/sessions/{id}/worker/otlp/{id}` | ? | S | ? | worker / otlp operation (name); body/query ? | ? | C18580:1101 |
| `/v1/code/sessions/{id}/worker/record-created-pr` | ? | S | ? | worker / record-created-pr operation (name); body/query ? | ? | C23082:1951 |
| `/v1/code/sessions/{id}/worker/register` | POST | S | J | {session_id}; returns worker epoch | B | C28439:16211 |
| `/v1/code/sessions/{id}/worker/sender_addr` | ? | S | ? | worker / sender_addr operation (name); body/query ? | ? | C18502:23450 |
| `/v1/code/sessions/{id}/worker/skill-manifest` | GET | S | J | Session skills manifest | B | C20586:7991 |
| `/v1/code/sessions/{id}/worker/synced_file` | GET/PUT | S | J | path,content(base64),if_match_sha256?,worker_epoch | B | C45007:6762 |
| `/v1/code/sessions/{id}/worker/wakes` | ? | S | ? | worker / wakes operation (name); body/query ? | ? | C66629:4707 |
| `/v1/code/triggers` | ? | A/S/? | ? | code / triggers operation (name); body/query ? | ? | C22770:12 |
| `/v1/code/triggers/{id}` | ? | A/S/? | ? | code / triggers operation (name); body/query ? | ? | C22771:11 |
| `/v1/code/triggers/{id}/run` | ? | A/S/? | ? | triggers / run operation (name); body/query ? | ? | C22774:12 |
| `/v1/code/webhook-triggers` | ? | A/S/? | ? | code / webhook-triggers operation (name); body/query ? | ? | C22775:31 |
| `/v1/environments/bridge` | POST | A/S/? | ? | environments / bridge operation (name); body/query ? | ? | C48361:2755 |
| `/v1/environments/bridge/{id}` | DELETE | A/S/? | ? | environments / bridge operation (name); body/query ? | ? | C48361:5817 |
| `/v1/environments/bridge/{id}/offline` | ? | A/S/? | ? | bridge / offline operation (name); body/query ? | ? | C48361:6183 |
| `/v1/environments/{id}/bridge/reconnect` | POST | A/S/? | ? | bridge / reconnect operation (name); body/query ? | ? | C48361:7053 |
| `/v1/session_ingress/session/{id}` | ? | A/S/? | ? | session_ingress / session operation (name); body/query ? | ? | C24337:18834 |
| `/v1/session_ingress/session/{id}/git_proxy/{id}/{id}.git` | ? | A/S/? | ? | git_proxy / {id}.git operation (name); body/query ? | ? | C29674:10533 |
| `/v2/ccr-sessions/-/chat-project` | POST | S/A | J | Chat project bridge; {op,...arguments} | Y | C33912:457 |
| `/v2/ccr-sessions/-/meta/mcp` | MCP | S | J/? | Remote metadata / trigger tooling | B | C14430:9 |
| `/v2/ccr-sessions/{id}/hearthbot/mcp` | ? | S | ? | hearthbot / mcp operation (name); body/query ? | ? | C14430:2092 |
| `/v2/ccr-sessions/{id}/mcp` | ? | S | ? | ccr-sessions / mcp operation (name); body/query ? | ? | C14508:16569 |
| `/v2/ccr-sessions/{id}/teleport/conversations/{id}/completion` | ? | S | ? | conversations / completion operation (name); body/query ? | ? | C55839:19737 |
| `/v2/session_ingress/mcp/ws/{suffix}` | WS prefix | S | J/? | WebSocket MCP candidate; suffix unresolved | B | C14508:8690 |
| `/v2/session_ingress/shttp/mcp/{suffix}` | MCP prefix | S | J/? | Streamable-HTTP MCP candidate; suffix unresolved | B | C14508:8657 |

### Telemetry and event logging
| Path | Method | Host | Auth | Purpose / request shape (I unless quoted) | U | Evidence |
|---|---|---|---|---|---|---|
| `/api/claude_code/metrics` | POST | A | W | First-party OTLP metric export; 5 s timeout | B | C52542:26989 |
| `/api/event_logging/v2/batch` | POST | api.anthropic.com or staging | O/K/optional | {events:[{event_type,event_data}]} | B | C15229:23081 |
| `/api/v2/logs` | POST | Datadog US5 intake hosts | ingestion credential | Operational logs/errors; batching; not an Anthropic-relative call | B | C22193:1384 |
| `/api/v2/rum` | ? | desktop anthropicHost | ? | Desktop exact-path RUM policy declaration; payload not recovered | B | C50258:19462 |
| `/v1/logs` | POST | configured OTLP/G/S | configured/GJ/J | OTLP logs | B | C50167:11589 |
| `/v1/metrics` | POST | configured OTLP/G/S | configured/GJ/J | OTLP metrics; may be gateway or session forwarding | B | C28481:1806 |
| `/v1/traces` | POST | configured OTLP/G/S | configured/GJ/J | OTLP traces | B | C50430:6917 |

### Design
| Path | Method | Host | Auth | Purpose / request shape (I unless quoted) | U | Evidence |
|---|---|---|---|---|---|---|
| `/v1/design/consent` | GET/POST/DELETE | A | design O / login O | Consent booleans; writes {consent:agent_design_projects} | Y | C44225:1264 |
| `/v1/design/grants` | GET/POST | A | design O / login O | Project write grants; POST {project_id}; GET grants array | Y | C44225:3724 |
| `/v1/design/mcp` | POST | A | design O | MCP JSON-RPC; Accept JSON/SSE, Mcp-Session-Id | Y | C54965:12391 |

### Bundled gateway server / device authorization
| Path | Method | Host | Auth | Purpose / request shape (I unless quoted) | U | Evidence |
|---|---|---|---|---|---|---|
| `/.well-known/oauth-authorization-server` | GET | G | N | Gateway OAuth issuer metadata | Y | C51412:15349 |
| `/device` | GET/POST | G | browser state | Device-code approval UI; user_code/origin/PKCE state | Y | C51412:17592 |
| `/device/deny` | POST | G | browser state | Deny pending device authorization | Y | C51412:17738 |
| `/healthz` | GET | G | N | Gateway liveness; response ok | Y | C51412:9792 |
| `/managed/settings` | GET | G | GJ | Gateway managed settings equivalent; conditional checksum | B | C15162:110866 |
| `/oauth/callback` | callback | G | OIDC state | Gateway IdP callback; code/error/state | Y | C51412:19912 |
| `/oauth/device_authorization` | POST | G | N | Gateway device authorization; rate-limited | Y | C51412:16567 |
| `/oauth/token` | POST | G | device grant | Gateway token exchange; form data | Y | C51412:21963 |
| `/protocol` | GET | G | N | Gateway protocol/config discovery | Y | C51412:15797 |
| `/readyz` | GET | G | N | Gateway readiness; backend/store probe | Y | C51412:9809 |
| `/user/bootstrap` | GET | G | GJ | Gateway desktop bootstrap and policy | Y | C51412:27070 |
| `/v1/organizations/spend_limits` | varies (handler) | G | gateway admin | Inbound spend-limit administration; not Anthropic account host | Y | C50400:425 |

### Other MCP, discovery and bundled SDK
| Path | Method | Host | Auth | Purpose / request shape (I unless quoted) | U | Evidence |
|---|---|---|---|---|---|---|
| `/sub/{clientKey}` | GET/SSE | GB api/stream host | ? | SDK feature subscription; activation not established | B | C15133:11756 |
| `/v1/agents` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:8731 |
| `/v1/agents/{id}` | GET/POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:8992 |
| `/v1/agents/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:9776 |
| `/v1/agents/{id}/versions` | ? | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:8363 |
| `/v1/claudeai-agents/mcp` | ? | A/S/? | ? | claudeai-agents / mcp operation (name); body/query ? | ? | C60931:1952 |
| `/v1/complete` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:28378 |
| `/v1/deployment_runs` | ? | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:392 |
| `/v1/deployment_runs/{id}` | GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:124 |
| `/v1/deployments` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:674 |
| `/v1/deployments/{id}` | GET/POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:935 |
| `/v1/deployments/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:1726 |
| `/v1/deployments/{id}/pause` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:1991 |
| `/v1/deployments/{id}/run` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:2252 |
| `/v1/deployments/{id}/unpause` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:2515 |
| `/v1/dreams` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:2796 |
| `/v1/dreams/{id}` | GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:3046 |
| `/v1/dreams/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:3552 |
| `/v1/dreams/{id}/cancel` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:3807 |
| `/v1/environment_providers` | ? | A/S/? | ? | v1 / environment_providers operation (name); body/query ? | ? | C18793:16755 |
| `/v1/environment_providers/cloud/create` | ? | A/S/? | ? | cloud / create operation (name); body/query ? | ? | C18793:18122 |
| `/v1/environments` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:36946 |
| `/v1/environments/{id}` | GET/POST/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:37208 |
| `/v1/environments/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:38263 |
| `/v1/environments/{id}/work` | ? | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:35072 |
| `/v1/environments/{id}/work/poll` | GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:35862 |
| `/v1/environments/{id}/work/stats` | GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:36134 |
| `/v1/environments/{id}/work/{id}` | GET/POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:34518 |
| `/v1/environments/{id}/work/{id}/ack` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:35291 |
| `/v1/environments/{id}/work/{id}/heartbeat` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:35563 |
| `/v1/environments/{id}/work/{id}/stop` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:36417 |
| `/v1/files` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:4733 |
| `/v1/files/{id}` | DELETE/GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:4988 |
| `/v1/files/{id}/content` | GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:5237 |
| `/v1/filestore/fs/listDirectory` | POST | A/S/? | ? | fs / listDirectory operation (name); body/query ? | ? | C65358:16346 |
| `/v1/filestore/fs/readFile` | POST | A/S/? | ? | fs / readFile operation (name); body/query ? | ? | C65358:13433 |
| `/v1/mcp/{id}` | MCP | mcp-proxy.anthropic.com | O/? | Hosted connector proxy; server-id template | Y | C13401:2019 |
| `/v1/mcp_servers` | ? | A/S/? | ? | v1 / mcp_servers operation (name); body/query ? | ? | C20378:2204 |
| `/v1/me` | GET | A/S/? | ? | v1 / me operation (name); body/query ? | ? | C34465:5986 |
| `/v1/memory_stores` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:41163 |
| `/v1/memory_stores/{id}` | GET/POST/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:41424 |
| `/v1/memory_stores/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:42475 |
| `/v1/memory_stores/{id}/memories` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:38578 |
| `/v1/memory_stores/{id}/memories/{id}` | GET/POST/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:38884 |
| `/v1/memory_stores/{id}/memory_versions` | ? | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:40445 |
| `/v1/memory_stores/{id}/memory_versions/{id}` | GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:40149 |
| `/v1/memory_stores/{id}/memory_versions/{id}/redact` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:40740 |
| `/v1/messages/batches` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:43500 |
| `/v1/messages/batches/{id}` | GET/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:43817 |
| `/v1/messages/batches/{id}/cancel` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:44623 |
| `/v1/messages/cache_touch` | ? | A/S/? | ? | messages / cache_touch operation (name); body/query ? | ? | C24340:417 |
| `/v1/messages/count_tokens` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:1279 |
| `/v1/models` | GET | D/G | SDK O/K/GJ | Model discovery; gateway opt-in and inbound handler also shipped | Y | C15190:7787 |
| `/v1/models/{id}` | GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:6092 |
| `/v1/organizations/api_keys` | ? | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:2179 |
| `/v1/organizations/api_keys/{id}` | GET/POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:1989 |
| `/v1/organizations/compliance_settings` | GET/POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:2293 |
| `/v1/organizations/external_keys` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:2505 |
| `/v1/organizations/external_keys/{id}` | GET/POST/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:2603 |
| `/v1/organizations/external_keys/{id}/validate` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:2997 |
| `/v1/organizations/federation_issuers` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:4023 |
| `/v1/organizations/federation_issuers/{id}` | GET/POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:4239 |
| `/v1/organizations/federation_issuers/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:4895 |
| `/v1/organizations/federation_rules` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:5942 |
| `/v1/organizations/federation_rules/{id}` | GET/POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:6156 |
| `/v1/organizations/federation_rules/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:6806 |
| `/v1/organizations/federation_rules/{id}/workspaces` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:5145 |
| `/v1/organizations/federation_rules/{id}/workspaces/{id}` | DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:5617 |
| `/v1/organizations/invites` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:3115 |
| `/v1/organizations/invites/{id}` | GET/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:3207 |
| `/v1/organizations/me` | GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:12357 |
| `/v1/organizations/rate_limits` | ? | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:3504 |
| `/v1/organizations/service_accounts` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:8049 |
| `/v1/organizations/service_accounts/{id}` | GET/POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:8263 |
| `/v1/organizations/service_accounts/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:8913 |
| `/v1/organizations/service_accounts/{id}/workspaces` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:7252 |
| `/v1/organizations/service_accounts/{id}/workspaces/{id}` | DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:7724 |
| `/v1/organizations/users` | ? | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:3807 |
| `/v1/organizations/users/{id}` | GET/POST/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:3623 |
| `/v1/organizations/workspaces` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:11321 |
| `/v1/organizations/workspaces/{id}` | GET/POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:11505 |
| `/v1/organizations/workspaces/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:11798 |
| `/v1/organizations/workspaces/{id}/members` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:9432 |
| `/v1/organizations/workspaces/{id}/members/{id}` | GET/POST/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:9182 |
| `/v1/organizations/workspaces/{id}/rate_limits` | ? | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:9804 |
| `/v1/organizations/workspaces/{id}/service_accounts` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:10454 |
| `/v1/organizations/workspaces/{id}/service_accounts/{id}` | GET/POST/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:9971 |
| `/v1/pages/mcp` | MCP | A | ? | Pages/Docs MCP | Y | C14456:867 |
| `/v1/sessions` | POST | A/S/? | ? | v1 / sessions operation (name); body/query ? | ? | C15228:92288 |
| `/v1/sessions/{id}` | GET/POST/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:17019 |
| `/v1/sessions/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:18059 |
| `/v1/sessions/{id}/events` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:12672 |
| `/v1/sessions/{id}/events/stream` | GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:13213 |
| `/v1/sessions/{id}/resources` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:14190 |
| `/v1/sessions/{id}/resources/{id}` | GET/POST/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:13623 |
| `/v1/sessions/{id}/threads` | ? | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:16016 |
| `/v1/sessions/{id}/threads/{id}` | GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:15739 |
| `/v1/sessions/{id}/threads/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:16296 |
| `/v1/sessions/{id}/threads/{id}/events` | ? | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:15045 |
| `/v1/sessions/{id}/threads/{id}/stream` | GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:15340 |
| `/v1/skills` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:19912 |
| `/v1/skills/{id}` | GET/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:20184 |
| `/v1/skills/{id}/versions` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:18393 |
| `/v1/skills/{id}/versions/{id}` | GET/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:18683 |
| `/v1/skills/{id}/versions/{id}/content` | GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:19497 |
| `/v1/toolbox/shttp/mcp/{id}` | MCP | localhost:8205 (local config) | ? | Local-development streamable HTTP MCP proxy | B | C13401:3137 |
| `/v1/tunnels` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:22201 |
| `/v1/tunnels/{id}` | GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:22455 |
| `/v1/tunnels/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:22969 |
| `/v1/tunnels/{id}/certificates` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:20989 |
| `/v1/tunnels/{id}/certificates/{id}` | GET | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:21266 |
| `/v1/tunnels/{id}/certificates/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:21824 |
| `/v1/tunnels/{id}/reveal_token` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:23233 |
| `/v1/tunnels/{id}/rotate_token` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:23500 |
| `/v1/ultrareview/preflight` | GET | A | O (teleport-org) | Cloud review eligibility; session identity header | Y | C43593:2481 |
| `/v1/ultrareview/quota` | GET | A | O (teleport-org) | Cloud review quota; timeout 3 s | Y | C35201:1002 |
| `/v1/user_profiles` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:6617 |
| `/v1/user_profiles/{id}` | GET/POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:6879 |
| `/v1/user_profiles/{id}/enrollment_url` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18463:7685 |
| `/v1/vaults` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:25898 |
| `/v1/vaults/{id}` | GET/POST/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:26154 |
| `/v1/vaults/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:27185 |
| `/v1/vaults/{id}/credentials` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:23820 |
| `/v1/vaults/{id}/credentials/{id}` | GET/POST/DELETE | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:24097 |
| `/v1/vaults/{id}/credentials/{id}/archive` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:25218 |
| `/v1/vaults/{id}/credentials/{id}/mcp_oauth_validate` | POST | D | SDK O/K/? | Bundled SDK resource; body/query schema not recovered | SDK | C18486:25510 |

## 3. Session ingress and remote control
**O:** `KU` strips the `sk-ant-si-` prefix, splits on dots, requires three parts and decodes the middle as base64url JSON; readers inspect `aud`, `session_id`, `exp` (C15166:807). One audience test is `"claude-gateway"`. The prefix is a token *type*, not a token value, and does not alone prove authorization. Envelopes have `session_ingress_token` (C48558:914). OAuth is not interchangeable with an ingress token; `sk-ant-sid` handling elsewhere uses `Cookie: sessionKey=...`, not necessarily bearer auth.
**O/I:** `Gf` converts ws/wss to http/https origins and compares against the startup ingress origin before accepting `/v2/session_ingress/shttp/mcp/`, `/v2/session_ingress/mcp/ws/`, `/v2/ccr-sessions/`, `/v1/code/` (C14508:8657). Path matching also checks owned session IDs and rejects encoded/foreign forms. MCP connector objects carry `"X-Session-UUID"` and `"anthropic-version":"2023-06-01"` (C14430:2092). The shttp and ws names indicate streamable HTTP versus WebSocket MCP transports (I); a complete shared wire schema/switch was not recovered. These are prefix hits, not proof of arbitrary suffix support.
**O:** worker registration/events/heartbeat/state/diagnostics are beneath `/v1/code/sessions/<id>`, not root worker APIs: `{worker_epoch:c,events:l.map((S)=>({payload:S}))}`, `{worker_epoch:c}` (C28439:16211–17872). Transcript upload is serialized `POST /worker/internal-events` relative to that session. Worker SSE appends `"/worker/events/stream"` (C47818:8408); request/event idempotency, delivery state and sequences appear in C47817. The protocol includes files, wakes, metadata, diagnostics and OTLP channels, not merely forwarding model messages.
**O:** cloud-session send checks first-party provider, posts `{events:[C]}`, with `C` from `ebn` (`{payload,device_attestation?}`); a bound-device refusal may trigger signed resend (C15228:98031). Watch uses `Accept:"text/event-stream","Last-Event-ID":i`, `include_trigger_sessions=true&exclude_tags=-` and resume-token machinery (C48361:9334). This is **SSE**, distinct from MCP WebSockets. Trusted-device/signature requirements mean copying identity headers cannot replace first-party device state.
**I:** Remote Control is a CLI/session feature, not evidence of a root HTTP `/remote-control` route: the literal is slash-command/help text (C8819:164). Environment bridge registration, polling/ack/stop/heartbeat/reconnect and session workers implement the underlying channel (C48361). Self-hosted runners have pool-secret/runner/session credentials, token refresh and per-session issuance (C28439); pool secrets are not ordinary account OAuth. UI refusal explicitly says Remote Control requires GrowthBook when `DISABLE_GROWTHBOOK` is set (string-table/help literal; exact search `Remote Control requires feature-flag evaluation`).

## 4. Telemetry and event logging
**O:** event exporter constructor: `this.maxBatchSize=e.maxBatchSize||200`, `this.batchDelayMs=e.batchDelayMs||100`, `this.maxBackoffDelayMs=e.maxBackoffDelayMs||30000`, `this.maxAttempts=e.maxAttempts??8`, timeout 10 s, base backoff 500 ms (C15229:23081). Endpoint defaults to api.anthropic.com; only the exact staging `ANTHROPIC_BASE_URL` literal selects staging, rather than any arbitrary inference proxy. Shape is `{events:[{event_type,event_data}]}` (C27589:8006 schema); retryable events are persisted in session telemetry batches. The higher-level first-party logger also has an OTel batch processor (10 s/200 defaults in C15236:14934 onward), so 100 ms is not every event's end-to-end flush delay.
**O:** event attributes include event name/ID, core metadata, user metadata, model/betas and event metadata; exposure events carry experiment/variation IDs, device/account/org/session identity and app-version context (C15236:14057–14934: `event_type:"GrowthbookExperimentEvent"`, `body:"growthbook_experiment"`). Do not assume all telemetry has identical privacy or payload semantics.
**O:** Datadog operational logs use `http-intake.logs.us5.datadoghq.com/api/v2/logs` (C22193:1384); browser-style error logs use `browser-intake-us5-datadoghq.com/api/v2/logs`, batches 25, default 30 s, timeout 10 s, `CLAUDE_CODE_DD_ERROR_TRACKING_FLUSH_INTERVAL_MS` (C22206:509). Embedded ingestion credentials are deliberately omitted. `/api/v2/rum` is a **desktop renderer egress-policy declaration**, with `toggleKey:"disableEssentialTelemetry"`, `exactPath:!0`, `undeliveredAs503:!0` (C50258:19462); it is not evidence that the standalone CLI starts browser RUM.
**O/I:** first-party metrics export to `rn().BASE_API_URL+"/api/claude_code/metrics"`, default 5 s (C52542:26989). Organization enablement check has one-hour/day constants (C43820). Opt-in OTel export is separate: `CLAUDE_CODE_ENABLE_TELEMETRY`, `OTEL_METRICS_EXPORTER`, `OTEL_LOGS_EXPORTER`, `OTEL_TRACES_EXPORTER`; `OTEL_EXPORTER_OTLP_ENDPOINT`, per-signal `OTEL_EXPORTER_OTLP_{METRICS,LOGS,TRACES}_ENDPOINT`, protocols, headers, compression, client cert/key/certificates; `OTEL_METRIC_EXPORT_INTERVAL`, `OTEL_LOGS_EXPORT_INTERVAL`, resource attributes and `otelHeadersHelper`. Gateway may route to `<gateway>/v1/<signal>`; session workers may route to `/worker/otlp/<signal>` (C52553:793, C18580:1101). `ANT_OTEL_*` and `BETA_TRACING_ENDPOINT` also exist in policy lists (C13777:1436), not a guarantee of user-configurable unrestricted export.
**O:** kill-switch priority is `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` > `DISABLE_TELEMETRY` > truthy-parsed `DO_NOT_TRACK`; first means essential-traffic-only, latter two no-telemetry (C13227:479/919). First-party telemetry/flags additionally suppress gateway/custom-account configurations: `function c_(){return AB()||a()||Wr()!==null||o()}` (C13811:3347; a/o are deployment gates). `DISABLE_ERROR_REPORTING` and `DISABLE_GROWTHBOOK` have distinct roles. Do not infer that `CLAUDE_CODE_ENABLE_TELEMETRY=0` alone disables first-party analytics; that variable controls opt-in OTel. Desktop settings separately use `disableEssentialTelemetry`, `disableNonessentialTelemetry`, `disableNonessentialServices` (C50258).
**O:** potentially sensitive OTel switches include `OTEL_LOG_USER_PROMPTS`, `OTEL_LOG_ASSISTANT_RESPONSES`, `OTEL_LOG_TOOL_CONTENT`, `OTEL_LOG_TOOL_DETAILS`, `OTEL_LOG_MANAGED_SETTINGS`, `OTEL_LOG_RAW_API_BODIES` (C13777:1436–3181); their presence is not evidence that content is sent by default. **Vocabulary:** 3,252 unique `tengu_[a-z0-9_]+` literals in cc_strings; this includes **flag names and diagnostics**, so it is not 3,252 proven emitted event names. Representative emitted/name families: `tengu_api_success`, `tengu_api_error`, `tengu_oauth_token_refresh_failure`, `tengu_headless_latency`, `tengu_client_event_signed`, tool/permission/remote-control/artifact/plugin activity; no full dump.

## 5. Feature flags and remote config (including GrowthBook)
**Key correction to a string-only interpretation:** GrowthBook SDK's default is `e.apiHost||"https://cdn.growthbook.io"` (C15133:36566), but this shipped CLI uses **remote evaluation on Anthropic**, not that CDN: `function jnr(e){return Di}var Di="https://api.anthropic.com/"`; `SG()` returns a build-inlined SDK client key (value omitted), not an environment credential (C15227:19115). SDK even rejects remoteEval on growthbook.io (C15133:37831). `CLAUDE_CODE_GB_BASE_URL` is in the env schema, but the observed resolver ignores its input.
**O:** `new Ms({apiHost:y,clientKey:h,attributes:s,remoteEval:!0,cacheKeyAttributes:["id","organizationUUID","slackTagConnected","atisPin","ccrDerivedSurface","sdkCachePartition"],apiHostRequestHeaders:{"User-Agent":Ds(),...S&&n.headers}})` (C15227:29140). It resolves auth only with workspace trust, excludes an unsuitable API key in `ZSi`, and initializes with a 5 s SDK timeout only if usable auth exists. Generic SDK `/api/features/<key>` GET is present, but this configured client uses `/api/eval/<key>` POST; flag `tengu_gb_eval_authed_enable` selects `/api/eval-authed/<key>`, falling back to stock eval on non-OK/exception (C15227:26235–29140). Remote payload contains `attributes,forcedVariations,forcedFeatures,url` (C15133:14976).
**Targeting context (O):** `id` and `deviceID` use device ID, plus `sessionId`, `platform`, `apiBaseUrlHost`, `organizationUUID`, `accountUUID`, `userType`, `subscriptionType`, `rateLimitTier`, `organizationRole`, `subscriptionCreatedAt` (parsed milliseconds), `firstTokenTime`, **email**, `appVersion`, `githubActionsMetadata`, `releaseChannel`, `entrypoint`, `sessionOrigin`, `ccrDerivedSurface`, `atisPin`, `hasUsedRemoteSession`, `hasRemoteEnvironment`, `slackTagConnected` (C15236:18842/23744). Optional fields only appear when populated; release-channel source is compiled undefined here. Env `CLAUDE_CODE_ACCOUNT_UUID` / `CLAUDE_CODE_ORGANIZATION_UUID` can supply context under identity checks. No separate `emailDomain` field was found in this builder: targeting by domain could be server-derived from email (I), not an observed client field.
**Cadence/cache (O):** SDK defaults `staleTTL:60000,maxAge:14400000,cacheKey:"gbFeaturesCache",backgroundSync:!0,maxEntries:10`, optional localStorage and in-memory request coalescing, subscription when `x-sse-support` allows it (C15133:11219). CLI separately saves `cachedGrowthBookFeatures`, `cachedExperimentFeatures`, `cachedExperimentData`, `cachedGrowthBookFeaturesAt` to global config; no disk sync for slack-tag context, non-default host, or auth loss (C15227:25515). Initialization/auth-change refresh and explicit refresh coexist with periodic refresh: default **360 minutes**, flag `tengu_gb_refresh_interval_minutes` clamped **5–360**, ±10% jitter, capped at 360 (C15236:26049). Do not mistake SDK 60 s stale TTL for CLI polling every minute. Environment refresh-interval name is observed, but this active cadence function reads the flag instead. Startup `growthbook_init_ms` / `growthbook_killswitch_recheck_ms` measure waits and kill-switch rechecks, not distinct server endpoints.
**Local overrides — production distinction (O):** dependency wiring mentions `readEnvironmentOverrides:()=>i.CLAUDE_INTERNAL_FC_OVERRIDES`, global `growthBookOverrides`, and setters. But implementation is `getEnvironmentOverrides(){return null}`, `readConfigOverrides(){return}`, `setConfigOverride(e,n){return}clearConfigOverrides(){return}` (C15227:21173–22562). Therefore claiming that an ordinary user can force flags via those env/settings in **this build** is unsupported; the public-shipping methods are no-ops. SDK `window._growthbook` requires browser/dev-mode guards (C15133:38226); it is not a CLI debugging console. The supplied `/growthbook` hint was not recovered as a concrete HTTP endpoint; the slash-looking hit is a regex `/growthbook\\.io$/i`.
**Eval harness is separate (O):** YAML/frontmatter `execution.growthbook_overrides` accepts bool/string/number/null, becomes `seededFlags`; only applied when artifact publishing is operator-granted, and each key must be in `allowFlagOverrides` or comma-separated `CLAUDE_CODE_EVAL_ALLOW_FLAG_OVERRIDES`; others are dropped (C61787:1970, C61936:12550, C61937:842, search `function wg(e,n)`). `CLAUDE_CODE_EVAL_ALLOW_ARTIFACT_PUBLISH` is the separate opt-in. This is not the same as an unrestricted user-settings override.
**Separate key sets (O):** `kr` excludes flags from saved cache/experiment fallback, not an override authorization list: `tengu_steady_bittern,tengu_twinkly_clover,tengu_candid_juniper,tengu_iterative_lightning,tengu_steadfast_wren,tengu_cheeky_toucan,tengu_cobalt_plinth_orpine,tengu_valiant_lantern,tengu_level_plover,tengu_splendid_lynx,tengu_tender_mitten,tengu_snug_quill,tengu_mellow_heron,tengu_wondrous_haven,tengu_mighty_flask,tengu_goofy_hare,tengu_zippy_harbor,tengu_fallow_lantern,tengu_mellow_hollerith,tengu_witty_kay,tengu_kind_dewdrop,tengu_thistle_orrery,tengu_proud_pearl,tengu_snug_wall,tengu_mossy_squid` (C15168:59245; C15227:25515/31689). `ES` separately contains steady_bittern, thistle_orrery, snug_wall. Do not conflate these with the eval operator allowlist.
**Observable gates (O/I):** `tengu_penguins_off` and organization policy disable Fast mode (C15168:39038); `tengu_skills_dashboard_enabled` suppresses skill health fetch (C63339:7858); `tengu_cobalt_plinth` plus admin policy gates artifact publishing (`return"growthbook_off"`, C22443:15008); remote-control availability requires flags; auto-mode startup can recheck a disabled-on-disk kill switch. Flags change tools, eligibility, rollout variants, retries and UI; they are not authorization grants.
**Other remote config (O):** bootstrap fetch is skipped under essential-only or non-firstParty, except gateway model discovery via `/v1/models` when `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY` (C34387:5606). Its parsed response includes client_data, additional model options/costs/access and OAuth account enrichment (C34387:1363 onward). Managed settings use conditional checksums, cache/signature/account checks; `If-None-Match`, 200/204/304/404 handling, with 204/404 from an intermediary treated differently (C31539:12842). Gateway uses `<gateway>/managed/settings`; forceRemoteSettingsRefresh can block startup on fresh fetch failure. Exact managed-settings polling interval is unresolved.
**Other cached eligibility (O):** Penguin Fast-mode prefetch window is **30 s** (`XN=30000`, `beginPrefetchWindow`, C15168:47834), not proof of an unconditional 30 s poll timer. Grove returns `grove_enabled,domain_excluded,notice_is_grace_period,notice_reminder_frequency`, consults account settings and notice-viewed date and caches config/account fetches (C38811:3108); no fixed refresh TTL was established.

## 6. Frame / artifacts
**I from calls and user-visible errors:** frame is the hosted artifact/app subsystem: publish web content/files, reserve stable slugs, retain versions, manage sharing/favorites/comments, instantiate types, operate blob/database services, and synchronize live updates. It is not a quota API. Direct account OAuth and remote-session agent-proxy relay coexist; `mti` rewrites a `/api/frame/` path to `<agent-proxy>/frame...` (C15168:28226). Presence of a desktop pane/terminal-link choice does not establish unrestricted third-party reuse.
**Lifecycle (O):** reserve `du.post("/api/frame/deploy/prepare",$Ht(),a)`; accept only HTTP 200 with valid slug (C31993:15677). Upload/deploy direct accepts per-attempt/resend/create-confirmation query markers and a metadata/file body; upload family explicitly constructs `{slug,...cross_org_ok,...files:Q.map(Vt)}` (C31996:17755). Retry paths handle 429/Retry-After; custom-thumbnail refusal may fall back to automatic screenshot (C31993:15732). Read-back, versions, diagnostics and contract endpoint families are in the table.
**Capabilities/live state (O):** contract/latest/pinned versions return parsed capability data; definitions limited 256 KiB, prompt 64 KiB; `x-contract-missing-caps` is a response marker (C31975:11775–13590). Watch-token POST returns a parsed live token path; live transport is checked as `transport:"live"` (search C32037 `renew_miss`); sync URL carries slug, comment_hint, vn (C32371:10846). Content reads have `x-frame-asset-token` (C32037:12908). Token values are deliberately omitted. `/api/artifacts/mcp`, `/v1/code/mcp/ccr-artifacts` and `/v1/pages/mcp` are separate MCP identities (C14456:867–985), not aliases for every frame REST operation.

## 7. Design endpoints
**O:** separate public `DESIGN_CLIENT_ID`, design scopes `["user:design:read","user:design:write"]` (C13401:0–2200). Credential selection prefers a refreshable design-scoped login, then separately stored design credential, with limited normal-login fallback (C44225:1264); no token values are reproduced. Consent vocabulary is recovered exactly: `var u=["agent_design_projects"]` (C44225:516), with UI text allowing read/edit of Design projects and revoke via Design settings or `/design revoke`.
**O:** GET consent -> boolean map; POST/DELETE `{consent:n}`; GET grants -> grants array; POST grant `{project_id:n}`; 404-tolerant probes permit older servers (C44225:3008–5333). `/v1/design/mcp` uses JSON-RPC, JSON/SSE Accept, `Mcp-Session-Id`, `X-Anthropic-Client:"claude-cli-design-tool"`, and first-party origin allowlisting (C54965:12391). `CLAUDE_CODE_ENABLE_DESIGN_SYNC` is present in the env/settings policy (C13777). **I:** unlocks project-scoped Design read/edit tools with explicit consent/write-grant gates, not broad account access.

## 8. Marketplace / plugins / skills
**O:** server directory is unauthenticated paginated GET, `limit:"500",visibility:r`, cursor, remote-type filtering (C15228:123379). Plugin-directory literal is used in an essential-traffic exception: `GET`, `auth:"none"`, account host and **no q parameter** (C15228:114190). The adjacent `s.sign(n,h)` function signs **session event payloads**; adjacency does **not** prove signed plugin directory requests.
**O:** organization plugin sync uses list-plugins with `enabled_only=true&compact=true`, optional component details/hooks/wrapped-skills flags, plus per-plugin download (C20586:126/2480). Skills list adds `include_wiggle_skills=true` and download (C34832:489/1520). Search, marketplace catalog, ratings, connector discovery and assets routes are enumerated above. `/api/claude_code/skills` is separately flag-gated skill health (C63339:7858), not the package-download route.
**O:** `/api/npm/internal/` appears as an **example allowed registry/tarball origin prefix** in managed policy prose, not evidence of a concrete Anthropic npm service: `https://npm.example.com/api/npm/internal/` (C7142:251). Accordingly it is excluded from the callable endpoint table. Likewise `/api/cask/<name>.json` belongs to formulae.brew.sh, not Anthropic (C29372:2926). `/api/graphql` is a GitHub Enterprise URL builder with caller-supplied host (C13169:1323), not established as an Anthropic endpoint.

## 9. Beta headers and feature headers
**Observed literal values, not an always-sent set.** Registry is C14743 (find `k("claude_code","claude-code-20250219")`); extra SDK beta values C18463/C18486; OAuth beta C13401. The date-shaped literal sweep also finds model IDs and `foo-2025-01-01` examples, which are **not** beta-header evidence. Actual beta assembly depends on provider, flags, request, privacy policy and user custom headers; `WKt` filters afk-mode and `Lj` is HIPAA suppression; `CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS` disables experimental additions (C14743 registry/sanitizer).
Literal API beta vocabulary recovered (50 distinct values; registry + SDK/call-site extras):
`advanced-tool-use-2025-11-20`, `advisor-tool-2026-03-01`, `afk-mode-2026-01-31`, `agent-memory-2026-07-22`, `auto-mode-classifier-2026-07-16`, `cache-diagnosis-2026-04-07`, `cache-keepalive-2026-09-03`, `ccr-byoc-2025-07-29`, `ccr-triggers-2026-01-30`, `claude-code-20250219`, `context-1m-2025-08-07`, `context-hint-2026-04-09`, `context-management-2025-06-27`, `dangerous-tool-use-2026-09-03`, `dreaming-2026-04-21`, `effort-2025-11-24`, `environments-2025-11-01`, `extended-cache-ttl-2025-04-11`, `fallback-credit-2026-06-01`, `fast-mode-2026-02-01`, `files-api-2025-04-14`, `inline-tools-2026-09-15`, `interleaved-thinking-2025-05-14`, `managed-agents-2026-04-01`, `mcp-servers-2025-12-04`, `mcp-tunnels-2026-06-22`, `message-batches-2024-09-24`, `message-threads-2026-08-12`, `mid-conversation-system-2026-04-07`, `mid-conversation-system-clear-at-2026-08-21`, `mid-conversation-tool-changes-2026-07-01`, `oauth-2025-04-20`, `oidc-federation-2026-04-01`, `per-turn-control-2026-07-01`, `prompt-caching-evict-2026-05-12`, `prompt-caching-scope-2026-01-05`, `redact-thinking-2026-02-12`, `server-side-fallback-2026-06-01`, `server-side-fallback-2026-07-01`, `structured-outputs-2025-12-15`, `task-budgets-2026-03-13`, `thinking-binding-controls-2026-08-01`, `thinking-display-updates-2026-08-18`, `thinking-resumption-2026-07-17`, `thinking-token-count-2026-05-13`, `timing-2026-09-09`, `token-counting-2024-11-01`, `tool-search-tool-2025-10-19`, `user-profiles-2026-08-18`, `web-search-2025-03-05`.
Two internal pseudo-beta registry strings: `x-cc-internal-mid-conv-cache-promotion`, `x-cc-internal-mid-conv-cache-promotion-ok` (not in the final LR registry array). `Ja("mid_conversation_system")`, `Ja("thinking_display")` explicitly have null headers. `bedrock-2023-05-31` and `vertex-2023-10-16` are transport-version vocabulary, not established anthropic-beta values.
Observed first-party request/feature-header names (literal inventory; values/usage are endpoint-specific): `anthropic-beta`, `anthropic-version`, `anthropic-client-platform`, `anthropic-client-version`, `anthropic-client-os`, `anthropic-client-feature`, `anthropic-client-request-reason`, `anthropic-user-profile-id`, `anthropic-workspace-id`, `anthropic-organization-id`, `anthropic-cache-keepalive`, `anthropic-mcp-client-capabilities`, `anthropic-mcp-discover-protocol-version`, `anthropic-dangerous-direct-browser-access`; `x-api-key`, `x-organization-uuid`, `X-Trusted-Device-Token`, `X-Session-UUID`, `X-Anthropic-Client`, `Mcp-Session-Id`, `MCP-Protocol-Version`, `x-cc-atis`, `x-anthropic-additional-protection`, `x-anthropic-attachment-prefetch`, `x-anthropic-attachment-prefetch-seen`, `x-claude-code-compaction`, `x-claude-code-context-compacted`, `x-claude-code-keep-branch`, `x-claude-code-prev-tool-durations`, `x-claude-code-route-final-answer`, `x-claude-code-tunnel-credential-epoch`, `x-claude-code-upload-lane`, `x-claude-code-wiggly-dove`, `x-config-keyterms`, `x-litellm-end-user-id`, `x-ssr-client-ip`, `x-ssr-client-ip-sig`; identity set in §10.
Frame request headers additionally include `x-frame-asset-token`, `X-Ccr-Relay-Tool-Use-Id`, `X-Ccr-Relay-Approved-Intent`. Do not classify every string beginning anthropic-/x- as a request header: `anthropic-ratelimit-unified-*`, `x-frame-content-route`, `x-frame-refusal`, `x-contract-missing-caps`, `x-claude-code-signature` have response/signature roles. Rate-limit response inventory belongs in the quota-parser investigation. Generic vendored x-stainless / AWS / Azure MSAL headers are not Claude identity gates. Server enforcement cannot be proven from client code.

## 10. Spoof / identity headers
**Exact resolved UA grammar (O):** `claude-cli/2.1.310 (external, <entrypoint>[, agent-sdk/<version>][, client-app/<app>][, workload/<id>])`; default entrypoint cli; sources include `CLAUDE_CODE_ENTRYPOINT`, `CLAUDE_AGENT_SDK_VERSION`, `CLAUDE_AGENT_SDK_CLIENT_APP` (C15223:1000). Account helpers also use the **different** `claude-code/2.1.310` UA (`Ds`, C12727:0; bootstrap has its own `claude-code/<ccVersion>`, C34387:4439). Desktop third-party marker `claude-desktop-3p` appears in the host helper (C13566:2905); don't assume every request has it.
**O:** inference default builder: `{"x-app":Ut()?"cli-bg":"cli","User-Agent":M$(),[P2t]:q()}` (C18502:24519). Reserved `cL` set (C15168:62060): `x-app`, `X-Claude-Code-Session-Id`, `x-claude-code-agent-id`, `x-claude-code-parent-agent-id`, `x-claude-code-prompt-id`, `x-claude-code-request-class`, `x-claude-code-agent-type`, `x-anthropic-additional-protection`, `x-claude-remote-session-origin`. Other identity/context names: `x-client-app`, `x-claude-remote-container-id`, `x-claude-remote-session-id`, gateway `x-claude-gateway-user-email` / `x-claude-gateway-user-id`. `anthropic-client-platform` maps cli/VSCode/remote surfaces (C12727, search `function py`). These are visible client assertions, not cryptographic proof of a first-party caller.
**Boundary (O):** bundle's api_error_origin documentation defines `first_party` only when firstParty provider, https://api.anthropic.com default port, direct/no redirect, verified TLS, no added CA/verification override/unix socket. Custom inference/WIF base, gateway, remote replay and client-generated errors do not get that stamp; absence means unknown (C7438:21). This is a **client-side provenance stamp**, not an observed server algorithm. Device attestations, trusted-device tokens, scoped OAuth/ingress claims, managed signatures and policy decisions are independent identity/authentication layers. UA/header spoofing cannot demonstrate entitlement, and this investigation authorizes no spoofing or calls.

## Confidence and gaps
High confidence: literal paths and routing branches, separate account/inference bases, token structures, GrowthBook production no-op override methods, worker route nesting, exporter constants and UA grammars. Medium: intended product purpose and route/auth associations reconstructed from wrappers. Unknown: server availability, scopes accepted in every deployment, entitlement/policy/terms permitting third-party reuse, complete schemas and all dynamically assembled suffixes. **Nothing here establishes legitimate reuse rights.** For pi-quotas, read-only OAuth usage/profile and response headers are the most relevant candidates; billing mutations, device registration, internal flags and session/artifact APIs are not substitutes for quota reporting.
Inventory boundaries: includes bundled SDK definitions (not just CLI calls), excludes vendor Google/AWS/OIDC authentication URLs, documentation/example paths, broad prefixes and non-HTTP slash commands. GitHub Enterprise `/api/graphql` / `/api/v3`, npm registry example and Homebrew are distinguished from Anthropic services. Some rows have unknown methods/auth/shapes rather than unsupported guesses. Full gateway credential bootstrap, every ingress packet type and managed-setting refresh interval remain unresolved. Bundled gateway server handlers also serve `/api/oauth/usage` (C51412:26931), `/managed/settings`, `/user/bootstrap`, OAuth device flow and health endpoints: these are inbound enterprise-gateway implementations, not additional public Anthropic services. No tests/build were run: documentation-only, offline static investigation; unrelated working-tree changes were preserved.
Verification: three load-bearing bounded corpus searches were re-run after drafting: (1) `ctx.py '/api/frame/deploy/prepare' --window 400 --max 2 --files code` → `du.post("/api/frame/deploy/prepare",$Ht(),a)`; (2) `ctx.py '/v1/design/grants' --window 400 --max 2 --files code` → `jt.post("/v1/design/grants",{project_id:n}`; (3) `ctx.py 'getEnvironmentOverrides(){return null}' --window 200 --max 1 --files code` → `getEnvironmentOverrides(){return null}`. Exact quoting was additionally checked against corpus substrings; no runtime behavior was claimed.
