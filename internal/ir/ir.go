package ir

import (
    "fmt"
    "github.com/tinyrange/cc/internal/ast"
    ty "github.com/tinyrange/cc/internal/types"
)

type Module struct {
    Name string
    Funcs []*Function
    Globals []Global
    StrLits []StrLit
}

func NewModule(name string) *Module { return &Module{Name: name} }

type Global struct {
    Name string
    Init int64
    Array bool
    Length int // number of elements if Array; element size is 8 for now
    ElemSize int
}

type StrLit struct {
    Name string // label
    Data string // bytes, zero-terminated when emitted
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
    OpLoad8
    OpParam
    OpAnd
    OpOr
    OpXor
    OpShl
    OpShr
    OpNot
    OpCopy // used during SSA destruction / phi elimination
    OpPhi  // phi nodes at start of a block; args aligned with Preds
    OpJmp  // unconditional jump; Args[0] holds target block index
    OpJnz  // conditional jump; Args[0]=cond, Args[1]=true blk idx, Args[2]=false blk idx
    OpCall // function call; Sym=callee, Args[] = arg value ids
    OpAddr // address-of local SSA slot; Args[0]=value id whose slot address to take
    OpGlobalAddr // address of global; Sym=name
    OpSlotAddr // address of a frame slot for SSA id; no materialize
    OpStore8
)

type Instr struct {
    Res ValueID // -1 if none
    Val Value
}

