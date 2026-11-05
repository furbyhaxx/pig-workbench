# reference-producer fixture

pig-web-shaped Go factory that exercises every Workbench contribution seam through `api.Client` in live tests.
- Registers on `session_start`: `web_search` presentation, `kb://` resource provider, `fixture.test` fetch backend, `web` status segment, `reference.research` frame producer with `knowledge_cutoff`, and `docs://reference-producer/`.
- Owns tool `web_search` and command `/reference` (`notice`, `status`). No model inference; no network.
- Change a seam contract in Workbench → update this fixture and `internal/host/live_reference_test.go` together.
- Do not advertise this fixture as a shipped extension.
