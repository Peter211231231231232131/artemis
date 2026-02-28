package compiler

import (
	"exon/ast"
	"exon/builtins"
	"exon/code"
	"exon/object"
	"fmt"
	"strings"
)

type CompilationScope struct {
	instructions code.Instructions
}

type loopContext struct {
	startPos        int
	breakPatches    []int
	continuePatches []int
}

type Compiler struct {
	constants   []object.Object
	symbolTable *SymbolTable
	scopes      []CompilationScope
	scopeIndex  int
	loopStack   []loopContext
}

type Bytecode struct {
	Instructions code.Instructions
	Constants    []object.Object
	SymbolTable  *SymbolTable
}

func New() *Compiler {
	symbolTable := NewSymbolTable()
	for i, name := range builtins.BuiltinNames {
		symbolTable.DefineBuiltin(i, name)
	}

	mainScope := CompilationScope{
		instructions: code.Instructions{},
	}

	return &Compiler{
		constants:   []object.Object{},
		symbolTable: symbolTable,
		scopes:      []CompilationScope{mainScope},
		scopeIndex:  0,
	}
}

func (c *Compiler) ResetInstructions() {
	c.scopes[c.scopeIndex].instructions = code.Instructions{}
}

func (c *Compiler) currentInstructions() code.Instructions {
	return c.scopes[c.scopeIndex].instructions
}

func (c *Compiler) enterScope() {
	scope := CompilationScope{
		instructions: code.Instructions{},
	}
	c.scopes = append(c.scopes, scope)
	c.scopeIndex++
	c.symbolTable = NewEnclosedSymbolTable(c.symbolTable)
}

func (c *Compiler) leaveScope() code.Instructions {
	instructions := c.currentInstructions()

	c.scopes = c.scopes[:len(c.scopes)-1]
	c.scopeIndex--
	c.symbolTable = c.symbolTable.Outer

	return instructions
}