func (f *Function) newBlock(name string) *BasicBlock {
    // Ensure unique label names for codegen by appending index
    b := &BasicBlock{Name: fmt.Sprintf("%s_%d", name, len(f.Blocks))}
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
        switch gd := d.(type) {
        case *ast.GlobalDecl:
            init := int64(0)
            if gd.Init != nil { init = gd.Init.Value }
            globalType := ty.FromBasicType(int(gd.Typ), gd.Ptr)
            esz := globalType.Size()
            m.Globals = append(m.Globals, Global{Name: gd.Name, Init: init, ElemSize: esz})
        case *ast.GlobalArrayDecl:
            elemType := ty.FromBasicType(int(gd.Elem), false)
            esz := elemType.Size()
            m.Globals = append(m.Globals, Global{Name: gd.Name, Array: true, Length: gd.Size, ElemSize: esz})
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
        // Set param types from AST
        for _, p := range fd.Params {
            if p.Ptr {
                if p.Typ == ast.BTChar { ctx.varTypes[p.Name] = ty.PointerTo(ty.ByteT()) } else { ctx.varTypes[p.Name] = ty.PointerTo(ty.Int()) }
            } else {
                if p.Typ == ast.BTChar { ctx.varTypes[p.Name] = ty.ByteT() } else { ctx.varTypes[p.Name] = ty.Int() }
            }
        }
        // Set function return type
        if fd.Ret == ast.BTChar { ctx.retType = ty.ByteT() } else { ctx.retType = ty.Int() }
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
    arrays map[string]struct{ base ValueID; size int; elemSize int }
    // minimal type info
    varTypes map[string]ty.Type
    // interned string literal labels
    strLabels map[string]string
    retType ty.Type
}

func (c *buildCtx) initParams() {
    c.curDef = map[*BasicBlock]map[string]ValueID{}
    c.pending = map[*BasicBlock]map[string]ValueID{}
    c.arrays = map[string]struct{ base ValueID; size int; elemSize int }{}
    c.varTypes = map[string]ty.Type{}
    c.strLabels = map[string]string{}
    c.curDef[c.b] = map[string]ValueID{}
    for _, p := range c.f.Params {
        id := c.newValue(OpParam, nil, 0)
        c.writeVar(p, c.b, id)
        // default int for now; parser now carries types but Function.Params is []string only.
        // Keep int until function signature typing is added to IR.
        c.varTypes[p] = ty.Int()
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
            v, t, err := c.buildExprWithType(s.Expr)
            if err != nil { return err }
            // simple type check: disallow pointer returns for now
            if t.IsPointer() {
                return fmt.Errorf("%s:%d:%d: type error: returning pointer not supported", c.f.Name, s.Pos.Line, s.Pos.Col)
            }
            c.add(OpRet, v)
        case *ast.DeclStmt:
            if s.Init != nil {
                v, t, err := c.buildExprWithType(s.Init)
                if err != nil { return err }
                c.writeVar(s.Name, c.b, v)
                c.varTypes[s.Name] = t
            } else {
                c.writeVar(s.Name, c.b, c.iconst(0))
                if s.Ptr {
                    if s.Typ == ast.BTChar { c.varTypes[s.Name] = ty.PointerTo(ty.ByteT()) } else { c.varTypes[s.Name] = ty.PointerTo(ty.Int()) }
                } else {
                    if s.Typ == ast.BTChar { c.varTypes[s.Name] = ty.ByteT() } else { c.varTypes[s.Name] = ty.Int() }
                }
            }
        case *ast.AssignStmt:
            // If assigning to a global (and no local of same name), emit store to global
            if g, ok := c.lookupGlobal(s.Name); ok {
                if _, isLocal := c.varTypes[s.Name]; !isLocal {
                    val, _, err := c.buildExprWithType(s.Value)
                    if err != nil { return err }
                    addr := c.newValue(OpGlobalAddr, nil, 0)
                    c.b.Instrs[len(c.b.Instrs)-1].Val.Sym = g.Name
                    if g.ElemSize == 1 { c.add(OpStore8, addr, val) } else { c.add(OpStore, addr, val) }
                    break
                }
            }
            v, t, err := c.buildExprWithType(s.Value)
            if err != nil { return err }
            // simple type checks for locals: pointer vs non-pointer, char vs pointer
            if vt, ok := c.varTypes[s.Name]; ok {
                if vt.IsPointer() != t.IsPointer() {
                    return fmt.Errorf("%s:%d:%d: type error: cannot assign %s to %s", c.f.Name, s.Pos.Line, s.Pos.Col, typeStr(t), typeStr(vt))
                }
            }
            c.writeVar(s.Name, c.b, v)
            // update visible type
            c.varTypes[s.Name] = t
        case *ast.ArrayDeclStmt:
            // Reserve contiguous stack slots by creating 'size' SSA values
            var base ValueID = -1
            for i := 0; i < s.Size; i++ {
                id := c.iconst(0)
                if i == 0 { base = id }
            }
            elemType := ty.FromBasicType(int(s.Elem), false)
            esz := elemType.Size()
            c.arrays[s.Name] = struct{ base ValueID; size int; elemSize int }{base: base, size: s.Size, elemSize: esz}
        case *ast.ArrayAssignStmt:
            // Compute address base + index*8 and store value
            if arr, ok := c.arrays[s.Name]; ok {
                basePtr := c.add(OpSlotAddr, arr.base)
                idxVal, _, err := c.buildExprWithType(s.Index)
                if err != nil { return err }
                scale := c.iconst(int64(arr.elemSize))
                off := c.add(OpMul, idxVal, scale)
                ptr := c.add(OpAdd, basePtr, off)
                val, _, err := c.buildExprWithType(s.Value)
                if err != nil { return err }
                if arr.elemSize == 1 { c.add(OpStore8, ptr, val) } else { c.add(OpStore, ptr, val) }
                break
            }
            // global array
            if c.m != nil {
                for _, g := range c.m.Globals {
                    if g.Name == s.Name && g.Array {
                        base := c.newValue(OpGlobalAddr, nil, 0)
                        c.b.Instrs[len(c.b.Instrs)-1].Val.Sym = g.Name
                        idxVal, _, err := c.buildExprWithType(s.Index)
                        if err != nil { return err }
                        esz := 8
                        if g.Array && g.Length >= 0 && g.Name == s.Name {
                            // g is the target; ElemSize isn't stored yet; assume 8 for int for now
                        }
                        scale := c.iconst(int64(esz))
                        off := c.add(OpMul, idxVal, scale)
                        ptr := c.add(OpAdd, base, off)
                        val, _, err := c.buildExprWithType(s.Value)
                        if err != nil { return err }
                        // we don't track elem size on Global yet; default 8
                        c.add(OpStore, ptr, val)
                        break
                    }
                }
                // if not found, error
                // Note: fallthrough if found; otherwise return error
                found := false
                for _, g := range c.m.Globals { if g.Name == s.Name && g.Array { found = true; break } }
                if !found { return fmt.Errorf("unknown array %s", s.Name) }
            } else {
                return fmt.Errorf("unknown array %s", s.Name)
            }
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
            if _, _, err := c.buildExprWithType(s.X); err != nil { return err }
        case *ast.BlockStmt:
            if err := c.buildBlock(s); err != nil { return err }
        default:
            return fmt.Errorf("unsupported stmt type")
        }
    }
    return nil
}

// buildExpr is a backward-compatible wrapper that discards type info.
func (c *buildCtx) buildExpr(e ast.Expr) (ValueID, error) {
    v, _, err := c.buildExprWithType(e)
    return v, err
}

