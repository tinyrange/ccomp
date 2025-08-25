package lexer

type TokenType int

const (
	// Special
	EOF TokenType = iota
	ILLEGAL

	// Identifiers + literals
	IDENT
	INT
	FLOAT
	CHAR
	STRING

	// Keywords
	KW_INT
	KW_CHAR
	KW_DOUBLE
	KW_STRUCT
	KW_ENUM
	KW_TYPEDEF
	KW_RETURN
	KW_IF
	KW_ELSE
	KW_WHILE
	KW_FOR
	KW_DO
	KW_BREAK
	KW_CONTINUE
	KW_SWITCH
	KW_CASE
	KW_DEFAULT

	// Symbols
	LPAREN // (
	RPAREN // )
	LBRACE // {
	RBRACE // }
	LBRACK // [
	RBRACK // ]
	SEMI   // ;
	COMMA  // ,
	COLON  // :
	DOT    // .
	ASSIGN // =
	AMP    // &

	// Arithmetic
	PLUS  // +
	MINUS // -
	STAR  // *
	SLASH // /

	// Shifts
	SHL // <<
	SHR // >>

	// Bitwise/logical
	ANDAND // &&
	OROR   // ||
	PIPE   // |
	CARET  // ^
	TILDE  // ~
	BANG   // !

	// Comparison
	EQEQ // ==
	NEQ  // !=
	LT   // <
	LE   // <=
	GT   // >
	GE   // >=
)

type Token struct {
	Type TokenType
	Lex  string
	Line int
	Col  int
}

func (t Token) Is(op TokenType) bool { return t.Type == op }
