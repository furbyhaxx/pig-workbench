package ask

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fakeInteractor struct {
	unavailable error
	calls       []string
	ask         func(context.Context, Question, Form) (Answer, error)
}

func (f *fakeInteractor) Unavailable() error { return f.unavailable }

func (f *fakeInteractor) Ask(ctx context.Context, q Question, form Form) (Answer, error) {
	f.calls = append(f.calls, q.ID)
	return f.ask(ctx, q, form)
}

type recordedEntries struct {
	records []Record
	wire    [][]byte
}

func (c *recordedEntries) append(kind string, payload any) error {
	if kind != "pig-workbench.ask" {
		return errors.New("wrong custom entry type")
	}
	record, ok := payload.(Record)
	if !ok {
		return errors.New("wrong record type")
	}
	wire, err := json.Marshal(record)
	if err != nil {
		return err
	}
	c.records = append(c.records, record)
	c.wire = append(c.wire, wire)
	return nil
}

type secretStoreFunc func(name, scope, description, env string, value []byte) (string, error)

func (f secretStoreFunc) Store(name, scope, description, env string, value []byte) (string, error) {
	return f(name, scope, description, env, value)
}

type fakeSecretStore struct {
	name, scope, description, env string
	value                         []byte
	placeholder                   string
	err                           error
}

func (f *fakeSecretStore) Store(name, scope, description, env string, value []byte) (string, error) {
	f.name, f.scope, f.description, f.env = name, scope, description, env
	f.value = append([]byte(nil), value...)
	placeholder := f.placeholder
	if placeholder == "" {
		placeholder = "red-sec-abcdefghijklmnop"
	}
	return placeholder, f.err
}

func runDeps(ui Interactor, entries *recordedEntries) Deps {
	return Deps{
		SessionID: "session-1", Interactor: ui, Append: entries.append,
		Clock: func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) },
	}
}

func twoQuestions() Form {
	form := validForm()
	first := form.Questions[0]
	first.ID = "z-first"
	second := first
	second.ID = "a-second"
	form.Questions = []Question{first, second}
	return form
}

func secretForm() Form {
	return Form{Title: "Credentials", Questions: []Question{{
		ID: "token", Label: "Token", Type: KindSecret,
		Secret: &SecretSpec{Name: "service-token", Scope: "project", Env: "SERVICE_TOKEN", Description: "API access"},
	}}}
}

func TestRunCancellationDropsAllAnswersAndKeepsPending(t *testing.T) {
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(_ context.Context, q Question, _ Form) (Answer, error) {
		if q.ID == "z-first" {
			return Answer{Values: []string{"project"}, Note: "first note"}, nil
		}
		return Answer{}, ErrCancelled
	}}
	result, err := Run(context.Background(), runDeps(ui, entries), twoQuestions())
	if err != nil || !result.Cancelled || len(result.Answers) != 0 || len(result.Order) != 0 {
		t.Fatalf("cancellation retained answers: %#v, %v", result, err)
	}
	if len(entries.records) != 1 || entries.records[0].State != StatePending {
		t.Fatalf("expected exactly one pending record: %#v", entries.records)
	}
	if !reflect.DeepEqual(ui.calls, []string{"z-first", "a-second"}) {
		t.Fatalf("wrong presentation order: %v", ui.calls)
	}
	wire, err := json.Marshal(result)
	if err != nil || bytes.Contains(wire, []byte(`"answers"`)) {
		t.Fatalf("cancelled wire result retained answers: %s, %v", wire, err)
	}
}

func TestRunContextCancellationDuringForm(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(_ context.Context, q Question, _ Form) (Answer, error) {
		if q.ID == "a-second" {
			cancel()
		}
		return Answer{Values: []string{"global"}}, nil
	}}
	result, err := Run(ctx, runDeps(ui, entries), twoQuestions())
	if err != nil || !result.Cancelled || len(result.Answers) != 0 || len(entries.records) != 1 {
		t.Fatalf("cancel did not discard partial form: %#v, %v, %d records", result, err, len(entries.records))
	}
}

