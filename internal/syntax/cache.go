package syntax

import (
	"container/list"
	"sync"

	ts "github.com/odvcencio/gotreesitter"
)

const (
	maxCacheEntries = 256
	maxCacheBytes   = 32 * 1024 * 1024
)

type cacheKey struct {
	identity [32]byte
	revision [32]byte
	content  [32]byte
	language *ts.Language
	grammar  string
	version  uint32
	folding  Folding
}

type cacheEntry struct {
	key     cacheKey
	summary Summary
	blocks  map[int]Block
	bytes   int
}

type resultCache struct {
	mu      sync.Mutex
	entries map[cacheKey]*list.Element
	lru     list.List
	bytes   int
}

func newResultCache() resultCache {
	return resultCache{entries: make(map[cacheKey]*list.Element)}
}

func (c *resultCache) get(key cacheKey) (Summary, map[int]Block, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if element := c.entries[key]; element != nil {
		c.lru.MoveToFront(element)
		entry := element.Value.(*cacheEntry)
		summary, blocks := cloneResult(entry.summary, entry.blocks)
		return summary, blocks, true
	}
	return Summary{}, nil, false
}

func (c *resultCache) put(key cacheKey, summary Summary, blocks map[int]Block) {
	size := 512 + len(key.grammar) + len(summary.Language) + len(summary.Omitted)*16 + len(blocks)*128
	for _, declaration := range summary.Declarations {
		size += 96 + len(declaration.Kind) + len(declaration.Text)
	}
	for _, block := range blocks {
		size += len(block.Kind)
	}
	if size > maxCacheBytes {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing := c.entries[key]; existing != nil {
		c.lru.MoveToFront(existing)
		return
	}
	for c.lru.Len() >= maxCacheEntries || c.bytes+size > maxCacheBytes {
		oldest := c.lru.Back()
		entry := oldest.Value.(*cacheEntry)
		delete(c.entries, entry.key)
		c.bytes -= entry.bytes
		c.lru.Remove(oldest)
	}
	copySummary, copyBlocks := cloneResult(summary, blocks)
	entry := &cacheEntry{key: key, summary: copySummary, blocks: copyBlocks, bytes: size}
	c.entries[key] = c.lru.PushFront(entry)
	c.bytes += size
}

func cloneResult(summary Summary, blocks map[int]Block) (Summary, map[int]Block) {
	if summary.Declarations != nil {
		summary.Declarations = append([]Declaration{}, summary.Declarations...)
	}
	if summary.Omitted != nil {
		summary.Omitted = append([][2]int{}, summary.Omitted...)
	}
	copyBlocks := make(map[int]Block, len(blocks))
	for line, block := range blocks {
		copyBlocks[line] = block
	}
	return summary, copyBlocks
}
