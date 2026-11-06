package instructions

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var statusLine = regexp.MustCompile(`^\s*status:\s*"?([^"\s#]+)"?\s*$`)

// maxPointerTargetBytes bounds how much of a pointer target is read to find its frontmatter status.
const maxPointerTargetBytes = 16 << 10

// AnnotatePointers marks every backlog pointer whose target is missing or whose status is not
// "open". Targets resolve against root and cannot escape it; the view keeps every line.
func AnnotatePointers(root, content string) string {
	lines := strings.SplitAfter(content, "\n")
	for i, line := range lines {
		plain := strings.TrimSuffix(line, "\n")
		if !pointerPattern.MatchString(plain) {
			continue
		}
		_, target, _ := strings.Cut(plain, " → ")
		if note := pointerNote(root, strings.TrimSpace(target)); note != "" {
			lines[i] = plain + " [" + note + "]" + strings.TrimPrefix(line, plain)
		}
	}
	return strings.Join(lines, "")
}

func pointerNote(root, target string) string {
	if !filepath.IsLocal(target) {
		return "target outside the project"
	}
	file, err := os.OpenInRoot(root, target)
	if err != nil {
		return "target missing"
	}
	defer file.Close()
	if info, err := file.Stat(); err != nil || !info.Mode().IsRegular() {
		return "target missing"
	}
	head, err := io.ReadAll(io.LimitReader(file, maxPointerTargetBytes))
	if err != nil {
		return "target unreadable"
	}
	status := frontmatterStatus(string(head))
	switch status {
	case "open":
		return ""
	case "":
		return "target has no status"
	}
	return "target status: " + status
}

func frontmatterStatus(text string) string {
	text = strings.ReplaceAll(strings.TrimPrefix(text, "\ufeff"), "\r\n", "\n")
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return ""
	}
	front, _, closed := strings.Cut(rest, "\n---")
	if !closed {
		return ""
	}
	for _, line := range strings.Split(front, "\n") {
		if m := statusLine.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	return ""
}
