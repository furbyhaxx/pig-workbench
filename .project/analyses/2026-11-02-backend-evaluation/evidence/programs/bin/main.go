package main

import (
	"debug/elf"
	"fmt"
	"os"
	"strings"

	"golang.org/x/arch/x86/x86asm"
)

func main() {
	f, err := elf.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	syms, serr := f.Symbols()
	dyn, _ := f.DynamicSymbols()
	libs, _ := f.ImportedLibraries()
	funcs := 0
	var target elf.Symbol
	for _, s := range syms {
		if elf.ST_TYPE(s.Info) == elf.STT_FUNC && s.Size > 0 {
			funcs++
			if s.Name == os.Args[2] {
				target = s
			}
		}
	}
	fmt.Printf("machine=%v class=%v type=%v syms=%d(err=%v) funcs=%d dynsyms=%d libs=%v\n", f.Machine, f.Class, f.Type, len(syms), serr, funcs, len(dyn), libs)
	if target.Name == "" {
		return
	}
	text := f.Section(".text")
	data, _ := text.Data()
	off := target.Value - text.Addr
	code := data[off : off+target.Size]
	pc := target.Value
	for i := 0; len(code) > 0 && i < 8; i++ {
		inst, err := x86asm.Decode(code, 64)
		if err != nil {
			fmt.Printf("%x: decode error %v\n", pc, err)
			break
		}
		fmt.Printf("%x: %s\n", pc, strings.ToLower(x86asm.GNUSyntax(inst, pc, nil)))
		code = code[inst.Len:]
		pc += uint64(inst.Len)
	}
}
