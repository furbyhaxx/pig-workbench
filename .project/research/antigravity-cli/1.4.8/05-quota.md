---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.8-05-quota"
  title: "agy 1.4.8 quota, entitlement, credits and reset mechanics"
  description: "Authoritative quota sources, protobuf field shapes, quota units, reset semantics, tier/billing decision tree, live vs dead seat-management surface, and the refresh/throttle policy"
  status: "published"
  revision: 3
  created_at: "2026-10-27T09:57:00+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "pi"
  updated_by: "orchestrator"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/antigravity-cli/1.4.8/05-quota.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 1
      at: "2026-10-27T09:57:00+01:00"
      note: "Initial extraction."
    - revision: 2
      at: "2026-10-27T10:12:00+01:00"
      note: "Folded in observed live payloads: the five real buckets across three groups and two windows, proof that reset times are rolling rather than boundary-aligned, and the correction that availableCredits carries a threshold rather than a balance (2.3, 4.1)."
    - revision: 3
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/antigravity-cli/1.4.8/05-quota.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---

# agy 1.4.8 — quota, entitlement, credits and reset mechanics

status: partially explained
confidence: high (field shapes, units, reset source, throttle constants, tier decision tree); medium (business/BAIC licensing path, per-model `quota_info` consumption); low (background worker period, `AGY_BUSINESS_PAYGO_TIER` exact read site)

Method: `tools/goresym.py` (`funcs`/`addr`/`strs`/`grep`), `tools/xwalk.py` (string→function
cross-reference), and a new `tools/pbdump.py` I added for this section: an embedded
`FileDescriptorProto` walker (`pbdump.py --find quota_summary.proto` → `0x5bf1ee7`, then
`pbdump.py 0x5bf1ee7`). Descriptor dumps are the primary evidence for field names and numbers;
klog output is the primary evidence for runtime behaviour. One throwaway call-target scanner
(rel32 `e8` / RIP-relative `lea` over `.text`) was used to find callers of the quota accessors;
it is not kept because `xwalk.py` covers string→function and callers mattered only for
`GetAvailableCredits`. Nothing under `~/.gemini/antigravity-cli/` or `~/.config/antigravity/`
was modified, no quota endpoint was polled, and no CLI command was run this session — every
observation below comes from bytes or from the logs already on disk.

Two caveats that apply to every citation:

* **pclntab entry offsets are not hot entries.** The recovered function addresses are
  systematically low by `0x80` for at least some functions (`time.Now` resolves to
  `0x66a5960`, every real call site targets `0x66a59e0`), an artefact of BOLT body splitting.
  Where I cite an address it is an **objdump** address from a printed disassembly, not a
  pclntab entry. Attribution of a literal to "the function after X" therefore means "in the
  fragment that X's body was split into".
* **Descriptor dumps overrun.** My walker parses greedily, so a dump that starts at
  `onboarding.proto` also prints the following descriptors; the `file:` header line can name a
  *later* descriptor. Field tables below were cross-checked against the generated `Get*`
  accessor names in `data/funcs.txt`, which are per-message and unambiguous.

---

## 1. The authoritative sources

Four distinct payloads carry quota/entitlement state. Only two matter for a quota UI, and they
are **not** the same call.

| # | endpoint | Go client path | cached as | carries |
|---|---|---|---|---|
| 1 | `POST https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist` | `codeassistclient.(*CodeAssistClient)` | `Cache[*v1internal_go_proto.LoadCodeAssistResponse]` named `loadCodeAssistResponse` | tier, plan name/description, upgrade copy, **`paid_tier`**, **`paid_tier.available_credits`**, `g1_tier`, ineligible reasons, `cloudaicompanion_project` |
| 2 | `v1internal:retrieveUserQuotaSummary` | `codeassistclient.(*CodeAssistClient).fetchRetrieveUserQuotaSummary` | `Cache[*v1internal_prediction_service_go_proto.RetrieveUserQuotaSummaryResponse]` named `retrieveUserQuotaSummary` | **the quota buckets: id, name, description, window, remaining fraction/amount, `disabled`, `reset_time`** |
| 3 | `v1internal:fetchAvailableModels` | `model_configs` path | 5 min cache, keyed by `Entitlement` | per-model `ModelDetails.quota_info {remaining_fraction, reset_time}` and the model catalogue |
| 4 | `google.gca.aicode.v1main.PredictionService/:fetchQuotaStatus` | operation-map entry `fetchQuotaStatus` | — | `BucketInfo{remaining_fraction, reset_time, quota_type, model}`; **registered but never invoked** (§6) |

Cache types, quoted from the instantiated generics in `data/funcs.txt`:

```
Cache[*google3/google/internal/cloud/code/v1internal/v1internal_go_proto.LoadCodeAssistResponse]
Cache[*google3/google/internal/cloud/code/v1internal/v1internal_prediction_service_go_proto.RetrieveUserQuotaSummaryResponse]
Cache[*google3/google/internal/cloud/code/v1internal/v1internal_jetski_service_go_proto.FetchUserInfoResponse]
Cache[*google3/third_party/jetski/language_server/code_assist_client/codeassistclient.peopleAPIUserInfo]
```

**`userInfo` is not a quota source.** The cache named `userInfo` holds
`*codeassistclient.peopleAPIUserInfo` and is populated by `(*CodeAssistClient).fetchUserInfo`
(People API / `https://www.googleapis.com/oauth2/v2/userinfo`, override via
`USER_INFO_OVERRIDE_URL`). The other `userInfo`-adjacent cache,
`*v1internal_jetski_service_go_proto.FetchUserInfoResponse`, is only
`{user_settings, user_tags, region_code}`. The log line
`Cache(userInfo): Singleflight refresh failed: failed to get load code assist response` is a
*dependency* error, not a tier lookup: the people-API fetch is gated behind a successful
`loadCodeAssist`.

`retrieveUserQuotaSummary` also declares the dependency explicitly — its singleflight error is
`failed to get load code assist response: error getting token source: …` (observed at
`cache.go:940` in `cli-20261027_082137.log`). Derived: the quota summary request needs the
project/entitlement that `loadCodeAssist` produced, so a reimplementation must call
`loadCodeAssist` first (and can then reuse its `current_tier.id` as the entitlement).

---

## 2. `loadCodeAssist` response shape

Descriptor: `google/internal/cloud/code/v1internal/onboarding.proto`, file offset `0x5de3c49`,
package `google.internal.cloud.code.v1internal`. Dumped verbatim (field numbers are the wire
numbers):

