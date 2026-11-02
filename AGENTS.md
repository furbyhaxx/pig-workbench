# pig-workbench

Go factory extension for PiG: enhanced builtin tools, working locations, harness frames, questionnaires and shell jobs. Exclude subagents, agent definitions and task/todo tracking.

- `.project/` — development artifacts; never in `docs/`; committed. Load skill `dot-project` before writing one. Every Markdown artifact needs `document:` frontmatter (`.project/AGENTS.md`). Open work: `BACKLOG.md` pointers + `.project/backlog/`; live claims: project-scope `tasks`. Never put drift/TODOs in AGENTS.md.
- Load skill `extending-pig` for extension work. Verify SDK signatures against `pig reload --sdk-path`; do not copy TypeScript runtime assumptions.
- Treat adjacent reference repositories as read-only. Preserve their unrelated changes.
- Proposals in `.project/drafts/` are not approved implementation contracts.
