package ask

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

type selectCall struct {
	title   string
	options []string
}

type inputCall struct {
	title       string
	placeholder string
}

type fakeDialogs struct {
	available bool
	selects   []selectCall
	inputs    []inputCall
	secrets   []inputCall
	selectFn  func(context.Context, string, []string) (string, bool, error)
	inputFn   func(context.Context, string, string) (string, bool, error)
	secretFn  func(context.Context, string, string) ([]byte, bool, error)
}

func (f *fakeDialogs) Available() bool { return f.available }

func (f *fakeDialogs) Select(ctx context.Context, title string, options []string) (string, bool, error) {
	f.selects = append(f.selects, selectCall{title: title, options: slices.Clone(options)})
	if f.selectFn == nil {
		return "", false, nil
	}
	return f.selectFn(ctx, title, options)
}

func (f *fakeDialogs) Input(ctx context.Context, title, placeholder string) (string, bool, error) {
	f.inputs = append(f.inputs, inputCall{title: title, placeholder: placeholder})
	if f.inputFn == nil {
		return "", false, nil
	}
	return f.inputFn(ctx, title, placeholder)
}

func (f *fakeDialogs) SecretInput(ctx context.Context, title, placeholder string) ([]byte, bool, error) {
	f.secrets = append(f.secrets, inputCall{title: title, placeholder: placeholder})
	if f.secretFn == nil {
		return nil, false, nil
	}
	return f.secretFn(ctx, title, placeholder)
}

func (f *fakeDialogs) text() []string {
	var all []string
	for _, call := range f.selects {
		all = append(all, call.title)
		all = append(all, call.options...)
	}
	for _, call := range append(slices.Clone(f.inputs), f.secrets...) {
		all = append(all, call.title, call.placeholder)
	}
	return all
}

func multiForm() Form {
	return Form{Title: "Targets", Questions: []Question{{
		ID: "targets", Label: "Targets", Prompt: "Which targets?",
		Type: KindMulti, AllowCustom: true,
		Options: []Option{
			{Value: "a", Label: "Alpha"}, {Value: "b", Label: "Beta"}, {Value: "c", Label: "Gamma"},
		},
	}}}
}

func singleNoteForm() Form { return validForm() }

func TestDialogsSingleChoiceWithNote(t *testing.T) {
	entries := &recordedEntries{}
	questions := []string{
		"Project only — Restrict discovery",
		"Project and global (recommended)",
		noteLabel,
		skipLabel,
	}
	dialogs := &fakeDialogs{available: true}
	dialogs.selectFn = func(_ context.Context, title string, options []string) (string, bool, error) {
		if title != "Scope: Where should history be discoverable?" {
			t.Fatalf("unexpected select title %q", title)
		}
		switch len(dialogs.selects) {
		case 1:
			if !reflect.DeepEqual(options, questions) {
				t.Fatalf("unexpected choices: %#v", options)
			}
			return noteLabel, true, nil
		case 2:
			return questions[0], true, nil
		}
		t.Fatalf("unexpected extra select: %#v", options)
		return "", false, nil
	}
	dialogs.inputFn = func(_ context.Context, title, placeholder string) (string, bool, error) {
		if title != "Note: Scope: Where should history be discoverable?" || placeholder != notePlaceholder {
			t.Fatalf("unexpected note input %q %q", title, placeholder)
		}
		return "keep it local", true, nil
	}
	result, err := Run(context.Background(), runDeps(NewDialogInteractor(dialogs), entries), singleNoteForm())
	if err != nil {
		t.Fatal(err)
	}
	answer := result.Answers["scope"]
	if !reflect.DeepEqual(answer.Values, []string{"project"}) || answer.Note != "keep it local" || answer.Custom != "" {
		t.Fatalf("unexpected answer: %#v", answer)
	}
	if len(entries.records) != 2 || entries.records[1].State != StateCompleted {
		t.Fatalf("questionnaire was not completed: %#v", entries.records)
	}
}