```proto
message LoadCodeAssistRequest {
  enum Mode { MODE_UNSPECIFIED = 0; FULL_ELIGIBILITY_CHECK = 1; HEALTH_CHECK = 2; }
  string cloudaicompanion_project = 1;   // optional
  ClientMetadata metadata = 2;
  Mode mode = 3;                         // optional
}

message LoadCodeAssistResponse {
  UserTier current_tier = 1;
  repeated UserTier allowed_tiers = 2;
  string cloudaicompanion_project = 3;   // optional
  google.rpc.Status project_validation_error = 4;  // optional
  repeated IneligibleTier ineligible_tiers = 5;
  bool gcp_managed = 6;                  // optional
  string manage_subscription_uri = 7;    // optional
  ReleaseChannel release_channel = 8;    // optional
  string upgrade_subscription_uri = 9;   // optional
  string g1_tier = 11;                   // optional
  UserTier paid_tier = 12;
}

message UserTier {
  enum UpgradeType {
    UPGRADE_TYPE_UNSPECIFIED = 0; GDP = 1; GOOGLE_ONE = 2;
    GDP_HELIUM = 3; GOOGLE_ONE_HELIUM = 4;
  }
  message PrivacyNotice { bool show_notice = 1; string notice_text = 2; bool minor_notice = 4; }
  string id = 1;
  string name = 2;
  string description = 3;
  bool user_defined_cloudaicompanion_project = 4;
  PrivacyNotice privacy_notice = 5;
  bool is_default = 6;
  string upgrade_subscription_uri = 7;      // optional
  string upgrade_subscription_text = 8;     // optional
  string upgrade_button_text = 11;          // optional
  string client_experience_tag = 12;        // optional
  UpgradeType upgrade_subscription_type = 9; // optional
  bool uses_gcp_tos = 13;
  repeated Credits available_credits = 14;
}

message Credits {                          // google/internal/cloud/code/v1internal/credits.proto
  enum CreditType { CREDIT_TYPE_UNSPECIFIED = 0; GOOGLE_ONE_AI = 1; }
  CreditType credit_type = 1;
  int64 credit_amount = 2;
  int64 minimum_credit_amount_for_usage = 3;
}

message IneligibleTier {
  enum IneligibleTierReasonCodes {
    UNKNOWN = 0; INELIGIBLE_ACCOUNT = 1; UNKNOWN_LOCATION = 2; UNSUPPORTED_LOCATION = 3;
    RESTRICTED_NETWORK = 4; RESTRICTED_AGE = 5; NON_USER_ACCOUNT = 6; DASHER_USER = 7;
    BYOID_USER = 8; RESTRICTED_DASHER_USER = 9; VALIDATION_REQUIRED = 10;
    GOOGLE_TOS_NOT_SUPPORTED_BY_CLIENT = 11; UNSUPPORTED_CLIENT = 12;
  }
  IneligibleTierReasonCodes reason_code = 1;
  string reason_message = 2;
  string tier_id = 3;  string tier_name = 4;
  string validation_error_message = 5; string validation_url_link_text = 6;
  string validation_url = 7; string validation_learn_more_link_text = 8;
  string validation_learn_more_url = 9;
}
```

`entitlement.proto` is a one-field message and is exactly what is forwarded to
`fetchAvailableModels` (`FetchAvailableModelsRequest.entitlement = 9`):

```proto
message Entitlement { string user_tier = 1; }
```

### 2.1 The fields that matter

| field | meaning | how the CLI uses it |
|---|---|---|
| `current_tier.id` | tier id (`free-tier`, `standard`, `legacy`, enterprise ids) | plan label, `fetchAvailableModels` entitlement, `SetUserTier` |
| `current_tier.name` / `.description` | human plan name/blurb | `UsageModel.renderPlanSection` (`"  Your Plan: %s\n"`) |
| `current_tier.is_default`, `.upgrade_subscription_*`, `.upgrade_button_text`, `.client_experience_tag` | upsell copy | upgrade block: `"  Upgrade at: "`, `"  Upgrade to get 1,500 model requests per day with Gemini CLI and\n  Gemini Code Assist's agent mode with Google AI Pro."` |
| `current_tier.available_credits[]` | legacy credit grant (only type `GOOGLE_ONE_AI`) | not the credit balance the panel shows |
| **`paid_tier`** | the paid tier record | **`GetG1Credits` reads `paid_tier.available_credits`; `aicodeEntitlement` reads the paid tier id** |
| **`paid_tier.available_credits[].credit_amount`** | G1/AI credit balance — **but see 2.3: the live payload does not always carry it** | `"  Available AI Credits: %s\n"` |
| `paid_tier.upgrade_subscription_type` | `GDP` / `GOOGLE_ONE` / `*_HELIUM` | upsell framing |
| `g1_tier` | Google One tier marker | paired with the `useG1Credits` toggle |
| `ineligible_tiers[]` | why a tier is unavailable | eligibility gate (`printmode.waitForEligibility`, `store.(*Manager).waitForEligibility`, 30 s) |
| `gcp_managed`, `manage_subscription_uri` | GCP-managed billing | subscription management link |
| `cloudaicompanion_project` | resolved project | request `project` for the quota summary and generation |

There is **no per-model limit and no reset time in `loadCodeAssist`**. Anyone reimplementing
this must not try to derive limits from it.

### 2.2 Credit balance is not a separate endpoint

`backend.(*ServerBackend).GetG1Credits` (0x9252b00+768) logs
`GetG1Credits: starting fetch`, then `GetG1Credits: paidTier is nil`, `no credits info found`,
`no code assist client configured`, `failed to get load code assist response: %w`. Its only
direct callee among the quota accessors is
`v1internal_go_proto.(*UserTier).GetAvailableCredits` (verified by call-target scan; every other
quota accessor is inlined). So:

```
credits = loadCodeAssist().paid_tier.available_credits[].credit_amount   // CreditType GOOGLE_ONE_AI
```

One call gives tier + plan + credit balance.

### 2.3 Observed live (2026-10-27): the real `loadCodeAssist` payload

Captured from `agy -v=5 models`; full body in
[`evidence/live-2026-10-27/loadCodeAssist.json`](evidence/live-2026-10-27/loadCodeAssist.json)
(email and project id redacted).

