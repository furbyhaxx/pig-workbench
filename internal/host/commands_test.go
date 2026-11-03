package host

import (
	"strconv"
	"strings"
	"testing"

	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
)

func TestNoticesFitAndStripUntrustedControls(t *testing.T) {
	hostile := "Workbench\n" + strings.Repeat("界", 100) + "\x1b[2J\x1b]52;c;payload\a\u009b\tEND"
	for _, width := range []int{36, 57, 80, 140} {
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			text := noticeText(hostile, width)
			if !strings.HasPrefix(text, "Workbench\n") {
				t.Fatal("identity lost")
			}
			if strings.ContainsAny(text, "\x1b\a\u009b\t") {
				t.Fatal("unsafe control retained")
			}
			for _, line := range strings.Split(text, "\n") {
				if tui.VisibleWidth(line) > width {
					t.Fatalf("line exceeds width %d", width)
				}
			}
		})
	}
}
