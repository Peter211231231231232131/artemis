package repl

import (
	"bufio"
	"xon/compiler"
	"xon/lexer"
	"xon/object"
	"xon/parser"
	"xon/vm"
	"fmt"
	"io"
	"sync"
)

const PROMPT = "xon>> "

func Start(in io.Reader, out io.Writer) {
	scanner := bufio.NewScanner(in)

	globals := make([]object.Object, vm.GlobalsSize)
	globalsMu := &sync.RWMutex{}
	comp := compiler.New()

	for {
		fmt.Fprintf(out, PROMPT)
		scanned := scanner.Scan()
		if !scanned {
			return
		}

		line := scanner.Text()
		if line == "exit" || line == "quit" {
			return
		}

		l := lexer.New(line)
		p := parser.New(l)
		program := p.ParseProgram()

		if len(p.Errors) > 0 {
			printParserErrors(out, p.Errors)
			continue
		}

		comp.ResetInstructions()
		err := comp.Compile(program)
		if err != nil {
			fmt.Fprintf(out, "Compiler error: %s\n", err)
			continue
		}

		bytecode := comp.Bytecode()

		machine := vm.NewWithGlobalsState(bytecode, globals, globalsMu)
		err = machine.Run()
		if err != nil {
			fmt.Fprintf(out, "VM error: %s\n", err)
			continue
		}

		stackTop := machine.LastPoppedStackElem()
		if stackTop != nil {
			io.WriteString(out, stackTop.Inspect())
			io.WriteString(out, "\n")
		}
	}
}

func printParserErrors(out io.Writer, errors []string) {
	io.WriteString(out, "Syntax Errors:\n")
	for _, msg := range errors {
		io.WriteString(out, "\t"+msg+"\n")
	}
}
