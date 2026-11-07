package host

import (
	"slices"
	"sync"
)

type presentationLayoutKey struct {
	call                                      string
	data, settings                            uint64
	width                                     int
	expanded, partial, hostPartial, hostError bool
	duration                                  int64
	durationKnown                             bool
}
type presentationLayout struct {
	lines    []string
	bytes    int
	sequence uint64
}
type presentationLayouts struct {
	mu       sync.Mutex
	values   map[presentationLayoutKey]presentationLayout
	bytes    int
	sequence uint64
}

func (c *presentationLayouts) render(key presentationLayoutKey, build func() []string) []string {
	c.mu.Lock()
	if cached, ok := c.values[key]; ok {
		c.mu.Unlock()
		return slices.Clone(cached.lines)
	}
	c.mu.Unlock()
	lines := build()
	size := 0
	for _, line := range lines {
		size += len(line)
	}
	if key.call == "" || key.data == 0 || size > 2<<20 {
		return lines
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.values == nil {
		c.values = make(map[presentationLayoutKey]presentationLayout)
	}
	if previous, ok := c.values[key]; ok {
		c.bytes -= previous.bytes
		delete(c.values, key)
	}
	for len(c.values) >= 256 || c.bytes+size > 2<<20 {
		var oldest presentationLayoutKey
		sequence := c.sequence
		found := false
		for candidate, value := range c.values {
			if !found || value.sequence < sequence {
				oldest, sequence, found = candidate, value.sequence, true
			}
		}
		if !found {
			break
		}
		previous := c.values[oldest]
		delete(c.values, oldest)
		c.bytes -= previous.bytes
	}
	c.sequence++
	c.values[key] = presentationLayout{lines: slices.Clone(lines), bytes: size, sequence: c.sequence}
	c.bytes += size
	return lines
}
