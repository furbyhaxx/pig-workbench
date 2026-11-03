package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/hashline"
)

func TestExistingRevisionIncludesBytesBeyondSnapshotLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large")
	source := append(bytes.Repeat([]byte{'x'}, 4<<20), []byte("tail-a")...)
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	_, revision, err := readExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	if revision != hashline.SourceRevision(source) {
		t.Fatal("revision omits file tail")
	}
	source[len(source)-1] = 'b'
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	_, changed, err := readExisting(path)
	if err != nil {
		t.Fatal(err)
	}
	if revision == changed {
		t.Fatal("tail modification retained same revision")
	}
}
