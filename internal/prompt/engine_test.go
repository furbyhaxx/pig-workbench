package prompt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nikolalohinski/gonja/v2/builtins"
	"github.com/nikolalohinski/gonja/v2/config"
	"github.com/nikolalohinski/gonja/v2/exec"
)

type memoryLoader map[string]string

func (m memoryLoader) Open(name string) (io.ReadCloser, error) {
	content, ok := m[name]
	if !ok {
		return nil, fmt.Errorf("template %q not found", name)
	}
	return io.NopCloser(strings.NewReader(content)), nil
}

func (m memoryLoader) List() []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	return names
}

func newTestEngine(t *testing.T, loader TemplateLoader, options Options) *Engine {
	t.Helper()
	options.Loader = loader
	if options.Deadline == 0 {
		options.Deadline = 2 * time.Second
	}
	engine, err := New(options)
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func TestRenderStableAndStrictUndefined(t *testing.T) {
	loader := memoryLoader{
		"good":   "A\n  {% if model.reasoning %}\nB {{ model.name }}\n  {% endif %}\nC\n",
		"name":   "prefix {{ missing_variable }} suffix",
		"field":  "{{ model.unknown_field }}",
		"if":     "{% if host.unknown_field %}bad{% endif %}",
		"lookup": "{{ tools.snippets.unknown_key }}",
		"attr":   `prefix {{ model | attr("unknown_field") }} suffix`,
	}
	engine := newTestEngine(t, loader, Options{})
	data := Data{Model: Model{Name: "<model>", Reasoning: true}, Tools: Tools{Declared: []string{"read"}}}
	first, err := engine.Render(context.Background(), "good", data)
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.Render(context.Background(), "good", data)
	if err != nil {
		t.Fatal(err)
	}
	if first.Text != "A\nB <model>\nC\n" || first.Text != second.Text {
		t.Fatalf("unexpected or unstable output: first %q; second %q", first.Text, second.Text)
	}
	for _, tc := range []struct{ name, missing string }{
		{"name", "missing_variable"}, {"field", "unknown_field"},
		{"if", "unknown_field"}, {"lookup", "unknown_key"}, {"attr", "unknown_field"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := engine.Render(context.Background(), tc.name, data)
			if err == nil || !strings.Contains(err.Error(), tc.missing) || got.Text != "" {
				t.Fatalf("got text %q and error %v, want undefined %q", got.Text, err, tc.missing)
			}
		})
	}
}

func TestStrictAttrFilter(t *testing.T) {
	loader := memoryLoader{
		"model":   `{{ model | attr("id") }}`,
		"mapping": `{{ tools.snippets | attr("read") }}`,
		"missing": `prefix {{ tools.snippets | attr("absent") }} suffix`,
		"field":   `prefix {{ model | attr("unknown_field") }} suffix`,
		"slice":   `prefix[{{ tools.declared | attr("0") }}]suffix`,
	}
	engine := newTestEngine(t, loader, Options{})
	data := Data{Model: Model{ID: "provider-id"}, Tools: Tools{Snippets: map[string]string{"read": "snippet"}, Declared: []string{"read"}}}
	for name, want := range map[string]string{"model": "provider-id", "mapping": "snippet", "slice": "prefix[]suffix"} {
		got, err := engine.Render(context.Background(), name, data)
		if err != nil || got.Text != want {
			t.Fatalf("%s: got %q, %v; want %q", name, got.Text, err, want)
		}
	}
	for _, tc := range []struct{ name, attribute, kind string }{
		{"missing", "absent", "map"}, {"field", "unknown_field", "struct"},
	} {
		got, err := engine.Render(context.Background(), tc.name, data)
		var unknown *ErrUnknownAttribute
		if !errors.As(err, &unknown) || unknown.Attribute != tc.attribute || unknown.Kind != tc.kind || got.Text != "" {
			t.Fatalf("%s: got %q, %v; want unknown %s attribute %q", tc.name, got.Text, err, tc.kind, tc.attribute)
		}
	}
	_, err := engine.render(context.Background(), "field", exec.NewContext(map[string]any{"model": &Model{}}))
	var unknown *ErrUnknownAttribute
	if !errors.As(err, &unknown) || unknown.Kind != "struct" || unknown.Attribute != "unknown_field" {
		t.Fatalf("pointer: got %v; want unknown struct attribute", err)
	}
}

