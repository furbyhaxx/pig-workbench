set shell := ["bash", "-euo", "pipefail", "-c"]

tmp := justfile_directory() + "/.tmp"

# Probe modules in .project are not part of the product build.
go_sources := "find . -type f -name '*.go' -not -path './.*' -print0"

default: check

# Write a temporary modfile that resolves the SDK to the staged copy of the running pig.
_modfile:
	mkdir -p "{{tmp}}"
	cp go.mod "{{tmp}}/go.mod"
	touch "{{tmp}}/go.sum"
	printf '\nreplace github.com/MichaelKinsy/PiG/extensions/sdk => %s\n' "$(pig reload --sdk-path)" >> "{{tmp}}/go.mod"

fmt:
	{{go_sources}} | xargs -0 -r gofmt -w

fmt-check:
	files="$({{go_sources}} | xargs -0 -r gofmt -l)"; test -z "$files" || { printf '%s\n' "$files"; exit 1; }

vet: _modfile
	go vet -modfile="{{tmp}}/go.mod" ./...

test: _modfile
	go test -modfile="{{tmp}}/go.mod" ./...

race: _modfile
	go test -race -modfile="{{tmp}}/go.mod" ./...

integration: _modfile
	go test -race -tags=integration -modfile="{{tmp}}/go.mod" ./internal/host

validate:
	pig install . --validate-only --json

check: fmt-check vet test validate
