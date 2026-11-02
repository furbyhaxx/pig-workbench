set shell := ["bash", "-euo", "pipefail", "-c"]

tmp := justfile_directory() + "/.tmp"

default: check

# Write a temporary modfile that resolves the SDK to the staged copy of the running pig.
_modfile:
	mkdir -p "{{tmp}}"
	cp go.mod "{{tmp}}/go.mod"
	touch "{{tmp}}/go.sum"
	printf '\nreplace github.com/MichaelKinsy/PiG/extensions/sdk => %s\n' "$(pig reload --sdk-path)" >> "{{tmp}}/go.mod"

fmt:
	gofmt -w .

fmt-check:
	test -z "$(gofmt -l .)" || { gofmt -l .; exit 1; }

vet: _modfile
	go vet -modfile="{{tmp}}/go.mod" ./...

test: _modfile
	go test -modfile="{{tmp}}/go.mod" ./...

race: _modfile
	go test -race -modfile="{{tmp}}/go.mod" ./...

validate:
	pig install . --validate-only --json

check: fmt-check vet test validate
