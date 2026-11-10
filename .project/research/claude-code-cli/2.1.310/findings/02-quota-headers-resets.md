---
document:
  type: "research"
  id: "RES-claude-code-cli-2.1.310-findings-02-quota-headers-resets"
  title: "Claude Code 2.1.310 quota, headers, spend and reset contracts"
  description: "Static reconstruction from the installed executable's extracted JavaScript, with wire and scenario-harness behavior separated."
  status: "published"
  revision: 3
  created_at: "2026-10-26T12:17:12+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "worker"
  updated_by: "orchestrator"
  source: "pi-quotas@78268296fed62ba14601b8cb80f1ff45a5c04796:.project/research/claude-binary-re/findings/02-quota-headers-resets.md"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/claude-code-cli/binary-re/findings/02-quota-headers-resets.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 3
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/claude-code-cli/binary-re/findings/02-quota-headers-resets.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---

# Scope and evidence convention

Target: installed Claude Code **2.1.310**; static inspection only, no authenticated requests. **Observed** means a literal/schema in the executable; **inferred** means an interpretation of its data flow; **unknown** means this inspection cannot establish it. All code citations below refer to **`/tmp/cc_code.txt`**. Offsets are zero-based **Unicode character offsets in the complete corpus**, not ctx.py's per-line offsets, byte offsets, or native instruction addresses. A range names the inspected region containing the exact trimmed excerpt. Separate excerpts in one fence are separated by `// …`; those separators are editorial, not executable source. Extraction provenance is in `../tools/CORPUS.md`.

Critical distinction: `Ob` is a **scenario/mock state class**, not the production wire parser. Its disabled accessors are direct evidence; do not turn its knobs or invented balances into a provider contract. The real readers are `we`, `Qt`, `mt`, and the low-priority parser family. The bundled gateway additionally **writes** compatible headers; its policy is not necessarily Anthropic's policy.

```js
function wqo(){let e=en();return e.enabled,null;return e.headers}
// …
function jvt(){return!1}
```
Evidence: 5,349,000–5,350,700. This report uses no credential values.

## 1. Usage endpoint contract

**Observed:** there is no single exhaustive validator on every successful fetch. A permissive known-field test (`KWe`) accepts a body containing any recognized usage key; `ri` validates the **persisted cache**, while the gateway spend lane has its own stricter validator. Preserve unknown fields rather than treating these as a closed server schema.

Evidence: 9,618,500–9,619,000; 9,639,300–9,640,600:
```js
function KWe(e){return typeof e==="object"&&e!==null&&!Array.isArray(e)&&At.some((n)=>(n in e))}
// …
let e=Tt({utilization:bn().nullable(),resets_at:ge().nullable()}).passthrough(),n=Tt({five_hour:e.nullish(),seven_day:e.nullish(),seven_day_oauth_apps:e.nullish(),seven_day_opus:e.nullish(),seven_day_sonnet:e.nullish(),cinder_cove:e.nullish(),wattle_ember:e.extend({label:ge().nullish(),limit_dollars:bn().nullish(),used_dollars:bn().nullish(),remaining_dollars:bn().nullish(),periods:lr(Tt({starts_at:ge(),ends_at:ge()}).passthrough()).nullish()}).nullish(),extra_usage:Tt({is_enabled:Go(),monthly_limit:bn().nullable(),used_credits:bn().nullable(),utilization:bn().nullable(),currency:ge().nullish(),disabled_reason:ge().nullish()}).passthrough().nullish(),limits:lr(Tt({kind:ge(),group:ge(),percent:bn(),resets_at:ge().nullable(),severity:ge(),is_active:Go(),scope:Tt({model:Tt({display_name:ge()}).passthrough().nullish(),surface:Tt({display_name:ge()}).passthrough().nullish()}).passthrough().nullish()}).passthrough()).nullish()}).passthrough();
```

Decoded schema (`nullish` = absent or null; `nullable` = present but may be null):

- `five_hour`, `seven_day`, `seven_day_oauth_apps`, `seven_day_opus`, `seven_day_sonnet`, `cinder_cove`: nullish window objects with `utilization: number|null`, `resets_at: string|null`; extra keys survive. First five names identify session, account-weekly, OAuth-app-weekly and model-weekly windows. **Unknown:** product meaning of `cinder_cove`.
- `wattle_ember`: same base plus nullish `label: string`, `limit_dollars`, `used_dollars`, `remaining_dollars: number`, and `periods: {starts_at: string, ends_at: string}[]`. Dollar units are explicit in names; **unknown:** exact product/eligibility and whether periods are rolling or calendar-based.
- `extra_usage`: nullish object; required boolean `is_enabled`; required nullable numbers `monthly_limit`, `used_credits`, `utilization`; nullish strings `currency`, `disabled_reason`. Money is **minor currency units** (USD cents); a null cap must not be coerced to zero. `is_enabled=false` means it cannot cover sends, not necessarily that no balance exists.
- `limits`: nullish ordered array; each entry requires `kind`, `group`, `severity: string`, `percent: number`, `resets_at: string|null`, `is_active: boolean`; nullish `scope`, with nullish `model`/`surface`, each requiring `display_name: string`. Objects allow additional keys.
- `limits[].percent` is **0–100 share used**, not a fraction. `resets_at` is ISO 8601. `kind` examples are `session`, `weekly_all`, `weekly_scoped`; `group` organizes rows, `severity` examples are `normal`, `warning`, `critical`; these are open strings, not closed enums. `is_active` is the server's single-value/headline pick. Render rows in server order and do not classify them by display label.
- Conventional window `utilization` uses the same display domain as `limits[].percent`: per-model row conversion assigns `percent` directly to `utilization` (evidence below). **Unknown:** full numeric constraints on each legacy field; the persisted validator merely accepts a number.
- `weekly_scoped` is a **row kind**, not an observed top-level key. `seven_day_cowork` and `seven_day_omelette` are recognized reset-grant limit names (§2), not members of this cache schema. `seven_day_breakdown` and therefore `seven_day_breakdown.rows[]` have **zero occurrences in either corpus**; no schema can be reconstructed. Absence here does not prove a server cannot send it.

Evidence for units/row semantics: 4,388,726–4,390,900 (SDK structured `/usage` twin):
```js
percent:v().describe("Share of the window used, 0-100."),resets_at:o().max(qe).nullable().describe("ISO 8601 timestamp when the window resets.")
// …
severity:o().max(qe).describe("The server's reading of the row for a meter's colour, e.g. 'normal', 'warning' or 'critical'. Every row here is the server's, so a client never grades a row itself."),is_active:M().describe("The server's headline pick: the row a single-value indicator shows.")
// …
"Extra-usage (overage) spend for the billing period, when the plan has it. Amounts are in minor units of `currency` (cents for USD); is_enabled is false while extra usage cannot cover sends."
```

The requested nonnegative chain is **gateway-specific**, not the main OAuth-body validator. It requires numeric spend fields, tolerates malformed optional `limits` entries as `{}`, chooses the first active row, normalizes a three-letter currency to uppercase/default USD, and accepts a reset only if `Date.parse` succeeds. Evidence: 12,204,000–12,204,900:
```js
var vOo=p(()=>u({extra_usage:u({monthly_limit:v().nonnegative(),used_credits:v().nonnegative(),utilization:v().nonnegative(),currency:o().regex(/^[A-Za-z]{3}$/).nullish().catch(void 0)}),limits:T(u({group:o().nullish().catch(void 0),resets_at:o().nullish().catch(void 0),is_active:M().nullish().catch(void 0)}).catch({})).nullish().catch(void 0)}));
// …
return{used:r.used_credits,limit:r.monthly_limit,currency:r.currency?.toUpperCase()??"USD",utilization:r.utilization,resetsAt:h?.resets_at&&Number.isFinite(Date.parse(h.resets_at))?h.resets_at:null,period:SOo.find((y)=>y===h?.group)??null}
```

