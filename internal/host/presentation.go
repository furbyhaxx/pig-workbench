package host

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/presentation"
)

type presentationRegistration struct {
	registrationID string
	owner          string
	key            string
	signature      [32]byte
	descriptor     presentation.Descriptor
}

type presentationCatalog struct {
	mu       sync.RWMutex
	byTool   map[string]presentationRegistration
	byID     map[string]presentationRegistration
	resolved map[string]presentationRegistration
}

func newPresentationCatalog() *presentationCatalog {
	return &presentationCatalog{byTool: make(map[string]presentationRegistration), byID: make(map[string]presentationRegistration), resolved: make(map[string]presentationRegistration)}
}

func (c *presentationCatalog) add(registrationID, owner, key string, descriptor presentation.Descriptor) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, name := range descriptor.ToolNames {
		if existing, ok := c.byTool[name]; ok {
			return fmt.Errorf("tool %q already has presentation owner %q", name, existing.owner)
		}
	}
	encoded, _ := json.Marshal(struct {
		Owner, Key string
		Descriptor presentation.Descriptor
	}{owner, key, descriptor})
	entry := presentationRegistration{registrationID: registrationID, owner: owner, key: key, signature: sha256.Sum256(encoded), descriptor: descriptor}
	for _, name := range descriptor.ToolNames {
		c.byTool[name] = entry
	}
	c.byID[registrationID] = entry
	return nil
}

func (c *presentationCatalog) remove(registrationID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.byID[registrationID]
	if !ok {
		return
	}
	for _, name := range entry.descriptor.ToolNames {
		if current, ok := c.byTool[name]; ok && current.registrationID == registrationID {
			delete(c.byTool, name)
		}
	}
	delete(c.byID, registrationID)
}

func (c *presentationCatalog) resolve(tool string) (presentationRegistration, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.byTool[tool]
	if _, answered := c.resolved[tool]; !answered {
		c.resolved[tool] = entry
	}
	return entry, ok
}

func (c *presentationCatalog) appliesAfterReload(id string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if entry, ok := c.byID[id]; ok {
		for _, tool := range entry.descriptor.ToolNames {
			if resolved, answered := c.resolved[tool]; answered && resolved.signature != entry.signature {
				return true
			}
		}
	}
	for tool, resolved := range c.resolved {
		if resolved.registrationID == id {
			current := c.byTool[tool]
			if current.signature != resolved.signature {
				return true
			}
		}
	}
	return false
}

type presentationChange struct{ Tool, Owner, Key, Change string }

func (c *presentationCatalog) changes() []presentationChange {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var changes []presentationChange
	for tool, resolved := range c.resolved {
		current, exists := c.byTool[tool]
		if current.signature == resolved.signature {
			continue
		}
		if exists {
			changes = append(changes, presentationChange{Tool: tool, Owner: current.owner, Key: current.key, Change: "changed"})
		} else {
			changes = append(changes, presentationChange{Tool: tool, Owner: resolved.owner, Key: resolved.key, Change: "withdrawn"})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Tool < changes[j].Tool })
	return changes
}

func (c *presentationCatalog) lookup(tool string) (presentationRegistration, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, ok := c.byTool[tool]
	return entry, ok
}

func (c *presentationCatalog) list() []presentationRegistration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entries := make([]presentationRegistration, 0, len(c.byID))
	for _, entry := range c.byID {
		entries = append(entries, entry)
	}
	return entries
}

func (a *app) resolveToolRenderer(toolName string, next func() *sdk.ToolRenderers) *sdk.ToolRenderers {
	s, err := a.current()
	if err != nil {
		return next()
	}
	entry, ok := s.presentations.resolve(toolName)
	if !ok {
		return next()
	}
	return &sdk.ToolRenderers{
		Call: func(_ sdk.Context, args map[string]any, _ sdk.ToolRenderContext, width int) ([]string, error) {
			return renderPresentationCall(entry.descriptor, args, width), nil
		},
		Result: func(_ sdk.Context, result sdk.ToolRenderResult, options sdk.ToolRenderResultOptions, render sdk.ToolRenderContext, width int) ([]string, error) {
			return renderPresentationResult(entry.descriptor, result, options, render, width), nil
		},
	}
}

func decodeToolSnapshot(descriptor presentation.Descriptor, details any) (presentation.Snapshot, error) {
	if details == nil {
		return presentation.Snapshot{}, errors.New("tool result has no details")
	}
	encoded, err := json.Marshal(details)
	if err != nil {
		return presentation.Snapshot{}, err
	}
	var root any
	if err := json.Unmarshal(encoded, &root); err != nil {
		return presentation.Snapshot{}, err
	}
	node, err := jsonPointerValue(root, descriptor.DetailsPath)
	if err != nil {
		return presentation.Snapshot{}, err
	}
	payload, err := json.Marshal(node)
	if err != nil {
		return presentation.Snapshot{}, err
	}
	return presentation.Decode(payload)
}

func jsonPointerValue(root any, pointer string) (any, error) {
	if pointer == "" || pointer == "/" {
		return root, nil
	}
	current := root
	for _, raw := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		part := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("details path %q does not address an object", pointer)
		}
		next, ok := object[part]
		if !ok {
			return nil, fmt.Errorf("details path %q is absent", pointer)
		}
		current = next
	}
	return current, nil
}

func argumentField(args map[string]any, pointer string) (string, bool) {
	if args == nil || pointer == "" {
		return "", false
	}
	current := any(args)
	for _, raw := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		part := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		object, ok := current.(map[string]any)
		if !ok {
			return "", false
		}
		next, ok := object[part]
		if !ok {
			return "", false
		}
		current = next
	}
	switch value := current.(type) {
	case string:
		return value, true
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64), true
	case bool:
		return strconv.FormatBool(value), true
	default:
		return "", false
	}
}
