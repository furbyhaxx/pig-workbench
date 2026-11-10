Development artifacts; committed, never shipped or placed in `docs/`. Load skill `dot-project` before creating, revising or deleting artifacts; it is the standard, this file lists what this repository uses.

- `plans/` — numbered implementation plans, dependencies and acceptance gates; rules in `plans/AGENTS.md`.
- `specs/` — numbered API and architecture contracts; rules in `specs/AGENTS.md`.
- `backlog/` — open work items `B-NNNN-<slug>.md`, each with one pointer line in a `BACKLOG.md` in the code tree; created with the first item.
- `reviews/` — dated ranked findings on a range, slice or diff with gate evidence; rules in `reviews/AGENTS.md`.
- `analyses/` — dated probe runs, measurements and backend evaluations with evidence; rules in `analyses/AGENTS.md`.
- `research/` — imported third-party dossiers and prior art, version-keyed and snapshot-only; rules in `research/AGENTS.md`.
- `drafts/` — undecided proposals and RFCs; rules in `drafts/AGENTS.md`.
- `sketches/` — UI/UX designs, wireframes and capture evidence; rules in `sketches/AGENTS.md`.
- `handoff/` — resume briefs and cross-repository requests; rules in `handoff/AGENTS.md`.
- Only what currently applies or is planned stays. Update in place; a consumed, superseded or decided artifact is deleted in the commit that makes it obsolete, with `Artifact-Deleted`, optional `Superseded-By` and `Reason` trailers. There is no archive. Do not read deleted artifacts from git history unless asked.
- Fix every reference to a deleted artifact's path and id in the same commit.
- Every Markdown artifact needs `document:` frontmatter: type, id, title, description, status, revision, created_at, updated_at, created_by, updated_by, changelog. Quote YAML strings; timestamps use Europe/Zurich offsets.
- Revise: bump revision, set updated_*, and replace the single changelog entry with this revision's reason; earlier entries are in git history.
- Numbered ids come from `ids.toml`; take the number and increment the counter in the same commit. Never lower a counter or reuse an id.
- Sources outside this repository use `host:/absolute/path`. Evidence belongs beside its artifact in `evidence/` and is deleted with it; redact secrets.
- Add another artifact type only when none fits, with its own AGENTS.md and a row here.
