package ir

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
            case OpAdd, OpSub, OpMul, OpDiv:
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
                }
                // Replace with const
                b.Instrs[i].Val.Op = OpConst
                b.Instrs[i].Val.Args = nil
                b.Instrs[i].Val.Const = k
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
                // Do not DCE calls even if result unused
                if ui.uses[ins.Res] == 0 && ins.Val.Op != OpParam && ins.Val.Op != OpCall {
                    changed = true
                    continue
                }
                out = append(out, ins)
            }
            b.Instrs = out
        }
    }
}
