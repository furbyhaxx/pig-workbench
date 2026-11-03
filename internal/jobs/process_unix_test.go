//go:build linux || darwin

package jobs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type lineWriter struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	lines chan string
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	w.buf.Write(p)
	for {
		line, err := w.buf.ReadString('\n')
		if err != nil {
			w.buf.WriteString(line)
			break
		}
		w.lines <- strings.TrimSpace(line)
	}
	return n, nil
}

func startTestProcess(t *testing.T, spec Spec) *Process {
	t.Helper()
	p, err := StartProcess(spec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := p.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	})
	return p
}

func waitTestProcess(t *testing.T, p *Process) Exit {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	exit, err := p.Wait(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return exit
}

func TestOutputAndExitCode(t *testing.T) {
	var output bytes.Buffer
	p := startTestProcess(t, Spec{Command: []string{"printf 'hello\\n'; printf 'error\\n' >&2; exit 7"}, Shell: true, Output: &output})
	exit := waitTestProcess(t, p)
	if exit.Code != 7 || exit.Signaled || exit.Signal != "" || exit.Duration <= 0 {
		t.Fatalf("unexpected exit: %+v", exit)
	}
	if got := output.String(); got != "hello\nerror\n" {
		t.Fatalf("unexpected combined output: %q", got)
	}
}

func TestFastExitsAreObserved(t *testing.T) {
	for i := 0; i < 100; i++ {
		p, err := StartProcess(Spec{Command: []string{"true"}})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		exit, err := p.Wait(ctx)
		cancel()
		if err != nil || exit.Code != 0 {
			p.Close()
			t.Fatalf("iteration %d: exit %+v, error %v", i, exit, err)
		}
		if err := p.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWaitCancellationDoesNotKill(t *testing.T) {
	p := startTestProcess(t, Spec{Command: []string{"sleep", "30"}})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := p.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait error = %v, want canceled", err)
	}
	if err := syscall.Kill(p.PID(), 0); err != nil {
		t.Fatalf("wait canceled the process: %v", err)
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	p := startTestProcess(t, Spec{Command: []string{"sleep", "30"}})
	for i := 0; i < 2; i++ {
		if err := p.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := syscall.Kill(p.PID(), 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("process still present after close: %v", err)
	}
	if exit := waitTestProcess(t, p); !exit.Signaled {
		t.Fatalf("close exit = %+v", exit)
	}
}

func TestKillProcessGroup(t *testing.T) {
	output := &lineWriter{lines: make(chan string, 1)}
	p := startTestProcess(t, Spec{Command: []string{"sleep 30 & echo $!; wait"}, Shell: true, Output: output})
	var grandchild int
	select {
	case line := <-output.lines:
		var err error
		grandchild, err = strconv.Atoi(line)
		if err != nil {
			t.Fatalf("invalid grandchild PID %q: %v", line, err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("grandchild did not start")
	}
	if err := p.Kill(200 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	for _, pid := range []int{p.PID(), grandchild} {
		if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
			t.Errorf("PID %d still present after Kill: %v", pid, err)
		}
	}
	if exit := waitTestProcess(t, p); !exit.Signaled || exit.Signal == "" {
		t.Fatalf("group kill exit = %+v", exit)
	}
}

func TestKillAfterLeaderExit(t *testing.T) {
	output := &lineWriter{lines: make(chan string, 1)}
	p := startTestProcess(t, Spec{Command: []string{"sleep 30 & echo $!; exit 0"}, Shell: true, Output: output})
	var grandchild int
	select {
	case line := <-output.lines:
		var err error
		grandchild, err = strconv.Atoi(line)
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("grandchild did not start")
	}
	select {
	case <-p.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("leader did not exit")
	}
	if err := p.Kill(200 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Kill(grandchild, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("grandchild %d still present after Kill: %v", grandchild, err)
	}
	if exit := waitTestProcess(t, p); exit.Signaled || exit.Code != 0 {
		t.Fatalf("leader exit = %+v", exit)
	}
}

func TestKillAfterWaitReturnsReaped(t *testing.T) {
	p := startTestProcess(t, Spec{Command: []string{"true"}})
	waitTestProcess(t, p)
	if err := p.Kill(0); !errors.Is(err, ErrReaped) {
		t.Fatalf("kill after wait: %v", err)
	}
	if err := p.Signal(syscall.SIGTERM); !errors.Is(err, ErrReaped) {
		t.Fatalf("signal after wait: %v", err)
	}
}

func TestCloseJoinsWatchers(t *testing.T) {
	baseline := runtime.NumGoroutine()
	for i := 0; i < 50; i++ {
		p, err := StartProcess(Spec{Command: []string{"sleep", "30"}})
		if err != nil {
			t.Fatal(err)
		}
		if err := p.Kill(0); err != nil {
			t.Fatal(err)
		}
		if err := p.Close(); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > baseline+2 && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if got := runtime.NumGoroutine(); got > baseline+2 {
		t.Fatalf("goroutines before = %d, after = %d", baseline, got)
	}
}

func TestKillEscalatesWhenLeaderExitsFirst(t *testing.T) {
	output := &lineWriter{lines: make(chan string, 1)}
	p := startTestProcess(t, Spec{Command: []string{"sh -c 'trap \"\" TERM; echo $$; exec sleep 30' & wait"}, Shell: true, Output: output})
	var grandchild int
	select {
	case line := <-output.lines:
		var err error
		grandchild, err = strconv.Atoi(line)
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("grandchild did not start")
	}
	start := time.Now()
	if err := p.Kill(200 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 150*time.Millisecond {
		t.Fatal("kill returned before the grace period and escalation")
	}
	if err := syscall.Kill(grandchild, 0); !errors.Is(err, syscall.ESRCH) {
		stat, statErr := os.ReadFile("/proc/" + strconv.Itoa(grandchild) + "/stat")
		t.Fatalf("grandchild %d still present after escalation: %v, group=%v, stat=%s (%v)", grandchild, err, syscall.Kill(-p.PID(), 0), stat, statErr)
	}
	if exit := waitTestProcess(t, p); !exit.Signaled {
		t.Fatalf("group kill exit = %+v", exit)
	}
}

var errOutputWrite = errors.New("output write failed")

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errOutputWrite }

func TestOutputErrorIsReturnedForNonzeroExit(t *testing.T) {
	p, err := StartProcess(Spec{Command: []string{"printf hello; exit 7"}, Shell: true, Output: failingWriter{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exit, err := p.Wait(ctx)
	if !errors.Is(err, errOutputWrite) || exit.Code != 7 {
		t.Fatalf("exit %+v, error %v", exit, err)
	}
	if err := p.Close(); !errors.Is(err, errOutputWrite) {
		t.Fatalf("close error = %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second close error = %v", err)
	}
}

type slowCounter struct{ size int64 }

func (w *slowCounter) Write(p []byte) (int, error) {
	time.Sleep(time.Millisecond)
	w.size += int64(len(p))
	return len(p), nil
}

func TestSlowOutputLargerThanPipe(t *testing.T) {
	const size = 2 * 1024 * 1024
	output := &slowCounter{}
	p := startTestProcess(t, Spec{Command: []string{"dd if=/dev/zero bs=1024 count=2048 2>/dev/null"}, Shell: true, Output: output})
	if exit := waitTestProcess(t, p); exit.Code != 0 {
		t.Fatalf("exit = %+v", exit)
	}
	if output.size != size {
		t.Fatalf("got %d bytes, want %d", output.size, size)
	}
}

func TestStdinRoundTrip(t *testing.T) {
	var output bytes.Buffer
	p := startTestProcess(t, Spec{Command: []string{"cat"}, Stdin: true, Output: &output})
	for _, data := range []string{"first line\n", "second line\n"} {
		n, err := p.Write([]byte(data))
		if err != nil || n != len(data) {
			t.Fatalf("write = %d, %v", n, err)
		}
	}
	if err := p.CloseStdin(); err != nil {
		t.Fatal(err)
	}
	if err := p.CloseStdin(); err != nil {
		t.Fatal(err)
	}
	if exit := waitTestProcess(t, p); exit.Code != 0 || exit.Signaled {
		t.Fatalf("exit = %+v", exit)
	}
	if got := output.String(); got != "first line\nsecond line\n" {
		t.Fatalf("output = %q", got)
	}
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Write([]byte("late")); !errors.Is(err, ErrStdinClosed) {
		t.Fatalf("write after close = %v", err)
	} else {
		var closed *StdinClosedError
		if !errors.As(err, &closed) {
			t.Fatalf("write error has no StdinClosedError type: %v", err)
		}
	}
}

func TestStdinDisabledByDefault(t *testing.T) {
	p := startTestProcess(t, Spec{Command: []string{"cat"}})
	if exit := waitTestProcess(t, p); exit.Code != 0 {
		t.Fatalf("exit = %+v", exit)
	}
	if _, err := p.Write([]byte("input")); !errors.Is(err, ErrStdinClosed) {
		t.Fatalf("write without stdin = %v", err)
	}
}

func TestWriteAfterLeaderExit(t *testing.T) {
	p := startTestProcess(t, Spec{Command: []string{"true"}, Stdin: true})
	select {
	case <-p.Exited():
	case <-time.After(5 * time.Second):
		t.Fatal("child did not exit")
	}
	if _, err := p.Write([]byte("late")); !errors.Is(err, ErrStdinClosed) {
		t.Fatalf("write after exit = %v", err)
	}
	if exit := waitTestProcess(t, p); exit.Code != 0 {
		t.Fatalf("exit = %+v", exit)
	}
}

func TestKillClosesBlockedStdinWrite(t *testing.T) {
	p := startTestProcess(t, Spec{Command: []string{"sleep", "30"}, Stdin: true})
	writeDone := make(chan error, 1)
	go func() {
		_, err := p.Write(bytes.Repeat([]byte{'x'}, 2*1024*1024))
		writeDone <- err
	}()
	select {
	case err := <-writeDone:
		t.Fatalf("write did not block: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	killDone := make(chan error, 1)
	go func() { killDone <- p.Kill(100 * time.Millisecond) }()
	select {
	case err := <-killDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		_ = syscall.Kill(-p.PID(), syscall.SIGKILL)
		t.Fatal("kill blocked on stdin write")
	}
	select {
	case err := <-writeDone:
		if !errors.Is(err, ErrStdinClosed) {
			t.Fatalf("blocked write error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("stdin writer did not stop")
	}
}

func TestWriteAfterProcessClose(t *testing.T) {
	p := startTestProcess(t, Spec{Command: []string{"cat"}, Stdin: true})
	done := make(chan error, 1)
	go func() { done <- p.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		_ = syscall.Kill(-p.PID(), syscall.SIGKILL)
		t.Fatal("close blocked on child waiting for stdin")
	}
	if _, err := p.Write([]byte("late")); !errors.Is(err, ErrStdinClosed) {
		t.Fatalf("write after process close = %v", err)
	}
}

func TestKillClosesStdinWhileChildWaitsForInput(t *testing.T) {
	p := startTestProcess(t, Spec{Command: []string{"cat"}, Stdin: true})
	done := make(chan error, 1)
	go func() { done <- p.Kill(100 * time.Millisecond) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("kill blocked on child waiting for stdin")
	}
	if _, err := p.Write([]byte("late")); !errors.Is(err, ErrStdinClosed) {
		t.Fatalf("write after kill = %v", err)
	}
}

func TestConcurrentStdinAndOutput(t *testing.T) {
	reader, writer := io.Pipe()
	p := startTestProcess(t, Spec{Command: []string{"cat"}, Stdin: true, Output: writer})
	t.Cleanup(func() { reader.Close(); writer.Close() })
	input := bytes.Repeat([]byte("concurrent input\n"), 64*1024)
	type readResult struct {
		data []byte
		err  error
	}
	readDone := make(chan readResult, 1)
	go func() {
		data, err := io.ReadAll(reader)
		readDone <- readResult{data, err}
	}()
	writeDone := make(chan error, 1)
	go func() {
		n, err := p.Write(input)
		if err == nil && n != len(input) {
			err = io.ErrShortWrite
		}
		writeDone <- errors.Join(err, p.CloseStdin())
	}()
	select {
	case err := <-writeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("stdin write deadlocked against output")
	}
	if exit := waitTestProcess(t, p); exit.Code != 0 {
		t.Fatalf("exit = %+v", exit)
	}
	writer.Close()
	select {
	case result := <-readDone:
		if result.err != nil || !bytes.Equal(result.data, input) {
			t.Fatalf("read = %d bytes, %v; want %d bytes", len(result.data), result.err, len(input))
		}
	case <-time.After(time.Second):
		t.Fatal("output reader did not stop")
	}
}

var errStdinClose = errors.New("stdin close failed")

type closeErrorWriter struct{ io.WriteCloser }

func (w closeErrorWriter) Close() error {
	return errors.Join(w.WriteCloser.Close(), errStdinClose)
}

type notifiedFailWriter struct{ called chan struct{} }

func (w notifiedFailWriter) Write([]byte) (int, error) {
	close(w.called)
	return 0, errOutputWrite
}

func TestCloseReportsJoinedFailuresOnlyOnce(t *testing.T) {
	output := notifiedFailWriter{called: make(chan struct{})}
	p, err := StartProcess(Spec{Command: []string{"printf hello; sleep 30"}, Shell: true, Stdin: true, Output: output})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	p.stdin = closeErrorWriter{p.stdin}
	select {
	case <-output.called:
	case <-time.After(5 * time.Second):
		t.Fatal("child did not write output")
	}
	err = p.Close()
	if !errors.Is(err, errStdinClose) || !errors.Is(err, errOutputWrite) {
		t.Fatalf("first close = %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("second close = %v", err)
	}
}

func TestCommandValidationAndShellArguments(t *testing.T) {
	if _, err := StartProcess(Spec{}); err == nil {
		t.Fatal("empty command accepted")
	}
	var output bytes.Buffer
	p := startTestProcess(t, Spec{Command: []string{"printf '%s' \"$1\"", "a b; false"}, Shell: true, Output: &output, Env: os.Environ()})
	if exit := waitTestProcess(t, p); exit.Code != 0 {
		t.Fatalf("exit = %+v", exit)
	}
	if got := output.String(); got != "a b; false" {
		t.Fatalf("shell argument = %q", got)
	}
}
