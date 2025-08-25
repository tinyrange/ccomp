package ir

import (
    "fmt"
    "github.com/tinyrange/cc/internal/ast"
)

type Module struct {
    Name string
    Funcs []*Function
    Globals []Global
}

func NewModule(name string) *Module { return &Module{Name: name} }

type Global struct {
    Name string
    Init int64
}

type Function struct {
    Name string
    Params []string
    Blocks []*BasicBlock
    entry *BasicBlock
}

type BasicBlock struct {
    Name string
    Instrs []Instr
    sealed bool // placeholder for Braun's algorithm
    Preds []*BasicBlock
    Succs []*BasicBlock
}

func (b *BasicBlock) terminated() bool {
    if len(b.Instrs) == 0 { return false }
    op := b.Instrs[len(b.Instrs)-1].Val.Op
    return op == OpJmp || op == OpJnz || op == OpRet
}

type ValueID int

type Value struct {
    ID   ValueID
    Op   Op
    Args []ValueID
    Const int64
    Sym string
}

type Op int
const (
    OpConst Op = iota
    OpAdd
    OpSub
    OpMul
    OpDiv
    // comparisons produce 0/1
    OpEq
    OpNe
    OpLt
    OpLe
    OpGt
    OpGe
    OpRet
    OpStore
    OpLoad
    OpParam
    OpCopy // used during SSA destruction / phi elimination
    OpPhi  // phi nodes at start of a block; args aligned with Preds
    OpJmp  // unconditional jump; Args[0] holds target block index
    OpJnz  // conditional jump; Args[0]=cond, Args[1]=true blk idx, Args[2]=false blk idx
    OpCall // function call; Sym=callee, Args[] = arg value ids
    OpAddr // address-of local SSA slot; Args[0]=value id whose slot address to take
    OpGlobalAddr // address of global; Sym=name
)

type Instr struct {
    Res ValueID // -1 if none
    Val Value
}

func (f *Function) newBlock(name string) *BasicBlock {
    b := &BasicBlock{Name: name}
    f.Blocks = append(f.Blocks, b)
    if f.entry == nil { f.entry = b }
    return b
}

func (f *Function) addEdge(pred, succ *BasicBlock) {
    pred.Succs = append(pred.Succs, succ)
    succ.Preds = append(succ.Preds, pred)
}

// BuildModule creates basic SSA IR for Phase 1 (expressions, variables, return)
func BuildModule(file *ast.File, m *Module) error {
    // First collect globals
    for _, d := range file.Decls {
        if gd, ok := d.(*ast.GlobalDecl); ok {
            init := int64(0)
            if gd.Init != nil { init = gd.Init.Value }
            m.Globals = append(m.Globals, Global{Name: gd.Name, Init: init})
        }
    }
    // Then build functions
    for _, d := range file.Decls {
        fd, ok := d.(*ast.FuncDecl)
        if !ok { continue }
        f := &Function{Name: fd.Name}
        for _, p := range fd.Params { f.Params = append(f.Params, p.Name) }
        b := f.newBlock("entry")
        ctx := &buildCtx{f: f, b: b, m: m}
        ctx.initParams()
        if err := ctx.buildBlock(fd.Body); err != nil { return err }
        m.Funcs = append(m.Funcs, f)
    }
    return nil
}

type buildCtx struct {
    f *Function
    b *BasicBlock
    nextID ValueID
    curDef map[*BasicBlock]map[string]ValueID
    pending map[*BasicBlock]map[string]ValueID // name -> phi id to fill on seal
    breakTargets []*BasicBlock
    contTargets  []*BasicBlock
    m *Module
}

func (c *buildCtx) initParams() {
    c.curDef = map[*BasicBlock]map[string]ValueID{}
    c.pending = map[*BasicBlock]map[string]ValueID{}
    c.curDef[c.b] = map[string]ValueID{}
    for _, name := range c.f.Params {
        id := c.newValue(OpParam, nil, 0)
        c.writeVar(name, c.b, id)
    }
}

