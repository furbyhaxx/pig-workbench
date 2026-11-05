package prompt

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/fstest"
)

func writeTemplate(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newFilesystemLoader(t *testing.T, options FileSystemOptions) *FileSystemLoader {
	t.Helper()
	loader, err := NewFileSystemLoader(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := loader.Close(); err != nil {
			t.Errorf("close loader: %v", err)
		}
	})
	return loader
}

func readTemplate(t *testing.T, loader TemplateLoader, name string) string {
	t.Helper()
	reader, err := loader.Open(name)
	if err != nil {
		t.Fatalf("open %q: %v", name, err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read %q: %v", name, err)
	}
	return string(data)
}

func builtinTemplates() fstest.MapFS {
	return fstest.MapFS{
		"builtin/workbench":        {Data: []byte("root {% block content %}base{% endblock %}")},
		"builtin/partials/base.md": {Data: []byte("builtin partial")},
		"builtin/partials":         {Mode: fs.ModeDir},
		"unreachable/not-prefixed": {Data: []byte("no namespace")},
	}
}

func TestFileSystemLoaderLayerPrecedence(t *testing.T) {
	user, project := t.TempDir(), t.TempDir()
	writeTemplate(t, user, "system.md", "user {{ project_root }}")
	writeTemplate(t, user, "partials/shared.md", "user partial")
	writeTemplate(t, user, "entry.md", `{% include "partials/shared.md" %}`)
	writeTemplate(t, project, "system.md", "project {{ project_root }}")
	writeTemplate(t, project, "include.md", `{% include "partials/shared.md" %}`)
	options := FileSystemOptions{Builtin: builtinTemplates(), UserRoot: user, ProjectRoot: project, TrustProject: true}

	trusted := newTestEngine(t, newFilesystemLoader(t, options), Options{})
	for name, want := range map[string]string{
		"system.md":          "project /work",
		"partials/shared.md": "user partial",
		"entry.md":           "user partial",
		"include.md":         "user partial",
		"builtin/workbench":  "root base",
	} {
		got, err := trusted.Render(context.Background(), name, Data{ProjectRoot: "/work"})
		if err != nil || got.Text != want {
			t.Fatalf("%s: got %q, %v; want %q", name, got.Text, err, want)
		}
	}
	if _, err := newFilesystemLoader(t, options).Open("partials/absent.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing template: got %v; want not exist", err)
	}

	options.TrustProject = false
	untrusted := newTestEngine(t, newFilesystemLoader(t, options), Options{})
	got, err := untrusted.Render(context.Background(), "system.md", Data{ProjectRoot: "/work"})
	if err != nil || got.Text != "user /work" {
		t.Fatalf("untrusted: got %q, %v; want user layer", got.Text, err)
	}
}

func TestFileSystemLoaderBuiltinCannotBeShadowed(t *testing.T) {
	user, project := t.TempDir(), t.TempDir()
	for _, root := range []string{user, project} {
		writeTemplate(t, root, "builtin/workbench", "shadow")
		writeTemplate(t, root, "builtin/missing.md", "shadow")
	}
	writeTemplate(t, project, "system.md", `{% extends "builtin/workbench" %}{% block content %}child{% endblock %}`)
	loader := newFilesystemLoader(t, FileSystemOptions{Builtin: builtinTemplates(), UserRoot: user, ProjectRoot: project, TrustProject: true})

	if got := readTemplate(t, loader, "builtin/workbench"); got != "root {% block content %}base{% endblock %}" {
		t.Fatalf("builtin template: got %q; want builtin layer", got)
	}
	if _, err := loader.Open("builtin/missing.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("unknown builtin name: got %v; want not exist", err)
	}
	engine := newTestEngine(t, loader, Options{})
	got, err := engine.Render(context.Background(), "system.md", Data{})
	if err != nil || got.Text != "root child" {
		t.Fatalf("extends builtin: got %q, %v; want %q", got.Text, err, "root child")
	}
	names := loader.List()
	want := []string{"builtin/partials/base.md", "builtin/workbench", "system.md"}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Fatalf("list: got %v; want %v", names, want)
	}
}

