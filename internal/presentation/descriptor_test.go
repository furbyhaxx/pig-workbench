package presentation

import (
	"errors"
	"strings"
	"testing"
)

const validDescriptor = `{"toolNames":["build_run","build_status"],"label":"Build","description":"Build output","category":"Build tools","presentationVersion":1,"preset":"process","defaultDisplay":{"density":"normal","previewLines":8},"callFields":[{"pointer":"/target","label":"target","format":"path"}],"metricDefinitions":[{"id":"durationMs","label":"duration","unit":"ms","priority":20}],"settings":[{"key":"showMetrics","label":"Show metrics","type":"boolean","default":true},{"key":"detail","label":"Detail","type":"enum","default":"normal","values":["compact","normal","full"]}]}`

func TestDisplayDefaultsDistinguishUnsetPreviewFromExplicitZero(t *testing.T) {
	base := `{"toolNames":["tool"],"label":"Tool","presentationVersion":1,"preset":"generic"}`
	defaults, err := DecodeDescriptor([]byte(base))
	if err != nil || defaults.DefaultDisplay.PreviewLines != 8 {
		t.Fatalf("preset preview default: %+v %v", defaults, err)
	}
	explicit := strings.TrimSuffix(base, "}") + `,"defaultDisplay":{"previewLines":0}}`
	zero, err := DecodeDescriptor([]byte(explicit))
	if err != nil || zero.DefaultDisplay.PreviewLines != 0 {
		t.Fatalf("explicit zero was replaced with an invented default: %+v %v", zero, err)
	}
	unbounded := strings.TrimSuffix(base, "}") + `,"settings":[{"key":"size","label":"Size","type":"integer","default":1}]}`
	if _, err := DecodeDescriptor([]byte(unbounded)); err == nil {
		t.Fatal("unbounded integer display setting was accepted")
	}
}

func TestDecodeDescriptor(t *testing.T) {
	descriptor, err := DecodeDescriptor([]byte(validDescriptor))
	if err != nil {
		t.Fatal(err)
	}
	if descriptor.Label != "Build" || descriptor.Preset != PresetProcess || descriptor.DetailsPath != "/workbenchPresentation" {
		t.Fatalf("descriptor = %#v", descriptor)
	}
	if len(descriptor.ToolNames) != 2 || descriptor.DefaultDisplay.PreviewLines != 8 || len(descriptor.MetricDefinitions) != 1 {
		t.Fatalf("descriptor fields = %#v", descriptor)
	}
}

func TestDecodeDescriptorRejectsInvalidContracts(t *testing.T) {
	cases := map[string]string{
		"unknown field":       `{"toolNames":["x"],"label":"X","presentationVersion":1,"preset":"generic","script":"rm"}`,
		"duplicate tool":      `{"toolNames":["x","x"],"label":"X","presentationVersion":1,"preset":"generic"}`,
		"bad tool name":       `{"toolNames":["bad name"],"label":"X","presentationVersion":1,"preset":"generic"}`,
		"unknown preset":      `{"toolNames":["x"],"label":"X","presentationVersion":1,"preset":"table"}`,
		"bad details path":    `{"toolNames":["x"],"label":"X","presentationVersion":1,"preset":"generic","detailsPath":"workbenchPresentation"}`,
		"bad metric unit":     `{"toolNames":["x"],"label":"X","presentationVersion":1,"preset":"generic","metricDefinitions":[{"id":"m","label":"M","unit":"furlong"}]}`,
		"duplicate metric":    `{"toolNames":["x"],"label":"X","presentationVersion":1,"preset":"generic","metricDefinitions":[{"id":"m","label":"M","unit":"count"},{"id":"m","label":"N","unit":"count"}]}`,
		"bad setting type":    `{"toolNames":["x"],"label":"X","presentationVersion":1,"preset":"generic","settings":[{"key":"x","label":"X","type":"color","default":"red"}]}`,
		"bad enum default":    `{"toolNames":["x"],"label":"X","presentationVersion":1,"preset":"generic","settings":[{"key":"x","label":"X","type":"enum","default":"other","values":["a","b"]}]}`,
		"trailing json":       validDescriptor + `{}`,
		"bad cadence version": `{"toolNames":["x"],"label":"X","presentationVersion":2,"preset":"generic"}`,
	}
	for name, payload := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeDescriptor([]byte(payload)); err == nil || !errors.Is(err, ErrInvalid) {
				t.Fatalf("accepted invalid descriptor %s: %v", payload, err)
			}
		})
	}
}