func (c *buildCtx) newValue(op Op, args []ValueID, k int64) ValueID {
    id := c.nextID
    c.nextID++
    v := Value{ID: id, Op: op, Args: append([]ValueID(nil), args...), Const: k}
    instr := Instr{Res: id, Val: v}
    c.b.Instrs = append(c.b.Instrs, instr)
    return id
}

func (c *buildCtx) add(op Op, args ...ValueID) ValueID { return c.newValue(op, args, 0) }
func (c *buildCtx) iconst(v int64) ValueID { return c.newValue(OpConst, nil, v) }

func (c *buildCtx) writeVar(name string, blk *BasicBlock, id ValueID) {
    if c.curDef[blk] == nil { c.curDef[blk] = map[string]ValueID{} }
    c.curDef[blk][name] = id
}

func (c *buildCtx) readVar(name string, blk *BasicBlock) (ValueID, error) {
    if m := c.curDef[blk]; m != nil {
        if v, ok := m[name]; ok { return v, nil }
    }
    if !blk.sealed {
        // Unsealed: if single predecessor, read from it; otherwise create placeholder phi.
        switch len(blk.Preds) {
        case 0:
            return 0, fmt.Errorf("undefined variable %s", name)
        case 1:
            return c.readVar(name, blk.Preds[0])
        default:
            phi := c.newPhi(blk)
            c.writeVar(name, blk, phi)
            if c.pending[blk] == nil { c.pending[blk] = map[string]ValueID{} }
            c.pending[blk][name] = phi
            return phi, nil
        }
    }
    if len(blk.Preds) == 0 {
        return 0, fmt.Errorf("undefined variable %s", name)
    } else if len(blk.Preds) == 1 {
        return c.readVar(name, blk.Preds[0])
    }
    // multiple predecessors: create phi
    phi := c.newPhi(blk)
    c.writeVar(name, blk, phi)
    if blk.sealed {
        // fill operands now
        c.addPhiOperands(blk, phi, name)
    }
    return phi, nil
}

func (c *buildCtx) newPhi(blk *BasicBlock) ValueID {
    id := c.nextID
    c.nextID++
    v := Value{ID: id, Op: OpPhi}
    ins := Instr{Res: id, Val: v}
    // insert at block start
    blk.Instrs = append([]Instr{ins}, blk.Instrs...)
    return id
}

func (c *buildCtx) addPhiOperands(blk *BasicBlock, phi ValueID, name string) {
    // locate phi instr
    for i := range blk.Instrs {
        if blk.Instrs[i].Res == phi && blk.Instrs[i].Val.Op == OpPhi {
            var args []ValueID
            for _, p := range blk.Preds {
                v, _ := c.readVar(name, p)
                args = append(args, v)
            }
            blk.Instrs[i].Val.Args = args
            return
        }
    }
}

func (c *buildCtx) sealBlock(blk *BasicBlock) {
    if blk.sealed { return }
    blk.sealed = true
    pend := c.pending[blk]
    for name, phi := range pend {
        c.addPhiOperands(blk, phi, name)
    }
    delete(c.pending, blk)
}