func TestRunCancellationWhileClockRuns(t *testing.T) {
	for _, cancelAt := range []int{1, 2} {
		ctx, cancel := context.WithCancel(context.Background())
		entries := &recordedEntries{}
		ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
			return Answer{Values: []string{"project"}}, nil
		}}
		deps := runDeps(ui, entries)
		calls := 0
		deps.Clock = func() time.Time {
			calls++
			if calls == cancelAt {
				cancel()
			}
			return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
		}
		result, err := Run(ctx, deps, validForm())
		cancel()
		if err != nil || !result.Cancelled || len(entries.records) != cancelAt-1 {
			t.Fatalf("cancellation at clock call %d: %#v, %v, %d records", cancelAt, result, err, len(entries.records))
		}
	}
}

func TestRunCancellationAndBoundaryErrors(t *testing.T) {
	t.Run("clock", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		entries := &recordedEntries{}
		ui := &fakeInteractor{}
		deps := runDeps(ui, entries)
		deps.Clock = func() time.Time { cancel(); return time.Time{} }
		result, err := Run(ctx, deps, validForm())
		if err != nil || !result.Cancelled || len(entries.records) != 0 {
			t.Fatalf("clock cancellation returned error: %#v, %v", result, err)
		}
	})
	t.Run("append", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ui := &fakeInteractor{}
		deps := runDeps(ui, &recordedEntries{})
		appendErr := errors.New("append cancelled")
		deps.Append = func(string, any) error { cancel(); return appendErr }
		result, err := Run(ctx, deps, validForm())
		if !errors.Is(err, appendErr) || result.Cancelled {
			t.Fatalf("failed append was not reported: %#v, %v", result, err)
		}
	})
	for _, tc := range []struct {
		name   string
		cancel bool
	}{
		{name: "store-failure"},
		{name: "store-failure-with-cancellation", cancel: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			secret := []byte("distinctive-synthetic-secret-value")
			const invalidPlaceholder = "invalid-store-placeholder"
			entries := &recordedEntries{}
			ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
				return Answer{Secret: secret}, nil
			}}
			var persisted []byte
			deps := runDeps(ui, entries)
			deps.Store = secretStoreFunc(func(_, _, _, _ string, value []byte) (string, error) {
				persisted = append([]byte(nil), value...)
				if tc.cancel {
					cancel()
				}
				return invalidPlaceholder, errors.New(string(secret))
			})
			result, err := Run(ctx, deps, secretForm())
			var finalization *FinalizationError
			if !errors.As(err, &finalization) || result.Cancelled || len(result.Answers) != 0 || len(entries.records) != 1 {
				t.Fatalf("store failure concealed possible persistence: %#v, %v", result, err)
			}
			if !bytes.Equal(persisted, secret) || finalization.PublicationAttempted || len(finalization.StoredPlaceholders) != 0 {
				t.Fatal("incorrect finalization state after first store failure")
			}
			wire, marshalErr := json.Marshal(result)
			if marshalErr != nil || bytes.Contains(wire, secret) || bytes.Contains(wire, []byte(invalidPlaceholder)) || strings.Contains(err.Error(), string(secret)) || strings.Contains(err.Error(), invalidPlaceholder) {
				t.Fatal("store failure exposed a secret or invalid placeholder")
			}
			for _, entry := range entries.wire {
				if bytes.Contains(entry, secret) || bytes.Contains(entry, []byte(invalidPlaceholder)) {
					t.Fatal("store failure exposed data in a session entry")
				}
			}
		})
	}
	t.Run("cancel-before-store", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		entries := &recordedEntries{}
		ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
			cancel()
			return Answer{Secret: []byte("synthetic-value")}, nil
		}}
		deps := runDeps(ui, entries)
		deps.Store = secretStoreFunc(func(string, string, string, string, []byte) (string, error) {
			t.Fatal("store called after cancellation")
			return "", nil
		})
		result, err := Run(ctx, deps, secretForm())
		if err != nil || !result.Cancelled || len(entries.records) != 1 {
			t.Fatalf("cancellation before store was not returned: %#v, %v", result, err)
		}
	})
}