func (c *Compiler) Compile(node ast.Node) error {
	switch node := node.(type) {
	case *ast.Program:
		for _, s := range node.Statements {
			err := c.Compile(s)
			if err != nil {
				return err
			}
		}

	case *ast.ExpressionStatement:
		err := c.Compile(node.Expression)
		if err != nil {
			return err
		}
		c.emit(code.OpPop)

	case *ast.OutStatement:
		err := c.Compile(node.Value)
		if err != nil {
			return err
		}
		c.emit(code.OpOut)

	case *ast.ReturnStatement:
		err := c.Compile(node.Value)
		if err != nil {
			return err
		}
		c.emit(code.OpReturnValue)

	case *ast.SpawnStatement:
		err := c.Compile(node.Call.Function)
		if err != nil {
			return err
		}

		for _, arg := range node.Call.Arguments {
			err := c.Compile(arg)
			if err != nil {
				return err
			}
		}

		c.emit(code.OpSpawn, len(node.Call.Arguments))

	case *ast.ImportStatement:
		err := c.Compile(node.Path)
		if err != nil {
			return err
		}

		c.emit(code.OpImport)

		var name string
		if node.Alias != nil {
			name = node.Alias.Value
		} else {
			// Extract name from path if no alias
			if str, ok := node.Path.(*ast.StringLiteral); ok {
				name = strings.TrimSuffix(str.Value, ".xn")
				// Handle paths like "std/math" -> "math"
				parts := strings.Split(name, "/")
				name = parts[len(parts)-1]
			}
		}

		if name != "" {
			symbol := c.symbolTable.Define(name)
			if symbol.Scope == GlobalScope {
				c.emit(code.OpSetGlobal, symbol.Index)
			} else {
				c.emit(code.OpSetLocal, symbol.Index)
			}
		}

	case *ast.SetStatement:
		err := c.Compile(node.Value)
		if err != nil {
			return err
		}
		var symbol Symbol
		if node.IsConst {
			symbol = c.symbolTable.DefineConst(node.Name.Value)
		} else {
			symbol = c.symbolTable.Define(node.Name.Value)
		}
		if symbol.Scope == GlobalScope {
			c.emit(code.OpSetGlobal, symbol.Index)
		} else if symbol.Scope == LocalScope {
			c.emit(code.OpSetLocal, symbol.Index)
		} else if symbol.Scope == FreeScope {
			c.emit(code.OpSetFree, symbol.Index)
		}

	case *ast.ThrowStatement:
		err := c.Compile(node.Value)
		if err != nil {
			return err
		}
		c.emit(code.OpThrow)

	case *ast.AssignStatement:
		err := c.Compile(node.Value)
		if err != nil {
			return err
		}
		symbol, ok := c.symbolTable.Resolve(node.Name.Value)
		if !ok {
			return fmt.Errorf("undefined variable %s", node.Name.Value)
		}
		if symbol.IsConst {
			return fmt.Errorf("cannot assign to constant %s", node.Name.Value)
		}
		if symbol.Scope == GlobalScope {
			c.emit(code.OpSetGlobal, symbol.Index)
		} else if symbol.Scope == LocalScope {
			c.emit(code.OpSetLocal, symbol.Index)
		} else if symbol.Scope == FreeScope {
			c.emit(code.OpSetFree, symbol.Index)
		}

	case *ast.InfixExpression:
		if node.Operator == "<" {
			err := c.Compile(node.Right)
			if err != nil {
				return err
			}

			err = c.Compile(node.Left)
			if err != nil {
				return err
			}

			c.emit(code.OpGreaterThan)
			return nil
		}
		if node.Operator == "&&" {
			err := c.Compile(node.Left)
			if err != nil {
				return err
			}
			jumpPos := c.emit(code.OpJumpNotTruthy, 9999)
			err = c.Compile(node.Right)
			if err != nil {
				return err
			}
			c.changeOperand(jumpPos, len(c.currentInstructions()))
			return nil
		}
		if node.Operator == "||" {
			err := c.Compile(node.Left)
			if err != nil {
				return err
			}
			jumpPos := c.emit(code.OpJumpTruthy, 9999)
			err = c.Compile(node.Right)
			if err != nil {
				return err
			}
			c.changeOperand(jumpPos, len(c.currentInstructions()))
			return nil
		}

		err := c.Compile(node.Left)
		if err != nil {
			return err
		}

		err = c.Compile(node.Right)
		if err != nil {
			return err
		}

		switch node.Operator {
		case "+":
			c.emit(code.OpAdd)
		case "-":
			c.emit(code.OpSub)
		case "*":
			c.emit(code.OpMul)
		case "/":
			c.emit(code.OpDiv)
		case "%":
			c.emit(code.OpMod)
		case ">":
			c.emit(code.OpGreaterThan)
		case "==":
			c.emit(code.OpEqual)
		case "!=":
			c.emit(code.OpNotEqual)
		case "&":
			c.emit(code.OpBitAnd)
		case "|":
			c.emit(code.OpBitOr)
		case "^":
			c.emit(code.OpBitXor)
		case "<<":
			c.emit(code.OpLshift)
		case ">>":
			c.emit(code.OpRshift)
		default:
			return fmt.Errorf("unknown operator %s", node.Operator)
		}

	case *ast.PrefixExpression:
		err := c.Compile(node.Right)
		if err != nil {
			return err
		}
		switch node.Operator {
		case "!":
			c.emit(code.OpBang)
		case "-":
			c.emit(code.OpMinus)
		case "~":
			c.emit(code.OpBitNot)
		default:
			return fmt.Errorf("unknown prefix operator %s", node.Operator)
		}

	case *ast.PostfixExpression:
		ident, ok := node.Left.(*ast.Identifier)
		if !ok {
			return fmt.Errorf("++ and -- require a variable (identifier)")
		}
		symbol, ok := c.symbolTable.Resolve(ident.Value)
		if !ok {
			return fmt.Errorf("undefined variable %s", ident.Value)
		}
		if symbol.IsConst {
			return fmt.Errorf("cannot modify constant %s", ident.Value)
		}
		c.loadSymbol(symbol)
		c.emit(code.OpDup)
		c.emit(code.OpConstant, c.addConstant(&object.Integer{Value: 1}))
		if node.Operator == "++" {
			c.emit(code.OpAdd)
		} else {
			c.emit(code.OpSub)
		}
		switch symbol.Scope {
		case GlobalScope:
			c.emit(code.OpSetGlobal, symbol.Index)
		case LocalScope:
			c.emit(code.OpSetLocal, symbol.Index)
		case FreeScope:
			c.emit(code.OpSetFree, symbol.Index)
		default:
			return fmt.Errorf("cannot assign to %s", ident.Value)
		}

	case *ast.IntegerLiteral:
		integer := &object.Integer{Value: node.Value}
		c.emit(code.OpConstant, c.addConstant(integer))

	case *ast.FloatLiteral:
		fl := &object.Float{Value: node.Value}
		c.emit(code.OpConstant, c.addConstant(fl))

	case *ast.StringLiteral:
		str := &object.String{Value: node.Value}
		c.emit(code.OpString, c.addConstant(str))

	case *ast.InterpolatedString:
		// Compile each part and concatenate with +
		if len(node.Parts) == 0 {
			c.emit(code.OpString, c.addConstant(&object.String{Value: ""}))
			return nil
		}
		for i, part := range node.Parts {
			err := c.Compile(part)
			if err != nil {
				return err
			}
			if i > 0 {
				c.emit(code.OpAdd)
			}
		}

	case *ast.Boolean:
		if node.Value {
			c.emit(code.OpTrue)
		} else {
			c.emit(code.OpFalse)
		}

	case *ast.ArrayLiteral:
		for _, el := range node.Elements {
			err := c.Compile(el)
			if err != nil {
				return err
			}
		}
		c.emit(code.OpArray, len(node.Elements))

	case *ast.HashLiteral:
		keys := []ast.Expression{}
		vals := []ast.Expression{}
		for k, v := range node.Pairs {
			keys = append(keys, k)
			vals = append(vals, v)
		}
		for i := 0; i < len(keys); i++ {
			err := c.Compile(keys[i])
			if err != nil {
				return err
			}
			err = c.Compile(vals[i])
			if err != nil {
				return err
			}
		}
		c.emit(code.OpHash, len(keys)*2)

	case *ast.IndexExpression:
		err := c.Compile(node.Left)
		if err != nil {
			return err
		}
		err = c.Compile(node.Index)
		if err != nil {
			return err
		}
		c.emit(code.OpIndex)

	case *ast.MemberExpression:
		err := c.Compile(node.Object)
		if err != nil {
			return err
		}
		memberStr := &object.String{Value: node.Member.Value}
		c.emit(code.OpMember, c.addConstant(memberStr))

	case *ast.Identifier:
		symbol, ok := c.symbolTable.Resolve(node.Value)
		if !ok {
			return fmt.Errorf("undefined variable %s", node.Value)
		}
		c.loadSymbol(symbol)

	case *ast.FunctionLiteral:
		c.enterScope()

		for _, p := range node.Parameters {
			c.symbolTable.Define(p.Value)
		}

		err := c.Compile(node.Body)
		if err != nil {
			return err
		}

		// If the last instruction isn't a return, add implicit return null
		ins := c.currentInstructions()
		if len(ins) == 0 || ins[len(ins)-1] != byte(code.OpReturnValue) {
			c.emit(code.OpReturn)
		}

		numLocals := c.symbolTable.numDefinitions
		freeSymbols := c.symbolTable.FreeSymbols
		instructions := c.leaveScope()

		for _, s := range freeSymbols {
			c.loadSymbol(s)
		}

		compiledFn := &object.CompiledFunction{
			Instructions:  instructions,
			NumLocals:     numLocals,
			NumParameters: len(node.Parameters),
		}

		fnIndex := c.addConstant(compiledFn)
		c.emit(code.OpClosure, fnIndex, len(freeSymbols))

	case *ast.CallExpression:
		err := c.Compile(node.Function)
		if err != nil {
			return err
		}

		for _, a := range node.Arguments {
			err := c.Compile(a)
			if err != nil {
				return err
			}
		}

		c.emit(code.OpCall, len(node.Arguments))

	case *ast.TryExpression:
		catchEmitPos := c.emit(code.OpCatch, 9999)
		err := c.compileBlockPreservingLast(node.Block)
		if err != nil {
			return err
		}
		c.emit(code.OpEndCatch)
		jumpOverPos := c.emit(code.OpJump, 9999)
		catchProloguePos := len(c.currentInstructions())
		if node.CatchParameter != nil {
			c.enterScope()
			paramSym := c.symbolTable.Define(node.CatchParameter.Value)
			c.emit(code.OpSetLocal, paramSym.Index)
		} else {
			c.emit(code.OpPop)
		}
		err = c.compileBlockPreservingLast(node.CatchBlock)
		if err != nil {
			if node.CatchParameter != nil {
				c.leaveScope()
			}
			return err
		}
		if node.CatchParameter != nil {
			c.leaveScope()
		}
		afterCatchPos := len(c.currentInstructions())
		c.changeOperand(catchEmitPos, catchProloguePos)
		c.changeOperand(jumpOverPos, afterCatchPos)

	case *ast.IfStatement:
		err := c.Compile(node.Condition)
		if err != nil {
			return err
		}

		jumpNotTruthyPos := c.emit(code.OpJumpNotTruthy, 9999)

		err = c.Compile(node.Consequence)
		if err != nil {
			return err
		}

		if node.Alternative == nil {
			afterConsequencePos := len(c.currentInstructions())
			c.changeOperand(jumpNotTruthyPos, afterConsequencePos)
		} else {
			jumpPos := c.emit(code.OpJump, 9999)

			afterConsequencePos := len(c.currentInstructions())
			c.changeOperand(jumpNotTruthyPos, afterConsequencePos)

			err = c.Compile(node.Alternative)
			if err != nil {
				return err
			}

			afterAlternativePos := len(c.currentInstructions())
			c.changeOperand(jumpPos, afterAlternativePos)
		}

	case *ast.WhileStatement:
		beforeLoopPos := len(c.currentInstructions())
		c.loopStack = append(c.loopStack, loopContext{startPos: beforeLoopPos})

		err := c.Compile(node.Condition)
		if err != nil {
			c.loopStack = c.loopStack[:len(c.loopStack)-1]
			return err
		}

		jumpNotTruthyPos := c.emit(code.OpJumpNotTruthy, 9999)

		err = c.Compile(node.Body)
		if err != nil {
			c.loopStack = c.loopStack[:len(c.loopStack)-1]
			return err
		}

		c.emit(code.OpJump, beforeLoopPos)

		afterBodyPos := len(c.currentInstructions())
		c.changeOperand(jumpNotTruthyPos, afterBodyPos)
		c.patchLoopExits(afterBodyPos)
		c.loopStack = c.loopStack[:len(c.loopStack)-1]

	case *ast.ForStatement:
		if node.Init != nil {
			err := c.Compile(node.Init)
			if err != nil {
				return err
			}
		}
		beforeCondPos := len(c.currentInstructions())
		c.loopStack = append(c.loopStack, loopContext{startPos: beforeCondPos})

		err := c.Compile(node.Condition)
		if err != nil {
			c.loopStack = c.loopStack[:len(c.loopStack)-1]
			return err
		}
		jumpNotTruthyPos := c.emit(code.OpJumpNotTruthy, 9999)

		err = c.Compile(node.Body)
		if err != nil {
			c.loopStack = c.loopStack[:len(c.loopStack)-1]
			return err
		}
		if node.Update != nil {
			err = c.Compile(node.Update)
			if err != nil {
				c.loopStack = c.loopStack[:len(c.loopStack)-1]
				return err
			}
		}
		c.emit(code.OpJump, beforeCondPos)
		afterBodyPos := len(c.currentInstructions())
		c.changeOperand(jumpNotTruthyPos, afterBodyPos)
		c.patchLoopExits(afterBodyPos)
		c.loopStack = c.loopStack[:len(c.loopStack)-1]

	case *ast.ForInStatement:
		c.enterScope()
		err := c.Compile(node.Iterable)
		if err != nil {
			c.leaveScope()
			return err
		}
		iterSym := c.symbolTable.Define("__for_iter")
		c.emit(code.OpSetLocal, iterSym.Index)
		idxSym := c.symbolTable.Define("__for_idx")
		c.symbolTable.Define(node.Variable.Value)
		c.emit(code.OpConstant, c.addConstant(&object.Integer{Value: 0}))
		c.emit(code.OpSetLocal, idxSym.Index)

		beforeLoopPos := len(c.currentInstructions())
		c.loopStack = append(c.loopStack, loopContext{startPos: beforeLoopPos})

		// condition: __for_idx < __for_iter.len()
		c.emit(code.OpGetLocal, iterSym.Index)
		c.emit(code.OpMember, c.addConstant(&object.String{Value: "len"}))
		c.emit(code.OpCall, 0)
		c.emit(code.OpGetLocal, idxSym.Index)
		c.emit(code.OpGreaterThan) // length > index  =>  index < length
		jumpNotTruthyPos := c.emit(code.OpJumpNotTruthy, 9999)

		// loop var = iterable[index]
		c.emit(code.OpGetLocal, iterSym.Index)
		c.emit(code.OpGetLocal, idxSym.Index)
		c.emit(code.OpIndex)
		loopVarSym, _ := c.symbolTable.Resolve(node.Variable.Value)
		c.emit(code.OpSetLocal, loopVarSym.Index)

		err = c.Compile(node.Body)
		if err != nil {
			c.loopStack = c.loopStack[:len(c.loopStack)-1]
			c.leaveScope()
			return err
		}

		// index++
		c.emit(code.OpGetLocal, idxSym.Index)
		c.emit(code.OpConstant, c.addConstant(&object.Integer{Value: 1}))
		c.emit(code.OpAdd)
		c.emit(code.OpSetLocal, idxSym.Index)

		c.emit(code.OpJump, beforeLoopPos)
		afterBodyPos := len(c.currentInstructions())
		c.changeOperand(jumpNotTruthyPos, afterBodyPos)
		c.patchLoopExits(afterBodyPos)
		c.loopStack = c.loopStack[:len(c.loopStack)-1]
		c.leaveScope()

	case *ast.BreakStatement:
		if len(c.loopStack) == 0 {
			return fmt.Errorf("break outside of loop")
		}
		pos := c.emit(code.OpJump, 9999)
		c.loopStack[len(c.loopStack)-1].breakPatches = append(c.loopStack[len(c.loopStack)-1].breakPatches, pos)

	case *ast.ContinueStatement:
		if len(c.loopStack) == 0 {
			return fmt.Errorf("continue outside of loop")
		}
		pos := c.emit(code.OpJump, 9999)
		c.loopStack[len(c.loopStack)-1].continuePatches = append(c.loopStack[len(c.loopStack)-1].continuePatches, pos)

	case *ast.BlockStatement:
		for _, s := range node.Statements {
			err := c.Compile(s)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (c *Compiler) loadSymbol(s Symbol) {
	switch s.Scope {
	case GlobalScope:
		c.emit(code.OpGetGlobal, s.Index)
	case LocalScope:
		c.emit(code.OpGetLocal, s.Index)
	case BuiltinScope:
		c.emit(code.OpGetBuiltin, s.Index)
	case FreeScope:
		c.emit(code.OpGetFree, s.Index)
	}
}

func (c *Compiler) Bytecode() *Bytecode {
	return &Bytecode{
		Instructions: c.currentInstructions(),
		Constants:    c.constants,
		SymbolTable:  c.symbolTable,
	}
}

func (c *Compiler) addConstant(obj object.Object) int {
	c.constants = append(c.constants, obj)
	return len(c.constants) - 1
}

func (c *Compiler) emit(op code.Opcode, operands ...int) int {
	ins := code.Make(op, operands...)
	pos := c.addInstruction(ins)
	return pos
}

func (c *Compiler) addInstruction(ins []byte) int {
	posNewInstruction := len(c.currentInstructions())
	updatedInstructions := append(c.currentInstructions(), ins...)
	c.scopes[c.scopeIndex].instructions = updatedInstructions
	return posNewInstruction
}

func (c *Compiler) changeOperand(opPos int, operand int) {
	op := code.Opcode(c.currentInstructions()[opPos])

	newInstruction := code.Make(op, operand)

	for i := 0; i < len(newInstruction); i++ {
		c.scopes[c.scopeIndex].instructions[opPos+i] = newInstruction[i]
	}
}

// compileBlockPreservingLast compiles a block; if the last statement is an expression, its value is left on stack.
func (c *Compiler) compileBlockPreservingLast(block *ast.BlockStatement) error {
	stmts := block.Statements
	for i, stmt := range stmts {
		isLast := i == len(stmts)-1
		if isLast {
			if es, ok := stmt.(*ast.ExpressionStatement); ok {
				return c.Compile(es.Expression)
			}
		}
		if err := c.Compile(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (c *Compiler) patchLoopExits(afterPos int) {
	if len(c.loopStack) == 0 {
		return
	}
	ctx := &c.loopStack[len(c.loopStack)-1]
	for _, pos := range ctx.breakPatches {
		c.changeOperand(pos, afterPos)
	}
	for _, pos := range ctx.continuePatches {
		c.changeOperand(pos, ctx.startPos)
	}
}
