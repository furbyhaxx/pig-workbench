package syntax

import (
	"context"
	"sort"

	ts "github.com/odvcencio/gotreesitter"
)

type markdownHeading struct {
	line, column, level int
	bad                 bool
}

func headingLevel(node *ts.Node, kind string, language *ts.Language) int {
	if kind != "atx_heading" && kind != "setext_heading" {
		return 0
	}
	for i := 0; i < node.NamedChildCount(); i++ {
		switch node.NamedChild(i).Type(language) {
		case "atx_h1_marker", "setext_h1_underline":
			return 1
		case "atx_h2_marker", "setext_h2_underline":
			return 2
		case "atx_h3_marker":
			return 3
		case "atx_h4_marker":
			return 4
		case "atx_h5_marker":
			return 5
		case "atx_h6_marker":
			return 6
		}
	}
	return 0
}

func markdownBlocks(ctx context.Context, headings []markdownHeading, faults [][2]int, total int, blocks map[int]Block, parseErrors bool) error {
	sort.Slice(faults, func(i, j int) bool { return faults[i][0] < faults[j][0] })
	for i := 1; i < len(faults); i++ {
		faults[i][1] = max(faults[i][1], faults[i-1][1])
	}
	ends := make([]int, len(headings))
	pending := make([]int, 0, len(headings))
	for i, heading := range headings {
		if i%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		ends[i] = total
		for len(pending) > 0 && headings[pending[len(pending)-1]].level >= heading.level {
			index := pending[len(pending)-1]
			pending = pending[:len(pending)-1]
			ends[index] = heading.line - 1
		}
		pending = append(pending, i)
	}
	for i, heading := range headings {
		if ends[i] <= heading.line {
			continue
		}
		fault := sort.Search(len(faults), func(j int) bool { return faults[j][0] > ends[i] })
		ambiguous := heading.bad || fault > 0 && faults[fault-1][1] >= heading.line || parseErrors && len(faults) == 0
		blocks[heading.line] = Block{Kind: "section", StartLine: heading.line, EndLine: ends[i], StartColumn: heading.column, Ambiguous: ambiguous}
	}
	return ctx.Err()
}

// foldSections lists sound headings as declarations and folds the text between them when it reaches minimum lines.
func foldSections(headings []markdownHeading, lines lineIndex, minimum int, summary *Summary, folds []int32, retained []bool) {
	previous := 0
	fold := func(next int) {
		if next-previous-1 >= minimum {
			folds[previous+1]++
			folds[next]--
		}
	}
	for _, heading := range headings {
		if heading.bad || heading.line <= previous || heading.line > len(lines.starts) {
			continue
		}
		summary.Declarations = append(summary.Declarations, Declaration{Kind: "heading", Line: heading.line, Text: lines.text(heading.line-1, heading.line-1), Indent: heading.column - 1})
		retained[heading.line] = true
		fold(heading.line)
		previous = heading.line
	}
	fold(len(lines.starts) + 1)
}
