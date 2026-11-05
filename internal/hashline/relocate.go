package hashline

import (
	"bytes"
	"fmt"
	"slices"
)

// relocationContext is the number of available neighbour lines on each side of a
// target that must match before the surrounding window alone proves its position.
const relocationContext = 3

// RelocationError reports an original line span without an unambiguous counterpart
// in the current content. Candidates holds the ambiguous current spans and stays
// empty when the target is absent. It never carries source text.
type RelocationError struct {
	Index      int
	Kind       OpKind
	Target     LineSpan
	Candidates []LineSpan
}

func (e *RelocationError) Error() string {
	if len(e.Candidates) == 0 {
		return fmt.Sprintf("operation %d (%s): original lines %d..%d are absent from the current content; read the current lines and retry", e.Index+1, e.Kind, e.Target.Start, e.Target.End)
	}
	return fmt.Sprintf("operation %d (%s): original lines %d..%d match %d current candidates; include more unchanged context", e.Index+1, e.Kind, e.Target.Start, e.Target.End, len(e.Candidates))
}

// lineContent resolves one-based line numbers of a normalized view to their
// exact bytes, terminators excluded.
type lineContent struct {
	view  []byte
	lines []LineRecord
}

func contentOf(snapshot Normalized) lineContent {
	return lineContent{view: snapshot.Bytes, lines: snapshot.Lines}
}

func (c lineContent) line(number int) []byte {
	record := c.lines[number-1]
	return c.view[record.Start:record.End]
}

// matchLineSpans returns the one-based start lines of up to limit occurrences of
// the pattern span as whole lines, using KMP so repeated lines stay linear.
func matchLineSpans(pattern lineContent, span LineSpan, source lineContent, limit int) []int {
	length := span.End - span.Start + 1
	if length > len(source.lines) {
		return nil
	}
	same := func(i, j int) bool {
		return bytes.Equal(pattern.line(span.Start+i), source.line(j+1))
	}
	failure := make([]int, length)
	for i, k := 1, 0; i < length; i++ {
		for k > 0 && !same(i, k) {
			k = failure[k-1]
		}
		if same(i, k) {
			k++
		}
		failure[i] = k
	}
	var starts []int
	for j, k := 0, 0; j < len(source.lines); j++ {
		for k > 0 && !same(k, j) {
			k = failure[k-1]
		}
		if same(k, j) {
			k++
		}
		if k < length {
			continue
		}
		starts = append(starts, j-length+2)
		if len(starts) == limit {
			break
		}
		k = failure[k-1]
	}
	return starts
}

func relocateSpan(original, current lineContent, target LineSpan, index int, kind OpKind) (LineSpan, error) {
	before := min(relocationContext, target.Start-1)
	after := min(relocationContext, len(original.lines)-target.End)
	window := LineSpan{Start: target.Start - before, End: target.End + after}
	if len(matchLineSpans(original, window, original, 2)) == 1 {
		if starts := matchLineSpans(original, window, current, 2); len(starts) == 1 {
			offset := target.Start - window.Start
			return LineSpan{Start: starts[0] + offset, End: starts[0] + offset + target.End - target.Start}, nil
		}
	}
	if len(matchLineSpans(original, target, original, 2)) == 1 {
		if starts := matchLineSpans(original, target, current, 2); len(starts) == 1 {
			return LineSpan{Start: starts[0], End: starts[0] + target.End - target.Start}, nil
		}
	}
	var candidates []LineSpan
	for _, start := range matchLineSpans(original, target, current, 2) {
		candidates = append(candidates, LineSpan{Start: start, End: start + target.End - target.Start})
	}
	return LineSpan{}, &RelocationError{Index: index, Kind: kind, Target: target, Candidates: candidates}
}

// RelocateOps maps original-coordinate line operations onto changed current
// content and deep-copies their payloads. Both snapshots must be intact
// Normalize results and ops must pass StageLines validation against original.
// A range or insertion anchor moves only when its exact content, trailing
// whitespace included, matches exactly once in both sources together with up to
// three available neighbours on each side; without such a window it moves only
// when the target alone is unique in both sources. Ambiguous or absent targets
// are refused instead of guessed, so a duplicate whose intended occurrence
// changed never lands on the surviving copy. Explicit <1 and >$ keep their
// global gap, and identical bytes return independently copied ops. Proven-equal
// content is the only reason the caller's seen authorization for original still
// applies to the current content.
func RelocateOps(original, current Normalized, ops []Op) ([]Op, error) {
	for _, snapshot := range []Normalized{original, current} {
		if !snapshot.ready || !bytes.Equal(snapshot.Bytes, snapshot.original) || !slices.Equal(snapshot.Lines, snapshot.originalLines) {
			return nil, fmt.Errorf("relocation requires intact snapshots returned by Normalize")
		}
	}
	if _, err := StageLines(original, ops); err != nil {
		return nil, err
	}
	if len(ops) == 0 {
		return nil, nil
	}

	relocated := make([]Op, len(ops))
	for i, op := range ops {
		relocated[i] = op
		relocated[i].Payload = slices.Clone(op.Payload)
	}
	if bytes.Equal(original.Bytes, current.Bytes) {
		return relocated, nil
	}

	originalContent, currentContent := contentOf(original), contentOf(current)
	for i, op := range ops {
		if op.InsertAfterEOF() || op.Kind == PUT_INSERT_BEFORE && op.Line == 1 {
			continue
		}
		insertion := op.Kind == PUT_INSERT_BEFORE || op.Kind == PUT_INSERT_AFTER
		target := LineSpan{Start: op.Line, End: op.Line}
		if !insertion {
			target.End = op.End
		}
		mapped, err := relocateSpan(originalContent, currentContent, target, i, op.Kind)
		if err != nil {
			return nil, err
		}
		relocated[i].Line = mapped.Start
		if !insertion {
			relocated[i].End = mapped.End
		}
	}
	if _, err := StageLines(current, relocated); err != nil {
		return nil, err
	}
	return relocated, nil
}
