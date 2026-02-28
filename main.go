package main

import (
	"xon/builtins"
	"xon/compiler"
	"xon/lexer"
	"xon/object"
	"xon/parser"
	"xon/repl"
	"xon/vm"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"sync"
)

// normalizeScriptSource strips UTF-8 BOM and normalizes line endings to \n
// so that scripts parse the same whether saved with CRLF or LF.
func normalizeScriptSource(s string) string {
	const utf8BOM = "\xef\xbb\xbf"
	s = strings.TrimPrefix(s, utf8BOM)
	return strings.ReplaceAll(s, "\r\n", "\n")
}

var EmbeddedScript string

func main() {
	var source string
	var scriptName string

	args := os.Args[1:]
	disassemble := false
	if len(args) > 0 && args[0] == "-d" {
		disassemble = true
		args = args[1:]
	}

	if EmbeddedScript != "" {
		source = EmbeddedScript
		scriptName = "embedded"
	} else if len(args) < 1 {
		fmt.Println("Xon REPL")
		fmt.Println("Type your code below. Press Ctrl+C to exit.")
		repl.Start(os.Stdin, os.Stdout)
		return
	} else {
		input, err := ioutil.ReadFile(args[0])
		if err != nil {
			fmt.Println("Error reading file:", err)
			return
		}
		source = normalizeScriptSource(string(input))
		scriptName = args[0]
	}

	// Load standard library source
	stdSource := ""
	stdContent, err := builtins.LoadStdLib()
	if err == nil {
		stdSource = normalizeScriptSource(stdContent)
	}

	// Combine std + user source
	fullSource := stdSource + "\n" + source

	l := lexer.New(fullSource)
	p := parser.New(l)
	program := p.ParseProgram()

	if len(p.Errors) > 0 {
		fmt.Println("Syntax Errors:")
		for _, msg := range p.Errors {
			fmt.Println("\t" + msg)
		}
		return
	}

	comp := compiler.New()
	err = comp.Compile(program)
	if err != nil {
		fmt.Printf("Compiler error: %s\n", err)
		return
	}

	bytecode := comp.Bytecode()
	globals := make([]object.Object, vm.GlobalsSize)
	globalsMu := &sync.RWMutex{}

	// Initialize builtins with VM context
	builtins.SetVMContext(bytecode.Constants, globals, globalsMu)

	// Set up the web server callback
	builtins.RunClosureCallback = func(cl *object.Closure, args []object.Object) object.Object {
		// Create a temporary bytecode for this closure
		// We use the same constants but the closure's instructions
		subVm := vm.NewWithGlobalsState(&compiler.Bytecode{
			Constants:    bytecode.Constants,
			Instructions: cl.Fn.Instructions,
		}, globals, globalsMu)

		// Set up arguments and locals
		// This part is slightly simplified manually from OpCall logic
		frame := vm.NewFrame(cl, 0)
		subVm.SetFrame(0, frame)
		subVm.SetFrameIndex(1)

		for i, arg := range args {
			subVm.SetStack(i, arg)
		}
		subVm.SetStackPointer(cl.Fn.NumLocals)

		err := subVm.Run()
		if err != nil {
			return &object.Error{Message: err.Error()}
		}
		return subVm.LastPoppedStackElem()
	}

	if disassemble {
		fmt.Printf("Engine: Xon VM Disassembler\n")
		fmt.Printf("Constants:\n")
		for i, constant := range comp.Bytecode().Constants {
			fmt.Printf("  %d: %s\n", i, constant.Inspect())
		}
		fmt.Printf("\nInstructions:\n%s", comp.Bytecode().Instructions.String())
		return
	}

	machine := vm.NewWithGlobalsState(bytecode, globals, globalsMu)
	err = machine.Run()
	if err != nil {
		fmt.Printf("VM error in %s: %s\n", scriptName, err)
		return
	}
}
