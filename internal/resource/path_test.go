package resource

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestSelectorOnANameAuthorityURIWithoutAPath(t *testing.T) {
	for _, c := range []struct{ input, uri, selector string }{
		{"skill://toml:1-3", "skill://toml", "1-3"},
		{"skill://toml:5", "skill://toml", "5"},
		{"out://ab12:raw", "out://ab12", "raw"},
		{"skill://toml", "skill://toml", ""},
	} {
		target, err := ParsePath(t.TempDir(), c.input, "")
		if err != nil {
			t.Fatalf("%s: %v", c.input, err)
		}
		want, err := ParseSelector(c.selector)
		if c.selector == "" {
			want, err = Selection{}, nil
		}
		if err != nil {
			t.Fatal(err)
		}
		if target.URI != c.uri || !reflect.DeepEqual(target.Selection, want) {
			t.Errorf("%s = %q %+v, want %q %+v", c.input, target.URI, target.Selection, c.uri, want)
		}
	}
	if target, err := ParsePath(t.TempDir(), "https://example.test:8080", ""); err != nil || target.URI != "https://example.test:8080/" {
		t.Fatalf("a port on a network scheme must stay a port: %+v, %v", target, err)
	}
	if _, err := ParsePath(t.TempDir(), "skill://toml:1-3", "5"); err == nil {
		t.Fatal("a suffix and an explicit selector were both accepted")
	}
}
