package parser

import (
	"xon/ast"
	"xon/lexer"
	"xon/token"
	"fmt"
	"strconv"
	"strings"
)

const (
	_ int = iota
	LOWEST
	PIPE
	OR          // ||
	AND         // &&
	EQUALS      // ==
	LESSGREATER // > or <
	SUM         // +
	PRODUCT     // *
	PREFIX      // -X or !X
	INDEX       // array[index]
	DOT         // obj.method
	CALL        // myFunction(X)
)

var precedences = map[token.TokenType]int{
	token.EQ:       EQUALS,
	token.NOT_EQ:   EQUALS,
	token.LT:       LESSGREATER,
	token.GT:       LESSGREATER,
	token.PLUS:     SUM,
	token.MINUS:    SUM,
	token.ASTERISK: PRODUCT,
	token.SLASH:    PRODUCT,
	token.MOD:      PRODUCT,
	token.LPAREN:   CALL,
	token.LBRACKET: INDEX,
	token.DOT:      DOT,
	token.AND:      AND,
	token.OR:       OR,
	token.PIPE:     PIPE,
	token.INC:      SUM,
	token.DEC:      SUM,
	token.BITAND:   PRODUCT,
	token.BITOR:    PRODUCT,
	token.BITXOR:   PRODUCT,
	token.LSHIFT:   PRODUCT,
	token.RSHIFT:   PRODUCT,
}

type (
	prefixParseFn func() ast.Expression
	infixParseFn  func(ast.Expression) ast.Expression
)

type Parser struct {
	l         *lexer.Lexer
	curToken  token.Token
	peekToken token.Token
	Errors    []string

	prefixParseFns map[token.TokenType]prefixParseFn
	infixParseFns  map[token.TokenType]infixParseFn
}

func New(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:      l,
		Errors: []string{},
	}

	p.prefixParseFns = make(map[token.TokenType]prefixParseFn)
	p.registerPrefix(token.IDENT, p.parseIdentifier)
	p.registerPrefix(token.INT, p.parseIntegerLiteral)
	p.registerPrefix(token.FLOAT, p.parseFloatLiteral)
	p.registerPrefix(token.STRING, p.parseStringLiteral)
	p.registerPrefix(token.LPAREN, p.parseGroupedExpression)
	p.registerPrefix(token.LBRACKET, p.parseArrayLiteral)
	p.registerPrefix(token.LBRACE, p.parseHashLiteral)
	p.registerPrefix(token.FN, p.parseFunctionLiteral)
	p.registerPrefix(token.TRUE, p.parseBoolean)
	p.registerPrefix(token.FALSE, p.parseBoolean)
	p.registerPrefix(token.BANG, p.parsePrefixExpression)
	p.registerPrefix(token.MINUS, p.parsePrefixExpression)
	p.registerPrefix(token.BITNOT, p.parsePrefixExpression)
	p.registerPrefix(token.MATCH, p.parseMatchExpression)
	p.registerPrefix(token.TRY, p.parseTryExpression)

	p.infixParseFns = make(map[token.TokenType]infixParseFn)
	p.registerInfix(token.PLUS, p.parseInfixExpression)
	p.registerInfix(token.MINUS, p.parseInfixExpression)
	p.registerInfix(token.SLASH, p.parseInfixExpression)
	p.registerInfix(token.ASTERISK, p.parseInfixExpression)
	p.registerInfix(token.MOD, p.parseInfixExpression)
	p.registerInfix(token.EQ, p.parseInfixExpression)
	p.registerInfix(token.NOT_EQ, p.parseInfixExpression)
	p.registerInfix(token.LT, p.parseInfixExpression)
	p.registerInfix(token.GT, p.parseInfixExpression)
	p.registerInfix(token.LPAREN, p.parseCallExpression)
	p.registerInfix(token.LBRACKET, p.parseIndexExpression)
	p.registerInfix(token.AND, p.parseInfixExpression)
	p.registerInfix(token.OR, p.parseInfixExpression)
	p.registerInfix(token.PIPE, p.parsePipeExpression)
	p.registerInfix(token.DOT, p.parseMemberExpression)
	p.registerInfix(token.INC, p.parsePostfixExpression)
	p.registerInfix(token.DEC, p.parsePostfixExpression)
	p.registerInfix(token.BITAND, p.parseInfixExpression)
	p.registerInfix(token.BITOR, p.parseInfixExpression)
	p.registerInfix(token.BITXOR, p.parseInfixExpression)
	p.registerInfix(token.LSHIFT, p.parseInfixExpression)
	p.registerInfix(token.RSHIFT, p.parseInfixExpression)

	p.nextToken()
	p.nextToken()
	return p
}

