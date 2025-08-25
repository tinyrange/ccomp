package parser

import (
    "fmt"
    "strconv"

    "github.com/tinyrange/cc/internal/ast"
    "github.com/tinyrange/cc/internal/lexer"
)

type Parser struct {
    lx  *lexer.Lexer
    tok lexer.Token
}

func ParseFile(filename, src string) (*ast.File, error) {
    p := &Parser{lx: lexer.New(src)}
    p.next()
    f := &ast.File{}
    for p.tok.Type != lexer.EOF {
        d, err := p.parseDecl()
        if err != nil { return nil, err }
        f.Decls = append(f.Decls, d)
    }
    return f, nil
}

func (p *Parser) next() { p.tok = p.lx.Next() }

func (p *Parser) expect(tt lexer.TokenType) (lexer.Token, error) {
    if p.tok.Type != tt {
        return lexer.Token{}, fmt.Errorf("expected %v, got %v at %d:%d", tt, p.tok.Type, p.tok.Line, p.tok.Col)
    }
    t := p.tok
    p.next()
    return t, nil
}

func (p *Parser) parseDecl() (ast.Decl, error) {
    // Either: int IDENT(params) { ... }  OR  int [*]* IDENT [= INT] ;  (global)
    if p.tok.Type != lexer.KW_INT {
        return nil, fmt.Errorf("only 'int' functions supported at %d:%d", p.tok.Line, p.tok.Col)
    }
    p.next()
    // optional pointer stars for global decls (ignored for now)
    for p.tok.Type == lexer.STAR { p.next() }
    nameTok, err := p.expect(lexer.IDENT)
    if err != nil { return nil, err }
    if p.tok.Type == lexer.LPAREN {
        // function
        p.next()
        params, err := p.parseParams()
        if err != nil { return nil, err }
        if _, err = p.expect(lexer.RPAREN); err != nil { return nil, err }
        body, err := p.parseBlock()
        if err != nil { return nil, err }
        return &ast.FuncDecl{Name: nameTok.Lex, Params: params, Body: body}, nil
    }
    // global variable
    var init *ast.IntLit
    if p.tok.Type == lexer.ASSIGN {
        p.next()
        if p.tok.Type != lexer.INT { return nil, fmt.Errorf("only int initializers for globals at %d:%d", p.tok.Line, p.tok.Col) }
        v, _ := strconv.ParseInt(p.tok.Lex, 10, 64)
        init = &ast.IntLit{Value: v}
        p.next()
    }
    if _, err := p.expect(lexer.SEMI); err != nil { return nil, err }
    return &ast.GlobalDecl{Name: nameTok.Lex, Init: init}, nil
}

func (p *Parser) parseParams() ([]ast.Param, error) {
    var params []ast.Param
    if p.tok.Type == lexer.RPAREN {
        return params, nil
    }
    for {
        if p.tok.Type != lexer.KW_INT {
            return nil, fmt.Errorf("only int params supported at %d:%d", p.tok.Line, p.tok.Col)
        }
        p.next()
        nameTok, err := p.expect(lexer.IDENT)
        if err != nil { return nil, err }
        params = append(params, ast.Param{Name: nameTok.Lex})
        if p.tok.Type == lexer.COMMA { p.next(); continue }
        break
    }
    return params, nil
}

func (p *Parser) parseBlock() (*ast.BlockStmt, error) {
    if _, err := p.expect(lexer.LBRACE); err != nil { return nil, err }
    var stmts []ast.Stmt
    for p.tok.Type != lexer.RBRACE && p.tok.Type != lexer.EOF {
        s, err := p.parseStmt()
        if err != nil { return nil, err }
        stmts = append(stmts, s)
    }
    if _, err := p.expect(lexer.RBRACE); err != nil { return nil, err }
    return &ast.BlockStmt{Stmts: stmts}, nil
}

