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
    KW_IF
    KW_ELSE
    KW_WHILE
    KW_FOR
    KW_DO
    KW_BREAK
    KW_CONTINUE

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

    // Comparison
    EQEQ   // ==
    NEQ    // !=
    LT     // <
    LE     // <=
    GT     // >
    GE     // >=
)

type Token struct {
    Type  TokenType
    Lex   string
    Line  int
    Col   int
}

func (t Token) Is(op TokenType) bool { return t.Type == op }
