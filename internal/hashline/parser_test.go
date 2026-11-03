package hashline

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const crossFileExample = "[src/old.go#A1B2]\nCUT 10* @helper\n[src/new.go#C3D4]\nPUT >20 @helper\nPUT >$:\n+\n+// end\n"

func TestParseCrossFileExample(t *testing.T) {
	doc, err := Parse(crossFileExample)
	if err != nil {
		t.Fatal(err)
	}
	want := Document{Sections: []Section{
		{Path: "src/old.go", Tag: "A1B2", Ops: []Op{{Kind: CUT_BLOCK, Line: 10, Block: true, Register: "helper"}}},
		{Path: "src/new.go", Tag: "C3D4", Ops: []Op{
			{Kind: PUT_FROM_REGISTER, Target: PUT_INSERT_AFTER, Line: 20, Register: "helper"},
			{Kind: PUT_INSERT_AFTER, Payload: []string{"", "// end"}},
		}},
	}}
	if !reflect.DeepEqual(doc, want) {
		t.Fatalf("Parse = %#v, want %#v", doc, want)
	}
	formatted, err := Format(doc)
	if err != nil || formatted != crossFileExample {
		t.Fatalf("Format = %q, %v", formatted, err)
	}
	if !doc.Sections[1].Ops[1].InsertAfterEOF() {
		t.Fatal("PUT >$: was not marked as EOF")
	}
}

func TestParseAllCanonicalOperations(t *testing.T) {
	input := "[file#ABCD]\n" +
		"PUT 2.=4:\n+replacement\n" +
		"PUT <1:\n+head\n" +
		"PUT >5:\n+after\n" +
		"PUT 6*:\n+block\n" +
		"PUT >7*:\n+sibling\n" +
		"CUT 8.=10\nCUT 11.=12 @range\nCUT 13*\nCUT 14* @block\n" +
		"PUT <15 @clip\nPUT >16 @clip\nPUT >17* @clip\n" +
		"PUT 18.=19 @clip\nPUT 20* @clip\n" +
		"PUT <21\nPUT >22\nPUT >23*\n" +
		"REM\nMV target\nMV target#part\nMV \"target path\"\n"
	want := []Op{
		{Kind: PUT_RANGE, Line: 2, End: 4, Payload: []string{"replacement"}},
		{Kind: PUT_INSERT_BEFORE, Line: 1, Payload: []string{"head"}},
		{Kind: PUT_INSERT_AFTER, Line: 5, Payload: []string{"after"}},
		{Kind: PUT_BLOCK, Line: 6, Block: true, Payload: []string{"block"}},
		{Kind: PUT_BLOCK_AFTER, Line: 7, Block: true, Payload: []string{"sibling"}},
		{Kind: CUT_RANGE, Line: 8, End: 10},
		{Kind: CUT_RANGE, Line: 11, End: 12, Register: "range"},
		{Kind: CUT_BLOCK, Line: 13, Block: true},
		{Kind: CUT_BLOCK, Line: 14, Block: true, Register: "block"},
		{Kind: PUT_FROM_REGISTER, Target: PUT_INSERT_BEFORE, Line: 15, Register: "clip"},
		{Kind: PUT_FROM_REGISTER, Target: PUT_INSERT_AFTER, Line: 16, Register: "clip"},
		{Kind: PUT_FROM_REGISTER, Target: PUT_BLOCK_AFTER, Line: 17, Block: true, Register: "clip"},
		{Kind: PUT_FROM_REGISTER, Target: PUT_RANGE, Line: 18, End: 19, Register: "clip"},
		{Kind: PUT_FROM_REGISTER, Target: PUT_BLOCK, Line: 20, Block: true, Register: "clip"},
		{Kind: PUT_FROM_REGISTER, Target: PUT_INSERT_BEFORE, Line: 21},
		{Kind: PUT_FROM_REGISTER, Target: PUT_INSERT_AFTER, Line: 22},
		{Kind: PUT_FROM_REGISTER, Target: PUT_BLOCK_AFTER, Line: 23, Block: true},
		{Kind: REM},
		{Kind: MV, Dest: "target"},
		{Kind: MV, Dest: "target#part"},
		{Kind: MV, Dest: "target path"},
	}
	doc, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Sections) != 1 || !reflect.DeepEqual(doc.Sections[0].Ops, want) {
		t.Fatalf("operations = %#v, want %#v", doc.Sections, want)
	}
	if got, err := Format(doc); err != nil || got != input {
		t.Fatalf("round trip = %q, %v", got, err)
	}
}

func TestPayloadRowsAndFinalNewline(t *testing.T) {
	for _, suffix := range []string{"", "\n"} {
		input := "[f#1234]\nPUT >$:\n+text\n+\n+- item\n++ item" + suffix
		doc, err := Parse(input)
		if err != nil {
			t.Fatal(err)
		}
		if got := doc.Sections[0].Ops[0].Payload; !reflect.DeepEqual(got, []string{"text", "", "- item", "+ item"}) {
			t.Fatalf("payload = %#v", got)
		}
		if doc.NoFinalNewline != (suffix == "") {
			t.Fatalf("NoFinalNewline = %v", doc.NoFinalNewline)
		}
		if got, err := Format(doc); err != nil || got != input {
			t.Fatalf("round trip = %q, %v; want %q", got, err, input)
		}
	}
}

