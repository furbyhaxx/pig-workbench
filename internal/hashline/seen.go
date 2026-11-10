package hashline

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// SeenView records fully delivered original source lines in a context epoch.
// Epoch is nonblank UTF-8 without control characters.
type SeenView struct {
	Epoch  string
	Ranges []LineSpan
}

// UnseenLinesError reports sorted, coalesced original source intervals that were not seen.
type UnseenLinesError struct {
	Missing []LineSpan
}

func (e *UnseenLinesError) Error() string {
	spans := make([]string, len(e.Missing))
	for i, span := range e.Missing {
		spans[i] = strconv.Itoa(span.Start)
		if span.End > span.Start {
			spans[i] += "-" + strconv.Itoa(span.End)
		}
	}
	return "source lines " + strings.Join(spans, ", ") + " were not seen in the current epoch; read them before editing"
}

// StaleSeenEpochError reports an invalid, empty or non-current visibility epoch.
type StaleSeenEpochError struct{}

func (e *StaleSeenEpochError) Error() string {
	return "seen epoch is invalid or stale; obtain a fresh source view"
}

func validSeenEpoch(epoch string) bool {
	return utf8.ValidString(epoch) && strings.TrimSpace(epoch) != "" && strings.IndexFunc(epoch, unicode.IsControl) < 0
}

func mergeSeenSpans(spans []LineSpan) []LineSpan {
	sort.Slice(spans, func(i, j int) bool {
		if spans[i].Start != spans[j].Start {
			return spans[i].Start < spans[j].Start
		}
		return spans[i].End < spans[j].End
	})
	merged := spans[:0]
	for _, span := range spans {
		if len(merged) == 0 || span.Start-1 > merged[len(merged)-1].End {
			merged = append(merged, span)
		} else if span.End > merged[len(merged)-1].End {
			merged[len(merged)-1].End = span.End
		}
	}
	return merged
}

func missingSeenSpans(required, seen []LineSpan) []LineSpan {
	var missing []LineSpan
	seenIndex := 0
	for _, span := range required {
		start := span.Start
		for seenIndex < len(seen) && seen[seenIndex].End < start {
			seenIndex++
		}
		i := seenIndex
		covered := false
		for i < len(seen) && seen[i].Start <= span.End {
			if seen[i].Start > start {
				missing = append(missing, LineSpan{Start: start, End: seen[i].Start - 1})
			}
			if seen[i].End >= span.End {
				covered = true
				break
			}
			start = seen[i].End + 1
			i++
		}
		seenIndex = i
		if !covered {
			missing = append(missing, LineSpan{Start: start, End: span.End})
		}
	}
	return missing
}

// CheckSeen authorizes resolved line operations against fully delivered source
// intervals in the current epoch. Explicit <1 and >$ insertions need no seen line;
// other insertions require their addressed line. The caller must bind view to this
// exact intact Normalize snapshot and resolve blocks and register pastes first.
// CUT register capture, file operations and any policy bypass remain caller duties.
func CheckSeen(source Normalized, view SeenView, currentEpoch string, ops []Op) error {
	if !validSeenEpoch(view.Epoch) || !validSeenEpoch(currentEpoch) || view.Epoch != currentEpoch {
		return &StaleSeenEpochError{}
	}
	if !source.ready || !bytes.Equal(source.Bytes, source.original) || !slices.Equal(source.Lines, source.originalLines) {
		return fmt.Errorf("seen validation requires an intact snapshot returned by Normalize")
	}

	lineCount := len(source.originalLines)
	seen := slices.Clone(view.Ranges)
	for _, span := range seen {
		if span.Start < 1 || span.End < span.Start || span.End > lineCount {
			return fmt.Errorf("seen interval %d..%d is outside the original source", span.Start, span.End)
		}
	}
	seen = mergeSeenSpans(seen)

	var required []LineSpan
	for i, op := range ops {
		switch op.Kind {
		case PUT_RANGE, PUT_INSERT_BEFORE, PUT_INSERT_AFTER, CUT_RANGE:
		default:
			return &UnsupportedOperationError{Index: i, Kind: op.Kind}
		}
		if op.Target != "" || op.Dest != "" {
			return stageOpError(i, op, "unresolved target or file operation")
		}
		if _, err := formatTarget(op, op.Kind, i+1); err != nil {
			return stageOpError(i, op, "invalid line operation target")
		}
		if op.Kind == CUT_RANGE {
			if len(op.Payload) != 0 || op.Register != "" && !validRegister(op.Register) {
				return stageOpError(i, op, "invalid CUT payload or register")
			}
		} else {
			if op.Register != "" {
				return stageOpError(i, op, "unresolved register")
			}
			if op.Kind != PUT_RANGE && len(op.Payload) == 0 {
				return stageOpError(i, op, "insertion requires at least one payload line")
			}
			for _, line := range op.Payload {
				if !utf8.ValidString(line) || strings.ContainsAny(line, "\r\n") {
					return stageOpError(i, op, "payload lines must be valid UTF-8 without CR or LF")
				}
			}
		}

		switch op.Kind {
		case PUT_RANGE, CUT_RANGE:
			if op.End > lineCount {
				return stageOpError(i, op, "range is outside the original source")
			}
			required = append(required, LineSpan{Start: op.Line, End: op.End})
		case PUT_INSERT_BEFORE:
			if op.Line > lineCount && !(lineCount == 0 && op.Line == 1) {
				return stageOpError(i, op, "insertion anchor is outside the original source")
			}
			if op.Line != 1 {
				required = append(required, LineSpan{Start: op.Line, End: op.Line})
			}
		case PUT_INSERT_AFTER:
			if op.Line > lineCount {
				return stageOpError(i, op, "insertion anchor is outside the original source")
			}
			if op.Line != 0 {
				required = append(required, LineSpan{Start: op.Line, End: op.Line})
			}
		}
	}

	missing := missingSeenSpans(mergeSeenSpans(required), seen)
	if len(missing) > 0 {
		return &UnseenLinesError{Missing: missing}
	}
	return nil
}
