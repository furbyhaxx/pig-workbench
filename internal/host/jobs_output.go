package host

import (
	"bytes"

	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
)

// terminalText reduces command output to what a terminal would leave on screen as text: escape
// sequences and control bytes are dropped, CRLF becomes LF, and a line redrawn with carriage
// returns keeps its last drawing. The stored log keeps the raw bytes.
func terminalText(raw []byte) []byte {
	raw = []byte(tui.StripTerminalSequences(string(raw)))
	out := make([]byte, 0, len(raw))
	lineStart := 0
	for i := 0; i < len(raw); i++ {
		b := raw[i]
		switch {
		case b == 0x1b && i+1 < len(raw) && raw[i+1] == '[':
			// A sequence cut off by the page end: drop its parameters with it.
			for i++; i+1 < len(raw) && raw[i+1] >= 0x20 && raw[i+1] <= 0x3f; i++ {
			}
		case b == '\r':
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
