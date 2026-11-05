package hashline

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// Register is a named line payload plus the exact source it was captured from.
// SourceURI, Revision and Epoch make a persisted register auditable; the planner
// only ever returns registers it captured from an original snapshot.
type Register struct {
	Lines     []string
	SourceURI string
	Revision  string
	Epoch     string
}

// BatchInput is one preflight transaction. Sources must already hold the snapshot
// chosen for each tagged path, newest-tag selection included; the planner never
// consults a store, filesystem, or lock. Registers are the registers a previous
// batch published and this batch may paste from. EnforceSeen makes line
// visibility and whole-source awareness mandatory. ResolveBlock resolves a
// syntax block anchored at line for path, and is required for any block operation.
type BatchInput struct {
	Document    Document
	Sources     map[string]Snapshot
	Registers   map[string]Register
	Epoch       string
	EnforceSeen bool
	// CurrentSources holds the exact source observed immediately before staging.
	// When it differs from Sources, resolved line operations relocate through the
	// unique-anchor rules before any mutation; a missing entry means the tagged
	// snapshot is still current.
	CurrentSources map[string]Snapshot
	ResolveBlock   func(path string, source Normalized, line int) (LineSpan, error)
}

// Effects is the staged outcome for one source. Bytes carries the full replacement
// content with the original BOM and line endings; Delete reports REM and leaves
// Bytes empty; MoveTo reports an MV destination. Touched lists the consumed
// original intervals and is empty for insertions and file operations.
type Effects struct {
	Path           string
	BeforeRevision string
	Bytes          []byte
	Delete         bool
	MoveTo         string
	Touched        []LineSpan
	Seen           []LineSpan
}

// BatchPlan is the ordered result of one preflight. Effects follow first section
// appearance; Registers holds only the named captures this batch proposed.
type BatchPlan struct {
	Effects   []Effects
	Registers map[string]Register
}

type batchSection struct {
	path              string
	ops               []Op
	snapshot          Snapshot
	normalized        Normalized
	current           Snapshot
	currentNormalized Normalized
	drifted           bool
	lineOps           []Op
	delete            bool
	moveTo            string
}

type batchRegisters struct {
	named    map[string]Register
	proposed map[string]Register
	defined  map[string]bool
	anon     []string
	anonCuts int
}

func batchOpError(path string, index int, op Op, reason string) error {
	return fmt.Errorf("section %q operation %d (%s): %s", path, index+1, op.Kind, reason)
}

// PlanBatch resolves blocks and registers, validates seen state and file
// operations, then stages every source. Any error returns an empty plan, so a
// refused batch publishes no effects and no registers. The inputs are not
// modified.
func PlanBatch(input BatchInput) (BatchPlan, error) {
	sections, work, err := batchSections(input)
	if err != nil {
		return BatchPlan{}, err
	}
	registers, err := newBatchRegisters(input.Registers)
	if err != nil {
		return BatchPlan{}, err
	}
	for _, item := range work {
		if err := resolveBatchOp(input, item, registers); err != nil {
			return BatchPlan{}, err
		}
	}
	if err := prepareBatchCurrent(input, sections); err != nil {
		return BatchPlan{}, err
	}
	if err := validateBatchMoves(sections); err != nil {
		return BatchPlan{}, err
	}
	plan := BatchPlan{Effects: make([]Effects, 0, len(sections)), Registers: registers.proposed}
	for _, section := range sections {
		effects, emit, err := stageBatchSection(input, section)
		if err != nil {
			return BatchPlan{}, err
		}
		if emit {
			plan.Effects = append(plan.Effects, effects)
		}
	}
	return plan, nil
}

// batchWork is one operation in document order. Index is its position across all
// sections, which orders register definitions and uses exactly as written.
type batchWork struct {
	section *batchSection
	index   int
	op      Op
}

