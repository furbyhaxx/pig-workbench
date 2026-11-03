---
document:
  type: "research"
  id: "RES-deepseek-harness-0.2.2-alpha.3-00-context"
  title: "DeepSeek Harness 0.2.2-alpha.3: source context"
  description: "Identity, pinned source, method, scope and reproduction"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:52:59+01:00"
  updated_at: "2026-11-03T11:52:59+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["Record target identity and source-analysis limits"]
---

# Context

[OBS] DeepSeek Harness is the TypeScript pnpm workspace identified by `@deepseek-ai/dsh-root`, version `0.2.2-alpha.3`, with pnpm `11.28.5` and Node `>=24.9.0` (`host:/tmp/re-recon/dsh/package.json:1-15`). Its checked-out commit is `8bd073392eef6f54e921a093b95076248eb08cbb`, dated 2026-11-03; origin is `https://github.com/deepseek-ai/deepseek-harness.git`. The actual Git commands and output are recorded in [evidence/source-check.txt](evidence/source-check.txt).

[OBS] The source tree uses package groups under `packages/*/*`, with additional workspace entries for `vendor`, `native/system`, `apps` and `website` (`host:/tmp/re-recon/dsh/package.json:10-17`).

## Method and scope

[OBS] This is a static source analysis of the pinned local clone. Source paths in the following chapters are relative to `host:/tmp/re-recon/dsh/` unless explicitly identified otherwise. `[OBS]` means directly supported by code, configuration, tests or recorded command output; `[INF]` means a reasoned implication; `[UNK]` identifies an unresolved fact. Source tests are evidence of intended behavior, not a claim that the tests were executed.

The dossier examines prompt assembly, personas and instruction sources, the tool declaration and execution seams, and the architecture needed to follow them. [UNK] Actual runtime prompts, enabled plugins and provider wire messages depend on deployment, scope, model and configuration; no `deepseek-harness` executable, application, network model request, or test suite was run. Source examples must not be read as runtime captures.

To reproduce the source checkout:

```sh
git clone --depth 1 https://github.com/deepseek-ai/deepseek-harness.git
cd deepseek-harness
git rev-parse HEAD
```

[INF] The shallow clone follows the remote's then-current default branch and may no longer contain this pinned commit. Verify its HEAD; obtaining the exact revision later may require fetching that commit separately.
