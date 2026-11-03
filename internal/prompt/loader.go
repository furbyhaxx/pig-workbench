package prompt

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/nikolalohinski/gonja/v2/loaders"
)

type loaderState struct {
	err          error
	rootRead     bool
	rootNonEmpty bool
}

type templateLoader struct {
	source   TemplateLoader
	ctx      context.Context
	depth    int
	maxDepth int
	state    *loaderState
}

func (l *templateLoader) Read(name string) (io.Reader, error) {
	if err := l.ctx.Err(); err != nil {
		return nil, err
	}
	reader, err := l.source.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open template %q: %w", name, err)
	}
	if reader == nil {
		return nil, fmt.Errorf("open template %q: loader returned a nil reader", name)
	}
	content, readErr := io.ReadAll(contextReader{ctx: l.ctx, reader: reader})
	closeErr := reader.Close()
	if readErr != nil {
		return nil, fmt.Errorf("read template %q: %w", name, readErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close template %q: %w", name, closeErr)
	}
	if l.depth == 0 && !l.state.rootRead {
		l.state.rootRead = true
		l.state.rootNonEmpty = len(content) > 0
	}
	return bytes.NewReader(content), nil
}

func (l *templateLoader) Resolve(name string) (string, error) {
	return name, l.ctx.Err()
}

func (l *templateLoader) Inherit(name string) (loaders.Loader, error) {
	if err := l.ctx.Err(); err != nil {
		l.state.err = err
		return nil, err
	}
	if l.depth >= l.maxDepth {
		err := fmt.Errorf("include depth exceeds %d at template %q", l.maxDepth, name)
		l.state.err = err
		return nil, err
	}
	return &templateLoader{source: l.source, ctx: l.ctx, depth: l.depth + 1, maxDepth: l.maxDepth, state: l.state}, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
