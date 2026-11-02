# pig-workbench

A [PiG](https://github.com/MichaelKinsy/PiG) extension that upgrades the builtin coding tools: hashline editing, selector-driven reads, `scheme://` resources, local and SSH working locations, harness frames with instruction and backlog injection, questionnaires and background jobs.

**Status: early bootstrap.** The extension loads but registers no tools or commands yet. Scope and progress are tracked in [the implementation plan](.project/plans/0001-pig-workbench.md).

## Develop

Requires Go and a `pig` binary on `PATH`; the SDK is the staged copy that matches the running `pig`.

```sh
just check            # gofmt, vet, tests and PiG validation
pig -e .              # load into a session; /reload after edits
```
