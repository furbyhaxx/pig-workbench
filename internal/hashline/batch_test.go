package hashline

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func batchSnapshot(t *testing.T, uri, epoch, source string) Snapshot {
	t.Helper()
	normalized, err := Normalize([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return Snapshot{URI: uri, Revision: SourceRevision([]byte(source)), Tag: normalized.Tag, Epoch: epoch, Raw: []byte(source)}
}

func batchSeen(snapshot Snapshot, ranges ...LineSpan) Snapshot {
	snapshot.Seen = ranges
	return snapshot
}

func batchDocument(t *testing.T, text string) Document {
	t.Helper()
	document, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse(%q): %v", text, err)
	}
	return document
}

func batchSectionText(snapshot Snapshot, path string, ops ...string) string {
	return Header(path, snapshot.Tag) + "\n" + strings.Join(ops, "\n") + "\n"
}

func TestPlanBatchCrossFileBlockCutAndPaste(t *testing.T) {
	const epoch = "epoch-1"
	first := batchSeen(batchSnapshot(t, "file:///src/a.go", epoch, "package a\n\nfunc helper() {\n\tx()\n}\n\nfunc other() {}\n"), LineSpan{1, 7})
	second := batchSeen(batchSnapshot(t, "file:///src/b.go", epoch, "package b\n\nfunc use() {}\n"), LineSpan{1, 3})
	patch := batchSectionText(first, "src/a.go", "CUT 3* @helper") +
		batchSectionText(second, "src/b.go", "PUT <3 @helper")
	input := BatchInput{
		Document:    batchDocument(t, patch),
		Sources:     map[string]Snapshot{"src/a.go": first, "src/b.go": second},
		Epoch:       epoch,
		EnforceSeen: true,
		ResolveBlock: func(path string, source Normalized, line int) (LineSpan, error) {
			if path == "src/a.go" && line == 3 {
				return LineSpan{Start: 3, End: 5}, nil
			}
			return LineSpan{}, errors.New("no block begins here")
		},
	}
	plan, err := PlanBatch(input)
	if err != nil {
		t.Fatalf("PlanBatch: %v", err)
	}
	want := []Effects{
		{
			Path:           "src/a.go",
			BeforeRevision: first.Revision,
			Bytes:          []byte("package a\n\n\nfunc other() {}\n"),
			Touched:        []LineSpan{{3, 5}},
		},
		{
			Path:           "src/b.go",
			BeforeRevision: second.Revision,
			Bytes:          []byte("package b\n\nfunc helper() {\n\tx()\n}\nfunc use() {}\n"),
		},
	}
	if !reflect.DeepEqual(plan.Effects, want) {
		t.Fatalf("effects = %#v, want %#v", plan.Effects, want)
	}
	register := Register{
		Lines:     []string{"func helper() {", "\tx()", "}"},
		SourceURI: "file:///src/a.go",
		Revision:  first.Revision,
		Epoch:     epoch,
	}
	if !reflect.DeepEqual(plan.Registers, map[string]Register{"helper": register}) {
		t.Fatalf("registers = %#v, want the named capture from a.go", plan.Registers)
	}
	if string(first.Raw) != "package a\n\nfunc helper() {\n\tx()\n}\n\nfunc other() {}\n" || string(second.Raw) != "package b\n\nfunc use() {}\n" {
		t.Fatal("PlanBatch mutated an input snapshot")
	}
}

func TestPlanBatchRefusesUnseenInteriorAndUndefinedRegister(t *testing.T) {
	const epoch = "epoch-1"
	first := batchSeen(batchSnapshot(t, "file:///src/a.go", epoch, "one\ntwo\nthree\nfour\nfive\n"), LineSpan{1, 3}, LineSpan{5, 5})
	second := batchSeen(batchSnapshot(t, "file:///src/b.go", epoch, "alpha\nbeta\n"), LineSpan{1, 2})
	block := func(path string, source Normalized, line int) (LineSpan, error) {
		return LineSpan{Start: 3, End: 4}, nil
	}
	t.Run("unseen block interior", func(t *testing.T) {
		input := BatchInput{
			Document: batchDocument(t, batchSectionText(first, "src/a.go", "CUT 3* @saved")+
				batchSectionText(second, "src/b.go", "PUT >2 @saved")),
			Sources:      map[string]Snapshot{"src/a.go": first, "src/b.go": second},
			Epoch:        epoch,
			EnforceSeen:  true,
			ResolveBlock: block,
		}
		plan, err := PlanBatch(input)
		var unseen *UnseenLinesError
		if !errors.As(err, &unseen) || !reflect.DeepEqual(unseen.Missing, []LineSpan{{4, 4}}) {
			t.Fatalf("PlanBatch = %#v, %v; want unseen line 4", plan, err)
		}
		if !reflect.DeepEqual(plan, BatchPlan{}) {
			t.Fatalf("refused batch published %#v", plan)
		}
		if string(first.Raw) != "one\ntwo\nthree\nfour\nfive\n" || !reflect.DeepEqual(first.Seen, []LineSpan{{1, 3}, {5, 5}}) {
			t.Fatal("refused batch mutated its input snapshot")
		}
	})
	t.Run("undefined register", func(t *testing.T) {
		input := BatchInput{
			Document: batchDocument(t, batchSectionText(second, "src/b.go", "CUT 1.=1 @saved")+
				batchSectionText(first, "src/a.go", "PUT 1.=1 @missing")),
			Sources:     map[string]Snapshot{"src/a.go": first, "src/b.go": second},
			Epoch:       epoch,
			EnforceSeen: true,
		}
		plan, err := PlanBatch(input)
		if err == nil || !strings.Contains(err.Error(), `register "missing" is not defined`) {
			t.Fatalf("PlanBatch = %#v, %v; want an undefined-register error", plan, err)
		}
		if !reflect.DeepEqual(plan, BatchPlan{}) {
			t.Fatalf("refused batch published %#v", plan)
		}
	})
	t.Run("forward register use", func(t *testing.T) {
		input := BatchInput{
			Document: batchDocument(t, batchSectionText(second, "src/b.go", "PUT >1 @later")+
				batchSectionText(first, "src/a.go", "CUT 1.=1 @later")),
			Sources:     map[string]Snapshot{"src/a.go": first, "src/b.go": second},
			Epoch:       epoch,
			EnforceSeen: true,
		}
		if plan, err := PlanBatch(input); err == nil || !strings.Contains(err.Error(), `register "later" is not defined`) || !reflect.DeepEqual(plan, BatchPlan{}) {
			t.Fatalf("PlanBatch = %#v, %v; want a forward-use error", plan, err)
		}
	})
}

func TestPlanBatchSuppliedRegisterRangePaste(t *testing.T) {
	const epoch = "epoch-1"
	source := batchSeen(batchSnapshot(t, "file:///src/c.go", epoch, "one\ntwo\nthree\nfour\n"), LineSpan{1, 4})
	saved := Register{Lines: []string{"A", "B"}, SourceURI: "file:///src/a.go", Revision: "rev-a", Epoch: epoch}
	input := BatchInput{
		Document:    batchDocument(t, batchSectionText(source, "src/c.go", "PUT 2.=3 @saved")),
		Sources:     map[string]Snapshot{"src/c.go": source},
		Registers:   map[string]Register{"saved": saved},
		Epoch:       epoch,
		EnforceSeen: true,
	}
	plan, err := PlanBatch(input)
	if err != nil {
		t.Fatalf("PlanBatch: %v", err)
	}
	if got, want := string(plan.Effects[0].Bytes), "one\nA\nB\nfour\n"; got != want {
		t.Fatalf("effects bytes = %q, want %q", got, want)
	}
	if len(plan.Registers) != 0 {
		t.Fatalf("proposed registers = %#v; supplied registers are not republished", plan.Registers)
	}
	if !reflect.DeepEqual(input.Registers["saved"], saved) {
		t.Fatal("PlanBatch mutated a supplied register")
	}
}

func TestPlanBatchSameGapOrdering(t *testing.T) {
	const epoch = "epoch-1"
	source := batchSnapshot(t, "file:///src/a.go", epoch, "a\nb\n")
	reg := Register{Lines: []string{"R1", "R2"}, Epoch: epoch}
	patch := batchSectionText(source, "src/a.go", "PUT >1:", "+one", "PUT >1 @reg", "PUT >1:", "+two")
	plan, err := PlanBatch(BatchInput{
		Document:  batchDocument(t, patch),
		Sources:   map[string]Snapshot{"src/a.go": source},
		Registers: map[string]Register{"reg": reg},
		Epoch:     epoch,
	})
	if err != nil {
		t.Fatalf("PlanBatch: %v", err)
	}
	if got, want := string(plan.Effects[0].Bytes), "a\none\nR1\nR2\ntwo\nb\n"; got != want {
		t.Fatalf("effects bytes = %q, want patch order %q", got, want)
	}
}

func TestPlanBatchAnonymousRegisters(t *testing.T) {
	const epoch = "epoch-1"
	source := batchSnapshot(t, "file:///src/a.go", epoch, "a\nb\nc\n")
	t.Run("single capture paste", func(t *testing.T) {
		plan, err := PlanBatch(BatchInput{
			Document: batchDocument(t, batchSectionText(source, "src/a.go", "CUT 2.=2", "PUT <1")),
			Sources:  map[string]Snapshot{"src/a.go": source},
			Epoch:    epoch,
		})
		if err != nil {
			t.Fatalf("PlanBatch: %v", err)
		}
		if got, want := string(plan.Effects[0].Bytes), "b\na\nc\n"; got != want {
			t.Fatalf("effects bytes = %q, want %q", got, want)
		}
		if len(plan.Registers) != 0 {
			t.Fatalf("anonymous capture published %#v", plan.Registers)
		}
	})
	for _, test := range []struct {
		name, patch string
		want        string
	}{
		{"no capture", "PUT <1", "no anonymous register"},
		{"two candidates", "CUT 2.=2\nCUT 3.=3\nPUT <1", "ambiguous"},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, err := PlanBatch(BatchInput{
				Document: batchDocument(t, batchSectionText(source, "src/a.go", strings.Split(test.patch, "\n")...)),
				Sources:  map[string]Snapshot{"src/a.go": source},
				Epoch:    epoch,
			})
			if err == nil || !strings.Contains(err.Error(), test.want) || !reflect.DeepEqual(plan, BatchPlan{}) {
				t.Fatalf("PlanBatch = %#v, %v; want %q", plan, err, test.want)
			}
		})
	}
}

