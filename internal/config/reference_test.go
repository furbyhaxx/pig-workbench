package config

import (
	"os"
	"strings"
	"testing"
)

const (
	referenceBegin = "<!-- settings-reference:begin (generated: UPDATE_DOCS=1 just check) -->\n"
	referenceEnd   = "<!-- settings-reference:end -->\n"
)

func TestTheDocumentedSettingsReferenceMatchesTheCatalog(t *testing.T) {
	const path = "../../docs/configuration.md"
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)
	head, rest, found := strings.Cut(page, referenceBegin)
	_, tail, closed := strings.Cut(rest, referenceEnd)
	if !found || !closed {
		t.Fatalf("%s has no settings-reference markers", path)
	}
	want := head + referenceBegin + Reference() + referenceEnd + tail
	if page == want {
		return
	}
	if os.Getenv("UPDATE_DOCS") == "" {
		t.Fatalf("%s settings reference is stale; run UPDATE_DOCS=1 just check", path)
	}
	if err := os.WriteFile(path, []byte(want), 0644); err != nil {
		t.Fatal(err)
	}
}