func (p *Parser) parseStmt() (ast.Stmt, error) {
    switch p.tok.Type {
    case lexer.KW_RETURN:
        p.next()
        e, err := p.parseExpr()
        if err != nil { return nil, err }
        if _, err := p.expect(lexer.SEMI); err != nil { return nil, err }
        return &ast.ReturnStmt{Expr: e}, nil
    case lexer.KW_INT:
        // declaration: int x; | int x = expr;
        p.next()
        for p.tok.Type == lexer.STAR { p.next() }
        nameTok, err := p.expect(lexer.IDENT)
        if err != nil { return nil, err }
        var init ast.Expr
        if p.tok.Type == lexer.ASSIGN {
            p.next()
            init, err = p.parseExpr()
            if err != nil { return nil, err }
        }
        if _, err := p.expect(lexer.SEMI); err != nil { return nil, err }
        return &ast.DeclStmt{Name: nameTok.Lex, Init: init}, nil
    case lexer.LBRACE:
        return p.parseBlock()
    case lexer.KW_IF:
        p.next()
        if _, err := p.expect(lexer.LPAREN); err != nil { return nil, err }
        cond, err := p.parseExpr()
        if err != nil { return nil, err }
        if _, err := p.expect(lexer.RPAREN); err != nil { return nil, err }
        thenBlk, err := p.parseStmt()
        if err != nil { return nil, err }
        var thenBody *ast.BlockStmt
        if tb, ok := thenBlk.(*ast.BlockStmt); ok { thenBody = tb } else { thenBody = &ast.BlockStmt{Stmts: []ast.Stmt{thenBlk}} }
        var elseBody *ast.BlockStmt
        if p.tok.Type == lexer.KW_ELSE {
            p.next()
            elseStmt, err := p.parseStmt()
            if err != nil { return nil, err }
            if eb, ok := elseStmt.(*ast.BlockStmt); ok { elseBody = eb } else { elseBody = &ast.BlockStmt{Stmts: []ast.Stmt{elseStmt}} }
        }
        return &ast.IfStmt{Cond: cond, Then: thenBody, Else: elseBody}, nil
    case lexer.KW_WHILE:
        p.next()
        if _, err := p.expect(lexer.LPAREN); err != nil { return nil, err }
        cond, err := p.parseExpr()
        if err != nil { return nil, err }
        if _, err := p.expect(lexer.RPAREN); err != nil { return nil, err }
        body, err := p.parseStmt()
        if err != nil { return nil, err }
        var b *ast.BlockStmt
        if bb, ok := body.(*ast.BlockStmt); ok { b = bb } else { b = &ast.BlockStmt{Stmts: []ast.Stmt{body}} }
        return &ast.WhileStmt{Cond: cond, Body: b}, nil
    case lexer.KW_SWITCH:
        p.next()
        if _, err := p.expect(lexer.LPAREN); err != nil { return nil, err }
        tag, err := p.parseExpr()
        if err != nil { return nil, err }
        if _, err := p.expect(lexer.RPAREN); err != nil { return nil, err }
        if _, err := p.expect(lexer.LBRACE); err != nil { return nil, err }
        var cases []ast.CaseClause
        var defBody *ast.BlockStmt
        for p.tok.Type != lexer.RBRACE && p.tok.Type != lexer.EOF {
            if p.tok.Type == lexer.KW_CASE {
                // parse one or more case labels possibly sharing a body
                var values []int64
                for {
                    p.next()
                    // only integer literals for now
                    t, err := p.expect(lexer.INT)
                    if err != nil { return nil, err }
                    v, _ := strconv.ParseInt(t.Lex, 10, 64)
                    values = append(values, v)
                    if _, err := p.expect(lexer.COLON); err != nil { return nil, err }
                    if p.tok.Type != lexer.KW_CASE { break }
                }
                // gather statements until next case/default or '}'
                var bodyStmts []ast.Stmt
                for p.tok.Type != lexer.KW_CASE && p.tok.Type != lexer.KW_DEFAULT && p.tok.Type != lexer.RBRACE {
                    s, err := p.parseStmt()
                    if err != nil { return nil, err }
                    bodyStmts = append(bodyStmts, s)
                }
                cases = append(cases, ast.CaseClause{Values: values, Body: &ast.BlockStmt{Stmts: bodyStmts}})
                continue
            }
            if p.tok.Type == lexer.KW_DEFAULT {
                p.next()
                if _, err := p.expect(lexer.COLON); err != nil { return nil, err }
                var bodyStmts []ast.Stmt
                for p.tok.Type != lexer.KW_CASE && p.tok.Type != lexer.RBRACE {
                    s, err := p.parseStmt()
                    if err != nil { return nil, err }
                    bodyStmts = append(bodyStmts, s)
                }
                defBody = &ast.BlockStmt{Stmts: bodyStmts}
                continue
            }
            return nil, fmt.Errorf("unexpected token in switch at %d:%d", p.tok.Line, p.tok.Col)
        }
        if _, err := p.expect(lexer.RBRACE); err != nil { return nil, err }
        return &ast.SwitchStmt{Tag: tag, Cases: cases, Default: defBody}, nil
    case lexer.KW_FOR:
        p.next()
        if _, err := p.expect(lexer.LPAREN); err != nil { return nil, err }
        // init
        var init ast.Stmt
        if p.tok.Type != lexer.SEMI {
            s, err := p.parseForInitOrExprNoSemi()
            if err != nil { return nil, err }
            init = s
        }
        if _, err := p.expect(lexer.SEMI); err != nil { return nil, err }
        // cond
        var cond ast.Expr
        if p.tok.Type != lexer.SEMI {
            e, err := p.parseExpr()
            if err != nil { return nil, err }
            cond = e
        }
        if _, err := p.expect(lexer.SEMI); err != nil { return nil, err }
        // post
        var post ast.Stmt
        if p.tok.Type != lexer.RPAREN {
            s, err := p.parseForInitOrExprNoSemi()
            if err != nil { return nil, err }
            post = s
        }
        if _, err := p.expect(lexer.RPAREN); err != nil { return nil, err }
        body, err := p.parseStmt()
        if err != nil { return nil, err }
        var b *ast.BlockStmt
        if bb, ok := body.(*ast.BlockStmt); ok { b = bb } else { b = &ast.BlockStmt{Stmts: []ast.Stmt{body}} }
        return &ast.ForStmt{Init: init, Cond: cond, Post: post, Body: b}, nil
    case lexer.KW_DO:
        p.next()
        body, err := p.parseStmt()
        if err != nil { return nil, err }
        var b *ast.BlockStmt
        if bb, ok := body.(*ast.BlockStmt); ok { b = bb } else { b = &ast.BlockStmt{Stmts: []ast.Stmt{body}} }
        if _, err := p.expect(lexer.KW_WHILE); err != nil { return nil, err }
        if _, err := p.expect(lexer.LPAREN); err != nil { return nil, err }
        cond, err := p.parseExpr()
        if err != nil { return nil, err }
        if _, err := p.expect(lexer.RPAREN); err != nil { return nil, err }
        if _, err := p.expect(lexer.SEMI); err != nil { return nil, err }
        return &ast.DoWhileStmt{Body: b, Cond: cond}, nil
    case lexer.KW_BREAK:
        p.next()
        if _, err := p.expect(lexer.SEMI); err != nil { return nil, err }
        return &ast.BreakStmt{}, nil
    case lexer.KW_CONTINUE:
        p.next()
        if _, err := p.expect(lexer.SEMI); err != nil { return nil, err }
        return &ast.ContinueStmt{}, nil
    case lexer.IDENT:
        // assignment or expr statement
        // Try lookahead '='
        id := p.tok
        p.next()
        if p.tok.Type == lexer.ASSIGN {
            p.next()
            v, err := p.parseExpr()
            if err != nil { return nil, err }
            if _, err := p.expect(lexer.SEMI); err != nil { return nil, err }
            return &ast.AssignStmt{Name: id.Lex, Value: v}, nil
        }
        // rollback: treat IDENT as start of primary in expr
        // Continue parsing the rest of the expression after this primary
        left := &ast.Ident{Name: id.Lex}
        e, err := p.parseAfterPrimary(left)
        if err != nil { return nil, err }
        if _, err := p.expect(lexer.SEMI); err != nil { return nil, err }
        return &ast.ExprStmt{X: e}, nil
    default:
        e, err := p.parseExpr()
        if err != nil { return nil, err }
        if _, err := p.expect(lexer.SEMI); err != nil { return nil, err }
        return &ast.ExprStmt{X: e}, nil
    }
}