func (p *Parser) registerPrefix(tokenType token.TokenType, fn prefixParseFn) {
	p.prefixParseFns[tokenType] = fn
}

func (p *Parser) registerInfix(tokenType token.TokenType, fn infixParseFn) {
	p.infixParseFns[tokenType] = fn
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) ParseProgram() *ast.Program {
	program := &ast.Program{}
	for p.curToken.Type != token.EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
	}
	return program
}

func (p *Parser) parseStatement() ast.Statement {
	switch p.curToken.Type {
	case token.SET:
		return p.parseSetStatement()
	case token.OUT:
		return p.parseOutStatement()
	case token.RETURN:
		return p.parseReturnStatement()
	case token.IF:
		return p.parseIfStatement()
	case token.WHILE:
		return p.parseWhileStatement()
	case token.FOR:
		return p.parseForStatement()
	case token.SPAWN:
		return p.parseSpawnStatement()
	case token.IMPORT:
		return p.parseImportStatement()
	case token.THROW:
		return p.parseThrowStatement()
	case token.BREAK:
		return p.parseBreakStatement()
	case token.CONTINUE:
		return p.parseContinueStatement()
	case token.IDENT:
		if p.peekToken.Type == token.ASSIGN {
			return p.parseAssignStatement()
		}
		return p.parseExpressionStatement()
	case token.SEMICOLON:
		// empty statement basically, ignore
		return nil
	default:
		return p.parseExpressionStatement()
	}
}