func TestPlanBatchRememberedWholeSource(t *testing.T) {
	const epoch = "epoch-1"
	raw := "one\ntwo\nthree\n"
	t.Run("partial visibility refuses REM", func(t *testing.T) {
		source := batchSeen(batchSnapshot(t, "file:///src/a.go", epoch, raw), LineSpan{1, 2})
		plan, err := PlanBatch(BatchInput{
			Document:    batchDocument(t, batchSectionText(source, "src/a.go", "REM")),
			Sources:     map[string]Snapshot{"src/a.go": source},
			Epoch:       epoch,
			EnforceSeen: true,
		})
		var unseen *UnseenLinesError
		if !errors.As(err, &unseen) || !reflect.DeepEqual(unseen.Missing, []LineSpan{{3, 3}}) || !reflect.DeepEqual(plan, BatchPlan{}) {
			t.Fatalf("PlanBatch = %#v, %v; want unseen line 3 and no effects", plan, err)
		}
	})
	t.Run("full visibility deletes", func(t *testing.T) {
		source := batchSeen(batchSnapshot(t, "file:///src/a.go", epoch, raw), LineSpan{1, 2}, LineSpan{3, 3})
		plan, err := PlanBatch(BatchInput{
			Document:    batchDocument(t, batchSectionText(source, "src/a.go", "REM")),
			Sources:     map[string]Snapshot{"src/a.go": source},
			Epoch:       epoch,
			EnforceSeen: true,
		})
		if err != nil {
			t.Fatalf("PlanBatch: %v", err)
		}
		want := []Effects{{Path: "src/a.go", BeforeRevision: source.Revision, Delete: true}}
		if !reflect.DeepEqual(plan.Effects, want) {
			t.Fatalf("effects = %#v, want %#v", plan.Effects, want)
		}
	})
	t.Run("stale epoch refuses REM", func(t *testing.T) {
		source := batchSeen(batchSnapshot(t, "file:///src/a.go", "old-epoch", raw), LineSpan{1, 3})
		plan, err := PlanBatch(BatchInput{
			Document:    batchDocument(t, batchSectionText(source, "src/a.go", "REM")),
			Sources:     map[string]Snapshot{"src/a.go": source},
			Epoch:       epoch,
			EnforceSeen: true,
		})
		var stale *StaleSeenEpochError
		if !errors.As(err, &stale) || !reflect.DeepEqual(plan, BatchPlan{}) {
			t.Fatalf("PlanBatch = %#v, %v; want a stale epoch error", plan, err)
		}
	})
	t.Run("REM with edits", func(t *testing.T) {
		source := batchSnapshot(t, "file:///src/a.go", epoch, raw)
		plan, err := PlanBatch(BatchInput{
			Document: batchDocument(t, batchSectionText(source, "src/a.go", "REM", "PUT 1.=1:", "+x")),
			Sources:  map[string]Snapshot{"src/a.go": source},
			Epoch:    epoch,
		})
		if err == nil || !strings.Contains(err.Error(), "REM must be the only operation") || !reflect.DeepEqual(plan, BatchPlan{}) {
			t.Fatalf("PlanBatch = %#v, %v; want a REM conflict error", plan, err)
		}
	})
}

