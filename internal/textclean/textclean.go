// Package textclean removes control characters from text that is shown or stored as plain text.
package textclean

import (
	"strings"
	"unicode"
)

// HasControl reports whether text contains a control character.
func HasControl(text string) bool {
	return strings.ContainsFunc(text, unicode.IsControl)
}

// StripControl removes every control character except the ones listed in keep.
func StripControl(text string, keep ...rune) string {
	return mapControl(text, -1, keep)
}

// SpaceControl replaces every control character except the ones listed in keep with a space.
func SpaceControl(text string, keep ...rune) string {
	return mapControl(text, ' ', keep)
}

func mapControl(text string, replacement rune, keep []rune) string {
	return strings.Map(func(r rune) rune {
		if !unicode.IsControl(r) {
			return r
		}
		for _, kept := range keep {
			if r == kept {
				return r
			}
		}
		return replacement
	}, text)
}