func TestRunLaterCancellationDoesNotStoreEarlierSecret(t *testing.T) {
	form := secretForm()
	second := validForm().Questions[0]
	form.Questions = append(form.Questions, second)
	store := &fakeSecretStore{}
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(_ context.Context, q Question, _ Form) (Answer, error) {
		if q.Type == KindSecret {
			return Answer{Secret: []byte("synthetic-secret-value")}, nil
		}
		return Answer{}, ErrCancelled
	}}
	deps := runDeps(ui, entries)
	deps.Store = store
	result, err := Run(context.Background(), deps, form)
	if err != nil || !result.Cancelled || len(result.Answers) != 0 || store.value != nil || len(entries.records) != 1 {
		t.Fatalf("later cancellation retained secret effects: %#v, %v", result, err)
	}
}

func TestRunInteractorCannotMutateQuestionMetadata(t *testing.T) {
	entries := &recordedEntries{}
	store := &fakeSecretStore{}
	ui := &fakeInteractor{ask: func(_ context.Context, q Question, form Form) (Answer, error) {
		q.Secret.Name = "modified-name"
		form.Questions[0].Secret.Scope = "global"
		return Answer{Secret: []byte("synthetic-secret-value")}, nil
	}}
	deps := runDeps(ui, entries)
	deps.Store = store
	_, err := Run(context.Background(), deps, secretForm())
	if err != nil || store.name != "service-token" || store.scope != "project" {
		t.Fatalf("interactor mutated authoritative secret metadata: %v", err)
	}
}

func TestRunInteractorErrorNeverExposesEnteredSecret(t *testing.T) {
	for _, failOnSecret := range []bool{true, false} {
		form := secretForm()
		if !failOnSecret {
			form.Questions = append(form.Questions, validForm().Questions[0])
		}
		secret := "synthetic-secret-value"
		entries := &recordedEntries{}
		ui := &fakeInteractor{ask: func(_ context.Context, q Question, _ Form) (Answer, error) {
			if q.Type == KindSecret && !failOnSecret {
				return Answer{Secret: []byte(secret)}, nil
			}
			return Answer{}, errors.New(secret)
		}}
		store := &fakeSecretStore{}
		deps := runDeps(ui, entries)
		deps.Store = store
		_, err := Run(context.Background(), deps, form)
		if err == nil || strings.Contains(err.Error(), secret) || store.value != nil || len(entries.records) != 1 {
			t.Fatal("interactor error exposed or stored a secret")
		}
	}
}

func TestRunValidatesLaterAnswersBeforeStoringSecrets(t *testing.T) {
	form := secretForm()
	form.Questions = append(form.Questions, validForm().Questions[0])
	store := &fakeSecretStore{}
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(_ context.Context, q Question, _ Form) (Answer, error) {
		if q.Type == KindSecret {
			return Answer{Secret: []byte("synthetic-secret-value")}, nil
		}
		return Answer{Values: []string{"not-an-option"}}, nil
	}}
	deps := runDeps(ui, entries)
	deps.Store = store
	_, err := Run(context.Background(), deps, form)
	if err == nil || store.value != nil || len(entries.records) != 1 {
		t.Fatal("invalid later answer was accepted or earlier secret was stored")
	}
}

func TestRunAllowsIncidentalSecretOverlapInOrdinaryAnswer(t *testing.T) {
	form := secretForm()
	form.Questions = append(form.Questions, Question{ID: "other", Label: "Other", Type: KindFreeform})
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(_ context.Context, q Question, _ Form) (Answer, error) {
		if q.Type == KindSecret {
			return Answer{Secret: []byte("red")}, nil
		}
		return Answer{Custom: "prefer red", Note: "red theme"}, nil
	}}
	store := &fakeSecretStore{}
	deps := runDeps(ui, entries)
	deps.Store = store
	result, err := Run(context.Background(), deps, form)
	if err != nil || !bytes.Equal(store.value, []byte("red")) || result.Answers["other"].Custom != "prefer red" || len(entries.records) != 2 {
		t.Fatalf("incidental overlap rejected: %#v, %v", result, err)
	}
}