func TestPlanBatchMoveDestinations(t *testing.T) {
	const epoch = "epoch-1"
	source := batchSnapshot(t, "file:///src/a.go", epoch, "a\nb\n")
	other := batchSnapshot(t, "file:///src/b.go", epoch, "c\n")
	for _, test := range []struct {
		name  string
		patch string
		want  string
	}{
		{
			"duplicate destination",
			batchSectionText(source, "src/a.go", "MV dst") + batchSectionText(other, "src/b.go", "MV dst"),
			"both move to",
		},
		{
			"occupied destination",
			batchSectionText(source, "src/a.go", "MV src/b.go") + batchSectionText(other, "src/b.go", "CUT 1.=1"),
			"also part of this batch",
		},
		{"self move", batchSectionText(source, "src/a.go", "MV src/a.go"), "source itself"},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, err := PlanBatch(BatchInput{
				Document: batchDocument(t, test.patch),
				Sources:  map[string]Snapshot{"src/a.go": source, "src/b.go": other},
				Epoch:    epoch,
			})
			if err == nil || !strings.Contains(err.Error(), test.want) || !reflect.DeepEqual(plan, BatchPlan{}) {
				t.Fatalf("PlanBatch = %#v, %v; want %q", plan, err, test.want)
			}
		})
	}
	t.Run("move after edit", func(t *testing.T) {
		moved := batchSnapshot(t, "file:///src/a.go", epoch, "\ufeffa\r\nb")
		plan, err := PlanBatch(BatchInput{
			Document: batchDocument(t, batchSectionText(moved, "src/a.go", "PUT 1.=1:", "+x", `MV "new name.go"`)),
			Sources:  map[string]Snapshot{"src/a.go": moved},
			Epoch:    epoch,
		})
		if err != nil {
			t.Fatalf("PlanBatch: %v", err)
		}
		want := []Effects{{
			Path:           "src/a.go",
			BeforeRevision: moved.Revision,
			Bytes:          []byte("\ufeffx\r\nb"),
			MoveTo:         "new name.go",
			Touched:        []LineSpan{{1, 1}},
		}}
		if !reflect.DeepEqual(plan.Effects, want) {
			t.Fatalf("effects = %#v, want %#v", plan.Effects, want)
		}
	})
}

