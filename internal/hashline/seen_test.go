package hashline

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestCheckSeenResolvedRangeAndEpoch(t *testing.T) {
	source := stageSnapshot(t, "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\nnine\n")
	ops := []Op{{Kind: PUT_RANGE, Line: 2, End: 8, Payload: []string{"replacement"}}}
	view := SeenView{Epoch: "epoch-1", Ranges: []LineSpan{{Start: 2, End: 2}, {Start: 8, End: 8}}}
	err := CheckSeen(source, view, "epoch-1", ops)
	var unseen *UnseenLinesError
	if !errors.As(err, &unseen) || !reflect.DeepEqual(unseen.Missing, []LineSpan{{Start: 3, End: 7}}) {
		t.Fatalf("CheckSeen error = %#v; want unseen lines 3..7", err)
	}
	if got := (&UnseenLinesError{Missing: []LineSpan{{Start: 3, End: 7}, {Start: 100, End: 100}}}).Error(); got != "source lines 3-7, 100 were not seen in the current epoch; read them before editing" {
		t.Fatalf("refusal text = %q", got)
	}
	view.Ranges = []LineSpan{{Start: 5, End: 8}, {Start: 2, End: 4}}
	if err := CheckSeen(source, view, "epoch-1", ops); err != nil {
		t.Fatalf("adjacent coverage rejected: %v", err)
	}
	err = CheckSeen(source, view, "epoch-2", ops)
	var stale *StaleSeenEpochError
	if !errors.As(err, &stale) {
		t.Fatalf("CheckSeen error = %#v; want stale epoch", err)
	}
}

func TestCheckSeenCutRequiresEntireRange(t *testing.T) {
	source := stageSnapshot(t, "a\nb\nc\n")
	ops := []Op{{Kind: CUT_RANGE, Line: 1, End: 3}}
	view := SeenView{Epoch: "e", Ranges: []LineSpan{{1, 1}, {3, 3}}}
	var unseen *UnseenLinesError
	if err := CheckSeen(source, view, "e", ops); !errors.As(err, &unseen) || !reflect.DeepEqual(unseen.Missing, []LineSpan{{2, 2}}) {
		t.Fatalf("CheckSeen CUT = %#v; want unseen line 2", err)
	}
	view.Ranges = []LineSpan{{1, 3}}
	if err := CheckSeen(source, view, "e", ops); err != nil {
		t.Fatalf("CheckSeen CUT with full coverage = %v", err)
	}
}

func TestCheckSeenPreciseMissingSpans(t *testing.T) {
	source := stageSnapshot(t, "private-1\nprivate-2\nprivate-3\nprivate-4\nprivate-5\nprivate-6\nprivate-7\nprivate-8\nprivate-9\nprivate-10\n")
	view := SeenView{Epoch: "e", Ranges: []LineSpan{{8, 8}, {2, 3}, {5, 5}, {3, 3}, {10, 10}}}
	ops := []Op{
		{Kind: PUT_RANGE, Line: 1, End: 8, Payload: []string{"replacement"}},
		{Kind: PUT_INSERT_AFTER, Line: 9, Payload: []string{"insert"}},
		{Kind: CUT_RANGE, Line: 10, End: 10, Register: "saved"},
	}
	err := CheckSeen(source, view, "e", ops)
	var unseen *UnseenLinesError
	want := []LineSpan{{1, 1}, {4, 4}, {6, 7}, {9, 9}}
	if !errors.As(err, &unseen) || !reflect.DeepEqual(unseen.Missing, want) {
		t.Fatalf("CheckSeen error = %#v; want missing %#v", err, want)
	}
	if strings.Contains(err.Error(), "private-") {
		t.Fatalf("error exposed source text: %v", err)
	}
}

func TestCheckSeenDisjointTargetsSharingCoverage(t *testing.T) {
	source := stageSnapshot(t, "1\n2\n3\n4\n5\n6\n7\n8\n9\n")
	view := SeenView{Epoch: "e", Ranges: []LineSpan{{Start: 3, End: 8}}}
	ops := []Op{{Kind: CUT_RANGE, Line: 8, End: 9}, {Kind: PUT_RANGE, Line: 2, End: 3}}
	err := CheckSeen(source, view, "e", ops)
	var unseen *UnseenLinesError
	want := []LineSpan{{Start: 2, End: 2}, {Start: 9, End: 9}}
	if !errors.As(err, &unseen) || !reflect.DeepEqual(unseen.Missing, want) {
		t.Fatalf("CheckSeen error = %#v; want unseen %#v", err, want)
	}
}

