---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.8-07-install-and-config"
  title: "agy 1.4.8 install, update and configuration"
  description: "Installer chain, shell-profile configuration, self-update, config and state layout, environment variable surface"
  status: "published"
  revision: 2
  created_at: "2026-10-27T09:59:34+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "pi"
  updated_by: "orchestrator"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/antigravity-cli/1.4.8/07-install-and-config.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 2
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/antigravity-cli/1.4.8/07-install-and-config.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---

# 07 — Install, update, and configuration

Everything in this section is either **observed** (public installer script, live
manifest fetch, on-disk state, klog output) or **derived** from recovered functions.
Marked where it matters.

## 1. Install chain

`curl -fsSL https://antigravity.google/cli/install.sh | bash` runs a POSIX shell
installer that does five things.

```
antigravity.google/cli/install.sh
        │
        ├─ 1. detect platform            -> linux_amd64 | darwin_arm64 | ...
        ├─ 2. GET manifests/<platform>.json
        │        base: https://antigravity-cli-auto-updater-974169037036.us-central1.run.app
        │        keys: version, url, sha512
        ├─ 3. GET <url>  (payload)       -> cli_linux_x64.tar.gz on storage.googleapis.com
        ├─ 4. verify sha512, extract member `antigravity`, install as `$TARGET_DIR/agy`
        └─ 5. exec `agy install`         -> Go-native shell-profile configuration
```

Observed constants from the installer script:

| item | value |
|---|---|
| download base | `https://antigravity-cli-auto-updater-974169037036.us-central1.run.app` |
| manifest path | `/manifests/<platform>.json` |
| manifest keys | `version`, `url`, `sha512` |
| default target dir | `$HOME/.local/bin` (override with `-d` / `--dir`) |
| archive member | `antigravity` (tar.gz) → copied to `<target>/agy` |
| checksum | sha512; `sha512sum` on Linux, `shasum -a 512` on macOS; mismatch aborts |
| macOS extra | `xattr -d com.apple.quarantine <binary>` |

Live verification of the manifest at the time of analysis:

```json
{
  "version": "1.4.8",
  "url": "https://storage.googleapis.com/antigravity-public/antigravity-cli/1.4.8-5447777392656384/linux-x64/cli_linux_x64.tar.gz",
  "sha512": "b23683771438e72bffe0a2231168f65a16a0d73ed2c7a8460b311dc5fe0c3c9d9dfc6cbfb7d8755173340071aa4cc5c7a552fb5d1be3fdd31c2b7559d67e404a"
}
```

The `1.4.8-5447777392656384` path segment is a build identifier, not a semver.

## 2. Shell profile configuration (`agy install`)

Implemented by `cli/installer/installer.ConfigureProfiles` (5,600 bytes) plus
`ResolvePaths` and `runPathDiagnostics`.

Targets, in the order the reference run touched them: `~/.bashrc`, `~/.zshrc`,
`~/.bash_profile`, `~/.profile`, `~/.config/fish/config.fish`.

- POSIX shells get `export PATH="<dir>:$PATH"`; fish gets `set -gx PATH "<dir>" $PATH`.
- Appends a marker comment `# Added by Antigravity CLI installer`.
- It also manages **aliases**: it can write `alias agy=` and `alias antigravity=`,
  and it *strips deprecated aliases* it previously wrote
  (`Stripped deprecated alias from shell profile %s: %s`). There is a CLI flag
  described as "Bypasses shell profile alias purging".
- Profile edits are atomic: temp file + rename, with `Warning: Failed to
  {stat,lstat,read,resolve symlink} shell profile %s` guards so an unreadable
  profile is skipped rather than clobbered.
- After writing, it re-checks PATH and logs
  `PATH verification: ~/.local/bin is correctly configured in active PATH environment.`

Note on the observed install output in the issue report: every `installer.go:*` log
line is prefixed `ERROR: logging before google.Init:`. That is klog complaining that
it was used before `flag`/`klog.InitFlags` ran. It is cosmetic — the installer still
succeeded. Anyone treating that output as a failure is misreading klog.

## 3. Self-update

`cli/updater` — `CheckForUpdate`, `checkManifest`, `TriggerUpdateAndRelaunch`,
`runUpdateShared` (10,304 bytes), `isNewerVersion`, `extractTarGzMember`,
`writeStatus`, `waitForUpdateStatus`, `relaunchProcess`.

