# Product documentation

Human-facing documentation for shipped behavior; development artifacts belong in `.project/`.
- Settings defaults, types and scope rules come from `internal/config/catalog.go`; document live versus unavailable consumers accurately.
- Command or setting changes update the relevant page and the root README in the same change.
- The settings reference table in `configuration.md` is generated from the catalog; after a catalog change run `UPDATE_DOCS=1 just check`. Do not edit between its markers.
