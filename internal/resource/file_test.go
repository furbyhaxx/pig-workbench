package resource

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileProviderDecodesURIExactlyOnce(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a b.txt", "a#b.txt", "a%20b.txt", "a?b.txt", "ü.txt"} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, name)
			if err := os.WriteFile(path, []byte(name), 0600); err != nil {
				t.Fatal(err)
			}
			uri := FileURI(path)
			selection, err := ParseSelector(":raw")
			if err != nil {
				t.Fatal(err)
			}
			result, err := (FileProvider{}).Read(context.Background(), ReadRequest{URI: uri, Requested: name, Selector: selection, Budget: Budget{Bytes: 4096, Lines: 100}})
			if err != nil {
				t.Fatal(err)
			}
			if result.Content != name {
				t.Fatalf("wrong file: %q", result.Content)
			}
			if result.Facts.RequestedURI != name {
				t.Fatalf("requested spelling lost: %q", result.Facts.RequestedURI)
			}
		})
	}
}

func TestFileProviderRejectsNonlocalOrMalformedURI(t *testing.T) {
	for _, uri := range []string{"file://otherhost/etc/passwd", "file:///tmp/a?query=1", "file:///tmp/a#fragment", "file:///tmp/%00", "file:///tmp/%zz", "https://host/file"} {
		if _, err := localPath(uri); err == nil {
			t.Errorf("accepted %q", uri)
		}
	}
}
