package main

import (
    "fmt"
    "io/ioutil"
    "os"
    "path/filepath"

    "github.com/tinyrange/cc/internal/codegen/x86_64"
    "github.com/tinyrange/cc/internal/ir"
    "github.com/tinyrange/cc/internal/parser"
)

func main() {
    var outPath string
    var srcPath string
    // Minimal arg parsing supporting -o anywhere
    args := os.Args[1:]
    for i := 0; i < len(args); i++ {
        a := args[i]
        if a == "-o" && i+1 < len(args) {
            outPath = args[i+1]
            i++
            continue
        }
        if len(srcPath) == 0 && len(a) > 0 && a[0] != '-' {
            srcPath = a
            continue
        }
    }
    if srcPath == "" {
        fmt.Fprintln(os.Stderr, "usage: ccomp [-o out.s] <file.c>")
        os.Exit(2)
    }
    data, err := ioutil.ReadFile(srcPath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "read error: %v\n", err)
        os.Exit(1)
    }

    astFile, perr := parser.ParseFile(srcPath, string(data))
    if perr != nil {
        fmt.Fprintf(os.Stderr, "parse error: %v\n", perr)
        os.Exit(1)
    }

    m := ir.NewModule(filepath.Base(srcPath))
    if err := ir.BuildModule(astFile, m); err != nil {
        fmt.Fprintf(os.Stderr, "ir error: %v\n", err)
        os.Exit(1)
    }

    // Phase 2: basic optimizations
    ir.Optimize(m)
    // SSA destruction groundwork: phi elimination (CFG-aware, no-op if no branches)
    for _, f := range m.Funcs { ir.PhiEliminate(f) }

    asm, err := x86_64.EmitModule(m)
    if err != nil {
        fmt.Fprintf(os.Stderr, "codegen error: %v\n", err)
        os.Exit(1)
    }

    if outPath == "" {
        fmt.Print(asm)
        return
    }
    if err := os.WriteFile(outPath, []byte(asm), 0644); err != nil {
        fmt.Fprintf(os.Stderr, "write error: %v\n", err)
        os.Exit(1)
    }
}
