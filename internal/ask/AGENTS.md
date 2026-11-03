# ask

Questionnaire schema, validation, and branch-local recovery for SPEC-0009 §2–§3 and SPEC-0013 §7.
- `form.go` defines the JSON argument object and validates 1–20 questions, unique IDs, 50 options per question, secret metadata and choice requirements. Preview text permits newline and tab; other user text rejects controls.
- `record.go` stores the complete form in `Record.Payload` (`payload` on the wire). Records require creation and update times. Pass `LoadPending` only the current branch's entries, ordered oldest to newest; terminal records resolve only their own session/ask pair.
- Secret questions contain metadata only. Never put secret values in forms, records, errors or session entries.
- Registration, UI, session I/O and the secret vault are not wired here; do not advertise `ask_user` until those integrations exist.
