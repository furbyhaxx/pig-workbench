package config

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestConfigWriterProcess(t *testing.T) {
	key := os.Getenv("WORKBENCH_CONFIG_WRITER")
	if key == "" {
		return
	}
	root := os.Getenv("WORKBENCH_CONFIG_ROOT")
	manager, err := New(Paths{User: filepath.Join(root, "Workbench.toml"), Project: filepath.Join(root, "project.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	baseline := current(t, manager)
	if err := os.WriteFile(filepath.Join(root, key+".ready"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(filepath.Join(root, "go")); err == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("parent did not release save barrier")
		case <-ticker.C:
		}
	}
	value := int64(2048)
	if key == "read.max-lines" {
		value = 800
	}
	if _, err := manager.Save(ctx, User, baseline, []Edit{{Key: key, Value: value}}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestCrossProcessDisjointSavesSurvive(t *testing.T) {
	root := t.TempDir()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	type writer struct {
		command *exec.Cmd
		done    chan error
		log     *os.File
	}
	var writers []writer
	defer func() {
		cancel()
		for _, w := range writers {
			select {
			case <-w.done:
			case <-time.After(5 * time.Second):
				t.Error("writer process did not join")
			}
			w.log.Close()
		}
	}()
	for i, key := range []string{"read.max-lines", "read.max-bytes"} {
		log, err := os.Create(filepath.Join(root, "writer-"+strconv.Itoa(i)+".log"))
		if err != nil {
			t.Fatal(err)
		}
		command := exec.CommandContext(ctx, binary, "-test.run=^TestConfigWriterProcess$")
		command.Env = append(os.Environ(), "WORKBENCH_CONFIG_WRITER="+key, "WORKBENCH_CONFIG_ROOT="+root)
		command.Stdout = log
		command.Stderr = log
		if err := command.Start(); err != nil {
			log.Close()
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- command.Wait() }()
		writers = append(writers, writer{command, done, log})
	}
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, first := os.Stat(filepath.Join(root, "read.max-lines.ready"))
		_, second := os.Stat(filepath.Join(root, "read.max-bytes.ready"))
		if first == nil && second == nil {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("writers did not reach captured-baseline barrier")
		case <-ticker.C:
		}
	}
	if err := os.WriteFile(filepath.Join(root, "go"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, w := range writers {
		select {
		case err := <-w.done:
			w.done <- err
			if err != nil {
				data, _ := os.ReadFile(w.log.Name())
				t.Fatalf("writer failed: %v: %s", err, data)
			}
		case <-ctx.Done():
			t.Fatal("cross-process save timed out")
		}
	}
	manager, err := New(Paths{User: filepath.Join(root, "Workbench.toml"), Project: filepath.Join(root, "project.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	policy := current(t, manager).Read()
	if policy.MaxLines != 800 || policy.MaxBytes != 2048 {
		t.Fatalf("disjoint process save lost data: %+v", policy)
	}
}
