package host

import (
	"context"
	"fmt"
	"time"

	"github.com/creachadair/tomledit/parser"
	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/contribution"
	"github.com/furbyhaxx/pig-workbench/internal/presentation"
)

type displayTrustKey struct{}

func displayNamespace(owner, key string) []string { return []string{"tools", owner, key} }
func displayKey(owner, key, field string) string {
	return parser.Key([]string{"tools", owner, key, field}).String()
}

func displayDefinitions(owner, key string, d presentation.Descriptor) ([]config.Definition, error) {
	field := func(name string, kind config.Kind, value any) config.Definition {
		return config.Definition{Key: displayKey(owner, key, name), Kind: kind, Default: value, Project: config.AllowProject, Application: "registered"}
	}
	density := field("density", config.String, d.DefaultDisplay.Density)
	density.Choices = []string{"compact", "normal", "full"}
	preview := field("preview-lines", config.Integer, int64(d.DefaultDisplay.PreviewLines))
	preview.Min, preview.Max = 0, 100
	children := field("child-density", config.String, "compact")
	children.Choices = []string{"compact", "normal", "full"}
	metrics := make([]string, 0, len(d.MetricDefinitions))
	for _, metric := range d.MetricDefinitions {
		metrics = append(metrics, metric.ID)
	}
	definitions := []config.Definition{field("enabled", config.Boolean, true), density, preview, field("metrics", config.Strings, metrics), field("target-shortening", config.Boolean, true), children}
	for i := 0; i < 4; i++ {
		definitions[i].Application = "live"
	}
	used := map[string]bool{}
	for _, definition := range definitions {
		used[definition.Key] = true
	}
	for _, setting := range d.Settings {
		path := displayKey(owner, key, setting.Key)
		if used[path] {
			return nil, fmt.Errorf("%w: display setting key conflicts with another control", contribution.ErrInvalidDescriptor)
		}
		used[path] = true
		definition := field(setting.Key, config.Boolean, setting.Default)
		definition.Description = setting.Label
		switch setting.Type {
		case "boolean":
		case "integer":
			definition.Kind = config.Integer
			definition.Min, definition.Max = int64(*setting.Minimum), int64(*setting.Maximum)
		case "enum":
			definition.Kind = config.String
			definition.Choices = append([]string(nil), setting.Values...)
		}
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

func installWithDisplaySchema(ctx context.Context, s *session, registration contribution.Registration, consumer contributionConsumer) (func(), error) {
	cleanup, err := consumer(s, registration)
	if err != nil {
		return nil, err
	}
	if registration.Kind != contribution.KindToolPresentation {
		return cleanup, nil
	}
	descriptor, err := presentation.DecodeDescriptor(registration.Descriptor)
	if err == nil {
		var definitions []config.Definition
		definitions, err = displayDefinitions(registration.Owner, registration.Key, descriptor)
		if err == nil {
			trusted, _ := ctx.Value(displayTrustKey{}).(bool)
			err = s.settings.UpdateDynamic(ctx, displayNamespace(registration.Owner, registration.Key), definitions, true, trusted)
		}
	}
	if err != nil {
		cleanup()
		return nil, fmt.Errorf("%w: display schema activation failed: %v", contribution.ErrInvalidDescriptor, err)
	}
	snapshot, err := s.settings.Current()
	if err != nil {
		cleanup()
		return nil, err
	}
	s.refreshPresentationDisplay(snapshot)
	return cleanup, nil
}

func withdrawDisplaySchema(ctx context.Context, s *session, registration contribution.Registration) error {
	if registration.Kind != contribution.KindToolPresentation {
		return nil
	}
	trusted, _ := ctx.Value(displayTrustKey{}).(bool)
	bounded, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	return s.settings.UpdateDynamic(bounded, displayNamespace(registration.Owner, registration.Key), nil, false, trusted)
}
