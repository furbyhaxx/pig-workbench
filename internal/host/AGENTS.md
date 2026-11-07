# host

SDK adapter and feature composition. `Register` is called once by the root factory.
- Keep SDK contexts at this boundary. Domain requests use a request cancellation adapter, never ctx.Signal as a substitute for per-request cancellation.
- Session state owns a lifecycle supervisor. Shutdown must not admit itself before draining.
- Restore custom entries from the active parent chain; GetBranch's projection omits custom data.
- Resolve config roots from SDK host helpers and the original ctx.Cwd; `cd` changes neither. Trust callbacks belong to one request and are never retained by config.
- Expose only wired capabilities; diagnostics describe actual state. Commands and companion aliases share handlers.
- `contributions.go` dispatches every kind through `contributionConsumers`: validate in `validateContributionDescriptor`, install in `contributed_<kind>.go`, keep install+rollback (and replace+restore) one unit. `advertisedCapabilities` in `services.go` lists only wired capabilities; add one only with its consumer and live test. Contributed mutations use `provider_mutation.go`: declared operations, canonical targets, commit-time revision/absence preconditions and validated receipts; no new permission prompts. Provider `edit` stays refused until snapshot commit support exists. `providerbus.go` is the only caller of contributions; no mutex across Emit. `presentation*.go` renders from retained Details only: sanitize, fit every line to the width, never infer success from a missing snapshot, and let the host error override producer success.