// buildExprWithType builds the expression and returns its SSA value and a minimal type.
func (c *buildCtx) buildExprWithType(e ast.Expr) (ValueID, ty.Type, error) {
    switch e := e.(type) {
    case *ast.IntLit:
        return c.iconst(e.Value), ty.Int(), nil
    case *ast.StringLit:
        // materialize string literal in module rodata and return its address
        lbl := c.internString(e.Value)
        id := c.newValue(OpGlobalAddr, nil, 0)
        c.b.Instrs[len(c.b.Instrs)-1].Val.Sym = lbl
        // type: pointer to byte
        return id, ty.PointerTo(ty.ByteT()), nil
    case *ast.Ident:
        if v, err := c.readVar(e.Name, c.b); err == nil {
            // obtain variable type if known; default int
            t := c.varTypes[e.Name]
            if t.K == 0 && !t.IsPointer() { t = ty.Int() }
            return v, t, nil
        }
        // fall back to global
        if c.m != nil {
            for _, g := range c.m.Globals {
                if g.Name == e.Name {
                    addr := c.newValue(OpGlobalAddr, nil, 0)
                    c.b.Instrs[len(c.b.Instrs)-1].Val.Sym = g.Name
                    if g.ElemSize == 1 { return c.add(OpLoad8, addr), ty.Int(), nil }
                    return c.add(OpLoad, addr), ty.Int(), nil
                }
            }
        }
        return 0, ty.Int(), fmt.Errorf("undefined variable %s", e.Name)
    case *ast.BinaryExpr:
        l, lt, err := c.buildExprWithType(e.Left)
        if err != nil { return 0, ty.Int(), err }
        r, rt, err := c.buildExprWithType(e.Right)
        if err != nil { return 0, ty.Int(), err }
        switch e.Op {
        case ast.OpAdd:
            // pointer-aware addition: ptr +/- int => scale by elem size
            if lt.IsPointer() && !rt.IsPointer() {
                sz := lt.ElemSize()
                if sz > 1 {
                    s := c.iconst(int64(sz))
                    r = c.add(OpMul, r, s)
                }
                return c.add(OpAdd, l, r), lt, nil
            }
            if rt.IsPointer() && !lt.IsPointer() {
                sz := rt.ElemSize()
                if sz > 1 {
                    s := c.iconst(int64(sz))
                    l = c.add(OpMul, l, s)
                }
                return c.add(OpAdd, l, r), rt, nil
            }
            return c.add(OpAdd, l, r), ty.Int(), nil
        case ast.OpSub:
            if lt.IsPointer() && !rt.IsPointer() {
                sz := lt.ElemSize()
                if sz > 1 {
                    s := c.iconst(int64(sz))
                    r = c.add(OpMul, r, s)
                }
                return c.add(OpSub, l, r), lt, nil
            }
            // ptr - ptr -> element count difference (byte diff / element size)
            if lt.IsPointer() && rt.IsPointer() {
                byteDiff := c.add(OpSub, l, r)
                sz := lt.ElemSize()
                if sz > 1 {
                    divisor := c.iconst(int64(sz))
                    return c.add(OpDiv, byteDiff, divisor), ty.Int(), nil
                }
                return byteDiff, ty.Int(), nil
            }
            return c.add(OpSub, l, r), ty.Int(), nil
        case ast.OpMul:
            return c.add(OpMul, l, r), ty.Int(), nil
        case ast.OpDiv:
            return c.add(OpDiv, l, r), ty.Int(), nil
        case ast.OpEq:
            return c.add(OpEq, l, r), ty.Int(), nil
        case ast.OpNe:
            return c.add(OpNe, l, r), ty.Int(), nil
        case ast.OpLt:
            return c.add(OpLt, l, r), ty.Int(), nil
        case ast.OpLe:
            return c.add(OpLe, l, r), ty.Int(), nil
        case ast.OpGt:
            return c.add(OpGt, l, r), ty.Int(), nil
        case ast.OpGe:
            return c.add(OpGe, l, r), ty.Int(), nil
        case ast.OpAnd:
            return c.add(OpAnd, l, r), ty.Int(), nil
        case ast.OpOr:
            return c.add(OpOr, l, r), ty.Int(), nil
        case ast.OpXor:
            return c.add(OpXor, l, r), ty.Int(), nil
        case ast.OpShl:
            return c.add(OpShl, l, r), ty.Int(), nil
        case ast.OpShr:
            return c.add(OpShr, l, r), ty.Int(), nil
        case ast.OpLAnd:
            v, err := c.buildLogical(true, e.Left, e.Right)
            return v, ty.Int(), err
        case ast.OpLOr:
            v, err := c.buildLogical(false, e.Left, e.Right)
            return v, ty.Int(), err
        }
    case *ast.CallExpr:
        // Evaluate args
        var argv []ValueID
        for _, a := range e.Args {
            v, _, err := c.buildExprWithType(a)
            if err != nil { return 0, ty.Int(), err }
            argv = append(argv, v)
        }
        id := c.newValue(OpCall, argv, 0)
        // attach callee symbol
        // patch the last inserted instruction's Sym
        c.b.Instrs[len(c.b.Instrs)-1].Val.Sym = e.Name
        return id, ty.Int(), nil
    case *ast.IndexExpr:
        // Local named array
        if b, ok := e.Base.(*ast.Ident); ok {
            if arr, ok := c.arrays[b.Name]; ok {
                basePtr := c.add(OpSlotAddr, arr.base)
                idxVal, _, err := c.buildExprWithType(e.Index)
                if err != nil { return 0, ty.Int(), err }
                scale := c.iconst(int64(arr.elemSize))
                off := c.add(OpMul, idxVal, scale)
                ptr := c.add(OpAdd, basePtr, off)
                if arr.elemSize == 1 { return c.add(OpLoad8, ptr), ty.Int(), nil }
                return c.add(OpLoad, ptr), ty.Int(), nil
            }
            // try global array
            if c.m != nil {
                for _, g := range c.m.Globals {
                    if g.Name == b.Name && g.Array {
                        addr := c.newValue(OpGlobalAddr, nil, 0)
                        c.b.Instrs[len(c.b.Instrs)-1].Val.Sym = g.Name
                        idxVal, _, err := c.buildExprWithType(e.Index)
                        if err != nil { return 0, ty.Int(), err }
                        scale := c.iconst(int64(g.ElemSize))
                        off := c.add(OpMul, idxVal, scale)
                        ptr := c.add(OpAdd, addr, off)
                        if g.ElemSize == 1 { return c.add(OpLoad8, ptr), ty.Int(), nil }
                        return c.add(OpLoad, ptr), ty.Int(), nil
                    }
                }
            }
            // fallthrough to generic pointer indexing on unknown ident
        }
        // Generic pointer indexing: base must be pointer
        base, bt, err := c.buildExprWithType(e.Base)
        if err != nil { return 0, ty.Int(), err }
        idxVal, _, err := c.buildExprWithType(e.Index)
        if err != nil { return 0, ty.Int(), err }
        sz := 1
        if bt.IsPointer() { sz = bt.ElemSize() }
        scale := c.iconst(int64(sz))
        off := c.add(OpMul, idxVal, scale)
        ptr := c.add(OpAdd, base, off)
        if sz == 1 { return c.add(OpLoad8, ptr), ty.ByteT(), nil }
        return c.add(OpLoad, ptr), ty.Int(), nil
    case *ast.UnaryExpr:
        switch e.Op {
        case ast.OpAddr:
            if idn, ok := e.X.(*ast.Ident); ok {
                v, err := c.readVar(idn.Name, c.b)
                if err != nil { return 0, ty.Int(), err }
                // pointer to whatever the variable is (default int)
                bt := c.varTypes[idn.Name]
                if bt.K == 0 && !bt.IsPointer() { bt = ty.Int() }
                return c.add(OpAddr, v), ty.PointerTo(bt), nil
            }
            return 0, ty.Int(), fmt.Errorf("address-of unsupported operand")
        case ast.OpDeref:
            ptr, pt, err := c.buildExprWithType(e.X)
            if err != nil { return 0, ty.Int(), err }
            // result type is pointee if known
            rt := ty.Int()
            if pt.IsPointer() && pt.Elem != nil { rt = *pt.Elem }
            if rt.Size() == 1 {
                return c.add(OpLoad8, ptr), rt, nil
            }
            return c.add(OpLoad, ptr), rt, nil
        case ast.OpNeg:
            x, _, err := c.buildExprWithType(e.X)
            if err != nil { return 0, ty.Int(), err }
            return c.add(OpSub, c.iconst(0), x), ty.Int(), nil
        case ast.OpBitNot:
            x, _, err := c.buildExprWithType(e.X)
            if err != nil { return 0, ty.Int(), err }
            return c.add(OpNot, x), ty.Int(), nil
        }
    case *ast.CastExpr:
        v, st, err := c.buildExprWithType(e.X)
        if err != nil { return 0, ty.Int(), err }
        // Build target type
        var tt ty.Type
        if e.Ptr {
            if e.To == ast.BTChar { tt = ty.PointerTo(ty.ByteT()) } else { tt = ty.PointerTo(ty.Int()) }
        } else {
            if e.To == ast.BTChar { tt = ty.ByteT() } else { tt = ty.Int() }
        }
        // For now, casts are mostly no-ops; if narrowing to char, mask to 0xFF
        if !tt.IsPointer() && st.IsPointer() {
            // pointer to int: no-op
            return v, tt, nil
        }
        if tt.IsPointer() && !st.IsPointer() {
            // int to pointer: no-op
            return v, tt, nil
        }
        if !tt.IsPointer() && !st.IsPointer() {
            if tt.Size() == 1 {
                // mask low byte
                m := c.iconst(0xFF)
                return c.add(OpAnd, v, m), tt, nil
            }
            return v, tt, nil
        }
        // pointer to pointer
        return v, tt, nil
    }
    return 0, ty.Int(), fmt.Errorf("unsupported expr")
}