| item | value | source |
|---|---|---|
| manifest base | same Cloud Run host as the installer | `GetManifestURLFromBase` |
| check throttle | "Last check was less than 15 minutes ago, skipping update (under lock)" | `checkManifest` |
| lock file | `~/.gemini/antigravity-cli/updater/update.lock` | observed on disk |
| status file | `~/.gemini/antigravity-cli/updater/update_status.json` | observed on disk |
| last check stamp | `~/.gemini/antigravity-cli/last_check.timestamp` | observed on disk |
| install method | in-place swap of the running binary via rename-with-retry, then relaunch | `renameWithRetry`, `relaunchProcess` |

The binary self-updates in the background during normal runs; the installer says so
explicitly. Practical consequence: a pinned version will drift on its own unless
self-update is disabled, so any reimplementation that depends on observed behaviour
must record the exact version it was validated against.

## 4. Configuration and state layout

Two roots under `~/.gemini`, plus a cache:

```
~/.gemini/antigravity-cli/          # runtime state + settings + token
~/.gemini/config/                   # SHARED with Gemini CLI: projects, migrations, extensions
~/.config/antigravity/              # XDG config root (plugin-secrets.json lives here)
~/.cache/antigravity/staging/       # download staging
```

`~/.gemini/config/` is not antigravity-private — it is the Gemini CLI config root, and
`agy` reads and writes into it:

| path | purpose | evidence |
|---|---|---|
| `config.json` | shared settings; `agy` falls back to its own settings when absent | `cli_setting_manager.go:79] config.json not found or unreadable, using CLI settings only` |
| `projects/<project-id>.json` | project records, e.g. `default-cli-project.json` | `project.go:78] project: created project "CLI Project" (id=default-cli-project) at …/projects/default-cli-project.json` |
| `.migrated` | one-shot marker for the AppDataDir → config-root migration | `migrate.go:50] Migration marker …/.migrated does not exist. Attempting migration from AppDataDir …` ; `migrate.go:158] Migration complete, marked with …` |
| `extensions/` | Gemini CLI extensions, imported | `gemini_extensions.go:28] Detecting Gemini extensions in ~/.gemini/extensions` |

Practical consequence: a project is a *file*, `id=default-cli-project` is the default, and
a conversation belongs to a project. See 06-mechanics for project switching.

### `~/.gemini/antigravity-cli/`

| path | purpose |
|---|---|
| `antigravity-oauth-token` | credentials, mode `0600`, JSON (see 02-auth) |
| `settings.json` | user settings |
| `jetski_state.pbtxt` | protobuf text: onboarding completion, `installation_uuid`, schema `migrations` |
| `installation_id` | installation identifier |
| `history.jsonl` | prompt history, one JSON object per line |
| `conversations/<uuid>.db` | SQLite, one per conversation |
| `conversation_summaries.db` | SQLite summary index |
| `brain/<uuid>/` | per-conversation scratch/memory |
| `implicit/<uuid>.pb` | protobuf "implicit" trajectory state |
| `jetbox_summaries_proto.pb` | serialised summaries |
| `presence/<uuid>.lock` | cross-process "conversation is open" locks |
| `builtin/` | shipped skills plus `.checksum` (tamper/replace detection) |
| `bin/webm_encoder` | downloaded helper for screen recording |
| `log/cli-<ts>.log`, `cli.log` | klog logs with internal `file:line` |
| `crashes/` | crash reports |
| `cache/` | `default_project_id.txt`, `onboarding.json`, `last_conversations.json`, `marketplaces/` |
| `updater/` | `update.lock`, `update_status.json` |

### `settings.json`

Observed baseline after a normal first run:

```json
{
  "agentMode": "accept-edits",
  "altScreenMode": "always",
  "editor": "/usr/bin/nano",
  "trustedWorkspaces": ["/mnt/Projects/furbyhaxx/pi-antigravity-auth"]
}
```

`trustedWorkspaces` is the workspace-trust allowlist — the analogue of an IDE's
"trust this folder". `agentMode` mirrors `--mode`.

### `jetski_state.pbtxt`

```textproto
post_onboarding: {
  completed_steps: POST_ONBOARDING_STEP_TYPE_MANAGER_WELCOME
  completed_steps: POST_ONBOARDING_STEP_TYPE_USAGE_MODE
  completed_steps: POST_ONBOARDING_STEP_TYPE_AGENT_CONFIGURATION
  completed_steps: POST_ONBOARDING_STEP_TYPE_ADD_WORKSPACE
}
installation_uuid: "a398c588-…"
migrations: { key: 3  value: MIGRATION_STATUS_COMPLETED }
…
```

`installation_uuid` in state is distinct from the separate `installation_id` file.

### Plugin secrets

`~/.config/antigravity/plugin-secrets.json`, reported by
`cli/store/auth_store.go:199` as `Using file-based secret storage at <path>`.
This is the fallback when the OS keyring is unavailable — the same keyring-first,
file-fallback policy as the main token (see 02-auth).