func TestRunCancellationRacingSuccessfulCompletionIsSubmitted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
		return Answer{Values: []string{"project"}}, nil
	}}
	deps := runDeps(ui, entries)
	deps.Append = func(kind string, payload any) error {
		if err := entries.append(kind, payload); err != nil {
			return err
		}
		if payload.(Record).State == StateCompleted {
			cancel()
		}
		return nil
	}
	result, err := Run(ctx, deps, validForm())
	if err != nil || result.Cancelled || len(result.Answers) != 1 || len(entries.records) != 2 {
		t.Fatalf("acknowledged completion was not submitted: %#v, %v", result, err)
	}
}

func TestRunCancellationRacingFailedCompletionReturnsPersistenceError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
		return Answer{Values: []string{"project"}}, nil
	}}
	deps := runDeps(ui, entries)
	appendErr := errors.New("completed append failed")
	deps.Append = func(kind string, payload any) error {
		if payload.(Record).State == StateCompleted {
			cancel()
			return appendErr
		}
		return entries.append(kind, payload)
	}
	result, err := Run(ctx, deps, validForm())
	if !errors.Is(err, appendErr) || result.Cancelled || len(entries.records) != 1 {
		t.Fatalf("failed completion did not return persistence error: %#v, %v", result, err)
	}
}

func TestRunSecondSecretStoreFailureReportsOnlyStoredPlaceholder(t *testing.T) {
	form := secretForm()
	second := form.Questions[0]
	second.ID = "second"
	form.Questions = append(form.Questions, second)
	firstSecret, secondSecret := []byte("synthetic-first-secret"), []byte("synthetic-second-secret")
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(_ context.Context, q Question, _ Form) (Answer, error) {
		if q.ID == "token" {
			return Answer{Secret: firstSecret}, nil
		}
		return Answer{Secret: secondSecret}, nil
	}}
	calls := 0
	deps := runDeps(ui, entries)
	deps.Store = secretStoreFunc(func(string, string, string, string, []byte) (string, error) {
		calls++
		if calls == 1 {
			return "red-sec-aaaaaaaaaaaaaaaa", nil
		}
		return "", errors.New(string(secondSecret))
	})
	result, err := Run(context.Background(), deps, form)
	var finalization *FinalizationError
	if !errors.As(err, &finalization) || finalization.PublicationAttempted || !reflect.DeepEqual(finalization.StoredPlaceholders, []string{"red-sec-aaaaaaaaaaaaaaaa"}) || len(result.Answers) != 0 || len(entries.records) != 1 {
		t.Fatalf("partial finalization was not reported safely: %v", err)
	}
	if strings.Contains(err.Error(), string(firstSecret)) || strings.Contains(err.Error(), string(secondSecret)) {
		t.Fatal("finalization error exposed a secret")
	}
	wire, err := json.Marshal(result)
	if err != nil || bytes.Contains(wire, firstSecret) || bytes.Contains(wire, secondSecret) {
		t.Fatal("partial result exposed a secret")
	}
	for _, entry := range entries.wire {
		if bytes.Contains(entry, firstSecret) || bytes.Contains(entry, secondSecret) {
			t.Fatal("session entry exposed a secret")
		}
	}
}

func TestRunCancellationAfterStoreReportsPartialFinalization(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
		return Answer{Secret: []byte("synthetic-secret-value")}, nil
	}}
	deps := runDeps(ui, entries)
	deps.Store = secretStoreFunc(func(string, string, string, string, []byte) (string, error) {
		cancel()
		return "red-sec-aaaaaaaaaaaaaaaa", nil
	})
	result, err := Run(ctx, deps, secretForm())
	var finalization *FinalizationError
	if !errors.As(err, &finalization) || finalization.PublicationAttempted || !reflect.DeepEqual(finalization.StoredPlaceholders, []string{"red-sec-aaaaaaaaaaaaaaaa"}) || result.Cancelled || len(entries.records) != 1 {
		t.Fatalf("stored secret was concealed by cancellation: %v", err)
	}
}

