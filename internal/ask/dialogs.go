package ask

import (
	"context"
	"errors"
	"fmt"
	"slices"
)

const (
	unavailableNoDialogs = "no interactive dialogs are configured"
	unavailableNoUI      = "interactive dialogs are unavailable in this mode"

	skipLabel       = "Skip this question"
	doneLabel       = "Done selecting"
	acceptLabel     = "Accept answer"
	editLabel       = "Edit answer…"
	otherPreview    = "Choose another option"
	usePreview      = "Use this option"
	customLabel     = "Custom answer…"
	editCustomLabel = "Edit custom answer…"
	noteLabel       = "Add question note…"
	editNoteLabel   = "Edit question note…"

	customPlaceholder = "Type your answer"
	notePlaceholder   = "Type a note for this question"
	secretNameHint    = "Secret value"
)

// Dialogs presents the host's native dialogs. Each call must return when ctx ends.
type Dialogs interface {
	// Available reports whether dialogs can be shown in the current host mode.
	Available() bool
	// Select shows options and returns the chosen entry; ok is false when dismissed.
	Select(ctx context.Context, title string, options []string) (choice string, ok bool, err error)
	// Input requests one line of text; ok is false when dismissed.
	Input(ctx context.Context, title, placeholder string) (text string, ok bool, err error)
	// SecretInput requests a secret value; ok is false when dismissed. Local visibility is
	// acceptable, but the host must not echo, log or record the value after entry.
	SecretInput(ctx context.Context, title, placeholder string) (value []byte, ok bool, err error)
}

// DialogInteractor answers questions one native dialog at a time.
type DialogInteractor struct{ dialogs Dialogs }

// NewDialogInteractor returns an Interactor that presents sequential native dialogs.
func NewDialogInteractor(dialogs Dialogs) *DialogInteractor {
	return &DialogInteractor{dialogs: dialogs}
}

// Unavailable reports a missing dialog host or a non-interactive host mode.
func (i *DialogInteractor) Unavailable() error {
	if i == nil || i.dialogs == nil {
		return &InteractionUnavailableError{Reason: unavailableNoDialogs}
	}
	if !i.dialogs.Available() {
		return &InteractionUnavailableError{Reason: unavailableNoUI}
	}
	return nil
}

// Ask presents one question through native dialogs and returns the final answer.
func (i *DialogInteractor) Ask(ctx context.Context, q Question, _ Form) (Answer, error) {
	if i == nil || i.dialogs == nil {
		return Answer{}, &InteractionUnavailableError{Reason: unavailableNoDialogs}
	}
	if err := ctx.Err(); err != nil {
		return Answer{}, err
	}
	switch q.Type {
	case KindSecret:
		return i.askSecret(ctx, q)
	case KindFreeform:
		return i.askText(ctx, q)
	case KindSingle, KindMulti, KindPreview:
		if len(q.Options) == 0 {
			if !q.AllowCustom {
				return Answer{}, fmt.Errorf("question %q: has neither options nor custom input", q.ID)
			}
			return i.askText(ctx, q)
		}
		switch q.Type {
		case KindMulti:
			return i.askMulti(ctx, q)
		case KindPreview:
			return i.askPreview(ctx, q)
		default:
			return i.askSingle(ctx, q)
		}
	default:
		return Answer{}, fmt.Errorf("question %q: unsupported question type", q.ID)
	}
}

func (i *DialogInteractor) askSingle(ctx context.Context, q Question) (Answer, error) {
	answer := Answer{}
	for {
		choice, ok, err := i.choose(ctx, dialogTitle(q), choiceEntries(q, answer, nil))
		if err != nil {
			return Answer{}, err
		}
		if !ok {
			return Answer{}, ErrCancelled
		}
		switch choice.kind {
		case entryOption:
			return Answer{Values: []string{choice.option.Value}, Note: answer.Note}, nil
		case entryCustom:
			text, err := i.askCustom(ctx, q)
			if err != nil {
				return Answer{}, err
			}
			if text != "" {
				return Answer{Custom: text, Note: answer.Note}, nil
			}
		case entryNote:
			note, err := i.askNote(ctx, q)
			if err != nil {
				return Answer{}, err
			}
			answer.Note = note
		case entrySkip:
			return Answer{Note: answer.Note}, nil
		}
	}
}

