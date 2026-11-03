package hashline

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

type oracleLine struct {
	text, ending string
}

func endingOracle(original []oracleLine, edited []string, dominant string) []int {
	best := -1
	masks := make([]int, len(edited))
	choices := make([]string, len(edited))
	for i := range choices {
		choices[i] = dominant
	}
	var visit func(int, int, int)
	visit = func(i, j, matched int) {
		if i == len(original) && j == len(edited) {
			if matched < best {
				return
			}
			if matched > best {
				best = matched
				clear(masks)
			}
			for k, ending := range choices {
				if ending == "\r\n" {
					masks[k] |= 2
				} else {
					masks[k] |= 1
				}
			}
			return
		}
		if i < len(original) {
			visit(i+1, j, matched)
		}
		if j < len(edited) {
			visit(i, j+1, matched)
		}
		if i < len(original) && j < len(edited) && original[i].text == edited[j] {
			choices[j] = original[i].ending
			visit(i+1, j+1, matched+1)
			choices[j] = dominant
		}
	}
	visit(0, 0, 0)
	return masks
}

func TestRestoreAgainstAllSmallAlignments(t *testing.T) {
	for n := 0; n <= 3; n++ {
		for texts := 0; texts < 1<<n; texts++ {
			for styles := 0; styles < 1<<n; styles++ {
				var original []oracleLine
				var source strings.Builder
				crlf := 0
				for i := 0; i < n; i++ {
					text := string(rune('a' + (texts>>i)&1))
					ending := "\n"
					if (styles>>i)&1 != 0 {
						ending = "\r\n"
						crlf++
					}
					original = append(original, oracleLine{text, ending})
					source.WriteString(text + ending)
				}
				dominant := "\n"
				if crlf > n-crlf {
					dominant = "\r\n"
				}
				snapshot, err := Normalize([]byte(source.String()))
				if err != nil {
					t.Fatal(err)
				}
				for m := 0; m <= 3; m++ {
					for bits := 0; bits < 1<<m; bits++ {
						var edited []string
						var view strings.Builder
						for i := 0; i < m; i++ {
							text := string(rune('a' + (bits>>i)&1))
							edited = append(edited, text)
							view.WriteString(text + "\n")
						}
						masks := endingOracle(original, edited, dominant)
						var expected strings.Builder
						ambiguousLine := 0
						for i, mask := range masks {
							if mask == 3 {
								ambiguousLine = i + 1
								break
							}
							expected.WriteString(edited[i])
							if mask == 2 {
								expected.WriteString("\r\n")
							} else {
								expected.WriteByte('\n')
							}
						}
						got, err := Restore(snapshot, []byte(view.String()))
						var ambiguous *AmbiguousEndingError
						if ambiguousLine != 0 {
							if !errors.As(err, &ambiguous) || ambiguous.Line != ambiguousLine {
								t.Fatalf("source %q view %q: got %q, %v; want ambiguous line %d", source.String(), view.String(), got, err, ambiguousLine)
							}
						} else if err != nil || string(got) != expected.String() {
							t.Fatal(fmt.Sprintf("source %q view %q: got %q, %v; want %q", source.String(), view.String(), got, err, expected.String()))
						}
					}
				}
			}
		}
	}
}
