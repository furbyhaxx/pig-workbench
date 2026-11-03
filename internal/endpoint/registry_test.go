package endpoint

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func registryForTest(t *testing.T) (*Registry, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "registry")
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r, dir
}

func recordForTest(id, token string, pid int) Record {
	return Record{
		SchemaVersion: 1, SessionID: id, Token: token, PID: pid,
		StartedAt:   time.Now().Add(-24 * time.Hour).UTC(),
		HeartbeatAt: time.Now().Add(-time.Hour).UTC(),
		Cwd:         "/project", ProjectID: "project", State: "idle", Discoverable: true,
	}
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestOwnershipAndMalformedList(t *testing.T) {
	ctx := context.Background()
	first, dir := registryForTest(t)
	second, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	original := recordForTest("session-1", "token-1", os.Getpid())
	competitor := original
	competitor.Token = "token-2"
	if err := first.Register(ctx, original); err != nil {
		t.Fatal(err)
	}
	if err := second.Register(ctx, competitor); !errors.Is(err, ErrSessionExists) {
		t.Fatalf("live registration was not protected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "corrupt.json"), []byte(`{"token":"do-not-report"`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := second.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Records) != 1 || result.Records[0].SessionID != original.SessionID {
		t.Fatalf("valid record not listed: %+v", result.Records)
	}
	if len(result.Problems) != 1 || result.Problems[0].File != "corrupt.json" || strings.Contains(result.Problems[0].Reason, "do-not-report") {
		t.Fatalf("malformed record not safely reported: %+v", result.Problems)
	}
	if err := first.Unregister(ctx, original.SessionID, "wrong"); !errors.Is(err, ErrTokenMismatch) {
		t.Fatalf("wrong unregister token: %v", err)
	}
	if err := first.Unregister(ctx, original.SessionID, original.Token); err != nil {
		t.Fatal(err)
	}
	if err := first.Unregister(ctx, original.SessionID, original.Token); err != nil {
		t.Fatalf("unregister should be idempotent: %v", err)
	}
	if err := second.Register(ctx, competitor); err != nil {
		t.Fatalf("replacement after unregister: %v", err)
	}
	got, err := first.Lookup(ctx, original.SessionID)
	if err != nil || got.Token != competitor.Token {
		t.Fatalf("replacement not visible: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".session-1.json.") {
			t.Fatalf("temporary file remained: %s", entry.Name())
		}
	}
}

func TestHeartbeatAndWireFormat(t *testing.T) {
	ctx := context.Background()
	r, dir := registryForTest(t)
	record := recordForTest("heartbeat", "token", os.Getpid())
	if err := r.Register(ctx, record); err != nil {
		t.Fatal(err)
	}
	if err := r.Heartbeat(ctx, record.SessionID, "wrong"); !errors.Is(err, ErrTokenMismatch) {
		t.Fatalf("wrong heartbeat token: %v", err)
	}
	before, err := r.Lookup(ctx, record.SessionID)
	if err != nil || !before.HeartbeatAt.Equal(record.HeartbeatAt) {
		t.Fatalf("wrong token changed record: %v", err)
	}
	if err := r.Heartbeat(ctx, record.SessionID, record.Token); err != nil {
		t.Fatal(err)
	}
	after, err := r.Lookup(ctx, record.SessionID)
	if err != nil || !after.HeartbeatAt.After(before.HeartbeatAt) {
		t.Fatalf("heartbeat did not advance: %v", err)
	}
	after.HeartbeatAt = before.HeartbeatAt
	if !reflect.DeepEqual(before, after) {
		t.Fatal("heartbeat changed other fields")
	}
	data, err := os.ReadFile(filepath.Join(dir, record.SessionID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schemaVersion", "sessionId", "projectId", "startedAt", "heartbeatAt"} {
		if _, ok := fields[key]; !ok {
			t.Errorf("missing JSON field %s", key)
		}
	}
	if len(fields) != 10 {
		t.Fatalf("unexpected record fields: %d", len(fields))
	}
}

func TestStaleAndPruneThreshold(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PID probing is unsupported on Windows")
	}
	ctx := context.Background()
	r, dir := registryForTest(t)
	old := recordForTest("dead-old", "token", 4194304)
	old.HeartbeatAt = time.Now().Add(-2 * time.Hour).UTC()
	recent := recordForTest("dead-recent", "token", 4194304)
	recent.HeartbeatAt = time.Now().Add(-time.Minute).UTC()
	live := recordForTest("live-old", "token", os.Getpid())
	for _, record := range []Record{old, recent, live} {
		if err := r.Register(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	result, err := r.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Problems) != 0 || !hasString(result.Stale, old.SessionID) || !hasString(result.Stale, recent.SessionID) || hasString(result.Stale, live.SessionID) {
		t.Fatalf("incorrect liveness: %+v", result)
	}
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("not JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	removed, err := r.Prune(ctx, time.Hour)
	var problems *ProblemsError
	if !errors.As(err, &problems) || len(problems.Problems) != 1 || problems.Problems[0].File != "bad.json" {
		t.Fatalf("prune must report malformed records: %v", err)
	}
	if len(removed) != 1 || removed[0] != old.SessionID {
		t.Fatalf("prune ignored threshold: %v", removed)
	}
	if _, err := os.Stat(filepath.Join(dir, "bad.json")); err != nil {
		t.Fatalf("prune removed malformed record: %v", err)
	}
	if _, err := r.Lookup(ctx, recent.SessionID); err != nil {
		t.Fatalf("prune removed recent record: %v", err)
	}
	if _, err := r.Lookup(ctx, live.SessionID); err != nil {
		t.Fatalf("prune removed live record: %v", err)
	}
	removed, _ = r.Prune(ctx, 0)
	if hasString(removed, live.SessionID) {
		t.Fatal("zero threshold removed a live record")
	}
	if _, err := r.Lookup(ctx, live.SessionID); err != nil {
		t.Fatalf("zero threshold removed a live record: %v", err)
	}
}

func TestStaleRegistrationCanBeReplaced(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PID probing is unsupported on Windows")
	}
	ctx := context.Background()
	r, _ := registryForTest(t)
	dead := recordForTest("stale", "old", 4194304)
	if err := r.Register(ctx, dead); err != nil {
		t.Fatal(err)
	}
	live := recordForTest(dead.SessionID, "new", os.Getpid())
	if err := r.Register(ctx, live); err != nil {
		t.Fatalf("stale registration could not be replaced: %v", err)
	}
	got, err := r.Lookup(ctx, dead.SessionID)
	if err != nil || got.PID != live.PID || got.Token != live.Token {
		t.Fatalf("replacement missing: %v", err)
	}
}

func TestBoundsInvalidPathsAndCancellation(t *testing.T) {
	ctx := context.Background()
	r, dir := registryForTest(t)
	for _, id := range []string{"", ".", "..", "../outside", `a\b`, "a/b"} {
		record := recordForTest(id, "token", os.Getpid())
		if err := r.Register(ctx, record); err == nil {
			t.Errorf("invalid ID %q accepted", id)
		}
	}
	large := recordForTest("large", "token", os.Getpid())
	large.Cwd = strings.Repeat("x", 65536)
	if err := r.Register(ctx, large); err == nil {
		t.Fatal("oversized record accepted")
	}
	if _, err := os.Stat(filepath.Join(dir, "large.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oversized record was written: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "huge.json"), []byte(strings.Repeat("x", 65537)), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := r.List(ctx)
	if err != nil || len(result.Problems) != 1 || result.Problems[0].File != "huge.json" {
		t.Fatalf("oversized record not reported: %+v, %v", result.Problems, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := r.List(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled list: %v", err)
	}
	if _, err := r.Lookup(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing record: %v", err)
	}
}

func TestConcurrentRegistries(t *testing.T) {
	ctx := context.Background()
	a, dir := registryForTest(t)
	b, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	base := recordForTest("shared", "owner", os.Getpid())
	if err := a.Register(ctx, base); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for i := range 12 {
		wg.Go(func() {
			var err error
			switch i % 3 {
			case 0:
				other := base
				other.Token = "competitor"
				err = b.Register(ctx, other)
				if errors.Is(err, ErrSessionExists) {
					err = nil
				} else if err == nil {
					err = errors.New("competing owner registered")
				}
			case 1:
				err = a.Heartbeat(ctx, base.SessionID, base.Token)
			case 2:
				var result ListResult
				result, err = b.List(ctx)
				if err == nil && (len(result.Records) != 1 || len(result.Problems) != 0 || len(result.Stale) != 0) {
					err = errors.New("list observed inconsistent registry")
				}
			}
			if err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