```json
{
  "currentTier": { "id": "free-tier", "name": "Antigravity",
                   "privacyNotice": { "showNotice": true, ... },
                   "upgradeSubscriptionType": "GOOGLE_ONE" },
  "paidTier":    { "id": "g1-pro-tier", "name": "Google AI Pro",
                   "availableCredits": [ { "creditType": "GOOGLE_ONE_AI",
                                           "minimumCreditAmountForUsage": "50" } ] },
  "allowedTiers": 2, "ineligibleTiers": 0,
  "cloudaicompanionProject": "<project-id>",
  "keys": [ allowedTiers, cloudaicompanionProject, currentTier,
            gcpManaged, paidTier, upgradeSubscriptionUri ]
```

Three corrections to the static reading:

1. **`availableCredits` is a *threshold*, not a balance here.** The only member present is
   `minimumCreditAmountForUsage: "50"` — meaning "≥50 credits required to use them". There is
   **no `creditAmount`**, so `GetG1Credits` finds nothing to print and the panel falls back to
   its `no credits info found` path. A reimplementation must render the balance as **unknown**,
   never as `0`, and must not treat the threshold as an amount.
2. **`currentTier` and `paidTier` are both populated and disagree by design.** Observed:
   `currentTier.id = free-tier` while `paidTier.id = g1-pro-tier`. The plan label must come
   from `paidTier` when present; `currentTier` alone under-reports the account.
3. **`keys` is not a field** — it is an artefact of the capture. The actual response keys are
   the six listed above. `gcp_managed` is present (boolean), `manage_subscription_uri` was not.

`fetchUserInfo` returns `{ "userSettings": {}, "regionCode": "CH" }` — this is where a
region comes from, and `userSettings` is where the data-collection opt-in lives (see 04-api
§9.4), empty for this account.

---

## 3. Quota model: what is actually limited

### 3.1 Unit

`retrieveUserQuotaSummary` returns `QuotaSummaryBucket`s with a `window` and a remaining
*amount or fraction* — the unit is not encoded in the message. The unit is stated in three
other places, and they agree:

* `google/gca/aicode/v1main/prediction_service.proto` →
  `FetchQuotaStatusResponse.BucketInfo.quota_type` with `enum QuotaType { QUOTA_TYPE_UNSPECIFIED = 0; REQUESTS = 1; }`.
  Only `REQUESTS` exists.
* UI copy `" quota per turn"` (rodata `0x59a5ba9`, referenced by
  `cli/model/model.renderQuotaNote`, +0x211), i.e. the limit is consumed **per turn/prompt**.
* Upgrade copy `"  Upgrade to get 1,500 model requests per day with Gemini CLI and"` — the
  consumer free tier is described to the user in **model requests per day**.

So the unit is **requests ("model requests", "turns", "prompts"), never tokens**. Token counts
(`UsageMetadata`, `Usage.ToTokenUsage`, `ChatModelMetadata.usage`) are reported separately for
cost/telemetry and do not move the quota.

### 3.2 Granularity: per model *bucket*, not per model

`QuotaSummaryBucket` (`google/internal/cloud/code/v1internal/quota_summary.proto`, descriptor at
file offset `0x5bf1ee7`):

```proto
message RetrieveUserQuotaSummaryRequest { string project = 1; }

message QuotaSummaryBucket {
  string bucket_id = 1;
  string display_name = 2;
  string window = 3;            // free-form, server-chosen (e.g. a window label, not an enum)
  oneof remaining {
    float remaining_fraction = 4;
    int64 remaining_amount = 5;
  }
  google.protobuf.Timestamp reset_time = 6;
  string description = 7;
  bool disabled = 8;
}

message QuotaSummaryGroup {
  string display_name = 2;
  string description = 3;
  repeated QuotaSummaryBucket buckets = 1;
}

message RetrieveUserQuotaSummaryResponse {
  repeated QuotaSummaryBucket buckets = 1;   // flat list
  repeated QuotaSummaryGroup groups = 2;     // the same buckets, grouped for display
  string description = 3;
}
```

The mapping from model → bucket exists in `models/model_configs.proto`:

```proto
message BeyondModelInfo { string model_id = 1; string quota_bucket_team = 2; string quota_bucket_name = 3; }
message ModelDetails   { … QuotaInfo quota_info = 10; … BeyondModelInfo beyond_model_info = 44; … }
message QuotaInfo      { float remaining_fraction = 1; google.protobuf.Timestamp reset_time = 2; }
```

#### Observed live: three groups, five buckets, two windows

`retrieveUserQuotaSummary` returned `groups[]` in this shape (verbatim, from
[`evidence/live-2026-10-27/retrieveUserQuotaSummary.json`](evidence/live-2026-10-27/retrieveUserQuotaSummary.json)):

```
Gemini Models              2 buckets   gemini-weekly     weekly
                                        gemini-5h         5h
  "Models within this group: Gemini Flash, Gemini Pro"

Claude and GPT models      2 buckets   3p-weekly         weekly
                                        3p-5h             5h
  "Models within this group: Claude Opus, Claude Sonnet, GPT-OSS"

Gemini 4 Argon             1 bucket    gemini-argon-wtu-weekly   weekly
  "For paid Pro plans, Argon quota is separate. Higher Argon quota (included as
   part of the larger Gemini quota bucket) is available on Ultra plans."
```

So **every group but Argon carries two limits**, and Argon carries one. This is the single
most load-bearing structural fact for the UI: a group is not one bar, it is one bar per
bucket, and the Argon group is a special case rather than a peer.

The response's own top-level `description` states the consumption model, and it is the best
documentation we will get:

> Within each group, models share a weekly limit and a 5-hour limit. Quota is consumed
> proportionally to the cost of the tokens. Thus, limits will last longer with shorter tasks
> or using more cost-effective models. The 5-hour limit smooths out aggregate demand to
> fairly distribute global capacity across all users, while your weekly limit is tied
> directly to your individual tier.

Consequence: `remainingFraction` is **cost-weighted, not request- or token-counted**. Never
convert it to "requests left". It is a fraction of a cost budget, and the same fraction means
different things for Argon and for Flash Lite.

Each bucket also carries a server-written `description` (e.g. `"You have used some of your
5-hour limit, it will fully refresh in 3 hours, 50 minutes."`). That sentence already
contains the humanised countdown, so a client may render it instead of computing one — the
one case where parsing prose is safer than doing the arithmetic. The Argon bucket has no
`description`, so that path needs a fallback.

and in the client-side model config (`exa.codeium_common_pb`, `ClientModelConfig`):

