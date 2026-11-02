package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

func main() {
	dir, _ := os.MkdirTemp("", "wbwatch")
	defer os.RemoveAll(dir)
	target := filepath.Join(dir, "AGENTS.md")
	os.WriteFile(target, []byte("v1"), 0o644)
	w, _ := fsnotify.NewWatcher()
	defer w.Close()
	w.Add(dir) // watch the directory, not the file
	events := []string{}
	go func() {
		for {
			select {
			case e, ok := <-w.Events:
				if !ok {
					return
				}
				if filepath.Base(e.Name) == "AGENTS.md" {
					events = append(events, e.Op.String())
				}
			case err := <-w.Errors:
				events = append(events, "ERR:"+err.Error())
			}
		}
	}()
	step := func(name string, f func()) {
		events = events[:0]
		f()
		time.Sleep(150 * time.Millisecond)
		b, _ := os.ReadFile(target)
		fmt.Printf("%-26s events=%v content=%q\n", name, events, b)
	}
	step("in-place write", func() { os.WriteFile(target, []byte("v2"), 0o644) })
	step("atomic temp+rename", func() {
		tmp := filepath.Join(dir, ".AGENTS.md.tmp")
		os.WriteFile(tmp, []byte("v3"), 0o644)
		os.Rename(tmp, target)
	})
	step("vim backup-rename", func() {
		os.Rename(target, target+"~")
		os.WriteFile(target, []byte("v4"), 0o644)
		os.Remove(target + "~")
	})
	step("delete", func() { os.Remove(target) })
	step("recreate", func() { os.WriteFile(target, []byte("v5"), 0o644) })
	sub := filepath.Join(dir, "sub")
	os.Mkdir(sub, 0o755)
	step("nested (unwatched dir)", func() { os.WriteFile(filepath.Join(sub, "AGENTS.md"), []byte("x"), 0o644) })
	b, _ := os.ReadFile("/proc/sys/fs/inotify/max_user_watches")
	fmt.Printf("max_user_watches=%s", b)
}