func (i *DialogInteractor) askMulti(ctx context.Context, q Question) (Answer, error) {
	answer := Answer{}
	selected := make(map[string]bool, len(q.Options))
	for {
		choice, ok, err := i.choose(ctx, dialogTitle(q), choiceEntries(q, answer, selected))
		if err != nil {
			return Answer{}, err
		}
		if !ok {
			return Answer{}, ErrCancelled
		}
		switch choice.kind {
		case entryOption:
			selected[choice.option.Value] = !selected[choice.option.Value]
		case entryCustom:
			text, err := i.askCustom(ctx, q)
			if err != nil {
				return Answer{}, err
			}
			answer.Custom = text
		case entryNote:
			note, err := i.askNote(ctx, q)
			if err != nil {
				return Answer{}, err
			}
			answer.Note = note
		case entryDone:
			var values []string
			for _, option := range q.Options {
				if selected[option.Value] {
					values = append(values, option.Value)
				}
			}
			return Answer{Values: values, Custom: answer.Custom, Note: answer.Note}, nil
		}
	}
}

func (i *DialogInteractor) askPreview(ctx context.Context, q Question) (Answer, error) {
	answer := Answer{}
	for {
		choice, ok, err := i.choose(ctx, dialogTitle(q), choiceEntries(q, answer, nil))
		if err != nil {
			return Answer{}, err
		}
		if !ok {
			return Answer{}, ErrCancelled
		}
		switch choice.kind {
		case entryOption:
			confirm, ok, err := i.choose(ctx, choice.option.Preview, []entry{
				{kind: entryAccept, label: usePreview},
				{kind: entryEdit, label: otherPreview},
			})
			if err != nil {
				return Answer{}, err
			}
			if !ok {
				return Answer{}, ErrCancelled
			}
			if confirm.kind == entryAccept {
				return Answer{Values: []string{choice.option.Value}, Note: answer.Note}, nil
			}
		case entryCustom:
			text, err := i.askCustom(ctx, q)
			if err != nil {
				return Answer{}, err
			}
			if text != "" {
				return Answer{Custom: text, Note: answer.Note}, nil
			}
		case entryNote:
			note, err := i.askNote(ctx, q)
			if err != nil {
				return Answer{}, err
			}
			answer.Note = note
		case entrySkip:
			return Answer{Note: answer.Note}, nil
		}
	}
}

func (i *DialogInteractor) askText(ctx context.Context, q Question) (Answer, error) {
	answer := Answer{}
	prompt := true
	for {
		if prompt {
			text, err := i.askCustom(ctx, q)
			if err != nil {
				return Answer{}, err
			}
			answer.Custom = text
			prompt = false
		}
		choice, ok, err := i.choose(ctx, dialogTitle(q), []entry{
			{kind: entryAccept, label: acceptLabel},
			{kind: entryEdit, label: editLabel},
			{kind: entryNote, label: noteEntryLabel(answer.Note != "")},
		})
		if err != nil {
			return Answer{}, err
		}
		if !ok {
			return Answer{}, ErrCancelled
		}
		switch choice.kind {
		case entryAccept:
			return answer, nil
		case entryEdit:
			prompt = true
		case entryNote:
			note, err := i.askNote(ctx, q)
			if err != nil {
				return Answer{}, err
			}
			answer.Note = note
		}
	}
}

func (i *DialogInteractor) askCustom(ctx context.Context, q Question) (string, error) {
	text, ok, err := i.input(ctx, dialogTitle(q), customPlaceholder)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrCancelled
	}
	return text, nil
}

func (i *DialogInteractor) askNote(ctx context.Context, q Question) (string, error) {
	note, ok, err := i.input(ctx, noteTitle(q), notePlaceholder)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrCancelled
	}
	return note, nil
}

