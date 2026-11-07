package presentation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
)

// Preset is one built-in presentation layout named by SPEC-0003 §2.
type Preset string

// The supported presentation presets.
const (
	PresetGeneric       Preset = "generic"
	PresetText          Preset = "text"
	PresetFileRead      Preset = "fileRead"
	PresetFileEdit      Preset = "fileEdit"
	PresetSearch        Preset = "search"
	PresetProcess       Preset = "process"
	PresetQuestionnaire Preset = "questionnaire"
	PresetComposite     Preset = "composite"
)

// FieldFormat is the closed formatting set for declarative call fields.
type FieldFormat string

// The call-field formats SPEC-0003 §2 allows.
const (
	FormatPlain    FieldFormat = "plain"
	FormatPath     FieldFormat = "path"
	FormatURI      FieldFormat = "uri"
	FormatCount    FieldFormat = "count"
	FormatDuration FieldFormat = "duration"
)

// Descriptor is one validated toolPresentation registration.
type Descriptor struct {
	ToolNames           []string           `json:"toolNames"`
	Label               string             `json:"label"`
	Description         string             `json:"description,omitempty"`
	Category            string             `json:"category,omitempty"`
	PresentationVersion int                `json:"presentationVersion"`
	Preset              Preset             `json:"preset"`
	DefaultDisplay      DisplayDefaults    `json:"defaultDisplay"`
	DetailsPath         string             `json:"detailsPath,omitempty"`
	CallFields          []CallField        `json:"callFields,omitempty"`
	MetricDefinitions   []MetricDefinition `json:"metricDefinitions,omitempty"`
	Settings            []DisplaySetting   `json:"settings,omitempty"`
}

// DisplayDefaults are the producer's initial display choices.
type DisplayDefaults struct {
	Density      string `json:"density"`
	PreviewLines int    `json:"previewLines"`
}

// CallField extracts one argument into the call header through a JSON Pointer.
type CallField struct {
	Pointer string      `json:"pointer"`
	Label   string      `json:"label"`
	Format  FieldFormat `json:"format"`
}

// MetricDefinition names one metric the producer may report.
type MetricDefinition struct {
	ID       string     `json:"id"`
	Label    string     `json:"label"`
	Unit     MetricUnit `json:"unit"`
	Priority int        `json:"priority,omitempty"`
}

// DisplaySetting is one allowlisted boolean, bounded integer or enum control.
type DisplaySetting struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type"`
	Default     any      `json:"default"`
	Minimum     *int     `json:"minimum,omitempty"`
	Maximum     *int     `json:"maximum,omitempty"`
	Values      []string `json:"values,omitempty"`
}

var (
	toolNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
)

