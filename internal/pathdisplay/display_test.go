package pathdisplay

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
)

// TestGoldenCases pins the SPEC-0003 §10.3 table. The home rows are rewritten
// for the running user because the donor examples use a fixed home directory.
func TestGoldenCases(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("no home directory")
	}
	cases := []struct {
		name  string
		t     Target
		width int
		want  string
	}{
		{"under root", Target{Value: "/work/app/src/a.ts", Root: "/work/app"}, 140, "src/a.ts"},
		{"sibling root", Target{Value: "/work/other/a.ts", Root: "/work/app"}, 140, "../other/a.ts"},
		{"unrelated absolute", Target{Value: "/etc/hosts", Root: "/work/deep/er/app"}, 140, "/etc/hosts"},
		{"home fish", Target{Value: home + "/projects/pi-workbench/src/renderers.ts"}, 30, "~/p/p/src/renderers.ts"},
		{"absolute fish", Target{Value: "/srv/projects/pi-workbench/src/renderers.ts"}, 22, "/s/p/p/s/renderers.ts"},
		{"relative elision", Target{Value: "../pkg/lib/deep/dir/file.ts"}, 18, "../pkg/…/file.ts"},
		{"ssh authority", Target{Value: "ssh://dev@build:2222/srv/app/src/a.ts", Root: "ssh://dev@build:2222/srv/app"}, 140, "ssh://dev@build:2222:src/a.ts"},
	}
	for _, c := range cases {
		if got := Display(c.t, c.width); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// TestWidthsNeverExceedBudget proves no shown path exceeds 36, 57, 80 or 140
// columns and that the basename survives wherever it fits.
func TestWidthsNeverExceedBudget(t *testing.T) {
	targets := []Target{
		{Value: "/srv/projects/pi-workbench/src/renderers/tool-presentation.ts"},
		{Value: "/a/b/c/d/e/f/g/h/i/j/k/l/m/n/o/p/q/r/s/t/u/v/w/x/y/z/file.go"},
		{Value: "~/projects/pi-workbench/internal/host/presentation_render.go"},
		{Value: "ssh://dev@build:2222/srv/app/src/renderers/deep/nested/file.ts", Root: "ssh://dev@build:2222/srv/app"},
		{Value: "session://0193f2c4-1111-2222-3333-444455556666/transcript"},
	}
	for _, width := range []int{36, 57, 80, 140} {
		for _, target := range targets {
			shown := Display(target, width)
			if got := tui.VisibleWidth(shown); got > width {
				t.Errorf("width %d: %q is %d cells", width, shown, got)
			}
			base := filepath.Base(target.Value)
			if tui.VisibleWidth(base) <= width && !containsBase(shown, base) {
				t.Errorf("width %d: %q lost the basename %q", width, shown, base)
			}
		}
	}
}

func containsBase(shown, base string) bool {
	return len(shown) >= len(base) && shown[len(shown)-len(base):] == base
}

// TestAuthorityIsNeverShortened proves the SSH authority survives the width.
func TestAuthorityIsNeverShortened(t *testing.T) {
	target := Target{Value: "ssh://dev@build:2222/srv/app/src/a.ts", Root: "ssh://dev@build:2222/srv/app"}
	for _, width := range []int{36, 57, 80, 140} {
		shown := Display(target, width)
		if !hasPrefix(shown, "ssh://dev@build:2222:") {
			t.Errorf("width %d: authority was rewritten in %q", width, shown)
		}
	}
}

func hasPrefix(text, prefix string) bool { return strings.HasPrefix(text, prefix) }

// TestResourceURIsKeepTheirScheme proves a Workbench URI is not made relative.
func TestResourceURIsKeepTheirScheme(t *testing.T) {
	for _, value := range []string{"artifact://prj-1/spec.md", "job://job-3/log", "skill://parser"} {
		shown := Display(Target{Value: value, Root: "/work/app"}, 140)
		if !hasPrefix(shown, value[:len(value)-len(filepath.Base(value))]) {
			t.Errorf("%q lost its scheme: %q", value, shown)
		}
	}
}

func TestNoFishElidesInsteadOfAbbreviatingAndStaysWithinWidth(t *testing.T) {
	value := "/srv/projects/pi-workbench/src/renderers.ts"
	fish := Display(Target{Value: value}, 22)
	plain := Display(Target{Value: value, NoFish: true}, 22)
	if fish != "/s/p/p/s/renderers.ts" {
		t.Fatalf("fish golden changed: %q", fish)
	}
	if strings.Contains(plain, "/s/p/") || !strings.Contains(plain, "…") || !strings.HasSuffix(plain, "renderers.ts") || tui.VisibleWidth(plain) > 22 {
		t.Fatalf("shortening off must elide, keep the basename and fit: %q", plain)
	}
	if got := Display(Target{Value: value, NoFish: true}, 200); got != value {
		t.Fatalf("a path that fits must be unchanged: %q", got)
	}
}
