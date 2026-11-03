package syntax

import (
	"bytes"
	"context"
	"sort"
	"strings"

	ts "github.com/odvcencio/gotreesitter"
)

const (
	minimumBodyLines    = 12
	minimumCommentLines = 8
)

type lineIndex struct {
	source []byte
	starts []int
}

func indexLines(ctx context.Context, source []byte) (lineIndex, error) {
	lines := lineIndex{source: source}
	if len(source) == 0 {
		return lines, nil
	}
	lines.starts = append(lines.starts, 0)
	for i, b := range source {
		if i%65536 == 0 {
			if err := ctx.Err(); err != nil {
				return lineIndex{}, err
			}
		}
		if b == '\n' && i+1 < len(source) {
			lines.starts = append(lines.starts, i+1)
		}
	}
	return lines, nil
}

func (l lineIndex) line(row int) []byte {
	start := l.starts[row]
	end := len(l.source)
	if row+1 < len(l.starts) {
		end = l.starts[row+1] - 1
	}
	return bytes.TrimSuffix(bytes.TrimSuffix(l.source[start:end], []byte{'\n'}), []byte{'\r'})
}

func (l lineIndex) text(first, last int) string {
	end := l.starts[last] + len(l.line(last))
	return string(l.source[l.starts[first]:end])
}

func (l lineIndex) indent(row int) int {
	line := l.line(row)
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return i
}

func endLine(node *ts.Node) int {
	end := node.EndPoint()
	if end.Column == 0 && end.Row > node.StartPoint().Row {
		return int(end.Row)
	}
	return int(end.Row) + 1
}

type nodeFrame struct {
	node           *ts.Node
	kind           string
	next           int
	bad            bool
	inError        bool
	inContinuation bool
	anchor         int
	declaration    bool
	headerLast     int
	body           *ts.Node
	commentStart   int
	commentEnd     int
	heading        int
}