func TestCheckSeenInsertionsUseAddressedAnchor(t *testing.T) {
	for _, test := range []struct {
		name, source string
		view         []LineSpan
		op           Op
		missing      []LineSpan
	}{
		{"BOF with no lines seen", "a\nb\nc\n", nil, Op{Kind: PUT_INSERT_BEFORE, Line: 1, Payload: []string{"x"}}, nil},
		{"EOF with no lines seen", "a\nb\nc\n", nil, Op{Kind: PUT_INSERT_AFTER, Payload: []string{"x"}}, nil},
		{"empty BOF", "", nil, Op{Kind: PUT_INSERT_BEFORE, Line: 1, Payload: []string{"x"}}, nil},
		{"empty EOF", "", nil, Op{Kind: PUT_INSERT_AFTER, Payload: []string{"x"}}, nil},
		{"unseen before anchor", "a\nb\nc\n", []LineSpan{{1, 1}}, Op{Kind: PUT_INSERT_BEFORE, Line: 2, Payload: []string{"x"}}, []LineSpan{{2, 2}}},
		{"unseen after anchor", "a\nb\nc\n", []LineSpan{{2, 2}}, Op{Kind: PUT_INSERT_AFTER, Line: 1, Payload: []string{"x"}}, []LineSpan{{1, 1}}},
		{"numbered EOF needs anchor", "a\nb\nc\n", []LineSpan{{2, 2}}, Op{Kind: PUT_INSERT_AFTER, Line: 3, Payload: []string{"x"}}, []LineSpan{{3, 3}}},
		{"seen before anchor", "a\nb\nc\n", []LineSpan{{2, 2}}, Op{Kind: PUT_INSERT_BEFORE, Line: 2, Payload: []string{"x"}}, nil},
		{"seen after anchor", "a\nb\nc\n", []LineSpan{{3, 3}}, Op{Kind: PUT_INSERT_AFTER, Line: 3, Payload: []string{"x"}}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := CheckSeen(stageSnapshot(t, test.source), SeenView{Epoch: "e", Ranges: test.view}, "e", []Op{test.op})
			if test.missing == nil {
				if err != nil {
					t.Fatalf("CheckSeen = %v; want success", err)
				}
				return
			}
			var unseen *UnseenLinesError
			if !errors.As(err, &unseen) || !reflect.DeepEqual(unseen.Missing, test.missing) {
				t.Fatalf("CheckSeen = %#v; want unseen %#v", err, test.missing)
			}
		})
	}
}

func TestCheckSeenRejectsStaleOrInvalidEpochs(t *testing.T) {
	source := stageSnapshot(t, "secret\n")
	ops := []Op{{Kind: PUT_INSERT_AFTER, Payload: []string{"x"}}}
	for _, test := range []struct{ view, current string }{
		{"", ""}, {"", "e"}, {"e", ""}, {"old", "new"},
		{" \t", " \t"}, {"e\x00", "e\x00"}, {string([]byte{0xff}), string([]byte{0xff})},
	} {
		err := CheckSeen(source, SeenView{Epoch: test.view}, test.current, ops)
		var stale *StaleSeenEpochError
		if !errors.As(err, &stale) || strings.Contains(err.Error(), "secret") {
			t.Errorf("CheckSeen epochs %q, %q = %#v; want non-disclosing stale epoch error", test.view, test.current, err)
		}
	}
}

func TestCheckSeenRejectsInvalidCoordinatesAndShapes(t *testing.T) {
	for _, test := range []struct {
		name, source string
		op           Op
	}{
		{"range zero", "a\nb\n", Op{Kind: PUT_RANGE, Line: 0, End: 1}},
		{"range reversed", "a\nb\n", Op{Kind: CUT_RANGE, Line: 2, End: 1}},
		{"range past end", "a\nb\n", Op{Kind: CUT_RANGE, Line: 1, End: 3}},
		{"range on empty", "", Op{Kind: PUT_RANGE, Line: 1, End: 1}},
		{"before zero", "a\nb\n", Op{Kind: PUT_INSERT_BEFORE, Line: 0, Payload: []string{"x"}}},
		{"before past end", "a\nb\n", Op{Kind: PUT_INSERT_BEFORE, Line: 3, Payload: []string{"x"}}},
		{"before past empty", "", Op{Kind: PUT_INSERT_BEFORE, Line: 2, Payload: []string{"x"}}},
		{"after negative", "a\nb\n", Op{Kind: PUT_INSERT_AFTER, Line: -1, Payload: []string{"x"}}},
		{"after past end", "a\nb\n", Op{Kind: PUT_INSERT_AFTER, Line: 3, Payload: []string{"x"}}},
		{"after empty", "", Op{Kind: PUT_INSERT_AFTER, Line: 1, Payload: []string{"x"}}},
		{"BOF with end", "", Op{Kind: PUT_INSERT_BEFORE, Line: 1, End: 1, Payload: []string{"x"}}},
		{"EOF with end", "", Op{Kind: PUT_INSERT_AFTER, End: 1, Payload: []string{"x"}}},
		{"empty BOF insertion", "", Op{Kind: PUT_INSERT_BEFORE, Line: 1}},
		{"empty EOF insertion", "", Op{Kind: PUT_INSERT_AFTER}},
		{"unresolved block flag", "a\n", Op{Kind: PUT_RANGE, Line: 1, End: 1, Block: true}},
		{"unresolved target", "a\n", Op{Kind: PUT_RANGE, Line: 1, End: 1, Target: PUT_RANGE}},
		{"unresolved PUT register", "a\n", Op{Kind: PUT_RANGE, Line: 1, End: 1, Register: "saved"}},
		{"invalid CUT register", "a\n", Op{Kind: CUT_RANGE, Line: 1, End: 1, Register: "bad name"}},
		{"file destination", "a\n", Op{Kind: CUT_RANGE, Line: 1, End: 1, Dest: "elsewhere"}},
		{"CUT payload", "a\n", Op{Kind: CUT_RANGE, Line: 1, End: 1, Payload: []string{"x"}}},
		{"embedded LF", "a\n", Op{Kind: PUT_RANGE, Line: 1, End: 1, Payload: []string{"x\ny"}}},
		{"embedded CR", "a\n", Op{Kind: PUT_INSERT_AFTER, Payload: []string{"x\ry"}}},
		{"invalid UTF-8", "a\n", Op{Kind: PUT_INSERT_AFTER, Payload: []string{string([]byte{0xff})}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := CheckSeen(stageSnapshot(t, test.source), SeenView{Epoch: "e"}, "e", []Op{test.op})
			var unseen *UnseenLinesError
			if err == nil || errors.As(err, &unseen) {
				t.Fatalf("CheckSeen(%+v) = %#v; want an invalid-operation error", test.op, err)
			}
		})
	}
}