// batchSections binds every section to its snapshot, rejects aliases of one
// canonical source, and returns sources in first-appearance order together with
// every operation in document order. Repeated same-path sections accumulate into
// one source without losing their place in the patch.
func batchSections(input BatchInput) ([]*batchSection, []batchWork, error) {
	if len(input.Document.Sections) == 0 {
		return nil, nil, errors.New("a batch requires at least one tagged section")
	}
	sections := make([]*batchSection, 0, len(input.Document.Sections))
	index := make(map[string]*batchSection, len(input.Document.Sections))
	uris := make(map[string]string, len(input.Document.Sections))
	work := make([]batchWork, 0, len(input.Document.Sections))
	for _, section := range input.Document.Sections {
		snapshot, ok := input.Sources[section.Path]
		if !ok {
			return nil, nil, fmt.Errorf("section %q has no supplied source snapshot", section.Path)
		}
		if snapshot.Tag != section.Tag {
			return nil, nil, fmt.Errorf("section %q tag %s does not match the supplied snapshot tag %s", section.Path, section.Tag, snapshot.Tag)
		}
		if snapshot.URI == "" {
			return nil, nil, fmt.Errorf("section %q snapshot requires a canonical URI", section.Path)
		}
		if snapshot.Revision == "" {
			return nil, nil, fmt.Errorf("section %q snapshot requires a source revision", section.Path)
		}
		grouped, repeated := index[section.Path]
		if !repeated {
			if other, alias := uris[snapshot.URI]; alias {
				return nil, nil, fmt.Errorf("sections %q and %q address the same canonical source %s", other, section.Path, snapshot.URI)
			}
			uris[snapshot.URI] = section.Path
			normalized, err := Normalize(snapshot.Raw)
			if err != nil {
				return nil, nil, fmt.Errorf("section %q: %w", section.Path, err)
			}
			grouped = &batchSection{path: section.Path, snapshot: snapshot, normalized: normalized, current: snapshot, currentNormalized: normalized}
			if observed, ok := input.CurrentSources[section.Path]; ok {
				if observed.URI != "" && observed.URI != snapshot.URI {
					return nil, nil, fmt.Errorf("section %q current source URI %s does not match %s", section.Path, observed.URI, snapshot.URI)
				}
				if observed.Revision == "" || SourceRevision(observed.Raw) != observed.Revision {
					return nil, nil, fmt.Errorf("section %q current source requires its exact observed revision", section.Path)
				}
				currentNormalized, err := Normalize(observed.Raw)
				if err != nil {
					return nil, nil, fmt.Errorf("section %q current source: %w", section.Path, err)
				}
				grouped.current = observed
				grouped.currentNormalized = currentNormalized
				grouped.drifted = observed.Revision != snapshot.Revision
			}
			index[section.Path] = grouped
			sections = append(sections, grouped)
		}
		grouped.ops = append(grouped.ops, section.Ops...)
		for _, op := range section.Ops {
			work = append(work, batchWork{section: grouped, index: len(work), op: op})
		}
	}
	return sections, work, nil
}

func resolveBatchOp(input BatchInput, item batchWork, registers *batchRegisters) error {
	section, index, op := item.section, item.index, item.op
	switch op.Kind {
	case REM:
		if len(section.ops) != 1 {
			return batchOpError(section.path, index, op, "REM must be the only operation for its source")
		}
		section.delete = true
		return nil
	case MV:
		if op.Dest == "" {
			return batchOpError(section.path, index, op, "MV requires a destination")
		}
		if section.moveTo != "" {
			return batchOpError(section.path, index, op, "a source can move only once")
		}
		if op.Dest == section.path {
			return batchOpError(section.path, index, op, "MV destination is the source itself")
		}
		section.moveTo = op.Dest
		return nil
	}
	lowered, err := lowerBatchOp(input, section, index, op, registers)
	if err != nil {
		return err
	}
	section.lineOps = append(section.lineOps, lowered)
	return nil
}

