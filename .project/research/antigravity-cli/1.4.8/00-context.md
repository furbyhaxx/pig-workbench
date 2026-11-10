---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.8-00-context"
  title: "agy 1.4.8 shared analysis context"
  description: "Target identity, symbol-recovery toolkit, established facts and working rules shared by all sections"
  status: "published"
  revision: 2
  created_at: "2026-10-27T09:59:34+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "pi"
  updated_by: "orchestrator"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/antigravity-cli/1.4.8/00-context.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 2
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/antigravity-cli/1.4.8/00-context.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---

# agy 1.4.8 — shared analysis context

Read this first. It records what is already established so you do not re-derive it,
and how to recover more.

## Target

| field | value |
|---|---|
| path | `/home/furbyhaxx/.local/bin/agy` |
| version | 1.4.8 (`agy --version`) |
| size | 228,897,168 bytes |
| sha256 | `70b1d27f1812f5ebccc8f13daf2209112447413437dd1ab6d87c58b8a04fe35b` |
| format | ELF64 x86-64 PIE, dynamically linked, **stripped** (no `.symtab`) |
| toolchain | `go1.28-20261015-RC02 cl/1000353293 +18a2e1f59d` — Google-internal Go, `boringcrypto`, BOLT-optimized (`.text.hot`, `.text.split`, `.lrodata`) |
| internal codename | **jetski** (`third_party/jetski/...`, `jetski.corp.google.com`) |
| user-state root | `~/.gemini/antigravity-cli/` |
| config root | `~/.config/antigravity/` |

Sections of note: `.lrodata` @ `0x15e8e30`, `.rodata` @ `0x58adf80`,
`.text` @ `0x6596000` (vaddr == file offset throughout, so offsets are usable directly).

## Symbol recovery

There is no symbol table, but pclntab survives at file offset `0x15e8e30`:
171,928 functions, 6,825 distinct source `*.go` names. Function addresses are
`0x6596000 + entryoff`. BOLT reorders and splits bodies, so an entry point may be a
hot fragment and `objdump` alignment at the entry is **not** reliable — never trust a
decoded instruction stream at a function entry without checking it disassembles
sensibly. Byte-scanning is the reliable path.

Tool: `tools/goresym.py` (already built its cache next to itself).

```bash
T=.project/research/antigravity-cli/1.4.8/tools/goresym.py
A=/home/furbyhaxx/.local/bin/agy

python3 $T --bin $A funcs 'jetski/cli/store'      # regex over function names
python3 $T --bin $A addr '<exact func name>'      # address + span
python3 $T --bin $A disas '<exact func name>'     # objdump of the span
python3 $T --bin $A strs '<exact func name>'      # string literals referenced by it
python3 $T --bin $A grep '<regex>'                # rodata scan, resolved printable runs
```

`strs` is the highest-yield command: it finds RIP-relative LEA targets in the function
span and resolves Go `(ptr,len)` string literals. A trailing `?` on the address means
the length could not be resolved, so the text is a truncated printable run that may
contain several adjacent strings concatenated — treat those as context, not as an
exact literal.

`data/` already holds: `funcs.txt` (all names), `funcmap.json`
(`[addr, span, name]`), `pkgs.txt` (functions per package), `rpc_methods.txt`
(863 RPCs), `source_files.txt` (recovered `*.go` names).

## Established facts — do not re-derive, cite and build on

### Runtime state layout (`~/.gemini/antigravity-cli/`)
`antigravity-oauth-token` (0600, JSON: `token.{access_token,token_type,refresh_token,expiry,expires_in}`, `auth_method`, `id_token_email`),
`settings.json`, `jetski_state.pbtxt`, `installation_id`, `history.jsonl`,
`conversations/*.db` (SQLite per conversation), `conversation_summaries.db`,
`brain/<uuid>/`, `implicit/*.pb`, `presence/*.lock`, `builtin/` (skills + `.checksum`),
`log/cli-<timestamp>.log` + `cli.log` (klog format with internal `file:line`),
`crashes/`, `cache/`, `updater/`, `bin/webm_encoder`, `last_check.timestamp`.
Plugin secrets: `~/.config/antigravity/plugin-secrets.json`.

### Backend hosts seen in logs (real, observed)
- `https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist`
- `https://daily-cloudcode-pa.googleapis.com/v1internal:fetchAvailableModels`

`daily-` marks the Cloud Code "daily" channel. Other hosts present in the binary:
`cloudcode-pa.googleapis.com`, `businessaicode.googleapis.com`, `aicode.googleapis.com`,
`aiplatform.googleapis.com` + `*-aiplatform.googleapis.com`, `generativelanguage.googleapis.com`,
`oauth2.googleapis.com`, `iamcredentials.googleapis.com`, `sts.googleapis.com`,
`cloudkms.googleapis.com`, `secretmanager.googleapis.com`, `safebrowsing.googleapis.com`,
`modelarmor.googleapis.com`, `antigravity-unleash.goog` (Unleash feature flags),
`antigravity.google`.

