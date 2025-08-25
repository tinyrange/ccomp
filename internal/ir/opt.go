package ir

import (
    "unsafe"
)

// Phase 2 basic optimizations: constant folding/propagation and DCE.

// Optimize applies simple SSA-based optimizations to all functions.
func Optimize(m *Module) {
    for _, f := range m.Funcs {
        constFoldFunc(f)
        dceFunc(f)
    }
}

type useInfo struct {
    uses   map[ValueID]int
    byInst []struct{ args []ValueID }
}

func buildUses(f *Function) useInfo {
    ui := useInfo{uses: map[ValueID]int{}}
    for _, b := range f.Blocks {
        for _, ins := range b.Instrs {
            ui.byInst = append(ui.byInst, struct{ args []ValueID }{args: append([]ValueID(nil), ins.Val.Args...)})
            for _, a := range ins.Val.Args { ui.uses[a]++ }
        }
    }
    return ui
}

func constFoldFunc(f *Function) {
    // Local rewrite when both operands are constants.
    for _, b := range f.Blocks {
        for i, ins := range b.Instrs {
            switch ins.Val.Op {
            case OpAdd, OpSub, OpMul, OpDiv, OpAnd, OpOr, OpXor, OpShl, OpShr:
                if len(ins.Val.Args) != 2 { continue }
                a := findConst(b, ins.Val.Args[0])
                c := findConst(b, ins.Val.Args[1])
                if a == nil || c == nil { continue }
                var k int64
                switch ins.Val.Op {
                case OpAdd: k = *a + *c
                case OpSub: k = *a - *c
                case OpMul: k = *a * *c
                case OpDiv:
                    if *c == 0 { continue }
                    k = *a / *c
                case OpAnd: k = *a & *c
                case OpOr:  k = *a | *c
                case OpXor: k = *a ^ *c
                case OpShl: k = *a << uint64(*c)
                case OpShr: k = *a >> uint64(*c)
                }
                // Replace with const
                b.Instrs[i].Val.Op = OpConst
                b.Instrs[i].Val.Args = nil
                b.Instrs[i].Val.Const = k
            case OpFAdd, OpFSub, OpFMul, OpFDiv:
                // Floating point constant folding
                if len(ins.Val.Args) != 2 { continue }
                a := findFConst(b, ins.Val.Args[0])
                c := findFConst(b, ins.Val.Args[1])
                if a == nil || c == nil { continue }
                var result float64
                switch ins.Val.Op {
                case OpFAdd: result = *a + *c
                case OpFSub: result = *a - *c
                case OpFMul: result = *a * *c
                case OpFDiv:
                    if *c == 0.0 { continue }
                    result = *a / *c
                }
                // Convert result to bits and replace with FConst
                bits := int64(*(*uint64)(unsafe.Pointer(&result)))
                b.Instrs[i].Val.Op = OpFConst
                b.Instrs[i].Val.Args = nil
                b.Instrs[i].Val.Const = bits
            }
        }
    }
}

func findConst(b *BasicBlock, id ValueID) *int64 {
    for _, ins := range b.Instrs {
        if ins.Res == id && ins.Val.Op == OpConst {
            v := ins.Val.Const
            return &v
        }
    }
    return nil
}

func findFConst(b *BasicBlock, id ValueID) *float64 {
    for _, ins := range b.Instrs {
        if ins.Res == id && ins.Val.Op == OpFConst {
            // Convert bits back to float64
            bits := uint64(ins.Val.Const)
            v := *(*float64)(unsafe.Pointer(&bits))
            return &v
        }
    }
    return nil
}

func dceFunc(f *Function) {
    // Remove instructions whose results are unused and have no side effects.
    // Iterate to fixed point since removing can cascade.
    changed := true
    for changed {
        changed = false
        ui := buildUses(f)
        for _, b := range f.Blocks {
            out := b.Instrs[:0]
            for _, ins := range b.Instrs {
                if ins.Val.Op == OpRet { out = append(out, ins); continue }
                if ins.Res < 0 { out = append(out, ins); continue }
                // No side effects in Phase 1 ops; can delete if unused
                // Keep params, calls, and stores (including byte stores)
                if ui.uses[ins.Res] == 0 && ins.Val.Op != OpParam && ins.Val.Op != OpCall && ins.Val.Op != OpStore && ins.Val.Op != OpStore8 {
                    changed = true
                    continue
                }
                out = append(out, ins)
            }
            b.Instrs = out
        }
    }
}
