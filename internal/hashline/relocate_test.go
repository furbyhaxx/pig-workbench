package hashline

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func relocatedSource(t *testing.T, original, current string, ops []Op) ([]Op, string) {
	t.Helper()
	relocated, err := RelocateOps(stageSnapshot(t, original), stageSnapshot(t, current), ops)
	if err != nil {
		t.Fatalf("RelocateOps: %v", err)
	}
	result, err := StageLines(stageSnapshot(t, current), relocated)
	if err != nil {
		t.Fatalf("StageLines(current): %v", err)
	}
	return relocated, string(result.Bytes)
}

func relocateError(t *testing.T, original, current string, ops []Op) *RelocationError {
	t.Helper()
	relocated, err := RelocateOps(stageSnapshot(t, original), stageSnapshot(t, current), ops)
	var relocation *RelocationError
	if !errors.As(err, &relocation) || relocated != nil {
		t.Fatalf("RelocateOps = %#v, %v; want a RelocationError and no operations", relocated, err)
	}
	return relocation
}

func TestRelocateOpsShiftsAcrossUnrelatedInsertions(t *testing.T) {
	for _, test := range []struct {
		name, original, current string
		ops                     []Op
		want                    string
		span                    LineSpan
	}{
		{
			name:     "unique target below insertions",
			original: "alpha\nbeta\ngamma\ndelta\nepsilon\n",
			current:  "new one\nnew two\nalpha\nbeta\ngamma\ndelta\nepsilon\n",
			ops:      []Op{{Kind: PUT_RANGE, Line: 3, End: 3, Payload: []string{"gamma-edited"}}},
			want:     "new one\nnew two\nalpha\nbeta\ngamma-edited\ndelta\nepsilon\n",
			span:     LineSpan{Start: 5, End: 5},
		},
		{
			name:     "unique target at the file start",
			original: "gamma\ndelta\nepsilon\n",
			current:  "inserted\ngamma\ndelta\nepsilon\n",
			ops:      []Op{{Kind: CUT_RANGE, Line: 1, End: 2}},
			want:     "inserted\nepsilon\n",
			span:     LineSpan{Start: 2, End: 3},
		},
		{
			name:     "unique anchor with changed neighbour",
			original: "a\nunique-target\nb\nc\n",
			current:  "a\nunique-target\nb-changed\nc\n",
			ops:      []Op{{Kind: PUT_RANGE, Line: 2, End: 2, Payload: []string{"edited"}}},
			want:     "a\nedited\nb-changed\nc\n",
			span:     LineSpan{Start: 2, End: 2},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			relocated, got := relocatedSource(t, test.original, test.current, test.ops)
			if got != test.want {
				t.Fatalf("staged output = %q, want %q", got, test.want)
			}
			if span := (LineSpan{Start: relocated[0].Line, End: relocated[0].End}); span != test.span {
				t.Fatalf("relocated span = %#v, want %#v", span, test.span)
			}
		})
	}
}

func TestRelocateOpsMapsIndependentOperationsAgainstInsertedBlock(t *testing.T) {
	original := "# title\n\nalpha\nbeta\n\ngamma\ndelta\n"
	current := "intro\n" + original
	ops := []Op{
		{Kind: PUT_RANGE, Line: 3, End: 3, Payload: []string{"alpha2"}},
		{Kind: CUT_RANGE, Line: 7, End: 7},
		{Kind: PUT_INSERT_BEFORE, Line: 6, Payload: []string{"inserted-before-gamma"}},
		{Kind: PUT_INSERT_AFTER, Line: 4, Payload: []string{"inserted-after-beta"}},
	}
	relocated, got := relocatedSource(t, original, current, ops)
	want := "intro\n# title\n\nalpha2\nbeta\ninserted-after-beta\n\ninserted-before-gamma\ngamma\n"
	if got != want {
		t.Fatalf("staged output = %q, want %q", got, want)
	}
	wantOps := []Op{
		{Kind: PUT_RANGE, Line: 4, End: 4, Payload: []string{"alpha2"}},
		{Kind: CUT_RANGE, Line: 8, End: 8},
		{Kind: PUT_INSERT_BEFORE, Line: 7, Payload: []string{"inserted-before-gamma"}},
		{Kind: PUT_INSERT_AFTER, Line: 5, Payload: []string{"inserted-after-beta"}},
	}
	if !reflect.DeepEqual(relocated, wantOps) {
		t.Fatalf("relocated = %#v, want %#v", relocated, wantOps)
	}
}

func TestRelocateOpsRefusesChangedWhitespace(t *testing.T) {
	relocation := relocateError(t, "one\n  two  \nthree\n", "one\n  two\nthree\n",
		[]Op{{Kind: PUT_RANGE, Line: 2, End: 2, Payload: []string{"replaced"}}})
	want := &RelocationError{Index: 0, Kind: PUT_RANGE, Target: LineSpan{Start: 2, End: 2}}
	if !reflect.DeepEqual(relocation, want) {
		t.Fatalf("relocation = %#v, want %#v", relocation, want)
	}
	if strings.Contains(relocation.Error(), "two") {
		t.Fatalf("error carries source text: %v", relocation)
	}
}

