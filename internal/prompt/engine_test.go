package prompt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

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
	_, err := engine.run(newRenderState(context.Background()), "field", exec.NewContext(map[string]any{"model": &Model{}}))
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

func TestFilteredLoopStrictUndefined(t *testing.T) {
	// gonja v2.9.1 for.go:202 discards loop-filter evaluation errors, so the
	// registered for override must surface them as strict-undefined failures.
	source := `{% for tool in tools.declared if model.unknown_field %}{{ tool }}{% endfor %}`
	loader := memoryLoader{
		"filter":  source,
		"partial": "prefix " + source,
		"else":    `{% for tool in tools.declared if model.unknown_field %}{% else %}fallback{% endfor %}`,
	}
	engine := newTestEngine(t, loader, Options{})
	data := Data{Model: Model{Name: "<model>"}, Tools: Tools{Declared: []string{"read"}}}
	for _, name := range []string{"filter", "partial", "else"} {
		got, err := engine.Render(context.Background(), name, data)
		if err == nil || !strings.Contains(err.Error(), "unknown_field") || got.Text != "" {
			t.Fatalf("%s: got %q, %v; want undefined loop-filter error", name, got.Text, err)
		}
	}
}

func TestFilteredLoopOutput(t *testing.T) {
	loader := memoryLoader{
		"filtered":   `{% for tool in tools.declared if tool != "bash" %}{{ loop.index }}/{{ loop.length }}={{ tool }};{% endfor %}`,
		"else":       `{% for tool in tools.declared if tool == "none" %}{{ tool }}{% else %}none:{{ tools.declared | length }}{% endfor %}`,
		"nested":     `{% for outer in tools.selected %}{% for inner in tools.hidden %}{{ loop.depth }}:{{ outer }}{{ inner }};{% endfor %}{% endfor %}`,
		"control":    `{% for i in [1, 2, 3, 4] %}{% if i == 2 %}{% continue %}{% endif %}{% if i == 4 %}{% break %}{% endif %}{{ i }}{% endfor %}`,
		"pairs":      `{% for name, value in tools.snippets %}{{ name }}={{ value }};{% endfor %}`,
		"descending": `{% for tool in tools.declared | sort(reverse=true) %}{{ tool }};{% endfor %}`,
		"helpers":    `{% for tool in tools.declared %}{{ loop.cycle("a", "b") }}{{ loop.changed(tool) }};{% endfor %}`,
		"steps":      `{% for i in range(1, 6, 2) %}{{ i }};{% endfor %}{% for i in range(3, 0, -1) %}{{ i }};{% endfor %}{% for i in range(3) %}{{ i }};{% endfor %}`,
		"recursive":  `{% for node in tree recursive %}{{ node.name }}{% if node.items is iterable %}{{ loop(node.items) }}{% endif %}{% endfor %}`,
	}
	engine := newTestEngine(t, loader, Options{})
	data := Data{
		Tools: Tools{Declared: []string{"read", "bash", "edit"}, Selected: []string{"a", "b"}, Hidden: []string{"x"}, Snippets: map[string]string{"read": "snippet"}},
	}
	for name, want := range map[string]string{
		"filtered":   "1/2=read;2/2=edit;",
		"else":       "none:3",
		"nested":     "2:ax;2:bx;",
		"control":    "13",
		"pairs":      "read=snippet;",
		"descending": "read;edit;bash;",
		"helpers":    "aTrue;bTrue;aTrue;",
		"steps":      "1;3;5;3;2;1;0;1;2;",
	} {
		got, err := engine.Render(context.Background(), name, data)
		if err != nil || got.Text != want {
			t.Fatalf("%s: got %q, %v; want %q", name, got.Text, err, want)
		}
	}
	tree := []any{
		map[string]any{"name": "a", "items": []any{map[string]any{"name": "b", "items": []any{}}}},
		map[string]any{"name": "c", "items": []any{}},
	}
	got, err := engine.run(newRenderState(context.Background()), "recursive", exec.NewContext(map[string]any{"tree": tree}))
	if err != nil || got.Text != "abc" {
		t.Fatalf("recursive: got %q, %v; want %q", got.Text, err, "abc")
	}
}

