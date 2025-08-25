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
    if len(m.Globals) > 0 {
        b.WriteString(".data\n")
        for _, g := range m.Globals {
            fmt.Fprintf(&b, ".globl %s\n%s:\n", g.Name, g.Name)
            fmt.Fprintf(&b, "  .quad %d\n", g.Init)
        }
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
            case ir.OpCopy:
                src := ins.Val.Args[0]
                if dr, okd := alloc.regOf[ins.Res]; okd {
                    if sr, oks := alloc.regOf[src]; oks {
                        fmt.Fprintf(b, "  mov %s, %s\n", sr, dr)
                    } else if cst, isC := isConst(bb, src); isC {
                        fmt.Fprintf(b, "  mov $%d, %s\n", cst, dr)
                    } else {
                        offS := slotOffset(src, frameSize)
                        fmt.Fprintf(b, "  mov %d(%%rbp), %s\n", offS, dr)
                    }
                } else {
                    offD := slotOffset(ins.Res, frameSize)
                    if sr, oks := alloc.regOf[src]; oks {
                        fmt.Fprintf(b, "  mov %s, %%rax\n", sr)
                        fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", offD)
                    } else if cst, isC := isConst(bb, src); isC {
                        fmt.Fprintf(b, "  mov $%d, %%rax\n", cst)
                        fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", offD)
                    } else {
                        offS := slotOffset(src, frameSize)
                        fmt.Fprintf(b, "  mov %d(%%rbp), %%rax\n", offS)
                        fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", offD)
                    }
                }
            case ir.OpAdd, ir.OpSub, ir.OpMul:
                emitArith(b, alloc, bb, frameSize, ins)
            case ir.OpAnd, ir.OpOr, ir.OpXor:
                emitBitwise(b, alloc, bb, frameSize, ins)
            case ir.OpShl, ir.OpShr:
                emitShift(b, alloc, bb, frameSize, ins)
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
            case ir.OpEq, ir.OpNe, ir.OpLt, ir.OpLe, ir.OpGt, ir.OpGe:
                // Compute comparison result 0/1
                // Load lhs into rax, rhs into rcx/immediate
                lhs := ins.Val.Args[0]
                rhs := ins.Val.Args[1]
                if lr, ok := alloc.regOf[lhs]; ok {
                    fmt.Fprintf(b, "  mov %s, %%rax\n", lr)
                } else {
                    offL := slotOffset(lhs, frameSize)
                    fmt.Fprintf(b, "  mov %d(%%rbp), %%rax\n", offL)
                }
                if cst, isC := isConst(bb, rhs); isC {
                    fmt.Fprintf(b, "  cmp $%d, %%rax\n", cst)
                } else if rr, ok := alloc.regOf[rhs]; ok {
                    fmt.Fprintf(b, "  cmp %s, %%rax\n", rr)
                } else {
                    offR := slotOffset(rhs, frameSize)
                    fmt.Fprintf(b, "  cmp %d(%%rbp), %%rax\n", offR)
                }
                cc := map[ir.Op]string{ir.OpEq: "e", ir.OpNe: "ne", ir.OpLt: "l", ir.OpLe: "le", ir.OpGt: "g", ir.OpGe: "ge"}[ins.Val.Op]
                fmt.Fprintf(b, "  set%s %%al\n", cc)
                b.WriteString("  movzx %al, %rax\n")
                if r, ok := alloc.regOf[ins.Res]; ok {
                    fmt.Fprintf(b, "  mov %%rax, %s\n", r)
                } else {
                    off := slotOffset(ins.Res, frameSize)
                    fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", off)
                }
            case ir.OpParam:
                // already spilled in prologue
            case ir.OpAddr:
                // address of SSA slot of arg0 -> dest; ensure base value is materialized to its slot
                base := ins.Val.Args[0]
                offBase := slotOffset(base, frameSize)
                // materialize base to its slot if needed
                if cst, isC := isConst(bb, base); isC {
                    fmt.Fprintf(b, "  mov $%d, %%rax\n", cst)
                    fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", offBase)
                } else if br, ok := alloc.regOf[base]; ok {
                    fmt.Fprintf(b, "  mov %s, %d(%%rbp)\n", br, offBase)
                } // else already in slot
                if r, ok := alloc.regOf[ins.Res]; ok {
                    fmt.Fprintf(b, "  lea %d(%%rbp), %s\n", offBase, r)
                } else {
                    off := slotOffset(ins.Res, frameSize)
                    fmt.Fprintf(b, "  lea %d(%%rbp), %%rax\n", offBase)
                    fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", off)
                }
            case ir.OpSlotAddr:
                base := ins.Val.Args[0]
                offBase := slotOffset(base, frameSize)
                if r, ok := alloc.regOf[ins.Res]; ok {
                    fmt.Fprintf(b, "  lea %d(%%rbp), %s\n", offBase, r)
                } else {
                    off := slotOffset(ins.Res, frameSize)
                    fmt.Fprintf(b, "  lea %d(%%rbp), %%rax\n", offBase)
                    fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", off)
                }
            case ir.OpGlobalAddr:
                if r, ok := alloc.regOf[ins.Res]; ok {
                    fmt.Fprintf(b, "  lea %s(%%rip), %s\n", ins.Val.Sym, r)
                } else {
                    off := slotOffset(ins.Res, frameSize)
                    fmt.Fprintf(b, "  lea %s(%%rip), %%rax\n", ins.Val.Sym)
                    fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", off)
                }
            case ir.OpLoad:
                ptr := ins.Val.Args[0]
                // Load pointer into rcx
                if rr, ok := alloc.regOf[ptr]; ok {
                    fmt.Fprintf(b, "  mov %s, %%rcx\n", rr)
                } else if cst, isC := isConst(bb, ptr); isC {
                    // treat as absolute? we don't support immediate addresses
                    fmt.Fprintf(b, "  mov $%d, %%rcx\n", cst)
                } else {
                    off := slotOffset(ptr, frameSize)
                    fmt.Fprintf(b, "  mov %d(%%rbp), %%rcx\n", off)
                }
                if r, ok := alloc.regOf[ins.Res]; ok {
                    fmt.Fprintf(b, "  mov (%%rcx), %s\n", r)
                } else {
                    off := slotOffset(ins.Res, frameSize)
                    fmt.Fprintf(b, "  mov (%%rcx), %%rax\n")
                    fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", off)
                }
            case ir.OpStore:
                // Args: ptr, value
                ptr := ins.Val.Args[0]
                val := ins.Val.Args[1]
                // rcx <- ptr
                if rr, ok := alloc.regOf[ptr]; ok {
                    fmt.Fprintf(b, "  mov %s, %%rcx\n", rr)
                } else if cst, isC := isConst(bb, ptr); isC {
                    fmt.Fprintf(b, "  mov $%d, %%rcx\n", cst)
                } else {
                    off := slotOffset(ptr, frameSize)
                    fmt.Fprintf(b, "  mov %d(%%rbp), %%rcx\n", off)
                }
                // rax <- val
                if cst, isC := isConst(bb, val); isC {
                    fmt.Fprintf(b, "  mov $%d, %%rax\n", cst)
                } else if vr, ok := alloc.regOf[val]; ok {
                    fmt.Fprintf(b, "  mov %s, %%rax\n", vr)
                } else {
                    off := slotOffset(val, frameSize)
                    fmt.Fprintf(b, "  mov %d(%%rbp), %%rax\n", off)
                }
                b.WriteString("  mov %rax, (%rcx)\n")
            case ir.OpCall:
                if len(ins.Val.Args) > len(argRegs) {
                    return fmt.Errorf("more than 6 integer args not supported")
                }
                // move args into registers
                for i, a := range ins.Val.Args {
                    if cst, isC := isConst(bb, a); isC {
                        fmt.Fprintf(b, "  mov $%d, %s\n", cst, argRegs[i])
                    } else if rr, ok := alloc.regOf[a]; ok {
                        fmt.Fprintf(b, "  mov %s, %s\n", rr, argRegs[i])
                    } else {
                        off := slotOffset(a, frameSize)
                        fmt.Fprintf(b, "  mov %d(%%rbp), %s\n", off, argRegs[i])
                    }
                }
                // align stack and call
                b.WriteString("  sub $8, %rsp\n")
                fmt.Fprintf(b, "  call %s\n", ins.Val.Sym)
                b.WriteString("  add $8, %rsp\n")
                if ins.Res >= 0 {
                    if r, ok := alloc.regOf[ins.Res]; ok {
                        fmt.Fprintf(b, "  mov %%rax, %s\n", r)
                    } else {
                        off := slotOffset(ins.Res, frameSize)
                        fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", off)
                    }
                }
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
            case ir.OpJmp:
                t := int(ins.Val.Args[0])
                if t >= 0 && t < len(f.Blocks) {
                    fmt.Fprintf(b, "  jmp %s\n", f.Blocks[t].Name)
                }
            case ir.OpJnz:
                cond := ins.Val.Args[0]
                if r, ok := alloc.regOf[cond]; ok {
                    fmt.Fprintf(b, "  test %s, %s\n", r, r)
                } else {
                    off := slotOffset(cond, frameSize)
                    fmt.Fprintf(b, "  cmp $0, %d(%%rbp)\n", off)
                }
                ti := int(ins.Val.Args[1])
                fi := int(ins.Val.Args[2])
                if ti >= 0 && ti < len(f.Blocks) { fmt.Fprintf(b, "  jne %s\n", f.Blocks[ti].Name) }
                if fi >= 0 && fi < len(f.Blocks) { fmt.Fprintf(b, "  jmp %s\n", f.Blocks[fi].Name) }
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

func emitBitwise(b *strings.Builder, alloc allocation, bb *ir.BasicBlock, frameSize int, ins ir.Instr) {
    destReg, hasDestReg := alloc.regOf[ins.Res]
    lhs := ins.Val.Args[0]
    rhs := ins.Val.Args[1]
    opInstr := map[ir.Op]string{ir.OpAnd: "and", ir.OpOr: "or", ir.OpXor: "xor"}[ins.Val.Op]
    if hasDestReg {
        if lr, ok := alloc.regOf[lhs]; ok {
            if lr != destReg { fmt.Fprintf(b, "  mov %s, %s\n", lr, destReg) }
        } else {
            offL := slotOffset(lhs, frameSize)
            fmt.Fprintf(b, "  mov %d(%%rbp), %s\n", offL, destReg)
        }
        if cst, isC := isConst(bb, rhs); isC {
            fmt.Fprintf(b, "  %s $%d, %s\n", opInstr, cst, destReg)
        } else if rr, ok := alloc.regOf[rhs]; ok {
            fmt.Fprintf(b, "  %s %s, %s\n", opInstr, rr, destReg)
        } else {
            offR := slotOffset(rhs, frameSize)
            fmt.Fprintf(b, "  %s %d(%%rbp), %s\n", opInstr, offR, destReg)
        }
        return
    }
    offDest := slotOffset(ins.Res, frameSize)
    if lr, ok := alloc.regOf[lhs]; ok {
        fmt.Fprintf(b, "  mov %s, %%rax\n", lr)
    } else {
        offL := slotOffset(lhs, frameSize)
        fmt.Fprintf(b, "  mov %d(%%rbp), %%rax\n", offL)
    }
    if cst, isC := isConst(bb, rhs); isC {
        fmt.Fprintf(b, "  %s $%d, %%rax\n", opInstr, cst)
    } else if rr, ok := alloc.regOf[rhs]; ok {
        fmt.Fprintf(b, "  %s %s, %%rax\n", opInstr, rr)
    } else {
        offR := slotOffset(rhs, frameSize)
        fmt.Fprintf(b, "  %s %d(%%rbp), %%rax\n", opInstr, offR)
    }
    fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", offDest)
}

func emitShift(b *strings.Builder, alloc allocation, bb *ir.BasicBlock, frameSize int, ins ir.Instr) {
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
        if cst, isC := isConst(bb, rhs); isC {
            if ins.Val.Op == ir.OpShl {
                fmt.Fprintf(b, "  shl $%d, %s\n", cst, destReg)
            } else {
                fmt.Fprintf(b, "  sar $%d, %s\n", cst, destReg)
            }
        } else {
            // load count into cl
            if rr, ok := alloc.regOf[rhs]; ok {
                fmt.Fprintf(b, "  mov %s, %%rcx\n", rr)
            } else {
                offR := slotOffset(rhs, frameSize)
                fmt.Fprintf(b, "  mov %d(%%rbp), %%rcx\n", offR)
            }
            if ins.Val.Op == ir.OpShl {
                b.WriteString("  shl %cl, " + destReg + "\n")
            } else {
                b.WriteString("  sar %cl, " + destReg + "\n")
            }
        }
        return
    }
    offDest := slotOffset(ins.Res, frameSize)
    if lr, ok := alloc.regOf[lhs]; ok {
        fmt.Fprintf(b, "  mov %s, %%rax\n", lr)
    } else {
        offL := slotOffset(lhs, frameSize)
        fmt.Fprintf(b, "  mov %d(%%rbp), %%rax\n", offL)
    }
    if cst, isC := isConst(bb, rhs); isC {
        if ins.Val.Op == ir.OpShl {
            fmt.Fprintf(b, "  shl $%d, %%rax\n", cst)
        } else {
            fmt.Fprintf(b, "  sar $%d, %%rax\n", cst)
        }
    } else {
        if rr, ok := alloc.regOf[rhs]; ok {
            fmt.Fprintf(b, "  mov %s, %%rcx\n", rr)
        } else {
            offR := slotOffset(rhs, frameSize)
            fmt.Fprintf(b, "  mov %d(%%rbp), %%rcx\n", offR)
        }
        if ins.Val.Op == ir.OpShl {
            b.WriteString("  shl %cl, %rax\n")
        } else {
            b.WriteString("  sar %cl, %rax\n")
        }
    }
    fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", offDest)
}