// DecodeDescriptor strictly validates a toolPresentation descriptor.
func DecodeDescriptor(data []byte) (Descriptor, error) {
	descriptor := Descriptor{DefaultDisplay: DisplayDefaults{Density: "normal", PreviewLines: 8}}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&descriptor); err != nil {
		return Descriptor{}, fmt.Errorf("%w: descriptor: %v", ErrInvalid, err)
	}
	if err := ensureEOF(decoder); err != nil {
		return Descriptor{}, err
	}
	if len(descriptor.ToolNames) == 0 || len(descriptor.ToolNames) > 32 {
		return Descriptor{}, fmt.Errorf("%w: toolNames must contain 1-32 names", ErrInvalid)
	}
	seen := make(map[string]bool, len(descriptor.ToolNames))
	for _, name := range descriptor.ToolNames {
		if !toolNamePattern.MatchString(name) {
			return Descriptor{}, fmt.Errorf("%w: tool name %q is invalid", ErrInvalid, name)
		}
		if seen[name] {
			return Descriptor{}, fmt.Errorf("%w: tool name %q is repeated", ErrInvalid, name)
		}
		seen[name] = true
	}
	if descriptor.Label == "" || len(descriptor.Label) > 128 || hasControl(descriptor.Label) {
		return Descriptor{}, fmt.Errorf("%w: label must be 1-128 bytes without controls", ErrInvalid)
	}
	if descriptor.PresentationVersion != 1 {
		return Descriptor{}, fmt.Errorf("%w: presentationVersion must be 1", ErrInvalid)
	}
	if !slices.Contains([]Preset{PresetGeneric, PresetText, PresetFileRead, PresetFileEdit, PresetSearch, PresetProcess, PresetQuestionnaire, PresetComposite}, descriptor.Preset) {
		return Descriptor{}, fmt.Errorf("%w: unknown preset %q", ErrInvalid, descriptor.Preset)
	}
	if descriptor.DefaultDisplay.Density == "" {
		descriptor.DefaultDisplay.Density = "normal"
	}
	if !slices.Contains([]string{"compact", "normal", "full"}, descriptor.DefaultDisplay.Density) {
		return Descriptor{}, fmt.Errorf("%w: defaultDisplay.density is invalid", ErrInvalid)
	}
	if descriptor.DefaultDisplay.PreviewLines < 0 || descriptor.DefaultDisplay.PreviewLines > 100 {
		return Descriptor{}, fmt.Errorf("%w: defaultDisplay.previewLines must be 0-100", ErrInvalid)
	}
	if descriptor.DetailsPath == "" {
		descriptor.DetailsPath = "/workbenchPresentation"
	} else if !validJSONPointer(descriptor.DetailsPath) {
		return Descriptor{}, fmt.Errorf("%w: detailsPath is not a JSON Pointer", ErrInvalid)
	}
	if len(descriptor.CallFields) > 16 {
		return Descriptor{}, fmt.Errorf("%w: at most 16 callFields are allowed", ErrInvalid)
	}
	for i, field := range descriptor.CallFields {
		if !validJSONPointer(field.Pointer) || field.Label == "" || hasControl(field.Label) {
			return Descriptor{}, fmt.Errorf("%w: callFields[%d] is invalid", ErrInvalid, i)
		}
		if !slices.Contains([]FieldFormat{FormatPlain, FormatPath, FormatURI, FormatCount, FormatDuration}, field.Format) {
			return Descriptor{}, fmt.Errorf("%w: callFields[%d].format is invalid", ErrInvalid, i)
		}
	}
	if len(descriptor.MetricDefinitions) > MaxMetrics {
		return Descriptor{}, fmt.Errorf("%w: at most %d metricDefinitions are allowed", ErrInvalid, MaxMetrics)
	}
	metrics := make(map[string]bool, len(descriptor.MetricDefinitions))
	for i, metric := range descriptor.MetricDefinitions {
		if !identifierPattern.MatchString(metric.ID) || metric.Label == "" || hasControl(metric.Label) {
			return Descriptor{}, fmt.Errorf("%w: metricDefinitions[%d] is invalid", ErrInvalid, i)
		}
		if metrics[metric.ID] {
			return Descriptor{}, fmt.Errorf("%w: metric ID %q is repeated", ErrInvalid, metric.ID)
		}
		metrics[metric.ID] = true
		if !validUnit(metric.Unit) {
			return Descriptor{}, fmt.Errorf("%w: metricDefinitions[%d].unit is invalid", ErrInvalid, i)
		}
	}
	if len(descriptor.Settings) > 16 {
		return Descriptor{}, fmt.Errorf("%w: at most 16 settings are allowed", ErrInvalid)
	}
	for i := range descriptor.Settings {
		if err := validateDisplaySetting(descriptor.Settings[i]); err != nil {
			return Descriptor{}, fmt.Errorf("%w: settings[%d]: %v", ErrInvalid, i, err)
		}
	}
	return descriptor, nil
}

func ensureEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: descriptor has trailing JSON", ErrInvalid)
	}
	return nil
}

func validJSONPointer(pointer string) bool {
	if pointer == "" || pointer[0] != '/' {
		return false
	}
	for _, part := range bytes.Split([]byte(pointer[1:]), []byte{'/'}) {
		for i := 0; i < len(part); i++ {
			if part[i] == '~' && (i+1 >= len(part) || part[i+1] != '0' && part[i+1] != '1') {
				return false
			}
		}
	}
	return true
}

func hasControl(value string) bool {
	for _, r := range value {
		if r < 32 || r == 127 {
			return true
		}
	}
	return false
}

func validUnit(unit MetricUnit) bool {
	switch unit {
	case UnitCount, UnitBytes, UnitLines, UnitMilliseconds, UnitPercent:
		return true
	}
	return len(unit) == 3 && unit[0] >= 'A' && unit[0] <= 'Z' && unit[1] >= 'A' && unit[1] <= 'Z' && unit[2] >= 'A' && unit[2] <= 'Z'
}

func validateDisplaySetting(setting DisplaySetting) error {
	if !identifierPattern.MatchString(setting.Key) || setting.Label == "" || hasControl(setting.Label) || len(setting.Description) > 512 || hasControl(setting.Description) {
		return errors.New("key or label is invalid")
	}
	switch setting.Type {
	case "boolean":
		if _, ok := setting.Default.(bool); !ok {
			return errors.New("boolean default is invalid")
		}
	case "integer":
		if setting.Minimum == nil || setting.Maximum == nil {
			return errors.New("integer display settings require explicit bounds")
		}
		value, ok := asInt(setting.Default)
		if !ok || value < 0 && setting.Minimum == nil {
			return errors.New("integer default is invalid")
		}
		if setting.Minimum != nil && value < *setting.Minimum || setting.Maximum != nil && value > *setting.Maximum {
			return errors.New("integer default is out of range")
		}
		if setting.Minimum != nil && setting.Maximum != nil && *setting.Minimum > *setting.Maximum {
			return errors.New("integer range is reversed")
		}
	case "enum":
		value, ok := setting.Default.(string)
		if !ok || !slices.Contains(setting.Values, value) || len(setting.Values) == 0 || len(setting.Values) > 16 {
			return errors.New("enum default is invalid")
		}
	default:
		return errors.New("unknown setting type")
	}
	return nil
}

func asInt(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), typed == float64(int(typed))
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}
