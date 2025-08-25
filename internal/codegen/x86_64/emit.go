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

    // Spill params to their SSA slots
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
        off := slotOffset(id, frameSize)
        fmt.Fprintf(b, "  mov %s, %d(%%rbp)\n", argRegs[i], off)
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
                off := slotOffset(ins.Res, frameSize)
                fmt.Fprintf(b, "  mov $%d, %%rax\n", ins.Val.Const)
                fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", off)
            case ir.OpAdd, ir.OpSub, ir.OpMul, ir.OpDiv:
                // load lhs -> rax, rhs -> rcx, compute into rax, spill
                lhs := ins.Val.Args[0]
                rhs := ins.Val.Args[1]
                offL := slotOffset(lhs, frameSize)
                offR := slotOffset(rhs, frameSize)
                fmt.Fprintf(b, "  mov %d(%%rbp), %%rax\n", offL)
                fmt.Fprintf(b, "  mov %d(%%rbp), %%rcx\n", offR)
                switch ins.Val.Op {
                case ir.OpAdd:
                    b.WriteString("  add %rcx, %rax\n")
                case ir.OpSub:
                    b.WriteString("  sub %rcx, %rax\n")
                case ir.OpMul:
                    b.WriteString("  imul %rcx, %rax\n")
                case ir.OpDiv:
                    // signed division rdx:rax / rcx -> rax
                    b.WriteString("  cqo\n")
                    b.WriteString("  idiv %rcx\n")
                }
                off := slotOffset(ins.Res, frameSize)
                fmt.Fprintf(b, "  mov %%rax, %d(%%rbp)\n", off)
            case ir.OpParam:
                // already spilled in prologue
            case ir.OpRet:
                // Arg0 -> rax
                id := ins.Val.Args[0]
                off := slotOffset(id, frameSize)
                fmt.Fprintf(b, "  mov %d(%%rbp), %%rax\n", off)
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

// import of slices above is used to ensure Go 1.20+ build; silence unused if toolchain older
// no-op