func TestEmptyRender(t *testing.T) {
	loader := memoryLoader{
		"empty-source": "",
		"blank":        " \t\n",
		"empty-output": `{% if model.reasoning %}content{% endif %}`,
		"blank-output": " \t{{ model.name }}\n",
		"child":        `{% extends "parent" %}{% block content %} {% endblock %}`,
		"parent":       `{% block content %}value{% endblock %}`,
	}
	engine := newTestEngine(t, loader, Options{})
	got, err := engine.Render(context.Background(), "empty-source", Data{})
	if err != nil || got.Text != "" {
		t.Fatalf("empty source: got %q, %v", got.Text, err)
	}
	for _, name := range []string{"blank", "empty-output", "blank-output", "child"} {
		got, err := engine.Render(context.Background(), name, Data{})
		var empty *ErrEmptyRender
		if !errors.As(err, &empty) || empty.Template != name || got.Text != "" || !strings.Contains(err.Error(), name) {
			t.Fatalf("%s: got %q, %v; want named ErrEmptyRender", name, got.Text, err)
		}
	}
}

func TestGonjaFilteredLoopUndefinedLimitation(t *testing.T) {
	// gonja v2.9.1 builtins/control_structures/for.go:202 ignores filter evaluation errors.
	source := `{% for tool in tools.declared if model.unknown_field %}{{ tool }}{% endfor %}`
	loader := memoryLoader{"filter": source}
	config := config.New()
	config.StrictUndefined = true
	environment := &exec.Environment{
		Context: exec.EmptyContext(), Filters: builtins.Filters, Tests: builtins.Tests,
		ControlStructures: builtins.ControlStructures, Methods: builtins.Methods,
	}
	adapter := &templateLoader{source: loader, ctx: context.Background(), maxDepth: 16, state: &loaderState{}}
	template, err := exec.NewTemplate("filter", config, adapter, environment)
	if err != nil {
		t.Fatal(err)
	}
	data := Data{Tools: Tools{Declared: []string{"read"}}}
	var output bytes.Buffer
	if err := template.Execute(&output, templateContext(data)); err != nil || output.String() != "" {
		t.Fatalf("gonja behavior changed: got %q, %v; remove the limitation note", output.String(), err)
	}
	engine := newTestEngine(t, loader, Options{})
	got, err := engine.Render(context.Background(), "filter", data)
	var empty *ErrEmptyRender
	if !errors.As(err, &empty) || empty.Template != "filter" || got.Text != "" {
		t.Fatalf("engine: got %q, %v; want named ErrEmptyRender", got.Text, err)
	}
}

