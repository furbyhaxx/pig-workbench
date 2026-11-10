package syntax

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"unicode/utf8"

	ts "github.com/odvcencio/gotreesitter"
	"github.com/odvcencio/gotreesitter/grammars"
)

const (
	maxSourceBytes     = 4 * 1024 * 1024
	parseTimeoutMicros = 5_000_000
	parseMemoryBytes   = 128 * 1024 * 1024
)

// ErrNoGrammar indicates that no supported syntax grammar is available.
var ErrNoGrammar = errors.New("syntax grammar unavailable")

// ErrTooLarge indicates that the source exceeds the syntax parsing limit.
var ErrTooLarge = errors.New("source exceeds syntax size limit")

// ErrUnparseable indicates that parsing could not produce a complete tree.
var ErrUnparseable = errors.New("source could not be parsed completely")

// Summary describes declarations and omitted, inclusive source-line intervals.
type Summary struct {
	Language     string
	Declarations []Declaration
	Omitted      [][2]int
	ParseErrors  bool
}

// Declaration is an original declaration header at its source coordinate.
type Declaration struct {
	Kind   string
	Line   int
	Text   string
	Indent int
}

// Block is an inclusive syntax span with a one-based byte column.
type Block struct {
	Kind        string
	StartLine   int
	EndLine     int
	StartColumn int
	Ambiguous   bool
}

// Service analyzes source using revision- and content-keyed cached results.
type Service struct {
	cache resultCache
}

// New returns a concurrent-safe syntax service with a bounded cache.
func New() *Service {
	return &Service{cache: newResultCache()}
}

// Analyze returns structural facts for source; folding only affects Summary.Omitted. Callers must not modify source during the call.
func (s *Service) Analyze(ctx context.Context, revision, filename string, source []byte, folding Folding) (Summary, map[int]Block, error) {
	if err := ctx.Err(); err != nil {
		return Summary{}, nil, err
	}
	if len(source) > maxSourceBytes {
		return Summary{}, nil, ErrTooLarge
	}
	if !utf8.Valid(source) || bytes.IndexByte(source, 0) >= 0 {
		return Summary{}, nil, fmt.Errorf("%w: source is not valid text", ErrUnparseable)
	}
	identity, grammarFilename := sourceIdentity(filename)
	entry := grammars.DetectLanguage(grammarFilename)
	if entry == nil || entry.Language == nil || entry.Name == "diff" {
		return Summary{}, nil, fmt.Errorf("%w: %s", ErrNoGrammar, filename)
	}
	language := entry.Language()
	if language == nil {
		return Summary{}, nil, fmt.Errorf("%w: %s", ErrNoGrammar, filename)
	}
	key := cacheKey{
		identity: sha256.Sum256([]byte(identity)),
		revision: sha256.Sum256([]byte(revision)),
		content:  sha256.Sum256(source),
		language: language,
		grammar:  entry.Name,
		version:  language.Version(),
		folding:  folding,
	}
	if err := ctx.Err(); err != nil {
		return Summary{}, nil, err
	}
	if summary, blocks, ok := s.cache.get(key); ok {
		if err := ctx.Err(); err != nil {
			return Summary{}, nil, err
		}
		return summary, blocks, nil
	}

	parser := ts.NewParser(language)
	parser.SetTimeoutMicros(parseTimeoutMicros)
	parser.SetMemoryBudgetBytes(parseMemoryBytes)
	var cancelled uint32
	parser.SetCancellationFlag(&cancelled)
	stop := context.AfterFunc(ctx, func() { atomic.StoreUint32(&cancelled, 1) })
	tree, err := parser.ParseStrict(source)
	stop()
	if tree != nil {
		defer tree.Release()
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return Summary{}, nil, ctxErr
	}
	if err != nil {
		return Summary{}, nil, fmt.Errorf("%w: %v", ErrUnparseable, err)
	}
	if tree == nil || tree.RootNode() == nil && len(source) > 0 {
		return Summary{}, nil, fmt.Errorf("%w: empty syntax tree", ErrUnparseable)
	}
	if tree.ParseStopReason() == ts.ParseStopNoStacksAlive {
		return Summary{}, nil, fmt.Errorf("%w: parser stopped without accepting the source", ErrUnparseable)
	}
	summary, blocks, err := collect(ctx, entry.Name, language, tree.RootNode(), source, folding)
	if err != nil {
		return Summary{}, nil, err
	}
	if err := ctx.Err(); err != nil {
		return Summary{}, nil, err
	}
	s.cache.put(key, summary, blocks)
	return summary, blocks, nil
}

func sourceIdentity(filename string) (string, string) {
	grammarFilename := filename
	if strings.Contains(filename, "://") {
		parsed, err := url.Parse(filename)
		if err != nil {
			return filename, filename
		}
		grammarFilename = parsed.Path
		if parsed.Scheme != "file" || parsed.Host != "" && parsed.Host != "localhost" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return filename, grammarFilename
		}
		filename = parsed.Path
	}
	absolute, err := filepath.Abs(filename)
	if err != nil {
		return filename, grammarFilename
	}
	if canonical, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = canonical
	}
	return absolute, grammarFilename
}