func TestRunCompletedAppendFailureReportsPublicationAttempt(t *testing.T) {
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
		return Answer{Secret: []byte("synthetic-secret-value")}, nil
	}}
	deps := runDeps(ui, entries)
	deps.Store = &fakeSecretStore{}
	deps.Append = func(kind string, payload any) error {
		if payload.(Record).State == StateCompleted {
			return errors.New("append failed")
		}
		return entries.append(kind, payload)
	}
	result, err := Run(context.Background(), deps, secretForm())
	var finalization *FinalizationError
	if !errors.As(err, &finalization) || !finalization.PublicationAttempted || !reflect.DeepEqual(finalization.StoredPlaceholders, []string{"red-sec-abcdefghijklmnop"}) || len(result.Answers) != 0 || len(entries.records) != 1 {
		t.Fatalf("publication failure was not reported: %v", err)
	}
}

func TestRunStoresSecretsOnlyAfterAllQuestions(t *testing.T) {
	form := secretForm()
	form.Questions = append(form.Questions, validForm().Questions[0])
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(_ context.Context, q Question, _ Form) (Answer, error) {
		if q.Type == KindSecret {
			return Answer{Secret: []byte("synthetic-secret-value")}, nil
		}
		return Answer{Values: []string{"project"}, Note: "note"}, nil
	}}
	deps := runDeps(ui, entries)
	deps.Store = secretStoreFunc(func(string, string, string, string, []byte) (string, error) {
		if !reflect.DeepEqual(ui.calls, []string{"token", "scope"}) {
			t.Fatal("secret stored before all questions were answered")
		}
		return "red-sec-abcdefghijklmnop", nil
	})
	result, err := Run(context.Background(), deps, form)
	if err != nil || !reflect.DeepEqual(result.Order, []string{"token", "scope"}) || result.Answers["scope"].Note != "note" {
		t.Fatalf("secret finalization lost ordered answers: %#v, %v", result, err)
	}
}

func TestRunSecretStoreNeverPersistsPlaintext(t *testing.T) {
	secret := []byte("synthetic-secret-value-for-test")
	store := &fakeSecretStore{}
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
		return Answer{Secret: secret, Custom: string(secret), Note: string(secret), Values: []string{string(secret)}}, nil
	}}
	deps := runDeps(ui, entries)
	deps.Store = store
	result, err := Run(context.Background(), deps, secretForm())
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(store.value, secret) || store.name != "service-token" || store.scope != "project" || store.env != "SERVICE_TOKEN" || store.description != "API access" {
		t.Fatal("secret store did not receive the value and metadata")
	}
	if result.Answers["token"].Placeholder != "red-sec-abcdefghijklmnop" {
		t.Fatal("missing secret placeholder")
	}
	if len(entries.records) != 2 || entries.records[0].State != StatePending || entries.records[1].State != StateCompleted {
		t.Fatal("pending and completed records were not appended")
	}
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(wire, secret) {
		t.Fatal("secret appeared in result")
	}
	for _, entry := range entries.wire {
		if bytes.Contains(entry, secret) {
			t.Fatal("secret appeared in an appended record")
		}
	}
}

func TestRunRejectsInvalidStorePlaceholderContainingSecret(t *testing.T) {
	secret := []byte("synthetic-secret-value-for-test")
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
		return Answer{Secret: secret}, nil
	}}
	deps := runDeps(ui, entries)
	deps.Store = &fakeSecretStore{placeholder: "wrong-" + string(secret)}
	result, err := Run(context.Background(), deps, secretForm())
	var finalization *FinalizationError
	if !errors.As(err, &finalization) || finalization.PublicationAttempted || len(finalization.StoredPlaceholders) != 0 || len(result.Answers) != 0 || len(entries.records) != 1 {
		t.Fatal("unsafe store placeholder was accepted or completed")
	}
	if strings.Contains(err.Error(), string(secret)) {
		t.Fatal("secret appeared in storage error")
	}
	for _, wire := range entries.wire {
		if bytes.Contains(wire, secret) {
			t.Fatal("secret appeared in an appended record")
		}
	}
}

