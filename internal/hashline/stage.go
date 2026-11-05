package hashline

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// LineSpan is an inclusive interval of one-based original source lines.
type LineSpan struct {
	Start, End int
}

// NormalizedResult contains staged LF bytes and the consumed original intervals.
// Touched intervals are sorted by Start and remain separate even when adjacent.
type NormalizedResult struct {
	Bytes   []byte
	Touched []LineSpan
}

// StagedSource is the LF staging of one source plus the origin of every emitted
// physical line. Origins aligns with the lines of Bytes: a one-based original
// line number for an unchanged surviving line, or 0 for an authored payload line.
// Nil Origins means the value carries no staging provenance, so RestoreStaged
// falls back to text alignment.
type StagedSource struct {
	Bytes   []byte
	Touched []LineSpan
	Origins []int
}

// UnsupportedOperationError identifies an operation outside the line transform.
// Index is the zero-based index into the supplied operations.
type UnsupportedOperationError struct {
	Index int
	Kind  OpKind
}

func (e *UnsupportedOperationError) Error() string {
	return fmt.Sprintf("operation %d: unsupported operation %q", e.Index+1, e.Kind)
}

type lineEdit struct {
	span  LineSpan
	index int
}

type lineInsertion struct {
	gap   int
	index int
}

// staging accumulates the emitted LF lines of one transform together with the
// original line number each of them came from.
type staging struct {
	bytes   []byte
	origins []int
	touched []LineSpan
}

func (s *staging) writePayload(payload []string) {
	for _, line := range payload {
		s.bytes = append(s.bytes, line...)
		s.bytes = append(s.bytes, '\n')
		s.origins = append(s.origins, 0)
	}
}

func (s *staging) writeSource(text []byte, terminated bool, origin int) {
	s.bytes = append(s.bytes, text...)
	if terminated {
		s.bytes = append(s.bytes, '\n')
	}
	s.origins = append(s.origins, origin)
}

func stageOpError(index int, op Op, reason string) error {
	return fmt.Errorf("operation %d (%s): %s", index+1, op.Kind, reason)
}

// StageLines transforms validated line operations against one original snapshot.
// source must be an intact value returned by Normalize. All coordinates refer to
// that original source; same-gap insertions retain operation order. An empty range
// payload deletes its lines; an insertion needs at least one payload line.
// Callers must resolve blocks and register pastes, capture CUT registers, and
// validate seen state and file operations separately. Insertions consume no source
// lines and are not touched. The returned LF bytes and intervals do not alias
// the inputs.
func StageLines(source Normalized, ops []Op) (NormalizedResult, error) {
	staged, err := stageLines(source, ops)
	if err != nil {
		return NormalizedResult{}, err
	}
	return NormalizedResult{Bytes: staged.bytes, Touched: staged.touched}, nil
}

// StageSource stages validated line operations exactly like StageLines and also
// reports where each emitted line came from, so a later pass can restore exact
// original endings and line visibility without re-aligning the staged text.
func StageSource(source Normalized, ops []Op) (StagedSource, error) {
	staged, err := stageLines(source, ops)
	if err != nil {
		return StagedSource{}, err
	}
	return StagedSource{Bytes: staged.bytes, Touched: staged.touched, Origins: staged.origins}, nil
}

// SeenAfterStaging maps delivered original intervals and authored payload lines
// into the staged view. Unchanged lines that were not seen stay unseen.
func SeenAfterStaging(staged StagedSource, seen []LineSpan) []LineSpan {
	if len(staged.Origins) == 0 {
		return nil
	}
	seen = mergeSeenSpans(append([]LineSpan(nil), seen...))
	var mapped []LineSpan
	for index, origin := range staged.Origins {
		visible := origin == 0
		for _, span := range seen {
			if origin >= span.Start && origin <= span.End {
				visible = true
				break
			}
		}
		if visible {
			mapped = append(mapped, LineSpan{Start: index + 1, End: index + 1})
		}
	}
	return mergeSeenSpans(mapped)
}

// RestoreStaged applies the BOM and line endings to a staging of the same source.
// A surviving unchanged line keeps its exact original ending; authored lines and
// surviving lines that ended the source without a terminator use the dominant ending.
// Provenance is authoritative, so repeated text with mixed endings restores
// without the ambiguity Restore reports for the same bytes. A staging without
// Origins is restored by Restore.
func RestoreStaged(source Normalized, staged StagedSource) ([]byte, error) {
	if !source.ready {
		return nil, errors.New("restore requires a snapshot returned by Normalize")
	}
	if staged.Origins == nil {
		return Restore(source, staged.Bytes)
	}
	if !utf8.Valid(staged.Bytes) {
		return nil, errors.New("edited view is not valid UTF-8")
	}
	lines := viewLines(staged.Bytes)
	if len(lines) != len(staged.Origins) {
		return nil, errors.New("staged origins do not match the staged lines")
	}
	endings := make([]string, len(lines))
	for i := range endings {
		endings[i] = source.dominantEnding
	}
	for i, line := range lines {
		origin := staged.Origins[i]
		if origin == 0 {
			continue
		}
		if origin < 1 || origin > len(source.originalLines) {
			return nil, fmt.Errorf("staged line %d references original line %d of %d", line.outputLine, origin, len(source.originalLines))
		}
		if ending := source.originalLines[origin-1].OriginalEnding; ending != "" {
			endings[i] = ending
		}
	}
	return restoreView(source, lines, endings)
}

