package host

import (
	"strings"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/presentation"
)

func TestPresentationCallUsesRecordedRootAndLabelsMissingOrPartialArguments(t *testing.T) {
	descriptor := presentation.Descriptor{Label: "Build", CallFields: []presentation.CallField{{Pointer: "/target", Format: "path"}}}
	args := map[string]any{"target": "/work/app/src/a.go"}
	shown := strings.Join(renderPresentationCall(descriptor, args, sdk.ToolRenderContext{ArgsComplete: true}, "/work/app", 80), "\n")
	if !strings.Contains(shown, "src/a.go") || strings.Contains(shown, "/work/app/src") {
		t.Fatalf("recorded root was not applied: %q", shown)
	}
	other := strings.Join(renderPresentationCall(descriptor, args, sdk.ToolRenderContext{ArgsComplete: true}, "/elsewhere", 80), "\n")
	if !strings.Contains(other, "/work/app/src/a.go") {
		t.Fatalf("a different root must not reuse the first row's relative form: %q", other)
	}
	if got := strings.Join(renderPresentationCall(descriptor, nil, sdk.ToolRenderContext{}, "", 80), "\n"); !strings.Contains(got, "arguments not retained") {
		t.Fatalf("missing arguments were not labelled: %q", got)
	}
	if got := strings.Join(renderPresentationCall(descriptor, args, sdk.ToolRenderContext{ArgsComplete: false}, "", 80), "\n"); !strings.Contains(got, "partial arguments") {
		t.Fatalf("streaming arguments were not labelled partial: %q", got)
	}
}

func TestPresentationHeaderHostDurationOverridesProducerApproximationOnly(t *testing.T) {
	descriptor := presentation.Descriptor{Label: "Build", MetricDefinitions: []presentation.MetricDefinition{{ID: "durationMs", Label: "duration", Unit: presentation.UnitMilliseconds}}}
	snapshot := presentation.Snapshot{Phase: presentation.PhaseSettled, Metrics: []presentation.Metric{{ID: "durationMs", Value: 10, Unit: presentation.UnitMilliseconds}}}
	host := int64(2500)
	if got := presentationHeader(descriptor, snapshot, false, 120, "", &host); !strings.Contains(got, "2.50s") || strings.Contains(got, "10ms") {
		t.Fatalf("host duration did not win: %q", got)
	}
	if got := presentationHeader(descriptor, snapshot, false, 120, "", nil); !strings.Contains(got, "10ms") {
		t.Fatalf("producer metric dropped without a host value: %q", got)
	}
	none := presentation.Snapshot{Phase: presentation.PhaseSettled}
	if got := presentationHeader(descriptor, none, false, 120, "", &host); strings.Contains(got, "duration") {
		t.Fatalf("a metric the producer never reported was invented: %q", got)
	}
}