func TestPlanBatchBlockResolutionFailures(t *testing.T) {
	const epoch = "epoch-1"
	source := batchSnapshot(t, "file:///src/a.go", epoch, "one\ntwo\nthree\nfour\n")
	for _, test := range []struct {
		name      string
		ops       []string
		resolver  func(path string, source Normalized, line int) (LineSpan, error)
		wantError string
	}{
		{"missing resolver", []string{"PUT 3*:", "+x"}, nil, "block operations require a block resolver"},
		{"resolver error", []string{"PUT 3*:", "+x"}, func(string, Normalized, int) (LineSpan, error) {
			return LineSpan{}, errors.New("unresolved syntax")
		}, "block resolution failed: unresolved syntax"},
		{"single-line span", []string{"PUT 3*:", "+x"}, func(string, Normalized, int) (LineSpan, error) {
			return LineSpan{Start: 3, End: 3}, nil
		}, "is not a multi-line span beginning at the anchor"},
		{"wrong anchor", []string{"CUT 3* @saved"}, func(string, Normalized, int) (LineSpan, error) {
			return LineSpan{Start: 2, End: 4}, nil
		}, "is not a multi-line span beginning at the anchor"},
		{"span past source", []string{"PUT >3*:", "+x"}, func(string, Normalized, int) (LineSpan, error) {
			return LineSpan{Start: 3, End: 5}, nil
		}, "is not a multi-line span beginning at the anchor"},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, err := PlanBatch(BatchInput{
				Document:     batchDocument(t, batchSectionText(source, "src/a.go", test.ops...)),
				Sources:      map[string]Snapshot{"src/a.go": source},
				Epoch:        epoch,
				ResolveBlock: test.resolver,
			})
			if err == nil || !strings.Contains(err.Error(), test.wantError) || !reflect.DeepEqual(plan, BatchPlan{}) {
				t.Fatalf("PlanBatch = %#v, %v; want %q", plan, err, test.wantError)
			}
		})
	}
}

