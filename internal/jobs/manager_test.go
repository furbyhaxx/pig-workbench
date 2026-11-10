package jobs

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func managerTestUnix(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("Unix process groups required")
	}
}

func newTestManager(t *testing.T, options ManagerOptions) *Manager {
	t.Helper()
	manager, err := NewManager(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Errorf("close manager: %v", err)
		}
	})
	return manager
}

func managerWait(t *testing.T, manager *Manager, id string) JobState {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	state, err := manager.Wait(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func TestManagerCancelCloseConcurrent(t *testing.T) {
	managerTestUnix(t)
	baseline := runtime.NumGoroutine()
	manager := newTestManager(t, ManagerOptions{Grace: 100 * time.Millisecond})
	id, err := manager.Start(context.Background(), Spec{Command: []string{"cat"}, Stdin: true})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() { <-start; results <- manager.Cancel(id) }()
	go func() { <-start; results <- manager.Close() }()
	close(start)
	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("concurrent cancel and close did not finish")
		}
	}
	state := managerWait(t, manager, id)
	status, err := manager.Status(id)
	if err != nil || state.Running || !state.Cancelled || state.Exit == nil || status.Running || !status.Cancelled || status.Exit == nil {
		t.Fatalf("state=%+v status=%+v err=%v", state, status, err)
	}
	if _, err := manager.Start(context.Background(), Spec{Command: []string{"true"}}); !errors.Is(err, ErrManagerClosed) {
		t.Fatalf("start after close = %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > baseline+2 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if got := runtime.NumGoroutine(); got > baseline+2 {
		t.Fatalf("goroutines before=%d after=%d", baseline, got)
	}
}

func TestManagerNormalExitAndPrivateOutput(t *testing.T) {
	managerTestUnix(t)
	manager := newTestManager(t, ManagerOptions{})
	var callerOutput bytes.Buffer
	id, err := manager.Start(context.Background(), Spec{Command: []string{"printf hello; printf world >&2; exit 7"}, Shell: true, Output: &callerOutput})
	if err != nil {
		t.Fatal(err)
	}
	state := managerWait(t, manager, id)
	if state.ID != id || state.Running || state.Cancelled || state.Exit == nil || state.Exit.Code != 7 || state.Error != "" {
		t.Fatalf("state = %+v", state)
	}
	data, next, truncated, gap, err := manager.Output(id, 0, 0)
	// SPEC-0008 §6: order across stdout and stderr is observed ingestion order, so only each stream is exact.
	if err != nil || (string(data) != "helloworld" && string(data) != "worldhello") || next != 10 || truncated || gap != 0 {
		t.Fatalf("output=%q next=%d truncated=%v gap=%d err=%v", data, next, truncated, gap, err)
	}
	for stream, want := range map[Stream]string{StreamStdout: "hello", StreamStderr: "world"} {
		got, _, _, _, err := manager.Stream(id, stream, 0, 0)
		if err != nil || string(got) != want {
			t.Fatalf("stream %d = %q, %v; want %q", stream, got, err, want)
		}
	}
	if callerOutput.Len() != 0 {
		t.Fatalf("caller output received %q", callerOutput.String())
	}
	state.Exit.Code = 0
	again, err := manager.Status(id)
	if err != nil || again.Exit == nil || again.Exit.Code != 7 {
		t.Fatalf("state was not copied: %+v, %v", again, err)
	}
}

func TestManagerOutputUsesLogQuota(t *testing.T) {
	managerTestUnix(t)
	manager := newTestManager(t, ManagerOptions{LogBytes: 4, SessionLogBytes: 8})
	id, err := manager.Start(context.Background(), Spec{Command: []string{"printf 0123456789"}, Shell: true})
	if err != nil {
		t.Fatal(err)
	}
	managerWait(t, manager, id)
	data, next, truncated, gap, err := manager.Output(id, 0, 0)
	if err != nil || string(data) != "6789" || next != 10 || !truncated || gap != 6 {
		t.Fatalf("output=%q next=%d truncated=%v gap=%d err=%v", data, next, truncated, gap, err)
	}
}

func TestManagerWaitAndAdmissionCancellation(t *testing.T) {
	managerTestUnix(t)
	manager := newTestManager(t, ManagerOptions{Grace: 10 * time.Millisecond})
	admission, cancelAdmission := context.WithCancel(context.Background())
	id, err := manager.Start(admission, Spec{Command: []string{"cat"}, Stdin: true})
	if err != nil {
		t.Fatal(err)
	}
	cancelAdmission()
	wait, cancelWait := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelWait()
	if _, err := manager.Wait(wait, id); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait error = %v", err)
	}
	state, err := manager.Status(id)
	if err != nil || !state.Running {
		t.Fatalf("job stopped after cancellation: %+v %v", state, err)
	}
	if err := manager.Cancel(id); err != nil {
		t.Fatal(err)
	}
	alreadyCanceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.Start(alreadyCanceled, Spec{Command: []string{"true"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admission = %v", err)
	}
}

func TestManagerRunningLimitReleased(t *testing.T) {
	managerTestUnix(t)
	manager := newTestManager(t, ManagerOptions{MaxActive: 1, Grace: 10 * time.Millisecond})
	id, err := manager.Start(context.Background(), Spec{Command: []string{"cat"}, Stdin: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(context.Background(), Spec{Command: []string{"true"}}); !errors.Is(err, ErrJobLimit) {
		t.Fatalf("second admission = %v", err)
	}
	if err := manager.Cancel(id); err != nil {
		t.Fatal(err)
	}
	next, err := manager.Start(context.Background(), Spec{Command: []string{"true"}})
	if err != nil {
		t.Fatal(err)
	}
	state := managerWait(t, manager, next)
	if state.Running || state.Exit == nil || state.Exit.Code != 0 || id == next {
		t.Fatalf("next job = %+v", state)
	}
	last, err := manager.Start(context.Background(), Spec{Command: []string{"true"}})
	if err != nil {
		t.Fatalf("natural exit did not release capacity: %v", err)
	}
	if last == id || last == next {
		t.Fatalf("job ID reused: %q", last)
	}
	managerWait(t, manager, last)
}

func TestManagerCancellationCanExitZero(t *testing.T) {
	managerTestUnix(t)
	manager := newTestManager(t, ManagerOptions{Grace: 100 * time.Millisecond})
	id, err := manager.Start(context.Background(), Spec{Command: []string{"trap '' TERM; printf 'ready\\n'; IFS= read -r line || exit 0"}, Shell: true, Stdin: true})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, _, _, _, err := manager.Output(id, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) == "ready\n" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not reach stdin barrier")
		}
		runtime.Gosched()
	}
	if err := manager.Cancel(id); err != nil {
		t.Fatal(err)
	}
	state := managerWait(t, manager, id)
	if state.Running || !state.Cancelled || state.Exit == nil || state.Exit.Code != 0 || state.Exit.Signaled {
		t.Fatalf("cancelled zero exit = %+v", state)
	}
	if err := manager.Cancel(id); err != nil {
		t.Fatal(err)
	}
}

func TestManagerNaturalExitWinsBeforeCancelAndClose(t *testing.T) {
	managerTestUnix(t)
	manager := newTestManager(t, ManagerOptions{Grace: 100 * time.Millisecond})
	id, err := manager.Start(context.Background(), Spec{Command: []string{"true"}})
	if err != nil {
		t.Fatal(err)
	}
	job, err := manager.job(id)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-job.process.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("natural exit barrier not reached")
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() { <-start; results <- manager.Cancel(id) }()
	go func() { <-start; results <- manager.Close() }()
	close(start)
	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("natural-exit cleanup did not finish")
		}
	}
	state := managerWait(t, manager, id)
	if state.Running || state.Cancelled || state.Exit == nil || state.Exit.Code != 0 || state.Exit.Signaled {
		t.Fatalf("natural exit = %+v", state)
	}
}