func TestFileSystemLoaderRejectsUnconfinedNames(t *testing.T) {
	user := t.TempDir()
	writeTemplate(t, user, "real.md", "real")
	loader := newFilesystemLoader(t, FileSystemOptions{Builtin: builtinTemplates(), UserRoot: user})
	names := []string{"", ".", "..", "../escape.md", "/etc/passwd", `..\escape.md`, "partials/../../escape.md", `partials\real.md`, "bad\x00name", "sub/"}
	for _, name := range names {
		reader, err := loader.Open(name)
		if err == nil {
			reader.Close()
			t.Fatalf("%q: opened; want rejection", name)
		}
		if errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("%q: got %v; want a rejection, not not-exist", name, err)
		}
	}
	if got := readTemplate(t, loader, "real.md"); got != "real" {
		t.Fatalf("valid name: got %q", got)
	}

	writeTemplate(t, user, "escape.md", `prefix {% include "../secret.md" %} suffix`)
	engine := newTestEngine(t, loader, Options{})
	got, err := engine.Render(context.Background(), "escape.md", Data{})
	if err == nil || got.Text != "" {
		t.Fatalf("traversal include: got %q, %v; want error without output", got.Text, err)
	}
}

func TestFileSystemLoaderSymlinkConfinement(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs elevated privileges on windows")
	}
	outside := t.TempDir()
	writeTemplate(t, outside, "secret.md", "secret")
	user := t.TempDir()
	writeTemplate(t, user, "real.md", "real")
	writeTemplate(t, user, "include.md", `prefix {% include "link.md" %} suffix`)
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(user, "link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", filepath.Base(outside)), filepath.Join(user, "linkdir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.md", filepath.Join(user, "alias.md")); err != nil {
		t.Fatal(err)
	}
	loader := newFilesystemLoader(t, FileSystemOptions{Builtin: builtinTemplates(), UserRoot: user})

	for _, name := range []string{"link.md", "linkdir/secret.md"} {
		reader, err := loader.Open(name)
		if err == nil {
			reader.Close()
			t.Fatalf("%q: followed an escaping symlink", name)
		}
		if errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("%q: got %v; want an escape error", name, err)
		}
	}
	if got := readTemplate(t, loader, "alias.md"); got != "real" {
		t.Fatalf("internal symlink: got %q; want %q", got, "real")
	}
	engine := newTestEngine(t, loader, Options{})
	got, err := engine.Render(context.Background(), "include.md", Data{})
	if err == nil || got.Text != "" {
		t.Fatalf("escaping include: got %q, %v; want error without output", got.Text, err)
	}
}

func TestFileSystemLoaderRejectsDirectories(t *testing.T) {
	user := t.TempDir()
	writeTemplate(t, user, "scripts/run.md", "run")
	loader := newFilesystemLoader(t, FileSystemOptions{Builtin: builtinTemplates(), UserRoot: user})
	for _, name := range []string{"scripts", "builtin/partials"} {
		reader, err := loader.Open(name)
		if err == nil {
			reader.Close()
			t.Fatalf("%q: opened a directory", name)
		}
		if !strings.Contains(err.Error(), "not a regular file") {
			t.Fatalf("%q: got %v; want regular-file rejection", name, err)
		}
	}
}

func TestFileSystemLoaderBoundsInput(t *testing.T) {
	user := t.TempDir()
	writeTemplate(t, user, "at.md", strings.Repeat("a", 16))
	writeTemplate(t, user, "over.md", strings.Repeat("a", 17))
	loader := newFilesystemLoader(t, FileSystemOptions{Builtin: builtinTemplates(), UserRoot: user, MaxFileBytes: 16})
	if _, err := loader.Open("at.md"); err != nil {
		t.Fatalf("at limit: %v", err)
	}
	if _, err := loader.Open("over.md"); err == nil || !strings.Contains(err.Error(), "exceeds 16 bytes") {
		t.Fatalf("over limit: got %v; want byte limit error", err)
	}

	writeTemplate(t, user, "default.md", strings.Repeat("a", defaultMaxFileBytes+1))
	fallback := newFilesystemLoader(t, FileSystemOptions{Builtin: builtinTemplates(), UserRoot: user})
	if _, err := fallback.Open("default.md"); err == nil || !strings.Contains(err.Error(), "exceeds 262144 bytes") {
		t.Fatalf("default limit: got %v; want 256 KiB limit error", err)
	}
}