func TestDialogsMultiTogglesKeepPriorSelection(t *testing.T) {
	entries := &recordedEntries{}
	dialogs := &fakeDialogs{available: true}
	dialogs.selectFn = func(_ context.Context, _ string, options []string) (string, bool, error) {
		switch len(dialogs.selects) {
		case 1:
			want := []string{"[ ] Alpha", "[ ] Beta", "[ ] Gamma", customLabel, noteLabel, doneLabel}
			if !reflect.DeepEqual(options, want) {
				t.Fatalf("unexpected choices: %#v", options)
			}
			return "[ ] Alpha", true, nil
		case 2:
			want := []string{"[x] Alpha", "[ ] Beta", "[ ] Gamma", customLabel, noteLabel, doneLabel}
			if !reflect.DeepEqual(options, want) {
				t.Fatalf("earlier toggle was lost: %#v", options)
			}
			return "[ ] Beta", true, nil
		case 3:
			want := []string{"[x] Alpha", "[x] Beta", "[ ] Gamma", customLabel, noteLabel, doneLabel}
			if !reflect.DeepEqual(options, want) {
				t.Fatalf("toggles were lost: %#v", options)
			}
			return doneLabel, true, nil
		}
		t.Fatalf("unexpected extra select: %#v", options)
		return "", false, nil
	}
	result, err := Run(context.Background(), runDeps(NewDialogInteractor(dialogs), entries), multiForm())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Answers["targets"].Values, []string{"a", "b"}) {
		t.Fatalf("unexpected selection: %#v", result.Answers["targets"])
	}
}

func TestDialogsMultiCustomAnswerAndToggle(t *testing.T) {
	dialogs := &fakeDialogs{available: true}
	dialogs.selectFn = func(_ context.Context, _ string, _ []string) (string, bool, error) {
		switch len(dialogs.selects) {
		case 1:
			return "Custom answer…", true, nil
		case 2:
			if !slices.Contains(dialogs.selects[1].options, editCustomLabel) {
				t.Fatalf("custom answer not retained: %#v", dialogs.selects[1].options)
			}
			return "[ ] Beta", true, nil
		}
		return doneLabel, true, nil
	}
	dialogs.inputFn = func(_ context.Context, _, placeholder string) (string, bool, error) {
		if placeholder != customPlaceholder {
			t.Fatalf("unexpected input placeholder %q", placeholder)
		}
		return "bespoke", true, nil
	}
	answer, err := NewDialogInteractor(dialogs).Ask(context.Background(), multiForm().Questions[0], multiForm())
	if err != nil {
		t.Fatal(err)
	}
	if answer.Custom != "bespoke" || !reflect.DeepEqual(answer.Values, []string{"b"}) {
		t.Fatalf("unexpected answer: %#v", answer)
	}
}

func TestDialogsCustomOnlyChoice(t *testing.T) {
	form := Form{Questions: []Question{{ID: "name", Label: "Name", Type: KindSingle, AllowCustom: true}}}
	entries := &recordedEntries{}
	dialogs := &fakeDialogs{available: true}
	dialogs.inputFn = func(_ context.Context, title, placeholder string) (string, bool, error) {
		if title != "Name" || placeholder != customPlaceholder {
			t.Fatalf("unexpected custom input %q %q", title, placeholder)
		}
		return "workbench", true, nil
	}
	dialogs.selectFn = func(_ context.Context, _ string, options []string) (string, bool, error) {
		if !reflect.DeepEqual(options, []string{acceptLabel, editLabel, noteLabel}) {
			t.Fatalf("unexpected finish choices: %#v", options)
		}
		return acceptLabel, true, nil
	}
	result, err := Run(context.Background(), runDeps(NewDialogInteractor(dialogs), entries), form)
	if err != nil {
		t.Fatal(err)
	}
	if result.Answers["name"].Custom != "workbench" || len(result.Answers["name"].Values) != 0 {
		t.Fatalf("unexpected answer: %#v", result.Answers)
	}
}

func TestDialogsPreviewShowsActualTextBeforeCommit(t *testing.T) {
	first := "first preview\nline two\tindented"
	second := "second preview"
	form := Form{Questions: []Question{{
		ID: "render", Label: "Render", Prompt: "Pick a layout", Type: KindPreview,
		Options: []Option{
			{Value: "one", Label: "One", Preview: first},
			{Value: "two", Label: "Two", Preview: second},
		},
	}}}
	dialogs := &fakeDialogs{available: true}
	dialogs.selectFn = func(_ context.Context, title string, options []string) (string, bool, error) {
		switch len(dialogs.selects) {
		case 1:
			return "One", true, nil
		case 2:
			if title != first || !reflect.DeepEqual(options, []string{usePreview, otherPreview}) {
				t.Fatalf("preview text was not shown verbatim: %q %#v", title, options)
			}
			return otherPreview, true, nil
		case 3:
			if !reflect.DeepEqual(options, []string{"One", "Two", noteLabel, skipLabel}) {
				t.Fatalf("unexpected choices: %#v", options)
			}
			return "Two", true, nil
		case 4:
			if title != second {
				t.Fatalf("second preview text was not shown: %q", title)
			}
			return usePreview, true, nil
		}
		t.Fatalf("unexpected extra select: %#v", options)
		return "", false, nil
	}
	answer, err := NewDialogInteractor(dialogs).Ask(context.Background(), form.Questions[0], form)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(answer.Values, []string{"two"}) {
		t.Fatalf("unexpected answer: %#v", answer)
	}
}