// lowerBatchOp turns one syntactic operation into an original-coordinate line
// operation, capturing CUT content and resolving register pastes at this point in
// the batch. Block operations resolve through the caller's syntax callback only.
func lowerBatchOp(input BatchInput, section *batchSection, index int, op Op, registers *batchRegisters) (Op, error) {
	switch op.Kind {
	case PUT_RANGE, PUT_INSERT_BEFORE, PUT_INSERT_AFTER:
		if op.Block || op.Target != "" || op.Dest != "" || op.Register != "" {
			return Op{}, batchOpError(section.path, index, op, "malformed payload operation")
		}
		return op, nil
	case CUT_RANGE:
		if err := registers.capture(input, section, index, op); err != nil {
			return Op{}, err
		}
		return op, nil
	case PUT_BLOCK, PUT_BLOCK_AFTER, CUT_BLOCK:
		span, err := resolveBatchBlock(input, section, index, op)
		if err != nil {
			return Op{}, err
		}
		switch op.Kind {
		case PUT_BLOCK:
			return Op{Kind: PUT_RANGE, Line: span.Start, End: span.End, Payload: op.Payload}, nil
		case PUT_BLOCK_AFTER:
			return Op{Kind: PUT_INSERT_AFTER, Line: span.End, Payload: op.Payload}, nil
		}
		lowered := Op{Kind: CUT_RANGE, Line: span.Start, End: span.End, Register: op.Register}
		if err := registers.capture(input, section, index, lowered); err != nil {
			return Op{}, err
		}
		return lowered, nil
	case PUT_FROM_REGISTER:
		lines, err := registers.read(op.Register)
		if err != nil {
			return Op{}, batchOpError(section.path, index, op, err.Error())
		}
		switch op.Target {
		case PUT_RANGE:
			if op.Block {
				return Op{}, batchOpError(section.path, index, op, "malformed payload operation")
			}
			return Op{Kind: PUT_RANGE, Line: op.Line, End: op.End, Payload: lines}, nil
		case PUT_INSERT_BEFORE, PUT_INSERT_AFTER:
			if op.Block {
				return Op{}, batchOpError(section.path, index, op, "malformed payload operation")
			}
			return Op{Kind: op.Target, Line: op.Line, Payload: lines}, nil
		case PUT_BLOCK, PUT_BLOCK_AFTER:
			span, err := resolveBatchBlock(input, section, index, op)
			if err != nil {
				return Op{}, err
			}
			if op.Target == PUT_BLOCK {
				return Op{Kind: PUT_RANGE, Line: span.Start, End: span.End, Payload: lines}, nil
			}
			return Op{Kind: PUT_INSERT_AFTER, Line: span.End, Payload: lines}, nil
		default:
			return Op{}, batchOpError(section.path, index, op, "register paste requires a PUT target")
		}
	default:
		return Op{}, batchOpError(section.path, index, op, "unknown operation")
	}
}

// resolveBatchBlock requires a strict multi-line span beginning at the anchor, so
// a failed or guessed block is never lowered to a brace range.
func resolveBatchBlock(input BatchInput, section *batchSection, index int, op Op) (LineSpan, error) {
	if input.ResolveBlock == nil {
		return LineSpan{}, batchOpError(section.path, index, op, "block operations require a block resolver")
	}
	span, err := input.ResolveBlock(section.path, section.normalized, op.Line)
	if err != nil {
		return LineSpan{}, batchOpError(section.path, index, op, "block resolution failed: "+err.Error())
	}
	if span.Start != op.Line || span.End <= span.Start || span.End > len(section.normalized.Lines) {
		return LineSpan{}, batchOpError(section.path, index, op, fmt.Sprintf("resolved block %d..%d is not a multi-line span beginning at the anchor", span.Start, span.End))
	}
	return span, nil
}

