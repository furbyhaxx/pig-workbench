package main

import (
	"fmt"
	"image/png"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/webassembly"
	"github.com/tetratelabs/wazero"
)

func main() {
	t0 := time.Now()
	pool, err := webassembly.Init(webassembly.Config{MinIdle: 1, MaxIdle: 1, MaxTotal: 1, FSConfig: wazero.NewFSConfig()})
	if err != nil {
		panic(err)
	}
	inst, err := pool.GetInstance(30 * time.Second)
	if err != nil {
		panic(err)
	}
	fmt.Println("init", time.Since(t0).Round(time.Millisecond))
	for _, path := range os.Args[1:] {
		b, _ := os.ReadFile(path)
		t1 := time.Now()
		doc, err := inst.OpenDocument(&requests.OpenDocument{File: &b})
		if err != nil {
			fmt.Println(path, "open error:", err)
			continue
		}
		pc, _ := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
		pg := requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: 0}}
		txt, err := inst.GetPageText(&requests.GetPageText{Page: pg})
		snippet := ""
		if err == nil {
			snippet = strings.Join(strings.Fields(txt.Text), " ")
			if len(snippet) > 90 {
				snippet = snippet[:90]
			}
		}
		r, err := inst.RenderPageInDPI(&requests.RenderPageInDPI{Page: pg, DPI: 96})
		if err != nil {
			fmt.Println("render error", err)
		} else {
			f, _ := os.Create("/tmp/wb-backends/" + strings.ReplaceAll(strings.TrimPrefix(path, "/"), "/", "_") + ".png")
			png.Encode(f, r.Result.Image)
			f.Close()
			r.Cleanup()
		}
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		fmt.Printf("%s pages=%d first-page-text=%q render=%v total=%s heapMB=%d\n", path, pc.PageCount, snippet, err == nil, time.Since(t1).Round(time.Millisecond), m.HeapAlloc>>20)
		inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})
	}
	// corrupt input
	bad := []byte("%PDF-1.7\n garbage")
	_, err = inst.OpenDocument(&requests.OpenDocument{File: &bad})
	fmt.Println("corrupt:", err)
	inst.Close()
	pool.Close()
}
