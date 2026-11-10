package host

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestReferencedEnvFilesFindsWhatACallNames(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "work")
	other := filepath.Join(root, "other dir")
	home := filepath.Join(root, "home")
	for _, dir := range []string{cwd, other, home, filepath.Join(other, "nested")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{
		filepath.Join(other, ".env"), filepath.Join(other, ".env.local"), filepath.Join(other, "prod.env"),
		filepath.Join(other, "notes.txt"), filepath.Join(other, "nested", ".env"), filepath.Join(home, ".envrc"),
	} {
		if err := os.WriteFile(file, []byte("API_KEY=value\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	env, local, prod := filepath.Join(other, ".env"), filepath.Join(other, ".env.local"), filepath.Join(other, "prod.env")
	for name, c := range map[string]struct {
		input map[string]any
		want  []string
	}{
		"path with spaces":        {map[string]any{"path": "../other dir/.env"}, []string{env}},
		"path with a selector":    {map[string]any{"path": env + ":1-20"}, []string{env}},
		"file URI":                {map[string]any{"path": "file://" + prod}, []string{prod}},
		"paths list":              {map[string]any{"paths": []any{prod, filepath.Join(other, "notes.txt")}}, []string{prod}},
		"directory":               {map[string]any{"path": other, "pattern": "KEY"}, []string{env, local}},
		"shell words":             {map[string]any{"command": "cd /tmp && cat '" + prod + "' | grep KEY"}, []string{prod}},
		"shell assignment":        {map[string]any{"command": "env $(cat x) FILE=\"" + prod + "\" run"}, []string{prod}},
		"call cwd":                {map[string]any{"command": "cat prod.env", "cwd": other}, []string{prod, env, local}},
		"home":                    {map[string]any{"command": "source ~/.envrc"}, []string{filepath.Join(home, ".envrc")}},
		"other scheme":            {map[string]any{"path": "https://example.com/.env"}, nil},
		"ordinary file":           {map[string]any{"path": filepath.Join(other, "notes.txt")}, nil},
		"missing file":            {map[string]any{"path": filepath.Join(other, "gone.env")}, nil},
		"nested argument objects": {map[string]any{"options": map[string]any{"files": []any{prod}}}, []string{prod}},
	} {
		if got := referencedEnvFiles(cwd, home, c.input); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %q, want %q", name, got, c.want)
		}
	}
}

func TestReferencedEnvFilesIsBounded(t *testing.T) {
	root := t.TempDir()
	for i := range 40 {
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf(".env.%02d", i)), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if got := referencedEnvFiles(root, "", map[string]any{"path": "."}); len(got) != maxReferencedEnvFiles {
		t.Fatalf("got %d files, want the cap %d", len(got), maxReferencedEnvFiles)
	}
	var words []any
	for range maxReferencedWords {
		words = append(words, "filler")
	}
	if got := referencedEnvFiles(root, "", map[string]any{"args": append(words, ".env.00")}); len(got) != 0 {
		t.Fatalf("a word past the cap was examined: %q", got)
	}
}