func TestManagerNaturalExitReapsDescendant(t *testing.T) {
	managerTestUnix(t)
	manager := newTestManager(t, ManagerOptions{})
	id, err := manager.Start(context.Background(), Spec{Command: []string{"sleep 30 & echo $!; exit 0"}, Shell: true})
	if err != nil {
		t.Fatal(err)
	}
	state := managerWait(t, manager, id)
	if state.Running || state.Cancelled || state.Exit == nil || state.Exit.Code != 0 || state.Exit.Signaled || state.Error != "" {
		t.Fatalf("natural leader exit = %+v", state)
	}
	data, _, _, _, err := manager.Output(id, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("descendant pid %q: %v", data, err)
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	defer process.Release()
	if err := process.Signal(syscall.Signal(0)); !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("descendant %d still exists: %v", pid, err)
	}
}

func TestManagerNaturalExitCancelCloseRace(t *testing.T) {
	managerTestUnix(t)
	for i := 0; i < 8; i++ {
		manager, err := NewManager(ManagerOptions{Grace: 10 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = manager.Close() })
		dir := t.TempDir()
		pidFile := filepath.Join(dir, "child")
		releaseFile := filepath.Join(dir, "release")
		spec := Spec{Command: []string{"sleep 30 & echo $! > \"$1\"; while [ ! -f \"$2\" ]; do :; done; exit 0", pidFile, releaseFile}, Shell: true}
		id, err := manager.Start(context.Background(), spec)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		for {
			if _, err := os.Stat(pidFile); err == nil {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("child did not start")
			}
			runtime.Gosched()
		}
		start := make(chan struct{})
		results := make(chan error, 3)
		go func() { <-start; results <- manager.Cancel(id) }()
		go func() { <-start; results <- manager.Close() }()
		go func() { <-start; results <- os.WriteFile(releaseFile, nil, 0600) }()
		close(start)
		for range 3 {
			select {
			case err := <-results:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("race did not finish")
			}
		}
		state := managerWait(t, manager, id)
		if state.Running || state.Exit == nil {
			t.Fatalf("race left job running: %+v", state)
		}
	}
}

