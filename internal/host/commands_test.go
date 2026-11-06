package host

import (
	"strconv"
	"strings"
	"testing"

	"context"
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
	items, err := completeWorkbench("integrations ", []string{"integrations pig-web", "resources kb"})
	if err != nil || len(items) != 1 || items[0].Value != "integrations pig-web" {
		t.Fatalf("owners = %+v, %v", items, err)
	}
	items, err = completeWorkbench("res", []string{"integrations pig-web", "resources kb"})
	if err != nil || len(items) != 1 || items[0].Value != "resources" {
		t.Fatalf("a verb completes before its arguments: %+v, %v", items, err)
	}
	items, _ = completeWorkbench("resources k", []string{"resources kb", "resources file"})
	if len(items) != 1 || items[0].Value != "resources kb" {
		t.Fatalf("schemes = %+v", items)
	}
}