// Expr grammar with precedence:
// expr = equality
// equality = relational { (==|!=) relational }
// relational = add { (<|<=|>|>=) add }
// add = mul { (+|-) mul }
// mul = primary { (*|/) primary }
// primary = IDENT | INT | '(' expr ')'
func (p *Parser) parseExpr() (ast.Expr, error) { return p.parseLogicalOr() }

func (p *Parser) parseLogicalOr() (ast.Expr, error) {
    left, err := p.parseLogicalAnd()
    if err != nil { return nil, err }
    for p.tok.Type == lexer.OROR {
        p.next()
        right, err := p.parseLogicalAnd()
        if err != nil { return nil, err }
        left = &ast.BinaryExpr{Op: ast.OpLOr, Left: left, Right: right}
    }
    return left, nil
}

func (p *Parser) parseLogicalAnd() (ast.Expr, error) {
    left, err := p.parseEquality()
    if err != nil { return nil, err }
    for p.tok.Type == lexer.ANDAND {
        p.next()
        right, err := p.parseEquality()
        if err != nil { return nil, err }
        left = &ast.BinaryExpr{Op: ast.OpLAnd, Left: left, Right: right}
    }
    return left, nil
}

func (p *Parser) parseAdd(left ast.Expr) (ast.Expr, error) {
    for p.tok.Type == lexer.PLUS || p.tok.Type == lexer.MINUS {
        op := p.tok.Type; p.next()
        right, err := p.parseTerm()
        if err != nil { return nil, err }
        left = &ast.BinaryExpr{Op: binOpFromToken(op), Left: left, Right: right}
    }
    return left, nil
}