func TestDialogsSecretUsesSecretDialogOnly(t *testing.T) {
	secret := []byte("synthetic-secret-value")
	store := &fakeSecretStore{}
	entries := &recordedEntries{}
	dialogs := &fakeDialogs{available: true}
	dialogs.selectFn = func(context.Context, string, []string) (string, bool, error) {
		t.Fatal("secret question opened an ordinary select")
		return "", false, nil
	}
	dialogs.inputFn = func(context.Context, string, string) (string, bool, error) {
		t.Fatal("secret question opened an ordinary input")
		return "", false, nil
	}
	dialogs.secretFn = func(_ context.Context, title, placeholder string) ([]byte, bool, error) {
		if title != "Token" || placeholder != "service-token" {
			t.Fatalf("unexpected secret dialog %q %q", title, placeholder)
		}
		return slices.Clone(secret), true, nil
	}
	deps := runDeps(NewDialogInteractor(dialogs), entries)
	deps.Store = store
	result, err := Run(context.Background(), deps, secretForm())
	if err != nil {
		t.Fatal(err)
	}
	if result.Answers["token"].Placeholder != "red-sec-abcdefghijklmnop" {
		t.Fatalf("secret was not stored as placeholder: %#v", result.Answers)
	}
	for _, text := range dialogs.text() {
		if strings.Contains(text, string(secret)) {
			t.Fatalf("dialog text exposed the secret: %q", text)
		}
	}
}

func TestDialogsSecretFailureIsValueFree(t *testing.T) {
	secret := "synthetic-secret-value"
	dialogs := &fakeDialogs{available: true}
	dialogs.secretFn = func(context.Context, string, string) ([]byte, bool, error) {
		return nil, false, errors.New(secret)
	}
	store := &fakeSecretStore{}
	deps := runDeps(NewDialogInteractor(dialogs), &recordedEntries{})
	deps.Store = store
	_, err := Run(context.Background(), deps, secretForm())
	if err == nil || strings.Contains(err.Error(), secret) || store.value != nil {
		t.Fatalf("secret dialog failure exposed the value: %v", err)
	}
}

func TestDialogsCancellationMidwayDiscardsDraft(t *testing.T) {
	entries := &recordedEntries{}
	dialogs := &fakeDialogs{available: true}
	dialogs.selectFn = func(_ context.Context, _ string, options []string) (string, bool, error) {
		if len(dialogs.selects) == 1 {
			return options[0], true, nil
		}
		return "", false, nil
	}
	result, err := Run(context.Background(), runDeps(NewDialogInteractor(dialogs), entries), multiForm())
	if err != nil || !result.Cancelled || len(result.Answers) != 0 || len(result.Order) != 0 {
		t.Fatalf("cancellation retained a draft: %#v, %v", result, err)
	}
	if len(entries.records) != 2 || entries.records[1].State != StateCancelled {
		t.Fatalf("expected the pending record resolved as cancelled: %#v", entries.records)
	}
}

func TestDialogsDismissedNoteEditorCancels(t *testing.T) {
	dialogs := &fakeDialogs{available: true}
	dialogs.selectFn = func(context.Context, string, []string) (string, bool, error) {
		return noteLabel, true, nil
	}
	dialogs.inputFn = func(context.Context, string, string) (string, bool, error) {
		return "", false, nil
	}
	_, err := NewDialogInteractor(dialogs).Ask(context.Background(), validForm().Questions[0], validForm())
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("dismissed note editor was not a cancellation: %v", err)
	}
}

func TestDialogsUnavailableWithoutInteractiveHost(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ui      Interactor
		execute bool
	}{
		{name: "no-dialogs", ui: NewDialogInteractor(nil)},
		{name: "noninteractive", ui: NewDialogInteractor(&fakeDialogs{})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := &recordedEntries{}
			result, err := Run(context.Background(), runDeps(tc.ui, entries), validForm())
			if err != nil || result.Unavailable == "" || len(entries.records) != 0 {
				t.Fatalf("unavailable host started an interaction: %#v, %v", result, err)
			}
		})
	}
}

