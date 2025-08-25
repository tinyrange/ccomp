package ir

import (
    "fmt"
    "github.com/tinyrange/cc/internal/ast"
)

type Module struct {
    Name string
    Funcs []*Function
}

func NewModule(name string) *Module { return &Module{Name: name} }

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
}

type ValueID int

type Value struct {
    ID   ValueID
    Op   Op
    Args []ValueID
    Const int64
}

type Op int
const (
    OpConst Op = iota
    OpAdd
    OpSub
    OpMul
    OpDiv
    OpRet
    OpStore
    OpLoad
    OpParam
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

// BuildModule creates basic SSA IR for Phase 1 (expressions, variables, return)
func BuildModule(file *ast.File, m *Module) error {
    for _, d := range file.Decls {
        fd, ok := d.(*ast.FuncDecl)
        if !ok { return fmt.Errorf("only functions supported in this phase") }
        f := &Function{Name: fd.Name}
        for _, p := range fd.Params { f.Params = append(f.Params, p.Name) }
        b := f.newBlock("entry")
        ctx := &buildCtx{f: f, b: b}
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
    curDef map[string]ValueID
}

func (c *buildCtx) initParams() {
    c.curDef = map[string]ValueID{}
    for i, name := range c.f.Params {
        id := c.newValue(OpParam, nil, 0)
        // record param in current defs map
        _ = i
        c.curDef[name] = id
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
                c.curDef[s.Name] = v
            } else {
                c.curDef[s.Name] = c.iconst(0)
            }
        case *ast.AssignStmt:
            v, err := c.buildExpr(s.Value)
            if err != nil { return err }
            c.curDef[s.Name] = v
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
        v, ok := c.curDef[e.Name]
        if !ok { return 0, fmt.Errorf("undefined variable %s", e.Name) }
        return v, nil
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
        }
    }
    return 0, fmt.Errorf("unsupported expr")
}