func TestManagerStartCloseRace(t *testing.T) {
	managerTestUnix(t)
	manager := newTestManager(t, ManagerOptions{Grace: time.Millisecond, MaxActive: 32})
	const count = 20
	start := make(chan struct{})
	type startResult struct {
		id  string
		err error
	}
	results := make(chan startResult, count)
	var wg sync.WaitGroup
	for range count {
		wg.Go(func() {
			<-start
			id, err := manager.Start(context.Background(), Spec{Command: []string{"sleep", "30"}})
			results <- startResult{id, err}
		})
	}
	close(start)
	deadline := time.Now().Add(5 * time.Second)
	for {
		manager.mu.Lock()
		active := manager.active
		manager.mu.Unlock()
		if active > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no admission started")
		}
		runtime.Gosched()
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	close(results)
	seen := make(map[string]bool)
	for result := range results {
		if result.err != nil {
			if !errors.Is(result.err, ErrManagerClosed) {
				t.Fatalf("start: %v", result.err)
			}
			continue
		}
		if result.id == "" || seen[result.id] {
			t.Fatalf("invalid or reused job ID %q", result.id)
		}
		seen[result.id] = true
		if state := managerWait(t, manager, result.id); state.Running || state.Exit == nil {
			t.Fatalf("job not joined: %+v", state)
		}
	}
}