func TestDataFieldsAndEmptyCollections(t *testing.T) {
	loader := memoryLoader{
		"fields": `{{ model.provider }}|{{ model.id }}|{{ model.name }}|{{ model.api }}|{{ model.family }}|{{ model.reasoning }}|{{ model.context_window }}|{{ model.max_output }}|{{ model.thinking }}|{{ model.mid_convo_system }}
{{ tools.selected[0] }}|{{ tools.declared[0] }}|{{ tools.hidden[0] }}|{{ tools.snippets.read }}|{{ tools.guidelines.read[0] }}
{{ host.custom_prompt }}|{{ host.append }}|{{ host.prompt_guidelines[0] }}|{{ host.docs }}|{{ host.context_files[0].path }}|{{ host.context_files[0].content }}|{{ host.skills[0].name }}|{{ host.sections.extra }}|{{ host.mode }}|{{ host.os }}|{{ host.shell }}
{{ instructions[0].class }}|{{ instructions[0].path }}|{{ instructions[0].display_path }}|{{ instructions[0].role }}|{{ instructions[0].content }}
{{ skills.items[0].name }}|{{ skills.items[0].description }}|{{ skills.items[0].location }}|{{ skills.items[0].source }}|{{ skills.items[0].bundle }}|{{ skills.listing }}
{{ workbench.edit.mode }}|{{ workbench.read.summarize }}|{{ workbench.read.selectors }}|{{ workbench.secrets.enabled }}|{{ workbench.schemes[0].name }}|{{ workbench.schemes[0].summary }}|{{ project_root }}`,
		"empty": `{{ instructions }}|{{ tools.declared }}|{{ host.context_files }}|{{ skills.items }}`,
	}
	engine := newTestEngine(t, loader, Options{})
	data := Data{
		Model:        Model{Provider: "provider", ID: "id", Name: "name", API: "api", Family: "family", Reasoning: true, ContextWindow: 42, MaxOutput: 7, Thinking: "high", MidConvoSystem: true},
		Tools:        Tools{Selected: []string{"selected"}, Declared: []string{"declared"}, Hidden: []string{"hidden"}, Snippets: map[string]string{"read": "snippet"}, Guidelines: map[string][]string{"read": {"guideline"}}},
		Host:         Host{CustomPrompt: "custom", Append: "append", PromptGuidelines: []string{"host rule"}, Docs: true, ContextFiles: []ContextFile{{Path: "path", Content: "content"}}, Skills: []HostSkill{{Name: "host skill"}}, Sections: map[string]string{"extra": "section"}, Mode: "rpc", OS: "linux", Shell: "sh"},
		Instructions: []Instruction{{Class: "class", Path: "path", DisplayPath: "display", Role: "role", Content: "instructions"}},
		Skills:       SkillCatalog{Items: []Skill{{Name: "skill", Description: "description", Location: "location", Source: "source", Bundle: "bundle"}}, Listing: "prompt"},
		Workbench:    Workbench{Edit: EditFeatures{Mode: "hashline"}, Read: ReadFeatures{Summarize: true, Selectors: true}, Secrets: SecretFeatures{Enabled: true}, Schemes: []Scheme{{Name: "docs", Summary: "summary"}}},
		ProjectRoot:  "/project",
	}
	want := "provider|id|name|api|family|True|42|7|high|True\n" +
		"selected|declared|hidden|snippet|guideline\n" +
		"custom|append|host rule|True|path|content|host skill|section|rpc|linux|sh\n" +
		"class|path|display|role|instructions\n" +
		"skill|description|location|source|bundle|prompt\n" +
		"hashline|True|True|True|docs|summary|/project"
	got, err := engine.Render(context.Background(), "fields", data)
	if err != nil || got.Text != want {
		t.Fatalf("fields: got %q, err %v; want %q", got.Text, err, want)
	}
	first, err := engine.Render(context.Background(), "empty", Data{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := engine.Render(context.Background(), "empty", Data{})
	if err != nil || first.Text != "[]|[]|[]|[]" || second.Text != first.Text {
		t.Fatalf("nil collections: first %q, second %q, err %v", first.Text, second.Text, err)
	}
}

func TestMapIterationIsStable(t *testing.T) {
	loader := memoryLoader{"ordered": `{% for name, value in tools.snippets %}{{ name }}={{ value }};{% endfor %}{% for name, value in host.sections %}{{ name }}={{ value }};{% endfor %}`}
	engine := newTestEngine(t, loader, Options{})
	data := Data{Tools: Tools{Snippets: map[string]string{"z": "Z", "a": "A", "m": "M"}}, Host: Host{Sections: map[string]string{"y": "Y", "b": "B"}}}
	for i := 0; i < 20; i++ {
		got, err := engine.Render(context.Background(), "ordered", data)
		if err != nil || got.Text != "a=A;m=M;z=Z;b=B;y=Y;" {
			t.Fatalf("render %d: got %q, err %v", i, got.Text, err)
		}
	}
}

func TestOutputLimit(t *testing.T) {
	engine := newTestEngine(t, memoryLoader{"at": "1234", "over": "12345", "utf8": "äöx"}, Options{MaxOutputBytes: 4})
	at, err := engine.Render(context.Background(), "at", Data{})
	if err != nil || at.Text != "1234" {
		t.Fatalf("at limit: %q, %v", at.Text, err)
	}
	for _, name := range []string{"over", "utf8"} {
		got, err := engine.Render(context.Background(), name, Data{})
		if err == nil || !strings.Contains(err.Error(), "output") || got.Text != "" {
			t.Fatalf("%s: got %q, %v; expected output limit error", name, got.Text, err)
		}
	}
}

func TestDeadline(t *testing.T) {
	engine := newTestEngine(t, memoryLoader{"loop": `{% for i in range(1000000) %}{{ i }}{% endfor %}`}, Options{Deadline: time.Nanosecond})
	got, err := engine.Render(context.Background(), "loop", Data{})
	if !errors.Is(err, context.DeadlineExceeded) || got.Text != "" {
		t.Fatalf("got %q, %v; want deadline exceeded", got.Text, err)
	}
}

type blockingLoader struct {
	started chan struct{}
	release chan struct{}
	closed  chan struct{}
}

func (l *blockingLoader) Open(string) (io.ReadCloser, error) {
	close(l.started)
	<-l.release
	return &notifyingReader{Reader: strings.NewReader("late"), closed: l.closed}, nil
}
func (l *blockingLoader) List() []string { return nil }

type notifyingReader struct {
	io.Reader
	closed chan struct{}
}

func (r *notifyingReader) Close() error { close(r.closed); return nil }

func TestContextCancellationDuringLoading(t *testing.T) {
	loader := &blockingLoader{started: make(chan struct{}), release: make(chan struct{}), closed: make(chan struct{})}
	engine := newTestEngine(t, loader, Options{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := engine.Render(ctx, "blocked", Data{})
		done <- err
	}()
	<-loader.started
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Errorf("got %v, want context canceled", err)
	}
	close(loader.release)
	select {
	case <-loader.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("loader was not closed after releasing it")
	}
}

func TestIncludeDepth(t *testing.T) {
	loader := memoryLoader{
		"root":             `{% include "one" %}`,
		"one":              `{% include "two" %}`,
		"two":              `done`,
		"deep":             `{% include "a" %}`,
		"a":                `{% include "b" %}`,
		"b":                `{% include "two" %}`,
		"ignored":          `{% include "ignore_a" %}`,
		"ignore_a":         `{% include "ignore_b" %}`,
		"ignore_b":         `{% include "two" ignore missing %}`,
		"siblings":         `{% include "two" %}|{% include "two" %}`,
		"extends":          `{% extends "parent" %}{% block b %}child{% endblock %}`,
		"parent":           `{% extends "grandparent" %}`,
		"grandparent":      `{% extends "greatgrandparent" %}`,
		"greatgrandparent": `{% block b %}root{% endblock %}`,
	}
	engine := newTestEngine(t, loader, Options{MaxIncludeDepth: 2})
	for name, want := range map[string]string{"root": "done", "siblings": "done|done"} {
		got, err := engine.Render(context.Background(), name, Data{})
		if err != nil || got.Text != want {
			t.Fatalf("%s: got %q, %v; want %q", name, got.Text, err, want)
		}
	}
	for _, name := range []string{"deep", "ignored", "extends"} {
		got, err := engine.Render(context.Background(), name, Data{})
		if err == nil || !strings.Contains(err.Error(), "depth") || got.Text != "" {
			t.Fatalf("%s: got %q, %v; want depth error", name, got.Text, err)
		}
	}
}

func TestOptionsValidation(t *testing.T) {
	var nilLoader *memoryLoader
	for _, options := range []Options{
		{}, {Loader: nilLoader}, {Loader: memoryLoader{}, Deadline: -1},
		{Loader: memoryLoader{}, MaxOutputBytes: -1}, {Loader: memoryLoader{}, MaxIncludeDepth: -1},
	} {
		if _, err := New(options); err == nil {
			t.Fatalf("accepted invalid options: %+v", options)
		}
	}
	if _, err := New(Options{Loader: memoryLoader{}}); err != nil {
		t.Fatalf("defaults rejected: %v", err)
	}
}

func TestConcurrentRenders(t *testing.T) {
	engine := newTestEngine(t, memoryLoader{"main": `{{ model.id }}:{% include "part" %}`, "part": `{{ project_root }}`}, Options{})
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Go(func() {
			got, err := engine.Render(context.Background(), "main", Data{Model: Model{ID: "same"}, ProjectRoot: "/root"})
			if err != nil || got.Text != "same:/root" {
				t.Errorf("got %q, %v", got.Text, err)
			}
		})
	}
	wg.Wait()
}
