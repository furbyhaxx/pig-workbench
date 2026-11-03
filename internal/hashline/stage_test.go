package hashline

import (
	"errors"
	"reflect"
	"testing"
)

func stageSnapshot(t *testing.T, source string) Normalized {
	t.Helper()
	snapshot, err := Normalize([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestStageLinesOriginalCoordinatesAndTouched(t *testing.T) {
	source := stageSnapshot(t, "1\n2\n3\n4\n5\n6\n7\n8\n9\n")
	first := Op{Kind: PUT_RANGE, Line: 2, End: 4, Payload: []string{"replacement"}}
	second := Op{Kind: PUT_RANGE, Line: 8, End: 8, Payload: []string{"eight"}}
	for _, ops := range [][]Op{{first, second}, {second, first}} {
		result, err := StageLines(source, ops)
		if err != nil {
			t.Fatal(err)
		}
		if got, want := string(result.Bytes), "1\nreplacement\n5\n6\n7\neight\n9\n"; got != want {
			t.Fatalf("StageLines(%+v) = %q, want %q", ops, got, want)
		}
		if want := []LineSpan{{Start: 2, End: 4}, {Start: 8, End: 8}}; !reflect.DeepEqual(result.Touched, want) {
			t.Fatalf("touched = %#v, want %#v", result.Touched, want)
		}
	}
}

func TestStageLinesGapOrderingAndRangeBoundaries(t *testing.T) {
	source := stageSnapshot(t, "a\nb\nc\nd\n")
	ops := []Op{
		{Kind: PUT_INSERT_BEFORE, Line: 2, Payload: []string{"first"}},
		{Kind: PUT_INSERT_AFTER, Line: 3, Payload: []string{"after-first"}},
		{Kind: PUT_RANGE, Line: 2, End: 3, Payload: []string{"replacement"}},
		{Kind: PUT_INSERT_AFTER, Line: 1, Payload: []string{"second", "third"}},
		{Kind: PUT_INSERT_BEFORE, Line: 4, Payload: []string{"after-second"}},
	}
	result, err := StageLines(source, ops)
	if err != nil {
		t.Fatal(err)
	}
	want := "a\nfirst\nsecond\nthird\nreplacement\nafter-first\nafter-second\nd\n"
	if string(result.Bytes) != want || !reflect.DeepEqual(result.Touched, []LineSpan{{Start: 2, End: 3}}) {
		t.Fatalf("StageLines = %q, %#v; want %q, [2..3]", result.Bytes, result.Touched, want)
	}
}

func TestStageLinesEmptySourceAndEndpointGaps(t *testing.T) {
	for _, test := range []struct {
		name, source, want string
		ops                []Op
	}{
		{"empty BOF", "", "start\n", []Op{{Kind: PUT_INSERT_BEFORE, Line: 1, Payload: []string{"start"}}}},
		{"empty EOF", "", "end\n", []Op{{Kind: PUT_INSERT_AFTER, Line: 0, Payload: []string{"end"}}}},
		{"same empty gap", "", "end\nstart\n\n", []Op{{Kind: PUT_INSERT_AFTER, Payload: []string{"end"}}, {Kind: PUT_INSERT_BEFORE, Line: 1, Payload: []string{"start", ""}}}},
		{"EOF aliases", "a\n", "a\nfirst\nsecond\n", []Op{{Kind: PUT_INSERT_AFTER, Line: 0, Payload: []string{"first"}}, {Kind: PUT_INSERT_AFTER, Line: 1, Payload: []string{"second"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := StageLines(stageSnapshot(t, test.source), test.ops)
			if err != nil || string(result.Bytes) != test.want || len(result.Touched) != 0 {
				t.Fatalf("StageLines = %q, %#v, %v; want %q and no touched spans", result.Bytes, result.Touched, err, test.want)
			}
		})
	}
}

func TestStageLinesTerminalNewline(t *testing.T) {
	for _, test := range []struct {
		name, source, want string
		ops                []Op
	}{
		{"unchanged", "a\nb", "a\nb", nil},
		{"replace earlier", "a\nb", "x\nb", []Op{{Kind: PUT_RANGE, Line: 1, End: 1, Payload: []string{"x"}}}},
		{"insert before final", "a\nb", "a\nx\nb", []Op{{Kind: PUT_INSERT_BEFORE, Line: 2, Payload: []string{"x"}}}},
		{"append EOF", "a\nb", "a\nb\nx\n", []Op{{Kind: PUT_INSERT_AFTER, Payload: []string{"x"}}}},
		{"append after final", "a\nb", "a\nb\nx\n", []Op{{Kind: PUT_INSERT_AFTER, Line: 2, Payload: []string{"x"}}}},
		{"replace final", "a\nb", "a\nx\n", []Op{{Kind: PUT_RANGE, Line: 2, End: 2, Payload: []string{"x"}}}},
		{"cut first", "a\nb", "b", []Op{{Kind: CUT_RANGE, Line: 1, End: 1}}},
		{"cut final", "a\nb", "a\n", []Op{{Kind: CUT_RANGE, Line: 2, End: 2}}},
		{"delete all via empty replacement", "a\nb", "", []Op{{Kind: PUT_RANGE, Line: 1, End: 2}}},
		{"replace terminated final", "a\nb\n", "a\nx\n", []Op{{Kind: PUT_RANGE, Line: 2, End: 2, Payload: []string{"x"}}}},
		{"LF normalized from BOM and CRLF", "\ufeffa\r\nb", "a\nx\nb", []Op{{Kind: PUT_INSERT_AFTER, Line: 1, Payload: []string{"x"}}}},
		{"blank original lines", "\n\n", "x\n\n\n", []Op{{Kind: PUT_INSERT_BEFORE, Line: 1, Payload: []string{"x"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := StageLines(stageSnapshot(t, test.source), test.ops)
			if err != nil || string(result.Bytes) != test.want {
				t.Fatalf("StageLines = %q, %v; want %q", result.Bytes, err, test.want)
			}
		})
	}
}

func TestStageLinesRestoreWithBOMAndCRLF(t *testing.T) {
	source := stageSnapshot(t, "\ufeffa\r\nb")
	result, err := StageLines(source, []Op{{Kind: PUT_INSERT_AFTER, Line: 1, Payload: []string{"x"}}})
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Restore(source, result.Bytes)
	if err != nil || string(restored) != "\ufeffa\r\nx\r\nb" {
		t.Fatalf("Restore after staging = %q, %v", restored, err)
	}
}

func TestStageLinesDisjointAndAdjacentRanges(t *testing.T) {
	source := stageSnapshot(t, "a\nb\nc\nd\n")
	ops := []Op{
		{Kind: CUT_RANGE, Line: 3, End: 4, Register: "saved"},
		{Kind: PUT_RANGE, Line: 1, End: 2, Payload: []string{"first"}},
		{Kind: PUT_INSERT_AFTER, Line: 2, Payload: []string{"between"}},
	}
	result, err := StageLines(source, ops)
	wantTouched := []LineSpan{{Start: 1, End: 2}, {Start: 3, End: 4}}
	if err != nil || string(result.Bytes) != "first\nbetween\n" || !reflect.DeepEqual(result.Touched, wantTouched) {
		t.Fatalf("StageLines = %q, %#v, %v; want adjacent spans %#v", result.Bytes, result.Touched, err, wantTouched)
	}
}

func TestStageLinesRejectsConflictsAndInvalidOperationsWithoutMutation(t *testing.T) {
	for _, test := range []struct {
		name, source string
		ops          []Op
	}{
		{"overlapping ranges", "a\nb\nc\nd\n", []Op{{Kind: PUT_RANGE, Line: 1, End: 2, Payload: []string{"x"}}, {Kind: CUT_RANGE, Line: 2, End: 4}}},
		{"same range", "a\nb\nc\nd\n", []Op{{Kind: CUT_RANGE, Line: 2, End: 3}, {Kind: PUT_RANGE, Line: 2, End: 3}}},
		{"insertion inside range", "a\nb\nc\nd\n", []Op{{Kind: PUT_RANGE, Line: 1, End: 3}, {Kind: PUT_INSERT_BEFORE, Line: 3, Payload: []string{"x"}}}},
		{"insertion inside preceding range", "a\nb\nc\nd\n", []Op{{Kind: PUT_INSERT_AFTER, Line: 2, Payload: []string{"x"}}, {Kind: CUT_RANGE, Line: 1, End: 3}}},
		{"negative range", "a\n", []Op{{Kind: CUT_RANGE, Line: -1, End: 1}}},
		{"zero range", "a\n", []Op{{Kind: PUT_RANGE, Line: 0, End: 1}}},
		{"range past end", "a\n", []Op{{Kind: CUT_RANGE, Line: 1, End: 2}}},
		{"reversed range", "a\nb\n", []Op{{Kind: PUT_RANGE, Line: 2, End: 1}}},
		{"before zero", "", []Op{{Kind: PUT_INSERT_BEFORE, Line: 0, Payload: []string{"x"}}}},
		{"before past empty", "", []Op{{Kind: PUT_INSERT_BEFORE, Line: 2, Payload: []string{"x"}}}},
		{"before past end", "a\n", []Op{{Kind: PUT_INSERT_BEFORE, Line: 2, Payload: []string{"x"}}}},
		{"after empty line", "", []Op{{Kind: PUT_INSERT_AFTER, Line: 1, Payload: []string{"x"}}}},
		{"negative after", "a\n", []Op{{Kind: PUT_INSERT_AFTER, Line: -1, Payload: []string{"x"}}}},
		{"after past end", "a\n", []Op{{Kind: PUT_INSERT_AFTER, Line: 2, Payload: []string{"x"}}}},
		{"empty insertion", "a\n", []Op{{Kind: PUT_INSERT_BEFORE, Line: 1}}},
		{"payload LF", "a\n", []Op{{Kind: PUT_RANGE, Line: 1, End: 1, Payload: []string{"x\ny"}}}},
		{"payload CR", "a\n", []Op{{Kind: PUT_INSERT_AFTER, Line: 1, Payload: []string{"x\ry"}}}},
		{"invalid UTF-8", "a\n", []Op{{Kind: PUT_INSERT_AFTER, Line: 1, Payload: []string{string([]byte{0xff})}}}},
		{"unresolved block", "a\n", []Op{{Kind: PUT_RANGE, Line: 1, End: 1, Block: true}}},
		{"unresolved target", "a\n", []Op{{Kind: PUT_RANGE, Line: 1, End: 1, Target: PUT_RANGE}}},
		{"unresolved register", "a\n", []Op{{Kind: PUT_RANGE, Line: 1, End: 1, Register: "saved"}}},
		{"invalid CUT register", "a\n", []Op{{Kind: CUT_RANGE, Line: 1, End: 1, Register: "bad name"}}},
		{"ignored destination", "a\n", []Op{{Kind: CUT_RANGE, Line: 1, End: 1, Dest: "other"}}},
		{"CUT payload", "a\n", []Op{{Kind: CUT_RANGE, Line: 1, End: 1, Payload: []string{"x"}}}},
		{"insertion end", "a\n", []Op{{Kind: PUT_INSERT_AFTER, Line: 1, End: 1, Payload: []string{"x"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := stageSnapshot(t, test.source)
			beforeSource := stageSnapshot(t, test.source)
			beforeOps := make([]Op, len(test.ops))
			copy(beforeOps, test.ops)
			for i := range beforeOps {
				if test.ops[i].Payload != nil {
					beforeOps[i].Payload = append([]string{}, test.ops[i].Payload...)
				}
			}
			result, err := StageLines(source, test.ops)
			if err == nil || !reflect.DeepEqual(result, NormalizedResult{}) {
				t.Fatalf("StageLines = %#v, %v; want empty result and an error", result, err)
			}
			if !reflect.DeepEqual(source, beforeSource) || !reflect.DeepEqual(test.ops, beforeOps) {
				t.Fatal("StageLines mutated its inputs on error")
			}
		})
	}
}

func TestStageLinesRejectsUnsupportedKinds(t *testing.T) {
	source := stageSnapshot(t, "a\n")
	for _, kind := range []OpKind{PUT_BLOCK, PUT_BLOCK_AFTER, CUT_BLOCK, PUT_FROM_REGISTER, REM, MV, "UNKNOWN", ""} {
		t.Run(string(kind), func(t *testing.T) {
			ops := []Op{{Kind: PUT_RANGE, Line: 1, End: 1, Payload: []string{"x"}}, {Kind: kind}}
			result, err := StageLines(source, ops)
			var unsupported *UnsupportedOperationError
			if !errors.As(err, &unsupported) || unsupported.Kind != kind || unsupported.Index != 1 || !reflect.DeepEqual(result, NormalizedResult{}) {
				t.Fatalf("StageLines = %#v, %#v; want unsupported kind %q at index 1", result, err, kind)
			}
		})
	}
}

func TestStageLinesRequiresIntactSnapshot(t *testing.T) {
	for _, test := range []struct {
		name   string
		source Normalized
	}{
		{"zero value", Normalized{}},
		{"manually constructed", Normalized{Bytes: []byte("a\n"), Lines: []LineRecord{{Number: 1, Start: 0, End: 1, OriginalEnding: "\n"}}}},
		{"modified bytes", func() Normalized { s := stageSnapshot(t, "a\n"); s.Bytes[0] = 'x'; return s }()},
		{"modified lines", func() Normalized { s := stageSnapshot(t, "a\n"); s.Lines[0].End = 0; return s }()},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := StageLines(test.source, nil)
			if err == nil || !reflect.DeepEqual(result, NormalizedResult{}) {
				t.Fatalf("StageLines = %#v, %v; want error for invalid snapshot", result, err)
			}
		})
	}
}

func TestStageLinesResultDoesNotAliasInputs(t *testing.T) {
	source := stageSnapshot(t, "a\nb\n")
	ops := []Op{{Kind: PUT_RANGE, Line: 1, End: 1, Payload: []string{"x"}}}
	beforeSource := stageSnapshot(t, "a\nb\n")
	result, err := StageLines(source, ops)
	if err != nil {
		t.Fatal(err)
	}
	result.Bytes[0] = 'y'
	result.Touched[0].Start = 2
	if !reflect.DeepEqual(source, beforeSource) || !reflect.DeepEqual(ops, []Op{{Kind: PUT_RANGE, Line: 1, End: 1, Payload: []string{"x"}}}) {
		t.Fatal("StageLines result or success mutated its inputs")
	}
}
