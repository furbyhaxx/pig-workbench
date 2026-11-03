# frames

Attributed, SDK-free frame admission and replay for SPEC-0002. The host owns persistence and producer policy; this package performs no I/O.
- Use one `Session` for all producers in a session; `NewLedger` creates an isolated session only. Each ledger has an immutable producer prefix, branch leaf and epoch. A source must equal its prefix or be a dotted child.
- Session admission keeps event-key receipts and all records, including superseded ones, for replay. Deduplication is per producer; reusing an event key with a different branch or epoch conflicts. Limit session pending frames to 256 and all retained body bytes to 2 MiB; reject overflow without dropping records.
- Coalesce only pending info/notice records with matching producer, branch, epoch, source, subject and explicit key. Accepted but superseded records remain deduplicable.
- Session `Snapshot` records have a schema version and preserve provenance, payload, state and sequence. Restore complete session records atomically, without discarding or rewriting active records. The caller persists them under `pig-workbench.frame`.
- External publications retain their requested data trust for replay but receive conservative untrusted/data authority until host policy exists. A nonce is a provenance cue, not authentication.
- Delivery transitions, expiry processing, branch selection and envelope encoding/decoding are not implemented here; do not claim admission means model delivery.
