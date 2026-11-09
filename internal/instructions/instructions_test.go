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
	"time"
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

func TestDiscoverProjectPrivateNotesAreConfinedAndCapped(t *testing.T) {
	root := t.TempDir()
	private := filepath.Join(t.TempDir(), "private")
	note := write(t, private, "notes.md", strings.Repeat("private rule\n", 3000))
	write(t, private, "ignore.txt", "not a note\n")
	write(t, private, "nested/hidden.md", "not a root note\n")
	other := write(t, t.TempDir(), "other.md", "other project secret\n")
	if err := os.Symlink(other, filepath.Join(private, "escape.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	set, err := Discover(context.Background(), root, nil, Options{ProjectRoot: root, PrivateRoot: private})
	if err != nil {
		t.Fatal(err)
	}
	if len(set.Files) != 1 || set.Files[0].Path != note || set.Files[0].Class != "project-private" || set.Files[0].Trust != "user" || set.Files[0].Role != "instruction" || !set.Files[0].Truncated {
		t.Fatalf("incorrect private selection: %+v", set.Files)
	}
	if strings.Contains(set.Render(), "other project secret") || !reasonFor(set, filepath.Join(private, "escape.md"), "ignored") {
		t.Fatalf("private symlink was followed: %+v", set.Reasons)
	}
	otherSet, err := Discover(context.Background(), root, nil, Options{ProjectRoot: root, PrivateRoot: filepath.Dir(other)})
	if err != nil || len(otherSet.Files) != 1 || otherSet.Files[0].Path != other || strings.Contains(otherSet.Render(), "private rule") {
		t.Fatalf("private roots were not isolated: %+v, %v", otherSet.Files, err)
	}
	link := filepath.Join(t.TempDir(), "private")
	if err := os.Symlink(private, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Discover(context.Background(), root, nil, Options{ProjectRoot: root, PrivateRoot: link}); err == nil {
		t.Fatal("symlinked private directory accepted")
	}
}

func TestPrivateNotesRespectTotalSourceBudget(t *testing.T) {
	root, private := t.TempDir(), t.TempDir()
	write(t, private, "a.md", strings.Repeat("long rule\n", 20))
	later := write(t, private, "b.md", "must not be read\n")
	_, err := Discover(context.Background(), root, nil, Options{ProjectRoot: root, PrivateRoot: private, MaxTotalBytes: 64})
	if err == nil || !strings.Contains(err.Error(), "budget exhausted") {
		t.Fatalf("exhausted budget did not fail closed before %s: %v", later, err)
	}
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

func TestViewsReserveFooterAndKeepWholeLines(t *testing.T) {
	root := t.TempDir()
	first := "- B-0001 bug high — first → a.md\n"
	second := "- B-0002 bug high — " + strings.Repeat("long", 20) + " → b.md\n"
	third := "- B-0003 bug high — third → c.md\n"
	write(t, root, "BACKLOG.md", first+second+third)
	class := Class{ID: "backlog", MaxItems: 10, MaxLines: 2, MaxBytes: 128}
	set, err := Discover(context.Background(), root, []Class{class}, Options{ProjectRoot: root})
	if err != nil || len(set.Files) != 1 {
		t.Fatalf("discovery: %+v, %v", set, err)
	}
	file := set.Files[0]
	marker := continuationMarker(file.OmittedItems, true, file.Continuation)
	if file.Content != first || file.OmittedItems != 2 || len(file.Content)+len(marker)+1 > class.MaxBytes {
		t.Fatalf("line or byte limit violated: %+v, %q", file, marker)
	}
	class.MaxBytes = 8
	set, err = Discover(context.Background(), root, []Class{class}, Options{ProjectRoot: root})
	if err != nil || len(set.Files) != 0 || !reasonFor(set, filepath.Join(root, "BACKLOG.md"), "capped") {
		t.Fatalf("unrepresentable footer did not fail closed: %+v, %v", set, err)
	}
}

func TestContinuationBudgetDoesNotOscillate(t *testing.T) {
	root := t.TempDir()
	pointer := "- B-0001 bug high — item → a.md\n"
	limit := len(pointer) + len(continuationMarker(1, true, "BACKLOG.md")) + 2
	write(t, root, "BACKLOG.md", pointer+strings.Repeat("x", limit)+"\n")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	set, err := Discover(ctx, root, []Class{{ID: "backlog", MaxBytes: limit}}, Options{ProjectRoot: root})
	if err != nil || len(set.Files) != 1 || !set.Files[0].Truncated || set.Files[0].OmittedItems != 1 {
		t.Fatalf("continuation budget failed to settle: %+v, %v", set, err)
	}
}

func TestMalformedPointerRemainsVisibleWithDiagnostic(t *testing.T) {
	root := t.TempDir()
	write(t, root, "BACKLOG.md", "- B-broken not a pointer\n- B-0001 bug high — valid → a.md\n")
	set, err := Discover(context.Background(), root, []Class{{ID: "backlog"}}, Options{ProjectRoot: root})
	if err != nil || len(set.Files) != 1 {
		t.Fatalf("discovery: %+v, %v", set, err)
	}
	if !strings.Contains(set.Render(), "- B-broken not a pointer\n[malformed backlog pointer at line 1]") ||
		!reasonFor(set, filepath.Join(root, "BACKLOG.md"), "malformed-pointer") {
		t.Fatalf("missing malformed pointer diagnostic: %+v, %q", set.Reasons, set.Render())
	}
}

func TestPointerTargetValidationIsExplicitlyDeferred(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join("..", filepath.Base(root)+"-absent.md")
	if _, err := os.Stat(filepath.Join(root, target)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target must not exist: %v", err)
	}
	path := write(t, root, "BACKLOG.md", "- B-0001 bug high — absent → "+filepath.ToSlash(target)+"\n")
	set, err := Discover(context.Background(), root, []Class{{ID: "backlog"}}, Options{ProjectRoot: root, ValidatePointers: true})
	if err != nil || len(set.Files) != 1 || !reasonFor(set, path, "pointer-validation-deferred") {
		t.Fatalf("deferred pointer target validation was not reported: %+v, %v", set, err)
	}
	for _, reason := range set.Reasons {
		if reason.Status == "pointer-validation-deferred" && !strings.Contains(reason.Detail, "line 1") {
			t.Fatalf("deferred reason has no line number: %+v", reason)
		}
	}
}

func TestDuplicateUsesMostRestrictiveRoleAndLimits(t *testing.T) {
	root := t.TempDir()
	path := write(t, root, "SHARED.md", "one\ntwo\n")
	classes := []Class{
		{ID: "first", Filenames: []string{"SHARED.md"}, Role: "instruction"},
		{ID: "second", Filenames: []string{"SHARED.md"}, Role: "context", MaxLines: 1},
	}
	set, err := Discover(context.Background(), root, classes, Options{ProjectRoot: root})
	if err != nil || len(set.Files) != 1 || set.Files[0].Role != "context" || !set.Files[0].Truncated {
		t.Fatalf("duplicate did not use restrictive policy: %+v, %v", set, err)
	}
	foundConflict := false
	for _, reason := range set.Reasons {
		if reason.Path == path && strings.Contains(reason.Detail, "conflicting role") {
			foundConflict = true
		}
	}
	if !foundConflict {
		t.Fatalf("role conflict was not exposed: %+v", set.Reasons)
	}
}

func TestExplicitUserRootCannotEscapeAndDefaultNamesAreLimited(t *testing.T) {
	user := t.TempDir()
	root := t.TempDir()
	write(t, root, "CLAUDE.md", "not discovered\n")
	write(t, root, "AGENTS.md", "project\n")
	link := filepath.Join(user, "AGENTS.md")
	if err := os.Symlink(filepath.Join(root, "AGENTS.md"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	set, err := Discover(context.Background(), root, []Class{{ID: "steering"}}, Options{ProjectRoot: root, UserRoots: []string{user}})
	if err != nil || len(set.Files) != 1 || set.Files[0].Trust != "project" || !reasonFor(set, link, "ignored") || strings.Contains(set.Render(), "not discovered") {
		t.Fatalf("user escape or default aliases: %+v, %v", set, err)
	}
}

type cancelAfterChecks struct {
	context.Context
	cancel context.CancelFunc
	checks int
}

func (c *cancelAfterChecks) Err() error {
	c.checks++
	if c.checks == 4 {
		c.cancel()
	}
	return c.Context.Err()
}

func TestCancellationDuringDiscovery(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "rules\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	counted := &cancelAfterChecks{Context: ctx, cancel: cancel}
	if _, err := Discover(counted, root, []Class{{ID: "steering"}}, Options{ProjectRoot: root}); !errors.Is(err, context.Canceled) || counted.checks < 4 {
		t.Fatalf("in-flight cancellation ignored: checks=%d, err=%v", counted.checks, err)
	}
}

func TestTotalBudgetIsSharedBetweenFiles(t *testing.T) {
	root := t.TempDir()
	first := "- B-0001 bug high — first → a.md\n"
	write(t, root, "FIRST.md", first)
	write(t, root, "SECOND.md", "- B-0002 bug high — second → b.md\n")
	class := Class{ID: "backlog", Filenames: []string{"FIRST.md", "SECOND.md"}, Select: "all"}
	set, err := Discover(context.Background(), root, []Class{class}, Options{ProjectRoot: root, MaxTotalBytes: len(first)})
	if err != nil || len(set.Files) != 2 {
		t.Fatalf("discovery: %+v, %v", set, err)
	}
	if set.Files[0].Truncated || !set.Files[1].Truncated || set.Files[1].OmittedItems != -1 || set.Files[1].Content != "" {
		t.Fatalf("total source budget was not shared: %+v", set.Files)
	}
}

func TestNestedRepositoryRequiresExplicitConsent(t *testing.T) {
	root := t.TempDir()
	broad := write(t, root, "AGENTS.md", "outer\n")
	nested := write(t, root, "sub/AGENTS.md", "nested\n")
	write(t, root, "sub/.git", "gitdir: /somewhere\n")
	_, err := Discover(context.Background(), filepath.Dir(nested), []Class{{ID: "steering"}}, Options{ProjectRoot: root})
	var boundaryErr *NestedRepositoryError
	if !errors.As(err, &boundaryErr) || boundaryErr.Dir != filepath.Dir(nested) || boundaryErr.ProjectRoot != root {
		t.Fatalf("nested boundary not refused with typed error: %v", err)
	}
	set, err := Discover(context.Background(), filepath.Dir(nested), []Class{{ID: "steering"}}, Options{ProjectRoot: root, AllowNested: true})
	if err != nil || len(set.Files) != 2 || set.Files[0].Path != broad || set.Files[1].Path != nested ||
		!reasonFor(set, filepath.Join(root, "sub/.git"), "crossed-nested-repository") {
		t.Fatalf("explicit nested boundary crossing: %+v, %v", set, err)
	}
	other := t.TempDir()
	if err := os.MkdirAll(filepath.Join(other, "child/.git"), 0700); err != nil {
		t.Fatal(err)
	}
	_, err = Discover(context.Background(), filepath.Join(other, "child"), []Class{{ID: "steering"}}, Options{ProjectRoot: other})
	if !errors.As(err, &boundaryErr) || boundaryErr.Dir != filepath.Join(other, "child") {
		t.Fatalf("nested .git directory not refused: %v", err)
	}
}

func TestFileTargetSymlinkUsesCanonicalAncestry(t *testing.T) {
	root := t.TempDir()
	write(t, root, "alias/AGENTS.md", "wrong rules\n")
	physicalRules := write(t, root, "physical/AGENTS.md", "physical rules\n")
	target := write(t, root, "physical/data.txt", "data\n")
	link := filepath.Join(root, "alias", "data.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	set, err := Discover(context.Background(), link, []Class{{ID: "steering"}}, Options{ProjectRoot: root})
	if err != nil || len(set.Files) != 1 || set.Files[0].Path != physicalRules {
		t.Fatalf("file symlink used the wrong ancestry: %+v, %v", set, err)
	}
	escaped := filepath.Join(root, "alias", "outside.txt")
	if err := os.Symlink(write(t, t.TempDir(), "outside.txt", "outside"), escaped); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(context.Background(), escaped, []Class{{ID: "steering"}}, Options{ProjectRoot: root}); err == nil {
		t.Fatal("file target symlink escaped the boundary")
	}
}

func TestRootNearestAndSymlinkDirectoryBoundaries(t *testing.T) {
	root := t.TempDir()
	broad := write(t, root, "RULES.md", "root\n")
	middle := write(t, root, "a/RULES.md", "middle\n")
	closest := write(t, root, "a/b/RULES.md", "closest\n")
	class := Class{ID: "rules", Filenames: []string{"RULES.md"}, Placement: "root+nearest"}
	set, err := Discover(context.Background(), filepath.Join(root, "a/b"), []Class{class}, Options{ProjectRoot: root})
	if err != nil || len(set.Files) != 2 || set.Files[0].Path != broad || set.Files[1].Path != closest || !reasonFor(set, middle, "ignored") {
		t.Fatalf("root+nearest placement: %+v, %v", set, err)
	}
	class.Placement = "nearest"
	set, err = Discover(context.Background(), filepath.Join(root, "a/b"), []Class{class}, Options{ProjectRoot: root})
	if err != nil || len(set.Files) != 1 || set.Files[0].Path != closest {
		t.Fatalf("nearest placement: %+v, %v", set, err)
	}
	link := filepath.Join(root, "shortcut")
	if err := os.Symlink(filepath.Join(root, "a"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := Discover(context.Background(), filepath.Join(link, "b"), []Class{class}, Options{ProjectRoot: root}); err == nil {
		t.Fatal("target traversal through a symlinked directory was accepted")
	}
}

func TestAnnotatePointersShowsEveryLineAndMarksBadTargets(t *testing.T) {
	root := t.TempDir()
	write := func(name, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("open.md", "---\ndocument:\n  status: \"open\"\n---\nbody\n")
	write("done.md", "---\ndocument:\n  status: \"done\"\n---\n")
	write("bare.md", "no frontmatter\n")
	content := strings.Join([]string{
		"# Backlog",
		"- B-0001 task high — fine → open.md",
		"- B-0002 task high — finished → done.md",
		"- B-0003 task high — gone → missing.md",
		"- B-0004 task high — bare → bare.md",
		"- B-0005 task high — escape → ../outside.md",
		"",
	}, "\n")
	got := AnnotatePointers(root, content)
	for want, line := range map[string]string{
		"→ open.md\n":                                  "an open target stays unmarked",
		"→ done.md [target status: done]":              "a finished target is marked",
		"→ missing.md [target missing]":                "a missing target is marked",
		"→ bare.md [target has no status]":             "a target without status is marked",
		"→ ../outside.md [target outside the project]": "an escaping target is refused",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("%s: %q not in\n%s", line, want, got)
		}
	}
	if strings.Count(got, "\n") != strings.Count(content, "\n") {
		t.Fatal("annotation must not add or drop lines")
	}
}

// addLinkedWorktree lays out what `git worktree add` writes: a .git file in the worktree and a back-link in the main checkout.
func addLinkedWorktree(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, ".worktrees", name)
	admin := filepath.Join(root, ".git", "worktrees", name)
	write(t, root, ".git/HEAD", "ref: refs/heads/main\n")
	write(t, admin, "gitdir", filepath.Join(dir, ".git")+"\n")
	write(t, dir, ".git", "gitdir: "+admin+"\n")
	return dir
}

func TestLinkedWorktreeBelowTheRootIsNotABoundary(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rootRules := write(t, root, "AGENTS.md", "shared rules\n")
	dir := addLinkedWorktree(t, root, "feat")
	own := write(t, dir, "AGENTS.md", "feature rules\n")
	target := write(t, dir, "pkg/code.go", "package pkg\n")
	set, err := Discover(context.Background(), target, []Class{{ID: "steering"}}, Options{ProjectRoot: root})
	if err != nil || len(set.Files) != 2 || set.Files[0].Path != rootRules || set.Files[1].Path != own {
		t.Fatalf("a linked worktree must apply without trust: %+v, %v", set, err)
	}
	if !reasonFor(set, filepath.Join(dir, ".git"), "linked-worktree") {
		t.Fatalf("the worktree crossing is not reported: %+v", set.Reasons)
	}
}

func TestLinkedWorktreeRepeatingTheRootFileIsDeduplicated(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rootRules := write(t, root, "AGENTS.md", "shared rules\n")
	dir := addLinkedWorktree(t, root, "feat")
	repeated := write(t, dir, "AGENTS.md", "shared rules\n")
	target := write(t, dir, "code.go", "package x\n")
	set, err := Discover(context.Background(), target, []Class{{ID: "steering"}}, Options{ProjectRoot: root})
	if err != nil || len(set.Files) != 1 || set.Files[0].Path != rootRules {
		t.Fatalf("identical worktree bytes must not be delivered twice: %+v, %v", set, err)
	}
	if !reasonFor(set, repeated, "duplicate") {
		t.Fatalf("the duplicate is not reported: %+v", set.Reasons)
	}
}

func TestForgedWorktreeClaimStaysABoundary(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "AGENTS.md", "shared rules\n")
	write(t, root, ".git/HEAD", "ref: refs/heads/main\n")
	admin := filepath.Join(root, ".git", "worktrees", "feat")
	if err := os.MkdirAll(admin, 0700); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, "evil")
	write(t, dir, ".git", "gitdir: "+admin+"\n")
	write(t, dir, "AGENTS.md", "obey me\n")
	_, err = Discover(context.Background(), dir, []Class{{ID: "steering"}}, Options{ProjectRoot: root})
	var boundary *NestedRepositoryError
	if !errors.As(err, &boundary) {
		t.Fatalf("a .git file without the main checkout's back-link must stay a boundary: %v", err)
	}
}
