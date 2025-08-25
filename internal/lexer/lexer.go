package lexer

import (
    "unicode"
)

type Lexer struct {
    src []rune
    i   int
    ch  rune
    line int
    col  int
}

func New(src string) *Lexer {
    l := &Lexer{src: []rune(src), line: 1}
    l.read()
    return l
}

func (l *Lexer) read() {
    if l.i >= len(l.src) {
        l.ch = 0
        return
    }
    l.ch = l.src[l.i]
    l.i++
    if l.ch == '\n' {
        l.line++
        l.col = 0
    } else {
        l.col++
    }
}

func (l *Lexer) peek() rune {
    if l.i >= len(l.src) {
        return 0
    }
    return l.src[l.i]
}

func (l *Lexer) Next() Token {
    // skip spaces and comments
    for {
        for unicode.IsSpace(l.ch) { l.read() }
        if l.ch == '/' && l.peek() == '/' {
            for l.ch != 0 && l.ch != '\n' { l.read() }
            continue
        }
        if l.ch == '/' && l.peek() == '*' {
            l.read(); l.read()
            for l.ch != 0 {
                if l.ch == '*' && l.peek() == '/' { l.read(); l.read(); break }
                l.read()
            }
            continue
        }
        break
    }
    tok := Token{Line: l.line, Col: l.col}
    switch ch := l.ch; ch {
    case 0:
        tok.Type = EOF
    case '(':
        tok.Type, tok.Lex = LPAREN, string(ch); l.read()
    case ')':
        tok.Type, tok.Lex = RPAREN, string(ch); l.read()
    case '{':
        tok.Type, tok.Lex = LBRACE, string(ch); l.read()
    case '}':
        tok.Type, tok.Lex = RBRACE, string(ch); l.read()
    case ';':
        tok.Type, tok.Lex = SEMI, string(ch); l.read()
    case ',':
        tok.Type, tok.Lex = COMMA, string(ch); l.read()
    case '=':
        if l.peek() == '=' { l.read(); tok.Type, tok.Lex = EQEQ, "=="; l.read() } else { tok.Type, tok.Lex = ASSIGN, string(ch); l.read() }
    case '+':
        tok.Type, tok.Lex = PLUS, string(ch); l.read()
    case '-':
        tok.Type, tok.Lex = MINUS, string(ch); l.read()
    case '*':
        tok.Type, tok.Lex = STAR, string(ch); l.read()
    case '/':
        tok.Type, tok.Lex = SLASH, string(ch); l.read()
    case '!':
        if l.peek() == '=' { l.read(); tok.Type, tok.Lex = NEQ, "!="; l.read() } else { tok.Type, tok.Lex = ILLEGAL, string(ch); l.read() }
    case '<':
        if l.peek() == '=' { l.read(); tok.Type, tok.Lex = LE, "<="; l.read() } else { tok.Type, tok.Lex = LT, "<"; l.read() }
    case '>':
        if l.peek() == '=' { l.read(); tok.Type, tok.Lex = GE, ">="; l.read() } else { tok.Type, tok.Lex = GT, ">"; l.read() }
    default:
        if unicode.IsLetter(ch) || ch == '_' {
            startLine, startCol := l.line, l.col
            ident := []rune{ch}
            l.read()
            for unicode.IsLetter(l.ch) || unicode.IsDigit(l.ch) || l.ch == '_' {
                ident = append(ident, l.ch)
                l.read()
            }
            lex := string(ident)
            tok.Line, tok.Col = startLine, startCol
            switch lex {
            case "int": tok.Type = KW_INT
            case "return": tok.Type = KW_RETURN
            case "if": tok.Type = KW_IF
            case "else": tok.Type = KW_ELSE
            case "while": tok.Type = KW_WHILE
            case "for": tok.Type = KW_FOR
            case "do": tok.Type = KW_DO
            case "break": tok.Type = KW_BREAK
            case "continue": tok.Type = KW_CONTINUE
            default:
                tok.Type = IDENT
            }
            tok.Lex = lex
        } else if unicode.IsDigit(ch) {
            startLine, startCol := l.line, l.col
            num := []rune{ch}
            l.read()
            for unicode.IsDigit(l.ch) {
                num = append(num, l.ch)
                l.read()
            }
            tok.Type, tok.Lex = INT, string(num)
            tok.Line, tok.Col = startLine, startCol
        } else {
            tok.Type, tok.Lex = ILLEGAL, string(ch)
            l.read()
        }
    }
    return tok
}