```proto
message ClientModelConfig {
  string label = 1;  ModelOrAlias model_or_alias = 2;
  float credit_multiplier = 3;          // relative cost of one turn on this model
  bool disabled = 4;  bool supports_images = 5; … bool is_premium = 7;
  ModelProvider provider = 10;          // ANTIGRAVITY|OPENAI|ANTHROPIC|GOOGLE|XAI|DEEPSEEK|GLM|KIMI
  repeated TeamsTier allowed_tiers = 12; // which licence tiers may use this model
  ModelPricingType pricing_type = 13;    // UNSPECIFIED|STATIC_CREDIT|API|BYOK
  QuotaInfo quota_info = 15;
  string model_id = 21;
  …
}
```

So the answer to "per model, per family, per agent, per window, per credit balance" is:
**per server-defined bucket**, grouped for display, addressed per model through
`beyond_model_info.quota_bucket_name` / `quota_bucket_team`. The CLI never computes a bucket
from a model name; it renders whatever buckets the server names. `ModelDetails.api_provider`
(`API_PROVIDER_INTERNAL|GOOGLE_VERTEX|GOOGLE_GEMINI|ANTHROPIC_VERTEX|GOOGLE_EVERGREEN|OPENAI_VERTEX`)
and `model_provider` are routing facts, not quota selectors — `models/model_config.InferAPIProviderFromModelName`
only maps a `claude` prefix to `/anthropic/` and does not touch quota.

### 3.3 Client-side vs server-side enforcement

**Server-side, with a per-turn credit report back.** Nothing in the client blocks a request on
`remaining_fraction`; the client displays the numbers and *reacts* to errors.

Evidence:

* `You have exhausted your quota on this model.` is not a client condition. It is emitted by
  `cortex/utils/utils.CustomUserErrorMessageFromHTTPError` (LEA at `0x7aba9d5`) and
  `cortex/utils/utils.StatusFromErrorDetails` (LEA at `0x7abb7e7`) — both map a *server error*
  to user text. The mapped reason codes are string literals in those functions:

  | server reason (`google.rpc.ErrorInfo.reason`) | client message |
  |---|---|
  | `RATE_LIMIT_EXCEEDED` | `You have exhausted your quota on this model.` |
  | `INSUFFICIENT_G1_CREDITS_BALANCE` | `Your AI credits balance is too low to continue.` |
  | `VALIDATION_REQUIRED` | `Verification Required` |
  | `MODEL_CAPACITY_EXHAUSTED` | (capacity wording; nearby literals `individual quota reached`, `PREFILL_QUEUE_OVERLOADED`, `Overloaded prefill`) |

  `StatusFromErrorDetails` also references the gRPC status names (`RESOURCE_EXHAUSTED`,
  `FAILED_PRECONDITION`, `UNAUTHENTICATED`, …) and the literal `uiMessage` sits in the same
  rodata block; derived (medium confidence): a server-supplied `uiMessage` detail is preferred
  over the built-in text.
* `cli/model/model.quotaStatus` returns a **string**, not a decision
  (`"Refreshes in " + formatDuration(time.Until(reset))` or `"Quota exhausted"`).
* `transcriptionQuotaInfo` (`0x9255120`) reuses the same `RATE_LIMIT_EXCEEDED` mapping plus a
  `quotaResetDelay` field for the audio-transcription quota.
* Per-turn accounting: the server reports consumption in the stream metadata —
  `ChatModelMetadata.consumed_credits` (`ConsumedCredits{credits, first_step_index, last_step_index}`),
  `ChatModelMetadata.credit_cost`, `.model_cost`, plus `CompletionDelta.consumed_credits` and
  `RetryInfo.consumed_credits`. `cortex/utils/utils.consumedCreditsForHeader` reads them,
  `ComputeCreditUsageSummary` / `UpdateCreditUsageSummary` fold them into
  `jetski_cortex_go_proto.CreditUsageSummary{consumed_credits}` and
  `CostAccumulatorState{executions[], rolled_up}`, which are persisted in the unified state sync
  topic `cost_accumulator_state` (`language_server/state_sync/model_credits.go`). That local
  accumulator, not the server, backs the `"  AI Credit Overages: %s\n"` line.

Client-side "respect" therefore means: stop showing green when the server said
`RATE_LIMIT_EXCEEDED`, and prefer `remaining_fraction <= 0` (or `disabled`) as the display
condition for exhausted, without pretending to enforce.

### 3.4 Two credit concepts, easily confused

| name | source | semantics |
|---|---|---|
| **AI credits / G1 credits** (`useG1Credits`, `use_ai_credits`) | `LoadCodeAssistResponse.paid_tier.available_credits` | a prepaid balance that can absorb turns after quota is spent. Opt-in: `AI Credits not enabled (enable in /settings)`, `use_ai_credits` global user setting, `useG1Credits=true from %s` override, `[AuthProvider]`-side `SetUseG1Credits`/`GetUseG1Credits`, state-sync `GetUseAICredits`/`SetUseAICredits` |
| **model credits / credit multiplier** | `ClientModelConfig.credit_multiplier`, `ModelPricingType` | the *price* of a turn on a model, in credit units: `STATIC_CREDIT` (covered by the subscription), `API` (metered API billing), `BYOK` (your own key) |

`aiCredits` panel copy: `"MODEL CREDITS"`, `"  Available AI Credits: %s\n"`,
`"  AI Credit Overages: %s\n"`,
`"  Use your AI credits to fulfill model requests once you're out of quota.\n\n"`,
`"AI Credits not enabled (enable in /settings)"`.

---

## 4. Reset mechanics

### 4.1 The reset time is server-provided, per bucket

`reset_time` is a `google.protobuf.Timestamp` on every quota bucket, in all three quota shapes:

| message | field | notes |
|---|---|---|
| `QuotaSummaryBucket` | `reset_time = 6` | the one the CLI's usage panel uses |
| `QuotaInfo` (per model, in both `model_configs` and `codeium_common`) | `reset_time = 2` | served with `fetchAvailableModels` / model config |
| `FetchQuotaStatusResponse.BucketInfo` | `reset_time = 2` | business path, unused by the CLI |

There is **no rolling window computed client-side and no fixed clock boundary in the binary**.
The only client-side time logic is `now < reset_time`. The window itself is opaque:
`QuotaSummaryBucket.window` is a `string` (free-form server label, no enum anywhere), so a
reimplementation must display it verbatim rather than parse it.

