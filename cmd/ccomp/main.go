package main

import (
    "flag"
    "fmt"
    "io/ioutil"
    "os"
    "path/filepath"

    "github.com/tinyrange/cc/internal/codegen/x86_64"
    "github.com/tinyrange/cc/internal/ir"
    "github.com/tinyrange/cc/internal/parser"
)

func main() {
    out := flag.String("o", "", "output file (defaults to stdout)")
    flag.Parse()

    if flag.NArg() < 1 {
        fmt.Fprintln(os.Stderr, "usage: ccomp <file.c> [-o out.s]")
        os.Exit(2)
    }
    srcPath := flag.Arg(0)
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

    asm, err := x86_64.EmitModule(m)
    if err != nil {
        fmt.Fprintf(os.Stderr, "codegen error: %v\n", err)
        os.Exit(1)
    }

    if *out == "" {
        fmt.Print(asm)
        return
    }
    if err := os.WriteFile(*out, []byte(asm), 0644); err != nil {
        fmt.Fprintf(os.Stderr, "write error: %v\n", err)
        os.Exit(1)
    }
}

