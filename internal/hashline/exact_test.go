package hashline

import (
	"bytes"
	"testing"
)

func TestExactEditsUseOriginalSourceAndPreserveBytes(t *testing.T) {
	source := []byte("\xef\xbb\xbfone\r\ntwo\r\nthree")
	result, changes, err := ReplaceExact(source, []TextEdit{{OldText: "three", NewText: "last"}, {OldText: "one", NewText: "two"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(result) != "\xef\xbb\xbftwo\r\ntwo\r\nlast" || len(changes) != 2 {
		t.Fatalf("wrong staging: %q %+v", result, changes)
	}
	if string(source) != "\xef\xbb\xbfone\r\ntwo\r\nthree" {
		t.Fatal("source mutated")
	}
}

func TestExactRefusesMissingAmbiguousAndOverlappingEdits(t *testing.T) {
	for _, edits := range [][]TextEdit{
		{{OldText: "", NewText: "x"}},
		{{OldText: "absent", NewText: "x"}},
		{{OldText: "aa", NewText: "x"}},
		{{OldText: "aaa", NewText: "x"}, {OldText: "aaab", NewText: "y"}},
	} {
		source := []byte("aaab")
		result, _, err := ReplaceExact(source, edits)
		if err == nil || result != nil {
			t.Fatalf("invalid replacements accepted: %+v", edits)
		}
		if !bytes.Equal(source, []byte("aaab")) {
			t.Fatal("error mutated input")
		}
	}
}
