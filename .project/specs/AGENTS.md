Numbered specifications define public contracts and accepted design direction. Draft status means review is pending, not implemented.

- `0001-extension-services.md` — shared protocol; all other public APIs depend on it.
- `0002-message-frames.md` — frame producers and context delivery.
- `0003-tool-presentation.md` — renderer contributions and settings.
- `0004-resource-providers.md` — scheme providers and tool routing.
- `0005-hashline-editing.md` — edit grammar, source snapshots, seen-state and commit/recovery guarantees.
- `0006-advanced-read.md` — selector semantics, adapters, search and output coverage.
- `0007-ambient-context.md` — file classes, applicability, pins, capped views and injection lifecycle.
- `0008-locations-jobs.md` — location leases, SSH, worktrees, process ownership and job controls.
- `0009-interaction-commands.md` — questionnaires, extraction, checkpoints and command routes.
- `0010-configuration-state-trust.md` — cross-cutting configuration, persistence, privacy and authorization.
- `0011-inter-session-messaging.md` — live-session endpoints, `agent://` messaging and the coordinator.
- `0012-projects-uri-namespace.md` — project identity and storage, `.project/` conventions, every scheme's tree, `docs://` and guides.
- Workflow UI: `../sketches/workflow-details/README.md`; shared/API UI: `../sketches/workbench-ui/README.md` and `../sketches/extension-apis/README.md`.
- Change source coordinates or selector semantics → review 0004/0005/0006 together; change applicability/epochs → review 0002/0005/0007/0010 together.
- Change common protocol → review all consumers, wire examples and acceptance matrices in the same change.
- Distinguish verified SDK mechanisms from proposed Workbench methods. Link host blockers explicitly; never invent SDK support.