Observed live, and this settles the question the static analysis could not: the reset times
are **rolling, anchored to when each bucket's window started**, and mutually unrelated. One
`retrieveUserQuotaSummary` at `2026-10-27T09:06:53Z` returned five buckets:

| bucket | `window` | `resetTime` | minus window | remaining |
|---|---|---|---|---|
| `gemini-weekly` | `weekly` | `2026-10-28T13:38:11Z` | → `10-21T13:38:11Z` | 0.4195 |
| `gemini-5h` | `5h` | `2026-10-27T12:57:10Z` | → `07:57:10Z` | 0.9863 |
| `3p-weekly` | `weekly` | `2026-10-30T16:20:30Z` | → `10-23T16:20:30Z` | 0.9838 |
| `3p-5h` | `5h` | `2026-10-27T12:57:14Z` | → `07:57:14Z` | 0.9675 |
| `gemini-argon-wtu-weekly` | `weekly` | `2026-11-03T09:06:53Z` | → **exactly now** | 1.0 |

Proof that this is not a clock boundary: nothing aligns to midnight or to a round hour. The
two 5-hour buckets reset **4 seconds apart**, so they track independent window starts rather
than a shared clock. And the Argon bucket's `resetTime` is precisely the request time plus
seven days — an untouched bucket whose window was created by this very request. That is
rolling-from-first-use, and a countdown rendered from it is only valid for that bucket.

Two consequences for us: never compute `resetTime` ourselves, and never assume a group's two
buckets share one clock even when their windows are the same length.

### 4.2 How the CLI renders it

`cli/model/model.quotaStatus` (fragment containing the LEAs at `0x984000a`–`0x9840086`):

```
call time.Now
call time.Time.Before          ; if now.Before(reset)
call time.Until                ;   d := time.Until(reset)
call model.formatDuration      ;   "3h 12m" / "45m"
runtime.concatstring3         ;   "Refreshes in " (len 0xd) + d  →  "Refreshes in 3h 12m"
…
else:  lea 0x59a5f1e, len 0xf  ;   "Quota exhausted"
```

`model.formatDuration` (0x98402c0+448) compares against `0xdf8475800` (60 s) and
`0x34630b8a000` (3600 s) and formats `%dh %dm` above an hour, minutes below. So:

* **TUI usage panel:** `Refreshes in <humanized duration>` while `reset_time` is in the future,
  `Quota exhausted` once it is not (`quotaStatus` is called from `UsageModel.renderQuotaGroup`
  at `0x983ed8a`), plus `" %.2f%%"` for the remaining fraction, `"Disabled: %s"` for
  `bucket.disabled`, and the group/bucket `display_name`/`description` as-is.
* **Machine/text output** (`printmode.quotaData`): `time.Time.Format("2006-01-02 15:04 MST")`
  (LEA at `0x989472d`); the RFC3339 layout `"2006-01-02T15:04:05Z07:00"` is used by the
  neighbouring `printmode.bucketResetLocal`.
* **`printmode.bucketReset(t, disabled)`** returns the zero time when `disabled` is set **or**
  the timestamp is zero — i.e. "no reset" and "disabled" are both rendered as absent, never as
  `epoch`.
* Other print-mode copy: `"Quota:%.0f%%"`, `"no quota summary is available for this account"`,
  `"no credit information is available for this account"`, `"Remaining credits"`,
  `"Model credits"`, `"Plan"`, `"retrieving quota summary: %w"`, `"retrieving credits: %w"`.

Note the two independent reset clocks a UI may have to reconcile: the **bucket** `reset_time`
(what the panel shows) and the **credit balance** refresh, which has no timestamp at all — only
the cached `paid_tier.available_credits` value.

### 4.3 Free/consumer vs paid

The binary distinguishes tiers by `UserTier.id` string only; there is no reset-policy switch.
Observed and derived:

* Consumer/free: `current_tier` is the free tier; the upsell text spells out the consumer
  allowance as **1,500 model requests per day**; `ReleaseChannel` and `upgrade_subscription_type`
  (`GOOGLE_ONE`, `GDP`, `*_HELIUM`) describe the paid options. The *actual* consumer reset
  boundary is whatever `reset_time` the server returns — the client never assumes midnight.
* Paid/credit: turns beyond the quota are paid from `paid_tier.available_credits` when
  `use_ai_credits` is on; the failure mode is `INSUFFICIENT_G1_CREDITS_BALANCE` →
  `Your AI credits balance is too low to continue.`, and there is no reset time associated with
  that balance anywhere in the client.
* Business/enterprise: quota arrives from the same `retrieveUserQuotaSummary` (the client has no
  separate business quota renderer); entitlement differences show up as `disabled` buckets,
  `allowed_tiers` on the model, and `IneligibleTier` entries.

---

## 5. Tier / billing decision tree

### 5.1 Three auth modes and one pending state

`codeassistclient.(*CLIAuthProvider).ResolveAuthMode` (0x83d6020+1408) is the decision point. It
calls `isBusinessAuth`, `IsBusinessLoginEnabled`, `GetEntitlement`, `GetCAICProject`, then logs
exactly one of:

| log line (literal) | resulting mode |
|---|---|
| `[AuthProvider] ResolveAuthMode: personal consumer mode` | `personal_consumer` |
| `[AuthProvider] ResolveAuthMode: pending business onboarding` | (no CAIC project yet) |
| `[AuthProvider] ResolveAuthMode: business paygo mode` | `business_agent_platform_paygo` |
| `[AuthProvider] ResolveAuthMode: AuthModeBusinessLicensedEnterprise userTier: %q` | `business_licensed_enterprise` |

Derived order (high confidence): if business login is not enabled → `personal_consumer`; else if
no CAIC project → pending onboarding; else switch on the entitlement's `userTier`:
`business_agent_platform_paygo` → pay-as-you-go, anything else → licensed enterprise (the log
prints the tier string). `IsBusinessEnterprise` compares against
`business_agent_platform_paygo` (`0x5a874e8`), i.e. "business, but not PAYG".

Every auth provider type implements the same four methods — `IDEAuthProvider`,
`HostAuthProvider`, `StandaloneAuthProvider`, `CLIAuthProvider`, `AntigravityHubAuthProvider`,
`StubbyAuthProvider`, `GeminiAPIKeyAuthProvider` — so the mode is a property of the provider,
not of the process.

### 5.2 Where the business user tier comes from

