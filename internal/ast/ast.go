package ast

type File struct {
    Decls []Decl
}

type Decl interface{ isDecl() }

type FuncDecl struct {
    Name string
    Params []Param
    Body *BlockStmt
}
func (*FuncDecl) isDecl() {}

type Param struct {
    Name string
}

type Stmt interface{ isStmt() }

type BlockStmt struct { Stmts []Stmt }
func (*BlockStmt) isStmt() {}

type ReturnStmt struct { Expr Expr }
func (*ReturnStmt) isStmt() {}

type ExprStmt struct { X Expr }
func (*ExprStmt) isStmt() {}

type DeclStmt struct { Name string; Init Expr }
func (*DeclStmt) isStmt() {}

type AssignStmt struct { Name string; Value Expr }
func (*AssignStmt) isStmt() {}

type IfStmt struct {
    Cond Expr
    Then *BlockStmt
    Else *BlockStmt
}
func (*IfStmt) isStmt() {}

type WhileStmt struct {
    Cond Expr
    Body *BlockStmt
}
func (*WhileStmt) isStmt() {}

type ForStmt struct {
    Init Stmt // may be nil
    Cond Expr // may be nil (treated as true)
    Post Stmt // may be nil
    Body *BlockStmt
}
func (*ForStmt) isStmt() {}

type DoWhileStmt struct {
    Body *BlockStmt
    Cond Expr
}
func (*DoWhileStmt) isStmt() {}

type BreakStmt struct{}
func (*BreakStmt) isStmt() {}

type ContinueStmt struct{}
func (*ContinueStmt) isStmt() {}

type Expr interface{ isExpr() }

type Ident struct { Name string }
func (*Ident) isExpr() {}

type IntLit struct { Value int64 }
func (*IntLit) isExpr() {}

type BinaryExpr struct { Op BinOp; Left, Right Expr }
func (*BinaryExpr) isExpr() {}

type BinOp int
const (
    OpAdd BinOp = iota
    OpSub
    OpMul
    OpDiv
    OpEq
    OpNe
    OpLt
    OpLe
    OpGt
    OpGe
)
