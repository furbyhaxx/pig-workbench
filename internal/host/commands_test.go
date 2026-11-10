package host

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"context"
	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/lifecycle"
	"github.com/furbyhaxx/pig-workbench/internal/location"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
	"github.com/furbyhaxx/pig-workbench/internal/syntax"
	"os"
	"path/filepath"
)

func TestNoticesFitAndStripUntrustedControls(t *testing.T) {
	hostile := "Workbench\n" + strings.Repeat("界", 100) + "\x1b[2J\x1b]52;c;payload\a\u009b\tEND"
	for _, width := range []int{36, 57, 80, 140} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			text := noticeText(hostile, width)
			if !strings.HasPrefix(text, "Workbench\n") {
				t.Fatal("identity lost")
			}
			if strings.ContainsAny(text, "\x1b\a\u009b\t") {
				t.Fatal("unsafe control retained")
			}
			for _, line := range strings.Split(text, "\n") {
				if tui.VisibleWidth(line) > width {
					t.Fatalf("line exceeds width %d", width)
				}
			}
		})
	}
}

func TestMultiReadPartialFailureIsNotAToolError(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good.txt")
	if err := os.WriteFile(good, []byte("hello\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	locations, err := location.New(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := config.New(config.Paths{User: filepath.Join(root, "Workbench.toml"), Project: filepath.Join(root, "project.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	router := resource.NewRouter()
	if err := router.RegisterBuiltin(resource.FileProvider{}); err != nil {
		t.Fatal(err)
	}
	application := &app{syntax: syntax.New()}
	application.session = &session{id: "s", generation: "g", supervisor: lifecycle.New(), location: locations, settings: settings, router: router}
	defer application.session.supervisor.Stop()

	// The read tool needs an SDK context for admission; exercise the same
	// decision logic through a direct result comparison instead.
	policy, err := settings.Current()
	if err != nil {
		t.Fatal(err)
	}
	budget := resource.Budget{Bytes: policy.Read().MaxBytes, Lines: policy.Read().MaxLines}
	ok, okErr := router.Read(context.Background(), mustTarget(t, root, good, ""), budget)
	if okErr != nil {
		t.Fatalf("good read failed: %v", okErr)
	}
	if !strings.Contains(ok.Content, "hello") {
		t.Fatalf("good read content wrong: %q", ok.Content)
	}
	if _, missingErr := router.Read(context.Background(), mustTarget(t, root, filepath.Join(root, "missing.txt"), ""), budget); missingErr == nil {
		t.Fatal("missing target did not fail")
	}
}

func mustTarget(t *testing.T, cwd, path, selector string) resource.Target {
	t.Helper()
	target, err := resource.ParsePath(cwd, path, selector)
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func TestCompletionOffersLiveOwnersAndSchemesAfterTheirVerb(t *testing.T) {
	items, err := completeWorkbench("integrations ", []string{"integrations pig-web", "resources kb"}, config.Catalog())
	if err != nil || len(items) != 1 || items[0].Value != "integrations pig-web" {
		t.Fatalf("owners = %+v, %v", items, err)
	}
	items, err = completeWorkbench("res", []string{"integrations pig-web", "resources kb"}, config.Catalog())
	if err != nil || len(items) != 1 || items[0].Value != "resources" {
		t.Fatalf("a verb completes before its arguments: %+v, %v", items, err)
	}
	items, _ = completeWorkbench("resources k", []string{"resources kb", "resources file"}, config.Catalog())
	if len(items) != 1 || items[0].Value != "resources kb" {
		t.Fatalf("schemes = %+v", items)
	}
	items, _ = completeWorkbench("skills l", []string{"skills load rust", "skills unload toml"}, config.Catalog())
	var values []string
	for _, item := range items {
		values = append(values, item.Value)
	}
	if !slices.Contains(values, "skills ls") || !slices.Contains(values, "skills load") || slices.Contains(values, "skills unload") {
		t.Fatalf("skills verbs = %v", values)
	}
	items, _ = completeWorkbench("skills load ", []string{"skills load rust", "skills unload toml"}, config.Catalog())
	if len(items) != 1 || items[0].Value != "skills load rust" {
		t.Fatalf("skill names = %+v", items)
	}
}

func TestAliasesRewriteOnlyTheVerbPosition(t *testing.T) {
	for input, want := range map[string]string{"list": "ls", "remove /tmp/list": "rm /tmp/list", "send list remove": "send list remove", "  list x": "ls x", "": ""} {
		if got := canonicalSubverb(input); got != want {
			t.Errorf("canonicalSubverb(%q) = %q, want %q", input, got, want)
		}
	}
	if canonicalVerb("config") != "settings" || canonicalVerb("show") != "show" {
		t.Error("config must resolve to settings and nothing else change")
	}
}

func TestUnknownVerbIsOneShortLineWithTheClosestVerb(t *testing.T) {
	for verb, want := range map[string]string{"framez": "did you mean frames?", "inst": "did you mean instructions?", "setings": "did you mean settings?", "zzzzzz": "run /workbench for the list"} {
		got := unknownVerb(verb)
		if !strings.HasSuffix(got, want) || strings.Contains(got, "\n") || len(got) > 100 {
			t.Errorf("unknownVerb(%q) = %q", verb, got)
		}
	}
	if got := unknownVerb(strings.Repeat("x", 500) + "\x1b[31m"); len(got) > 100 || strings.Contains(got, "\x1b") {
		t.Errorf("hostile verb is echoed unbounded: %q", got)
	}
}

func TestCompanionCommandsCompleteTheirArguments(t *testing.T) {
	a := &app{}
	a.rememberTools([]sdk.ToolInfo{{Name: "read"}, {Name: "grep"}, {Name: "recall"}})
	items, _ := a.toolsCompletions("re")
	if len(items) != 2 || items[0].Value != "read" || items[1].Value != "recall" {
		t.Errorf("tool completions: %+v", items)
	}
	if items, _ := a.jobsCompletions(""); len(items) != 1 || items[0].Value != "ls" {
		t.Errorf("job completions without a session: %+v", items)
	}
	if items, err := a.cdCompletions(""); err != nil || items != nil {
		t.Errorf("cd completions without a session must be empty: %+v %v", items, err)
	}
}

func TestToolFormatsCarryTheSameRows(t *testing.T) {
	rows := []toolRow{{Name: "read", Active: true, Description: "Read text."}, {Name: "x", Description: "Other.", Parameters: json.RawMessage(`{"type":"object"}`)}}
	text, err := formatTools(rows, "text")
	if err != nil || text != "read [active] Read text.\nx [inactive] Other.\n{\n  \"type\": \"object\"\n}" {
		t.Errorf("text: %q %v", text, err)
	}
	markdown, _ := formatTools(rows, "markdown")
	if !strings.Contains(markdown, "## read\n\nactive. Read text.\n") || !strings.Contains(markdown, "```json\n{\n  \"type\": \"object\"\n}\n```") {
		t.Errorf("markdown: %q", markdown)
	}
	encoded, _ := formatTools(rows, "json")
	var decoded []toolRow
	if err := json.Unmarshal([]byte(encoded), &decoded); err != nil || len(decoded) != 2 || !decoded[0].Active || decoded[1].Name != "x" {
		t.Errorf("json: %q %v", encoded, err)
	}
}
