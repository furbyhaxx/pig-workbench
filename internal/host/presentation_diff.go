package host

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/furbyhaxx/pig-workbench/internal/instructions"
	"github.com/furbyhaxx/pig-workbench/internal/presentation"
	"github.com/furbyhaxx/pig-workbench/internal/textclean"
)

const (
	presentationBlocksKey = "workbenchBlocks"
	diffContextLines      = 3
	// The line diff is quadratic, so a changed region beyond this many lines carries identities only.
	maxDiffWindowLines = 2000
	maxDiffHunkLines   = 200
	maxDiffTextBytes   = 24 << 10
	maxDiffLineBytes   = 300
)

// diffBlock describes one file operation; hunks are present only when both sides are text and the change is small enough to diff.
func diffBlock(operation presentation.DiffOperation, before, after *presentation.FileIdentity, old, new []byte) presentation.Block {
	diff := &presentation.DiffBlock{Operation: operation, Before: before, After: after}
	if old != nil || new != nil {
		diff.Hunks = diffHunks(string(old), string(new))
	}
	return presentation.Block{Kind: presentation.BlockDiff, Diff: diff}
}

func diffHunks(old, new string) []presentation.Hunk {
	if old == new || !utf8.ValidString(old) || !utf8.ValidString(new) {
		return nil
	}
	a, b := strings.Split(old, "\n"), strings.Split(new, "\n")
	prefix := 0
	for prefix < len(a) && prefix < len(b) && a[prefix] == b[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(a)-prefix && suffix < len(b)-prefix && a[len(a)-1-suffix] == b[len(b)-1-suffix] {
		suffix++
	}
	skipped := max(0, prefix-diffContextLines)
	kept := min(suffix, diffContextLines)
	a, b = a[skipped:len(a)-suffix+kept], b[skipped:len(b)-suffix+kept]
	if len(a) > maxDiffWindowLines || len(b) > maxDiffWindowLines {
		return nil
	}
	var hunks []presentation.Hunk
	lines, bytes := 0, 0
	for _, row := range strings.Split(strings.TrimSuffix(instructions.UnifiedDiff(strings.Join(a, "\n"), strings.Join(b, "\n"), diffContextLines), "\n"), "\n") {
		if strings.HasPrefix(row, "@@ ") {
			if lines >= maxDiffHunkLines || bytes >= maxDiffTextBytes {
				break
			}
			hunk, ok := hunkHeader(row, skipped)
			if !ok {
				return nil
			}
			hunks = append(hunks, hunk)
			continue
		}
		if len(hunks) == 0 || row == "" || lines >= maxDiffHunkLines || bytes >= maxDiffTextBytes {
			continue
		}
		kind := presentation.HunkContext
		switch row[0] {
		case '+':
			kind = presentation.HunkAdd
		case '-':
			kind = presentation.HunkRemove
		}
		text := diffLineText(row[1:])
		last := &hunks[len(hunks)-1]
		last.Lines = append(last.Lines, presentation.HunkLine{Kind: kind, Text: text})
		lines++
		bytes += len(text)
	}
	if len(hunks) > 0 && len(hunks[len(hunks)-1].Lines) == 0 {
		hunks = hunks[:len(hunks)-1]
	}
	return hunks
}

func hunkHeader(row string, offset int) (presentation.Hunk, bool) {
	fields := strings.Fields(row)
	if len(fields) < 3 {
		return presentation.Hunk{}, false
	}
	oldStart, oldLines, okOld := hunkRange(fields[1], "-")
	newStart, newLines, okNew := hunkRange(fields[2], "+")
	return presentation.Hunk{OldStart: oldStart + offset, OldLines: oldLines, NewStart: newStart + offset, NewLines: newLines}, okOld && okNew
}

func hunkRange(field, sign string) (int, int, bool) {
	start, count, ok := strings.Cut(strings.TrimPrefix(field, sign), ",")
	first, errFirst := strconv.Atoi(start)
	length, errLength := strconv.Atoi(count)
	return first, length, ok && errFirst == nil && errLength == nil
}

func diffLineText(text string) string {
	text = textclean.StripControl(text, '\t')
	if len(text) <= maxDiffLineBytes {
		return text
	}
	cut := maxDiffLineBytes
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}

func fileIdentity(path, revision string) *presentation.FileIdentity {
	if path == "" {
		return nil
	}
	return &presentation.FileIdentity{Path: path, Revision: revision}
}