// prepareBatchCurrent validates authorization in the coordinates the model saw,
// then relocates resolved operations onto the observed current source. External
// changes never widen what the model was allowed to modify.
func prepareBatchCurrent(input BatchInput, sections []*batchSection) error {
	for _, section := range sections {
		if !section.drifted {
			continue
		}
		if section.delete {
			return fmt.Errorf("section %q changed since it was tagged; read the current source before deleting it", section.path)
		}
		if len(section.lineOps) == 0 {
			continue
		}
		if input.EnforceSeen {
			view := SeenView{Epoch: section.snapshot.Epoch, Ranges: section.snapshot.Seen}
			if err := CheckSeen(section.normalized, view, input.Epoch, section.lineOps); err != nil {
				return fmt.Errorf("section %q: %w", section.path, err)
			}
		}
		relocated, err := RelocateOps(section.normalized, section.currentNormalized, section.lineOps)
		if err != nil {
			var relocation *RelocationError
			if errors.As(err, &relocation) {
				annotated := *relocation
				annotated.Path = section.path
				return &annotated
			}
			return fmt.Errorf("section %q could not be relocated onto the current source: %w", section.path, err)
		}
		section.lineOps = relocated
	}
	return nil
}

func validateBatchMoves(sections []*batchSection) error {
	paths := make(map[string]bool, len(sections))
	for _, section := range sections {
		paths[section.path] = true
	}
	destinations := make(map[string]string, len(sections))
	for _, section := range sections {
		if section.moveTo == "" {
			continue
		}
		if paths[section.moveTo] {
			return fmt.Errorf("section %q moves onto %q, which is also part of this batch", section.path, section.moveTo)
		}
		if other, taken := destinations[section.moveTo]; taken {
			return fmt.Errorf("sections %q and %q both move to %q", other, section.path, section.moveTo)
		}
		destinations[section.moveTo] = section.path
	}
	return nil
}

func stageBatchSection(input BatchInput, section *batchSection) (Effects, bool, error) {
	if section.delete {
		if input.EnforceSeen {
			if err := checkWholeSourceSeen(section.snapshot, input.Epoch, len(section.normalized.Lines)); err != nil {
				return Effects{}, false, fmt.Errorf("section %q: %w", section.path, err)
			}
		}
		return Effects{Path: section.path, BeforeRevision: section.current.Revision, Delete: true}, true, nil
	}
	if len(section.lineOps) == 0 && section.moveTo == "" {
		return Effects{}, false, nil
	}
	if input.EnforceSeen && len(section.lineOps) > 0 && !section.drifted {
		view := SeenView{Epoch: section.snapshot.Epoch, Ranges: section.snapshot.Seen}
		if err := CheckSeen(section.normalized, view, input.Epoch, section.lineOps); err != nil {
			return Effects{}, false, fmt.Errorf("section %q: %w", section.path, err)
		}
	}
	result, err := StageSource(section.currentNormalized, section.lineOps)
	if err != nil {
		return Effects{}, false, fmt.Errorf("section %q: %w", section.path, err)
	}
	restored, err := RestoreStaged(section.currentNormalized, result)
	if err != nil {
		return Effects{}, false, fmt.Errorf("section %q: %w", section.path, err)
	}
	seen := []LineSpan(nil)
	if !section.drifted {
		seen = section.snapshot.Seen
	}
	return Effects{
		Path:           section.path,
		BeforeRevision: section.current.Revision,
		Bytes:          restored,
		MoveTo:         section.moveTo,
		Touched:        result.Touched,
		Seen:           SeenAfterStaging(result, seen),
	}, true, nil
}

// checkWholeSourceSeen requires every original line of a REM target.
func checkWholeSourceSeen(snapshot Snapshot, epoch string, lineCount int) error {
	if !validSeenEpoch(snapshot.Epoch) || !validSeenEpoch(epoch) || snapshot.Epoch != epoch {
		return &StaleSeenEpochError{}
	}
	if lineCount == 0 {
		return nil
	}
	seen := slices.Clone(snapshot.Seen)
	for _, span := range seen {
		if span.Start < 1 || span.End < span.Start || span.End > lineCount {
			return fmt.Errorf("seen interval %d..%d is outside the original source", span.Start, span.End)
		}
	}
	if missing := missingSeenSpans([]LineSpan{{Start: 1, End: lineCount}}, mergeSeenSpans(seen)); len(missing) > 0 {
		return &UnseenLinesError{Missing: missing}
	}
	return nil
}

