package contribution

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/furbyhaxx/pig-workbench/api"
)

type testDescriptor struct {
	Label   string        `json:"label"`
	Schemes []string      `json:"schemes"`
	Sources []SourceClaim `json:"sources"`
	Reject  bool          `json:"reject"`
}

func testValidator(kind Kind, key string, descriptor json.RawMessage) (Claims, error) {
	var value testDescriptor
	if err := json.Unmarshal(descriptor, &value); err != nil {
		return Claims{}, fmt.Errorf("decode %s descriptor: %w", kind, err)
	}
	if value.Reject {
		return Claims{}, errors.New("label is rejected by the host schema")
	}
	return Claims{Schemes: value.Schemes, Sources: value.Sources}, nil
}

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := New("g-1", Options{Validator: testValidator})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func testIdentity(instance string) api.Identity {
	return api.Identity{Owner: "build-tools", Instance: instance}
}

func mustRegister(t *testing.T, registry *Registry, identity api.Identity, kind Kind, key string, descriptor string) Receipt {
	t.Helper()
	receipt, err := registry.Register(identity, kind, key, json.RawMessage(descriptor))
	if err != nil {
		t.Fatalf("register %s/%s: %v", kind, key, err)
	}
	return receipt
}

func TestRegisterIsIdempotentForCanonicallyEqualDescriptors(t *testing.T) {
	registry := newTestRegistry(t)
	identity := testIdentity("i-1")
	first := mustRegister(t, registry, identity, KindResourceProvider, "builds",
		`{"label":"Builds","schemes":["build"],"sources":[{"source":"session.model","field":"knowledge_cutoff"}]}`)
	second := mustRegister(t, registry, identity, KindResourceProvider, "builds",
		"{ \"sources\": [ {\"field\": \"knowledge_cutoff\", \"source\": \"session.model\"} ],\n \"schemes\": [\"build\"], \"label\": \"Builds\" }")
	if second.RegistrationID != first.RegistrationID {
		t.Fatalf("reordered descriptor changed the registration ID: %q vs %q", first.RegistrationID, second.RegistrationID)
	}
	if !second.Idempotent || second.DescriptorRevision != first.DescriptorRevision {
		t.Fatalf("identical descriptor was not idempotent: %+v", second)
	}
	if listed := registry.List(); len(listed) != 1 {
		t.Fatalf("idempotent register added a registration: %d", len(listed))
	}
	if _, err := registry.Register(identity, KindResourceProvider, "builds",
		json.RawMessage(`{"label":"Builds v2","schemes":["build"]}`)); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed descriptor did not conflict: %v", err)
	}
	current, err := registry.Get(identity, first.RegistrationID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(current.Descriptor), `"Builds"`) || current.DescriptorRevision != first.DescriptorRevision {
		t.Fatalf("conflict changed the live descriptor: %s", current.Descriptor)
	}
}

