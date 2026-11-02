package main

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	gopty "github.com/aymanbagabas/go-pty"
)

type buf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *buf) Write(p []byte) (int, error) { b.mu.Lock(); defer b.mu.Unlock(); return b.b.Write(p) }
func (b *buf) String() string              { b.mu.Lock(); defer b.mu.Unlock(); return b.b.String() }

func run(name string, args []string, input func(p gopty.Pty)) {
	p, err := gopty.New()
	if err != nil {
		panic(err)
	}
	defer p.Close()
	p.Resize(100, 30)
	cmd := p.Command(name, args...)
	if err := cmd.Start(); err != nil {
		panic(err)
	}
	out := &buf{}
	go io.Copy(out, p)
	time.Sleep(300 * time.Millisecond)
	input(p)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		status := "exit 0"
		if ee, ok := err.(*exec.ExitError); ok {
			ws := ee.Sys().(syscall.WaitStatus)
			status = fmt.Sprintf("exit=%d signaled=%v sig=%v", ws.ExitStatus(), ws.Signaled(), ws.Signal())
		}
		fmt.Printf("%-28s %s output=%q\n", name+" "+strings.Join(args, " "), status, strings.TrimSpace(out.String()))
	case <-time.After(3 * time.Second):
		fmt.Printf("%-28s TIMEOUT output=%q\n", name, out.String())
		cmd.Process.Kill()
	}
}

func main() {
	run("sleep", []string{"30"}, func(p gopty.Pty) { p.Write([]byte{0x03}) })
	run("cat", nil, func(p gopty.Pty) { p.Write([]byte("abc\n")); time.Sleep(100 * time.Millisecond); p.Write([]byte{0x04}) })
	run("sh", []string{"-c", "trap 'stty size; exit 7' WINCH; while :; do sleep 0.1; done"}, func(p gopty.Pty) { p.Resize(120, 40) })
	run("sh", []string{"-c", "read -r line; echo got:$line"}, func(p gopty.Pty) { p.Write([]byte("hello\r")) })
}
