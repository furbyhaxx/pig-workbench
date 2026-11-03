package prompt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/nikolalohinski/gonja/v2/builtins"
	"github.com/nikolalohinski/gonja/v2/config"
	"github.com/nikolalohinski/gonja/v2/exec"
)

const (
	defaultDeadline        = 100 * time.Millisecond
	defaultMaxOutputBytes  = 64 * 1024
	defaultMaxIncludeDepth = 16
)

// TemplateLoader opens templates by name and lists available names.
type TemplateLoader interface {
	Open(name string) (io.ReadCloser, error)
	List() []string
}

// Options configures the loader and per-render limits; zero limits select defaults.
type Options struct {
	Loader          TemplateLoader
	Deadline        time.Duration
	MaxOutputBytes  int
	MaxIncludeDepth int
}

// Engine renders templates loaded through its configured loader.
type Engine struct {
	loader          TemplateLoader
	filters         *exec.FilterSet
	deadline        time.Duration
	maxOutputBytes  int
	maxIncludeDepth int
}

// Render contains the complete rendered text and any diagnostics.
type Render struct {
	Text     string
	Warnings []string
}

// ErrEmptyRender identifies a non-empty template that produced no prompt text.
type ErrEmptyRender struct {
	Template string
}

func (e *ErrEmptyRender) Error() string {
	return fmt.Sprintf("template %q rendered empty or whitespace-only text", e.Template)
}

// New validates options and constructs a renderer.
func New(options Options) (*Engine, error) {
	if nilLoader(options.Loader) {
		return nil, errors.New("prompt template loader is required")
	}
	if options.Deadline < 0 || options.MaxOutputBytes < 0 || options.MaxIncludeDepth < 0 {
		return nil, errors.New("prompt limits must not be negative")
	}
	if options.Deadline == 0 {
		options.Deadline = defaultDeadline
	}
	if options.MaxOutputBytes == 0 {
		options.MaxOutputBytes = defaultMaxOutputBytes
	}
	if options.MaxIncludeDepth == 0 {
		options.MaxIncludeDepth = defaultMaxIncludeDepth
	}
	filters := exec.NewFilterSet(map[string]exec.FilterFunction{}).Update(builtins.Filters)
	attr, ok := filters.Get("attr")
	if !ok {
		return nil, errors.New("gonja attr filter is unavailable")
	}
	if err := filters.Replace("attr", strictAttributeFilter(attr)); err != nil {
		return nil, fmt.Errorf("register strict attr filter: %w", err)
	}
	return &Engine{
		loader:          options.Loader,
		filters:         filters,
		deadline:        options.Deadline,
		maxOutputBytes:  options.MaxOutputBytes,
		maxIncludeDepth: options.MaxIncludeDepth,
	}, nil
}

func nilLoader(loader TemplateLoader) bool {
	if loader == nil {
		return true
	}
	value := reflect.ValueOf(loader)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

type result struct {
	render Render
	err    error
}

// Render evaluates one template, returning no partial output on failure.
func (e *Engine) Render(ctx context.Context, name string, data Data) (Render, error) {
	ctx, cancel := context.WithTimeout(ctx, e.deadline)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return Render{}, fmt.Errorf("render template %q: %w", name, err)
	}

	input := templateContext(data)
	if err := ctx.Err(); err != nil {
		return Render{}, fmt.Errorf("render template %q: %w", name, err)
	}

	finished := make(chan result, 1)
	go func() {
		var outcome result
		defer func() {
			if recover() != nil {
				outcome = result{err: errors.New("template evaluation panicked")}
			}
			finished <- outcome
		}()
		outcome.render, outcome.err = e.render(ctx, name, input)
	}()

	select {
	case <-ctx.Done():
		return Render{}, fmt.Errorf("render template %q: %w", name, ctx.Err())
	case outcome := <-finished:
		if err := ctx.Err(); err != nil {
			return Render{}, fmt.Errorf("render template %q: %w", name, err)
		}
		if outcome.err != nil {
			return Render{}, fmt.Errorf("render template %q: %w", name, outcome.err)
		}
		return outcome.render, nil
	}
}

func (e *Engine) render(ctx context.Context, name string, input *exec.Context) (Render, error) {
	attributes := &attributeState{}
	input.Set(attributeStateKey, attributes)
	cfg := config.New()
	cfg.StrictUndefined = true
	cfg.TrimBlocks = true
	cfg.LeftStripBlocks = true
	cfg.KeepTrailingNewline = true
	cfg.AutoEscape = false

	environment := &exec.Environment{
		Context:           exec.EmptyContext().Update(builtins.GlobalFunctions).Update(builtins.GlobalVariables),
		Filters:           e.filters,
		Tests:             builtins.Tests,
		ControlStructures: builtins.ControlStructures,
		Methods:           builtins.Methods,
	}
	state := &loaderState{}
	loader := &templateLoader{source: e.loader, ctx: ctx, maxDepth: e.maxIncludeDepth, state: state}
	template, err := exec.NewTemplate(name, cfg, loader, environment)
	if state.err != nil {
		return Render{}, state.err
	}
	if err != nil {
		return Render{}, err
	}

	output := &limitedWriter{ctx: ctx, limit: e.maxOutputBytes}
	err = template.Execute(output, input)
	if state.err != nil {
		return Render{}, state.err
	}
	if attributes.err != nil {
		return Render{}, attributes.err
	}
	if output.err != nil {
		return Render{}, output.err
	}
	if err != nil {
		return Render{}, err
	}
	text := output.buf.String()
	if state.rootNonEmpty && strings.TrimSpace(text) == "" {
		return Render{}, &ErrEmptyRender{Template: name}
	}
	return Render{Text: text}, nil
}

type limitedWriter struct {
	ctx   context.Context
	buf   bytes.Buffer
	limit int
	err   error
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if err := w.ctx.Err(); err != nil {
		w.err = err
		return 0, err
	}
	if len(p) > w.limit-w.buf.Len() {
		w.err = fmt.Errorf("rendered output exceeds %d bytes", w.limit)
		return 0, w.err
	}
	return w.buf.Write(p)
}