func TestPlanBatchBlockLowering(t *testing.T) {
	const epoch = "epoch-1"
	source := batchSnapshot(t, "file:///src/a.go", epoch, "one\ntwo\nthree\nfour\nfive\n")
	resolver := func(path string, source Normalized, line int) (LineSpan, error) {
		return LineSpan{Start: line, End: line + 1}, nil
	}
	t.Run("replace block", func(t *testing.T) {
		plan, err := PlanBatch(BatchInput{
			Document:     batchDocument(t, batchSectionText(source, "src/a.go", "PUT 2*:", "+two-three", "+four-five")),
			Sources:      map[string]Snapshot{"src/a.go": source},
			Epoch:        epoch,
			ResolveBlock: resolver,
		})
		if err != nil {
			t.Fatalf("PlanBatch: %v", err)
		}
		if got, want := string(plan.Effects[0].Bytes), "one\ntwo-three\nfour-five\nfour\nfive\n"; got != want {
			t.Fatalf("effects bytes = %q, want %q", got, want)
		}
		if !reflect.DeepEqual(plan.Effects[0].Touched, []LineSpan{{2, 3}}) {
			t.Fatalf("touched = %#v, want [2..3]", plan.Effects[0].Touched)
		}
	})
	t.Run("insert after block", func(t *testing.T) {
		plan, err := PlanBatch(BatchInput{
			Document:     batchDocument(t, batchSectionText(source, "src/a.go", "PUT >2*:", "+after")),
			Sources:      map[string]Snapshot{"src/a.go": source},
			Epoch:        epoch,
			ResolveBlock: resolver,
		})
		if err != nil {
			t.Fatalf("PlanBatch: %v", err)
		}
		if got, want := string(plan.Effects[0].Bytes), "one\ntwo\nthree\nafter\nfour\nfive\n"; got != want {
			t.Fatalf("effects bytes = %q, want %q", got, want)
		}
	})
	t.Run("paste over block", func(t *testing.T) {
		reg := Register{Lines: []string{"replacement"}, Epoch: epoch}
		plan, err := PlanBatch(BatchInput{
			Document:     batchDocument(t, batchSectionText(source, "src/a.go", "PUT 2* @reg")),
			Sources:      map[string]Snapshot{"src/a.go": source},
			Registers:    map[string]Register{"reg": reg},
			Epoch:        epoch,
			ResolveBlock: resolver,
		})
		if err != nil {
			t.Fatalf("PlanBatch: %v", err)
		}
		if got, want := string(plan.Effects[0].Bytes), "one\nreplacement\nfour\nfive\n"; got != want {
			t.Fatalf("effects bytes = %q, want %q", got, want)
		}
	})
}

