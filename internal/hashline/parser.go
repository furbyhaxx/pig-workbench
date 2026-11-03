package hashline

import (
	"strconv"
	"strings"
)

func parseSection(text string, line int) (Section, error) {
	if len(text) < 2 || text[0] != '[' || text[len(text)-1] != ']' {
		return Section{}, parseError(line, "expected a [path#TAG] header")
	}
	body := text[1 : len(text)-1]
	var section Section
	if strings.HasPrefix(body, "\"") {
		end, escaped := -1, false
		for i := 1; i < len(body); i++ {
			if escaped {
				escaped = false
				continue
			}
			if body[i] == '\\' {
				escaped = true
			} else if body[i] == '"' {
				end = i
				break
			}
		}
		if end < 0 {
			return Section{}, parseError(line, "unclosed quoted path")
		}
		path, err := strconv.Unquote(body[:end+1])
		if err != nil || end+1 >= len(body) || body[end+1] != '#' {
			return Section{}, parseError(line, "invalid quoted path or missing tag")
		}
		section.Path, section.Tag, section.Quoted = path, body[end+2:], true
	} else {
		separator := strings.IndexByte(body, '#')
		if separator < 0 || strings.LastIndexByte(body, '#') != separator {
			return Section{}, parseError(line, "path containing # must be quoted and followed by a tag")
		}
		section.Path, section.Tag = body[:separator], body[separator+1:]
	}
	if section.Path == "" {
		return Section{}, parseError(line, "section path is empty")
	}
	if !tagValid(section.Tag) {
		return Section{}, parseError(line, "section tag must be four hexadecimal characters")
	}
	if section.Header() != text {
		return Section{}, parseError(line, "section header is not canonical; quote paths with spaces or #")
	}
	return section, nil
}

func parseNumber(text string, line int) (int, error) {
	if text == "" || len(text) > 1 && text[0] == '0' {
		return 0, parseError(line, "line numbers must be positive canonical decimal integers")
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return 0, parseError(line, "line numbers must be positive canonical decimal integers")
		}
	}
	n, err := strconv.Atoi(text)
	if err != nil || n <= 0 {
		return 0, parseError(line, "line number is zero or too large")
	}
	return n, nil
}

func parseTarget(text string, line int) (Op, error) {
	var op Op
	switch {
	case strings.HasPrefix(text, "<"):
		op.Kind = PUT_INSERT_BEFORE
		n, err := parseNumber(text[1:], line)
		op.Line = n
		return op, err
	case strings.HasPrefix(text, ">"):
		text = text[1:]
		if text == "$" {
			op.Kind = PUT_INSERT_AFTER
			return op, nil
		}
		op.Kind = PUT_INSERT_AFTER
		if strings.HasSuffix(text, "*") {
			op.Kind, op.Block = PUT_BLOCK_AFTER, true
			text = strings.TrimSuffix(text, "*")
		}
		n, err := parseNumber(text, line)
		op.Line = n
		return op, err
	case strings.HasSuffix(text, "*"):
		op.Kind, op.Block = PUT_BLOCK, true
		n, err := parseNumber(strings.TrimSuffix(text, "*"), line)
		op.Line = n
		return op, err
	default:
		start, end, ok := strings.Cut(text, ".=")
		if !ok || strings.Contains(end, ".=") {
			return Op{}, parseError(line, "expected an insertion, range or block target")
		}
		first, err := parseNumber(start, line)
		if err != nil {
			return Op{}, err
		}
		last, err := parseNumber(end, line)
		if err != nil {
			return Op{}, err
		}
		if last < first {
			return Op{}, parseError(line, "range endpoints are reversed")
		}
		return Op{Kind: PUT_RANGE, Line: first, End: last}, nil
	}
}

func parseRegister(token string, line int) (string, error) {
	if !strings.HasPrefix(token, "@") || !validRegister(token[1:]) {
		return "", parseError(line, "expected a non-empty @register")
	}
	return token[1:], nil
}