**Utilization caution:** the bundled gateway explicitly emits `extra_usage.utilization` as whole percent, potentially above 100; main OAuth's cache schema supplies no scale constraint. This is strong evidence for the gateway lane, not proof of every Anthropic account's live payload. Evidence: 32,121,000–32,121,500:
```js
extra_usage:{is_enabled:!0,monthly_limit:Number(o.cap_cents),used_credits:Math.floor(o.spent_cents),utilization:Math.round(C0(o)*100),currency:"USD"}
```

`resetTime` and `limitName` are **formatted client output**, not evidence of those wire keys. Evidence: 9,625,200–9,625,800:
```js
function HHr(e){let n=e.resetsAt?LE(e.resetsAt,!0):void 0;if(!n)return null;switch(e.rateLimitType){case"five_hour":case"seven_day":case"seven_day_opus":return{limitName:Qle[e.rateLimitType],resetTime:n};case"seven_day_sonnet":return{limitName:q$t(),resetTime:n};case"seven_day_overage_included":case"overage":case void 0:return null;default:return null}}
```

## 2. What the query flags change

**Observed URL construction**, 9,621,400–9,622,100:
```js
var me="starry_waterfall";function Be(e){return e.surface??"cli_menu"}function le(e){switch(e.read){case void 0:case"plain":return"/api/oauth/usage";case"cedar_ember":return"/api/oauth/usage?cedar_ember=1&skip_spend=1";case"at_wall":return zgt()?`/api/oauth/usage?at_wall=1&${me}=${Be(e)}&skip_spend=1`:"/api/oauth/usage?at_wall=1&skip_spend=1";case"starry_waterfall":return`/api/oauth/usage?${me}=${e.surface}&skip_spend=1`}}
```

- `skip_spend=1`: accompanies supplementary offer/reset reads. **Inferred:** avoids ordinary spend work. **Unknown:** server-side omission/cost semantics; no client-side branch proves precisely which fields disappear.
- `cedar_ember=1`: requests the `cedar_ember` **reset-grant** block. This is also a reset program identifier, not just a query nickname. `at_wall` reads can also consume `cedar_ember`.
- `at_wall=1`: fetches wall-time recovery eligibility; the client consumes `juniper_tide` and can obtain cedar grants in the same response. **Inferred:** “at a usage wall”; it is not a timestamp or timezone flag.
- `starry_waterfall=<surface>`: requests a usage-credit promotion block. Combined with `at_wall` only when `tengu_starry_waterfall.enabled`, `!_t()`, and organization policy `allow_promo_offers` all pass. Meaning of `_t()` alone is not established here.
- Accepted surface enum: `cli_menu`, `cli_credits_required`, `ide_banner`, `alert_band`, `overage_band`, `code_usage_band`, `credits_required`, `code_credits_required`, `unknown`. **Observed actual CLI fetch callers:** first three; default surface is `cli_menu`. Other enum entries are accepted telemetry/response vocabulary, not demonstrated CLI fetch calls.

Evidence: 9,617,000–9,617,700; 36,186,800–36,188,100; 36,192,000–36,192,500:
```js
function zgt(){return de().enabled===!0&&!_t()&&on("allow_promo_offers")}
// …
q=["cli_menu","cli_credits_required","ide_banner","alert_band","overage_band","code_usage_band","credits_required","code_credits_required","unknown"]
// …
function tus(e,n){let r=E(e);if(!zgt()||!yae()||r===null)return;k(r,"cli_menu",n)}function nus(e){if(!zgt()||!yae())return;k(ne,"cli_credits_required",e)}function rus(e){let n=E(e);if(!zgt()||!Qm()||!te()||n===null)return;k(n,"ide_banner",void 0)}
```

Supplementary response contracts, **observed**:

- `cedar_ember`: `eligible` boolean; optional/nullish `ineligible_reason`, `at_limit`, `exhausted: limitName[]`, `grants`, `next_grant_id`, `weekly_resets_at`, `cooldown_until`, `event_props`. Each parsed grant: `id`, `label`, nonnegative integer `resets_total`, `resets_left`, string/null `starts_at`, `ends_at`, limit lists `clears`, `blocking`, booleans `paused`, `usable_now`, `use_requires_limit`, `gives_back_weekly`, `percent_used` map (retains only integer 0–100 recognized names), optional `type: session|weekly|full`, `unavailable_reason`. Invalid grants are dropped; next id must match a parsed grant. Event props: `surface`, `tier`, `tenure_bucket`, `billing_path`, `billing_period`, `extra_usage_state`.
- Recognized grant names: `five_hour`, `seven_day`, `seven_day_overage_included`, `seven_day_opus`, `seven_day_sonnet`, `seven_day_cowork`, `seven_day_omelette`, `seven_day_oauth_apps`.
- `juniper_tide`: `eligible`, `ineligible_reason`, `in_experiment`, `arm: control|reset|null`, `available`, `next_available_at`, `weekly_resets_at`, `resets_per_week` (client default 1), same event-prop categories. It is a reset opportunity, not a usage meter.
- `starry_waterfall`: `eligible`, `ineligible_reason`, `active`, `arm_id`, `kind: none|pct_off|extra_credit|null`, `headline_pct` integer 0–300, `menu: base|plus40|unknown`, `default_bundle_id`, `segment: never|lapsed|recent|unknown`, `first_offered_at`, `expires_at`, nonnegative integer/null `days_left`, `redeemed`, `event_props` (`offer_eligible`, `offer_reason`, `offer_arm`, `offer_kind`, `offer_menu`, `offer_segment`, `offer_days_left`, `offer_surface`). Timestamp fields validate as strings, not strict ISO dates.

Evidence: 36,194,325–36,199,850; 36,240,000–36,243,800; 36,187,000–36,190,500:
```js
Ypr=["five_hour","seven_day","seven_day_overage_included","seven_day_opus","seven_day_sonnet","seven_day_cowork","seven_day_omelette","seven_day_oauth_apps"]
// …
pe=p(()=>u({id:o().regex(x),label:o().catch(""),resets_total:v().int().min(0).catch(0),resets_left:v().int().min(0),starts_at:o().nullable().optional().catch(null),ends_at:o().nullable().optional().catch(null),clears:S(),paused:M().catch(!1),usable_now:M().catch(!1),use_requires_limit:M().catch(!0),percent_used:me(o(),oe()).catch({}),blocking:S(),type:B(["session","weekly","full"]).optional().catch(void 0),gives_back_weekly:M().catch(!1),unavailable_reason:B(se).nullable().optional().catch(null)}))
// …
return{kind:"answered",status:Nr(s?.juniper_tide)}
// …
let r=await _O(e,{read:"starry_waterfall",surface:n});
// …
function L(e){return e!==void 0&&e!==null&&e.active&&!e.redeemed&&(e.kind==="pct_off"||e.kind==="extra_credit")&&e.headlinePct>0&&e.daysLeft!==null&&e.daysLeft>0}
```
Both reset programs POST `/api/organizations/${org}/reset_rate_limits`: cedar sends `{program:ae,grant_id:n,request_id:r}`; juniper sends `{program:Sr}`. Cedar results: `reset|already_used|not_limited|cooldown|ineligible|unavailable` with reason, remaining resets, cleared names and timestamps; juniper lacks cedar's `cooldown` result and grant fields. Evidence: 36,197,000–36,200,000; 36,240,000–36,244,500:
```js
jt.post(`/api/organizations/${i}/reset_rate_limits`,{program:ae,grant_id:n,request_id:r}
// …
jt.post(`/api/organizations/${s}/reset_rate_limits`,{program:Sr}
```

