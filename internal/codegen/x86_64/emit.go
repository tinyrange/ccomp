package x86_64

import (
    "fmt"
    "strings"

    "github.com/tinyrange/cc/internal/ir"
)

// EmitModule emits AT&T syntax x86_64 assembly for System V AMD64.
func EmitModule(m *ir.Module) (string, error) {
    var b strings.Builder
    b.WriteString(".text\n")
    for _, f := range m.Funcs {
        if err := emitFunc(&b, f); err != nil { return "", err }
    }
    return b.String(), nil
}

var argRegs = []string{"%rdi", "%rsi", "%rdx", "%rcx", "%r8", "%r9"}

func emitFunc(b *strings.Builder, f *ir.Function) error {
    fmt.Fprintf(b, ".globl %s\n%s:\n", f.Name, f.Name)
    // Prologue
    b.WriteString("  push %rbp\n")
    b.WriteString("  mov %rsp, %rbp\n")

    // Allocate registers (simple linear scan, avoid %rax)
    alloc := allocateRegisters(f)

    // Assign stack slots for SSA values
    // Reserve 8 bytes per SSA value id used in function
    // Find max ID
    maxID := ir.ValueID(-1)
    for _, bb := range f.Blocks {
        for _, ins := range bb.Instrs {
            if ins.Res > maxID { maxID = ins.Res }
        }
    }
    // params handled via OpParam results, which also need slots
    numSlots := int(maxID) + 1
    frameSize := align(numSlots*8, 16)
    if frameSize > 0 {
        fmt.Fprintf(b, "  sub $%d, %%rsp\n", frameSize)
    }

    // Move params into their home (reg or spill)
    // Traverse instructions to find first runs of OpParam to determine order
    var paramIDs []ir.ValueID
    for _, ins := range f.Blocks[0].Instrs {
        if ins.Val.Op == ir.OpParam {
            paramIDs = append(paramIDs, ins.Res)
        }
    }
    for i, id := range paramIDs {
        if i >= len(argRegs) {
            return fmt.Errorf("more than 6 integer params not supported")
        }
        if r, ok := alloc.regOf[id]; ok {
            fmt.Fprintf(b, "  mov %s, %s\n", argRegs[i], r)
        } else {
            off := slotOffset(id, frameSize)
            fmt.Fprintf(b, "  mov %s, %d(%%rbp)\n", argRegs[i], off)
        }
    }

    // Emit body
    for _, bb := range f.Blocks {
        // Labels only for non-entry blocks (not used in phase 1)
        if bb != f.Blocks[0] {
            fmt.Fprintf(b, "%s: \n", bb.Name)
        }
        for _, ins := range bb.Instrs {
            switch ins.Val.Op {
            case ir.OpConst:
                if r, ok := alloc.regOf[ins.Res]; ok {
                    fmt.Fprintf(b, "  mov $%d, %s\n", ins.Val.Const, r)
                } else {
                    off := slotOffset(ins.Res, frameSize)
                    fmt.Fprintf(b, "  mov $%d, %%rax\n", ins.Val.Const)
                    fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", off)
                }
            case ir.OpAdd, ir.OpSub, ir.OpMul:
                emitArith(b, alloc, bb, frameSize, ins)
            case ir.OpDiv:
                // signed division rdx:rax / rcx -> rax (special path)
                lhs := ins.Val.Args[0]
                rhs := ins.Val.Args[1]
                // load lhs into rax
                if lr, ok := alloc.regOf[lhs]; ok {
                    fmt.Fprintf(b, "  mov %s, %%rax\n", lr)
                } else {
                    offL := slotOffset(lhs, frameSize)
                    fmt.Fprintf(b, "  mov %d(%%rbp), %%rax\n", offL)
                }
                // load rhs into rcx
                if cst, isC := isConst(bb, rhs); isC {
                    fmt.Fprintf(b, "  mov $%d, %%rcx\n", cst)
                } else if rr, ok := alloc.regOf[rhs]; ok {
                    fmt.Fprintf(b, "  mov %s, %%rcx\n", rr)
                } else {
                    offR := slotOffset(rhs, frameSize)
                    fmt.Fprintf(b, "  mov %d(%%rbp), %%rcx\n", offR)
                }
                b.WriteString("  cqo\n")
                b.WriteString("  idiv %rcx\n")
                if r, ok := alloc.regOf[ins.Res]; ok {
                    fmt.Fprintf(b, "  mov %%rax, %s\n", r)
                } else {
                    off := slotOffset(ins.Res, frameSize)
                    fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", off)
                }
            case ir.OpParam:
                // already spilled in prologue
            case ir.OpRet:
                // Arg0 -> rax
                id := ins.Val.Args[0]
                if r, ok := alloc.regOf[id]; ok {
                    fmt.Fprintf(b, "  mov %s, %%rax\n", r)
                } else {
                    off := slotOffset(id, frameSize)
                    fmt.Fprintf(b, "  mov %d(%%rbp), %%rax\n", off)
                }
                // Epilogue
                if frameSize > 0 {
                    fmt.Fprintf(b, "  add $%d, %%rsp\n", frameSize)
                }
                b.WriteString("  pop %rbp\n")
                b.WriteString("  ret\n")
            default:
                // ignore
            }
        }
    }

    // In case no return found, emit default 0
    // This keeps assembler happy for empty functions
    b.WriteString("  mov $0, %eax\n")
    if frameSize > 0 {
        fmt.Fprintf(b, "  add $%d, %%rsp\n", frameSize)
    }
    b.WriteString("  pop %rbp\n")
    b.WriteString("  ret\n")

    return nil
}