func TestCheckSeenRejectsInvalidCoverageAndSnapshot(t *testing.T) {
	source := stageSnapshot(t, "a\nb\n")
	for _, ranges := range [][]LineSpan{{{0, 2}}, {{-1, 1}}, {{2, 1}}, {{1, 3}}} {
		if err := CheckSeen(source, SeenView{Epoch: "e", Ranges: ranges}, "e", nil); err == nil {
			t.Errorf("CheckSeen accepted invalid coverage %#v", ranges)
		}
	}
	modified := stageSnapshot(t, "a\nb\n")
	modified.Bytes[0] = 'x'
	modifiedLines := stageSnapshot(t, "a\nb\n")
	modifiedLines.Lines[0].End = 0
	for _, invalid := range []Normalized{{}, modified, modifiedLines} {
		if err := CheckSeen(invalid, SeenView{Epoch: "e"}, "e", nil); err == nil {
			t.Error("CheckSeen accepted invalid snapshot")
		}
	}
}

func TestCheckSeenRejectsUnsupportedOperations(t *testing.T) {
	source := stageSnapshot(t, "a\n")
	for _, kind := range []OpKind{PUT_BLOCK, PUT_BLOCK_AFTER, CUT_BLOCK, PUT_FROM_REGISTER, REM, MV, "UNKNOWN", ""} {
		t.Run(string(kind), func(t *testing.T) {
			ops := []Op{{Kind: PUT_RANGE, Line: 1, End: 1}, {Kind: kind}}
			err := CheckSeen(source, SeenView{Epoch: "e"}, "e", ops)
			var unsupported *UnsupportedOperationError
			if !errors.As(err, &unsupported) || unsupported.Index != 1 || unsupported.Kind != kind {
				t.Fatalf("CheckSeen error = %#v; want unsupported kind %q at index 1", err, kind)
			}
		})
	}
}

func TestCheckSeenDoesNotMutateInputs(t *testing.T) {
	for _, ranges := range [][]LineSpan{{{3, 4}, {1, 2}}, {{4, 4}, {1, 1}}} {
		source := stageSnapshot(t, "a\nb\nc\nd\n")
		beforeSource := stageSnapshot(t, "a\nb\nc\nd\n")
		view := SeenView{Epoch: "e", Ranges: ranges}
		beforeView := SeenView{Epoch: "e", Ranges: append([]LineSpan(nil), ranges...)}
		ops := []Op{{Kind: PUT_RANGE, Line: 1, End: 4, Payload: []string{"replacement"}}}
		beforeOps := []Op{{Kind: PUT_RANGE, Line: 1, End: 4, Payload: []string{"replacement"}}}
		err := CheckSeen(source, view, "e", ops)
		if ranges[0].Start == 3 && err != nil || ranges[0].Start == 4 && err == nil {
			t.Fatalf("CheckSeen = %v", err)
		}
		var unseen *UnseenLinesError
		if errors.As(err, &unseen) {
			unseen.Missing[0].Start = 42
		}
		if !reflect.DeepEqual(source, beforeSource) || !reflect.DeepEqual(view, beforeView) || !reflect.DeepEqual(ops, beforeOps) {
			t.Fatal("CheckSeen or its error mutated caller inputs")
		}
	}
}