`codeassistclient.(*CLIAuthProvider).GetEntitlement` (0x83d7140) is
`os.LookupEnv("BAICODE_USER_TIER")` (verified: `mov ebx,0x11` = 17 = `len("BAICODE_USER_TIER")`
followed by a call to `os.LookupEnv` at `0x83d71f0`), folded into the entitlement that is sent as
`entitlement.userTier`.

`codeassistclient.(*CodeAssistClient).aicodeEntitlement` carries the other half and its literals
are the proof of the chain:

```
failed to get the user tier: %w
LoadCodeAssist response has no paid tier
failed to read AI Code response body: %w
failed to create FetchConfig request: %w
```

i.e. for the business surface the entitlement tier is **`loadCodeAssist().paid_tier.id`**, and
`loadCodeAssist` is the gate: no paid tier → no business path.

### 5.3 Environment surface (all observed as literals)

| var | use |
|---|---|
| `BAICODE_USER_TIER` | overrides the entitlement user tier (`GetEntitlement`) |
| `BAICODE_ENDPOINT_URL` | base URL for `getBusinessAICodeURL` and `CreateCloudCodeHTTPClient` |
| `BAICODE_PREDICTION_ENDPOINT_URL` | prediction base URL |
| `BAICODE_MANAGEMENT_ENDPOINT_URL` | management base URL (`.../{version}/projects/...:{operation}`) |
| `BAICODE_MANAGEMENT_RESOURCE_LOCATION` | resource location segment (`getBusinessAICodeResourceURL`) |
| `BAICODE_TELEMETRY_ENDPOINT_URI` | BAIC telemetry |
| `AGY_BUSINESS_PAYGO_TIER` | package-level string, present only as a `.lrodata` string value (`0x5a48d43`, length beside it at `0x17c3b0`); no LEA call site recovered |
| `AGY_LLM_GATEWAY_TIER_FLASH_LITE` | gateway tier flag |
| `JETSKI_SERVICE_ENDPOINT_URL` | jetski service override |
| `USER_INFO_OVERRIDE_URL` | people-API override |

`AGY_BUSINESS_PAYGO_TIER` is the one env var from the brief I could not pin to a read site: it
exists as a string constant in `.lrodata` and nothing else in the binary references that address
(scan of absolute pointers and RIP-relative LEAs). **UNRESOLVED** whether it is dead code, a
`flag`-registered value read through reflection, or consumed by a path my scan could not follow.

### 5.4 `AI Code FetchConfig`, `FetchLicenses`, `SelfAssignLicense`

The Business AI Code (BAIC) surface is real and partially live. `codeassistclient.init`
(0x83cbe60) builds an **operation-name → path** table:

```
generateContent  streamGenerateContent  predictionServiceSendTelemetry
queryConfig  fetchQuotaStatus  fetchConfig  fetchLicenses  selfAssignLicense  sendTelemetry
```

Path templates used with it: `/%s/projects/%s/locations/%s:%s` (version, project, location,
operation) and `/%s:fetchLicenses` (version only, location `global`).

| function | operation | what it does | live? |
|---|---|---|---|
| `(*CodeAssistClient).fetchAICodeConfig` | `fetchConfig` | `Fetched BAIC FetchConfigResponse: %v`; `FetchConfigResponse.Model` carries per-model `ModelProvider` + features for the *business* catalogue | yes |
| `(*CodeAssistClient).fetchBAICAdminControls` | `fetchConfig` | admin controls (`FetchAdminControlsRequest{project}`) | yes |
| `(*CodeAssistClient).IsBusinessLicensed` | `fetchConfig` | licensed iff the BAIC config resolves a licensed tier; same log line | yes |
| `(*CodeAssistClient).FetchLicenses` | `/%s:fetchLicenses` POST, location `global` | `failed to FetchLicenses: %w`; response `FetchLicensesResponse{licenses: [License{user_tier, tier_display_name, upgrade_subscription{text,url}}]}` | yes |
| `(*CodeAssistClient).SelfAssignLicense` | `/%s/projects/%s/locations/%s:selfAssignLicense` POST + `Content-Type: application/json` | requires a project id and a **multi-region** location: `invalid location %q for SelfAssignLicense: Gemini Enterprise licensing endpoints require a multi-region location (e.g. 'global', 'us', 'eu') and do not support regional endpoints`; errors `SelfAssignLicense failed (HTTP %d): %s`, `failed to marshal SelfAssignLicenseRequest`, `failed to unmarshal SelfAssignLicenseResponse`; sandbox host `https://autopush-aicode.sandbox.googleapis.com` present | yes |
| `loadCodeAssistResponseFromLicenses` | — | synthesises a `LoadCodeAssistResponse` out of the licence list, so the rest of the client keeps one tier model | yes |
| `fetchQuotaStatus` | `:fetchQuotaStatus` | **never called** (see §6) | no |

`FetchLicensesFromAICode` (0x83cf7c0+2784) is the alternate `businessaicode` variant: `GET
{fetchLicenses}`, `Making AI Code FetchLicenses request to: %s`, headers
`x-goog-sherlog-Link`, `x-cloudaicompanion-trace-id`.

### 5.5 Decision tree, condensed

```
aad/consumer login (auth_method=consumer, observed server_oauth.go:224)
  └─ ResolveAuthMode
       ├─ business login disabled ─────────────→ personal_consumer
       │     ├─ loadCodeAssist().current_tier  → plan label + upsell
       │     └─ retrieveUserQuotaSummary()     → buckets + reset_time  ← the only limits
       └─ business login enabled
             ├─ CAIC project missing ─────────→ pending onboarding (GCP setup / licence picker)
             └─ entitlement = BAICODE_USER_TIER || loadCodeAssist().paid_tier.id
                   ├─ "business_agent_platform_paygo" →  PAYG (BAIC billing, same quota API)
                   └─ otherwise                       →  licensed enterprise
                         └─ FetchLicenses / SelectLicense / SelfAssignLicense
                              → loadCodeAssistResponseFromLicenses → same tier model
```

---

## 6. Seat management: live surface vs dead proto