## 3. Every rate-limit header individually

Names below omit `anthropic-ratelimit-unified-`; only `retry-after` is bare. The corrected inventory contains **31 distinct literal names in code**, plus generic per-window expansion. Earlier counts of 22/23 excluded digits and were incomplete; `grace-5h-utilization0` in the strings corpus is adjacent-code concatenation, not a 32nd header. Header presence in the binary does not prove a particular server sends it.

**Real unified reader**, evidence 9,636,600–9,638,100 (anchor `function we(e,n)` follows `et`):
```js
let r=e.get("anthropic-ratelimit-unified-status")||"allowed",s=e.get("anthropic-ratelimit-unified-reset"),d=s?Math.round(Number(s)):void 0,f=e.get("anthropic-ratelimit-unified-fallback")==="available",m=e.get("anthropic-ratelimit-unified-representative-claim"),h=e.get("anthropic-ratelimit-unified-overage-status"),y=e.get("anthropic-ratelimit-unified-overage-reset"),S=y?Math.round(Number(y)):void 0,R=e.get("anthropic-ratelimit-unified-overage-disabled-reason"),A=at(e),L=e.get(Wfn)==="true",v=Ze(e),w=e.get("anthropic-ratelimit-unified-upgrade-paths"),E=w?w.split(",").map((ut)=>ut.trim()):void 0,x=e.get("anthropic-ratelimit-unified-overage-period-monthly-utilization"),I=x?Number(x):NaN,P=Number.isFinite(I)?{utilization:I}:void 0,z=e.get("anthropic-ratelimit-unified-overage-period-channel-utilization"),O=z?Number(z):NaN,Y=Number.isFinite(O)?{utilization:O}:void 0
```

| Header suffix | Observed shape/unit; reader → behavior |
|---|---|
| `status` | String, normally `allowed|allowed_warning|rejected`; absent/empty defaults allowed. `we` normalizes warning to allowed, then derives warnings separately; `mt` uses presence to determine whether limiter ran. |
| `reset` | Numeric **Unix epoch seconds**, rounded; `we` → `resetsAt`, reset messages/waiting. Harness setter instead accepts hours and converts (§4/§9). |
| `representative-claim` | String → `rateLimitType`, no whitelist in `we`; picks label/model/window. Not a bare `claim` wire header. |
| `fallback` | Exact `available` → boolean `unifiedRateLimitFallbackAvailable`; `we`, warning derivation/event state. Other strings are false; no broader enum observed. |
| `overage-status` | String, same three normal statuses; `we` → paid continuation/credit warning/refusal. |
| `overage-reset` | Numeric **epoch seconds**, rounded → `overageResetsAt`; paid-limit reset UI, combined reset choice (§6). |
| `overage-disabled-reason` | String; SDK enum and extra gateway value in §5; `we`/`Xe` cache it, fast mode and error UI distinguish no credits, disabled service, admin/member/group caps. |
| `overage-in-use` | Exact string `true` → independent boolean `overageInUse`; grace considers it coverage; low-priority stops when extra usage takes over. |
| `overage-scope` | `service|channel|group_pool|organization` accepted by `at`; SDK only exposes first three. Determines service/channel/team shared-budget wording. |
| `overage-period` | Gateway writer sends cap period `daily|weekly|monthly`; no corresponding field in `we`. Bundled gateway documentation and writer establish that lane, not a closed Anthropic server enum. |
| `overage-utilization` | Finite numeric **fraction**, may exceed 1; generic `Qt` → overage warning state. |
| `overage-surpassed-threshold` | Finite numeric **fraction**; `Qt` → warning trigger; gateway writes 0.75/0.95/1. |
| `overage-period-monthly-utilization` | Finite numeric fraction-of-cap → `overagePeriodMonthly.utilization`; SDK identifies Claude-in-Slack monthly service spend telemetry. |
| `overage-period-channel-utilization` | Same → `overagePeriodChannel.utilization`; absent if no per-channel cap. |
| `upgrade-paths` | Comma-separated trimmed strings → `upgradePaths`; UI checks `upgrade_plan` and `overage`, suppressing unavailable upgrade/credit actions (§6). Unknown extra strings survive. |
| `grace-5h-utilization` | Numeric fraction clamped 0–1; absent/nonfinite → 0. `mt` identifies session grace and wrap-up hints. Not a status enum. |
| `grace-7d-utilization` | Same, weekly grace; reset selection chooses controlling zone (§5). |
| `slow-offer` | `treatment|control`; anything else ignored. `NYs`/`Ze` → lower-priority offer fields. |
| `slow-status` | `active|not_needed|slot_busy|weekly_limit|budget_exhausted|ineligible|off`; absent undefined, unknown string `unrecognized`. `Pa` → wait/stop/served accounting (§10). |
| `slow-retry-after` | Nonnegative finite number **seconds**, `KBr` → `lowPriorityRetryAfterSeconds`, retry interval converted/clamped to ms. |
| `slow-max-wait` | Nonnegative finite **seconds**, `VBr` → max waiting budget converted/clamped to ms. |
| `slow-budget-utilization` | Nonnegative finite fraction capped at 1, `Ad` → remaining lower-priority allowance UI/telemetry. |
| `slow-budget-reset` | Positive finite **epoch seconds**, `Xg` → budget-exhausted expiry; falls back to `7d-reset`, clamps to now + 691200s (8 days). |
| `retry-after` | Harness/gateway write relative **seconds**. Fast-mode path parses integer seconds ×1000; short retry vs standard-speed fallback. Other retry helpers may support additional HTTP syntax; not established by this path. |

| Additional header suffix | Shape; reader → behavior |
|---|---|
| `5h-utilization` | Finite fractional usage; `Qt` → session window/warning. |
| `5h-reset` | Finite epoch seconds rounded; `Qt`/`mt` → session reset/grace. |
| `5h-surpassed-threshold` | Finite fraction; `Qt` → explicit session warning before heuristic. |
| `5h-status` | Three-status enum; `Qt` → session window state. |
| `7d-utilization` | Finite fractional usage; `Qt` → weekly window/warning. |
| `7d-reset` | Finite epoch seconds rounded; `Qt`/`mt`/`Xg` → weekly reset/grace/budget fallback. |
| `7d-surpassed-threshold` | Finite fraction; `Qt` → explicit weekly warning before heuristic. |
| `7d-status` | Three-status enum; `Qt` → weekly window state. |

These eight complete the 31 literal names. `Qt` also constructs `7d_oi-utilization`, `7d_oi-reset`, `7d_oi-surpassed-threshold`, `7d_oi-status` dynamically. Each utilization/threshold is a finite fraction, each reset finite epoch seconds rounded, and each status accepts exactly the three statuses. These drive session/weekly/overage-included model warnings and tracked-window events. `overage-*` shares this generic parser but deliberately ignores per-window status there (handled at top level instead).

Evidence: 9,633,800–9,635,200:
```js
var D=[["five_hour","5h"],["seven_day","7d"],["seven_day_overage_included","7d_oi"],["overage","overage"]]
// …
function Qt(e){let n={};for(let[r,s]of D){let d=be(e.get(`anthropic-ratelimit-unified-${s}-utilization`)),f=be(e.get(`anthropic-ratelimit-unified-${s}-reset`)),m=be(e.get(`anthropic-ratelimit-unified-${s}-surpassed-threshold`)),h=e.get(`anthropic-ratelimit-unified-${s}-status`),y=r==="overage"?void 0:qt.find((S)=>S===h);if(d===void 0&&f===void 0&&m===void 0&&y===void 0)continue;n[r]={...d!==void 0&&{utilization:d},...f!==void 0&&{resets_at:Math.round(f)},...m!==void 0&&{surpassedThreshold:m},...y!==void 0&&{status:y}}}return n}var qt=["allowed","allowed_warning","rejected"];
```