func newBatchRegisters(supplied map[string]Register) (*batchRegisters, error) {
	registers := &batchRegisters{
		named:    make(map[string]Register, len(supplied)),
		proposed: make(map[string]Register),
		defined:  make(map[string]bool),
	}
	for name, register := range supplied {
		if !validRegister(name) {
			return nil, fmt.Errorf("register name %q is invalid", name)
		}
		if err := validateRegisterLines(register.Lines); err != nil {
			return nil, fmt.Errorf("register %q: %w", name, err)
		}
		registers.named[name] = Register{
			Lines:     append([]string(nil), register.Lines...),
			SourceURI: register.SourceURI,
			Revision:  register.Revision,
			Epoch:     register.Epoch,
		}
	}
	return registers, nil
}

func validateRegisterLines(lines []string) error {
	if len(lines) == 0 {
		return errors.New("register has no lines")
	}
	for _, line := range lines {
		if !utf8.ValidString(line) || strings.ContainsAny(line, "\r\n") {
			return errors.New("register lines must be valid UTF-8 without CR or LF")
		}
	}
	return nil
}

// capture stores a CUT payload read from the original source. A named register is
// defined once per batch; the unnamed register keeps the last unnamed capture and
// is only pastable while exactly one is pending.
func (r *batchRegisters) capture(input BatchInput, section *batchSection, index int, op Op) error {
	lineCount := len(section.normalized.Lines)
	if op.Line < 1 || op.End < op.Line || op.End > lineCount {
		return batchOpError(section.path, index, op, "range is reversed or outside the original source")
	}
	if op.Register != "" && !validRegister(op.Register) {
		return batchOpError(section.path, index, op, "invalid register name")
	}
	if !validSeenEpoch(input.Epoch) {
		return batchOpError(section.path, index, op, "register capture requires a valid context epoch")
	}
	lines := make([]string, 0, op.End-op.Line+1)
	for number := op.Line; number <= op.End; number++ {
		record := section.normalized.Lines[number-1]
		lines = append(lines, string(section.normalized.Bytes[record.Start:record.End]))
	}
	if op.Register == "" {
		r.anon, r.anonCuts = lines, r.anonCuts+1
		return nil
	}
	if r.defined[op.Register] {
		return batchOpError(section.path, index, op, fmt.Sprintf("register %q is defined twice in one batch", op.Register))
	}
	register := Register{Lines: lines, SourceURI: section.snapshot.URI, Revision: section.snapshot.Revision, Epoch: input.Epoch}
	r.named[op.Register] = register
	r.proposed[op.Register] = register
	r.defined[op.Register] = true
	return nil
}

func (r *batchRegisters) read(name string) ([]string, error) {
	if name == "" {
		switch {
		case r.anonCuts == 0:
			return nil, errors.New("no anonymous register in this batch; name the CUT register")
		case r.anonCuts > 1:
			return nil, fmt.Errorf("anonymous register is ambiguous after %d unnamed cuts; name the CUT register", r.anonCuts)
		}
		r.anonCuts = 0
		return append([]string(nil), r.anon...), nil
	}
	register, ok := r.named[name]
	if !ok {
		return nil, fmt.Errorf("register %q is not defined before this paste; defined registers: %s", name, r.known())
	}
	return append([]string(nil), register.Lines...), nil
}

func (r *batchRegisters) known() string {
	if len(r.named) == 0 {
		return "none"
	}
	names := make([]string, 0, len(r.named))
	for name := range r.named {
		names = append(names, name)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}
