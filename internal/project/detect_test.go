package project

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func requireTestGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
}

func testGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir}, args...)...)
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_TERMINAL_PROMPT=0")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}

func newTestRepository(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	testGit(t, dir, "init", "--quiet")
	testGit(t, dir, "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--quiet", "--allow-empty", "--no-gpg-sign", "-m", "root "+filepath.Base(dir))
}

func testClone(t *testing.T, source, clone string) {
	t.Helper()
	testGit(t, filepath.Dir(source), "clone", "--quiet", source, clone)
}

func testCanonical(t *testing.T, path string) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func testTreeSnapshot(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		paths = append(paths, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	return paths
}

func writeDetectionDocument(t *testing.T, root, contents string) string {
	t.Helper()
	path := filepath.Join(root, ".project", "Project.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDetectRootCommitAcrossClones(t *testing.T) {
	requireTestGit(t)
	root := t.TempDir()
	r := openTestRegistry(t, filepath.Join(root, "agent"))
	source, clone := filepath.Join(root, "source"), filepath.Join(root, "clone")
	newTestRepository(t, source)
	p := testRegistryProject(t, "prj-shared")
	if err := r.Register(p, Location{Host: "local", Path: source, Kind: "main"}); err != nil {
		t.Fatal(err)
	}
	testClone(t, source, clone)
	nested := filepath.Join(clone, "nested")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	traceConfig := filepath.Join(root, "trace-config")
	config := fmt.Sprintf("[trace2]\n  normalTarget = %q\n  eventTarget = %q\n  perfTarget = %q\n", filepath.Join(root, "trace2"), filepath.Join(root, "trace2-events"), filepath.Join(root, "trace2-perf"))
	if err := os.WriteFile(traceConfig, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", traceConfig)
	t.Setenv("GIT_TRACE", filepath.Join(root, "git-trace"))
	t.Setenv("GIT_DIR", filepath.Join(source, ".git"))
	t.Setenv("GIT_WORK_TREE", source)
	before := testTreeSnapshot(t, root)
	detection, err := r.Detect(context.Background(), nested)
	if err != nil || detection.How != "root-commit" || detection.ProjectID != p.ID || detection.Slug != p.Slug || detection.Name != p.Name || detection.Root != testCanonical(t, clone) || detection.RegistryDir != filepath.Join(root, "agent", "projects", p.ID) {
		t.Fatalf("clone detection: %+v, err=%v", detection, err)
	}
	if after := testTreeSnapshot(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("detection changed tree: before=%v after=%v", before, after)
	}
}

func TestDetectProjectFileWithoutGitAndGlobalWithoutWrites(t *testing.T) {
	root := t.TempDir()
	r := openTestRegistry(t, filepath.Join(root, "agent"))
	projectRoot := filepath.Join(root, "plain")
	writeDetectionDocument(t, projectRoot, validDocument)
	nested := filepath.Join(projectRoot, "child")
	if err := os.Mkdir(nested, 0700); err != nil {
		t.Fatal(err)
	}
	bare := filepath.Join(root, "bare")
	if err := os.Mkdir(bare, 0700); err != nil {
		t.Fatal(err)
	}
	before := testTreeSnapshot(t, root)
	t.Setenv("PATH", t.TempDir())
	got, err := r.Detect(context.Background(), nested)
	if err != nil || got.How != "project-file" || got.ProjectID != "prj-abc123" || got.Slug != "workbench" || got.Name != "Workbench" || got.Root != testCanonical(t, projectRoot) {
		t.Fatalf("project file without git: %+v, err=%v", got, err)
	}
	got, err = r.Detect(context.Background(), bare)
	if err != nil || got.How != "global" || got.ProjectID != "global" || got.Slug != "global" || got.Root != testCanonical(t, bare) || got.RegistryDir != filepath.Join(root, "agent", "projects", "global") {
		t.Fatalf("global without git: %+v, err=%v", got, err)
	}
	if after := testTreeSnapshot(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("detection changed tree: before=%v after=%v", before, after)
	}
}

func TestDetectGlobalWithGitDoesNotWrite(t *testing.T) {
	requireTestGit(t)
	root := t.TempDir()
	r := openTestRegistry(t, filepath.Join(root, "agent"))
	bare := filepath.Join(root, "bare")
	if err := os.Mkdir(bare, 0700); err != nil {
		t.Fatal(err)
	}
	before := testTreeSnapshot(t, root)
	got, err := r.Detect(context.Background(), bare)
	if err != nil || got.How != "global" || got.Root != testCanonical(t, bare) {
		t.Fatalf("bare directory: %+v, %v", got, err)
	}
	if after := testTreeSnapshot(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("global detection changed tree: before=%v after=%v", before, after)
	}
}

func TestDetectInvalidProjectFile(t *testing.T) {
	root := t.TempDir()
	r := openTestRegistry(t, filepath.Join(root, "agent"))
	plain := filepath.Join(root, "plain")
	path := writeDetectionDocument(t, plain, "id = 'invalid'\n")
	got, err := r.Detect(context.Background(), plain)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("invalid project document was ignored: %+v, %v", got, err)
	}
}

func TestDetectGitUsesOnlyRepositoryTopDocument(t *testing.T) {
	requireTestGit(t)
	root := t.TempDir()
	r := openTestRegistry(t, filepath.Join(root, "agent"))
	repo := filepath.Join(root, "repo")
	newTestRepository(t, repo)
	nested := filepath.Join(repo, "nested")
	writeDetectionDocument(t, nested, validDocument)
	got, err := r.Detect(context.Background(), nested)
	if err != nil || got.How != "global" || got.Root != testCanonical(t, repo) {
		t.Fatalf("nested document inside git repository must be ignored: %+v, %v", got, err)
	}
	known := testRegistryProject(t, "prj-registered")
	if err := r.Register(known, Location{Host: "local", Path: repo, Kind: "main"}); err != nil {
		t.Fatal(err)
	}
	writeDetectionDocument(t, repo, validDocument)
	got, err = r.Detect(context.Background(), nested)
	if err != nil || got.How != "project-file" || got.ProjectID != "prj-abc123" || got.Root != testCanonical(t, repo) {
		t.Fatalf("repository top document did not take precedence: %+v, %v", got, err)
	}
}

func TestDetectLongestLocalLocation(t *testing.T) {
	root := t.TempDir()
	r := openTestRegistry(t, filepath.Join(root, "agent"))
	base, nested := filepath.Join(root, "checkout"), filepath.Join(root, "checkout", "nested")
	leaf := filepath.Join(nested, "child")
	if err := os.MkdirAll(leaf, 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ id, host, path string }{
		{"prj-parent", "local", base}, {"prj-child", "", nested}, {"prj-otherhost", "other.example.invalid", leaf},
	} {
		if err := r.Register(testRegistryProject(t, tc.id), Location{Host: tc.host, Path: tc.path, Kind: "main"}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := r.Detect(context.Background(), leaf)
	if err != nil || got.How != "path" || got.ProjectID != "prj-child" || got.Root != testCanonical(t, nested) {
		t.Fatalf("longest local match: %+v, %v", got, err)
	}
	sibling := filepath.Join(root, "checkout-sibling")
	if err := os.Mkdir(sibling, 0700); err != nil {
		t.Fatal(err)
	}
	got, err = r.Detect(context.Background(), sibling)
	if err != nil || got.How != "global" {
		t.Fatalf("path prefix matched unrelated directory: %+v, %v", got, err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Register(testRegistryProject(t, "prj-host"), Location{Host: hostname, Path: sibling, Kind: "main"}); err != nil {
		t.Fatal(err)
	}
	got, err = r.Detect(context.Background(), sibling)
	if err != nil || got.How != "path" || got.ProjectID != "prj-host" {
		t.Fatalf("hostname did not match local checkout: %+v, %v", got, err)
	}
}

func TestDetectRemoteAndAmbiguousRoot(t *testing.T) {
	requireTestGit(t)
	root := t.TempDir()
	r := openTestRegistry(t, filepath.Join(root, "agent"))
	source, first, second, target := filepath.Join(root, "source"), filepath.Join(root, "first"), filepath.Join(root, "second"), filepath.Join(root, "target")
	newTestRepository(t, source)
	testClone(t, source, first)
	testClone(t, source, second)
	testClone(t, source, target)
	for _, tc := range []struct{ id, path, remote string }{
		{"prj-first", first, "example.com/owner/first"}, {"prj-second", second, "example.com/owner/second"},
	} {
		p := testRegistryProject(t, tc.id)
		p.Component.DefaultRemote = tc.remote
		if err := r.Register(p, Location{Host: "local", Path: tc.path, Kind: "clone"}); err != nil {
			t.Fatal(err)
		}
	}
	testGit(t, target, "remote", "set-url", "origin", "git@EXAMPLE.COM:Owner/Second.git")
	got, err := r.Detect(context.Background(), target)
	if err != nil || got.How != "root-commit" || got.ProjectID != "prj-second" {
		t.Fatalf("remote did not disambiguate root: %+v, %v", got, err)
	}
	testGit(t, target, "remote", "set-url", "origin", "git@example.com:owner/unknown.git")
	if _, err := r.Detect(context.Background(), target); err == nil || !strings.Contains(err.Error(), "prj-first") || !strings.Contains(err.Error(), "prj-second") {
		t.Fatalf("ambiguous root not reported: %v", err)
	}
	unrelated := filepath.Join(root, "unrelated")
	newTestRepository(t, unrelated)
	testGit(t, unrelated, "remote", "add", "origin", "https://EXAMPLE.com/Owner/Second.git")
	got, err = r.Detect(context.Background(), unrelated)
	if err != nil || got.How != "remote" || got.ProjectID != "prj-second" {
		t.Fatalf("remote fallback: %+v, %v", got, err)
	}
}

func TestDetectRemoteFromRegisteredLocation(t *testing.T) {
	requireTestGit(t)
	root := t.TempDir()
	r := openTestRegistry(t, filepath.Join(root, "agent"))
	known, other := filepath.Join(root, "known"), filepath.Join(root, "other")
	newTestRepository(t, known)
	newTestRepository(t, other)
	testGit(t, known, "remote", "add", "origin", "git@example.com:Owner/Repo.git")
	testGit(t, other, "remote", "add", "origin", "https://example.com/owner/repo")
	p := testRegistryProject(t, "prj-observed")
	p.Component.DefaultRemote = ""
	if err := r.Register(p, Location{Host: "local", Path: known, Kind: "main"}); err != nil {
		t.Fatal(err)
	}
	got, err := r.Detect(context.Background(), other)
	if err != nil || got.How != "remote" || got.ProjectID != p.ID {
		t.Fatalf("observed remote did not match: %+v, %v", got, err)
	}
}

func TestDetectDoesNotGuessOnInvalidRecordsOrPathConflicts(t *testing.T) {
	root := t.TempDir()
	r := openTestRegistry(t, filepath.Join(root, "agent"))
	location := filepath.Join(root, "location")
	if err := os.Mkdir(location, 0700); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"prj-first", "prj-second"} {
		if err := r.Register(testRegistryProject(t, id), Location{Host: "local", Path: location, Kind: "main"}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := r.Detect(context.Background(), location); err == nil || !strings.Contains(err.Error(), "prj-first") || !strings.Contains(err.Error(), "prj-second") {
		t.Fatalf("ambiguous paths were not reported: %v", err)
	}
	bad := filepath.Join(root, "agent", "projects", "prj-invalid")
	if err := os.Mkdir(bad, 0700); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(bad, "project.toml")
	if err := os.WriteFile(badPath, []byte("name = [\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Detect(context.Background(), location); err == nil || !strings.Contains(err.Error(), badPath) {
		t.Fatalf("invalid registry record was not reported: %v", err)
	}
}

func TestResolveRemote(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"git@GitHub.COM:Owner/Repo.git", "github.com/owner/repo"},
		{"https://GITHUB.COM/Owner/Repo.git", "github.com/owner/repo"},
		{"https://github.com/owner/repo", "github.com/owner/repo"},
		{"ssh://git@GitHub.com:2222/Owner/Repo.GIT", "github.com/owner/repo"},
		{"github.com/OWNER/REPO", "github.com/owner/repo"},
		{"ssh://git@example.com/group/subgroup/repo.git", "example.com/group/subgroup/repo"},
		{"", ""}, {"/tmp/owner/repo", ""}, {"../owner/repo", ""},
		{"file:///tmp/owner/repo", ""}, {"https://example.com/repo", ""},
		{"https://example.com/owner/repo?query=1", ""}, {"git@example.com:../repo.git", ""},
		{"https://example.com//owner/repo", ""}, {"git@[example.com:owner/repo", ""},
		{"not a remote", ""}, {"https:///owner/repo", ""},
	} {
		t.Run(fmt.Sprintf("%q", tc.raw), func(t *testing.T) {
			if got := resolveRemote(tc.raw); got != tc.want {
				t.Fatalf("resolveRemote(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

func TestDetectHonorsCancellation(t *testing.T) {
	root := t.TempDir()
	r := openTestRegistry(t, filepath.Join(root, "agent"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := r.Detect(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context: %v", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("slow git fixture requires a POSIX shell")
	}
	fakeBin := t.TempDir()
	fakeGit := filepath.Join(fakeBin, "git")
	if err := os.WriteFile(fakeGit, []byte("#!/bin/sh\nexec /bin/sleep 10\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin)
	ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := r.Detect(ctx, root); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("running git ignored cancellation: %v", err)
	}
}
