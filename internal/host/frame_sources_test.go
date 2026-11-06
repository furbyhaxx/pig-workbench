package host

import "testing"

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