func parseOperation(text string, line int) (Op, bool, error) {
	if text == "REM" {
		return Op{Kind: REM}, false, nil
	}
	if text == "MV" || strings.HasPrefix(text, "MV ") {
		if text == "MV" || text == "MV " {
			return Op{}, false, parseError(line, "MV requires a destination")
		}
		dest := strings.TrimPrefix(text, "MV ")
		if strings.HasPrefix(dest, "\"") {
			var err error
			dest, err = strconv.Unquote(dest)
			if err != nil {
				return Op{}, false, parseError(line, "invalid or unclosed quoted destination")
			}
		}
		return Op{Kind: MV, Dest: dest}, false, nil
	}
	isPut := strings.HasPrefix(text, "PUT ")
	isCut := strings.HasPrefix(text, "CUT ")
	if !isPut && !isCut {
		return Op{}, false, parseError(line, "unknown operation")
	}
	parts := strings.Fields(text[4:])
	if len(parts) < 1 || len(parts) > 2 {
		return Op{}, false, parseError(line, "operation requires a target and at most one register")
	}
	payload := isPut && strings.HasSuffix(parts[0], ":")
	target := parts[0]
	if payload {
		target = strings.TrimSuffix(target, ":")
		if len(parts) != 1 {
			return Op{}, false, parseError(line, "payload PUT cannot specify a register")
		}
	}
	op, err := parseTarget(target, line)
	if err != nil {
		return Op{}, false, err
	}
	if len(parts) == 2 {
		op.Register, err = parseRegister(parts[1], line)
		if err != nil {
			return Op{}, false, err
		}
	}
	if isCut {
		switch op.Kind {
		case PUT_RANGE:
			op.Kind = CUT_RANGE
		case PUT_BLOCK:
			op.Kind = CUT_BLOCK
		default:
			return Op{}, false, parseError(line, "CUT requires a range or block")
		}
	} else if !payload {
		op.Target, op.Kind = op.Kind, PUT_FROM_REGISTER
	}
	return op, payload, nil
}

// Parse reads canonical hashline patch syntax without applying any operations.
func Parse(input string) (Document, error) {
	if input == "" {
		return Document{}, parseError(1, "a tagged section is required")
	}
	document := Document{NoFinalNewline: !strings.HasSuffix(input, "\n")}
	lines := strings.Split(input, "\n")
	if !document.NoFinalNewline {
		lines = lines[:len(lines)-1]
	}
	for i := 0; i < len(lines); {
		text, line := lines[i], i+1
		if strings.HasPrefix(text, "[") {
			section, err := parseSection(text, line)
			if err != nil {
				return Document{}, err
			}
			document.Sections = append(document.Sections, section)
			i++
			continue
		}
		if len(document.Sections) == 0 {
			return Document{}, parseError(line, "a tagged section header is required")
		}
		section := &document.Sections[len(document.Sections)-1]
		if strings.HasPrefix(text, "+") {
			if len(section.Ops) > 0 && section.Ops[len(section.Ops)-1].Kind == PUT_FROM_REGISTER {
				return Document{}, parseError(line, "register paste cannot have a payload")
			}
			return Document{}, parseError(line, "payload row requires a colon-form PUT")
		}
		op, payload, err := parseOperation(text, line)
		if err != nil {
			return Document{}, err
		}
		i++
		if payload {
			for i < len(lines) && strings.HasPrefix(lines[i], "+") {
				op.Payload = append(op.Payload, lines[i][1:])
				i++
			}
			if len(op.Payload) == 0 {
				return Document{}, parseError(line, "PUT payload is missing; provide one or more + rows")
			}
		}
		canonical, err := formatOperation(op, line)
		if err != nil {
			return Document{}, err
		}
		if canonical != text {
			return Document{}, parseError(line, "operation is not in canonical form")
		}
		section.Ops = append(section.Ops, op)
	}
	return document, nil
}