func TestRunRejectsNoncanonicalStorePlaceholders(t *testing.T) {
	for _, placeholder := range []string{
		"", " red-sec-abcdefghijklmnop", "red-sec-abcdefghijklmnop-extra",
		"red-sec-abcdefghijklmnop\n", "red-sec-ABCDEFGHIJKLMNOP",
	} {
		entries := &recordedEntries{}
		ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
			return Answer{Secret: []byte("synthetic-secret-value")}, nil
		}}
		deps := runDeps(ui, entries)
		deps.Store = secretStoreFunc(func(string, string, string, string, []byte) (string, error) {
			return placeholder, nil
		})
		_, err := Run(context.Background(), deps, secretForm())
		var finalization *FinalizationError
		if !errors.As(err, &finalization) || len(finalization.StoredPlaceholders) != 0 || finalization.PublicationAttempted || len(entries.records) != 1 {
			t.Fatalf("noncanonical placeholder accepted: %q", placeholder)
		}
	}
}

func TestRunSecretStoreErrorIsValueFree(t *testing.T) {
	secret := []byte("synthetic-secret-value-for-test")
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
		return Answer{Secret: secret}, nil
	}}
	deps := runDeps(ui, entries)
	deps.Store = &fakeSecretStore{err: errors.New(string(secret))}
	_, err := Run(context.Background(), deps, secretForm())
	if err == nil || strings.Contains(err.Error(), string(secret)) || len(entries.records) != 1 {
		t.Fatal("secret store failure was not safely returned")
	}
}

func TestRunExistingSecretPlaceholder(t *testing.T) {
	value := "red-sec-abcdefghijklmnop"
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
		return Answer{Secret: []byte(value)}, nil
	}}
	deps := runDeps(ui, entries)
	deps.Store = &fakeSecretStore{placeholder: value}
	result, err := Run(context.Background(), deps, secretForm())
	if err != nil || result.Answers["token"].Placeholder != value {
		t.Fatalf("existing placeholder was rejected: %v", err)
	}
}

func TestRunSecretRequiresStore(t *testing.T) {
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
		t.Fatal("secret input was requested without a store")
		return Answer{}, nil
	}}
	_, err := Run(context.Background(), runDeps(ui, entries), secretForm())
	var unavailable *SecretStorageUnavailableError
	if !errors.As(err, &unavailable) || len(entries.records) != 0 {
		t.Fatalf("expected typed secret storage error with no records: %v", err)
	}
}

func TestRunUnavailableDoesNotPersist(t *testing.T) {
	entries := &recordedEntries{}
	ui := &fakeInteractor{unavailable: &InteractionUnavailableError{Reason: "print mode"}}
	result, err := Run(context.Background(), runDeps(ui, entries), validForm())
	if err != nil || result.Unavailable != "print mode" || len(entries.records) != 0 {
		t.Fatalf("unavailable: %#v, %v, %d records", result, err, len(entries.records))
	}
	wire, err := json.Marshal(result)
	if err != nil || string(wire) != `{"interactionUnavailable":true,"reason":"print mode"}` {
		t.Fatalf("unexpected unavailable result: %s, %v", wire, err)
	}
}

func TestRunTimeoutIsOursAndDropsAnswers(t *testing.T) {
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(ctx context.Context, q Question, _ Form) (Answer, error) {
		if q.ID == "z-first" {
			return Answer{Values: []string{"project"}}, nil
		}
		<-ctx.Done()
		return Answer{}, ctx.Err()
	}}
	deps := runDeps(ui, entries)
	deps.Options.DialogTimeout = 5 * time.Millisecond
	result, err := Run(context.Background(), deps, twoQuestions())
	if err != nil || !result.TimedOut || result.Reason == "" || len(result.Answers) != 0 || len(entries.records) != 1 {
		t.Fatalf("timeout was not terminal: %#v, %v, %d records", result, err, len(entries.records))
	}
}

func TestRunDoesNotMisclassifyInteractorError(t *testing.T) {
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
		return Answer{}, context.DeadlineExceeded
	}}
	deps := runDeps(ui, entries)
	deps.Options.DialogTimeout = time.Second
	result, err := Run(context.Background(), deps, validForm())
	if !errors.Is(err, context.DeadlineExceeded) || result.TimedOut || len(entries.records) != 1 {
		t.Fatalf("interactor error misclassified: %#v, %v", result, err)
	}
}

func TestRunParentDeadlineIsCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(ctx context.Context, _ Question, _ Form) (Answer, error) {
		<-ctx.Done()
		return Answer{}, ctx.Err()
	}}
	deps := runDeps(ui, entries)
	deps.Options.DialogTimeout = time.Second
	result, err := Run(ctx, deps, validForm())
	if err != nil || !result.Cancelled || result.TimedOut || len(entries.records) != 1 {
		t.Fatalf("parent deadline was misclassified: %#v, %v", result, err)
	}
}

func TestRunAnswersAndStableWireOrder(t *testing.T) {
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(_ context.Context, q Question, _ Form) (Answer, error) {
		return Answer{Values: []string{"project"}, Note: q.ID + " note"}, nil
	}}
	result, err := Run(context.Background(), runDeps(ui, entries), twoQuestions())
	if err != nil || !reflect.DeepEqual(result.Order, []string{"z-first", "a-second"}) || !reflect.DeepEqual(result.Answers["z-first"].Values, []string{"project"}) || result.Answers["z-first"].Note != "z-first note" || result.Answers["a-second"].Note != "a-second note" {
		t.Fatalf("incorrect ordered answers: %#v, %v", result, err)
	}
	if len(entries.records) != 2 || entries.records[1].State != StateCompleted {
		t.Fatal("completed record missing")
	}
	// This pins deterministic formatting; JSON object order is not a consumer contract.
	result.Answers = map[string]Answered{"a-second": result.Answers["a-second"], "z-first": result.Answers["z-first"]}
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	first, second := strings.Index(string(wire), `"z-first"`), strings.Index(string(wire), `"a-second"`)
	if first < 0 || second < 0 || first >= second {
		t.Fatalf("wire format not deterministic: %s", wire)
	}
}

func TestMarshalTerminalResultsSuppressPartialAnswers(t *testing.T) {
	for _, result := range []Result{
		{Cancelled: true},
		{TimedOut: true, Reason: "dialog timed out"},
		{Unavailable: "print mode"},
	} {
		result.Answers = map[string]Answered{"scope": {Values: []string{"project"}}}
		wire, err := json.Marshal(result)
		if err != nil || bytes.Contains(wire, []byte(`"answers"`)) {
			t.Fatalf("terminal result included partial answers: %s, %v", wire, err)
		}
	}
}

func TestRunUnansweredRequiredQuestionDoesNotInventAnswer(t *testing.T) {
	entries := &recordedEntries{}
	ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
		return Answer{}, nil
	}}
	result, err := Run(context.Background(), runDeps(ui, entries), validForm())
	if err != nil || len(result.Answers) != 0 || len(entries.records) != 2 {
		t.Fatalf("unanswered advisory question was not submitted empty: %#v, %v", result, err)
	}
	wire, err := json.Marshal(result)
	if err != nil || string(wire) != `{"answers":{}}` {
		t.Fatalf("unexpected empty result: %s, %v", wire, err)
	}
}

func TestRunOptionsAndAppendFailure(t *testing.T) {
	ui := &fakeInteractor{ask: func(context.Context, Question, Form) (Answer, error) {
		t.Fatal("unexpected question")
		return Answer{}, nil
	}}
	entries := &recordedEntries{}
	deps := runDeps(ui, entries)
	deps.Options.DialogTimeout = -time.Second
	if _, err := Run(context.Background(), deps, validForm()); err == nil || len(entries.records) != 0 {
		t.Fatalf("negative timeout accepted: %v", err)
	}
	deps.Options = Options{MaxQuestions: 1}
	if _, err := Run(context.Background(), deps, twoQuestions()); err == nil || len(entries.records) != 0 {
		t.Fatalf("question limit ignored: %v", err)
	}
	deps.Options = Options{}
	writeErr := errors.New("append failed")
	deps.Append = func(string, any) error { return writeErr }
	if _, err := Run(context.Background(), deps, validForm()); !errors.Is(err, writeErr) {
		t.Fatalf("append failure not returned: %v", err)
	}
}