func (c *buildCtx) buildBlock(b *ast.BlockStmt) error {
    for _, s := range b.Stmts {
        switch s := s.(type) {
        case *ast.ReturnStmt:
            v, err := c.buildExpr(s.Expr)
            if err != nil { return err }
            c.add(OpRet, v)
        case *ast.DeclStmt:
            if s.Init != nil {
                v, err := c.buildExpr(s.Init)
                if err != nil { return err }
                c.writeVar(s.Name, c.b, v)
            } else {
                c.writeVar(s.Name, c.b, c.iconst(0))
            }
        case *ast.AssignStmt:
            v, err := c.buildExpr(s.Value)
            if err != nil { return err }
            c.writeVar(s.Name, c.b, v)
        case *ast.IfStmt:
            if err := c.buildIf(s); err != nil { return err }
        case *ast.WhileStmt:
            if err := c.buildWhile(s); err != nil { return err }
        case *ast.ForStmt:
            if err := c.buildFor(s); err != nil { return err }
        case *ast.DoWhileStmt:
            if err := c.buildDoWhile(s); err != nil { return err }
        case *ast.BreakStmt:
            if len(c.breakTargets) == 0 { return fmt.Errorf("break outside loop") }
            t := c.breakTargets[len(c.breakTargets)-1]
            ti := blockIndexOf(c.f, t)
            c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(ti)}}})
            c.f.addEdge(c.b, t)
        case *ast.ContinueStmt:
            if len(c.contTargets) == 0 { return fmt.Errorf("continue outside loop") }
            t := c.contTargets[len(c.contTargets)-1]
            ti := blockIndexOf(c.f, t)
            c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(ti)}}})
            c.f.addEdge(c.b, t)
        case *ast.SwitchStmt:
            if err := c.buildSwitch(s); err != nil { return err }
        case *ast.ExprStmt:
            if _, err := c.buildExpr(s.X); err != nil { return err }
        case *ast.BlockStmt:
            if err := c.buildBlock(s); err != nil { return err }
        default:
            return fmt.Errorf("unsupported stmt type")
        }
    }
    return nil
}

func (c *buildCtx) buildExpr(e ast.Expr) (ValueID, error) {
    switch e := e.(type) {
    case *ast.IntLit:
        return c.iconst(e.Value), nil
    case *ast.Ident:
        if v, err := c.readVar(e.Name, c.b); err == nil {
            return v, nil
        }
        // fall back to global
        if c.m != nil {
            for _, g := range c.m.Globals {
                if g.Name == e.Name {
                    addr := c.newValue(OpGlobalAddr, nil, 0)
                    c.b.Instrs[len(c.b.Instrs)-1].Val.Sym = g.Name
                    return c.add(OpLoad, addr), nil
                }
            }
        }
        return 0, fmt.Errorf("undefined variable %s", e.Name)
    case *ast.BinaryExpr:
        l, err := c.buildExpr(e.Left)
        if err != nil { return 0, err }
        r, err := c.buildExpr(e.Right)
        if err != nil { return 0, err }
        switch e.Op {
        case ast.OpAdd:
            return c.add(OpAdd, l, r), nil
        case ast.OpSub:
            return c.add(OpSub, l, r), nil
        case ast.OpMul:
            return c.add(OpMul, l, r), nil
        case ast.OpDiv:
            return c.add(OpDiv, l, r), nil
        case ast.OpEq:
            return c.add(OpEq, l, r), nil
        case ast.OpNe:
            return c.add(OpNe, l, r), nil
        case ast.OpLt:
            return c.add(OpLt, l, r), nil
        case ast.OpLe:
            return c.add(OpLe, l, r), nil
        case ast.OpGt:
            return c.add(OpGt, l, r), nil
        case ast.OpGe:
            return c.add(OpGe, l, r), nil
        }
    case *ast.CallExpr:
        // Evaluate args
        var argv []ValueID
        for _, a := range e.Args {
            v, err := c.buildExpr(a)
            if err != nil { return 0, err }
            argv = append(argv, v)
        }
        id := c.newValue(OpCall, argv, 0)
        // attach callee symbol
        // patch the last inserted instruction's Sym
        c.b.Instrs[len(c.b.Instrs)-1].Val.Sym = e.Name
        return id, nil
    case *ast.UnaryExpr:
        switch e.Op {
        case ast.OpAddr:
            if idn, ok := e.X.(*ast.Ident); ok {
                v, err := c.readVar(idn.Name, c.b)
                if err != nil { return 0, err }
                return c.add(OpAddr, v), nil
            }
            return 0, fmt.Errorf("address-of unsupported operand")
        case ast.OpDeref:
            ptr, err := c.buildExpr(e.X)
            if err != nil { return 0, err }
            return c.add(OpLoad, ptr), nil
        }
    }
    return 0, fmt.Errorf("unsupported expr")
}