func TestFileSystemLoaderUnicodeNames(t *testing.T) {
	user := t.TempDir()
	writeTemplate(t, user, "ümlaut/grüße.md", "Grüße {{ project_root }}")
	loader := newFilesystemLoader(t, FileSystemOptions{Builtin: builtinTemplates(), UserRoot: user})
	if got := readTemplate(t, loader, "ümlaut/grüße.md"); got != "Grüße {{ project_root }}" {
		t.Fatalf("unicode template: got %q", got)
	}
	engine := newTestEngine(t, loader, Options{})
	got, err := engine.Render(context.Background(), "ümlaut/grüße.md", Data{ProjectRoot: "/work"})
	if err != nil || got.Text != "Grüße /work" {
		t.Fatalf("unicode render: got %q, %v", got.Text, err)
	}
	if names := loader.List(); len(names) != 3 || names[2] != "ümlaut/grüße.md" {
		t.Fatalf("unicode list: got %v", names)
	}
}

func TestFileSystemLoaderPermissionIsNotMissing(t *testing.T) {
	user := t.TempDir()
	writeTemplate(t, user, "locked.md", "locked")
	path := filepath.Join(user, "locked.md")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(path, 0o644) })
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("permission model does not deny the owner read access")
	}
	loader := newFilesystemLoader(t, FileSystemOptions{Builtin: builtinTemplates(), UserRoot: user})
	_, err := loader.Open("locked.md")
	if !errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("locked template: got %v; want permission, not missing", err)
	}
}

func TestFileSystemLoaderList(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs elevated privileges on windows")
	}
	outside := t.TempDir()
	writeTemplate(t, outside, "secret.md", "secret")
	user, project := t.TempDir(), t.TempDir()
	writeTemplate(t, user, "system.md", "user")
	writeTemplate(t, user, "ümlaut/grüße.md", "Grüße")
	writeTemplate(t, user, "builtin/fake.md", "shadow")
	writeTemplate(t, user, "scripts/run.md", "run")
	if err := os.Symlink(filepath.Join(outside, "secret.md"), filepath.Join(user, "escape.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("system.md", filepath.Join(user, "alias.md")); err != nil {
		t.Fatal(err)
	}
	writeTemplate(t, project, "system.md", "project")
	writeTemplate(t, project, "project-only.md", "only")

	options := FileSystemOptions{Builtin: builtinTemplates(), UserRoot: user, ProjectRoot: project, TrustProject: true}
	names := newFilesystemLoader(t, options).List()
	want := []string{"alias.md", "builtin/partials/base.md", "builtin/workbench", "project-only.md", "scripts/run.md", "system.md", "ümlaut/grüße.md"}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Fatalf("trusted list: got %v; want %v", names, want)
	}

	options.TrustProject = false
	names = newFilesystemLoader(t, options).List()
	want = []string{"alias.md", "builtin/partials/base.md", "builtin/workbench", "scripts/run.md", "system.md", "ümlaut/grüße.md"}
	if strings.Join(names, "|") != strings.Join(want, "|") {
		t.Fatalf("untrusted list: got %v; want %v", names, want)
	}

	missing := newFilesystemLoader(t, FileSystemOptions{Builtin: builtinTemplates(), UserRoot: filepath.Join(t.TempDir(), "absent"), ProjectRoot: filepath.Join(t.TempDir(), "absent"), TrustProject: true})
	want = []string{"builtin/partials/base.md", "builtin/workbench"}
	if names := missing.List(); strings.Join(names, "|") != strings.Join(want, "|") {
		t.Fatalf("missing roots: got %v; want %v", names, want)
	}
}

func TestFileSystemLoaderOptionsValidation(t *testing.T) {
	if _, err := NewFileSystemLoader(FileSystemOptions{UserRoot: t.TempDir()}); err == nil {
		t.Fatal("accepted a loader without builtin templates")
	}
	if _, err := NewFileSystemLoader(FileSystemOptions{Builtin: builtinTemplates(), MaxFileBytes: -1}); err == nil {
		t.Fatal("accepted a negative byte limit")
	}
	file := filepath.Join(t.TempDir(), "file.md")
	writeTemplate(t, filepath.Dir(file), "file.md", "file")
	if _, err := NewFileSystemLoader(FileSystemOptions{Builtin: builtinTemplates(), ProjectRoot: file, TrustProject: true}); err == nil {
		t.Fatal("accepted a project root that is a file")
	}

	untrusted := newFilesystemLoader(t, FileSystemOptions{Builtin: builtinTemplates(), UserRoot: filepath.Join(t.TempDir(), "absent"), ProjectRoot: file})
	if _, err := untrusted.Open("file.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("untrusted project root: got %v; want not exist", err)
	}
}
