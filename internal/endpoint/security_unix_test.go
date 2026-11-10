//go:build unix

package endpoint

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gofrs/flock"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

func TestPrivateModesAndPermissionProblems(t *testing.T) {
	ctx := context.Background()
	r, dir := registryForTest(t)
	info, err := os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("directory mode: %v, %v", info, err)
	}
	record := recordForTest("mode", "token", os.Getpid())
	if err := r.Register(ctx, record); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "mode.json")
	info, err = os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("record mode: %v, %v", info, err)
	}
	info, err = os.Stat(filepath.Join(dir, ".registry.lock"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("lock mode: %v, %v", info, err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	result, err := r.List(ctx)
	if err != nil || len(result.Problems) != 1 {
		t.Fatalf("insecure file not reported: %+v %v", result.Problems, err)
	}
	var permission *PermissionError
	if !errors.As(result.Problems[0], &permission) || permission.Path != path {
		t.Fatalf("file problem is not typed: %+v", result.Problems[0])
	}
	if err := r.Heartbeat(ctx, record.SessionID, record.Token); !errors.As(err, &permission) {
		t.Fatalf("insecure file accepted for heartbeat: %v", err)
	}
	if err := r.Register(ctx, record); !errors.As(err, &permission) {
		t.Fatalf("insecure file replaced by registration: %v", err)
	}
	info, err = os.Stat(path)
	if err != nil || info.Mode().Perm() != 0644 {
		t.Fatal("insecure file permissions were silently changed")
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(dir); !errors.As(err, &permission) {
		t.Fatalf("insecure directory accepted by Open: %v", err)
	}
	if _, err := r.List(ctx); !errors.As(err, &permission) {
		t.Fatalf("changed directory permissions were not checked: %v", err)
	}
	info, err = os.Stat(dir)
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatal("directory permissions were silently changed")
	}
}

func TestInsecureLockIsRejected(t *testing.T) {
	ctx := context.Background()
	r, dir := registryForTest(t)
	path := filepath.Join(dir, ".registry.lock")
	if err := os.WriteFile(path, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	var permission *PermissionError
	if _, err := r.List(ctx); !errors.As(err, &permission) || permission.Path != path {
		t.Fatalf("insecure lock not rejected: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0644 {
		t.Fatal("insecure lock permissions were silently changed")
	}
}

func TestSpecialFilesAndMissingHeartbeatAreNotPruned(t *testing.T) {
	ctx := context.Background()
	r, dir := registryForTest(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("private"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link.json")); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(dir, "fifo.json"), 0600); err != nil {
		t.Fatal(err)
	}
	record := recordForTest("unknown-age", "token", 4194304)
	record.HeartbeatAt = time.Time{}
	if err := r.Register(ctx, record); err != nil {
		t.Fatal(err)
	}
	removed, err := r.Prune(ctx, 0)
	var problems *ProblemsError
	if !errors.As(err, &problems) || len(problems.Problems) != 3 || len(removed) != 0 {
		t.Fatalf("unsafe files or unknown age were pruned: %v, %v", removed, err)
	}
	for _, name := range []string{"link.json", "fifo.json", "unknown-age.json"} {
		if _, err := os.Lstat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s unexpectedly removed: %v", name, err)
		}
	}
	data, err := os.ReadFile(outside)
	if err != nil || string(data) != "private" {
		t.Fatal("symlink target changed")
	}
}

func TestLockWaitHonorsCancellation(t *testing.T) {
	ctx := context.Background()
	r, dir := registryForTest(t)
	if _, err := r.List(ctx); err != nil {
		t.Fatal(err)
	}
	lock := flock.New(filepath.Join(dir, ".registry.lock"), flock.SetFlag(fileio.LockFlags()), flock.SetPermissions(0600))
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	cancelled, cancel := context.WithTimeout(ctx, 30*time.Millisecond)
	defer cancel()
	if _, err := r.List(cancelled); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock wait did not honor deadline: %v", err)
	}
}

func TestRegisterProcessHelper(t *testing.T) {
	id := os.Getenv("ENDPOINT_TEST_CHILD")
	if id == "" {
		return
	}
	dir := os.Getenv("ENDPOINT_TEST_DIR")
	pid, err := strconv.Atoi(os.Getenv("ENDPOINT_TEST_OWNER_PID"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ready-"+id), nil, 0600); err != nil {
		t.Fatal(err)
	}
	err = r.Register(context.Background(), recordForTest("cross-process", id, pid))
	switch {
	case err == nil:
		fmt.Println("registered")
	case errors.Is(err, ErrSessionExists):
		fmt.Println("exists")
	default:
		t.Fatal(err)
	}
}

func TestConcurrentCrossProcessRegistration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r, dir := registryForTest(t)
	if _, err := r.List(ctx); err != nil {
		t.Fatal(err)
	}
	lock := flock.New(filepath.Join(dir, ".registry.lock"), flock.SetFlag(fileio.LockFlags()), flock.SetPermissions(0600))
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	commands := make([]*exec.Cmd, 2)
	outputs := make([]bytes.Buffer, 2)
	waited := make([]bool, len(commands))
	defer func() {
		cancel()
		for i, cmd := range commands {
			if cmd != nil && !waited[i] {
				cmd.Wait()
			}
		}
	}()
	for i := range commands {
		id := strconv.Itoa(i)
		cmd := exec.CommandContext(ctx, binary, "-test.run=^TestRegisterProcessHelper$")
		cmd.Env = append(os.Environ(), "ENDPOINT_TEST_CHILD="+id, "ENDPOINT_TEST_DIR="+dir, "ENDPOINT_TEST_OWNER_PID="+strconv.Itoa(os.Getpid()))
		cmd.Stdout, cmd.Stderr = &outputs[i], &outputs[i]
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands[i] = cmd
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, first := os.Stat(filepath.Join(dir, "ready-0"))
		_, second := os.Stat(filepath.Join(dir, "ready-1"))
		if first == nil && second == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("child processes did not reach lock barrier")
		case <-ticker.C:
		}
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	registered, exists := 0, 0
	for i, cmd := range commands {
		err := cmd.Wait()
		waited[i] = true
		if err != nil {
			t.Fatalf("child failed: %v: %s", err, outputs[i].String())
		}
		if strings.Contains(outputs[i].String(), "registered\n") {
			registered++
		}
		if strings.Contains(outputs[i].String(), "exists\n") {
			exists++
		}
	}
	if registered != 1 || exists != 1 {
		t.Fatalf("concurrent owners were not serialized: %q %q", outputs[0].String(), outputs[1].String())
	}
}
