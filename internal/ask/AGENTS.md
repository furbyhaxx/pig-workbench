# ask

Questionnaire schema, validation, and branch-local recovery for SPEC-0009 §2–§3 and SPEC-0013 §7.
- `form.go` defines the JSON argument object and validates 1–20 questions, unique IDs, 50 options per question, secret metadata and choice requirements. Preview text permits newline and tab; other user text rejects controls.
- `record.go` stores the complete form in `Record.Payload` (`payload` on the wire). Records require creation and update times. Pass `LoadPending` only the current branch's entries, ordered oldest to newest; terminal records resolve only their own session/ask pair.
- `run.go` executes one form through the `Interactor` seam; interactors must return when their context ends and report unavailability with `InteractionUnavailableError`. `Deps.Append` is a host-bound closure.
- `Deps.Options.DialogTimeout` is per question; zero disables it. `MaxQuestions` can lower the form's 20-question hard limit; non-positive uses the default.
- `Result` marshals unavailable interactions as `interactionUnavailable: true` with `reason`; submitted answers are emitted deterministically, but JSON object order is not a consumer contract. `Order` is for Go callers only.
- Collect and validate all answers before storing secrets; the store must copy any bytes it retains. Accept only whole `red-sec-[a-z2-7]{16}` placeholders. Never put secret input in forms, records, errors or session entries; do not scan unrelated text for incidental substring matches.
- A successful completed append is the commit point. After an acknowledged secret write, cancellation or failure returns `FinalizationError` with validated stored placeholders and publication status; invalid store output has unknown persistence outcome.
- TUI panel, RPC dialogs, secret-store adapter and tool registration are not wired here; do not advertise `ask_user` until those integrations exist.
