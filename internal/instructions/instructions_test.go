package instructions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func reasonFor(set Set, path, status string) bool {
	for _, reason := range set.Reasons {
		if reason.Path == path && reason.Status == status {
			return true
		}
	}
	return false
}

func TestDiscoverNestedSteeringAndCappedBacklog(t *testing.T) {
	root := t.TempDir()
	broad := write(t, root, "AGENTS.md", "root rules\n")
	nested := write(t, root, "a/b/AGENTS.md", "nested rules\n")
	var backlog strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&backlog, "- B-%04d bug high — task %d → .project/backlog/%04d.md\n", i, i, i)
	}
	backlogPath := write(t, root, "BACKLOG.md", backlog.String())
	classes := DefaultClasses()
	classes[1].MaxItems = 10
	set, err := Discover(context.Background(), filepath.Join(root, "a/b"), classes, Options{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Files) != 3 || set.Files[0].Path != broad || set.Files[1].Path != backlogPath || set.Files[2].Path != nested {
		t.Fatalf("wrong root-first selection: %+v", set.Files)
	}
	if !reasonFor(set, broad, "selected") || !reasonFor(set, nested, "selected") {
		t.Fatalf("missing selected reasons: %+v", set.Reasons)
	}
	item := set.Files[1]
	if !item.Truncated || item.OmittedItems != 20 || item.Continuation == "" {
		t.Fatalf("bad cap metadata: %+v", item)
	}
	rendered := set.Render()
	for i := 1; i <= 10; i++ {
		line := fmt.Sprintf("- B-%04d bug high — task %d → .project/backlog/%04d.md\n", i, i, i)
		if !strings.Contains(rendered, line) {
			t.Fatalf("missing or cut pointer %d: %q", i, rendered)
		}
	}
	if strings.Contains(rendered, "- B-0011") || !strings.Contains(rendered, "(+20 more → "+item.Continuation+")") {
		t.Fatalf("bad continuation: %q", rendered)
	}
	if strings.Contains(rendered, root) {
		t.Fatalf("render leaked absolute root: %q", rendered)
	}
}

