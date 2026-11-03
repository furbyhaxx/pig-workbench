# host

SDK adapter and feature composition. `Register` is called once by the root factory.
- Keep SDK contexts at this boundary. Domain requests use a request cancellation adapter, never ctx.Signal as a substitute for per-request cancellation.
- Session state owns a lifecycle supervisor. Shutdown must not admit itself before draining.
- Restore custom entries from the active parent chain; GetBranch's projection omits custom data.
- Resolve config roots from SDK host helpers and the original ctx.Cwd; `cd` changes neither. Trust callbacks belong to one request and are never retained by config.
- Expose only wired capabilities; diagnostics describe actual state. Commands and companion aliases share handlers.