func TestRelocateOpsRefusesDuplicateAfterIntendedOccurrenceChanged(t *testing.T) {
	relocation := relocateError(t, "one\ntwo\ntarget\none\ntwo\ntarget\nthree\n",
		"one\ntwo\nchanged\none\ntwo\ntarget\nthree\n",
		[]Op{{Kind: PUT_RANGE, Line: 3, End: 3, Payload: []string{"replaced"}}})
	want := &RelocationError{
		Index: 0, Kind: PUT_RANGE, Target: LineSpan{Start: 3, End: 3},
		Candidates: []LineSpan{{Start: 6, End: 6}},
	}
	if !reflect.DeepEqual(relocation, want) {
		t.Fatalf("relocation = %#v, want the surviving duplicate only: %#v", relocation, want)
	}
}

func TestRelocateOpsRefusesDuplicateBlockAfterIntendedOccurrenceChanged(t *testing.T) {
	relocation := relocateError(t, "x\none\ntwo\nthree\nx\none\ntwo\nthree\n",
		"x\none-changed\ntwo\nthree\nx\none\ntwo\nthree\n",
		[]Op{{Kind: CUT_RANGE, Line: 2, End: 3}})
	want := &RelocationError{
		Index: 0, Kind: CUT_RANGE, Target: LineSpan{Start: 2, End: 3},
		Candidates: []LineSpan{{Start: 6, End: 7}},
	}
	if !reflect.DeepEqual(relocation, want) {
		t.Fatalf("relocation = %#v, want the surviving duplicate only: %#v", relocation, want)
	}
}

func TestRelocateOpsRefusesAmbiguousCurrentMatches(t *testing.T) {
	relocation := relocateError(t, "a\ntarget\nb\n", "a\ntarget\nb\nx\na\ntarget\nb\n",
		[]Op{{Kind: PUT_RANGE, Line: 2, End: 2, Payload: []string{"replaced"}}})
	want := &RelocationError{
		Index: 0, Kind: PUT_RANGE, Target: LineSpan{Start: 2, End: 2},
		Candidates: []LineSpan{{Start: 2, End: 2}, {Start: 6, End: 6}},
	}
	if !reflect.DeepEqual(relocation, want) {
		t.Fatalf("relocation = %#v, want two candidates %#v", relocation, want)
	}
}

func TestRelocateOpsRefusesAbsentTarget(t *testing.T) {
	relocation := relocateError(t, "a\nb\nc\n", "a\nc\n",
		[]Op{{Kind: PUT_INSERT_BEFORE, Line: 2, Payload: []string{"x"}}})
	want := &RelocationError{Index: 0, Kind: PUT_INSERT_BEFORE, Target: LineSpan{Start: 2, End: 2}}
	if !reflect.DeepEqual(relocation, want) {
		t.Fatalf("relocation = %#v, want an empty-candidate refusal %#v", relocation, want)
	}
}

