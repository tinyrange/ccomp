package ir

// PhiEliminate lowers OpPhi nodes into parallel copies on incoming edges.
// It assumes block Preds/Succs are populated. If not, it becomes a no-op.
func PhiEliminate(f *Function) {
    // For each block with phis at the top
    for _, b := range f.Blocks {
        // collect phi instructions at block start
        var phis []Instr
        idx := 0
        for idx < len(b.Instrs) && b.Instrs[idx].Val.Op == OpPhi {
            phis = append(phis, b.Instrs[idx])
            idx++
        }
        if len(phis) == 0 || len(b.Preds) == 0 {
            continue
        }
        // For each predecessor insert copies; handle critical edges
        for pi, pred := range b.Preds {
            ip := pred
            if isCritical(pred, b) {
                ip = splitCriticalEdge(f, pred, b)
            }
            // Insert copies before terminator (at end)
            for _, phi := range phis {
                if pi >= len(phi.Val.Args) { continue }
                src := phi.Val.Args[pi]
                dst := phi.Res
                // copy src -> dst
                ip.Instrs = append(ip.Instrs, Instr{Res: dst, Val: Value{Op: OpCopy, Args: []ValueID{src}}})
            }
            // If we created a split block, add jump to successor
            if ip != pred {
                // emit jump to b using OpJmp with target index of b
                // find index of b in function blocks
                ti := blockIndexOf(f, b)
                ip.Instrs = append(ip.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(ti)}}})
            }
        }
        // Remove phi nodes from b
        b.Instrs = b.Instrs[idx:]
    }
}

func isCritical(p, s *BasicBlock) bool {
    return len(p.Succs) > 1 && len(s.Preds) > 1
}

func splitCriticalEdge(f *Function, p, s *BasicBlock) *BasicBlock {
    nb := f.newBlock(p.Name + "_to_" + s.Name)
    // redirect p->s edge to p->nb->s
    // remove s from p.Succs
    var newSuccs []*BasicBlock
    for _, x := range p.Succs { if x != s { newSuccs = append(newSuccs, x) } }
    p.Succs = newSuccs
    f.addEdge(p, nb)

    // remove p from s.Preds
    var newPreds []*BasicBlock
    for _, x := range s.Preds { if x != p { newPreds = append(newPreds, x) } }
    s.Preds = newPreds
    f.addEdge(nb, s)
    return nb
}

func blockIndexOf(f *Function, b *BasicBlock) int {
    for i, bb := range f.Blocks { if bb == b { return i } }
    return -1
}

