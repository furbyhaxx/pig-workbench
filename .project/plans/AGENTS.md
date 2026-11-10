Implementation plans. Use numbered paths `NNNN-<slug>.md` or `NNNN-<slug>/plan.md`; artifact type `plan`.

- Record scope, dependencies, deliverables, acceptance evidence and blocking decisions.
- Detail current and next slices; keep later slices at milestone level until their prerequisites are verified.
- Track done/now/next/later in the plan. Never label intended behavior as implemented.
- Scope changes require an explicit decision and a new revision; sequencing must not quietly remove requirements.
- A plan is executable: an agent that reads only that plan can finish the work. Overlapping plans are merged, not cross-referenced.
- Work merged: delete the plan and its evidence, with `Artifact-Deleted` trailers.