func (c *buildCtx) buildIf(s *ast.IfStmt) error {
    cond, err := c.buildExpr(s.Cond)
    if err != nil { return err }
    f := c.f
    thenB := f.newBlock("then")
    elseB := f.newBlock("else")
    joinB := f.newBlock("endif")
    // current block branches to then/else
    tIdx := blockIndexOf(f, thenB)
    eIdx := blockIndexOf(f, elseB)
    c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJnz, Args: []ValueID{cond, ValueID(tIdx), ValueID(eIdx)}}})
    f.addEdge(c.b, thenB)
    f.addEdge(c.b, elseB)
    // build then
    c.b = thenB
    if err := c.buildBlock(s.Then); err != nil { return err }
    // jump to join
    jIdx := blockIndexOf(f, joinB)
    if !c.b.terminated() {
        c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(jIdx)}}})
        f.addEdge(c.b, joinB)
    }
    thenEnd := c.b
    // build else
    c.b = elseB
    if s.Else != nil { if err := c.buildBlock(s.Else); err != nil { return err } }
    if !c.b.terminated() {
        c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(jIdx)}}})
        f.addEdge(c.b, joinB)
    }
    elseEnd := c.b
    // seal join and move current block
    c.sealBlock(joinB)
    c.b = joinB
    _ = thenEnd; _ = elseEnd
    return nil
}

func (c *buildCtx) buildWhile(s *ast.WhileStmt) error {
    f := c.f
    condB := f.newBlock("while.cond")
    bodyB := f.newBlock("while.body")
    exitB := f.newBlock("while.end")
    // jump to cond
    ci := blockIndexOf(f, condB)
    c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(ci)}}})
    f.addEdge(c.b, condB)
    // Predeclare backedge in CFG so cond reads create phis
    f.addEdge(bodyB, condB)
    // build cond
    c.b = condB
    cond, err := c.buildExpr(s.Cond)
    if err != nil { return err }
    bi := blockIndexOf(f, bodyB)
    ei := blockIndexOf(f, exitB)
    c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJnz, Args: []ValueID{cond, ValueID(bi), ValueID(ei)}}})
    f.addEdge(c.b, bodyB)
    f.addEdge(c.b, exitB)
    // body
    c.b = bodyB
    // push loop context
    c.breakTargets = append(c.breakTargets, exitB)
    c.contTargets = append(c.contTargets, condB)
    if err := c.buildBlock(s.Body); err != nil { return err }
    // pop loop context
    c.breakTargets = c.breakTargets[:len(c.breakTargets)-1]
    c.contTargets = c.contTargets[:len(c.contTargets)-1]
    // Emit backedge jump from the current block (could be a join inside body)
    endB := c.b
    if !endB.terminated() {
        endB.Instrs = append(endB.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(ci)}}})
    }
    // Fix CFG: redirect predeclared body->cond to actual endB->cond if different
    if endB != bodyB {
        // remove bodyB -> condB
        // bodyB.Succs
        var ns []*BasicBlock
        for _, s := range bodyB.Succs { if s != condB { ns = append(ns, s) } }
        bodyB.Succs = ns
        // condB.Preds
        var np []*BasicBlock
        for _, p := range condB.Preds { if p != bodyB { np = append(np, p) } }
        condB.Preds = np
        // add endB -> condB
        f.addEdge(endB, condB)
    }
    // continue at exit
    c.b = exitB
    // Seal header now that backedge exists; fill any pending phis
    c.sealBlock(condB)
    c.sealBlock(exitB)
    return nil
}

