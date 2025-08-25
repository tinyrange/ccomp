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
    // Only: int IDENT(params) { ... }
    if p.tok.Type != lexer.KW_INT {
        return nil, fmt.Errorf("only 'int' functions supported at %d:%d", p.tok.Line, p.tok.Col)
    }
    p.next()
    nameTok, err := p.expect(lexer.IDENT)
    if err != nil { return nil, err }
    if _, err = p.expect(lexer.LPAREN); err != nil { return nil, err }
    params, err := p.parseParams()
    if err != nil { return nil, err }
    if _, err = p.expect(lexer.RPAREN); err != nil { return nil, err }
    body, err := p.parseBlock()
    if err != nil { return nil, err }
    return &ast.FuncDecl{Name: nameTok.Lex, Params: params, Body: body}, nil
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
        // We simulate rollback by creating an Ident and parsing rest via parseExprTail
        left := &ast.Ident{Name: id.Lex}
        e, err := p.parseExprTail(left)
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

// Expr grammar:
// expr = term { (+|-) term }
// term = factor { (*|/) factor }
// factor = IDENT | INT | '(' expr ')'
func (p *Parser) parseExpr() (ast.Expr, error) {
    left, err := p.parseTerm()
    if err != nil { return nil, err }
    return p.parseExprTail(left)
}

func (p *Parser) parseExprTail(left ast.Expr) (ast.Expr, error) {
    for p.tok.Type == lexer.PLUS || p.tok.Type == lexer.MINUS {
        op := p.tok.Type; p.next()
        right, err := p.parseTerm()
        if err != nil { return nil, err }
        left = &ast.BinaryExpr{Op: binOpFromToken(op), Left: left, Right: right}
    }
    return left, nil
}

func (p *Parser) parseTerm() (ast.Expr, error) {
    left, err := p.parseFactor()
    if err != nil { return nil, err }
    for p.tok.Type == lexer.STAR || p.tok.Type == lexer.SLASH {
        op := p.tok.Type; p.next()
        right, err := p.parseFactor()
        if err != nil { return nil, err }
        left = &ast.BinaryExpr{Op: binOpFromToken(op), Left: left, Right: right}
    }
    return left, nil
}

func (p *Parser) parseFactor() (ast.Expr, error) {
    switch p.tok.Type {
    case lexer.IDENT:
        id := &ast.Ident{Name: p.tok.Lex}
        p.next()
        return id, nil
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

func binOpFromToken(t lexer.TokenType) ast.BinOp {
    switch t {
    case lexer.PLUS: return ast.OpAdd
    case lexer.MINUS: return ast.OpSub
    case lexer.STAR: return ast.OpMul
    case lexer.SLASH: return ast.OpDiv
    default: return ast.OpAdd
    }
}

