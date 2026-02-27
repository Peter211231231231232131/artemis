package compiler

import (
	"artemis/ast"
	"artemis/code"
	"artemis/object"
	"fmt"
)

type CompilationScope struct {
	instructions code.Instructions
}

type Compiler struct {
	constants   []object.Object
	symbolTable *SymbolTable
	scopes      []CompilationScope
	scopeIndex  int
}

type Bytecode struct {
	Instructions code.Instructions
	Constants    []object.Object
}

func New() *Compiler {
	mainScope := CompilationScope{
		instructions: code.Instructions{},
	}

	return &Compiler{
		constants:   []object.Object{},
		symbolTable: NewSymbolTable(),
		scopes:      []CompilationScope{mainScope},
		scopeIndex:  0,
	}
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

	case *ast.SetStatement:
		err := c.Compile(node.Value)
		if err != nil {
			return err
		}
		symbol := c.symbolTable.Define(node.Name.Value)
		if symbol.Scope == GlobalScope {
			c.emit(code.OpSetGlobal, symbol.Index)
		} else {
			c.emit(code.OpSetLocal, symbol.Index)
		}

	case *ast.AssignStatement:
		err := c.Compile(node.Value)
		if err != nil {
			return err
		}
		symbol, ok := c.symbolTable.Resolve(node.Name.Value)
		if !ok {
			return fmt.Errorf("undefined variable %s", node.Name.Value)
		}
		if symbol.Scope == GlobalScope {
			c.emit(code.OpSetGlobal, symbol.Index)
		} else {
			c.emit(code.OpSetLocal, symbol.Index)
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
		case ">":
			c.emit(code.OpGreaterThan)
		case "==":
			c.emit(code.OpEqual)
		case "!=":
			c.emit(code.OpNotEqual)
		default:
			return fmt.Errorf("unknown operator %s", node.Operator)
		}

	case *ast.IntegerLiteral:
		integer := &object.Integer{Value: node.Value}
		c.emit(code.OpConstant, c.addConstant(integer))

	case *ast.StringLiteral:
		str := &object.String{Value: node.Value}
		c.emit(code.OpString, c.addConstant(str))

	case *ast.Boolean:
		if node.Value {
			c.emit(code.OpTrue)
		} else {
			c.emit(code.OpFalse)
		}

	case *ast.Identifier:
		symbol, ok := c.symbolTable.Resolve(node.Value)
		if !ok {
			return fmt.Errorf("undefined variable %s", node.Value)
		}
		if symbol.Scope == GlobalScope {
			c.emit(code.OpGetGlobal, symbol.Index)
		} else {
			c.emit(code.OpGetLocal, symbol.Index)
		}

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
		instructions := c.leaveScope()

		compiledFn := &object.CompiledFunction{
			Instructions:  instructions,
			NumLocals:     numLocals,
			NumParameters: len(node.Parameters),
		}
		c.emit(code.OpConstant, c.addConstant(compiledFn))

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

	case *ast.IfStatement:
		err := c.Compile(node.Condition)
		if err != nil {
			return err
		}

		// Emit OpJumpNotTruthy with placeholder
		jumpNotTruthyPos := c.emit(code.OpJumpNotTruthy, 9999)

		err = c.Compile(node.Consequence)
		if err != nil {
			return err
		}

		if node.Alternative == nil {
			afterConsequencePos := len(c.currentInstructions())
			c.changeOperand(jumpNotTruthyPos, afterConsequencePos)
		} else {
			// Emit OpJump with placeholder to skip else
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

		err := c.Compile(node.Condition)
		if err != nil {
			return err
		}

		// Emit OpJumpNotTruthy with placeholder
		jumpNotTruthyPos := c.emit(code.OpJumpNotTruthy, 9999)

		err = c.Compile(node.Body)
		if err != nil {
			return err
		}

		// Emit OpJump to go back to condition
		c.emit(code.OpJump, beforeLoopPos)

		afterBodyPos := len(c.currentInstructions())
		c.changeOperand(jumpNotTruthyPos, afterBodyPos)

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

func (c *Compiler) Bytecode() *Bytecode {
	return &Bytecode{
		Instructions: c.currentInstructions(),
		Constants:    c.constants,
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
