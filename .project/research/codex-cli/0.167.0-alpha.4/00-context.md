---
document:
  type: "research"
  id: "RES-codex-cli-0.167.0-alpha.4-00-context"
  title: "Codex CLI 0.167.0-alpha.4 source analysis context"
  description: "Pinned identity, source-only method, scope, and checkout reproduction"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:53:02+01:00"
  updated_at: "2026-11-03T11:53:02+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["recorded target identity and source-analysis scope"]
---

# Context

[OBS] The target is OpenAI's Codex CLI Rust workspace at tag
`rust-v0.167.0-alpha.4`, commit `fb036d4ebd89f263d34a1f888c279ce85bb383c6`.
`git log -1` records the commit as 2026-11-03 02:19:36 Europe/Zurich
(2026-11-02 17:19:36 at UTC-08:00); see
[evidence/commands.txt](evidence/commands.txt). The checked-out source is
`host:/tmp/re-recon/codex-tag/codex-rs`. [OBS] The workspace package version is
**`0.167.0-alpha.4`**, not `0.0.0` (`Cargo.toml:172-173`). The tag and commit
pin this snapshot independently of a workspace version.

[OBS] This is **source analysis**, limited to the `codex-rs/` sparse checkout.
The `codex` executable was not found on this host's PATH; no Codex binary or
session was executed. Source-derived descriptions are not runtime observations.
`[OBS]` denotes code or text read from this snapshot, `[INF]` an inference,
and `[UNK]` an unresolved fact. Source citations are relative to `codex-rs/`.

Scope: prompt selection and instruction hierarchy, turn and request flow, and
model-facing tool declaration and dispatch. Other repository trees, binary
reverse engineering, live provider behavior and undocumented server-side
instructions are outside scope. The tool surface can vary with configuration,
model, provider and runtime-supplied tools; no single observed live tool list is
claimed.

Reproduce the source checkout:

```sh
git clone --depth 1 --branch rust-v0.167.0-alpha.4 --filter=blob:none --sparse https://github.com/openai/codex.git
cd codex
git sparse-checkout set codex-rs
git rev-parse HEAD
```

The final command should print `fb036d4ebd89f263d34a1f888c279ce85bb383c6`.