func TestLoopCancellation(t *testing.T) {
	loader := memoryLoader{
		"silent":   `{% for i in range(1000000000) %}{% endfor %}`,
		"filtered": `{% for i in range(1000000000) if i > 2000000000 %}{% endfor %}`,
		"output":   `{% for i in range(1000000000) %}{{ i }}{% endfor %}`,
	}
	engine := newTestEngine(t, loader, Options{Deadline: 20 * time.Millisecond, MaxOutputBytes: 1 << 20})
	for _, name := range []string{"silent", "filtered", "output"} {
		started := time.Now()
		got, err := engine.Render(context.Background(), name, Data{})
		elapsed := time.Since(started)
		if !errors.Is(err, context.DeadlineExceeded) || got.Text != "" {
			t.Fatalf("%s: got %q, %v; want deadline exceeded", name, got.Text, err)
		}
		if elapsed > 500*time.Millisecond {
			t.Fatalf("%s: cancellation took %v", name, elapsed)
		}
	}
}

func TestRenderJoinsRangeProducers(t *testing.T) {
	loader := memoryLoader{
		"unused":   `{% set xs = range(1000000000) %}text`,
		"canceled": `{% for i in range(1000000000) %}{% endfor %}`,
		"aborted":  `{% for i in range(1000000000) if missing_variable %}{% endfor %}`,
	}
	for _, tc := range []struct {
		name     string
		deadline time.Duration
		maxSpent time.Duration
		want     string
	}{
		{"unused", 10 * time.Second, time.Second, "text"},
		{"canceled", 20 * time.Millisecond, time.Second, "deadline"},
		{"aborted", 10 * time.Second, time.Second, "missing_variable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := newTestEngine(t, loader, Options{Deadline: tc.deadline})
			ctx, cancel := context.WithTimeout(context.Background(), tc.deadline)
			defer cancel()
			state := newRenderState(ctx)
			started := time.Now()
			got, err := engine.run(state, tc.name, templateContext(Data{}))
			spent := time.Since(started)
			switch tc.want {
			case "text":
				if err != nil || got.Text != "text" {
					t.Fatalf("got %q, %v; want %q", got.Text, err, "text")
				}
			case "deadline":
				if !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("got %v, want deadline exceeded", err)
				}
			default:
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("got %v, want undefined loop-filter error", err)
				}
			}
			if registered, running := state.total.Load(), state.live.Load(); registered == 0 || running != 0 {
				t.Fatalf("producers started %d, running %d; want all %d joined", registered, running, registered)
			}
			if spent > tc.maxSpent {
				t.Fatalf("render spent %v; want it to join producers before its %v deadline", spent, tc.deadline)
			}
		})
	}
}

func TestRangeProducerTerminatesOnOverflow(t *testing.T) {
	for _, tc := range []struct {
		name  string
		args  []int
		first int
	}{
		{"ascending", []int{math.MaxInt64 - 1, math.MaxInt64, 2}, math.MaxInt64 - 1},
		{"descending", []int{math.MinInt64 + 1, math.MinInt64, -2}, math.MinInt64 + 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := newRenderState(context.Background())
			evaluator := &exec.Evaluator{Environment: &exec.Environment{Context: exec.NewContext(map[string]any{renderStateKey: state})}}
			params := &exec.VarArgs{}
			for _, arg := range tc.args {
				params.Args = append(params.Args, exec.AsValue(arg))
			}
			channel, err := rangeFunction(evaluator, params)
			if err != nil {
				t.Fatal(err)
			}
			var values []int
			timeout := time.After(2 * time.Second)
		read:
			for {
				select {
				case value, ok := <-channel:
					if !ok {
						break read
					}
					values = append(values, value)
					if len(values) > 2 {
						t.Fatalf("range produced %v; want it to stop after %d", values, tc.first)
					}
				case <-timeout:
					t.Fatalf("range did not terminate: %v", values)
				}
			}
			if len(values) != 1 || values[0] != tc.first {
				t.Fatalf("got %v; want [%d]", values, tc.first)
			}
			state.join()
			if running := state.live.Load(); running != 0 {
				t.Fatalf("producers still running: %d", running)
			}
		})
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

func TestExpiredRenderHasJoinedItsEvaluation(t *testing.T) {
	loader := memoryLoader{"endless": `{% for i in range(1000000000) %}{{ i }}{% endfor %}`}
	engine := newTestEngine(t, loader, Options{Deadline: 20 * time.Millisecond, MaxOutputBytes: 1 << 20})
	before := runtime.NumGoroutine()
	for range 20 {
		if _, err := engine.Render(context.Background(), "endless", Data{}); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("render error %v, want deadline exceeded", err)
		}
		if after := runtime.NumGoroutine(); after > before {
			t.Fatalf("%d goroutines after an expired render, %d before: evaluation is still running", after, before)
		}
	}
}
