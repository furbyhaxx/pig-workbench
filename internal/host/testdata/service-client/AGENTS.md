# service-client fixture

Independent Go factory used by live host tests to exercise Workbench's shared event bus.
- No model inference; `/fixture` probes discovery, location and stale-generation rejection.
- Module replacement points at this checkout; the SDK is resolved by PiG's staged build.
- Do not advertise this fixture as a shipped Workbench command.
