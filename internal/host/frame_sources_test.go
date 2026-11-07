package host

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func TestInstructionUpdateContextSettingControlsDiff(t *testing.T) {
	root := t.TempDir()
	manager, err := config.New(config.Paths{User: filepath.Join(root, "user.toml"), Project: filepath.Join(root, "project.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	baseline, err := manager.Current()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Save(t.Context(), config.Runtime, baseline, []config.Edit{{Key: "instructions.updates.context", Value: int64(0)}}, nil); err != nil {
		t.Fatal(err)
	}
	zero, err := manager.Current()
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for range 100 {
		lines = append(lines, "unchanged contextual instruction line")
	}
	previous := strings.Join(lines, "\n") + "\n"
	lines[50] = "changed instruction"
	current := strings.Join(lines, "\n") + "\n"
	fact := contextFact{resource: "AGENTS.md"}
	withoutContext := instructionDiff(fact, previous, current, updateContext(zero))
	withContext := instructionDiff(fact, previous, current, updateContext(baseline))
	if withoutContext == "" || withContext == "" || strings.Count(withContext, "unchanged contextual instruction line") <= strings.Count(withoutContext, "unchanged contextual instruction line") {
		t.Fatalf("context setting did not affect the emitted diff:\n%s\n%s", withoutContext, withContext)
	}
}

func TestIncompleteInstructionRequiresCurrentFullyDeliveredSource(t *testing.T) {
	store, err := hashline.NewStore(hashline.StoreLimits{}, "epoch-one")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "AGENTS.md")
	uri := resource.FileURI(path)
	snapshot, err := store.Retain(uri, []byte("one\ntwo\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := &session{snapshots: store}
	fact := contextFact{resource: path, revision: snapshot.Revision, incomplete: true}
	if s.instructionFullySeen(fact) {
		t.Fatal("undelivered instructions were treated as seen")
	}
	if err := store.Delivered(uri, snapshot.Revision, store.Epoch(), []hashline.LineSpan{{Start: 1, End: 1}}, true); err != nil {
		t.Fatal(err)
	}
	if s.instructionFullySeen(fact) {
		t.Fatal("partially delivered instructions were treated as complete")
	}
	if err := store.Delivered(uri, snapshot.Revision, store.Epoch(), []hashline.LineSpan{{Start: 2, End: 2}}, true); err != nil {
		t.Fatal(err)
	}
	if !s.instructionFullySeen(fact) {
		t.Fatal("full delivered coverage was not recognized")
	}
	fact.revision = hashline.SourceRevision([]byte("changed\n"))
	if s.instructionFullySeen(fact) {
		t.Fatal("an old delivered revision authorized new instructions")
	}
}

func TestBeforePromptRequiresAllSixContextFrames(t *testing.T) {
	facts := make([]contextFact, len(requiredContextSources))
	for i, source := range requiredContextSources {
		facts[i] = contextFact{source: source}
	}
	if missing := missingContextSources(facts); len(missing) != 0 {
		t.Fatalf("complete set reported missing %v", missing)
	}
	if missing := missingContextSources(facts[:5]); len(missing) != 1 || missing[0] != "session.capabilities" {
		t.Fatalf("missing = %v", missing)
	}
}
