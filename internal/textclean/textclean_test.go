package textclean

import "testing"

func TestControlCharactersAreRemovedOrSpacedExceptTheKeptOnes(t *testing.T) {
	text := "a\x1b[31mb\tc\nd\x7fe\u0085f"
	if got := StripControl(text, '\n', '\t'); got != "a[31mb\tc\ndef" {
		t.Errorf("StripControl kept: %q", got)
	}
	if got := StripControl(text); got != "a[31mbcdef" {
		t.Errorf("StripControl: %q", got)
	}
	if got := SpaceControl(text, '\n'); got != "a [31mb c\nd e f" {
		t.Errorf("SpaceControl: %q", got)
	}
	if !HasControl(text) || HasControl("plain ü text") {
		t.Error("HasControl is wrong")
	}
}