func TestManagerPreservesOutputFailure(t *testing.T) {
	managerTestUnix(t)
	manager, err := NewManager(ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	release := filepath.Join(t.TempDir(), "release")
	id, err := manager.Start(context.Background(), Spec{Command: []string{"while [ ! -f \"$1\" ]; do :; done; printf hello", release}, Shell: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.logs.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(release, nil, 0600); err != nil {
		t.Fatal(err)
	}
	state := managerWait(t, manager, id)
	if state.Running || state.Exit == nil || state.Exit.Code != 0 || !strings.Contains(state.Error, ErrLogClosed.Error()) {
		t.Fatalf("output failure state = %+v", state)
	}
	if err := manager.Cancel(id); !errors.Is(err, ErrLogClosed) {
		t.Fatalf("cancel hid output failure: %v", err)
	}
	if err := manager.Close(); !errors.Is(err, ErrLogClosed) {
		t.Fatalf("close hid output failure: %v", err)
	}
}

func TestManagerValidationAndUnknownID(t *testing.T) {
	for _, options := range []ManagerOptions{{LogBytes: -1}, {SessionLogBytes: -1}, {Grace: -1}, {MaxActive: -1}} {
		if _, err := NewManager(options); err == nil {
			t.Fatalf("accepted options %+v", options)
		}
	}
	manager := newTestManager(t, ManagerOptions{})
	if _, err := manager.Status("missing"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("unknown status = %v", err)
	} else {
		var notFound *JobNotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("unknown status has wrong error type: %v", err)
		}
	}
	if _, err := manager.Wait(context.Background(), "missing"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("unknown wait = %v", err)
	}
	if err := manager.Cancel("missing"); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("unknown cancel = %v", err)
	}
	if _, _, _, _, err := manager.Output("missing", 0, 0); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("unknown output = %v", err)
	}
}

func TestManagerRetiresOldestFinishedBeyondCap(t *testing.T) {
	managerTestUnix(t)
	manager := newTestManager(t, ManagerOptions{MaxFinished: 2, Grace: 100 * time.Millisecond})
	active, err := manager.Start(context.Background(), Spec{Command: []string{"cat"}, Stdin: true})
	if err != nil {
		t.Fatal(err)
	}
	finished := make([]string, 0, 4)
	for range 4 {
		id, err := manager.Start(context.Background(), Spec{Command: []string{"printf", "x"}})
		if err != nil {
			t.Fatal(err)
		}
		managerWait(t, manager, id)
		finished = append(finished, id)
	}
	listed := manager.List()
	if len(listed) != 3 || manager.Retired() != 2 {
		t.Fatalf("retained states = %+v, retired = %d", listed, manager.Retired())
	}
	if listed[0].ID != active || !listed[0].Running {
		t.Fatalf("running job not retained first: %+v", listed)
	}
	if listed[1].ID != finished[2] || listed[2].ID != finished[3] || listed[2].Exit == nil {
		t.Fatalf("retained finished states = %+v", listed)
	}
	for _, id := range finished[:2] {
		_, err := manager.Status(id)
		if !errors.Is(err, ErrJobRetired) || !errors.Is(err, ErrJobNotFound) {
			t.Fatalf("status of retired job = %v", err)
		}
		var retired *JobRetiredError
		if !errors.As(err, &retired) {
			t.Fatalf("retired status has wrong error type: %v", err)
		}
		if _, err := manager.Wait(context.Background(), id); !errors.Is(err, ErrJobRetired) {
			t.Fatalf("wait of retired job = %v", err)
		}
		if err := manager.Cancel(id); !errors.Is(err, ErrJobRetired) {
			t.Fatalf("cancel of retired job = %v", err)
		}
		if _, _, _, _, err := manager.Output(id, 0, 0); !errors.Is(err, ErrJobRetired) {
			t.Fatalf("output of retired job = %v", err)
		}
		if _, _, _, err := manager.logs.Facts(id); !errors.Is(err, ErrLogNotFound) {
			t.Fatalf("log metadata survived retirement: %v", err)
		}
	}
	if written, _, _, err := manager.logs.Facts(finished[3]); err != nil || written != 1 {
		t.Fatalf("retained log written=%d err=%v", written, err)
	}
	if _, err := manager.Status("missing"); !errors.Is(err, ErrJobNotFound) || errors.Is(err, ErrJobRetired) {
		t.Fatalf("unknown job = %v", err)
	}
	if err := manager.Cancel(active); err != nil {
		t.Fatal(err)
	}
}

