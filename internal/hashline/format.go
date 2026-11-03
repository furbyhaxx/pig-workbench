package hashline

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

func tagValid(tag string) bool {
	if len(tag) != 4 {
		return false
	}
	for i := 0; i < len(tag); i++ {
		c := tag[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func parseError(line int, message string) error {
	return &ParseError{Line: line, Message: message}
}

func validRegister(name string) bool {
	return name != "" && strings.IndexFunc(name, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) < 0
}

func formatTarget(op Op, kind OpKind, line int) (string, error) {
	if op.Line < 0 || op.End < 0 {
		return "", parseError(line, "line numbers must be positive")
	}
	switch kind {
	case PUT_RANGE, CUT_RANGE:
		if op.Line == 0 || op.End < op.Line || op.Block {
			return "", parseError(line, "range must have positive, ordered endpoints")
		}
		return fmt.Sprintf("%d.=%d", op.Line, op.End), nil
	case PUT_INSERT_BEFORE, PUT_INSERT_AFTER:
		if op.End != 0 || op.Block || (op.Line == 0 && kind != PUT_INSERT_AFTER) {
			return "", parseError(line, "invalid insertion anchor")
		}
		if kind == PUT_INSERT_BEFORE {
			return "<" + strconv.Itoa(op.Line), nil
		}
		if op.Line == 0 {
			return ">$", nil
		}
		return ">" + strconv.Itoa(op.Line), nil
	case PUT_BLOCK, PUT_BLOCK_AFTER, CUT_BLOCK:
		if op.Line == 0 || op.End != 0 || !op.Block {
			return "", parseError(line, "invalid block anchor")
		}
		target := strconv.Itoa(op.Line) + "*"
		if kind == PUT_BLOCK_AFTER {
			target = ">" + target
		}
		return target, nil
	default:
		return "", parseError(line, "invalid operation target")
	}
}

func formatOperation(op Op, line int) (string, error) {
	if op.Kind != PUT_FROM_REGISTER && op.Target != "" {
		return "", parseError(line, "only a register paste can specify Target")
	}
	if op.Register != "" && !validRegister(op.Register) {
		return "", parseError(line, "invalid register name")
	}
	if op.Kind == REM || op.Kind == MV {
		if op.Line != 0 || op.End != 0 || op.Block || op.Register != "" || len(op.Payload) != 0 {
			return "", parseError(line, "file operation cannot have an anchor, register or payload")
		}
		if op.Kind == REM {
			if op.Dest != "" {
				return "", parseError(line, "REM cannot have a destination")
			}
			return "REM", nil
		}
		if op.Dest == "" {
			return "", parseError(line, "MV requires a destination")
		}
		dest := op.Dest
		if needsQuotes(dest) {
			dest = strconv.Quote(dest)
		}
		return "MV " + dest, nil
	}
	if op.Dest != "" {
		return "", parseError(line, "only MV can have a destination")
	}
	kind := op.Kind
	if kind == PUT_FROM_REGISTER {
		kind = op.Target
		switch kind {
		case PUT_RANGE, PUT_INSERT_BEFORE, PUT_INSERT_AFTER, PUT_BLOCK, PUT_BLOCK_AFTER:
		default:
			return "", parseError(line, "register paste requires a PUT target")
		}
		if len(op.Payload) != 0 {
			return "", parseError(line, "register paste cannot have a payload")
		}
		if op.Line == 0 {
			return "", parseError(line, "register paste requires a numbered target")
		}
		if op.Register == "" && (kind == PUT_RANGE || kind == PUT_BLOCK) {
			return "", parseError(line, "range and block pastes require a named register")
		}
	}
	target, err := formatTarget(op, kind, line)
	if err != nil {
		return "", err
	}
	switch op.Kind {
	case PUT_FROM_REGISTER:
		if op.Register != "" {
			target += " @" + op.Register
		}
		return "PUT " + target, nil
	case PUT_RANGE, PUT_INSERT_BEFORE, PUT_INSERT_AFTER, PUT_BLOCK, PUT_BLOCK_AFTER:
		if op.Register != "" || len(op.Payload) == 0 {
			return "", parseError(line, "PUT payload form requires payload rows and no register")
		}
		for _, row := range op.Payload {
			if strings.Contains(row, "\n") {
				return "", parseError(line, "payload rows cannot contain a newline")
			}
		}
		return "PUT " + target + ":", nil
	case CUT_RANGE, CUT_BLOCK:
		if len(op.Payload) != 0 {
			return "", parseError(line, "CUT cannot have a payload")
		}
		if op.Register != "" {
			target += " @" + op.Register
		}
		return "CUT " + target, nil
	default:
		return "", parseError(line, "unknown operation")
	}
}

// Format validates and renders a document in canonical patch syntax.
func Format(document Document) (string, error) {
	if len(document.Sections) == 0 {
		return "", parseError(1, "a tagged section is required")
	}
	var lines []string
	for _, section := range document.Sections {
		line := len(lines) + 1
		if section.Path == "" {
			return "", parseError(line, "section path is empty")
		}
		if !tagValid(section.Tag) {
			return "", parseError(line, "section tag must be four hexadecimal characters")
		}
		lines = append(lines, section.Header())
		for _, op := range section.Ops {
			header, err := formatOperation(op, len(lines)+1)
			if err != nil {
				return "", err
			}
			lines = append(lines, header)
			for _, row := range op.Payload {
				lines = append(lines, "+"+row)
			}
		}
	}
	output := strings.Join(lines, "\n")
	if !document.NoFinalNewline {
		output += "\n"
	}
	return output, nil
}
