package lexer

type TokenType int

const (
    // Special
    EOF TokenType = iota
    ILLEGAL

    // Identifiers + literals
    IDENT
    INT

    // Keywords
    KW_INT
    KW_RETURN

    // Symbols
    LPAREN // (
    RPAREN // )
    LBRACE // {
    RBRACE // }
    SEMI   // ;
    COMMA  // ,
    ASSIGN // =

    PLUS  // +
    MINUS // -
    STAR  // *
    SLASH // /
)

type Token struct {
    Type  TokenType
    Lex   string
    Line  int
    Col   int
}

func (t Token) Is(op TokenType) bool { return t.Type == op }