func TestPlanBatchPreservesBOMAndEndings(t *testing.T) {
	const epoch = "epoch-1"
	source := batchSnapshot(t, "file:///src/a.go", epoch, "\ufeffone\r\ntwo\r\nthree\r\n")
	plan, err := PlanBatch(BatchInput{
		Document: batchDocument(t, batchSectionText(source, "src/a.go", "PUT 2.=2:", "+TWO")),
		Sources:  map[string]Snapshot{"src/a.go": source},
		Epoch:    epoch,
	})
	if err != nil {
		t.Fatalf("PlanBatch: %v", err)
	}
	if got, want := string(plan.Effects[0].Bytes), "\ufeffone\r\nTWO\r\nthree\r\n"; got != want {
		t.Fatalf("effects bytes = %q, want %q", got, want)
	}
}

func TestPlanBatchMergesSamePathSections(t *testing.T) {
	const epoch = "epoch-1"
	source := batchSnapshot(t, "file:///src/a.go", epoch, "a\nb\nc\n")
	patch := batchSectionText(source, "src/a.go", "PUT 1.=1:", "+x") +
		batchSectionText(source, "src/a.go", "PUT 3.=3:", "+z")
	plan, err := PlanBatch(BatchInput{
		Document: batchDocument(t, patch),
		Sources:  map[string]Snapshot{"src/a.go": source},
		Epoch:    epoch,
	})
	if err != nil {
		t.Fatalf("PlanBatch: %v", err)
	}
	if len(plan.Effects) != 1 || string(plan.Effects[0].Bytes) != "x\nb\nz\n" {
		t.Fatalf("effects = %#v; want one merged source", plan.Effects)
	}
}

func TestPlanBatchRegisterOrderAcrossRepeatedSections(t *testing.T) {
	const epoch = "epoch-1"
	first := batchSnapshot(t, "file:///src/a.go", epoch, "a\nb\n")
	second := batchSnapshot(t, "file:///src/b.go", epoch, "c\n")
	t.Run("later definition cannot satisfy earlier paste", func(t *testing.T) {
		patch := batchSectionText(first, "src/a.go", "PUT 1.=1:", "+x") +
			batchSectionText(second, "src/b.go", "PUT >1 @later") +
			batchSectionText(first, "src/a.go", "CUT 2.=2 @later")
		plan, err := PlanBatch(BatchInput{
			Document: batchDocument(t, patch),
			Sources:  map[string]Snapshot{"src/a.go": first, "src/b.go": second},
			Epoch:    epoch,
		})
		if err == nil || !strings.Contains(err.Error(), `register "later" is not defined`) {
			t.Fatalf("PlanBatch = %#v, %v; want a forward-reference error", plan, err)
		}
		if !reflect.DeepEqual(plan, BatchPlan{}) {
			t.Fatalf("refused batch published %#v", plan)
		}
	})
	t.Run("earlier capture feeds later paste", func(t *testing.T) {
		patch := batchSectionText(first, "src/a.go", "CUT 2.=2 @saved") +
			batchSectionText(second, "src/b.go", "PUT >1 @saved") +
			batchSectionText(first, "src/a.go", "PUT 1.=1:", "+x")
		plan, err := PlanBatch(BatchInput{
			Document: batchDocument(t, patch),
			Sources:  map[string]Snapshot{"src/a.go": first, "src/b.go": second},
			Epoch:    epoch,
		})
		if err != nil {
			t.Fatalf("PlanBatch: %v", err)
		}
		want := []Effects{
			{Path: "src/a.go", BeforeRevision: first.Revision, Bytes: []byte("x\n"), Touched: []LineSpan{{1, 1}, {2, 2}}},
			{Path: "src/b.go", BeforeRevision: second.Revision, Bytes: []byte("c\nb\n")},
		}
		if !reflect.DeepEqual(plan.Effects, want) {
			t.Fatalf("effects = %#v, want %#v", plan.Effects, want)
		}
		capture := Register{Lines: []string{"b"}, SourceURI: "file:///src/a.go", Revision: first.Revision, Epoch: epoch}
		if !reflect.DeepEqual(plan.Registers, map[string]Register{"saved": capture}) {
			t.Fatalf("registers = %#v, want the a.go capture", plan.Registers)
		}
	})
}