func (c *buildCtx) buildFor(s *ast.ForStmt) error {
    f := c.f
    // handle init in current block
    if s.Init != nil {
        if err := c.buildBlock(&ast.BlockStmt{Stmts: []ast.Stmt{s.Init}}); err != nil { return err }
    }
    condB := f.newBlock("for.cond")
    bodyB := f.newBlock("for.body")
    postB := f.newBlock("for.post")
    exitB := f.newBlock("for.end")
    // jump to cond
    ci := blockIndexOf(f, condB)
    c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(ci)}}})
    f.addEdge(c.b, condB)
    // Predeclare backedges to cond for SSA
    f.addEdge(bodyB, condB)
    f.addEdge(postB, condB)
    // build cond
    c.b = condB
    if s.Cond != nil {
        cond, err := c.buildExpr(s.Cond)
        if err != nil { return err }
        bi := blockIndexOf(f, bodyB)
        ei := blockIndexOf(f, exitB)
        c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJnz, Args: []ValueID{cond, ValueID(bi), ValueID(ei)}}})
        f.addEdge(c.b, bodyB)
        f.addEdge(c.b, exitB)
    } else {
        // no cond => always true
        bi := blockIndexOf(f, bodyB)
        c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(bi)}}})
        f.addEdge(c.b, bodyB)
    }
    // body
    c.b = bodyB
    // loop context: continue -> post (if any) else cond
    cont := postB
    if s.Post == nil { cont = condB }
    c.breakTargets = append(c.breakTargets, exitB)
    c.contTargets = append(c.contTargets, cont)
    if err := c.buildBlock(s.Body); err != nil { return err }
    c.breakTargets = c.breakTargets[:len(c.breakTargets)-1]
    c.contTargets = c.contTargets[:len(c.contTargets)-1]
    // jump to post/cond
    if s.Post != nil {
        pi := blockIndexOf(f, postB)
        if !c.b.terminated() {
            c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(pi)}}})
            f.addEdge(c.b, postB)
        }
        // post
        c.b = postB
        if err := c.buildBlock(&ast.BlockStmt{Stmts: []ast.Stmt{s.Post}}); err != nil { return err }
        // back to cond
        if !c.b.terminated() {
            c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(ci)}}})
            f.addEdge(c.b, condB)
        }
    } else {
        // no post: jump directly to cond
        if !c.b.terminated() {
            c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(ci)}}})
            f.addEdge(c.b, condB)
        }
    }
    // continue at exit
    c.b = exitB
    // Seal header and exit
    c.sealBlock(condB)
    c.sealBlock(exitB)
    return nil
}

func (c *buildCtx) buildDoWhile(s *ast.DoWhileStmt) error {
    f := c.f
    headB := f.newBlock("do.head")
    bodyB := f.newBlock("do.body")
    condB := f.newBlock("do.cond")
    exitB := f.newBlock("do.end")
    // jump to header first
    hi := blockIndexOf(f, headB)
    c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(hi)}}})
    f.addEdge(c.b, headB)
    // Predeclare backedge to header so reads in header can create phis
    f.addEdge(condB, headB)
    // header falls through to body
    c.b = headB
    bi := blockIndexOf(f, bodyB)
    c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(bi)}}})
    f.addEdge(c.b, bodyB)
    // body
    c.b = bodyB
    c.breakTargets = append(c.breakTargets, exitB)
    c.contTargets = append(c.contTargets, condB)
    if err := c.buildBlock(s.Body); err != nil { return err }
    c.breakTargets = c.breakTargets[:len(c.breakTargets)-1]
    c.contTargets = c.contTargets[:len(c.contTargets)-1]
    // jump to cond
    ci := blockIndexOf(f, condB)
    if !c.b.terminated() {
        c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(ci)}}})
        f.addEdge(c.b, condB)
    }
    // cond
    c.b = condB
    cond, err := c.buildExpr(s.Cond)
    if err != nil { return err }
    // branch: true -> head (already predeclared), false -> exit
    hi2 := blockIndexOf(f, headB)
    ei := blockIndexOf(f, exitB)
    c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJnz, Args: []ValueID{cond, ValueID(hi2), ValueID(ei)}}})
    // Do not add head edge here to avoid duplicate; exit edge is new
    f.addEdge(c.b, exitB)
    // Now preds of header are entry and cond; seal to fill phis
    c.sealBlock(headB)
    // continue at exit
    c.b = exitB
    c.sealBlock(condB)
    c.sealBlock(exitB)
    return nil
}

