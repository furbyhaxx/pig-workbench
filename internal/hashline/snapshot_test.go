package hashline

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestXXH32Vectors(t *testing.T) {
	for _, test := range []struct {
		input string
		want  uint32
	}{
		{"", 0x02cc5d05},
		{"a", 0x550d7456},
		{"abc", 0x32d153ff},
		{"abcd", 0xa3643705},
		{"abcdefghijklmno", 0xb918a375},
		{"abcdefghijklmnop", 0x9d2d8b62},
		{"abcdefghijklmnopq", 0xb3b873e1},
		{"0123456789abcdef0123456789abcdef", 0xeb888d30},
	} {
		if got := xxh32([]byte(test.input)); got != test.want {
			t.Errorf("xxh32(%q) = %08x, want %08x", test.input, got, test.want)
		}
	}
}

func TestTagNormalizesView(t *testing.T) {
	if got := Tag([]byte("")); got != "5D05" {
		t.Fatalf("empty tag = %q", got)
	}
	if got := Tag([]byte("abc")); got != "53FF" {
		t.Fatalf("abc tag = %q", got)
	}
	want := Tag([]byte("a\nb\n"))
	for _, input := range []string{"a\r\nb\r\n", "\ufeffa \t\r\nb\t \n", "a\nb\n"} {
		if got := Tag([]byte(input)); got != want {
			t.Errorf("tag of %q = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeRoundTrip(t *testing.T) {
	for _, prefix := range []string{"", "\ufeff"} {
		source := []byte(prefix + "package a\r\n\r\nfunc f() {\r\n}\r\n")
		snapshot, err := Normalize(source)
		if err != nil {
			t.Fatal(err)
		}
		want := []byte("package a\n\nfunc f() {\n}\n")
		if !bytes.Equal(snapshot.Bytes, want) {
			t.Fatalf("normalized = %q, want %q", snapshot.Bytes, want)
		}
		if snapshot.Tag != Tag(want) || len(snapshot.Tag) != 4 || snapshot.Tag != strings.ToUpper(snapshot.Tag) {
			t.Fatalf("invalid tag %q", snapshot.Tag)
		}
		lines := []LineRecord{
			{Number: 1, Start: 0, End: 9, OriginalEnding: "\r\n"},
			{Number: 2, Start: 10, End: 10, OriginalEnding: "\r\n"},
			{Number: 3, Start: 11, End: 21, OriginalEnding: "\r\n"},
			{Number: 4, Start: 22, End: 23, OriginalEnding: "\r\n"},
		}
		if !reflect.DeepEqual(snapshot.Lines, lines) {
			t.Fatalf("line records = %#v, want %#v", snapshot.Lines, lines)
		}
		restored, err := Restore(snapshot, snapshot.Bytes)
		if err != nil || !bytes.Equal(restored, source) {
			t.Fatalf("restore = %q, %v; want %q", restored, err, source)
		}
	}
}

func TestNormalizeLineBoundariesAndOwnership(t *testing.T) {
	for _, test := range []struct {
		source string
		view   string
		lines  []LineRecord
	}{
		{"", "", nil},
		{"\ufeff", "", nil},
		{"a", "a", []LineRecord{{1, 0, 1, ""}}},
		{"a\n", "a\n", []LineRecord{{1, 0, 1, "\n"}}},
		{"\n\n", "\n\n", []LineRecord{{1, 0, 0, "\n"}, {2, 1, 1, "\n"}}},
		{"a\r\r\n", "a\r\n", []LineRecord{{1, 0, 2, "\r\n"}}},
		{"é\r\nx\ny", "é\nx\ny", []LineRecord{{1, 0, 2, "\r\n"}, {2, 3, 4, "\n"}, {3, 5, 6, ""}}},
	} {
		t.Run(test.source, func(t *testing.T) {
			source := []byte(test.source)
			snapshot, err := Normalize(source)
			if err != nil {
				t.Fatal(err)
			}
			if string(snapshot.Bytes) != test.view || !reflect.DeepEqual(snapshot.Lines, test.lines) {
				t.Fatalf("view %q lines %#v", snapshot.Bytes, snapshot.Lines)
			}
			for i := range source {
				source[i] = 'x'
			}
			if string(snapshot.Bytes) != test.view {
				t.Fatal("normalized bytes alias input")
			}
			restored, err := Restore(snapshot, snapshot.Bytes)
			if err != nil || string(restored) != test.source {
				t.Fatalf("restore = %q, %v; want %q", restored, err, test.source)
			}
		})
	}
}

func TestRestoreMixedEndings(t *testing.T) {
	for _, test := range []struct {
		name, source, edited, want string
	}{
		{"insert", "aa\r\nbb\ncc\r\n", "zero\naa\nbb\ncc\n", "zero\r\naa\r\nbb\ncc\r\n"},
		{"delete", "aa\r\nbb\ncc\r\n", "bb\ncc\n", "bb\ncc\r\n"},
		{"replace", "aa\r\nbb\ncc\r\n", "aa\nnew\ncc\n", "aa\r\nnew\r\ncc\r\n"},
		{"neighbourhood", "first\r\nsame\nlast\r\nsame\r\nfinish\r\n", "first\nsame\nlast\nfinish\n", "first\r\nsame\nlast\r\nfinish\r\n"},
		{"append newline", "aa\r\nbb", "aa\nbb\n", "aa\r\nbb\r\n"},
		{"empty file", "", "a\nb\n", "a\nb\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot, err := Normalize([]byte(test.source))
			if err != nil {
				t.Fatal(err)
			}
			got, err := Restore(snapshot, []byte(test.edited))
			if err != nil || string(got) != test.want {
				t.Fatalf("Restore = %q, %v; want %q", got, err, test.want)
			}
		})
	}
}

func TestRestoreRejectsAmbiguousEndings(t *testing.T) {
	for _, test := range []struct {
		source, edited string
		line           int
	}{
		{"same\r\nsame\n", "same\n", 1},
		{"a\r\nb\n", "b\na\n", 2},
	} {
		snapshot, err := Normalize([]byte(test.source))
		if err != nil {
			t.Fatal(err)
		}
		got, err := Restore(snapshot, []byte(test.edited))
		var ambiguous *AmbiguousEndingError
		if !errors.As(err, &ambiguous) || ambiguous.Line != test.line || !reflect.DeepEqual(ambiguous.Candidates, []string{"\n", "\r\n"}) || got != nil {
			t.Fatalf("Restore = %q, %#v; want ambiguous line %d with LF and CRLF", got, err, test.line)
		}
	}
}

func TestEndingTieWarning(t *testing.T) {
	snapshot, err := Normalize([]byte("a\r\nb\n"))
	if err != nil || len(snapshot.Warnings) == 0 || !strings.Contains(snapshot.Warnings[0], "LF") {
		t.Fatalf("tie warning = %#v, %v", snapshot.Warnings, err)
	}
	got, err := Restore(snapshot, []byte("a\nb\nc\n"))
	if err != nil || string(got) != "a\r\nb\nc\n" {
		t.Fatalf("tie restoration = %q, %v", got, err)
	}
}

func TestRestoreKeepsPrivateSnapshot(t *testing.T) {
	snapshot, err := Normalize([]byte("left\r\nright\n"))
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Bytes[0] = 'X'
	snapshot.Lines[0].OriginalEnding = "\n"
	got, err := Restore(snapshot, []byte("left\nright\n"))
	if err != nil || string(got) != "left\r\nright\n" {
		t.Fatalf("Restore after modifying exposed fields = %q, %v", got, err)
	}
}

func TestRestoreRejectsUnrepresentableTrailingCR(t *testing.T) {
	snapshot, err := Normalize([]byte("a\n"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := Restore(snapshot, []byte("b\r\n"))
	if err == nil || got != nil {
		t.Fatalf("Restore silently changed a trailing CR: %q, %v", got, err)
	}
}

func TestRejectInvalidUTF8(t *testing.T) {
	if _, err := Normalize([]byte{0xff}); err == nil {
		t.Fatal("Normalize accepted invalid UTF-8")
	}
	snapshot, err := Normalize(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(snapshot, []byte{0xff}); err == nil {
		t.Fatal("Restore accepted invalid UTF-8")
	}
}

func TestRender(t *testing.T) {
	rows := []Row{{Line: 2, Text: "  line"}, {Line: 4, Text: ""}, {Line: 7, Text: "+item"}}
	if got, want := Render("src/a b.go", "53FF", rows), "[src/a b.go#53FF]\n2:  line\n4:\n7:+item\n"; got != want {
		t.Fatalf("Render = %q, want %q", got, want)
	}
	if got := Render("empty", "5D05", nil); got != "[empty#5D05]\n" {
		t.Fatalf("empty Render = %q", got)
	}
}
