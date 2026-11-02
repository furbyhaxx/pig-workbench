package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/mholt/archives"
)

func main() {
	ctx := context.Background()
	for _, p := range os.Args[1:] {
		f, _ := os.Open(p)
		t := time.Now()
		format, stream, err := archives.Identify(ctx, filepath.Base(p), f)
		if err != nil {
			fmt.Printf("%-10s identify: %v\n", filepath.Base(p), err)
			f.Close()
			continue
		}
		ex, ok := format.(archives.Extractor)
		if !ok {
			fmt.Printf("%-10s %T not an archive\n", filepath.Base(p), format)
			f.Close()
			continue
		}
		members := []string{}
		var expanded int64
		err = ex.Extract(ctx, stream, func(ctx context.Context, fi archives.FileInfo) error {
			name := fi.NameInArchive
			kind := "file"
			if fi.IsDir() {
				kind = "dir"
			} else if fi.LinkTarget != "" {
				kind = "link->" + fi.LinkTarget
			}
			if !fi.IsDir() && fi.LinkTarget == "" {
				rc, err := fi.Open()
				if err != nil {
					return err
				}
				n, _ := io.Copy(io.Discard, io.LimitReader(rc, 50<<20+1))
				rc.Close()
				expanded += n
				if n > 50<<20 {
					kind += " (EXCEEDS 50MiB cap)"
				}
			}
			members = append(members, name+":"+kind)
			return nil
		})
		fmt.Printf("%-10s %-22T err=%v expanded=%d %s members=%v\n", filepath.Base(p), format, err, expanded, time.Since(t).Round(time.Millisecond), members)
		f.Close()
	}
}