func slotOffset(id ir.ValueID, frameSize int) int {
    // Layout: rbp is base, we place slots at negative offsets.
    // We'll assign offset = - (8*(id+1))
    off := -8 * (int(id) + 1)
    // Ensure within allocated frame (debug safety)
    _ = frameSize
    return off
}

func align(n, a int) int { return (n + (a-1)) &^ (a - 1) }

func isConst(bb *ir.BasicBlock, id ir.ValueID) (int64, bool) {
    if bb == nil { return 0, false }
    for _, ins := range bb.Instrs {
        if ins.Res == id && ins.Val.Op == ir.OpConst { return ins.Val.Const, true }
    }
    return 0, false
}

func emitArith(b *strings.Builder, alloc allocation, bb *ir.BasicBlock, frameSize int, ins ir.Instr) {
    destReg, hasDestReg := alloc.regOf[ins.Res]
    lhs := ins.Val.Args[0]
    rhs := ins.Val.Args[1]
    if hasDestReg {
        if lr, ok := alloc.regOf[lhs]; ok {
            if lr != destReg { fmt.Fprintf(b, "  mov %s, %s\n", lr, destReg) }
        } else {
            offL := slotOffset(lhs, frameSize)
            fmt.Fprintf(b, "  mov %d(%%rbp), %s\n", offL, destReg)
        }
        // rhs
        if cst, isC := isConst(bb, rhs); isC {
            switch ins.Val.Op {
            case ir.OpAdd:
                fmt.Fprintf(b, "  add $%d, %s\n", cst, destReg)
            case ir.OpSub:
                fmt.Fprintf(b, "  sub $%d, %s\n", cst, destReg)
            case ir.OpMul:
                fmt.Fprintf(b, "  imul $%d, %s\n", cst, destReg)
            }
        } else if rr, ok := alloc.regOf[rhs]; ok {
            switch ins.Val.Op {
            case ir.OpAdd:
                fmt.Fprintf(b, "  add %s, %s\n", rr, destReg)
            case ir.OpSub:
                fmt.Fprintf(b, "  sub %s, %s\n", rr, destReg)
            case ir.OpMul:
                fmt.Fprintf(b, "  imul %s, %s\n", rr, destReg)
            }
        } else {
            offR := slotOffset(rhs, frameSize)
            switch ins.Val.Op {
            case ir.OpAdd:
                fmt.Fprintf(b, "  add %d(%%rbp), %s\n", offR, destReg)
            case ir.OpSub:
                fmt.Fprintf(b, "  sub %d(%%rbp), %s\n", offR, destReg)
            case ir.OpMul:
                fmt.Fprintf(b, "  imul %d(%%rbp), %s\n", offR, destReg)
            }
        }
        return
    }
    // Spilled destination: use %rax as temp
    offDest := slotOffset(ins.Res, frameSize)
    if lr, ok := alloc.regOf[lhs]; ok {
        fmt.Fprintf(b, "  mov %s, %%rax\n", lr)
    } else {
        offL := slotOffset(lhs, frameSize)
        fmt.Fprintf(b, "  mov %d(%%rbp), %%rax\n", offL)
    }
    if cst, isC := isConst(bb, rhs); isC {
        switch ins.Val.Op {
        case ir.OpAdd:
            fmt.Fprintf(b, "  add $%d, %%rax\n", cst)
        case ir.OpSub:
            fmt.Fprintf(b, "  sub $%d, %%rax\n", cst)
        case ir.OpMul:
            fmt.Fprintf(b, "  imul $%d, %%rax\n", cst)
        }
    } else if rr, ok := alloc.regOf[rhs]; ok {
        switch ins.Val.Op {
        case ir.OpAdd:
            fmt.Fprintf(b, "  add %s, %%rax\n", rr)
        case ir.OpSub:
            fmt.Fprintf(b, "  sub %s, %%rax\n", rr)
        case ir.OpMul:
            fmt.Fprintf(b, "  imul %s, %%rax\n", rr)
        }
    } else {
        offR := slotOffset(rhs, frameSize)
        switch ins.Val.Op {
        case ir.OpAdd:
            fmt.Fprintf(b, "  add %d(%%rbp), %%rax\n", offR)
        case ir.OpSub:
            fmt.Fprintf(b, "  sub %d(%%rbp), %%rax\n", offR)
        case ir.OpMul:
            fmt.Fprintf(b, "  imul %d(%%rbp), %%rax\n", offR)
        }
    }
    fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", offDest)
}
