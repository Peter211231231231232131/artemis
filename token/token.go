package token

type TokenType string

const (
	ILLEGAL = "ILLEGAL"
	EOF     = "EOF"

	IDENT  = "IDENT"
	INT    = "INT"
	FLOAT  = "FLOAT"
	STRING = "STRING"

	ASSIGN    = "="
	FAT_ARROW = "=>"
	PLUS      = "+"
	INC       = "++"
	MINUS     = "-"
	DEC       = "--"
	ASTERISK  = "*"
	SLASH     = "/"
	MOD       = "%"
	BANG      = "!"

	LT     = "<"
	GT     = ">"
	EQ     = "=="
	NOT_EQ = "!="
	AND    = "&&"
	OR     = "||"
	PIPE   = "|>"
	RSHIFT = ">>"

	COMMA     = ","
	SEMICOLON = ";"
	COLON     = ":"
	DOT       = "."

	LPAREN   = "("
	RPAREN   = ")"
	LBRACE   = "{"
	RBRACE   = "}"
	LBRACKET = "["
	RBRACKET = "]"

	SET    = "SET"
	OUT    = "OUT"
	IF     = "IF"
	ELSE   = "ELSE"
	FOR    = "FOR"
	WHILE  = "WHILE"
	FN     = "FN"
	RETURN = "RETURN"
	MATCH  = "MATCH"
	SPAWN  = "SPAWN"
	IMPORT = "IMPORT"
	AS     = "AS"
	TRY    = "TRY"
	CATCH  = "CATCH"
	THROW  = "THROW"
	TRUE   = "TRUE"
	FALSE  = "FALSE"
	BREAK  = "BREAK"
	CONTINUE = "CONTINUE"
	IN     = "IN"
	CONST  = "CONST"

	BITAND  = "&"
	BITOR   = "|"
	BITXOR  = "^"
	BITNOT  = "~"
	LSHIFT  = "<<"
)

var keywords = map[string]TokenType{
	"set":    SET,
	"out":    OUT,
	"if":     IF,
	"else":   ELSE,
	"for":    FOR,
	"while":  WHILE,
	"fn":     FN,
	"return": RETURN,
	"match":  MATCH,
	"spawn":  SPAWN,
	"import": IMPORT,
	"as":     AS,
	"try":    TRY,
	"catch":  CATCH,
	"throw":  THROW,
	"true":   TRUE,
	"false":  FALSE,
	"break":  BREAK,
	"continue": CONTINUE,
	"in":     IN,
	"const":  CONST,
}

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Col     int
}

func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return IDENT
}
