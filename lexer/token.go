// Package lexer provides tokenization for the vish shell.
package lexer

// TokenType enumerates all lexical token types.
type TokenType int

const (
	TokIllegal   TokenType = iota
	TokEOF                 // end of input
	TokNewline             // \n
	TokWord                // general word / keyword / assignment
	TokIONum               // digit(s) immediately before a redirect op
	TokSemi                // ;
	TokDSemi               // ;;
	TokAmp                 // &
	TokPipe                // |
	TokAndIf               // &&
	TokOrIf                // ||
	TokLess                // <
	TokGreat               // >
	TokDLess               // <<
	TokDGreat              // >>
	TokLessAnd             // <&
	TokGreatAnd            // >&
	TokLessGreat           // <>
	TokDLessDash           // <<-
	TokClobber             // >|
	TokLParen              // (
	TokRParen              // )
)

var tokNames = map[TokenType]string{
	TokIllegal:   "ILLEGAL",
	TokEOF:       "EOF",
	TokNewline:   "NEWLINE",
	TokWord:      "WORD",
	TokIONum:     "IO_NUMBER",
	TokSemi:      ";",
	TokDSemi:     ";;",
	TokAmp:       "&",
	TokPipe:      "|",
	TokAndIf:     "&&",
	TokOrIf:      "||",
	TokLess:      "<",
	TokGreat:     ">",
	TokDLess:     "<<",
	TokDGreat:    ">>",
	TokLessAnd:   "<&",
	TokGreatAnd:  ">&",
	TokLessGreat: "<>",
	TokDLessDash: "<<-",
	TokClobber:   ">|",
	TokLParen:    "(",
	TokRParen:    ")",
}

func (tt TokenType) String() string {
	if s, ok := tokNames[tt]; ok {
		return s
	}
	return "UNKNOWN"
}

// Token is a single lexical element.
type Token struct {
	Type TokenType
	Val  string
	Line int
	Col  int
}

func (t Token) String() string {
	switch t.Type {
	case TokEOF:
		return "<EOF>"
	case TokNewline:
		return "<NL>"
	default:
		return t.Val
	}
}

// IsRedirOp reports whether the token is a redirect operator.
func (t Token) IsRedirOp() bool {
	switch t.Type {
	case TokLess, TokGreat, TokDLess, TokDGreat, TokLessAnd,
		TokGreatAnd, TokLessGreat, TokDLessDash, TokClobber:
		return true
	}
	return false
}
