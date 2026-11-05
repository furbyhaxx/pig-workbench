package hashline

import "testing"

func TestCopiedReadPreservesTerminalNewlineChoice(t *testing.T) {
	for _, source := range []string{"one\ntwo\n", "one\ntwo", "\n", ""} {
		body := "1:one\n2:two\n"
		if source == "\n" {
			body = "1:\n"
		}
		if source == "" {
			body = ""
		}
		copied := Header("file:///source", Tag([]byte(source))) + "\n" + body
		actual, recognized, err := UnwrapRead(copied)
		if err != nil || !recognized || actual.Content != source {
			t.Fatalf("unwrap %q: %q %v %v", source, actual, recognized, err)
		}
	}
}

func TestCopiedReadRefusesTruncatedOrEditedPresentation(t *testing.T) {
	header := Header("file:///source", Tag([]byte("one\ntwo\n"))) + "\n"
	for _, body := range []string{"2:two\n", "1:one\n… incomplete\n", "1:changed\n2:two\n", "1:one\n"} {
		if _, recognized, err := UnwrapRead(header + body); !recognized || err == nil {
			t.Fatalf("accepted incomplete/ambiguous copy %q", body)
		}
	}
	ordinary := "[literal text]\n1:literal source\n"
	actual, recognized, err := UnwrapRead(ordinary)
	if err != nil || recognized || actual.Content != ordinary {
		t.Fatal("stripped an ordinary source literal")
	}
}