func TestDialogsHonorContext(t *testing.T) {
	t.Run("dialog-timeout", func(t *testing.T) {
		entries := &recordedEntries{}
		dialogs := &fakeDialogs{available: true}
		dialogs.selectFn = func(ctx context.Context, _ string, _ []string) (string, bool, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("dialog context carried no deadline")
			}
			<-ctx.Done()
			return "", false, ctx.Err()
		}
		deps := runDeps(NewDialogInteractor(dialogs), entries)
		deps.Options.DialogTimeout = 5 * time.Millisecond
		result, err := Run(context.Background(), deps, validForm())
		if err != nil || !result.TimedOut || len(result.Answers) != 0 || len(entries.records) != 2 || entries.records[1].State != StateTimedOut {
			t.Fatalf("dialog timeout was not terminal: %#v, %v", result, err)
		}
	})
	t.Run("parent-cancel", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		entries := &recordedEntries{}
		dialogs := &fakeDialogs{available: true}
		dialogs.selectFn = func(ctx context.Context, _ string, _ []string) (string, bool, error) {
			cancel()
			<-ctx.Done()
			return "", false, ctx.Err()
		}
		deps := runDeps(NewDialogInteractor(dialogs), entries)
		deps.Options.DialogTimeout = time.Second
		result, err := Run(ctx, deps, validForm())
		if err != nil || !result.Cancelled || result.TimedOut || len(entries.records) != 2 {
			t.Fatalf("parent cancellation was misclassified: %#v, %v", result, err)
		}
	})
}

func TestDialogsDisambiguatesLabelsThatCollide(t *testing.T) {
	form := Form{Questions: []Question{{
		ID: "collide", Label: "Collide", Type: KindSingle,
		Options: []Option{{Value: "keep", Label: skipLabel}},
	}}}
	dialogs := &fakeDialogs{available: true}
	dialogs.selectFn = func(_ context.Context, _ string, options []string) (string, bool, error) {
		if !reflect.DeepEqual(options, []string{skipLabel, noteLabel, skipLabel + " [2]"}) {
			t.Fatalf("colliding labels were not disambiguated: %#v", options)
		}
		return options[0], true, nil
	}
	answer, err := NewDialogInteractor(dialogs).Ask(context.Background(), form.Questions[0], form)
	if err != nil || !reflect.DeepEqual(answer.Values, []string{"keep"}) {
		t.Fatalf("colliding option label selected the wrong entry: %#v, %v", answer, err)
	}
}

func TestDialogsFreeformWithNote(t *testing.T) {
	form := Form{Questions: []Question{{ID: "free", Label: "Comment", Type: KindFreeform}}}
	entries := &recordedEntries{}
	dialogs := &fakeDialogs{available: true}
	dialogs.inputFn = func(_ context.Context, _, placeholder string) (string, bool, error) {
		if placeholder == notePlaceholder {
			return "second thought", true, nil
		}
		return "first thought", true, nil
	}
	dialogs.selectFn = func(_ context.Context, _ string, options []string) (string, bool, error) {
		switch len(dialogs.selects) {
		case 1:
			if !reflect.DeepEqual(options, []string{acceptLabel, editLabel, noteLabel}) {
				t.Fatalf("unexpected freeform choices: %#v", options)
			}
			return noteLabel, true, nil
		case 2:
			if !reflect.DeepEqual(options, []string{acceptLabel, editLabel, editNoteLabel}) {
				t.Fatalf("saved note was not reflected: %#v", options)
			}
			return acceptLabel, true, nil
		}
		t.Fatalf("unexpected extra select: %#v", options)
		return "", false, nil
	}
	result, err := Run(context.Background(), runDeps(NewDialogInteractor(dialogs), entries), form)
	if err != nil {
		t.Fatal(err)
	}
	answer := result.Answers["free"]
	if answer.Custom != "first thought" || answer.Note != "second thought" {
		t.Fatalf("unexpected freeform answer: %#v", answer)
	}
}

func TestDialogsRejectUnknownChoice(t *testing.T) {
	dialogs := &fakeDialogs{available: true}
	dialogs.selectFn = func(context.Context, string, []string) (string, bool, error) {
		return "invented", true, nil
	}
	_, err := NewDialogInteractor(dialogs).Ask(context.Background(), validForm().Questions[0], validForm())
	if err == nil || errors.Is(err, ErrCancelled) {
		t.Fatalf("unknown dialog choice was accepted: %v", err)
	}
}
