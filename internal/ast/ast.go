package ast

type File struct {
    Decls []Decl
}

type Decl interface{ isDecl() }

type FuncDecl struct {
    Name string
    Params []Param
    Body *BlockStmt
    Ret  BasicType
}
func (*FuncDecl) isDecl() {}

type Param struct {
    Name string
    Typ  BasicType
    Ptr  bool
}

type Stmt interface{ isStmt() }

type BlockStmt struct { Stmts []Stmt }
func (*BlockStmt) isStmt() {}

type ReturnStmt struct { Expr Expr; Pos Pos }
func (*ReturnStmt) isStmt() {}

type ExprStmt struct { X Expr }
func (*ExprStmt) isStmt() {}

type DeclStmt struct { Name string; Init Expr; Typ BasicType; Ptr bool; Pos Pos; TypedefName string }
func (*DeclStmt) isStmt() {}

type ArrayDeclStmt struct { Name string; Size int; Elem BasicType }
func (*ArrayDeclStmt) isStmt() {}

type StructVarDeclStmt struct { Name string; StructType string }
func (*StructVarDeclStmt) isStmt() {}

type AssignStmt struct { Name string; Value Expr; Pos Pos }
func (*AssignStmt) isStmt() {}

type ArrayAssignStmt struct { Name string; Index Expr; Value Expr }
func (*ArrayAssignStmt) isStmt() {}

type FieldAssignStmt struct { Base string; Field string; Value Expr }
func (*FieldAssignStmt) isStmt() {}

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

type SwitchStmt struct {
    Tag Expr
    Cases []CaseClause
    Default *BlockStmt // may be nil
}
func (*SwitchStmt) isStmt() {}

type CaseClause struct {
    Values []int64 // case constants
    Body *BlockStmt
}

type Expr interface{ isExpr() }

type Ident struct { Name string }
func (*Ident) isExpr() {}

type IntLit struct { Value int64 }
func (*IntLit) isExpr() {}

type StringLit struct { Value string }
func (*StringLit) isExpr() {}

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
    OpLAnd
    OpLOr
    OpAnd
    OpOr
    OpXor
    OpShl
    OpShr
)

type CallExpr struct {
    Name string
    Args []Expr
}
func (*CallExpr) isExpr() {}

type UnOp int
const (
    OpAddr UnOp = iota
    OpDeref
    OpNeg
    OpBitNot
)

type UnaryExpr struct { Op UnOp; X Expr }
func (*UnaryExpr) isExpr() {}

type IndexExpr struct { Base Expr; Index Expr }
func (*IndexExpr) isExpr() {}

type CastExpr struct { To BasicType; Ptr bool; X Expr }
func (*CastExpr) isExpr() {}

type FieldExpr struct { Base Expr; Field string }
func (*FieldExpr) isExpr() {}

type GlobalDecl struct { Name string; Init *IntLit; Typ BasicType; Ptr bool }
func (*GlobalDecl) isDecl() {}

// GlobalArrayDecl represents a global array like: int g[N]; (zero-initialized)
type GlobalArrayDecl struct { Name string; Size int; Elem BasicType }
func (*GlobalArrayDecl) isDecl() {}

// StructDecl represents a struct definition: struct S { int x; int y; };
type StructDecl struct {
    Name   string
    Fields []StructField
}
func (*StructDecl) isDecl() {}

type StructField struct {
    Name string
    Typ  BasicType
    Ptr  bool
}

// EnumDecl represents an enum definition: enum E { A=1, B=2 };
type EnumDecl struct {
    Name   string
    Values []EnumValue
}
func (*EnumDecl) isDecl() {}

type EnumValue struct {
    Name  string
    Value int64
}

// TypedefDecl represents a typedef: typedef int i32;
type TypedefDecl struct {
    Name string
    Typ  BasicType
    Ptr  bool
}
func (*TypedefDecl) isDecl() {}

type BasicType int
const (
    BTInt BasicType = iota
    BTChar
)

type Pos struct { Line int; Col int }