func TestPathsAndQuotedHeaders(t *testing.T) {
	input := "[\"dir/my #file.go\"#aBcD]\nMV \"new #path.go\"\n[other#0000]\nREM\n[\"dir/my #file.go\"#EEEE]\nREM\n[other#0000]\nREM\n"
	doc, err := Parse(input)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := doc.Paths(), []string{"dir/my #file.go", "other"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Paths = %#v, want %#v", got, want)
	}
	if !doc.Sections[0].Quoted || doc.Sections[0].Header() != "[\"dir/my #file.go\"#aBcD]" || doc.Sections[0].Ops[0].Dest != "new #path.go" {
		t.Fatalf("quoted section = %#v", doc.Sections[0])
	}
	if got, err := Format(doc); err != nil || got != input {
		t.Fatalf("round trip = %q, %v", got, err)
	}
}

func TestFormatHandBuiltDocument(t *testing.T) {
	doc := Document{Sections: []Section{
		{Path: "src/old.go", Tag: "A1B2", Ops: []Op{{Kind: CUT_BLOCK, Line: 10, Block: true, Register: "helper"}}},
		{Path: "src/new.go", Tag: "C3D4", Ops: []Op{
			{Kind: PUT_FROM_REGISTER, Target: PUT_INSERT_AFTER, Line: 20, Register: "helper"},
			{Kind: PUT_INSERT_AFTER, Payload: []string{"", "// end"}},
		}},
	}}
	if got, err := Format(doc); err != nil || got != crossFileExample {
		t.Fatalf("hand-built format = %q, %v", got, err)
	}
	doc.NoFinalNewline = true
	if got, err := Format(doc); err != nil || got != strings.TrimSuffix(crossFileExample, "\n") {
		t.Fatalf("hand-built format without final newline = %q, %v", got, err)
	}
}

func TestParseErrorsNameOffendingLine(t *testing.T) {
	for _, test := range []struct {
		name, input string
		line        int
	}{
		{"empty", "", 1},
		{"missing header", "REM\n", 1},
		{"short tag", "[f#ABC]\nREM\n", 1},
		{"nonhex tag", "[f#ABCZ]\nREM\n", 1},
		{"unquoted hash", "[x#y#ABCD]\nREM\n", 1},
		{"unclosed header quote", "[\"bad#ABCD]\nREM\n", 1},
		{"unknown operation", "[f#ABCD]\nWHAT\n", 2},
		{"register payload", "[f#ABCD]\nPUT >3 @name\n+unexpected\n", 3},
		{"register EOF anchor", "[f#ABCD]\nPUT >$ @name\n", 2},
		{"missing payload", "[f#ABCD]\nPUT >3:\nREM\n", 2},
		{"invalid single line", "[f#ABCD]\nPUT 2: \n", 2},
		{"zero line", "[f#ABCD]\nPUT <0:\n+x\n", 2},
		{"negative line", "[f#ABCD]\nCUT -1.=2\n", 2},
		{"overflow line", "[f#ABCD]\nCUT 9999999999999999999999999.=3\n", 2},
		{"reversed range", "[f#ABCD]\nCUT 5.=4\n", 2},
		{"empty destination", "[f#ABCD]\nMV\n", 2},
		{"unclosed destination quote", "[f#ABCD]\nMV \"bad path\n", 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Parse(test.input)
			var parseErr *ParseError
			if !errors.As(err, &parseErr) || parseErr.Line != test.line || parseErr.Message == "" {
				t.Fatalf("Parse error = %#v; want ParseError at line %d", err, test.line)
			}
		})
	}
}

func FuzzParseFormat(f *testing.F) {
	f.Add(crossFileExample)
	f.Add("[\"src/a #b\"#abcd]\nPUT <1:\n+hello")
	f.Add("[f#1234]\nPUT >2 @reg\n+invalid\n")
	f.Fuzz(func(t *testing.T, input string) {
		doc, err := Parse(input)
		if err != nil {
			return
		}
		formatted, err := Format(doc)
		if err != nil || formatted != input {
			t.Fatalf("Format(Parse(%q)) = %q, %v", input, formatted, err)
		}
	})
}

func TestFormatRejectsInvalidOperations(t *testing.T) {
	for _, op := range []Op{
		{Kind: PUT_FROM_REGISTER, Register: "x"},
		{Kind: PUT_FROM_REGISTER, Target: PUT_FROM_REGISTER, Line: 1, Register: "x"},
		{Kind: PUT_INSERT_BEFORE, Line: 1, Target: PUT_RANGE, Payload: []string{"x"}},
		{Kind: PUT_INSERT_BEFORE, Payload: []string{"x"}},
		{Kind: CUT_RANGE, Line: 2, End: 1},
		{Kind: PUT_FROM_REGISTER, Target: PUT_INSERT_AFTER, Line: 2, Register: "x", Payload: []string{"x"}},
		{Kind: PUT_FROM_REGISTER, Target: PUT_INSERT_AFTER, Register: "x"},
	} {
		doc := Document{Sections: []Section{{Path: "file", Tag: "1234", Ops: []Op{op}}}}
		_, err := Format(doc)
		var parseErr *ParseError
		if !errors.As(err, &parseErr) || parseErr.Line != 2 {
			t.Errorf("Format(%+v) error = %#v; want ParseError at line 2", op, err)
		}
	}
}