Low-priority numeric parsing evidence: 6,850,155–6,851,500:
```js
function NYs(e){let n=e?.get("anthropic-ratelimit-unified-slow-offer");return n==="treatment"||n==="control"?n:void 0}
// …
function Ma(e,n){let r=e?.get(n);if(r===null||r===void 0||r.trim()==="")return;let s=Number(r);return Number.isFinite(s)&&s>=0?s:void 0}function KBr(e){return Ma(e,"anthropic-ratelimit-unified-slow-retry-after")}function VBr(e){return Ma(e,"anthropic-ratelimit-unified-slow-max-wait")}function Ad(e){let n=Ma(e,"anthropic-ratelimit-unified-slow-budget-utilization");return n===void 0?void 0:Math.min(1,n)}
```

**Summary versus per-window preference:** the unprefixed status/reset/representative-claim describe the currently limiting summary; per-window headers independently track all known windows. There is no observed unprefixed `anthropic-ratelimit-unified-utilization`. This is not evidence of a legacy/new-version ordering. When summary is rejected it wins unchanged; when summary is allowed, per-window thresholds/heuristics may replace it with an allowed_warning. Server explicit thresholds beat local heuristics. Previous still-valid readings can supplement later observations; stale observations are fenced. Evidence: 9,637,700–9,638,000 and §10:
```js
function ei(e,n){if(e.status!=="allowed")return e;let r=et(n,e.unifiedRateLimitFallbackAvailable);return r?Se(r,e.upgradePaths,e):e}
```

## 4. Claim header and exceededLimits model

**Observed harness only**, 5,341,800–5,345,000:
```js
if(e==="reset"||e==="overage-reset"){let h=Number(n);if(!isNaN(h))n=String(Math.floor(Date.now()/1000)+h*3600)}if(e==="claim"){if(["five_hour","seven_day","seven_day_opus","seven_day_sonnet"].includes(n)){let y;if(n==="five_hour")y=Math.floor(Date.now()/1000)+18000;else if(n==="seven_day"||n==="seven_day_opus"||n==="seven_day_sonnet")y=Math.floor(Date.now()/1000)+604800;else y=Math.floor(Date.now()/1000)+3600;this.exceededLimits=this.exceededLimits.filter((S)=>S.type!==n),this.exceededLimits.push({type:n,resetsAt:y}),this.updateRepresentativeClaim();return}}
// …
let e=this.exceededLimits.reduce((n,r)=>r.resetsAt>n.resetsAt?r:n);
// …
addExceededLimit(e,n){this.enabled=!0;let r=Math.floor(Date.now()/1000)+n*3600;if(this.exceededLimits=this.exceededLimits.filter((s)=>s.type!==e),this.exceededLimits.push({type:e,resetsAt:r}),this.exceededLimits.length>0)this.headers["anthropic-ratelimit-unified-status"]="rejected";this.updateRepresentativeClaim()}
```

**Inferred:** `exceededLimits` is a deduplicated-by-type list of `{type,resetsAt}`. Setting one recognized `claim` replaces that type, adds 5h for `five_hour`, 7d for the three recognized weekly names, and selects the **latest reset** among exceeded limits. Equal reset times keep the earlier item (`>` rather than `>=`). `addExceededLimit(type,hours)` accepts arbitrary type and explicit hours, sets rejected, then recomputes.

The apparent **1h fallback is unreachable** inside this `claim` branch: the preceding four-name allowlist and if/else cover every accepted name. Do not claim unknown claims default to 1h in production. An unrecognized internal `claim` falls through to generic header storage; the real representative-claim reader accepts any string. Other observed type vocabulary includes `seven_day_overage_included`, `overage`, and the grant-only names in §2; those are not all accepted by the harness claim shortcut.

Representative recomputation writes `representative-claim`, `reset`, and (only when base rejected and overage absent/rejected) relative `retry-after`. Empty list deletes those three headers. Separate status updates synthesize retry-after under the same condition. Evidence: 5,342,700–5,344,500:
```js
updateRetryAfter(){let e=this.headers["anthropic-ratelimit-unified-status"],n=this.headers["anthropic-ratelimit-unified-overage-status"],r=this.headers["anthropic-ratelimit-unified-reset"];if(e==="rejected"&&(!n||n==="rejected")&&r){let s=Number(r),h=Math.max(0,s-Math.floor(Date.now()/1000));this.headers["retry-after"]=String(h)}else delete this.headers["retry-after"]}
// …
if(this.exceededLimits.length===0){delete this.headers["anthropic-ratelimit-unified-representative-claim"],delete this.headers["anthropic-ratelimit-unified-reset"],delete this.headers["retry-after"];return}
```

**Scenario harness inventory**, observed in `setScenario` at 5,345,700–5,348,800; entire class anchor 5,341,376. Below `N=now+18000s`, `W=now+604800s`, `M=first next month local midnight`; H means `setOverageScenarioHeaders(N,status,reason?)`: base rejected, representative/latest exceeded reset, overage status, overage-reset M, optional reason. It seeds five_hour=N only when the list is empty. Scenario entry clears headers/message/overages; only the three overage-active/warning/exhausted cases preserve prior exceededLimits. It does **not** reset every fast/trial/billing field, so this is not a fresh-object promise.

| Scenario | Header set | exceededLimits | Direct message/other state |
|---|---|---|---|
| `clear` | empty | unchanged | clears message/overages, disables harness |
| `normal` | status allowed, reset N | empty | none |
| `session-limit-reached` | claim five_hour, reset N, status rejected | five_hour=N | none |
| `approaching-weekly-limit` | status allowed_warning, reset W, claim seven_day | empty | none |
| `weekly-limit-reached` | claim seven_day, reset W, status rejected | seven_day=W | none |
| `overage-active` | H(allowed) | preserved or seeded | none |
| `overage-warning` | H(allowed_warning) | preserved or seeded | none |
| `overage-exhausted` | H(rejected) | preserved or seeded | none |
| `out-of-credits` | H(rejected,out_of_credits) | five_hour=N | none |
| `org-zero-credit-limit` | H(rejected,org_service_level_disabled) | five_hour=N | none |
| `org-spend-cap-hit` | H(rejected,org_level_disabled_until) | five_hour=N | sets mock overage cents |
| `member-zero-credit-limit` | H(rejected,member_zero_credit_limit) | five_hour=N | none |
| `seat-tier-zero-credit-limit` | H(rejected,seat_tier_zero_credit_limit) | five_hour=N | none |
| `opus-limit` | claim seven_day_opus, reset W, status rejected | seven_day_opus=W | none |
| `opus-warning` | status allowed_warning, reset W, claim seven_day_opus | empty | none |
| `sonnet-limit` | claim seven_day_sonnet, reset W, status rejected | seven_day_sonnet=W | none |
| `sonnet-warning` | status allowed_warning, reset W, claim seven_day_sonnet | empty | none |
| `fast-mode-limit` | status rejected | empty | duration 600000ms |
| `fast-mode-short-limit` | status rejected | empty | duration 10000ms |
| `extra-usage-required` | empty | empty | headerless429Message below |