func TestManagerFinishedRetentionDefaultsAndValidation(t *testing.T) {
	manager := newTestManager(t, ManagerOptions{})
	if manager.maxFinished != defaultMaxFinished {
		t.Fatalf("default retention = %d", manager.maxFinished)
	}
	if _, err := NewManager(ManagerOptions{MaxFinished: -1}); err == nil {
		t.Fatal("accepted a negative finished-job cap")
	}
}

func TestManagerWaitKeepsTerminalStateThroughRetirement(t *testing.T) {
	managerTestUnix(t)
	manager, err := NewManager(ManagerOptions{MaxFinished: 1, Grace: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	release := filepath.Join(t.TempDir(), "release")
	id, err := manager.Start(context.Background(), Spec{Command: []string{"while [ ! -f \"$1\" ]; do sleep 0.01; done; printf waited", release}, Shell: true})
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		state JobState
		err   error
	}
	waiter := make(chan outcome, 1)
	go func() {
		state, err := manager.Wait(context.Background(), id)
		waiter <- outcome{state, err}
	}()
	time.Sleep(10 * time.Millisecond)
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	managerWait(t, manager, id)
	for range 2 {
		next, err := manager.Start(context.Background(), Spec{Command: []string{"true"}})
		if err != nil {
			t.Fatal(err)
		}
		managerWait(t, manager, next)
	}
	if _, err := manager.Status(id); !errors.Is(err, ErrJobRetired) {
		t.Fatalf("waited job was not retired: %v", err)
	}
	result := <-waiter
	if result.err != nil || result.state.ID != id || result.state.Running || result.state.Exit == nil || result.state.Exit.Code != 0 {
		t.Fatalf("attached wait lost its result: %+v %v", result.state, result.err)
	}
}

func TestManagerRetentionConcurrentAccess(t *testing.T) {
	managerTestUnix(t)
	manager := newTestManager(t, ManagerOptions{MaxFinished: 2, MaxActive: 8, Grace: 50 * time.Millisecond})
	stop := make(chan struct{})
	var readers sync.WaitGroup
	var churn sync.WaitGroup
	stopped := false
	stopAll := func() {
		if stopped {
			return
		}
		stopped = true
		close(stop)
		readers.Wait()
		churn.Wait()
	}
	defer stopAll()
	for range 4 {
		readers.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				states := manager.List()
				if len(states) > 8+2 {
					t.Errorf("retained states grew to %d", len(states))
					return
				}
				for _, state := range states {
					_, err := manager.Status(state.ID)
					if err != nil && !errors.Is(err, ErrJobNotFound) && !errors.Is(err, ErrJobRetired) {
						t.Errorf("status %s: %v", state.ID, err)
						return
					}
					_, _, _, _, err = manager.Output(state.ID, 0, 0)
					if err != nil && !errors.Is(err, ErrJobNotFound) && !errors.Is(err, ErrJobRetired) {
						t.Errorf("output %s: %v", state.ID, err)
						return
					}
				}
			}
		})
	}
	for range 2 {
		churn.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				id, err := manager.Start(context.Background(), Spec{Command: []string{"cat"}, Stdin: true})
				if err != nil {
					if errors.Is(err, ErrJobLimit) {
						runtime.Gosched()
						continue
					}
					t.Errorf("start: %v", err)
					return
				}
				_ = manager.Cancel(id)
			}
		})
	}
	for range 30 {
		id, err := manager.Start(context.Background(), Spec{Command: []string{"printf", "retained"}})
		if err != nil {
			t.Fatalf("start: %v", err)
		}
		managerWait(t, manager, id)
	}
	stopAll()
	states := manager.List()
	if len(states) == 0 || len(states) > 2 {
		t.Fatalf("retained states after churn = %+v", states)
	}
	if entries := len(manager.logs.index.Load().entries); entries > 2 {
		t.Fatalf("log metadata grew to %d entries", entries)
	}
	for _, state := range states {
		if state.Running {
			continue
		}
		if state.Exit == nil {
			t.Fatalf("retained terminal state lost its exit: %+v", state)
		}
	}
}

