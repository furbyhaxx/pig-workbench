package host

import "bytes"

// terminalText reduces command output to what a terminal would leave on screen as text: escape
// sequences and control bytes are dropped, CRLF becomes LF, and a line redrawn with carriage
// returns keeps its last drawing. The stored log keeps the raw bytes.
func terminalText(raw []byte) []byte {
	out := make([]byte, 0, len(raw))
	lineStart := 0
	for i := 0; i < len(raw); i++ {
		b := raw[i]
		switch {
		case b == 0x1b:
			i = skipEscape(raw, i)
		case b == '\r':
			if i+1 < len(raw) && raw[i+1] == '\n' {
				continue
			}
			if rest := bytes.TrimLeft(raw[i+1:], "\r"); len(rest) > 0 && rest[0] != '\n' {
				out = out[:lineStart]
			}
		case b == '\n':
			out = append(out, b)
			lineStart = len(out)
		case b == '\t' || b >= 0x20 && b != 0x7f:
			out = append(out, b)
		}
	}
	return out
}

// skipEscape returns the index of the last byte of the escape sequence starting at i.
func skipEscape(raw []byte, i int) int {
	if i+1 >= len(raw) {
		return i
	}
	switch raw[i+1] {
	case '[':
		for j := i + 2; j < len(raw); j++ {
			if raw[j] >= 0x40 && raw[j] <= 0x7e {
				return j
			}
		}
		return len(raw) - 1
	case ']', 'P', '_', '^', 'X':
		for j := i + 2; j < len(raw); j++ {
			if raw[j] == 0x07 {
				return j
			}
			if raw[j] == 0x1b && j+1 < len(raw) && raw[j+1] == '\\' {
				return j + 1
			}
		}
		return len(raw) - 1
	}
	return i + 1
}