func TestReplaceIsRevisionGuardedAndAtomic(t *testing.T) {
	registry := newTestRegistry(t)
	identity := testIdentity("i-1")
	receipt := mustRegister(t, registry, identity, KindResourceProvider, "builds", `{"label":"v1","schemes":["build"]}`)

	if _, err := registry.Replace(identity, receipt.RegistrationID, "rev-unknown",
		json.RawMessage(`{"label":"v2","schemes":["build"]}`)); !errors.Is(err, ErrStaleRevision) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	current, err := registry.Get(identity, receipt.RegistrationID)
	if err != nil {
		t.Fatal(err)
	}
	if string(current.Descriptor) != `{"label":"v1","schemes":["build"]}` {
		t.Fatalf("stale replace changed the descriptor: %s", current.Descriptor)
	}

	other := api.Identity{Owner: "pig-web", Instance: "w-1"}
	mustRegister(t, registry, other, KindResourceProvider, "kb", `{"label":"kb","schemes":["kb"]}`)
	if _, err := registry.Replace(identity, receipt.RegistrationID, receipt.DescriptorRevision,
		json.RawMessage(`{"label":"v2","schemes":["kb"]}`)); !errors.Is(err, ErrConflict) {
		t.Fatalf("claim conflict during replace was not reported: %v", err)
	}
	current, err = registry.Get(identity, receipt.RegistrationID)
	if err != nil {
		t.Fatal(err)
	}
	if string(current.Descriptor) != `{"label":"v1","schemes":["build"]}` {
		t.Fatalf("failed replace changed the descriptor: %s", current.Descriptor)
	}
	if _, err := registry.Register(other, KindResourceProvider, "build-now", json.RawMessage(`{"label":"b","schemes":["build"]}`)); !errors.Is(err, ErrConflict) {
		t.Fatalf("failed replace released the old scheme claim: %v", err)
	}

	replaced, err := registry.Replace(identity, receipt.RegistrationID, receipt.DescriptorRevision,
		json.RawMessage(`{"label":"v2","schemes":["build2"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Idempotent || replaced.DescriptorRevision == receipt.DescriptorRevision {
		t.Fatalf("replace did not advance the revision: %+v", replaced)
	}
	current, err = registry.Get(identity, replaced.RegistrationID)
	if err != nil {
		t.Fatal(err)
	}
	if string(current.Descriptor) != `{"label":"v2","schemes":["build2"]}` {
		t.Fatalf("replace did not store the new descriptor: %s", current.Descriptor)
	}
	mustRegister(t, registry, other, KindResourceProvider, "build-any", `{"label":"b","schemes":["build"]}`)
	if _, err := registry.Register(other, KindResourceProvider, "build2-any", json.RawMessage(`{"label":"b","schemes":["build2"]}`)); !errors.Is(err, ErrConflict) {
		t.Fatalf("replacement did not keep its new claim: %v", err)
	}
}

func TestReplaceOwnerRetiresPreviousIncarnation(t *testing.T) {
	registry := newTestRegistry(t)
	old := testIdentity("i-1")
	next := testIdentity("i-2")
	other := api.Identity{Owner: "pig-web", Instance: "w-1"}
	first := mustRegister(t, registry, old, KindResourceProvider, "builds", `{"label":"builds","schemes":["build"]}`)
	mustRegister(t, registry, old, KindDocs, "manual", `{"label":"manual"}`)
	foreign := mustRegister(t, registry, other, KindResourceProvider, "kb", `{"label":"kb","schemes":["kb"]}`)

	withdrawal, err := registry.ReplaceOwner("build-tools", "i-1", "i-2")
	if err != nil {
		t.Fatal(err)
	}
	if withdrawal.Owner != "build-tools" || withdrawal.Instance != "i-1" || len(withdrawal.Registrations) != 2 {
		t.Fatalf("withdrawal does not describe the retired incarnation: %+v", withdrawal)
	}
	for _, registration := range withdrawal.Registrations {
		if registration.Instance != "i-1" || len(registration.Descriptor) == 0 {
			t.Fatalf("withdrawal lost a descriptor snapshot: %+v", registration)
		}
	}

	if _, err := registry.Get(next, first.RegistrationID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("retired registration ID is still available: %v", err)
	}
	if _, err := registry.Get(old, first.RegistrationID); !errors.Is(err, ErrStaleInstance) {
		t.Fatalf("stale incarnation read its old registration: %v", err)
	}
	if _, err := registry.Register(old, KindDocs, "manual-2", json.RawMessage(`{"label":"x"}`)); !errors.Is(err, ErrStaleInstance) {
		t.Fatalf("stale incarnation registered: %v", err)
	}
	if err := registry.Unregister(old, first.RegistrationID); !errors.Is(err, ErrStaleInstance) {
		t.Fatalf("stale incarnation unregistered: %v", err)
	}
	if _, err := registry.Replace(old, first.RegistrationID, "rev-any", json.RawMessage(`{"label":"x"}`)); !errors.Is(err, ErrStaleInstance) {
		t.Fatalf("stale incarnation replaced: %v", err)
	}
	mustRegister(t, registry, next, KindResourceProvider, "builds", `{"label":"builds","schemes":["build"]}`)

	if _, err := registry.Get(other, foreign.RegistrationID); err != nil {
		t.Fatalf("replacement touched another owner: %v", err)
	}
	repeated, err := registry.ReplaceOwner("build-tools", "i-1", "i-2")
	if err != nil || len(repeated.Registrations) != 0 {
		t.Fatalf("repeated replacement was not a no-op: %+v %v", repeated, err)
	}
	mustRegister(t, registry, next, KindDocs, "manual-2", `{"label":"manual"}`)
}

func TestRetireOwnerReleasesClaims(t *testing.T) {
	registry := newTestRegistry(t)
	old := testIdentity("i-1")
	receipt := mustRegister(t, registry, old, KindResourceProvider, "builds", `{"label":"builds","schemes":["build"]}`)
	other := api.Identity{Owner: "pig-web", Instance: "w-1"}

	withdrawal, err := registry.RetireOwner(old)
	if err != nil {
		t.Fatal(err)
	}
	if len(withdrawal.Registrations) != 1 || withdrawal.Registrations[0].RegistrationID != receipt.RegistrationID {
		t.Fatalf("retirement withdrew %+v", withdrawal)
	}
	mustRegister(t, registry, other, KindResourceProvider, "kb", `{"label":"kb","schemes":["build"]}`)
	if _, err := registry.Register(old, KindDocs, "manual", json.RawMessage(`{"label":"manual"}`)); !errors.Is(err, ErrStaleInstance) {
		t.Fatalf("retired incarnation registered again: %v", err)
	}
	if _, err := registry.Register(testIdentity("i-2"), KindDocs, "manual", json.RawMessage(`{"label":"manual"}`)); !errors.Is(err, ErrInstanceConflict) {
		t.Fatalf("retired owner was reactivated implicitly: %v", err)
	}
	if _, err := registry.ReplaceOwner("build-tools", "", "i-2"); err != nil {
		t.Fatalf("explicit activation failed: %v", err)
	}
	mustRegister(t, registry, testIdentity("i-2"), KindDocs, "manual", `{"label":"manual"}`)
	repeated, err := registry.RetireOwner(old)
	if err != nil || len(repeated.Registrations) != 0 {
		t.Fatalf("repeated retirement was not a no-op: %+v %v", repeated, err)
	}
}

func TestSecondIncarnationNeedsExplicitReplacement(t *testing.T) {
	registry := newTestRegistry(t)
	first := mustRegister(t, registry, testIdentity("i-1"), KindDocs, "manual", `{"label":"manual"}`)
	if _, err := registry.Register(testIdentity("i-2"), KindDocs, "manual", json.RawMessage(`{"label":"manual"}`)); !errors.Is(err, ErrInstanceConflict) {
		t.Fatalf("arrival order admitted a second incarnation: %v", err)
	}
	if _, err := registry.Get(testIdentity("i-2"), first.RegistrationID); !errors.Is(err, ErrInstanceConflict) {
		t.Fatalf("foreign incarnation read the registration: %v", err)
	}
	if _, err := registry.ReplaceOwner("build-tools", "i-9", "i-2"); !errors.Is(err, ErrStaleInstance) {
		t.Fatalf("replacement of a non-active incarnation was accepted: %v", err)
	}
	if _, err := registry.ReplaceOwner("unregistered", "i-1", "i-2"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("replacement of an unknown owner was accepted: %v", err)
	}
	if _, err := registry.ReplaceOwner("build-tools", "i-1", "i-1"); !errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("self replacement was accepted: %v", err)
	}
}

func TestSchemeAndSourceClaimsAreExclusive(t *testing.T) {
	registry := newTestRegistry(t)
	owner := testIdentity("i-1")
	other := api.Identity{Owner: "pig-web", Instance: "w-1"}
	mustRegister(t, registry, owner, KindResourceProvider, "builds", `{"label":"builds","schemes":["build"]}`)
	mustRegister(t, registry, owner, KindFrameProducer, "facts", `{"label":"facts","sources":[{"source":"session.model","field":"knowledge_cutoff"}]}`)
	mustRegister(t, registry, owner, KindDocs, "manual", `{"label":"manual"}`)

	cases := []struct {
		name       string
		identity   api.Identity
		kind       Kind
		key        string
		descriptor string
		namespace  string
		claim      string
	}{
		{"scheme", other, KindResourceProvider, "kb", `{"label":"kb","schemes":["build"]}`, "scheme", "build"},
		{"alias", other, KindResourceProvider, "kb", `{"label":"kb","schemes":["kb","build"]}`, "scheme", "build"},
		{"same owner other key", owner, KindResourceProvider, "builds-2", `{"label":"b","schemes":["build"]}`, "scheme", "build"},
		{"source", other, KindFrameProducer, "facts", `{"label":"f","sources":[{"source":"session.model","field":"knowledge_cutoff"}]}`, "source", "session.model/knowledge_cutoff"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := registry.Register(test.identity, test.kind, test.key, json.RawMessage(test.descriptor))
			if !errors.Is(err, ErrConflict) {
				t.Fatalf("claim conflict not reported: %v", err)
			}
			var conflict *ConflictError
			if !errors.As(err, &conflict) || conflict.Namespace != test.namespace || conflict.Claim != test.claim {
				t.Fatalf("conflict does not name %s claim %q: %#v", test.namespace, test.claim, err)
			}
			if conflict.Owner != "build-tools" {
				t.Fatalf("conflict does not name the holder: %q", conflict.Owner)
			}
			if listed := registry.List(); len(listed) != 3 {
				t.Fatalf("refused registration was partly applied: %d registrations", len(listed))
			}
		})
	}

	// Stable keys are owner-scoped: the same key from another owner is independent.
	foreign := mustRegister(t, registry, other, KindResourceProvider, "builds", `{"label":"builds","schemes":["builds"]}`)
	if foreign.RegistrationID == "" {
		t.Fatal("independent owner registration has no ID")
	}
}

func TestLimitsAreAtomicAndNamed(t *testing.T) {
	registry, err := New("g-1", Options{Validator: testValidator, Limits: Limits{PerOwner: 2}})
	if err != nil {
		t.Fatal(err)
	}
	identity := testIdentity("i-1")
	mustRegister(t, registry, identity, KindDocs, "one", `{"label":"one"}`)
	mustRegister(t, registry, identity, KindDocs, "two", `{"label":"two"}`)
	_, err = registry.Register(identity, KindDocs, "three", json.RawMessage(`{"label":"three"}`))
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("owner limit not enforced: %v", err)
	}
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Scope != "owner" || limit.Limit != 2 {
		t.Fatalf("limit error does not name the limit: %#v", err)
	}
	if listed := registry.List(); len(listed) != 2 {
		t.Fatalf("limit refusal changed the registry: %d", len(listed))
	}
	mustRegister(t, registry, api.Identity{Owner: "pig-web", Instance: "w-1"}, KindDocs, "three", `{"label":"three"}`)
}

func TestDefaultLimitsMatchTheSpecification(t *testing.T) {
	registry := newTestRegistry(t)
	identity := testIdentity("i-1")
	for index := range MaxRegistrationsPerOwner {
		mustRegister(t, registry, identity, KindDocs, fmt.Sprintf("key-%d", index), `{"label":"x"}`)
	}
	_, err := registry.Register(identity, KindDocs, "overflow", json.RawMessage(`{"label":"x"}`))
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("default owner limit not enforced: %v", err)
	}
	_, err = registry.Register(testIdentity("i-2"), KindDocs, "huge",
		json.RawMessage(`{"pad":"`+strings.Repeat("a", MaxDescriptorBytes)+`"}`))
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("descriptor limit not enforced: %v", err)
	}
	var limit *LimitError
	if !errors.As(err, &limit) || limit.Scope != "descriptor" || limit.Limit != MaxDescriptorBytes {
		t.Fatalf("descriptor limit error does not name the limit: %#v", err)
	}
	if _, err := New("g-1", Options{Validator: testValidator, Limits: Limits{PerOwner: MaxRegistrationsPerOwner + 1}}); !errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("raised owner limit was accepted: %v", err)
	}
	if _, err := New("g-1", Options{Validator: testValidator, Limits: Limits{DescriptorBytes: -1}}); !errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("negative limit was accepted: %v", err)
	}
	if _, err := New("g-1", Options{}); !errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("registry without validator was accepted: %v", err)
	}
	if _, err := New("", Options{Validator: testValidator}); !errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("empty generation was accepted: %v", err)
	}
}

func TestValidationRunsBeforeTheRegistryLock(t *testing.T) {
	var registry *Registry
	created, err := New("g-1", Options{Validator: func(kind Kind, key string, descriptor json.RawMessage) (Claims, error) {
		if listed := registry.List(); len(listed) != 0 {
			t.Errorf("validator observed %d registrations", len(listed))
		}
		return testValidator(kind, key, descriptor)
	}})
	if err != nil {
		t.Fatal(err)
	}
	registry = created
	registered := make(chan error, 1)
	go func() {
		_, err := registry.Register(testIdentity("i-1"), KindDocs, "manual", json.RawMessage(`{"label":"manual"}`))
		registered <- err
	}()
	select {
	case err := <-registered:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("descriptor validation ran under the registry lock")
	}
}

func TestValidatorRefusalAdmitsNothing(t *testing.T) {
	registry := newTestRegistry(t)
	_, err := registry.Register(testIdentity("i-1"), KindDocs, "manual", json.RawMessage(`{"label":"manual","schemes":["manual"],"reject":true}`))
	if !errors.Is(err, ErrInvalidDescriptor) {
		t.Fatalf("validator refusal not reported: %v", err)
	}
	if !strings.Contains(err.Error(), "host schema") {
		t.Fatalf("validator cause is lost: %v", err)
	}
	if listed := registry.List(); len(listed) != 0 {
		t.Fatalf("refused registration was stored: %d", len(listed))
	}
	for _, descriptor := range []string{``, `null`, `[]`, `{"label":"a"} {"label":"b"}`, `{"label":"a"`} {
		if _, err := registry.Register(testIdentity("i-1"), KindDocs, "manual", json.RawMessage(descriptor)); !errors.Is(err, ErrInvalidDescriptor) {
			t.Fatalf("malformed descriptor %q was accepted: %v", descriptor, err)
		}
	}
	if _, err := registry.Register(testIdentity("i-1"), Kind("nonsense"), "manual", json.RawMessage(`{"label":"a"}`)); !errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("unknown kind was accepted: %v", err)
	}
	if _, err := registry.Register(api.Identity{Owner: "build-tools"}, KindDocs, "manual", json.RawMessage(`{"label":"a"}`)); !errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("identity without instance was accepted: %v", err)
	}
}

func TestGenerationBindingRefusesStaleHandles(t *testing.T) {
	first := newTestRegistry(t)
	identity := testIdentity("i-1")
	receipt := mustRegister(t, first, identity, KindDocs, "manual", `{"label":"manual"}`)
	second, err := New("g-2", Options{Validator: testValidator})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := second.Get(identity, receipt.RegistrationID); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("previous generation handle was readable: %v", err)
	}
	if err := second.Unregister(identity, receipt.RegistrationID); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("previous generation handle was mutable: %v", err)
	}
	unknown := receipt.RegistrationID[:len(receipt.RegistrationID)-1]
	if strings.HasSuffix(receipt.RegistrationID, "0") {
		unknown += "1"
	} else {
		unknown += "0"
	}
	if _, err := first.Get(identity, unknown); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown handle of the current generation is misclassified: %v", err)
	}
	if _, err := first.Get(identity, "not-a-handle"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("malformed handle is misclassified: %v", err)
	}
}

func TestSnapshotsAreDeepCopiedAndOrdered(t *testing.T) {
	registry := newTestRegistry(t)
	a := api.Identity{Owner: "pig-web", Instance: "w-1"}
	b := testIdentity("i-1")
	mustRegister(t, registry, a, KindResourceProvider, "kb", `{"label":"kb"}`)
	mustRegister(t, registry, b, KindDocs, "manual", `{"label":"manual"}`)
	mustRegister(t, registry, b, KindDocs, "api", `{"label":"api"}`)

	descriptor := json.RawMessage(`{"label":"caller"}`)
	receipt, err := registry.Register(b, KindStatusSegment, "status", descriptor)
	if err != nil {
		t.Fatal(err)
	}
	descriptor[2] = 'X'

	current, err := registry.Get(b, receipt.RegistrationID)
	if err != nil {
		t.Fatal(err)
	}
	if string(current.Descriptor) != `{"label":"caller"}` {
		t.Fatalf("caller mutation changed the stored descriptor: %s", current.Descriptor)
	}
	current.Descriptor[2] = 'Y'

	listed := registry.List()
	order := make([]string, 0, len(listed))
	for _, registration := range listed {
		order = append(order, registration.Owner+"/"+string(registration.Kind)+"/"+registration.Key)
	}
	want := []string{
		"build-tools/docs/api", "build-tools/docs/manual", "build-tools/statusSegment/status", "pig-web/resourceProvider/kb",
	}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("snapshot order %v, want %v", order, want)
	}
	listed[0].Descriptor[2] = 'Z'
	again, err := registry.Get(b, receipt.RegistrationID)
	if err != nil {
		t.Fatal(err)
	}
	if string(again.Descriptor) != `{"label":"caller"}` {
		t.Fatalf("snapshot mutation changed the stored descriptor: %s", again.Descriptor)
	}
}

func TestUnregisterIsIdempotentAndOwnerScoped(t *testing.T) {
	registry := newTestRegistry(t)
	identity := testIdentity("i-1")
	receipt := mustRegister(t, registry, identity, KindDocs, "manual", `{"label":"manual"}`)
	other := api.Identity{Owner: "pig-web", Instance: "w-1"}
	if err := registry.Unregister(other, receipt.RegistrationID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign owner unregistered the registration: %v", err)
	}
	if _, err := registry.Get(other, receipt.RegistrationID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("foreign owner read the registration: %v", err)
	}
	if err := registry.Unregister(identity, receipt.RegistrationID); err != nil {
		t.Fatal(err)
	}
	if err := registry.Unregister(identity, receipt.RegistrationID); err != nil {
		t.Fatalf("repeated unregister was not idempotent: %v", err)
	}
	unknown := receipt.RegistrationID[:len(receipt.RegistrationID)-1]
	if strings.HasSuffix(receipt.RegistrationID, "0") {
		unknown += "1"
	} else {
		unknown += "0"
	}
	if err := registry.Unregister(identity, unknown); err != nil {
		t.Fatalf("unknown unregister was not idempotent: %v", err)
	}
	if listed := registry.List(); len(listed) != 0 {
		t.Fatalf("unregister left registrations behind: %d", len(listed))
	}
}

func TestConcurrentRegisterListUnregister(t *testing.T) {
	registry := newTestRegistry(t)
	const owners, keys, rounds = 4, 8, 20
	var group sync.WaitGroup
	for owner := range owners {
		for key := range keys {
			group.Add(1)
			go func(owner, key int) {
				defer group.Done()
				identity := api.Identity{Owner: fmt.Sprintf("owner-%d", owner), Instance: "i-1"}
				name := fmt.Sprintf("key-%d", key)
				descriptor := json.RawMessage(fmt.Sprintf(`{"label":%q,"schemes":["s%d%d"]}`, name, owner, key))
				for range rounds {
					receipt, err := registry.Register(identity, KindResourceProvider, name, descriptor)
					if err != nil {
						t.Errorf("register %s: %v", name, err)
						return
					}
					if _, err := registry.Get(identity, receipt.RegistrationID); err != nil {
						t.Errorf("get %s: %v", name, err)
						return
					}
					registry.List()
					if err := registry.Unregister(identity, receipt.RegistrationID); err != nil {
						t.Errorf("unregister %s: %v", name, err)
						return
					}
				}
				if _, err := registry.Register(identity, KindResourceProvider, name, descriptor); err != nil {
					t.Errorf("final register %s: %v", name, err)
				}
			}(owner, key)
		}
	}
	group.Wait()
	if listed := registry.List(); len(listed) != owners*keys {
		t.Fatalf("concurrent churn left %d registrations, want %d", len(listed), owners*keys)
	}
}

func TestRetiredOwnerStaysInactiveUntilExplicitActivation(t *testing.T) {
	registry := newTestRegistry(t)
	const cycles = 10 // more than the retained instance tombstones
	oldest := testIdentity("instance-0")
	receipt := mustRegister(t, registry, oldest, KindDocs, "manual", `{"label":"manual"}`)
	previous := oldest.Instance
	for cycle := 1; cycle <= cycles; cycle++ {
		next := fmt.Sprintf("instance-%d", cycle)
		if _, err := registry.ReplaceOwner("build-tools", previous, next); err != nil {
			t.Fatalf("replacement %d: %v", cycle, err)
		}
		mustRegister(t, registry, testIdentity(next), KindDocs, "manual", `{"label":"manual"}`)
		previous = next
	}
	if _, err := registry.RetireOwner(testIdentity(previous)); err != nil {
		t.Fatal(err)
	}
	if listed := registry.List(); len(listed) != 0 {
		t.Fatalf("retirement left %d registrations", len(listed))
	}

	// The oldest incarnation fell out of the bounded tombstones; replaying it must still fail
	// closed instead of reactivating the retired owner.
	for _, identity := range []api.Identity{oldest, testIdentity(previous)} {
		_, err := registry.Register(identity, KindDocs, "manual", json.RawMessage(`{"label":"manual"}`))
		if identity == oldest {
			if !errors.Is(err, ErrInstanceConflict) {
				t.Fatalf("forgotten incarnation was admitted: %v", err)
			}
		} else if !errors.Is(err, ErrStaleInstance) {
			t.Fatalf("retired incarnation was admitted: %v", err)
		}
		if _, err := registry.Get(identity, receipt.RegistrationID); err == nil {
			t.Fatalf("inactive owner answered %q", identity.Instance)
		}
		if err := registry.Unregister(identity, receipt.RegistrationID); err == nil {
			t.Fatalf("inactive owner accepted an unregister from %q", identity.Instance)
		}
	}

	// Only an owner that was never seen may establish itself implicitly.
	established := mustRegister(t, registry, api.Identity{Owner: "brand-new", Instance: "n-1"}, KindDocs, "manual", `{"label":"manual"}`)
	if established.RegistrationID == "" {
		t.Fatal("new owner did not establish an incarnation")
	}
	if _, err := registry.ReplaceOwner("brand-new", "", "n-2"); !errors.Is(err, ErrInstanceConflict) {
		t.Fatalf("activation bypassed the named incumbent: %v", err)
	}
	if _, err := registry.ReplaceOwner("build-tools", "instance-fresh", "instance-11"); !errors.Is(err, ErrStaleInstance) {
		t.Fatalf("inactive owner was replaced through a stale incumbent: %v", err)
	}

	withdrawal, err := registry.ReplaceOwner("build-tools", "", "instance-fresh")
	if err != nil || len(withdrawal.Registrations) != 0 || withdrawal.Instance != "" {
		t.Fatalf("activation of an inactive owner failed: %+v %v", withdrawal, err)
	}
	mustRegister(t, registry, testIdentity("instance-fresh"), KindDocs, "manual", `{"label":"manual"}`)
	if again, err := registry.ReplaceOwner("build-tools", "", "instance-fresh"); err != nil || len(again.Registrations) != 0 {
		t.Fatalf("repeated activation was not a no-op: %+v %v", again, err)
	}
	if _, err := registry.Register(oldest, KindDocs, "manual-2", json.RawMessage(`{"label":"manual"}`)); !errors.Is(err, ErrInstanceConflict) {
		t.Fatalf("forgotten incarnation acted on an active owner: %v", err)
	}

	// A tombstoned incarnation cannot be activated again and retired owners never revive themselves.
	if _, err := registry.RetireOwner(testIdentity("instance-fresh")); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ReplaceOwner("build-tools", "", previous); !errors.Is(err, ErrStaleInstance) {
		t.Fatalf("tombstoned incarnation was activated: %v", err)
	}
	if _, err := registry.Register(testIdentity(previous), KindDocs, "manual", json.RawMessage(`{"label":"manual"}`)); !errors.Is(err, ErrInstanceConflict) && !errors.Is(err, ErrStaleInstance) {
		t.Fatalf("retired owner revived itself: %v", err)
	}
}