func (c *buildCtx) buildSwitch(s *ast.SwitchStmt) error {
    f := c.f
    // Evaluate tag
    tag, err := c.buildExpr(s.Tag)
    if err != nil { return err }
    exitB := f.newBlock("switch.end")
    // Create blocks for cases
    caseBlocks := make([]*BasicBlock, len(s.Cases))
    for i := range s.Cases { caseBlocks[i] = f.newBlock(fmt.Sprintf("case.%d", i)) }
    var defaultB *BasicBlock
    if s.Default != nil { defaultB = f.newBlock("default") }
    // Build compare chain
    // Start from a dispatch block (current)
    nextB := defaultB
    if nextB == nil { nextB = exitB }
    // We'll create a sequence of cmp blocks in reverse to chain else branches
    for i := len(s.Cases) - 1; i >= 0; i-- {
        cmpB := f.newBlock(fmt.Sprintf("sw.cmp.%d", i))
        // jump from current to first cmp if building first, else link previous
        if i == len(s.Cases)-1 {
            // from current block to cmpB
            ci := blockIndexOf(f, cmpB)
            c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(ci)}}})
            f.addEdge(c.b, cmpB)
        }
        // In cmpB, compare tag equals any of the case values (chain OR inside the block)
        c.b = cmpB
        var fall *BasicBlock = nextB
        // For each value in this case
        for vi, v := range s.Cases[i].Values {
            tblock := caseBlocks[i]
            // build compare
            cv := c.iconst(v)
            cond := c.add(OpEq, tag, cv)
            ti := blockIndexOf(f, tblock)
            fi := -1
            // false target is either next comparison within this same case-values list or the overall nextB
            if vi == len(s.Cases[i].Values)-1 {
                fi = blockIndexOf(f, fall)
            } else {
                // create an inner cmp block for next value
                inner := f.newBlock(fmt.Sprintf("sw.cmp.%d.%d", i, vi))
                fi = blockIndexOf(f, inner)
                fall = inner
            }
            c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJnz, Args: []ValueID{cond, ValueID(ti), ValueID(fi)}}})
            f.addEdge(c.b, tblock)
            f.addEdge(c.b, f.Blocks[fi])
            // move to inner for next value if any
            if vi < len(s.Cases[i].Values)-1 {
                c.b = f.Blocks[fi]
            }
        }
        nextB = cmpB
    }
    // At this point, control reaches nextB to start comparisons; we already linked entry to first cmp
    // Build case bodies
    // Push break target
    c.breakTargets = append(c.breakTargets, exitB)
    for i, cc := range s.Cases {
        c.b = caseBlocks[i]
        if err := c.buildBlock(cc.Body); err != nil { return err }
        // If body not terminated, fall through to next case or default/exit
        if !c.b.terminated() {
            var ft *BasicBlock
            if i+1 < len(caseBlocks) {
                ft = caseBlocks[i+1]
            } else if defaultB != nil {
                ft = defaultB
            } else {
                ft = exitB
            }
            fi := blockIndexOf(f, ft)
            c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(fi)}}})
            f.addEdge(c.b, ft)
        }
    }
    // default body
    if defaultB != nil {
        c.b = defaultB
        if err := c.buildBlock(s.Default); err != nil { return err }
        if !c.b.terminated() {
            ei := blockIndexOf(f, exitB)
            c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(ei)}}})
            f.addEdge(c.b, exitB)
        }
    }
    // pop break
    c.breakTargets = c.breakTargets[:len(c.breakTargets)-1]
    // continue at exit
    c.b = exitB
    c.sealBlock(exitB)
    return nil
}
