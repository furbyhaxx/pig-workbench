set shell := ["bash", "-euo", "pipefail", "-c"]

tmp := justfile_directory() + "/.tmp"

# Probe modules in .project are not part of the product build.
go_sources := "find . -type f -name '*.go' -not -path './.*' -print0"

default: check

# Write a temporary modfile that resolves the SDK to the staged copy of the running pig.
_modfile:
	mkdir -p "{{tmp}}"
	cp go.mod "{{tmp}}/go.mod"
	if test -f go.sum; then cp go.sum "{{tmp}}/go.sum"; else : > "{{tmp}}/go.sum"; fi
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

# Opt-in live evaluation with real provider spend; the key stays in the environment.
live-eval pattern="TestLiveEval": _modfile
	mkdir -p "{{justfile_directory()}}/.tmp/live-eval"
	DEEPSEEK_API_KEY="$(pig auth print-api-key --provider deepseek)" LIVE_EVAL_OUT="${LIVE_EVAL_OUT:-{{justfile_directory()}}/.tmp/live-eval}" go test -tags='integration liveeval' -modfile="{{tmp}}/go.mod" ./internal/host -run '{{pattern}}' -count=1 -v -timeout 30m

validate:
	pig install . --validate-only --json

# Verify the COMMITTED tree builds and passes in a throwaway worktree.
# Guards against committing code that imports still-untracked files.
[script]
head-check:
    set -euo pipefail
    work="$(mktemp -d)"
    trap 'git worktree remove --force "$work" 2>/dev/null || rm -rf "$work"' EXIT
    git worktree add -q --detach "$work" HEAD
    cp go.mod go.sum "$work/"
    mkdir -p "$work/.tmp"
    cp go.mod "$work/.tmp/go.mod"
    cp go.sum "$work/.tmp/go.sum"
    printf '\nreplace github.com/MichaelKinsy/PiG/extensions/sdk => %s\n' "$(pig reload --sdk-path)" >> "$work/.tmp/go.mod"
    ( cd "$work" && go build -modfile=.tmp/go.mod ./... && go test -modfile=.tmp/go.mod ./... )

check: fmt-check vet test validate
