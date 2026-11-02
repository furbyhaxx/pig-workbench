package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed svgrender.wasm
var module []byte

func main() {
	ctx := context.Background()
	t0 := time.Now()
	cache, _ := wazero.NewCompilationCacheWithDir("/tmp/wb-backends/wazero-cache")
	rt := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfig().WithCompilationCache(cache).WithMemoryLimitPages(4096).WithCloseOnContextDone(true))
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)
	compiled, err := rt.CompileModule(ctx, module)
	if err != nil {
		panic(err)
	}
	fmt.Println("compile", time.Since(t0).Round(time.Millisecond))
	for _, p := range os.Args[1:] {
		src, _ := os.ReadFile(p)
		var out, errb bytes.Buffer
		t := time.Now()
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		fs := wazero.NewFSConfig().WithReadOnlyDirMount("/usr/share/fonts", "/fonts")
		cfg := wazero.NewModuleConfig().WithStdin(bytes.NewReader(src)).WithStdout(&out).WithStderr(&errb).WithArgs("svgrender", "800").WithFSConfig(fs).WithName("")
		_, err := rt.InstantiateModule(cctx, compiled, cfg)
		cancel()
		name := "/tmp/wb-backends/resvg-" + filepath.Base(p) + ".png"
		os.WriteFile(name, out.Bytes(), 0o644)
		fmt.Println(p, "err=", err, "stderr=", errb.String(), "bytes=", out.Len(), time.Since(t).Round(time.Millisecond))
	}
	// hostile: external reference + huge size + script
	hostile := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink" width="100000" height="100000"><script>alert(1)</script><image xlink:href="file:///etc/passwd" width="10" height="10"/></svg>`
	os.WriteFile("/tmp/wb-backends/hostile.svg", []byte(hostile), 0o644)
}
