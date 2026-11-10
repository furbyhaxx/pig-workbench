package host

import (
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/presentation"
)

var ownToolNames = []string{"read", "write", "edit", "grep", "find", "ls", "bash", "jobs", "cd", "ask_user"}

func TestEveryOwnToolResolvesToABuiltinDescriptor(t *testing.T) {
	catalog := newPresentationCatalog()
	for _, tool := range ownToolNames {
		entry, ok := catalog.lookup(tool)
		if !ok || entry.owner != builtinOwner || entry.builtin == nil || entry.descriptor.Label != tool {
			t.Errorf("%s: no builtin descriptor: %+v", tool, entry)
		}
	}
	if len(catalog.list()) != 0 {
		t.Errorf("builtin descriptors are listed as contributions: %v", catalog.list())
	}
	foreign := presentation.Descriptor{ToolNames: []string{"read"}, Label: "Read", PresentationVersion: 1, Preset: presentation.PresetGeneric}
	if err := catalog.add("r-1", "other", "read", foreign); err == nil {
		t.Error("a contribution claimed an own tool")
	}
}