func TestOverrideWinsAndBoundaryStops(t *testing.T) {
	outer := t.TempDir()
	write(t, outer, "AGENTS.md", "must not appear\n")
	root := filepath.Join(outer, "repo")
	override := write(t, root, "AGENTS.override.md", "override\n")
	shadowed := write(t, root, "AGENTS.md", "shadowed\n")
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	set, err := Discover(context.Background(), filepath.Join(root, "nested"), []Class{{ID: "steering"}}, Options{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Files) != 1 || set.Files[0].Path != override || !reasonFor(set, shadowed, "shadowed") || strings.Contains(set.Render(), "must not appear") {
		t.Fatalf("override or boundary failed: %+v", set)
	}
}

func TestCanonicalDedupAndEscape(t *testing.T) {
	root := t.TempDir()
	original := write(t, root, "AGENTS.md", "original\n")
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "nested", "AGENTS.md")
	if err := os.Symlink(original, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	set, err := Discover(context.Background(), filepath.Join(root, "nested"), []Class{{ID: "steering"}}, Options{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Files) != 1 || set.Files[0].Path != original || !reasonFor(set, link, "selected") {
		t.Fatalf("canonical duplicate not collapsed: %+v", set)
	}

	outside := filepath.Join(root, "nested", "AGENTS.override.md")
	if err := os.Symlink("/etc/hostname", outside); err != nil {
		t.Fatal(err)
	}
	set, err = Discover(context.Background(), filepath.Join(root, "nested"), []Class{{ID: "steering"}}, Options{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if !reasonFor(set, outside, "ignored") {
		t.Fatalf("escaped symlink was not ignored: %+v", set.Reasons)
	}
	for _, file := range set.Files {
		if file.Path == "/etc/hostname" {
			t.Fatal("escaped symlink was read")
		}
	}
}

func TestUserRootsPrecedeProject(t *testing.T) {
	user := t.TempDir()
	root := t.TempDir()
	write(t, user, "AGENTS.md", "user rules\n")
	write(t, root, "AGENTS.md", "project rules\n")
	set, err := Discover(context.Background(), root, []Class{{ID: "steering"}}, Options{ProjectRoot: root, UserRoots: []string{user}})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Files) != 2 || set.Files[0].Trust != "user" || set.Files[1].Trust != "project" {
		t.Fatalf("wrong scope or order: %+v", set.Files)
	}
	rendered := set.Render()
	if strings.Index(rendered, "user rules") < 0 || strings.Index(rendered, "user rules") > strings.Index(rendered, "project rules") ||
		strings.Contains(rendered, user) || strings.Contains(rendered, root) {
		t.Fatalf("wrong rendered order or leaked path: %q", rendered)
	}
}

func TestPlacementsAndSelectAll(t *testing.T) {
	root := t.TempDir()
	middle := write(t, root, "a/RULES.md", "middle\n")
	nested := write(t, root, "a/b/RULES.md", "nested\n")
	class := Class{ID: "rules", Filenames: []string{"RULES.md"}, Placement: "root+nearest"}
	set, err := Discover(context.Background(), filepath.Join(root, "a/b"), []Class{class}, Options{ProjectRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Files) != 2 || set.Files[0].Path != middle || set.Files[1].Path != nested {
		t.Fatalf("root+nearest did not select broadest existing and nearest: %+v", set.Files)
	}
	class.Placement = "root"
	set, err = Discover(context.Background(), filepath.Join(root, "a/b"), []Class{class}, Options{ProjectRoot: root})
	if err != nil || len(set.Files) != 0 {
		t.Fatalf("root selected a non-root file: %+v, %v", set.Files, err)
	}
	first := write(t, root, "ONE.md", "one\n")
	second := write(t, root, "TWO.md", "two\n")
	class = Class{ID: "all", Filenames: []string{"ONE.md", "TWO.md"}, Placement: "root", Select: "all"}
	set, err = Discover(context.Background(), root, []Class{class}, Options{ProjectRoot: root})
	if err != nil || len(set.Files) != 2 || set.Files[0].Path != first || set.Files[1].Path != second {
		t.Fatalf("select all order: %+v, %v", set.Files, err)
	}
}

func TestMissingCancellationAndValidation(t *testing.T) {
	root := t.TempDir()
	set, err := Discover(context.Background(), root, []Class{{ID: "steering"}}, Options{ProjectRoot: root})
	if err != nil || len(set.Files) != 0 || len(set.Reasons) == 0 || set.Reasons[0].Status != "missing" {
		t.Fatalf("missing files: %+v, %v", set, err)
	}
	if err := os.Mkdir(filepath.Join(root, "AGENTS.md"), 0700); err != nil {
		t.Fatal(err)
	}
	set, err = Discover(context.Background(), root, []Class{{ID: "steering"}}, Options{ProjectRoot: root})
	if err != nil || !reasonFor(set, filepath.Join(root, "AGENTS.md"), "missing") {
		t.Fatalf("unreadable candidate not reported: %+v, %v", set, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Discover(ctx, root, nil, Options{ProjectRoot: root}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
	if _, err := Discover(context.Background(), root, []Class{{ID: "bad", Filenames: []string{"../escape"}}}, Options{ProjectRoot: root}); err == nil {
		t.Fatal("invalid filename accepted")
	}
	if _, err := Discover(context.Background(), t.TempDir(), []Class{{ID: "steering"}}, Options{ProjectRoot: root}); err == nil {
		t.Fatal("target outside boundary accepted")
	}
}

func TestDigestCoversRawBytesAndViewNormalizes(t *testing.T) {
	root := t.TempDir()
	path := write(t, root, "AGENTS.md", "\xef\xbb\xbffirst\r\nsecond\r\n")
	discover := func() Selected {
		t.Helper()
		set, err := Discover(context.Background(), root, []Class{{ID: "steering"}}, Options{ProjectRoot: root})
		if err != nil || len(set.Files) != 1 {
			t.Fatalf("discovery: %+v, %v", set, err)
		}
		return set.Files[0]
	}
	crlf := discover()
	sum := sha256.Sum256([]byte("\xef\xbb\xbffirst\r\nsecond\r\n"))
	if crlf.Digest != hex.EncodeToString(sum[:]) || crlf.Content != "first\nsecond\n" {
		t.Fatalf("digest or view: %+v", crlf)
	}
	if err := os.WriteFile(path, []byte("first\nsecond\n"), 0600); err != nil {
		t.Fatal(err)
	}
	lf := discover()
	if lf.Digest == crlf.Digest || lf.Content != crlf.Content {
		t.Fatalf("line-ending revision mismatch: %+v %+v", crlf, lf)
	}
}

func TestBoundedScanReportsUnknownOmissions(t *testing.T) {
	root := t.TempDir()
	body := "- B-0001 bug high — first → a.md\n- B-0002 bug high — second → b.md\n"
	write(t, root, "BACKLOG.md", body)
	limit := len("- B-0001 bug high — first → a.md\n") + 8
	set, err := Discover(context.Background(), root, []Class{{ID: "backlog"}}, Options{ProjectRoot: root, MaxTotalBytes: limit})
	if err != nil || len(set.Files) != 1 {
		t.Fatalf("bounded discovery: %+v, %v", set, err)
	}
	file := set.Files[0]
	sum := sha256.Sum256([]byte(body[:limit]))
	if !file.Truncated || file.OmittedItems != -1 || file.Digest != hex.EncodeToString(sum[:]) ||
		strings.Contains(file.Content, "B-0002") || !strings.Contains(set.Render(), "count unavailable") {
		t.Fatalf("dishonest partial scan: %+v, %q", file, set.Render())
	}
}
