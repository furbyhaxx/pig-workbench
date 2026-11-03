package resource

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLiteralFilesAndURIPunctuation(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"file:17", "comma, file:raw", "spaces here", "file:1,3"} {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("data"), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := ParsePath(root, name, "")
		if err != nil {
			t.Fatal(err)
		}
		if got.Path != path || got.Selection.Explicit {
			t.Fatalf("literal path split: %+v", got)
		}
		got, err = ParsePath(root, FileURI(path), "")
		if err != nil {
			t.Fatal(err)
		}
		if got.Path != path || got.Selection.Explicit {
			t.Fatalf("literal file URI split: %+v", got)
		}
	}
	target, err := ParsePath(root, "ssh://dev@host:2222/a:20-40?x=a:b#z", "")
	if err != nil {
		t.Fatal(err)
	}
	if target.URI != "ssh://dev@host:2222/a?x=a:b#z" || target.Selection.Ranges[0] != (Range{20, 40}) {
		t.Fatalf("port/query damaged: %+v", target)
	}
	target, err = ParsePath(root, "https://host/a%3A20?x=1,2:5#frag:8", "")
	if err != nil {
		t.Fatal(err)
	}
	if target.Selection.Explicit || target.URI != "https://host/a%3A20?x=1,2:5#frag:8" {
		t.Fatalf("opaque punctuation parsed: %+v", target)
	}
	if _, err := ParsePath(root, "a:20-40", ":1"); err == nil {
		t.Fatal("mixed selectors accepted")
	}
	if _, err := ParsePath(root, "file://", ""); err == nil {
		t.Fatal("file URI without absolute path accepted")
	}
	if _, err := ParsePath(root, "file://otherhost/a", ""); err == nil {
		t.Fatal("foreign file authority accepted")
	}
}