func (p *Parser) parseSetStatement() *ast.SetStatement {
	stmt := &ast.SetStatement{Token: p.curToken}
	p.nextToken() // past set
	if p.curToken.Type == token.CONST {
		stmt.IsConst = true
		p.nextToken()
	}
	if p.curToken.Type != token.IDENT {
		p.Errors = append(p.Errors, fmt.Sprintf("Line %d, Col %d: expected identifier", p.curToken.Line, p.curToken.Col))
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekToken.Type != token.ASSIGN {
		p.Errors = append(p.Errors, fmt.Sprintf("Line %d, Col %d: expected assign =", p.peekToken.Line, p.peekToken.Col))
		return nil
	}
	p.nextToken() // to =
	p.nextToken() // past =

	stmt.Value = p.parseExpression(LOWEST)

	if p.peekToken.Type == token.SEMICOLON {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseAssignStatement() *ast.AssignStatement {
	stmt := &ast.AssignStatement{Token: token.Token{Type: token.ASSIGN, Literal: "="}}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	p.nextToken() // past identifier (to =)
	p.nextToken() // past =

	stmt.Value = p.parseExpression(LOWEST)

	if p.peekToken.Type == token.SEMICOLON {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseThrowStatement() *ast.ThrowStatement {
	stmt := &ast.ThrowStatement{Token: p.curToken}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	if p.peekToken.Type == token.SEMICOLON {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseBreakStatement() *ast.BreakStatement {
	stmt := &ast.BreakStatement{Token: p.curToken}
	p.nextToken()
	if p.peekToken.Type == token.SEMICOLON {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseContinueStatement() *ast.ContinueStatement {
	stmt := &ast.ContinueStatement{Token: p.curToken}
	p.nextToken()
	if p.peekToken.Type == token.SEMICOLON {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseOutStatement() *ast.OutStatement {
	stmt := &ast.OutStatement{Token: p.curToken}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	if p.peekToken.Type == token.SEMICOLON {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseReturnStatement() *ast.ReturnStatement {
	stmt := &ast.ReturnStatement{Token: p.curToken}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	if p.peekToken.Type == token.SEMICOLON {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseImportStatement() *ast.ImportStatement {
	stmt := &ast.ImportStatement{Token: p.curToken}
	p.nextToken()
	stmt.Path = p.parseExpression(LOWEST)

	if p.peekToken.Type == token.AS {
		p.nextToken() // past path
		p.nextToken() // past as
		if p.curToken.Type != token.IDENT {
			p.Errors = append(p.Errors, fmt.Sprintf("expected identifier after 'as', got %s", p.curToken.Type))
			return nil
		}
		stmt.Alias = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	}

	if p.peekToken.Type == token.SEMICOLON {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseIfStatement() *ast.IfStatement {
	stmt := &ast.IfStatement{Token: p.curToken}
	p.nextToken() // past if

	stmt.Condition = p.parseExpression(LOWEST)

	if p.peekToken.Type == token.LBRACE {
		p.nextToken()
		stmt.Consequence = p.parseBlockStatement()
	} else {
		// support single statement without block
		p.nextToken()
		consequence := p.parseStatement()
		stmt.Consequence = &ast.BlockStatement{Statements: []ast.Statement{consequence}}
	}

	if p.peekToken.Type == token.ELSE {
		p.nextToken()
		if p.peekToken.Type == token.LBRACE {
			p.nextToken()
			stmt.Alternative = p.parseBlockStatement()
		} else {
			p.nextToken()
			alternative := p.parseStatement()
			stmt.Alternative = &ast.BlockStatement{Statements: []ast.Statement{alternative}}
		}
	}
	return stmt
}

func (p *Parser) parseWhileStatement() *ast.WhileStatement {
	stmt := &ast.WhileStatement{Token: p.curToken}
	p.nextToken()
	stmt.Condition = p.parseExpression(LOWEST)

	if p.peekToken.Type == token.LBRACE {
		p.nextToken()
		stmt.Body = p.parseBlockStatement()
	} else {
		p.nextToken()
		body := p.parseStatement()
		stmt.Body = &ast.BlockStatement{Statements: []ast.Statement{body}}
	}
	return stmt
}

func (p *Parser) parseBlockStatement() *ast.BlockStatement {
	block := &ast.BlockStatement{Token: p.curToken}
	block.Statements = []ast.Statement{}

	p.nextToken()

	for p.curToken.Type != token.RBRACE && p.curToken.Type != token.EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()
	}
	return block
}

func (p *Parser) parseExpressionStatement() *ast.ExpressionStatement {
	stmt := &ast.ExpressionStatement{Token: p.curToken}
	stmt.Expression = p.parseExpression(LOWEST)
	if p.peekToken.Type == token.SEMICOLON {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseExpression(precedence int) ast.Expression {
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.Errors = append(p.Errors, fmt.Sprintf("Line %d, Col %d: no prefix function for %s", p.curToken.Line, p.curToken.Col, p.curToken.Type))
		return nil
	}
	leftExp := prefix()

	for p.peekToken.Type != token.SEMICOLON && precedence < p.peekPrecedence() {
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}
		p.nextToken()
		leftExp = infix(leftExp)
	}
	return leftExp
}

func (p *Parser) parseIdentifier() ast.Expression {
	return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseIntegerLiteral() ast.Expression {
	lit := &ast.IntegerLiteral{Token: p.curToken}
	value, _ := strconv.ParseInt(p.curToken.Literal, 0, 64)
	lit.Value = value
	return lit
}

func (p *Parser) parsePrefixExpression() ast.Expression {
	expression := &ast.PrefixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
	}
	p.nextToken()
	expression.Right = p.parseExpression(PREFIX)
	return expression
}

func (p *Parser) parseStringLiteral() ast.Expression {
	lit := p.curToken.Literal
	if !strings.Contains(lit, "${") {
		return &ast.StringLiteral{Token: p.curToken, Value: lit}
	}

	exp := &ast.InterpolatedString{Token: p.curToken, Parts: []ast.Expression{}}

	// Simple interpolation parser
	i := 0
	for i < len(lit) {
		idx := strings.Index(lit[i:], "${")
		if idx == -1 {
			exp.Parts = append(exp.Parts, &ast.StringLiteral{Value: lit[i:]})
			break
		}

		// Add literal part before ${
		if idx > 0 {
			exp.Parts = append(exp.Parts, &ast.StringLiteral{Value: lit[i : i+idx]})
		}

		i += idx + 2 // move past ${

		// Find closing }
		// This is a bit naive (doesn't handle nested {})
		// but for a start it works.
		end := strings.Index(lit[i:], "}")
		if end == -1 {
			p.Errors = append(p.Errors, "unterminated interpolation")
			return nil
		}

		exprStr := lit[i : i+end]
		// Lex and Parse the expression inside
		subL := lexer.New(exprStr)
		subP := New(subL)
		subProg := subP.ParseProgram()
		if len(subProg.Statements) > 0 {
			if stmt, ok := subProg.Statements[0].(*ast.ExpressionStatement); ok {
				exp.Parts = append(exp.Parts, stmt.Expression)
			}
		}

		i += end + 1 // move past }
	}

	return exp
}

func (p *Parser) parseBoolean() ast.Expression {
	return &ast.Boolean{Token: p.curToken, Value: p.curToken.Type == token.TRUE}
}

func (p *Parser) parseGroupedExpression() ast.Expression {
	p.nextToken()
	exp := p.parseExpression(LOWEST)
	if p.peekToken.Type != token.RPAREN {
		p.Errors = append(p.Errors, fmt.Sprintf("Line %d, Col %d: expected )", p.peekToken.Line, p.peekToken.Col))
		return nil
	}
	p.nextToken()
	return exp
}

func (p *Parser) parseArrayLiteral() ast.Expression {
	array := &ast.ArrayLiteral{Token: p.curToken}
	array.Elements = p.parseExpressionList(token.RBRACKET)
	return array
}

func (p *Parser) parseHashLiteral() ast.Expression {
	hash := &ast.HashLiteral{Token: p.curToken}
	hash.Pairs = make(map[ast.Expression]ast.Expression)

	for p.peekToken.Type != token.RBRACE {
		p.nextToken()
		key := p.parseExpression(LOWEST)

		if p.peekToken.Type != token.COLON {
			p.Errors = append(p.Errors, fmt.Sprintf("Line %d, Col %d: expected :", p.peekToken.Line, p.peekToken.Col))
			return nil
		}
		p.nextToken() // move to colon
		p.nextToken() // past colon

		value := p.parseExpression(LOWEST)
		hash.Pairs[key] = value

		if p.peekToken.Type != token.RBRACE && p.peekToken.Type != token.COMMA {
			p.Errors = append(p.Errors, fmt.Sprintf("Line %d, Col %d: expected , or }", p.peekToken.Line, p.peekToken.Col))
			return nil
		}
		if p.peekToken.Type == token.COMMA {
			p.nextToken()
		}
	}
	p.nextToken()
	return hash
}

func (p *Parser) parseExpressionList(end token.TokenType) []ast.Expression {
	list := []ast.Expression{}
	if p.peekToken.Type == end {
		p.nextToken()
		return list
	}
	p.nextToken()
	list = append(list, p.parseExpression(LOWEST))
	for p.peekToken.Type == token.COMMA {
		p.nextToken()
		p.nextToken()
		list = append(list, p.parseExpression(LOWEST))
	}
	if p.peekToken.Type != end {
		return nil
	}
	p.nextToken()
	return list
}

func (p *Parser) parseFunctionLiteral() ast.Expression {
	lit := &ast.FunctionLiteral{Token: p.curToken}
	if p.peekToken.Type != token.LPAREN {
		return nil
	}
	p.nextToken()

	lit.Parameters = p.parseFunctionParameters()

	if p.peekToken.Type != token.LBRACE {
		return nil
	}
	p.nextToken()

	lit.Body = p.parseBlockStatement()
	return lit
}

func (p *Parser) parseFunctionParameters() []*ast.Identifier {
	identifiers := []*ast.Identifier{}
	if p.peekToken.Type == token.RPAREN {
		p.nextToken()
		return identifiers
	}
	p.nextToken()

	ident := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	identifiers = append(identifiers, ident)

	for p.peekToken.Type == token.COMMA {
		p.nextToken()
		p.nextToken()
		ident := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		identifiers = append(identifiers, ident)
	}
	if p.peekToken.Type != token.RPAREN {
		return nil
	}
	p.nextToken()
	return identifiers
}

func (p *Parser) parseInfixExpression(left ast.Expression) ast.Expression {
	expression := &ast.InfixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}
	precedence := p.curPrecedence()
	p.nextToken()
	expression.Right = p.parseExpression(precedence)
	return expression
}

func (p *Parser) parseCallExpression(function ast.Expression) ast.Expression {
	exp := &ast.CallExpression{Token: p.curToken, Function: function}
	exp.Arguments = p.parseExpressionList(token.RPAREN)
	return exp
}

func (p *Parser) parseIndexExpression(left ast.Expression) ast.Expression {
	exp := &ast.IndexExpression{Token: p.curToken, Left: left}
	p.nextToken()
	exp.Index = p.parseExpression(LOWEST)
	if p.peekToken.Type != token.RBRACKET {
		return nil
	}
	p.nextToken()
	return exp
}

func (p *Parser) parseFloatLiteral() ast.Expression {
	lit := &ast.FloatLiteral{Token: p.curToken}
	value, err := strconv.ParseFloat(p.curToken.Literal, 64)
	if err != nil {
		p.Errors = append(p.Errors, fmt.Sprintf("could not parse %q as float", p.curToken.Literal))
		return nil
	}
	lit.Value = value
	return lit
}

func (p *Parser) parsePostfixExpression(left ast.Expression) ast.Expression {
	return &ast.PostfixExpression{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}
}

func (p *Parser) parseMemberExpression(left ast.Expression) ast.Expression {
	exp := &ast.MemberExpression{Token: p.curToken, Object: left}

	p.nextToken() // move to member name
	if p.curToken.Type != token.IDENT {
		p.Errors = append(p.Errors, fmt.Sprintf("expected identifier after '.', got %s", p.curToken.Type))
		return nil
	}
	exp.Member = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	return exp
}

func (p *Parser) parsePipeExpression(left ast.Expression) ast.Expression {
	exp := &ast.PipeExpression{Token: p.curToken, Left: left}
	precedence := p.curPrecedence()
	p.nextToken()
	exp.Right = p.parseExpression(precedence)
	return exp
}

func (p *Parser) parseForStatement() ast.Statement {
	tok := p.curToken
	p.nextToken() // past for

	// for x in expr { ... }
	if p.curToken.Type == token.IDENT && p.peekToken.Type == token.IN {
		stmt := &ast.ForInStatement{Token: tok, Variable: &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}}
		p.nextToken() // past ident
		p.nextToken() // past in
		stmt.Iterable = p.parseExpression(LOWEST)
		if p.peekToken.Type != token.LBRACE {
			p.Errors = append(p.Errors, fmt.Sprintf("expected { for for-in body, got %s", p.peekToken.Type))
			return nil
		}
		p.nextToken()
		stmt.Body = p.parseBlockStatement()
		return stmt
	}

	// C-style for ( init ; condition ; update ) { ... }
	stmt := &ast.ForStatement{Token: tok}

	if p.curToken.Type != token.LPAREN {
		p.Errors = append(p.Errors, fmt.Sprintf("expected ( after for, got %s", p.curToken.Type))
		return nil
	}
	p.nextToken() // past (

	stmt.Init = p.parseStatement()
	if p.curToken.Type != token.SEMICOLON {
		// some statements like SetStatement consume their own semicolon if present
	}

	p.nextToken() // past ;
	stmt.Condition = p.parseExpression(LOWEST)
	p.nextToken() // past condition

	if p.curToken.Type != token.SEMICOLON {
		p.Errors = append(p.Errors, fmt.Sprintf("expected ; after for condition, got %s", p.curToken.Type))
		return nil
	}
	p.nextToken() // past ;

	stmt.Update = p.parseStatement()

	if p.peekToken.Type != token.RPAREN {
		p.Errors = append(p.Errors, fmt.Sprintf("expected ) after for update, got %s", p.peekToken.Type))
		return nil
	}
	p.nextToken() // to )

	if p.peekToken.Type != token.LBRACE {
		p.Errors = append(p.Errors, fmt.Sprintf("expected { for for-loop body, got %s", p.peekToken.Type))
		return nil
	}
	p.nextToken() // to {

	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseMatchExpression() ast.Expression {
	exp := &ast.MatchExpression{Token: p.curToken}
	p.nextToken() // past match

	exp.Value = p.parseExpression(LOWEST)

	if p.peekToken.Type != token.LBRACE {
		p.Errors = append(p.Errors, fmt.Sprintf("expected { after match expression, got %s", p.peekToken.Type))
		return nil
	}
	p.nextToken() // move to {

	for p.peekToken.Type != token.RBRACE && p.peekToken.Type != token.EOF {
		p.nextToken()
		mCase := &ast.MatchCase{}
		mCase.Pattern = p.parseExpression(LOWEST)

		if p.peekToken.Type != token.FAT_ARROW {
			p.Errors = append(p.Errors, fmt.Sprintf("expected => after pattern, got %s", p.peekToken.Type))
			return nil
		}
		p.nextToken() // to =>
		p.nextToken() // past =>

		if p.curToken.Type == token.LBRACE {
			mCase.Body = p.parseBlockStatement()
		} else {
			// support single statement
			stmt := p.parseStatement()
			mCase.Body = &ast.BlockStatement{Statements: []ast.Statement{stmt}}
		}
		exp.Cases = append(exp.Cases, mCase)

		if p.peekToken.Type == token.COMMA {
			p.nextToken()
		}
	}

	if p.peekToken.Type != token.RBRACE {
		p.Errors = append(p.Errors, "missing } in match expression")
		return nil
	}
	p.nextToken() // past }

	return exp
}

func (p *Parser) parseSpawnStatement() ast.Statement {
	stmt := &ast.SpawnStatement{Token: p.curToken}
	p.nextToken() // to next token

	exp := p.parseExpression(LOWEST)
	call, ok := exp.(*ast.CallExpression)
	if !ok {
		p.Errors = append(p.Errors, "spawn requires a function call")
		return nil
	}
	stmt.Call = call

	if p.peekToken.Type == token.SEMICOLON {
		p.nextToken()
	}
	return stmt
}

func (p *Parser) parseTryExpression() ast.Expression {
	exp := &ast.TryExpression{Token: p.curToken}
	if p.peekToken.Type != token.LBRACE {
		p.Errors = append(p.Errors, "expected { after try")
		return nil
	}
	p.nextToken()
	exp.Block = p.parseBlockStatement()

	if p.peekToken.Type != token.CATCH {
		p.Errors = append(p.Errors, "expected catch after try block")
		return nil
	}
	p.nextToken() // to catch

	if p.peekToken.Type == token.LPAREN {
		p.nextToken() // to (
		p.nextToken() // to ident
		if p.curToken.Type != token.IDENT {
			p.Errors = append(p.Errors, "expected identifier in catch")
			return nil
		}
		exp.CatchParameter = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		if p.peekToken.Type != token.RPAREN {
			p.Errors = append(p.Errors, "expected ) after catch parameter")
			return nil
		}
		p.nextToken() // to )
	}

	if p.peekToken.Type != token.LBRACE {
		p.Errors = append(p.Errors, "expected { after catch")
		return nil
	}
	p.nextToken()
	exp.CatchBlock = p.parseBlockStatement()

	return exp
}