// TestManagerForegroundCommandsDoNotRetireBackgroundJobs pins the separate retention windows.
func TestManagerForegroundCommandsDoNotRetireBackgroundJobs(t *testing.T) {
	manager := newTestManager(t, ManagerOptions{MaxFinished: 2, MaxForeground: 3})
	background, err := manager.Start(context.Background(), Spec{Command: []string{"sh", "-c", "echo kept"}})
	if err != nil {
		t.Fatal(err)
	}
	managerWait(t, manager, background)
	var foreground []string
	for range 30 {
		id, err := manager.StartForeground(context.Background(), Spec{Command: []string{"true"}})
		if err != nil {
			t.Fatal(err)
		}
		managerWait(t, manager, id)
		foreground = append(foreground, id)
	}
	if data, _, _, _, err := manager.Output(background, 0, 100); err != nil || string(data) != "kept\n" {
		t.Fatalf("30 foreground commands retired the background job: %q %v", data, err)
	}
	if listed := manager.List(); len(listed) != 4 || manager.Retired() != 27 {
		t.Fatalf("retained %d states, retired %d", len(listed), manager.Retired())
	}
	if _, err := manager.Status(foreground[0]); !errors.Is(err, ErrJobRetired) {
		t.Fatalf("the oldest foreground command is still retained: %v", err)
	}
	// A promoted command joins the background window and can push out the oldest there.
	for _, id := range foreground[28:] {
		if err := manager.Promote(id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := manager.Status(background); !errors.Is(err, ErrJobRetired) {
		t.Fatalf("promotion did not apply the background cap: %v", err)
	}
	if _, err := manager.Status(foreground[29]); err != nil {
		t.Fatal(err)
	}
}

// TestShortIDsExpandOnlyInTheirOwnManager pins the displayed form of a job ID.
func TestShortIDsExpandOnlyInTheirOwnManager(t *testing.T) {
	first, err := NewManager(ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	id, err := first.Start(t.Context(), Spec{Command: []string{"true"}})
	if err != nil {
		t.Fatal(err)
	}
	short := ShortID(id)
	if short != id[:4]+"-1" || first.FullID(short) != id || first.FullID(id) != id {
		t.Fatalf("short %q of %q expands to %q", short, id, first.FullID(short))
	}
	if _, err := first.Status(first.FullID(short)); err != nil {
		t.Fatalf("status through the short ID: %v", err)
	}
	for _, other := range []string{"zzzz-1", short + "x", id[:4] + "-", "plain", ""} {
		if got := first.FullID(other); got != other {
			t.Errorf("FullID(%q) = %q, want it unchanged", other, got)
		}
	}
	if ShortID("not-an-id") != "not-an-id" {
		t.Error("ShortID must leave other text alone")
	}
}

func TestManagerRefusesAnOversizedStdinWriteWithoutSendingIt(t *testing.T) {
	manager, err := NewManager(ManagerOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	id, err := manager.Start(t.Context(), Spec{Command: []string{"cat"}, Stdin: true})
	if err != nil {
		t.Fatal(err)
	}
	if n, err := manager.Write(id, make([]byte, MaxInput+1)); !errors.Is(err, ErrInputTooLarge) || n != 0 {
		t.Fatalf("oversized write: n=%d err=%v", n, err)
	}
	if _, err := manager.Write(id, []byte("ok\n")); err != nil {
		t.Fatal(err)
	}
	if err := manager.EOF(id); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Wait(t.Context(), id); err != nil {
		t.Fatal(err)
	}
	if data, _, _, _, err := manager.Output(id, 0, 1024); err != nil || string(data) != "ok\n" {
		t.Fatalf("output %q err %v", data, err)
	}
}
