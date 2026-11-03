// Package resource implements Workbench paths, selectors and bounded resource access.
package resource

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Range is an inclusive one-based source span.
type Range struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Selection retains absolute, onward and tail ranges before source size is known.
type Selection struct {
	Ranges   []Range `json:"ranges,omitempty"`
	Tails    []int   `json:"tails,omitempty"`
	Raw      bool    `json:"raw"`
	Explicit bool    `json:"explicit"`
}

// ParseSelector parses the one shared line-selector grammar.
func ParseSelector(input string) (Selection, error) {
	s := Selection{Explicit: true}
	input = strings.TrimPrefix(input, ":")
	if input == "" {
		return s, errors.New("selector is empty")
	}
	pieces := strings.Split(input, ":")
	numeric := ""
	for _, p := range pieces {
		if p == "raw" {
			if s.Raw {
				return s, errors.New("raw selector repeated")
			}
			s.Raw = true
		} else {
			if numeric != "" || p == "" {
				return s, fmt.Errorf("invalid selector %q", input)
			}
			numeric = p
		}
	}
	if numeric == "" {
		if s.Raw {
			return s, nil
		}
		return s, errors.New("numeric selector required")
	}
	items := strings.Split(numeric, ",")
	for _, item := range items {
		if strings.HasPrefix(item, "-") {
			n, err := positive(strings.TrimPrefix(item, "-"))
			if err != nil {
				return s, err
			}
			s.Tails = append(s.Tails, n)
			continue
		}
		if a, b, ok := strings.Cut(item, "+"); ok {
			start, err := positive(a)
			if err != nil {
				return s, err
			}
			count, err := positive(b)
			if err != nil {
				return s, err
			}
			if count > maxInt()-start+1 {
				return s, errors.New("selector count overflows")
			}
			s.Ranges = append(s.Ranges, Range{Start: start, End: start + count - 1})
			continue
		}
		if a, b, ok := strings.Cut(item, "-"); ok {
			start, err := positive(a)
			if err != nil {
				return s, err
			}
			end := 0
			if b != "" {
				end, err = positive(b)
				if err != nil {
					return s, err
				}
				if end < start {
					return s, errors.New("selector range is reversed")
				}
			}
			s.Ranges = append(s.Ranges, Range{Start: start, End: end})
			continue
		}
		n, err := positive(item)
		if err != nil {
			return s, err
		}
		end := n
		if len(items) == 1 && !s.Raw {
			end = 0
		}
		s.Ranges = append(s.Ranges, Range{Start: n, End: end})
	}
	return s, nil
}

// Resolve expands and merges ranges, with ordinary non-raw context when requested.
func (s Selection) Resolve(lines int, context bool) ([]Range, error) {
	if lines < 0 {
		return nil, errors.New("negative source line count")
	}
	if lines == 0 {
		if len(s.Ranges) > 0 {
			return nil, errors.New("selection starts beyond empty source")
		}
		return nil, nil
	}
	ranges := append([]Range(nil), s.Ranges...)
	for _, n := range s.Tails {
		start := lines - n + 1
		if start < 1 {
			start = 1
		}
		ranges = append(ranges, Range{Start: start, End: lines})
	}
	if len(ranges) == 0 {
		ranges = []Range{{Start: 1, End: lines}}
	}
	for i, r := range ranges {
		if r.Start > lines {
			return nil, fmt.Errorf("selection starts at %d beyond EOF (%d lines)", r.Start, lines)
		}
		if r.End == 0 || r.End > lines {
			r.End = lines
		}
		if context && !s.Raw && len(s.Ranges) == 1 && len(s.Tails) == 0 {
			if r.Start > 1 {
				r.Start--
			}
			if lines-r.End < 3 {
				r.End = lines
			} else {
				r.End += 3
			}
		}
		ranges[i] = r
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].Start < ranges[j].Start })
	merged := make([]Range, 0, len(ranges))
	for _, r := range ranges {
		if len(merged) > 0 && r.Start-1 <= merged[len(merged)-1].End {
			if r.End > merged[len(merged)-1].End {
				merged[len(merged)-1].End = r.End
			}
		} else {
			merged = append(merged, r)
		}
	}
	return merged, nil
}

func positive(s string) (int, error) {
	if s == "" {
		return 0, errors.New("selector needs a positive integer")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("invalid line number %q", s)
		}
	}
	n, err := strconv.ParseUint(s, 10, 63)
	if err != nil || n == 0 || n > uint64(maxInt()) {
		return 0, fmt.Errorf("invalid or overflowing line number %q", s)
	}
	return int(n), nil
}

func maxInt() int { return int(^uint(0) >> 1) }