`exa.seat_management_pb.SeatManagementService` **is linked** — 1,833 functions in
`data/funcs.txt` reference the package, including every message accessor and
`UnimplementedSeatManagementServiceServer`. That is the whole Codeium/Windsurf team backend:
`Team`, `TeamConfig`, `BillingUpdate{AmountDueImmediately, PricePerSeat, NumSeats, SubInterval,
UnusedPlanRefunded, HasSsoAddOn}`, `EnterpriseLicenseConfig{SeatLimit, MucsTotal, LicenseLengthMonths,
…}`, `FlexCreditChronicleEntry`, `GetCascadeAnalyticsResponse{UsedPromptCredits, UsedFlowCredits,
PurchasedCredits}`, `GetPlanStatusResponse{plan_status}`, `GetUserStatusResponse{user_status,
plan_info}`, `MigrateApiKeyRequest{api_key} → MigrateApiKeyResponse{session_token}`,
`AddTeamDomain`, `SAMLAuthProvider`, …

### 6.1 How to tell it is dead surface

1. **No client stubs.** `data/funcs.txt` contains no `NewSeatManagementServiceClient`, no
   `SeatManagementServiceClient`, and no `_SeatManagementService_*_Handler` invoker. Only the
   server-side `Unimplemented…Server` and per-message accessors are present — the signature of a
   generated `pb.go`/`grpc.pb.go` that was *linked because something imports the package*, not
   because anything calls it.
2. **No request builders.** Nothing constructs `GetUserStatusRequest{metadata}`,
   `GetPlanStatusRequest{auth_token}`, `MigrateApiKeyRequest{api_key}`, or
   `AddTeamDomainRequest{auth_token, domain}`.
3. **The CLI's own licence API is a different service.** The client-facing surface is
   `exa.language_server_pb.LanguageServerService` with exactly **three** licence RPCs:
   `ListLicenses`, `SelectLicense`, `SelfAssignLicense` (plus `AuthResult.license_required` /
   `LicenseRequiredInfo`), implemented in `language_server.(*Server).{ListLicenses,SelectLicense,SelfAssignLicense}`
   and `authclient.(*AuthClient).SelectLicense` / `(*GatewayAuthClient).SelectLicense`.
4. **Two different `License` messages.** `exa.language_server_pb.License` is
   `{user_tier, tier_display_name, project_id, location}` — GCP-shaped. The
   `seat_management_pb` team/billing objects are Windsurf-shaped. The CLI only ever uses the
   former; `ListLicensesRequest` is empty and `SelectLicenseRequest{user_tier, project_id,
   location, tier_display_name}` mirrors the Google licence tuple.

### 6.2 What is actually live

| RPC (language-server API) | trigger | reads |
|---|---|---|
| `ListLicenses` | `GcpSetupModel.fetchLicensesCmd` during GCP/enterprise setup; also `store.(*Manager).FetchLicenses` → `backend.(*ServerBackend).FetchLicenses` (`Cloud Assist service connection is unavailable` on failure) | `License{user_tier, tier_display_name, project_id, location}`, `ListLicensesResponse.data_sharing_disclaimer` |
| `SelectLicense` | `GcpSetupModel.applyLicenseSelection` after the user picks `"%s (Project: %s, Location: %s)"` from `"Select available license:"` | `SelectLicenseResponse.auth_result` → `applyAuthResult` |
| `SelfAssignLicense` | `GcpSetupModel.selfAssignLicenseCmd` in the `"Select License"` / `"No license"` menu; licence text `Licenses are provided by your organisation admin. Your Customer Data will be logged into the project that corresponds to …` | `SelfAssignLicenseResponse{license, assigned, error_message, data_sharing_disclaimer}`; success logs `[Auth State Transition] GCP setup completed successfully for project %s` |

Everything else in `seat_management_pb` — `GetUserStatus`, `GetPlanStatus`, `MigrateApiKey`,
`UpdateProfile`, `AddTeamDomain`, billing, SSO, FlexCredits — is **dead surface**: a vendored
Codeium backend proto kept alive by a package-level import. A reimplementation should not model
it.

---

## 7. Refresh, cache and throttle policy

Three independent layers, all measured from immediates:

| layer | value | where |
|---|---|---|
| HTTP cache TTL (every Code Assist cache) | **300 s** | `codeassistclient.NewCodeAssistClient`, `movabs rdi,0x45d964b800` before each `cache.NewCache` (five instances) |
| quota reload hard throttle | **60 s** | `store.(*Manager).doRefreshQuota`: `time.Since(lastQuotaRefresh)` vs `movabs rdx,0xdf8475800` at `0x95c3be7`; below it → `doRefreshQuota: skipped (throttled)` (`quota_manager.go:41`) |
| G1 credits reload throttle | **5 s** | `store.(*Manager).refreshG1CreditsAt`: `time.Time.Sub` then `movabs rdx,0x12a05f200` at `0x958e058` |
| model config reload | **300 s** | `store.(*Manager).ReloadModelConfig` |
| statusline repaint | **5 s** | `store.(*StatusLineRunner).run` |
| eligibility wait | 30 s | `store.(*Manager).waitForEligibility`, `printmode.waitForEligibility` |

`doRefreshQuota(force bool)` reconstructed from the disassembly at `0x95c3ad3`–`0x95c3f26`:

```
m.mu.Lock()
auth  := m.<token/auth state>          // nil → "skipped (not logged in)"  (quota_manager.go:36)
last  := m.lastQuotaRefresh            // time.Time at m+0x568..0x578
m.mu.Unlock()
if !loggedIn      → log, return
if !force && time.Since(last) < 60s → log "skipped (throttled)", return
log "doRefreshQuota: starting reload (force=%t)"        (quota_manager.go:45)
resp, err := m.backend.RetrieveUserQuotaSummary(ctx)    // singleflight, 300 s cache
if err != nil → log "Failed to retrieve user quota summary: %v", return
m.mu.Lock(); m.quotaSummary = deepCopy(resp); m.lastQuotaRefresh = time.Now(); m.mu.Unlock()
m.publishSnapshot()                                     // → UI
```

* **What forces a refresh:** the `force` argument (`force=%t` in the log). `TriggerQuotaRefresh`
  (0x95c39c0) is the named entry point, and `doRefreshQuota` is also called from
  `refreshExperiments-fm` after a feature-flag refresh. Observed in klog:
  `quota_manager.go:45] doRefreshQuota: starting reload (force=true)` immediately after
  `server_oauth.go:224] applyAuthResult: email=…, authMethod=consumer, quotaProject=` and after
  `model_config_manager.go:340] Propagating selected model override to backend: label="…"` —
  i.e. **after login and after a model switch**.
