package host

import (
	"sort"
	"strings"
	"sync"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

const (
	maxChildParents = 128
	maxChildrenEach = 64
)

type childCall struct {
	ID     string `json:"id"`
	Tool   string `json:"tool"`
	Status string `json:"status"`
}

type childParent struct {
	ID       string      `json:"parent"`
	Children []childCall `json:"children"`
	Omitted  int         `json:"omitted,omitempty"`
	order    uint64
}

// childCalls records nested-call identity and outcome only; arguments and output stay out.
type childCalls struct {
	mu      sync.Mutex
	counter uint64
	parents map[string]*childParent
}

func newChildCalls() *childCalls { return &childCalls{parents: make(map[string]*childParent)} }

func (c *childCalls) observe(parent, id, tool, status string) {
	if parent == "" || id == "" || !strings.HasPrefix(id, parent+"/") {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry := c.parents[parent]
	if entry == nil {
		if len(c.parents) >= maxChildParents {
			oldest, min := "", ^uint64(0)
			for key, value := range c.parents {
				if value.order < min {
					oldest, min = key, value.order
				}
			}
			delete(c.parents, oldest)
		}
		c.counter++
		entry = &childParent{ID: parent, order: c.counter}
		c.parents[parent] = entry
	}
	for i := range entry.Children {
		if entry.Children[i].ID == id {
			if entry.Children[i].Status == "running" {
				entry.Children[i].Status = status
			}
			return
		}
	}
	if len(entry.Children) >= maxChildrenEach {
		entry.Omitted++
		return
	}
	entry.Children = append(entry.Children, childCall{ID: id, Tool: tool, Status: status})
}

func (c *childCalls) snapshot(parent string) []childParent {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []childParent
	for key, entry := range c.parents {
		if parent == "" || parent == key {
			copy := *entry
			copy.Children = append([]childCall(nil), entry.Children...)
			out = append(out, copy)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].order < out[j].order })
	return out
}

func (a *app) captureChildStart(_ sdk.Context, data map[string]any) (any, error) {
	if s, err := a.current(); err == nil {
		parent, _ := data["parentToolCallId"].(string)
		id, _ := data["toolCallId"].(string)
		tool, _ := data["toolName"].(string)
		s.children.observe(parent, id, tool, "running")
	}
	return nil, nil
}

func (a *app) captureChildEnd(_ sdk.Context, data map[string]any) (any, error) {
	if s, err := a.current(); err == nil {
		parent, _ := data["parentToolCallId"].(string)
		id, _ := data["toolCallId"].(string)
		tool, _ := data["toolName"].(string)
		status := "ok"
		if failed, _ := data["isError"].(bool); failed {
			status = "error"
		}
		s.children.observe(parent, id, tool, status)
	}
	return nil, nil
}

func (a *app) codemodeCommand(ctx sdk.Context, args string) error {
	s, err := a.current()
	if err != nil {
		return err
	}
	parents := s.children.snapshot(strings.TrimSpace(args))
	if len(parents) == 0 {
		a.notify(ctx, "No retained codemode child calls for this session runtime. Children from before a reload or resume are not retained here.")
		return nil
	}
	return a.notifyJSON(ctx, map[string]any{"codemodeChildren": parents})
}