func stageLines(source Normalized, ops []Op) (staging, error) {
	if !source.ready || !slices.Equal(source.Bytes, source.original) || !slices.Equal(source.Lines, source.originalLines) {
		return staging{}, fmt.Errorf("staging requires an intact snapshot returned by Normalize")
	}

	lineCount := len(source.originalLines)
	var edits []lineEdit
	var insertions []lineInsertion
	gaps := make(map[int][]int)
	for i, op := range ops {
		switch op.Kind {
		case PUT_RANGE, PUT_INSERT_BEFORE, PUT_INSERT_AFTER, CUT_RANGE:
		default:
			return staging{}, &UnsupportedOperationError{Index: i, Kind: op.Kind}
		}
		if op.Block || op.Target != "" || op.Dest != "" {
			return staging{}, stageOpError(i, op, "unresolved block, target or file operation")
		}
		if op.Kind == CUT_RANGE {
			if len(op.Payload) != 0 || op.Register != "" && !validRegister(op.Register) {
				return staging{}, stageOpError(i, op, "invalid CUT payload or register")
			}
		} else {
			if op.Register != "" {
				return staging{}, stageOpError(i, op, "unresolved register")
			}
			if op.Kind != PUT_RANGE && len(op.Payload) == 0 {
				return staging{}, stageOpError(i, op, "insertion requires at least one payload line")
			}
			for _, line := range op.Payload {
				if !utf8.ValidString(line) || strings.ContainsAny(line, "\r\n") {
					return staging{}, stageOpError(i, op, "payload lines must be valid UTF-8 without CR or LF")
				}
			}
		}

		switch op.Kind {
		case PUT_RANGE, CUT_RANGE:
			if op.Line < 1 || op.End < op.Line || op.End > lineCount {
				return staging{}, stageOpError(i, op, "range is reversed or outside the original source")
			}
			edits = append(edits, lineEdit{span: LineSpan{Start: op.Line, End: op.End}, index: i})
		case PUT_INSERT_BEFORE, PUT_INSERT_AFTER:
			if op.End != 0 {
				return staging{}, stageOpError(i, op, "insertion cannot specify an end line")
			}
			gap := op.Line
			if op.Kind == PUT_INSERT_BEFORE {
				if op.Line < 1 || op.Line > lineCount && !(lineCount == 0 && op.Line == 1) {
					return staging{}, stageOpError(i, op, "insertion anchor is outside the original source")
				}
				gap--
			} else if op.Line == 0 {
				gap = lineCount
			} else if op.Line < 1 || op.Line > lineCount {
				return staging{}, stageOpError(i, op, "insertion anchor is outside the original source")
			}
			insertions = append(insertions, lineInsertion{gap: gap, index: i})
			gaps[gap] = append(gaps[gap], i)
		}
	}

	sort.Slice(edits, func(i, j int) bool { return edits[i].span.Start < edits[j].span.Start })
	for i := 1; i < len(edits); i++ {
		if edits[i].span.Start <= edits[i-1].span.End {
			return staging{}, stageOpError(edits[i].index, ops[edits[i].index], "range overlaps another range")
		}
	}
	for _, insertion := range insertions {
		i := sort.Search(len(edits), func(i int) bool { return edits[i].span.End > insertion.gap })
		if i < len(edits) && edits[i].span.Start <= insertion.gap {
			return staging{}, stageOpError(insertion.index, ops[insertion.index], "insertion falls inside a replaced or deleted range")
		}
	}

	staged := staging{origins: make([]int, 0, lineCount+len(insertions))}
	writeGap := func(gap int) {
		for _, index := range gaps[gap] {
			staged.writePayload(ops[index].Payload)
		}
	}
	for position, editIndex := 0, 0; position < lineCount; {
		writeGap(position)
		if editIndex < len(edits) && edits[editIndex].span.Start == position+1 {
			edit := edits[editIndex]
			staged.writePayload(ops[edit.index].Payload)
			staged.touched = append(staged.touched, edit.span)
			position = edit.span.End
			editIndex++
			continue
		}
		line := source.originalLines[position]
		terminated := line.OriginalEnding != "" || position < lineCount-1 || len(gaps[lineCount]) > 0
		staged.writeSource(source.original[line.Start:line.End], terminated, position+1)
		position++
	}
	writeGap(lineCount)
	return staged, nil
}