func (c *buildCtx) internString(s string) string {
    if lbl, ok := c.strLabels[s]; ok { return lbl }
    lbl := fmt.Sprintf(".Lstr%d", len(c.m.StrLits))
    c.strLabels[s] = lbl
    c.m.StrLits = append(c.m.StrLits, StrLit{Name: lbl, Data: s})
    return lbl
}

func typeStr(t ty.Type) string {
    if t.IsPointer() {
        return "pointer"
    }
    switch t.K {
    case ty.Int64:
        return "int"
    case ty.Byte:
        return "char"
    default:
        return "unknown"
    }
}

func (c *buildCtx) buildLogical(isAnd bool, left, right ast.Expr) (ValueID, error) {
    // Evaluate left
    l, err := c.buildExpr(left)
    if err != nil { return 0, err }
    f := c.f
    rightB := f.newBlock("log.right")
    endB := f.newBlock("log.end")
    // Prepare result temp var
    tmp := fmt.Sprintf("$t%d", c.nextID)
    if isAnd {
        // default false
        c.writeVar(tmp, c.b, c.iconst(0))
        // if l!=0 -> right, else -> end
        ri := blockIndexOf(f, rightB)
        ei := blockIndexOf(f, endB)
        c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJnz, Args: []ValueID{l, ValueID(ri), ValueID(ei)}}})
        f.addEdge(c.b, rightB)
        f.addEdge(c.b, endB)
        // right path
        c.b = rightB
        r, err := c.buildExpr(right)
        if err != nil { return 0, err }
        one := c.iconst(0)
        one = c.add(OpNe, r, one)
        c.writeVar(tmp, c.b, one)
        // jump to end
        ei2 := blockIndexOf(f, endB)
        c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(ei2)}}})
        f.addEdge(c.b, endB)
    } else {
        // OR: default true
        c.writeVar(tmp, c.b, c.iconst(1))
        // if l!=0 -> end, else -> right
        ri := blockIndexOf(f, rightB)
        ei := blockIndexOf(f, endB)
        // Need cond != 0
        // Use l directly for jnz (nonzero true)
        c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJnz, Args: []ValueID{l, ValueID(ei), ValueID(ri)}}})
        f.addEdge(c.b, endB)
        f.addEdge(c.b, rightB)
        // right path
        c.b = rightB
        r, err := c.buildExpr(right)
        if err != nil { return 0, err }
        one := c.iconst(0)
        one = c.add(OpNe, r, one)
        c.writeVar(tmp, c.b, one)
        // jump to end
        ei2 := blockIndexOf(f, endB)
        c.b.Instrs = append(c.b.Instrs, Instr{Res: -1, Val: Value{Op: OpJmp, Args: []ValueID{ValueID(ei2)}}})
        f.addEdge(c.b, endB)
    }
    // seal end and read result
    c.sealBlock(endB)
    c.b = endB
    v, err := c.readVar(tmp, endB)
    if err != nil { return 0, err }
    return v, nil
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
func (c *buildCtx) lookupGlobal(name string) (*Global, bool) {
    if c.m == nil { return nil, false }
    for i := range c.m.Globals {
        if c.m.Globals[i].Name == name { return &c.m.Globals[i], true }
    }
    return nil, false
}
