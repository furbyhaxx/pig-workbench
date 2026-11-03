package ask

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// ErrCancelled is returned by an interactor when the user dismisses the form.
var ErrCancelled = errors.New("questionnaire cancelled")

var canonicalPlaceholder = regexp.MustCompile(`^red-sec-[a-z2-7]{16}$`)

// SecretStorageUnavailableError reports that a secret cannot be stored safely.
type SecretStorageUnavailableError struct{}

// Error describes the unavailable secret storage without including secret data.
func (*SecretStorageUnavailableError) Error() string { return "secretStorageUnavailable" }

// FinalizationError reports acknowledged secret writes without exposing values.
type FinalizationError struct {
	StoredPlaceholders   []string
	PublicationAttempted bool
}

// Error reports the secret writes that remain after incomplete finalization.
func (e *FinalizationError) Error() string {
	operation := "questionnaire finalization failed"
	if e.PublicationAttempted {
		operation = "completed questionnaire publication failed"
	}
	if len(e.StoredPlaceholders) == 0 {
		return operation + "; a secret may remain stored with an unavailable identifier"
	}
	return operation + "; secrets remain stored: " + strings.Join(e.StoredPlaceholders, ", ")
}

// InteractionUnavailableError reports why an interactive mode cannot run.
type InteractionUnavailableError struct{ Reason string }

// Error returns the unavailable interaction reason.
func (e *InteractionUnavailableError) Error() string { return e.Reason }

// SecretStore stores a user-provided secret and returns its safe placeholder.
// Store must copy any input bytes it retains after returning.
type SecretStore interface {
	Store(name, scope, description, env string, value []byte) (placeholder string, err error)
}

// Interactor presents questions and must return promptly when the context ends.
type Interactor interface {
	Ask(ctx context.Context, q Question, form Form) (Answer, error)
	Unavailable() error
}

// Answer is an interactor's response to one question.
type Answer struct {
	Values []string
	Custom string
	Note   string
	Secret []byte
}

// Options configures per-question timeouts and the maximum form size.
type Options struct {
	DialogTimeout time.Duration
	MaxQuestions  int
}

// Deps supplies the execution boundaries for one questionnaire.
type Deps struct {
	SessionID  string
	Append     func(customType string, payload any) error
	Interactor Interactor
	Store      SecretStore
	Clock      func() time.Time
	Options    Options
}

