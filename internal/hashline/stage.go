package hashline

import (
	"bytes"
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

func stageOpError(index int, op Op, reason string) error {
	return fmt.Errorf("operation %d (%s): %s", index+1, op.Kind, reason)
}

func writeStagedPayload(output *bytes.Buffer, payload []string) {
	for _, line := range payload {
		output.WriteString(line)
		output.WriteByte('\n')
	}
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
	if !source.ready || !bytes.Equal(source.Bytes, source.original) || !slices.Equal(source.Lines, source.originalLines) {
		return NormalizedResult{}, fmt.Errorf("staging requires an intact snapshot returned by Normalize")
	}

	lineCount := len(source.originalLines)
	var edits []lineEdit
	var insertions []lineInsertion
	gaps := make(map[int][]int)
	for i, op := range ops {
		switch op.Kind {
		case PUT_RANGE, PUT_INSERT_BEFORE, PUT_INSERT_AFTER, CUT_RANGE:
		default:
			return NormalizedResult{}, &UnsupportedOperationError{Index: i, Kind: op.Kind}
		}
		if op.Block || op.Target != "" || op.Dest != "" {
			return NormalizedResult{}, stageOpError(i, op, "unresolved block, target or file operation")
		}
		if op.Kind == CUT_RANGE {
			if len(op.Payload) != 0 || op.Register != "" && !validRegister(op.Register) {
				return NormalizedResult{}, stageOpError(i, op, "invalid CUT payload or register")
			}
		} else {
			if op.Register != "" {
				return NormalizedResult{}, stageOpError(i, op, "unresolved register")
			}
			if op.Kind != PUT_RANGE && len(op.Payload) == 0 {
				return NormalizedResult{}, stageOpError(i, op, "insertion requires at least one payload line")
			}
			for _, line := range op.Payload {
				if !utf8.ValidString(line) || strings.ContainsAny(line, "\r\n") {
					return NormalizedResult{}, stageOpError(i, op, "payload lines must be valid UTF-8 without CR or LF")
				}
			}
		}

		switch op.Kind {
		case PUT_RANGE, CUT_RANGE:
			if op.Line < 1 || op.End < op.Line || op.End > lineCount {
				return NormalizedResult{}, stageOpError(i, op, "range is reversed or outside the original source")
			}
			edits = append(edits, lineEdit{span: LineSpan{Start: op.Line, End: op.End}, index: i})
		case PUT_INSERT_BEFORE, PUT_INSERT_AFTER:
			if op.End != 0 {
				return NormalizedResult{}, stageOpError(i, op, "insertion cannot specify an end line")
			}
			gap := op.Line
			if op.Kind == PUT_INSERT_BEFORE {
				if op.Line < 1 || op.Line > lineCount && !(lineCount == 0 && op.Line == 1) {
					return NormalizedResult{}, stageOpError(i, op, "insertion anchor is outside the original source")
				}
				gap--
			} else if op.Line == 0 {
				gap = lineCount
			} else if op.Line < 1 || op.Line > lineCount {
				return NormalizedResult{}, stageOpError(i, op, "insertion anchor is outside the original source")
			}
			insertions = append(insertions, lineInsertion{gap: gap, index: i})
			gaps[gap] = append(gaps[gap], i)
		}
	}

	sort.Slice(edits, func(i, j int) bool { return edits[i].span.Start < edits[j].span.Start })
	for i := 1; i < len(edits); i++ {
		if edits[i].span.Start <= edits[i-1].span.End {
			return NormalizedResult{}, stageOpError(edits[i].index, ops[edits[i].index], "range overlaps another range")
		}
	}
	for _, insertion := range insertions {
		i := sort.Search(len(edits), func(i int) bool { return edits[i].span.End > insertion.gap })
		if i < len(edits) && edits[i].span.Start <= insertion.gap {
			return NormalizedResult{}, stageOpError(insertion.index, ops[insertion.index], "insertion falls inside a replaced or deleted range")
		}
	}

	var output bytes.Buffer
	writeGap := func(gap int) {
		for _, index := range gaps[gap] {
			writeStagedPayload(&output, ops[index].Payload)
		}
	}
	var touched []LineSpan
	for position, editIndex := 0, 0; position < lineCount; {
		writeGap(position)
		if editIndex < len(edits) && edits[editIndex].span.Start == position+1 {
			edit := edits[editIndex]
			writeStagedPayload(&output, ops[edit.index].Payload)
			touched = append(touched, edit.span)
			position = edit.span.End
			editIndex++
			continue
		}
		line := source.originalLines[position]
		output.Write(source.original[line.Start:line.End])
		if line.OriginalEnding != "" || position < lineCount-1 || len(gaps[lineCount]) > 0 {
			output.WriteByte('\n')
		}
		position++
	}
	writeGap(lineCount)
	return NormalizedResult{Bytes: output.Bytes(), Touched: touched}, nil
}
