# AGENTS.md — deepseek-harness dossier

Source analysis of `deepseek-ai/deepseek-harness`, a TypeScript pnpm monorepo. Not installed; nothing was executed.

- Version `0.2.2-alpha.3`, commit `8bd073392eef6f54e921a093b95076248eb08cbb`. `0.2.2-alpha.3/` is the only version dir.
- Reproduce: `git clone --depth 1 https://github.com/deepseek-ai/deepseek-harness.git`.
- Start at `packages/core/system-prompt/src/index.ts` (paths in this file are relative to the cloned repo, not to this dossier): it builds named sections with an explicit order. Its tests are direct behavioural evidence — read `tests/system-prompt.spec.ts`, `tests/tool-order.spec.ts` (intended tool ordering) and `tests/scoped.spec.ts` (scoped prompts) before the chapters.
- Two tool executors, native and PTC, project declarations differently. Which projection a tool uses shapes the schema the model sees: don't describe a tool without saying which.
- Evidence cites `host:/tmp/re-recon/dsh`, a transient path. Its commit SHA is the real identity — re-clone and check the SHA before resolving a citation.

## Verify

```bash
python3 0.2.2-alpha.3/evidence/verify.py
```

Checks frontmatter, the injection inventory (58 rows), citations and templates.

## Traps

- No fully rendered shipped-profile prompt was captured. Evidence holds verbatim fragments with `path:line` plus SDK scenario captures — never present a fragment set as one complete prompt.
- Locale files exist per language. Establish whether a string is inlined in code or looked up per locale before calling it canonical.
- A `[UNK]` in a chapter is deliberate and names what would settle it. Don't paper over it with a plausible guess.

## Cross-updates

- Re-pin the commit → rename the version dir, update `README.md`, re-run the verifier.
- New package under `packages/context/` → that is a new injection; extend `04-instructions-and-injections.md` and the inventory.

## Freshness

Alpha versions move fast. Re-pin before quoting prompt text, a package layout or a tool schema.
