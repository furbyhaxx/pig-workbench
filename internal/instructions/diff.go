package instructions

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrDiffMismatch reports a diff that does not apply to the supplied base.
var ErrDiffMismatch = errors.New("instruction diff does not apply to its base")

type diffOp struct {
	kind byte
	text string
}

// UnifiedDiff returns a unified diff turning old into new with the given context lines, or "" when
// they are equal. Texts are compared as "\n"-separated lines so Apply reproduces new byte for byte.
func UnifiedDiff(old, new string, context int) string {
	if old == new {
		return ""
	}
	if context < 0 {
		context = 0
	}
	a, b := strings.Split(old, "\n"), strings.Split(new, "\n")
	ops := diffLines(a, b)
	var out strings.Builder
	out.WriteString("--- delivered view\n+++ current view\n")
	i := 0
	for i < len(ops) {
		for i < len(ops) && ops[i].kind == ' ' {
			i++
		}
		if i >= len(ops) {
			break
		}
		start := max(0, i-context)
		last := i
		for k := i; k < len(ops) && k-last <= 2*context; k++ {
			if ops[k].kind != ' ' {
				last = k
			}
		}
		stop := min(len(ops), last+1+context)
		hunk := ops[start:stop]
		aStart, bStart := 1, 1
		for _, op := range ops[:start] {
			if op.kind != '+' {
				aStart++
			}
			if op.kind != '-' {
				bStart++
			}
		}
		aLen, bLen := 0, 0
		for _, op := range hunk {
			if op.kind != '+' {
				aLen++
			}
			if op.kind != '-' {
				bLen++
			}
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n", aStart, aLen, bStart, bLen)
		for _, op := range hunk {
			out.WriteByte(op.kind)
			out.WriteString(op.text)
			out.WriteByte('\n')
		}
		i = stop
	}
	return out.String()
}

// ApplyDiff applies a diff made by UnifiedDiff to base.
func ApplyDiff(base, diff string) (string, error) {
	lines := strings.Split(base, "\n")
	var out []string
	pos := 0
	diffLines := strings.Split(strings.TrimSuffix(diff, "\n"), "\n")
	for i := 0; i < len(diffLines); i++ {
		line := diffLines[i]
		if !strings.HasPrefix(line, "@@ -") {
			continue
		}
		var aStart, aLen int
		header, _, _ := strings.Cut(strings.TrimPrefix(line, "@@ -"), " +")
		startText, lenText, _ := strings.Cut(header, ",")
		var err error
		if aStart, err = strconv.Atoi(startText); err != nil {
			return "", ErrDiffMismatch
		}
		if aLen, err = strconv.Atoi(lenText); err != nil {
			return "", ErrDiffMismatch
		}
		target := aStart - 1
		if target < pos || target > len(lines) {
			return "", ErrDiffMismatch
		}
		out = append(out, lines[pos:target]...)
		pos = target
		consumed := 0
		for i+1 < len(diffLines) && !strings.HasPrefix(diffLines[i+1], "@@ -") {
			i++
			body := diffLines[i]
			if body == "" {
				return "", ErrDiffMismatch
			}
			switch body[0] {
			case ' ', '-':
				if pos >= len(lines) || lines[pos] != body[1:] {
					return "", ErrDiffMismatch
				}
				if body[0] == ' ' {
					out = append(out, lines[pos])
				}
				pos++
				consumed++
			case '+':
				out = append(out, body[1:])
			default:
				return "", ErrDiffMismatch
			}
		}
		if consumed != aLen {
			return "", ErrDiffMismatch
		}
	}
	out = append(out, lines[pos:]...)
	return strings.Join(out, "\n"), nil
}

// diffLines computes a minimal line edit script by longest common subsequence.
func diffLines(a, b []string) []diffOp {
	table := make([][]int, len(a)+1)
	for i := range table {
		table[i] = make([]int, len(b)+1)
	}
	for i := len(a) - 1; i >= 0; i-- {
		for j := len(b) - 1; j >= 0; j-- {
			if a[i] == b[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}
	var ops []diffOp
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i]})
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			ops = append(ops, diffOp{'-', a[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j]})
			j++
		}
	}
	for ; i < len(a); i++ {
		ops = append(ops, diffOp{'-', a[i]})
	}
	for ; j < len(b); j++ {
		ops = append(ops, diffOp{'+', b[j]})
	}
	return ops
}