func TestPlanBatchRejectsInvalidInputs(t *testing.T) {
	const epoch = "epoch-1"
	source := batchSnapshot(t, "file:///src/a.go", epoch, "a\nb\n")
	alias := batchSnapshot(t, "file:///src/a.go", epoch, "a\nb\n")
	for _, test := range []struct {
		name  string
		input BatchInput
		want  string
	}{
		{
			"no sections",
			BatchInput{Sources: map[string]Snapshot{"src/a.go": source}, Epoch: epoch},
			"at least one tagged section",
		},
		{
			"missing snapshot",
			BatchInput{Document: batchDocument(t, batchSectionText(source, "src/a.go", "CUT 1.=1")), Epoch: epoch},
			"no supplied source snapshot",
		},
		{
			"tag mismatch",
			BatchInput{Document: batchDocument(t, "[src/a.go#FFFF]\nCUT 1.=1\n"), Sources: map[string]Snapshot{"src/a.go": source}, Epoch: epoch},
			"does not match the supplied snapshot tag",
		},
		{
			"alias of one canonical source",
			BatchInput{
				Document: batchDocument(t, batchSectionText(source, "src/a.go", "CUT 1.=1")+batchSectionText(alias, "src/alias.go", "CUT 1.=1")),
				Sources:  map[string]Snapshot{"src/a.go": source, "src/alias.go": alias},
				Epoch:    epoch,
			},
			"same canonical source",
		},
		{
			"missing canonical URI",
			BatchInput{
				Document: batchDocument(t, batchSectionText(source, "src/a.go", "CUT 1.=1")),
				Sources:  map[string]Snapshot{"src/a.go": {Revision: source.Revision, Tag: source.Tag, Epoch: epoch, Raw: source.Raw}},
				Epoch:    epoch,
			},
			"requires a canonical URI",
		},
		{
			"missing revision",
			BatchInput{
				Document: batchDocument(t, batchSectionText(source, "src/a.go", "CUT 1.=1")),
				Sources:  map[string]Snapshot{"src/a.go": {URI: source.URI, Tag: source.Tag, Epoch: epoch, Raw: source.Raw}},
				Epoch:    epoch,
			},
			"requires a source revision",
		},
		{
			"capture without epoch",
			BatchInput{
				Document: batchDocument(t, batchSectionText(source, "src/a.go", "CUT 1.=1")),
				Sources:  map[string]Snapshot{"src/a.go": source},
			},
			"requires a valid context epoch",
		},
		{
			"duplicate named definition",
			BatchInput{
				Document: batchDocument(t, batchSectionText(source, "src/a.go", "CUT 1.=1 @saved", "CUT 2.=2 @saved")),
				Sources:  map[string]Snapshot{"src/a.go": source},
				Epoch:    epoch,
			},
			"defined twice in one batch",
		},
		{
			"empty supplied register",
			BatchInput{
				Document:  batchDocument(t, batchSectionText(source, "src/a.go", "PUT 1.=1 @saved")),
				Sources:   map[string]Snapshot{"src/a.go": source},
				Registers: map[string]Register{"saved": {}},
				Epoch:     epoch,
			},
			"register has no lines",
		},
		{
			"invalid supplied register name",
			BatchInput{
				Document:  batchDocument(t, batchSectionText(source, "src/a.go", "PUT 1.=1 @saved")),
				Sources:   map[string]Snapshot{"src/a.go": source},
				Registers: map[string]Register{"bad name": {Lines: []string{"x"}}},
				Epoch:     epoch,
			},
			"is invalid",
		},
		{
			"register line with CR",
			BatchInput{
				Document:  batchDocument(t, batchSectionText(source, "src/a.go", "PUT 1.=1 @saved")),
				Sources:   map[string]Snapshot{"src/a.go": source},
				Registers: map[string]Register{"saved": {Lines: []string{"x\ry"}}},
				Epoch:     epoch,
			},
			"without CR or LF",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, err := PlanBatch(test.input)
			if err == nil || !strings.Contains(err.Error(), test.want) || !reflect.DeepEqual(plan, BatchPlan{}) {
				t.Fatalf("PlanBatch = %#v, %v; want %q and an empty plan", plan, err, test.want)
			}
		})
	}
}

