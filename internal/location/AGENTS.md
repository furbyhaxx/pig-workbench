# location

Local location engine for SPEC-0008 §2–§3. Backend expansion must keep captured revisions and transactional persistence.
- No process/host chdir. Validate directories before committing; symlink aliases use canonical paths.
- Reads may finish against old revisions. Mutations hold leases; transitions block new mutations and report conflicting operation IDs on timeout.
- Persist before activation. Notification failure must not undo a committed location.