func TestRelocateOpsEndpointGapsAndEmptySources(t *testing.T) {
	for _, test := range []struct {
		name, original, current string
		ops                     []Op
		want                    string
	}{
		{
			name: "empty to empty",
			ops:  []Op{{Kind: PUT_INSERT_BEFORE, Line: 1, Payload: []string{"top"}}, {Kind: PUT_INSERT_AFTER, Payload: []string{"end"}}},
			want: "top\nend\n",
		},
		{
			name:    "empty original into filled current",
			current: "x\ny\n",
			ops:     []Op{{Kind: PUT_INSERT_BEFORE, Line: 1, Payload: []string{"top"}}, {Kind: PUT_INSERT_AFTER, Payload: []string{"end"}}},
			want:    "top\nx\ny\nend\n",
		},
		{
			name:     "filled original into empty current",
			original: "a\n",
			ops:      []Op{{Kind: PUT_INSERT_BEFORE, Line: 1, Payload: []string{"top"}}},
			want:     "top\n",
		},
		{
			name:     "endpoint gaps stay global",
			original: "a\nb\n",
			current:  "z\na\nb\n",
			ops:      []Op{{Kind: PUT_INSERT_BEFORE, Line: 1, Payload: []string{"top"}}, {Kind: PUT_INSERT_AFTER, Line: 2, Payload: []string{"after-b"}}, {Kind: PUT_INSERT_AFTER, Payload: []string{"end"}}},
			want:     "top\nz\na\nb\nafter-b\nend\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, got := relocatedSource(t, test.original, test.current, test.ops); got != test.want {
				t.Fatalf("staged output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRelocateOpsIdenticalBytesCopyOperations(t *testing.T) {
	original, current := stageSnapshot(t, "a\nb\n"), stageSnapshot(t, "a\nb\n")
	ops := []Op{
		{Kind: PUT_RANGE, Line: 1, End: 1, Payload: []string{"x"}},
		{Kind: CUT_RANGE, Line: 2, End: 2, Register: "saved"},
	}
	relocated, err := RelocateOps(original, current, ops)
	if err != nil || !reflect.DeepEqual(relocated, ops) {
		t.Fatalf("RelocateOps = %#v, %v; want unchanged operations", relocated, err)
	}
	relocated[0].Payload[0] = "mutated"
	if ops[0].Payload[0] != "x" {
		t.Fatal("relocated payload aliases the input payload")
	}
	if result, err := StageLines(current, ops); err != nil || string(result.Bytes) != "x\n" {
		t.Fatalf("StageLines = %q, %v; want the copies to be usable", result.Bytes, err)
	}
}

func TestRelocateOpsRequiresIntactSnapshots(t *testing.T) {
	ops := []Op{{Kind: PUT_RANGE, Line: 1, End: 1, Payload: []string{"x"}}}
	for _, test := range []struct {
		name              string
		original, current Normalized
	}{
		{"zero original", Normalized{}, stageSnapshot(t, "a\n")},
		{"zero current", stageSnapshot(t, "a\n"), Normalized{}},
		{"modified original bytes", func() Normalized { s := stageSnapshot(t, "a\n"); s.Bytes[0] = 'b'; return s }(), stageSnapshot(t, "b\n")},
		{"modified current lines", stageSnapshot(t, "a\n"), func() Normalized { s := stageSnapshot(t, "b\n"); s.Lines[0].End = 0; return s }()},
	} {
		t.Run(test.name, func(t *testing.T) {
			relocated, err := RelocateOps(test.original, test.current, ops)
			if err == nil || relocated != nil {
				t.Fatalf("RelocateOps = %#v, %v; want an error for an invalid snapshot", relocated, err)
			}
		})
	}
}

func TestRelocateOpsValidatesOriginalOperations(t *testing.T) {
	for _, test := range []struct {
		name, original string
		ops            []Op
		unsupported    bool
	}{
		{"unresolved block", "a\nb\n", []Op{{Kind: PUT_BLOCK, Line: 1, Block: true, Payload: []string{"x"}}}, true},
		{"register paste", "a\nb\n", []Op{{Kind: PUT_FROM_REGISTER, Target: PUT_RANGE, Line: 1, End: 1, Register: "saved"}}, true},
		{"unresolved register", "a\nb\n", []Op{{Kind: PUT_RANGE, Line: 1, End: 1, Register: "saved"}}, false},
		{"reversed range", "a\nb\n", []Op{{Kind: PUT_RANGE, Line: 2, End: 1, Payload: []string{"x"}}}, false},
		{"overlapping ranges", "a\nb\nc\n", []Op{{Kind: PUT_RANGE, Line: 1, End: 2, Payload: []string{"x"}}, {Kind: CUT_RANGE, Line: 2, End: 3}}, false},
		{"empty insertion", "a\n", []Op{{Kind: PUT_INSERT_AFTER, Line: 1}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			original := stageSnapshot(t, test.original)
			relocated, err := RelocateOps(original, stageSnapshot(t, test.original), test.ops)
			if err == nil || relocated != nil {
				t.Fatalf("RelocateOps = %#v, %v; want a validation error", relocated, err)
			}
			var unsupported *UnsupportedOperationError
			if errors.As(err, &unsupported) != test.unsupported {
				t.Fatalf("error %v: unsupported kind reported = %v, want %v", err, !test.unsupported, test.unsupported)
			}
		})
	}
}

func TestRelocateOpsDoesNotMutateInputs(t *testing.T) {
	original, current := "a\nb\nc\nd\n", "intro\na\nb\nc\nd\n"
	ops := []Op{{Kind: PUT_RANGE, Line: 2, End: 2, Payload: []string{"x"}}}
	for _, test := range []struct {
		name, current string
		ops           []Op
	}{
		{"success", current, ops},
		{"refusal", "a\nchanged\nc\nd\n", ops},
	} {
		t.Run(test.name, func(t *testing.T) {
			source, target := stageSnapshot(t, original), stageSnapshot(t, test.current)
			beforeSource, beforeTarget := stageSnapshot(t, original), stageSnapshot(t, test.current)
			beforeOps := []Op{{Kind: PUT_RANGE, Line: 2, End: 2, Payload: []string{"x"}}}
			_, _ = RelocateOps(source, target, test.ops)
			if !reflect.DeepEqual(source, beforeSource) || !reflect.DeepEqual(target, beforeTarget) || !reflect.DeepEqual(test.ops, beforeOps) {
				t.Fatal("RelocateOps mutated its inputs")
			}
		})
	}
}