Exact branch evidence (5,345,700–5,348,800); scenario headers are inputs to downstream messages in §6, not messages emitted by setScenario:
```js
setScenario(e){if(e==="clear"){this.headers={},this.headerless429Message=null,this.overages=null,this.enabled=!1;return}this.enabled=!0;let n=Math.floor(Date.now()/1000)+18000,r=Math.floor(Date.now()/1000)+604800;if(this.headers={},this.headerless429Message=null,this.overages=null,!["overage-active","overage-warning","overage-exhausted"].includes(e))this.exceededLimits=[];switch(e){case"normal":this.headers={"anthropic-ratelimit-unified-status":"allowed","anthropic-ratelimit-unified-reset":String(n)};break;case"session-limit-reached":this.exceededLimits=[{type:"five_hour",resetsAt:n}],this.updateRepresentativeClaim(),this.headers["anthropic-ratelimit-unified-status"]="rejected";break;case"approaching-weekly-limit":this.headers={"anthropic-ratelimit-unified-status":"allowed_warning","anthropic-ratelimit-unified-reset":String(r),"anthropic-ratelimit-unified-representative-claim":"seven_day"};break;case"weekly-limit-reached":this.exceededLimits=[{type:"seven_day",resetsAt:r}],this.updateRepresentativeClaim(),this.headers["anthropic-ratelimit-unified-status"]="rejected";break;case"overage-active":this.setOverageScenarioHeaders(n,"allowed");break;case"overage-warning":this.setOverageScenarioHeaders(n,"allowed_warning");break;case"overage-exhausted":this.setOverageScenarioHeaders(n,"rejected");break;case"out-of-credits":this.setOverageScenarioHeaders(n,"rejected","out_of_credits");break;case"org-zero-credit-limit":this.setOverageScenarioHeaders(n,"rejected","org_service_level_disabled");break;case"org-spend-cap-hit":this.setOverageScenarioHeaders(n,"rejected","org_level_disabled_until"),this.setOverages({isEnabled:!0,balanceCents:6532,spendLimitCents:4000,usedCents:4000});break;case"member-zero-credit-limit":this.setOverageScenarioHeaders(n,"rejected","member_zero_credit_limit");break;case"seat-tier-zero-credit-limit":this.setOverageScenarioHeaders(n,"rejected","seat_tier_zero_credit_limit");break;case"opus-limit":{this.exceededLimits=[{type:"seven_day_opus",resetsAt:r}],this.updateRepresentativeClaim(),this.headers["anthropic-ratelimit-unified-status"]="rejected";break}case"opus-warning":{this.headers={"anthropic-ratelimit-unified-status":"allowed_warning","anthropic-ratelimit-unified-reset":String(r),"anthropic-ratelimit-unified-representative-claim":"seven_day_opus"};break}case"sonnet-limit":{this.exceededLimits=[{type:"seven_day_sonnet",resetsAt:r}],this.updateRepresentativeClaim(),this.headers["anthropic-ratelimit-unified-status"]="rejected";break}case"sonnet-warning":{this.headers={"anthropic-ratelimit-unified-status":"allowed_warning","anthropic-ratelimit-unified-reset":String(r),"anthropic-ratelimit-unified-representative-claim":"seven_day_sonnet"};break}case"fast-mode-limit":{this.updateRepresentativeClaim(),this.headers["anthropic-ratelimit-unified-status"]="rejected",this.fastModeRateLimitDurationMs=600000;break}case"fast-mode-short-limit":{this.updateRepresentativeClaim(),this.headers["anthropic-ratelimit-unified-status"]="rejected",this.fastModeRateLimitDurationMs=1e4;break}case"extra-usage-required":{this.headerless429Message="Usage credits are required for long context requests.";break}default:break}}
```
Subtlety: limit scenarios call updateRepresentativeClaim **before** assigning rejected, so those branches do not necessarily write retry-after immediately; do not infer its presence from final status alone. Harness is excellent semantic/test-intent evidence, but disabled accessors mean it cannot prove wire reachability or server enum completeness.

## 5. Status enums and behavioral transitions

**Observed SDK enum precisely:** `rateLimitType = five_hour | seven_day | seven_day_opus | seven_day_sonnet | seven_day_overage_included | overage`. `unifiedWindows` has optional `five_hour`, `seven_day`, `seven_day_overage_included`, each requiring numeric `utilization` and integer epoch-seconds `resetsAt`, with optional three-state `status` and numeric `surpassedThreshold` (invalid optional values caught as undefined). Fractions may exceed 1; absent windows remain absent. **Observed:** base/overage/per-window normal status vocabulary is `allowed`, `allowed_warning`, `rejected`. `we` accepts raw strings and normalizes base `allowed_warning` to `allowed`; `Qt` validates per-window status and derived warning logic restores a warning if warranted. Thus SDK enum strictness and raw-header permissiveness differ. Base rejected + overage allowed/warning means paid continuation (`isUsingOverage`), not refusal. A rejected window alone does not imply the request was refused.

Evidence: 9,637,000–9,637,900; 4,382,492–4,384,900:
```js
status:r==="allowed_warning"?"allowed":r
// …
rateLimitType:B(["five_hour","seven_day","seven_day_opus","seven_day_sonnet","seven_day_overage_included","overage"]).optional(),utilization:v().optional(),unifiedWindows:u({five_hour:u({utilization:v(),resetsAt:v().int(),status:B(["allowed","allowed_warning","rejected"]).optional().catch(void 0),surpassedThreshold:v().optional().catch(void 0)}).optional(),seven_day:u({utilization:v(),resetsAt:v().int(),status:B(["allowed","allowed_warning","rejected"]).optional().catch(void 0),surpassedThreshold:v().optional().catch(void 0)}).optional(),seven_day_overage_included:u({utilization:v(),resetsAt:v().int(),status:B(["allowed","allowed_warning","rejected"]).optional().catch(void 0),surpassedThreshold:v().optional().catch(void 0)}).optional()}).optional()
// …
isUsingOverage:r==="rejected"&&(h==="allowed"||h==="allowed_warning")
// …
A rejected window alone does not mean requests are refused; the top-level status says whether they are. surpassedThreshold is the highest warning threshold passed, as a fraction of the limit (1 once rejected).
```

`overage-disabled-reason` SDK enum: `overage_not_provisioned`, `org_level_disabled`, `org_level_disabled_until`, `out_of_credits`, `seat_tier_level_disabled`, `member_level_disabled`, `seat_tier_zero_credit_limit`, `group_zero_credit_limit`, `member_zero_credit_limit`, `org_service_level_disabled`, `no_limits_configured`, `fetch_error`, `unknown`. Additionally **observed gateway/client-handled** `org_spend_cap_reached`; it is missing from this SDK enum, so enum alone is incomplete. No server trigger conditions beyond the literal labels are proven.

Evidence: 4,384,500–4,385,200; 9,624,800–9,625,000:
```js
 overageDisabledReason:B(["overage_not_provisioned","org_level_disabled","org_level_disabled_until","out_of_credits","seat_tier_level_disabled","member_level_disabled","seat_tier_zero_credit_limit","group_zero_credit_limit","member_zero_credit_limit","org_service_level_disabled","no_limits_configured","fetch_error","unknown"]).optional()
// …
var UVn=new Set(["org_level_disabled_until","org_spend_cap_reached"])
```

`overage-scope`: runtime recognizes `service|channel|group_pool|organization`; SDK `limitScope` omits organization. `overage-period`: gateway documents `daily|weekly|monthly`, writer emits `e.period`; do not impose that as a closed enum on all incoming unified headers. `grace-*` are numeric, **no grace-status enum observed**.

Evidence: 9,653,994; 32,142,200–32,142,600:
```js
function at(e){let n=e?.get?.($Ys);if(n==="service"||n==="channel"||n==="group_pool"||n==="organization")return n;return}
// …
(\`overage-period\` is \`daily\`, \`weekly\`, or
\`monthly\`)
```