func collect(ctx context.Context, name string, language *ts.Language, root *ts.Node, source []byte) (Summary, map[int]Block, error) {
	lines, err := indexLines(ctx, source)
	if err != nil {
		return Summary{}, nil, err
	}
	summary := Summary{Language: name}
	blocks := make(map[int]Block)
	if root == nil {
		return summary, blocks, nil
	}
	folds := make([]int32, len(lines.starts)+2)
	retained := make([]bool, len(lines.starts)+1)
	var comments [][2]int
	var headings []markdownHeading
	var faults [][2]int
	var stack []nodeFrame
	visited := 0
	push := func(node *ts.Node, isRoot, inError, inContinuation bool) {
		point := node.StartPoint()
		row := int(point.Row)
		last := endLine(node)
		kind := node.Type(language)
		frame := nodeFrame{node: node, kind: kind, bad: node.HasError() || node.IsError() || node.IsMissing(), inError: inError, inContinuation: inContinuation}
		validStart := row < len(lines.starts) && int(point.Column) == lines.indent(row) && int(point.Column) < len(lines.line(row))
		if name == "markdown" && row < len(lines.starts) {
			indent := lines.indent(row)
			level := headingLevel(node, kind, language)
			line := lines.line(row)
			if level > 0 && int(point.Column) <= indent && indent < len(line) && (kind != "atx_heading" || line[indent] == '#') {
				headings = append(headings, markdownHeading{line: row + 1, column: indent + 1, level: level, bad: inError})
				frame.heading = len(headings)
			}
		}
		if name == "markdown" && len(lines.starts) > 0 && (node.IsError() || node.IsMissing()) {
			faults = append(faults, [2]int{min(row+1, len(lines.starts)), min(max(row+1, last), len(lines.starts))})
		}
		if name != "markdown" && !isRoot && validStart && !inContinuation && node.IsNamed() && !node.IsError() && !node.IsMissing() && last > row+1 && eligibleBlock(kind, node) && !strings.ContainsRune("}])", rune(lines.line(row)[point.Column])) {
			line := row + 1
			if _, exists := blocks[line]; !exists {
				blocks[line] = Block{Kind: kind, StartLine: line, EndLine: last, StartColumn: int(point.Column) + 1, Ambiguous: inError}
				frame.anchor = line
			}
		}
		if name != "markdown" && validStart && isDeclaration(kind) {
			body := node.ChildByFieldName("body", language)
			headerLast := last
			if body != nil {
				headerLast = int(body.StartPoint().Row) + 1
				start := int(body.StartByte())
				if start >= len(source) || source[start] != '{' {
					headerLast--
				}
				headerLast = max(row+1, headerLast)
			}
			headerLast = min(headerLast, len(lines.starts))
			if headerLast >= row+1 {
				summary.Declarations = append(summary.Declarations, Declaration{Kind: kind, Line: row + 1, Text: lines.text(row, headerLast-1), Indent: int(point.Column)})
				for line := row + 1; line <= headerLast; line++ {
					retained[line] = true
				}
				frame.declaration, frame.headerLast, frame.body = true, headerLast, body
			}
		}
		if name != "markdown" && validStart && (kind == "comment" || kind == "block_comment" || kind == "line_comment") {
			frame.commentStart, frame.commentEnd = row+1, min(last, len(lines.starts))
		}
		stack = append(stack, frame)
	}
	push(root, true, false, false)
	for len(stack) > 0 {
		visited++
		if visited%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return Summary{}, nil, err
			}
		}
		frame := &stack[len(stack)-1]
		if frame.next < frame.node.ChildCount() {
			child := frame.node.Child(frame.next)
			frame.next++
			if child != nil {
				inContinuation := frame.inContinuation || continuationContainer(frame.kind) && child.StartPoint().Row > frame.node.StartPoint().Row
				push(child, false, frame.inError || frame.node.IsError(), inContinuation)
			}
			continue
		}
		finished := *frame
		stack = stack[:len(stack)-1]
		if finished.anchor != 0 {
			block := blocks[finished.anchor]
			block.Ambiguous = block.Ambiguous || finished.bad
			blocks[finished.anchor] = block
		}
		if finished.heading != 0 {
			headings[finished.heading-1].bad = headings[finished.heading-1].bad || finished.bad
		}
		if !finished.bad && !finished.inError {
			if finished.declaration && finished.body != nil {
				first := finished.headerLast + 1
				last := min(endLine(finished.body), len(lines.starts))
				bodyStart, bodyEnd := int(finished.body.StartByte()), int(finished.body.EndByte())
				if bodyStart < len(source) && source[bodyStart] == '{' && bodyEnd > bodyStart && bodyEnd <= len(source) && source[bodyEnd-1] == '}' {
					last--
				}
				if last-first+1 >= minimumBodyLines {
					folds[first]++
					folds[last+1]--
				}
			}
			if finished.commentStart > 0 {
				comments = append(comments, [2]int{finished.commentStart, finished.commentEnd})
			}
		}
		if len(stack) > 0 {
			stack[len(stack)-1].bad = stack[len(stack)-1].bad || finished.bad
		} else {
			summary.ParseErrors = finished.bad
		}
	}
	if err := ctx.Err(); err != nil {
		return Summary{}, nil, err
	}
	if name == "markdown" {
		if err := markdownBlocks(ctx, headings, faults, len(lines.starts), blocks, summary.ParseErrors); err != nil {
			return Summary{}, nil, err
		}
	}
	foldComments(comments, folds)
	depth := int32(0)
	for line := 1; line <= len(lines.starts); line++ {
		if line%65536 == 0 {
			if err := ctx.Err(); err != nil {
				return Summary{}, nil, err
			}
		}
		depth += folds[line]
		if depth > 0 && !retained[line] {
			if len(summary.Omitted) > 0 && summary.Omitted[len(summary.Omitted)-1][1] == line-1 {
				summary.Omitted[len(summary.Omitted)-1][1] = line
			} else {
				summary.Omitted = append(summary.Omitted, [2]int{line, line})
			}
		}
	}
	return summary, blocks, nil
}

func eligibleBlock(kind string, node *ts.Node) bool {
	switch kind {
	case "block", "statement_block", "compound_statement", "suite", "class_body", "declaration_list", "statement_list", "statement_sequence", "body_statement", "statements", "body", "indented_block", "block_mapping", "block_sequence", "import_list", "hash_literal_body":
		first := node.NamedChild(0)
		return first == nil || first.StartByte() != node.StartByte()
	}
	return !continuationContainer(kind)
}

func continuationContainer(kind string) bool {
	switch kind {
	case "argument_list", "parameter_list", "parameters", "type_arguments", "type_parameters", "parenthesized_expression", "expression_list":
		return true
	}
	return false
}

func isDeclaration(kind string) bool {
	switch kind {
	case "decorator", "annotation", "attribute_item", "declaration", "lexical_declaration":
		return true
	}
	for _, prefix := range []string{"function_", "method_", "class_", "interface_", "record_", "struct_", "enum_", "type_", "trait_", "impl_", "module_", "namespace_", "const_", "var_"} {
		if strings.HasPrefix(kind, prefix) && (strings.HasSuffix(kind, "_declaration") || strings.HasSuffix(kind, "_definition") || strings.HasSuffix(kind, "_item")) {
			return true
		}
	}
	return false
}

func foldComments(comments [][2]int, folds []int32) {
	sort.Slice(comments, func(i, j int) bool { return comments[i][0] < comments[j][0] })
	for i := 0; i < len(comments); {
		first, last := comments[i][0], comments[i][1]
		i++
		for i < len(comments) && comments[i][0] <= last+1 {
			last = max(last, comments[i][1])
			i++
		}
		if last-first+1 >= minimumCommentLines {
			folds[first+1]++
			folds[last]--
		}
	}
}