* **What schedules it:** `store.(*Manager).startBackgroundWorkers` registers
  `doRefreshQuota-fm` and `doRefreshG1Credits-fm` through `async.(*Group).StartLoop` /
  `StartForceLoop` / `Start`; the loop period is a Manager field, not an immediate in the
  function, so **the background period is UNRESOLVED** (the 60 s throttle is the floor in any
  case).
* **When it is skipped:** not logged in (`quota_manager.go:36`, logged once per call, dominates
  the observed logs because the earlier sessions had no token), throttled (`:41`), or the
  singleflight is already in flight (`cache.go:135 Cache(loadCodeAssistResponse): Singleflight
  refresh failed: error getting token source: You are not logged into Antigravity.`).
* **How state reaches the UI:** `publishSnapshot` → the `store.(*Manager)` snapshot that
  `cli/model/model.UsageModel` (`Store()`, `NewUsageModel`, `UsageModel.Update`) and
  `cli/model/model.CreditsModel` subscribe to; repainted every 5 s by
  `(*StatusLineRunner).run` when the statusline is on. `/usage` (`usageCommand`,
  `"View model quota usage"`) and `/credits` (`creditsCommand`) open those two models; their
  renderers and store accessors contain no HTTP literals, so the panels display the snapshot
  rather than fetching payloads themselves. **UNRESOLVED:** whether opening a panel additionally
  forces a reload — `TriggerQuotaRefresh` exists but its pclntab entry is a split fragment, so my
  call-target scan found no callers for it either way.
* `printmode.usageReport` / `quotaRows` / `quotaData` / `creditsReport` are the headless
  equivalent and *do* fetch (they surface `retrieving quota summary: %w`,
  `retrieving credits: %w`, `no quota summary is available for this account`).

---

## 8. What a client must do

**Contract, minimal and sufficient:**

1. `POST /v1internal:loadCodeAssist` once per session (and after login / model change).
   Read `current_tier.{id,name,description,upgrade_*}`, `paid_tier.available_credits[].credit_amount`,
   `g1_tier`, `ineligible_tiers[]`, `cloudaicompanion_project`. This is the only source of
   tier/plan/credit-balance; it carries **no limits and no reset times**.
2. `POST /v1internal:retrieveUserQuotaSummary` with `{project: cloudaicompanion_project}`.
   Read `groups[]` → `buckets[]` (or the flat `buckets[]`) with
   `{bucket_id, display_name, description, window, remaining_fraction | remaining_amount,
   reset_time, disabled}` and the top-level `description`. This is the only source of limits and
   resets. `reset_time` is authoritative; `window` is a display string.
3. Optionally `v1internal:fetchAvailableModels` for the per-model `quota_info` and
   `beyond_model_info.quota_bucket_*` (bucket → model mapping) if the UI wants per-model rows.
4. Treat exhaustion as `remaining_fraction <= 0 || disabled || now >= reset_time`, and let the
   server be the enforcer: on `429`/`RESOURCE_EXHAUSTED`, read `ErrorInfo.reason` and map
   `RATE_LIMIT_EXCEEDED` / `INSUFFICIENT_G1_CREDITS_BALANCE` / `VALIDATION_REQUIRED` /
   `MODEL_CAPACITY_EXHAUSTED` to the same user text the CLI uses.

**Polling.** The CLI's own numbers are the safe envelope: cache for **300 s**, never re-fetch
sooner than **60 s** after the last refresh unless something user-visible changed (login, model
switch), and repaint from cache at 5 s. Polling `retrieveUserQuotaSummary` faster than once a
minute exceeds what the vendor client does and gains nothing — the numbers only move when a turn
completes, and the turn already returns `consumed_credits`.

**Fields a reimplementation should surface**, in the CLI's own vocabulary:
plan (`Your Plan: %s`), upgrade URI/text, per-bucket `display_name` + `description` + `window`,
remaining as a percentage (`Quota:%.0f%%` / `" %.2f%%"`), `Refreshes in <duration>` or
`Quota exhausted`, `Disabled: %s`, AI-credit balance (`Available AI Credits: %s`), local credit
overage (`AI Credit Overages: %s`), and the `use_ai_credits` on/off state (`AI Credits not
enabled (enable in /settings)`).

---

## 9. Open questions / unresolved

1. **`AGY_BUSINESS_PAYGO_TIER` read site.** The literal exists (`0x5a48d43`, dup at `0x5c25d30`)
   with a string-value header at `0x17c3b0`, and one data pointer to it (`0xb203344`), but no
   code LEA. Dead, reflection-read, or reached through a table I did not resolve.
2. **Background worker period** for `doRefreshQuota` / `doRefreshG1Credits`
   (`startBackgroundWorkers` → `async.(*Group).StartLoop`). The interval is a struct field; only
   the 60 s / 5 s in-function throttles are measured.
3. **`retrieveUserQuota` (non-summary, `0x5d2b023`).** The endpoint string exists next to
   `retrieveUserQuotaSummary` and `fetchAvailableModels` but no LEA references it; presumably
   legacy. Not proven dead by call graph.
4. **`fetchQuotaStatus` (BAIC `:fetchQuotaStatus`).** Registered in the operation map, message
   shapes recovered (`BucketInfo{remaining_fraction, reset_time, quota_type=REQUESTS, model}`),
   but zero call sites. Whether the business path falls back to `retrieveUserQuotaSummary`
   (likely, since there is one renderer) is **UNRESOLVED**.
5. **`uiMessage`.** The literal sits in the rodata block `StatusFromErrorDetails` reads; I infer
   the server can override the error text but did not observe a payload.
6. **Whether `ModelDetails.quota_info` / `ClientModelConfig.quota_info` are rendered anywhere
   in the TUI.** `GetQuotaInfo` has no direct call site (inlined), and
   `UsageModel.renderQuotaGroup` renders `retrieveUserQuotaSummary` buckets, not per-model
   configs. Per-model quota may only feed model-picker greying — not confirmed.
7. **`ContextWindow.QuotaMultiplier`** (`backend.(*ContextWindow).QuotaMultiplier`,
   `ContextWindowOption.expensive`): a long-context option appears to multiply quota
   consumption. No multiplier value or binding to `credit_multiplier` was recovered.
8. **Reset semantics for the consumer free tier.** Nothing in the binary fixes a daily
   boundary; the 1,500-requests/day figure is marketing copy in the upsell string, and the real
   boundary is whatever `reset_time` the server returns. A live observation of `reset_time`
   across a boundary would settle it (not done here: no quota polling permitted).