// Run validates and presents a form, persisting pending and completed records.
func Run(ctx context.Context, deps Deps, form Form) (Result, error) {
	if deps.Options.DialogTimeout < 0 {
		return Result{}, errors.New("dialog timeout must not be negative")
	}
	limit := deps.Options.MaxQuestions
	if limit <= 0 {
		limit = maxQuestions
	}
	form, err := ValidateForm(form)
	if err != nil {
		return Result{}, err
	}
	if len(form.Questions) > limit {
		return Result{}, fmt.Errorf("questions: exceeds configured limit of %d", limit)
	}
	if ctx.Err() != nil {
		return Result{Cancelled: true}, nil
	}
	if deps.Interactor == nil {
		return Result{}, errors.New("interactor is required")
	}
	unavailable := deps.Interactor.Unavailable()
	if ctx.Err() != nil {
		return Result{Cancelled: true}, nil
	}
	if unavailable != nil {
		reason := unavailable.Error()
		if reason == "" {
			reason = "interactive UI is unavailable"
		}
		return Result{Unavailable: reason}, nil
	}
	for _, question := range form.Questions {
		if question.Type == KindSecret && deps.Store == nil {
			return Result{}, &SecretStorageUnavailableError{}
		}
	}
	if deps.Append == nil {
		return Result{}, errors.New("session append function is required")
	}
	clock := deps.Clock
	if clock == nil {
		clock = time.Now
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return Result{}, errors.New("cannot generate questionnaire id")
	}
	record, err := NewRecord(hex.EncodeToString(id[:]), deps.SessionID, "ask_user", form, clock())
	if ctx.Err() != nil {
		return Result{Cancelled: true}, nil
	}
	if err != nil {
		return Result{}, err
	}
	if err := deps.Append("pig-workbench.ask", record); err != nil {
		return Result{}, fmt.Errorf("append pending questionnaire: %w", err)
	}

	type secretDraft struct {
		id    string
		spec  SecretSpec
		value []byte
	}
	var drafts []secretDraft
	defer func() {
		for i := range drafts {
			clear(drafts[i].value)
		}
	}()
	result := Result{Answers: make(map[string]Answered, len(form.Questions))}
	for i, question := range form.Questions {
		if ctx.Err() != nil {
			return Result{Cancelled: true}, nil
		}
		presentation := cloneForm(form)
		answer, outcome, err := askOne(ctx, deps.Interactor, presentation.Questions[i], presentation, deps.Options.DialogTimeout)
		if err != nil {
			if len(drafts) != 0 {
				return Result{}, errors.New("questionnaire interaction failed")
			}
			return Result{}, err
		}
		if outcome != nil {
			return *outcome, nil
		}
		if err := validateAnswer(question, answer); err != nil {
			return Result{}, err
		}
		if question.Type == KindSecret {
			if answer.Secret != nil {
				drafts = append(drafts, secretDraft{id: question.ID, spec: *question.Secret, value: slices.Clone(answer.Secret)})
				result.Order = append(result.Order, question.ID)
			}
			continue
		}
		if len(answer.Values) == 0 && answer.Custom == "" && answer.Note == "" {
			continue
		}
		result.Answers[question.ID] = Answered{
			Values: slices.Clone(answer.Values), Custom: answer.Custom, Note: answer.Note,
		}
		result.Order = append(result.Order, question.ID)
	}
	if ctx.Err() != nil {
		return Result{Cancelled: true}, nil
	}
	var stored []string
	wroteSecret := false
	finalizationError := func(attempted bool) error {
		return &FinalizationError{StoredPlaceholders: slices.Clone(stored), PublicationAttempted: attempted}
	}
	for _, draft := range drafts {
		if ctx.Err() != nil {
			if wroteSecret {
				return Result{}, finalizationError(false)
			}
			return Result{Cancelled: true}, nil
		}
		value := slices.Clone(draft.value)
		placeholder, err := deps.Store.Store(draft.spec.Name, draft.spec.Scope, draft.spec.Description, draft.spec.Env, value)
		clear(value)
		if err != nil {
			return Result{}, finalizationError(false)
		}
		wroteSecret = true
		if !canonicalPlaceholder.MatchString(placeholder) {
			return Result{}, finalizationError(false)
		}
		if !slices.Contains(stored, placeholder) {
			stored = append(stored, placeholder)
		}
		result.Answers[draft.id] = Answered{Placeholder: placeholder}
	}
	if ctx.Err() != nil {
		if wroteSecret {
			return Result{}, finalizationError(false)
		}
		return Result{Cancelled: true}, nil
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		if wroteSecret {
			return Result{}, finalizationError(false)
		}
		return Result{}, fmt.Errorf("encode completed questionnaire: %w", err)
	}
	updated := clock().UTC()
	if ctx.Err() != nil {
		if wroteSecret {
			return Result{}, finalizationError(false)
		}
		return Result{Cancelled: true}, nil
	}
	if updated.IsZero() {
		if wroteSecret {
			return Result{}, finalizationError(false)
		}
		return Result{}, errors.New("completed questionnaire time is required")
	}
	record.State = StateCompleted
	record.UpdatedAt = updated
	record.Result = encoded
	if ctx.Err() != nil {
		if wroteSecret {
			return Result{}, finalizationError(false)
		}
		return Result{Cancelled: true}, nil
	}
	if err := deps.Append("pig-workbench.ask", record); err != nil {
		if wroteSecret {
			return Result{}, finalizationError(true)
		}
		return Result{}, fmt.Errorf("append completed questionnaire: %w", err)
	}
	return result, nil
}

func validateAnswer(question Question, answer Answer) error {
	if question.Type == KindSecret {
		return nil
	}
	if answer.Secret != nil {
		return fmt.Errorf("question %q: unexpected secret answer", question.ID)
	}
	options := make(map[string]struct{}, len(question.Options))
	for _, option := range question.Options {
		options[option.Value] = struct{}{}
	}
	seen := make(map[string]struct{}, len(answer.Values))
	for _, value := range answer.Values {
		if _, ok := options[value]; !ok {
			return fmt.Errorf("question %q: selected value is not an option", question.ID)
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("question %q: selected value is duplicated", question.ID)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func askOne(ctx context.Context, ui Interactor, question Question, form Form, timeout time.Duration) (Answer, *Result, error) {
	dialogCtx := ctx
	cancel := func() {}
	if timeout > 0 {
		dialogCtx, cancel = context.WithTimeout(ctx, timeout)
	}
	answer, err := ui.Ask(dialogCtx, question, form)
	timedOut := timeout > 0 && errors.Is(dialogCtx.Err(), context.DeadlineExceeded)
	cancel()
	if ctx.Err() != nil {
		return Answer{}, &Result{Cancelled: true}, nil
	}
	if timedOut {
		return Answer{}, &Result{TimedOut: true, Reason: "dialog timed out"}, nil
	}
	if errors.Is(err, ErrCancelled) {
		return Answer{}, &Result{Cancelled: true}, nil
	}
	if err != nil {
		if question.Type == KindSecret {
			return Answer{}, nil, errors.New("questionnaire interaction failed")
		}
		return Answer{}, nil, err
	}
	return answer, nil, nil
}