func (i *DialogInteractor) askSecret(ctx context.Context, q Question) (Answer, error) {
	placeholder := secretNameHint
	if q.Secret != nil && q.Secret.Name != "" {
		placeholder = q.Secret.Name
	}
	if err := ctx.Err(); err != nil {
		return Answer{}, err
	}
	value, ok, err := i.dialogs.SecretInput(ctx, dialogTitle(q), placeholder)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Answer{}, ctxErr
		}
		return Answer{}, errors.New("secret input failed")
	}
	if !ok {
		return Answer{}, ErrCancelled
	}
	if len(value) == 0 {
		return Answer{}, nil
	}
	return Answer{Secret: value}, nil
}

func (i *DialogInteractor) choose(ctx context.Context, title string, entries []entry) (entry, bool, error) {
	labels := make([]string, len(entries))
	for k := range entries {
		labels[k] = entries[k].label
	}
	labels = uniqueLabels(labels)
	if err := ctx.Err(); err != nil {
		return entry{}, false, err
	}
	choice, ok, err := i.dialogs.Select(ctx, title, labels)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return entry{}, false, ctxErr
		}
		return entry{}, false, fmt.Errorf("select dialog failed: %w", err)
	}
	if !ok {
		return entry{}, false, nil
	}
	index := slices.Index(labels, choice)
	if index < 0 {
		return entry{}, false, errors.New("select dialog returned an unknown choice")
	}
	return entries[index], true, nil
}

func (i *DialogInteractor) input(ctx context.Context, title, placeholder string) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	text, ok, err := i.dialogs.Input(ctx, title, placeholder)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", false, ctxErr
		}
		return "", false, fmt.Errorf("input dialog failed: %w", err)
	}
	return text, ok, nil
}

type entryKind int

const (
	entryOption entryKind = iota
	entryCustom
	entryNote
	entryDone
	entrySkip
	entryAccept
	entryEdit
)

type entry struct {
	kind   entryKind
	label  string
	option Option
}

func choiceEntries(q Question, answer Answer, selected map[string]bool) []entry {
	entries := make([]entry, 0, len(q.Options)+4)
	for _, option := range q.Options {
		label := optionLabel(option)
		if selected != nil {
			mark := "[ ] "
			if selected[option.Value] {
				mark = "[x] "
			}
			label = mark + label
		}
		entries = append(entries, entry{kind: entryOption, label: label, option: option})
	}
	if q.AllowCustom {
		entries = append(entries, entry{kind: entryCustom, label: customEntryLabel(answer.Custom != "")})
	}
	entries = append(entries, entry{kind: entryNote, label: noteEntryLabel(answer.Note != "")})
	if selected != nil {
		entries = append(entries, entry{kind: entryDone, label: doneLabel})
	} else {
		entries = append(entries, entry{kind: entrySkip, label: skipLabel})
	}
	return entries
}

func optionLabel(option Option) string {
	label := option.Label
	if option.Recommended {
		label += " (recommended)"
	}
	if option.Description != "" {
		label += " — " + option.Description
	}
	return label
}

func customEntryLabel(hasAnswer bool) string {
	if hasAnswer {
		return editCustomLabel
	}
	return customLabel
}

func noteEntryLabel(hasNote bool) string {
	if hasNote {
		return editNoteLabel
	}
	return noteLabel
}

func dialogTitle(q Question) string {
	if q.Prompt == "" {
		return q.Label
	}
	return q.Label + ": " + q.Prompt
}

func noteTitle(q Question) string {
	return "Note: " + dialogTitle(q)
}

// uniqueLabels keeps entry lookup by label unambiguous when labels collide.
func uniqueLabels(labels []string) []string {
	used := make(map[string]struct{}, len(labels))
	unique := make([]string, len(labels))
	for index, label := range labels {
		candidate := label
		for suffix := 2; ; suffix++ {
			if _, taken := used[candidate]; !taken {
				break
			}
			candidate = fmt.Sprintf("%s [%d]", label, suffix)
		}
		used[candidate] = struct{}{}
		unique[index] = candidate
	}
	return unique
}