### OAuth (recovered from `auth.getOauthParams`)
Three installed-app client IDs are embedded:
```
884354919052-36trc1jjb3tguiac32ov6cod268c5blh.apps.googleusercontent.com
1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com
1098991175725-mdu1oe5cbbe1ckggsebobh94toiap657.apps.googleusercontent.com
```
and two embedded client secrets (`GOCSPX-…`). Confirm which client belongs to which
auth method rather than assuming.

Redirect: `https://antigravity.google/oauth-callback`; success pages
`https://antigravity.google/auth-success?app=%s`; also
`https://antigravity.google/oauth/client-metadata.json` (client metadata document).

Scopes present: `aicode` (confirmed, `getScopesByAuthMethod`), `openid`,
`cloud-platform`, `userinfo.email`, `userinfo.profile`, `experimentsandconfigs`,
`cclog`, `drive`, `drive.file`, `drive.appdata`, `drive.metadata`, `drive.metadata.readonly`,
`drive.apps.readonly`, `drive.photos.readonly`, `drive.readonly`, `drive.scripts`.

### Auth architecture
`cli/backend/auth/`, all methods behind `ChainedAuth`:
`oauthMethod`, `keyringAuth`, `geminiAPIKeyAuth`, `adcAuth`, `gatewayAuth`,
`externalCorpLoginAuth`. Token storages: `cliFileTokenStorage`, `adcTokenStorage`,
plus a `composite` storage. Key funcs: `RunLogin`, `RunAuthActionWithOptions`,
`refreshAndSaveStoredToken`, `fetchUserInfo`, `fetchAdcUserInfo`, `generateState`,
`getOAuthConfig`, `isEnterpriseAuth`, `defaultTokenStorage`, `NewCLITokenStorage`.
Source files in this package: `auth.go`, `adc_auth.go`, `token_storage.go`,
`keyring.go`, `gemini_api_key.go`, `gateway.go`, `corplogin_external.go`, `cde_login.go`,
`wif_browser.go`, `browser.go`, `validate.go`.

### Quota / entitlement plumbing
`cli/store/quota_manager.go` (`doRefreshQuota`, forced vs. throttled reload),
`language_server/rpcs_seat_management.go`, `language_server/state_sync/model_credits.go`,
`cli/model/credits.go`, `cli/model/usage.go`, `cli/store/one_shot_model.go`.
Logs show `quota_manager` refreshing `loadCodeAssistResponse`, a `userInfo` cache
derived from it, `retrieveUserQuotaSummary` (singleflight), and the string
`You have exhausted your quota on this model.`
`exa.seat_management_pb.SeatManagementService` exposes 96 RPCs including
`GetUserStatus`, `GetPlanStatus`, `MigrateApiKey`, `UpdateProfile`, `AddTeamDomain`.

### Models (`agy models`, live)
| id | display |
|---|---|
| gemini-4-argon-low / -medium / -high | Gemini 4 Argon |
| gemini-3.8-flash-low / -medium / -high | Gemini 3.8 Flash |
| claude-opus-5-5-low / -medium / -high | Claude Opus 5.5 |
| claude-sonnet-5-5-low / -medium / -high | Claude Sonnet 5.5 |
| gpt-oss-120b-medium | GPT-OSS 120B |

Packages: `models/model_config`, `models/defaults`, `models/geminimodels`,
`models/model_config/{catalog,model_groups,context_window}.go`.

### CLI surface (`agy --help`)
Flags: `--add-dir --agent -c/--continue --conversation --dangerously-skip-permissions
--disable-slash-commands --effort --input-format --json-schema --log-file --mode
--model --new-project --output-format -p/--print --print-timeout --project --prompt
--prompt-interactive --remote-control --sandbox`.
Subcommands: `agent(s) changelog help install mcp mic-serve models plugin(s)
remote-control update`.

## Working rules

- Distinguish **observed** (bytes, logs, live call output), **derived** (inferred from
  recovered structure) and **unknown**. Never present inference as observation.
- Cite evidence as `function name` / `file.go:line` from logs / exact address.
- Ground every conclusion in something you actually ran. Do not describe a subsystem
  from its name alone.
- Read-only. Do not modify `~/.gemini/antigravity-cli/` or `~/.config/antigravity/`,
  do not overwrite the token, and do not run commands that could trip abuse detection
  (no repeated auth attempts, no bulk quota polling). A single ordinary CLI invocation
  is acceptable; do not loop.
- Write your section to the file named in your brief under
  `.project/research/antigravity-cli/`. Do not edit other sections.
