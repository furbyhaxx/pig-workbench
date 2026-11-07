package config

import (
	"context"
	"errors"
	"maps"
	"sort"

	"github.com/creachadair/tomledit/parser"
)

// UpdateDynamic activates or withdraws a tools owner/key schema without writing overrides.
func (m *Manager) UpdateDynamic(ctx context.Context, namespace []string, fields []Definition, active, trusted bool) error {
	if len(namespace) != 3 || namespace[0] != "tools" || namespace[1] == "" || namespace[2] == "" {
		return errors.New("dynamic settings require a tools owner/key namespace")
	}
	release, err := m.acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	if m.current == nil {
		return errors.New("load configuration before activating dynamic settings")
	}
	candidate := maps.Clone(m.definitions)
	prefix := parser.Key(namespace)
	for key, definition := range candidate {
		parts, err := parser.ParseKey(key)
		if err != nil {
			return errors.New("configuration schema contains an invalid key")
		}
		if prefix.IsPrefixOf(parts) {
			definition.Application = "unavailable"
			candidate[key] = definition
		}
	}
	seen := map[string]bool{}
	for _, definition := range fields {
		parts, err := parser.ParseKey(definition.Key)
		if err != nil || len(parts) != 4 || !prefix.IsPrefixOf(parts) || parts.String() != definition.Key || seen[definition.Key] {
			return errors.New("invalid or duplicate dynamic setting key")
		}
		if definition.Project != AllowProject {
			return errors.New("dynamic settings must be display-only project-allowlisted values")
		}
		if previous, ok := candidate[definition.Key]; ok && previous.Kind != definition.Kind {
			return &Error{Key: definition.Key, Reason: "descriptor type changed; use a new setting key or an explicit migration"}
		}
		normalized, err := definition.validate(definition.Default)
		if err != nil {
			return &Error{Key: definition.Key, Reason: "invalid dynamic default"}
		}
		definition.Default = cloneValue(normalized)
		definition.Choices = append([]string(nil), definition.Choices...)
		definition.Application = "unavailable"
		if active {
			definition.Application = "live"
		}
		candidate[definition.Key] = definition
		seen[definition.Key] = true
	}
	if len(candidate) > len(Catalog())+4096 {
		return errors.New("dynamic settings exceed the 4096-field limit")
	}
	previous := m.definitions
	m.definitions = candidate
	layers := maps.Clone(m.current.documents)
	if !trusted {
		delete(layers, Project)
	}
	m.mu.RLock()
	previousError := m.lastErr
	m.mu.RUnlock()
	if active {
		layers, err = m.readLayers(trusted)
		if err != nil {
			m.definitions = previous
			return err
		}
	}
	retainedError := previousError
	if active {
		retainedError = nil
	}
	if _, err = m.publish(ctx, layers, retainedError); err != nil {
		m.definitions = previous
		return err
	}
	return nil
}

func orderedDefinitions(definitions map[string]Definition) []Definition {
	result := make([]Definition, 0, len(definitions))
	seen := map[string]bool{}
	for _, builtin := range Catalog() {
		if definition, ok := definitions[builtin.Key]; ok {
			result = append(result, definition)
			seen[builtin.Key] = true
		}
	}
	var dynamic []string
	for key := range definitions {
		if !seen[key] {
			dynamic = append(dynamic, key)
		}
	}
	sort.Strings(dynamic)
	for _, key := range dynamic {
		result = append(result, definitions[key])
	}
	return result
}

// Catalog returns independent schema metadata captured with this snapshot.
func (s *Snapshot) Catalog() []Definition {
	result := append([]Definition(nil), s.catalog...)
	for i := range result {
		result[i].Default = cloneValue(result[i].Default)
		result[i].Choices = append([]string(nil), result[i].Choices...)
	}
	return result
}
