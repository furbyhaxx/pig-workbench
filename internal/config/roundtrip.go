package config

import (
	"errors"
	"reflect"

	"github.com/creachadair/tomledit/parser"
)

func verifyRoundTrip(original, candidate *document, edits []Edit) error {
	for _, edit := range edits {
		value, present := candidate.values[edit.Key]
		if edit.Reset {
			if present {
				return errors.New("round-trip editor retained a reset override; original file is unchanged")
			}
		} else if !present || !reflect.DeepEqual(value, edit.Value) {
			return errors.New("round-trip editor changed the intended value; original file is unchanged")
		}
	}
	if !reflect.DeepEqual(uneditedTree(original.tree, edits), uneditedTree(candidate.tree, edits)) {
		return errors.New("round-trip editor altered an unrelated setting; original file is unchanged")
	}
	return nil
}

func uneditedTree(tree map[string]any, edits []Edit) map[string]any {
	var clone func(map[string]any) map[string]any
	clone = func(source map[string]any) map[string]any {
		result := make(map[string]any, len(source))
		for key, value := range source {
			if table, ok := value.(map[string]any); ok {
				result[key] = clone(table)
			} else {
				result[key] = value
			}
		}
		return result
	}
	result := clone(tree)
	var remove func(map[string]any, []string)
	remove = func(table map[string]any, parts []string) {
		if len(parts) == 1 {
			delete(table, parts[0])
			return
		}
		child, ok := table[parts[0]].(map[string]any)
		if !ok {
			return
		}
		remove(child, parts[1:])
		if len(child) == 0 {
			delete(table, parts[0])
		}
	}
	delete(result, "version")
	for _, edit := range edits {
		parts, _ := parser.ParseKey(edit.Key)
		remove(result, parts)
	}
	return result
}