Grace: only responses with unified status count as limiter observations. Clamp each grace utilization to 0–1. Both zero clears the zone. Choose five-hour when its positive reading has a future reset later than weekly (or weekly reset absent); otherwise choose weekly if positive. Covered if overage allowed/warning or `overage-in-use=true`. Coverage suppresses wrap-up obligation. No limiter header means leave prior state alone; late observations before a fence are ignored. Expire a named reset against the local clock.

Evidence: 9,611,243–9,613,600:
```js
function Te(e,n){let r=Number(e.get(n)??0);return Number.isFinite(r)?Math.max(0,Math.min(1,r)):0}function Ie(e,n,r){let s=Number(e.get(n)??NaN);return Number.isFinite(s)&&s*1000>r?Math.round(s):void 0}
// …
if(r===0&&s===0)return{limiterRan:!0,zone:null};
// …
m=r>0&&d!==void 0&&(f===void 0||d>f),h=s>0&&!m?"seven_day":"five_hour"
// …
covered:S!==void 0||e.get(Wfn)==="true"
// …
if(!r.limiterRan||n<this.fenceMs)return;
```
SDK prose explicitly says the server supplies grace utilization only after request header `anthropic-usage-limit: extended` under `tengu_lantern_spool`; main-loop/subagent served responses count, side calls and refused 429s do not. Grace may remain true at hard exhaustion; combine it with status, never read it alone as “usable.” Evidence: 4,385,100–4,386,400:
```js
A refused request's 429 is not read either, so while requests are refused the last value persists: still expire any UI derived from this field via resetsAt.
```

Slow-status transitions are client interpretations, not proven server allocation policy: active → lower-priority served; not_needed → standard served; slot_busy → retry; weekly_limit/budget_exhausted/off → stop; ineligible + overage-in-use → leave lower priority for paid usage. **No unified quota status-to-`/login` transition was located**. Authentication/profile failures must not be represented as quota reset events; exact `/login` recovery wiring remains outside the established evidence.

## 6. Overage, extra usage, trial, grace and spend

Keep **quota-window fractions**, **extra-usage cents**, **account credit balances**, and **session cost** separate. `extra_usage` schema is in §1. Overage is a continuation lane when subscription quota rejects; `overageInUse` is also tracked independently of inferred `isUsingOverage` (§3). SDK session totals include `total_cost_usd`, duration-ms, line counts and per-model usage; these are locally accumulated session accounting, not remaining subscription quota (4,660,700–4,661,500).