### Token storage is conditional on the session, not just on the keyring

One detail worth calling out early because it changes what a reimplementation must
handle: the file-backed token store is selected when an **SSH session is detected**, not
only when the keyring times out.

```
composite_token_storage.go:123] Using file-based token storage because SSH session detected
```

Session detection comes from `terminal_info_manager.go:354/503`, which records `TERM`,
altScreen, `ssh`, `osc8`, `osc52`, `appleTerminal` and the raw response to an SSH DA2
probe. On this machine both paths were exercised: `auth.go:157] ChainedAuth:
authenticated via keyring (effective: keyring)` on one run and the SSH file branch on
another. Full detail in 02-auth.

## 5. Environment variable surface

Recovered `AGY_*` / `ANTIGRAVITY_*` / `JETSKI_*` variables that affect behaviour:

| variable | role |
|---|---|
| `AGY_CLI_THEME` | theme override |
| `AGY_CLI_HIDE_LOGO`, `AGY_CLI_HIDE_ACCOUNT_INFO` | suppress startup chrome |
| `AGY_CLI_CMD_OUTPUT_PERCENTAGE` | tool-output display budget |
| `AGY_CLI_DISABLE_LATEX`, `AGY_CLI_DISABLE_ESCAPE_SEQUENCE_OPTIMIZATIONS` | rendering kill-switches |
| `AGY_CLI_AUTO_COMPACT_OVERRIDE` | context compaction threshold |
| `AGY_CLI_CDE_AUTH_ACTION` | CDE sign-in action selector |
| `AGY_LLM_GATEWAY_URL` | override the LLM gateway |
| `AGY_ACCOUNT`, `AGY_ACCOUNT_TIER`-family, `AGY_BUSINESS_PAYGO_TIER` | account/tier overrides |
| `AGY_ADC_AUTH` | enable Application Default Credentials path |
| `AGY_CDE_ENTERPRISE_AUTH` | enterprise CDE auth |
| `ANTIGRAVITY_CONFIG_REL_DIR` | config root override |
| `ANTIGRAVITY_PROJECT_ID` | pin the GCP project |
| `ANTIGRAVITY_LS_ADDRESS` | address of the language server |
| `ANTIGRAVITY_CREDENTIAL_SCOPE` | credential scoping |
| `ANTIGRAVITY_PERM_GRANTS` | permission grants injection |
| `ANTIGRAVITY_SANDBOX*`, `ANTIGRAVITY_CSRF_TOKEN`, `ANTIGRAVITY_SIDECAR_UI_TOKEN`, `ANTIGRAVITY_SIDECAR_WEB_PORT` | sandbox / local server auth |
| `ANTIGRAVITY_SENTRY_SAMPLE_RATE` | telemetry sampling |
| `ANTIGRAVITY_EXECUTABLE_DATA_DIR` | data-dir override |
| `JETSKI_HUD`, `JETSKI_INTERNAL`, `JETSKI_LIVE_MODE`, `JETSKI_TRAJECTORY`, `JETSKI_VERIFIERS` … | internal feature gates |

Standard Google/Go variables also apply: `GOOGLE_APPLICATION_CREDENTIALS`,
`GOOGLE_API_KEY`, `GOOGLE_CLOUD_PROJECT`, `GOOGLE_GEMINI_BASE_URL`,
`GOOGLE_VERTEX_BASE_URL`, `GOOGLE_GENAI_USE_VERTEXAI`, `GOOGLE_GENAI_USE_ENTERPRISE`,
`GOOGLE_LOG_DIR`, `GOOGLE_MAX_LOG_MB`, `GOOGLE_STDERRTHRESHOLD`,
`XDG_CACHE_HOME`, `EDITOR`.

Note `AGY_LLM_GATEWAY_URL` plus `gatewayAuth` plus a `gateway` package
(`gateway/auth_provider.go`, `ca_cert.go`, `model_discovery.go`): there is a
first-class LLM-gateway deployment mode with custom CA handling. That is an
enterprise/proxy path, not the consumer path.

## 6. `install` subcommand vs `update` subcommand

- `agy install [--dir <path>]` — re-runs shell-profile configuration only. It does
  not download anything. This is what the curl installer calls at the end.
- `agy update` — update machinery backed by `cli/updater`, exposed manually. Normal
  runs also self-update in the background.

## Open questions

- Whether `agy install` accepts a flag to disable self-update was not confirmed; the
  help output shows only `--dir`-equivalent behaviour.
- Exact set of platforms in `manifests/` beyond `linux_amd64` was not enumerated
  (only the platform the installer reported was fetched).