func TestPlanBatchDoesNotAliasInputs(t *testing.T) {
	const epoch = "epoch-1"
	source := batchSeen(batchSnapshot(t, "file:///src/a.go", epoch, "a\nb\n"), LineSpan{1, 2})
	input := BatchInput{
		Document:  batchDocument(t, batchSectionText(source, "src/a.go", "CUT 1.=1 @saved")),
		Sources:   map[string]Snapshot{"src/a.go": source},
		Registers: map[string]Register{"other": {Lines: []string{"keep"}}},
		Epoch:     epoch,
	}
	plan, err := PlanBatch(input)
	if err != nil {
		t.Fatalf("PlanBatch: %v", err)
	}
	if got, want := plan.Registers["saved"].Lines, []string{"a"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("proposed register lines = %#v, want %#v", got, want)
	}
	plan.Effects[0].Bytes[0] = 'X'
	plan.Registers["saved"].Lines[0] = "changed"
	if string(source.Raw) != "a\nb\n" || !reflect.DeepEqual(input.Registers["other"].Lines, []string{"keep"}) {
		t.Fatal("PlanBatch result aliased or mutated its inputs")
	}
}

func TestPlanBatchKeepsEndingOfSurvivingDuplicateLine(t *testing.T) {
	const epoch = "epoch-1"
	source := batchSnapshot(t, "file:///src/a.go", epoch, "same\r\nsame\n")
	for _, test := range []struct {
		name, op, want string
		touched        LineSpan
	}{
		{"cut first keeps LF", "CUT 1.=1", "same\n", LineSpan{1, 1}},
		{"cut second keeps CRLF", "CUT 2.=2", "same\r\n", LineSpan{2, 2}},
	} {
		t.Run(test.name, func(t *testing.T) {
			plan, err := PlanBatch(BatchInput{
				Document: batchDocument(t, batchSectionText(source, "src/a.go", test.op)),
				Sources:  map[string]Snapshot{"src/a.go": source},
				Epoch:    epoch,
			})
			if err != nil {
				t.Fatalf("PlanBatch: %v", err)
			}
			if len(plan.Effects) != 1 || string(plan.Effects[0].Bytes) != test.want {
				t.Fatalf("effects = %#v; want one source %q", plan.Effects, test.want)
			}
			if !reflect.DeepEqual(plan.Effects[0].Touched, []LineSpan{test.touched}) {
				t.Fatalf("touched = %#v, want %#v", plan.Effects[0].Touched, []LineSpan{test.touched})
			}
		})
	}
}