**Observed UI:** paid warning says close to usage-credit limit; rejected calls select a reset message, admin action, or credit warning. Generic warning messages below utilization 0.7 are suppressed; some team/enterprise members with enabled extra usage also suppress them. Evidence: 9,625,761–9,626,500:
```js
if(e.isUsingOverage){if(e.overageStatus==="allowed_warning")return{message:`You're close to your ${FO()?"usage limit":"usage credit limit"}`,severity:"warning"};return null}
// …
if(e.utilization!==void 0&&e.utilization<Oe)return null;
```
Disabled-reason rendering distinguishes out of credits, org monthly cap vs individual cap, channel monthly limit, pooled team budget, seat exclusion, disabled service, disabled member allocation, group $0 allocation. If both ordinary and overage reset exist in generic overage rejection, the message chooses the **earlier**; cap-specific branches may separately name the spend reset and subscription reset. Evidence: 9,626,500–9,629,900:
```js
if(f&&e.overageResetsAt)if(f<e.overageResetsAt)v=` \xB7 resets ${m}`;else v=` \xB7 resets ${h}`;
// …
if(e.overageDisabledReason==="org_service_level_disabled")return"This service is disabled for your org";
// …
if(e.overageDisabledReason==="group_zero_credit_limit")return`Your group's usage limit is set to $0${he()}`;
```
Upgrade actions: `/usage-credits` enables/raises caps or asks an admin; `/upgrade` is suggested for five-hour limits on pro/max without extra usage and with upgrade eligibility. Highest max tier prefers usage credits rather than upgrade. Explicit `upgrade-paths` controls action availability, including empty arrays suppressing both choices. Evidence: 9,630,361–9,631,100; 36,373,600–36,374,300:
```js
if(n==="max"&&oA()==="default_claude_max_20x")return!r&&cR()?"/usage-credits to keep using Claude Code":null;if(e==="five_hour"&&(n==="pro"||n==="max")&&!r&&Qde())return"/upgrade to keep using Claude Code";
// …
D=E!==void 0&&!E.includes("upgrade_plan"),W=E!==void 0&&!E.includes("overage")
```
Credits-required errors also expose `error.details.error_code=credits_required`, `disabled_reason`, `can_user_purchase_credits`, `has_chargeable_saved_payment_method`; no number is inferred from these flags. Evidence: 9,638,600–9,639,200:
```js
if(n?.error_code!=="credits_required")return{};return{errorCode:"credits_required",...typeof n.disabled_reason==="string"&&{overageDisabledReason:n.disabled_reason},...typeof n.can_user_purchase_credits==="boolean"&&{canUserPurchaseCredits:n.can_user_purchase_credits},...typeof n.has_chargeable_saved_payment_method==="boolean"&&{hasChargeableSavedPaymentMethod:n.has_chargeable_saved_payment_method}}
```
Trial/billing/grace fields on `Ob` are **not wire schema evidence**. `trial` and `billingAccessOverride` have setters, but production-facing adjacent accessors return null; `graceBand` occurs only in initialization/reset (two code hits), with no setter/consumer found. Their production shapes, eligibility and messages are **unknown**. It would be unsound to manufacture trial/grace windows from these names. Real grace is the numeric header model in §5. Evidence: 5,349,000–5,351,100:
```js
function bVr(){return null}function dH(){return null}function vqo(){return null;
// …
function Yei(){return null}function Xei(e){return}
```
The harness's `overages` uses explicit cents, currency and purchase/poll state; that is not the `extra_usage` HTTP response. Evidence: 5,349,000–5,349,700:
```js
setBillingAccess(e){this.enabled=!0,this.billingAccessOverride=e}setTrial(e){if(e===null){this.trial=null;return}this.enabled=!0,this.trial=e}
```
Grace/wrap-up messaging is explicit: do not begin long work or subagents; finish/checkpoint the current step. Feature texts also permit “next steps” and near-limit checkpoint modes. Evidence: 9,640,700–9,641,700:
```js
var NFs="[Usage limit reached \u2014 grace window active. Wrap up: finish or "+"checkpoint; don't start subagents or long work.]"
```
Promotions use percentage-off or extra-credit, positive headline and days-left, active and not redeemed (§2), not an extra quota bucket.

## 7. Fast mode

**Observed harness:** `fastModeRateLimitDurationMs` is set only by `fast-mode-limit` (600000 ms = 10m) and `fast-mode-short-limit` (10000 ms = 10s). `fastModeRateLimitExpiresAt` starts on first eligible check, not when the scenario is set; expiry resets the harness. It synthesizes bare retry-after as max(1, ceil(remaining milliseconds/1000)). These two fields have no demonstrated server response source.

Evidence: 5,348,100–5,350,200:
```js
this.fastModeRateLimitDurationMs=600000;
// …
this.fastModeRateLimitDurationMs=1e4;
// …
if(this.fastModeRateLimitExpiresAt===null)this.fastModeRateLimitExpiresAt=Date.now()+this.fastModeRateLimitDurationMs;let n=this.fastModeRateLimitExpiresAt-Date.now(),r={...this.headers};return r["retry-after"]=String(Math.max(1,Math.ceil(n/1000))),r
```
**Production differs:** fast requests receiving 429 or overload first honor overage-disabled-reason, disable fast mode for that attempt and surface the credit reason. Otherwise parse retry-after in seconds. Under 20s is treated as short retry; longer/absent cooldown uses max(retry-after or **30m default**, **10m minimum**), enters cooldown, and falls back to standard speed. Lazy expiry re-enables fast mode. Therefore **10m is not the production default** merely because the harness scenario uses it.

Evidence: 14,148,700–14,149,600; 14,161,897–14,162,350; 5,420,000–5,420,800:
```js
let Wi=x_r(an),wa=Wi!==null&&Wi<yet;
// …
let bi=Math.max(Wi??R_r,C_r),ms=Kz(an)?"overloaded":"rate_limit";if(wti(Date.now()+bi,ms),Yo())S.fastMode=!1;
// …
var R_r=1800000,yet=20000,C_r=600000;function x_r(e){let n=bet(e);if(n){let r=parseInt(n,10);if(!isNaN(r))return r*1000}return null}
// …
if(e.status==="cooldown"&&Date.now()>=e.resetAt)
```
Credits exhausted/org cap reached are notified once per turn; other disabled reasons can clear persisted fast opt-in and mark organization fast mode disabled. Evidence: 5,422,000–5,423,800:
```js
function l_n(e){return e==="org_level_disabled_until"||e==="org_spend_cap_reached"||e==="out_of_credits"}
// …
if(!Du.claimCreditsExhaustedNotice()){t("Fast mode credits rejection already surfaced this turn, suppressing repeat");return}
```

## 8. Policy limits versus quota

`GET /api/claude_code/policy_limits` is **organization feature policy**, not numeric spend or remaining quota. Its validator is present (contrary to an initial orientation gap): required `restrictions: Record<string,{allowed:boolean}>`; `compliance_taints` defaults [], normalizes/deduplicates regime strings and keeps at most 16; `monitoring_notice` nullable/default null with text max 500 and HTTPS URL max 2048; `defaults: Record<string,unknown>` defaults {}; optional `core_skills` max 256 strings of max 128 chars; nullish `artifact_create_notice` sanitized nonempty text with fallback notice. Policy failures include parse/network/timeout/auth/request failure and stale-cache/sign-in diagnostics, not quota reset calculations.

Evidence: 6,488,600–6,489,700; 10,834,500–10,835,600:
```js
restrictions:me(o(),u({allowed:M()}))
// …
monitoring_notice:u({text:o().max(500).transform((e)=>SC(e,"",{keepEmojiJoiners:!0})),url:o().max(2048).url().startsWith("https://").refine((e)=>!btt(e)).nullish().catch(null)}).nullable().default(null).catch(null),defaults:me(o(),oe()).default({}).catch({}),core_skills:T(o().max(128)).max(256).optional().catch([])
// …
function Tt(e){return`${e}${DRe}`}
```
The enforcement path checks `restrictions[key].allowed`, compliance regimes and already-hinted denials; restrictions can remain session-tainted after a restricted org was signed in. Policy gates include promo offers (§2) and feature availability, not a meter. Evidence: 6,515,000–6,516,600; 4,074,250–4,074,650:
```js
let l=e.restrictions[n];if(l)return l.allowed;
// …
v="disabled for the rest of this session because an organization with a restricted policy was signed in earlier"
```
`GET /api/claude_code/organizations/metrics_enabled`: response must contain `metrics_logging_enabled: boolean`. Consumer returns `{enabled:thatBoolean,hasError:false}`; request/shape failure yields `{enabled:false,hasError:true}`. This gates metrics logging/opt-out, **not quota reporting eligibility**. Evidence: 25,994,400–25,996,100:
```js
if(typeof n.data?.metrics_logging_enabled!=="boolean")throw Error("metrics_enabled response missing metrics_logging_enabled");return n.data
// …
{enabled:n.metrics_logging_enabled,hasError:!1}
// …
{enabled:!1,hasError:!0}
```

## 9. Reset semantics end to end

- **Inbound headers:** epoch seconds, not milliseconds and not durations. `we` rounds `Number`; grace `Ie` multiplies seconds by 1000 and requires a future value; retry reset calculation multiplies by 1000 and subtracts caller clock. `we` itself does not finite-check the top-level reset, so downstream validation matters.
- **Usage body:** reset strings represent ISO timestamps; gateway parsing rejects invalid date strings; SDK documents ISO (§1). Display helper parses with `new Date`, converts ms to seconds, and delegates to epoch formatter.
- **Harness setter:** numeric input to internal `reset`/`overage-reset` is **hours from now**, exactly `floor(Date.now()/1000)+hours*3600` (§4). This is not evidence that a server header contains hours. Claims synthesize 18000/604800s; the 3600s branch is unreachable.
- **Harness overage scenario:** next calendar month's first day, midnight **local time**, converted to epoch seconds; no relation to a proven server monthly timezone.
- **Bundled gateway spend caps:** next daily/monthly boundary in **UTC**, weekly next Monday UTC. Among blocked caps it selects the latest reset; otherwise fullest utilization. Gateway headers and usage `resets_at` refer to the same instant; do not generalize its calendar policy to subscription windows.
- **Tracked validity:** accept windows only if finite and future, less than one year ahead; grace drops at reset. No server-Date-based clock-skew compensation was found. A backwards local clock invalidates cached observations via age checks; late response fences prevent resurrection (§5).
- **Formatting:** epoch seconds → Date(ms); optional timezone uses Intl and is appended; otherwise local machine timezone. More than 24h away (or forced) renders month/day, adds year if different; nearer resets use en-US 12-hour clock, omit minutes on the hour. This changes presentation, not reset instant.

Evidence: 14,162,100–14,162,700; 5,348,500–5,349,200; 32,099,180–32,099,600; 9,652,100–9,652,550:
```js
function A_r(e,n){let r=e.headers?.get?.("anthropic-ratelimit-unified-reset");if(!r)return null;let s=Number(r);if(!Number.isFinite(s))return null;let h=Math.round(s*1000-n.now());return h>0?h:null}
// …
let s=new Date;s.setMonth(s.getMonth()+1,1),s.setHours(0,0,0,0),this.headers["anthropic-ratelimit-unified-overage-reset"]=String(Math.floor(s.getTime()/1000))
// …
function Oz(e,t=new Date){let n=t.getUTCFullYear(),o=t.getUTCMonth(),r=t.getUTCDate();if(e==="monthly")return new Date(Date.UTC(n,o+1,1));if(e==="daily")return new Date(Date.UTC(n,o,r+1));let s=8-(t.getUTCDay()||7);return new Date(Date.UTC(n,o,r+s))}
// …
function rt(e,n){let r=n/1000,s=r+31536000;return tTe(e)&&e.resets_at>r&&e.resets_at<s}
```
Formatting evidence: 3,315,547–3,316,650:
```js
let u=new Date(e*1000),o=new Date,{minutes:l,year:f}=Se(u,r),a=t||r!==void 0?` (${r??Sui()})`:"",i=(u.getTime()-o.getTime())/3600000;
// …
function AAt(e,t=!1,n=!0,s=!1){let r=new Date(e);return`${yC(Math.floor(r.getTime()/1000),t,n,s)}`}
```
Cache freshness is distinct from a quota reset: persisted usage TTL 3600000ms, minimum observation interval 60000ms; account mismatch clears cache; negative age rejected. Evidence: 9,639,200–9,640,700:
```js
var ni=3600000,it=60000
// …
if(s<0||s>ni)return null;
```
Countdown/continuation behavior also knows sleep/stale waits: a delayed timer past its grace allowance marks auto-resume stale rather than blindly sending; user sees “press enter to continue.” It stops auto-continuing if the usage reset is more than 24h away. Evidence: 18,923,200–18,924,200; 36,210,000–36,211,600:
```js
if(s>u&&e>=o.state.fireAtMs)o.sleptThroughReset=!0;
// …
"Automatic continue stopped \xB7 the usage limit now resets more than 24 hours out, so this task will not resume on its own (/rate-limit-options to wait anyway)"
```

## 10. Client-side derived limits

**Observed local warning projection:** elapsed fraction = clamp((nowSeconds - (resetSeconds-windowSeconds))/windowSeconds,0,1). Five-hour warning at utilization ≥0.9 while elapsed ≤0.72; weekly at ≥0.75/≤0.6, ≥0.5/≤0.35, ≥0.25/≤0.15. This is a heuristic “using too fast” warning, **not a forecast of remaining requests/tokens or precise exhaustion time**. Explicit server surpassed-threshold wins before heuristic checks. Lower-priority mode skips the five-hour warning. Display clamps ordinary percent to 100 but does not necessarily clamp overage percent.

Evidence: 9,632,659–9,636,600; 3,316,400–3,316,700:
```js
var Bt=[{rateLimitType:"five_hour",windowSeconds:18000,thresholds:[{utilization:0.9,timePct:0.72}]},{rateLimitType:"seven_day",windowSeconds:604800,thresholds:[{utilization:0.75,timePct:0.6},{utilization:0.5,timePct:0.35},{utilization:0.25,timePct:0.15}]}];
// …
function Vt(e,n){let r=Date.now()/1000,s=e-n,d=r-s;return Math.max(0,Math.min(1,d/n))}
// …
if(!f.some((A)=>h>=A.utilization&&S<=A.timePct))return null;
// …
function _st(e){return Math.round(e*1000)/10}function dSe(e){return Math.min(_st(e),100)}
```
**Slow budgets:** allowance is separate from normal five-hour quota but uses weekly quota. Server telemetry supplies utilized fraction; the client does not invent an allowance size. `slow-retry-after` seconds becomes clamped 5s–10m; `slow-max-wait` becomes 1m–6h; wait jitter ±30%. Budget exhaustion stores epoch reset from slow-budget-reset or 7d-reset, capped at 8 days. Stop reasons weekly/budget/off/ineligible/wall/max_wait are different from productive reset/user/account transitions. `slot_busy` retries only while the wait episode is valid; active/not_needed count lower/standard served requests. Low-priority reset expiry permits an extra 60s boundary allowance (not a new quota window).

Evidence: 6,852,500–6,853,700; 6,856,200–6,859,400:
```js
function Fd(e,n,r){if(n!==void 0&&Number.isFinite(n))e.retryAfterMs=xd(n*1000,Lg,Fg);if(r!==void 0&&Number.isFinite(r))e.maxWaitMs=xd(r*1000,Ug,Ng)}
// …
let K=1+(Math.random()*2-1)*Bg,Q=Math.round(y.retryAfterMs*K);
// …
var Vg=691200;function Xg(e,n,r){let s=Number(n?.get(Rd)??NaN)||Number(n?.get(Wg)??NaN);if(Number.isFinite(s)&&s>0)e.budgetSpentUntilSeconds=Math.round(Math.min(s,r/1000+Vg))}
// …
function Jg(e,n,r){switch(e){case"weekly_limit":return"weekly";case"budget_exhausted":return"budget";case"off":return"off";case"ineligible":return n?"ineligible":null;case"slot_busy":return null;case void 0:case"unrecognized":case"active":case"not_needed":return n&&r?"wall":null}}
```
**Overage-included allowlist:** feature key `tengu_usage_overage_included_models`; absent value falls back to a built-in display-name list (or empty in another lane); an array keeps string entries, non-array gives empty. Filter `limits` to `kind=weekly_scoped` with `scope.model.display_name`, case-insensitive membership; title is `Current week (<display name>)`, utilization copied from row percent. This is display selection, not a locally imposed provider limit. SDK `model_scoped` is additive: absent when unknown/hidden/cached; empty when fresh endpoint lists no model window before filtering.

Evidence: 9,618,300–9,620,200:
```js
function eTe(){let e=k(He,null);if(e===null||e===void 0)return Me();return Array.isArray(e)?e.filter((n)=>typeof n==="string"):[]}
// …
function Ge(e){return e!==null&&typeof e==="object"&&e.kind==="weekly_scoped"&&typeof e.scope?.model?.display_name==="string"}
// …
return e.filter(Ge).filter((s)=>r.includes(s.scope.model.display_name.toLowerCase())).map((s)=>({title:`Current week (${s.scope.model.display_name})`,displayName:s.scope.model.display_name,limit:{utilization:s.percent,resets_at:s.resets_at}}))
```
**Derived spend remaining exists:** ultrareview preflight, USD-only, computes cap minus used credits (null cap → infinity), intersects with actual balance unless auto reload enabled, clamps at zero, and returns unknown for unsupported currency/missing data. It compares cents against estimated dollars ×100; this is spend sufficiency, not remaining quota tokens. Evidence: 25,829,650–25,830,450:
```js
let{monthly_limit:u,used_credits:r}=o,s;if(u===null)s=1/0;else if(typeof u==="number"&&typeof r==="number")s=u-r;else return null;let l=Math.min(s,e.auto_reload_settings?.enabled===!0?1/0:e.amount);return Number.isFinite(l)?Math.max(0,l):null
// …
if(s===null||s>=e*100)return null;
```
Bundled gateway computes utilization = spent_cents/cap_cents (zero cap → 1), emits whole-percent usage versus fractional headers, chooses the fullest cap while usable and **latest-reset cap while blocked**, rounds header utilization to hundredths, caps nonblocked at 0.99; warning thresholds are >0.75 and >0.95. Those are gateway-local spend rules, not the five-hour/weekly projection above. Evidence: 32,120,500–32,122,200:
```js
function C0(e){let t=Math.round(e.utilization*100)/100;return e.exceeded?t:Math.min(t,0.99)}
// …
function ube(e,t){if(e.exceeded!==t.exceeded)return e.exceeded?e:t;if(e.exceeded)return e.resetsAt.getTime()>t.resetsAt.getTime()?e:t;return e.utilization>t.utilization?e:t}
```

## Confidence and gaps

**High confidence, static:** epoch-second header resets versus hour-based harness setters; latest-reset representative selection; separate wire and mock paths; permissive cache schema and stricter gateway schema; numeric grace/slow budgets; policy response shape; production fast cooldown units/defaults. These conclusions are literal-backed, not live provider verification.

**Unresolved:** exact live Anthropic `extra_usage.utilization` scale across account types (gateway lane explicitly percent); cinder/wattle product semantics; server-side effects of skip_spend and at_wall beyond consumed blocks; production trial/billing override meaning; exhaustive subscription reset scheduling/timezone; `/login` recovery relationship; additional future server fields/enum values. Settle with redacted authenticated response/header captures plus account/billing context, not mocks. No claim that the named absent keys can never appear server-side.

**Coverage boundary:** this is an exhaustive inventory of the requested unified-header family and located usage/reset/spend paths, not a proof that every generic API request-rate/token-rate limit or every billing/account API in the executable was reconstructed. Static presence does not establish reachability, rollout or account eligibility. No network, binary execution or credentials were needed.

**Verification performed:** `grep -o -E 'anthropic-ratelimit-unified-[a-z0-9-]+' /tmp/cc_code.txt | sort -u | wc -l` returned **31**. Re-ran `ctx.py 'exceededLimits.push({type:n,resetsAt:y})' --window 400 --max 1 --files code` and `ctx.py 'q=["cli_menu","cli_credits_required","ide_banner"' --window 240 --max 1 --files code`: claim durations and all nine surfaces matched verbatim. Python substring assertions verified **102 exact evidence segments** against the full corpus; report **495 lines**. `git diff --check -- <report-path>` returned success (report is new/untracked, so substring validation is the substantive check). Docs-only work: no code test gate required or claimed.