func (p *Parser) parseTerm() (ast.Expr, error) {
    left, err := p.parseUnary()
    if err != nil { return nil, err }
    for p.tok.Type == lexer.STAR || p.tok.Type == lexer.SLASH {
        op := p.tok.Type; p.next()
        right, err := p.parseUnary()
        if err != nil { return nil, err }
        left = &ast.BinaryExpr{Op: binOpFromToken(op), Left: left, Right: right}
    }
    return left, nil
}

func (p *Parser) parseFactor() (ast.Expr, error) {
    switch p.tok.Type {
    case lexer.IDENT:
        name := p.tok.Lex
        p.next()
        if p.tok.Type == lexer.LPAREN {
            // call
            p.next()
            var args []ast.Expr
            if p.tok.Type != lexer.RPAREN {
                for {
                    e, err := p.parseExpr()
                    if err != nil { return nil, err }
                    args = append(args, e)
                    if p.tok.Type == lexer.COMMA { p.next(); continue }
                    break
                }
            }
            if _, err := p.expect(lexer.RPAREN); err != nil { return nil, err }
            return &ast.CallExpr{Name: name, Args: args}, nil
        }
        return &ast.Ident{Name: name}, nil
    case lexer.INT:
        v, _ := strconv.ParseInt(p.tok.Lex, 10, 64)
        lit := &ast.IntLit{Value: v}
        p.next()
        return lit, nil
    case lexer.LPAREN:
        p.next()
        e, err := p.parseExpr()
        if err != nil { return nil, err }
        if _, err := p.expect(lexer.RPAREN); err != nil { return nil, err }
        return e, nil
    default:
        return nil, fmt.Errorf("unexpected token %v at %d:%d", p.tok.Type, p.tok.Line, p.tok.Col)
    }
}

func (p *Parser) parseUnary() (ast.Expr, error) {
    if p.tok.Type == lexer.AMP {
        p.next()
        x, err := p.parseUnary()
        if err != nil { return nil, err }
        return &ast.UnaryExpr{Op: ast.OpAddr, X: x}, nil
    }
    if p.tok.Type == lexer.STAR {
        p.next()
        x, err := p.parseUnary()
        if err != nil { return nil, err }
        return &ast.UnaryExpr{Op: ast.OpDeref, X: x}, nil
    }
    return p.parseFactor()
}

func (p *Parser) parseRelational() (ast.Expr, error) {
    left, err := p.parseTerm()
    if err != nil { return nil, err }
    for p.tok.Type == lexer.LT || p.tok.Type == lexer.LE || p.tok.Type == lexer.GT || p.tok.Type == lexer.GE {
        op := p.tok.Type; p.next()
        right, err := p.parseTerm()
        if err != nil { return nil, err }
        left = &ast.BinaryExpr{Op: binOpFromToken(op), Left: left, Right: right}
    }
    return left, nil
}

func (p *Parser) parseEquality() (ast.Expr, error) {
    left, err := p.parseRelational()
    if err != nil { return nil, err }
    for p.tok.Type == lexer.EQEQ || p.tok.Type == lexer.NEQ {
        op := p.tok.Type; p.next()
        right, err := p.parseRelational()
        if err != nil { return nil, err }
        left = &ast.BinaryExpr{Op: binOpFromToken(op), Left: left, Right: right}
    }
    return p.parseAdd(left)
}

