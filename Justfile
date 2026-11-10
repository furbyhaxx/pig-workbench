set shell := ["bash", "-euo", "pipefail", "-c"]

# The root go.work only serves PiG's package build; the checks use the staged modfile instead.
export GOWORK := "off"

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

# Cross-vet for windows/amd64 and, when wine is installed, run the test binaries of the packages
# that are expected to pass there. This is not a Windows release gate (SPEC-0008: Windows is not a supported platform yet).
windows: _modfile
	GOOS=windows GOARCH=amd64 go vet -modfile="{{tmp}}/go.mod" ./...
	GOOS=windows GOARCH=amd64 go vet -tags=integration -modfile="{{tmp}}/go.mod" ./...
	command -v wine >/dev/null || { echo "wine not installed: compile-only"; exit 0; }
	mkdir -p "{{tmp}}/wintest"
	for n in api contribution frames hashline htmlread jobs lifecycle location pathdisplay presentation project prompt skills ask; do \
		dir=internal/$n; test -d "$dir" || dir=$n; \
		GOOS=windows GOARCH=amd64 go test -c -modfile="{{tmp}}/go.mod" -o "{{tmp}}/wintest/$n.exe" "./$dir"; \
		(cd "$dir" && WINEDEBUG=-all WINEPREFIX="{{tmp}}/wineprefix" wine "{{tmp}}/wintest/$n.exe" >/dev/null) || { echo "FAIL under wine: $n"; exit 1; }; \
	done

validate: build
	pig package validate . --json

# Build and load the installable extension through PiG, as an install would.
build:
	pig --no-session --offline -ne -e ./extensions/pig-workbench --list-models synthetic > /dev/null

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

artifacts:
	python3 scripts/check_artifacts.py .

check: fmt-check vet test validate artifacts