// parse a simple statement used in for-init/post without trailing semicolon
func (p *Parser) parseForInitOrExprNoSemi() (ast.Stmt, error) {
    switch p.tok.Type {
    case lexer.KW_INT:
        p.next()
        nameTok, err := p.expect(lexer.IDENT)
        if err != nil { return nil, err }
        var init ast.Expr
        if p.tok.Type == lexer.ASSIGN {
            p.next()
            e, err := p.parseExpr()
            if err != nil { return nil, err }
            init = e
        }
        return &ast.DeclStmt{Name: nameTok.Lex, Init: init}, nil
    case lexer.IDENT:
        id := p.tok
        p.next()
        if p.tok.Type == lexer.ASSIGN {
            p.next()
            e, err := p.parseExpr()
            if err != nil { return nil, err }
            return &ast.AssignStmt{Name: id.Lex, Value: e}, nil
        }
        // treat as expression statement starting with this ident
        left := &ast.Ident{Name: id.Lex}
        e, err := p.parseAfterPrimary(left)
        if err != nil { return nil, err }
        return &ast.ExprStmt{X: e}, nil
    default:
        // expression
        e, err := p.parseExpr()
        if err != nil { return nil, err }
        return &ast.ExprStmt{X: e}, nil
    }
}

// Continue parsing after an already-read primary expression (IDENT/INT/(...)).
func (p *Parser) parseAfterPrimary(left ast.Expr) (ast.Expr, error) {
    // handle * and /
    for p.tok.Type == lexer.STAR || p.tok.Type == lexer.SLASH {
        op := p.tok.Type; p.next()
        right, err := p.parseFactor()
        if err != nil { return nil, err }
        left = &ast.BinaryExpr{Op: binOpFromToken(op), Left: left, Right: right}
    }
    // handle + and -
    for p.tok.Type == lexer.PLUS || p.tok.Type == lexer.MINUS {
        op := p.tok.Type; p.next()
        right, err := p.parseTerm()
        if err != nil { return nil, err }
        left = &ast.BinaryExpr{Op: binOpFromToken(op), Left: left, Right: right}
    }
    // relational
    for p.tok.Type == lexer.LT || p.tok.Type == lexer.LE || p.tok.Type == lexer.GT || p.tok.Type == lexer.GE {
        op := p.tok.Type; p.next()
        right, err := p.parseAdd(left)
        if err != nil { return nil, err }
        // Note: parseAdd ignores its left, so rebuild properly
        left = &ast.BinaryExpr{Op: binOpFromToken(op), Left: left, Right: right}
    }
    // equality
    for p.tok.Type == lexer.EQEQ || p.tok.Type == lexer.NEQ {
        op := p.tok.Type; p.next()
        right, err := p.parseRelational()
        if err != nil { return nil, err }
        left = &ast.BinaryExpr{Op: binOpFromToken(op), Left: left, Right: right}
    }
    // logical and/or
    for p.tok.Type == lexer.ANDAND {
        p.next()
        right, err := p.parseEquality()
        if err != nil { return nil, err }
        left = &ast.BinaryExpr{Op: ast.OpLAnd, Left: left, Right: right}
    }
    for p.tok.Type == lexer.OROR {
        p.next()
        right, err := p.parseLogicalAnd()
        if err != nil { return nil, err }
        left = &ast.BinaryExpr{Op: ast.OpLOr, Left: left, Right: right}
    }
    return left, nil
}

func binOpFromToken(t lexer.TokenType) ast.BinOp {
    switch t {
    case lexer.PLUS: return ast.OpAdd
    case lexer.MINUS: return ast.OpSub
    case lexer.STAR: return ast.OpMul
    case lexer.SLASH: return ast.OpDiv
    case lexer.EQEQ: return ast.OpEq
    case lexer.NEQ: return ast.OpNe
    case lexer.LT: return ast.OpLt
    case lexer.LE: return ast.OpLe
    case lexer.GT: return ast.OpGt
    case lexer.GE: return ast.OpGe
    case lexer.ANDAND: return ast.OpLAnd
    case lexer.OROR: return ast.OpLOr
    default: return ast.OpAdd
    }
}
