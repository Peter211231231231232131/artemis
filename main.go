package main

import (
	"artemis/compiler"
	"artemis/evaluator"
	"artemis/lexer"
	"artemis/object"
	"artemis/parser"
	"artemis/repl"
	"artemis/vm"
	"fmt"
	"io/ioutil"
	"os"
)

var EmbeddedScript string

func main() {
	var source string
	var scriptName string
	var useVM bool

	args := os.Args[1:]
	if len(args) > 0 && args[0] == "-vm" {
		useVM = true
		args = args[1:]
	}

	if EmbeddedScript != "" {
		source = EmbeddedScript
		scriptName = "embedded"
	} else if len(args) < 1 {
		fmt.Println("Artemis Language REPL")
		fmt.Println("Type your code below. Press Ctrl+C to exit.")
		repl.Start(os.Stdin, os.Stdout)
		return
	} else {
		input, err := ioutil.ReadFile(args[0])
		if err != nil {
			fmt.Println("Error reading file:", err)
			return
		}
		source = string(input)
		scriptName = args[0]
	}

	l := lexer.New(source)
	p := parser.New(l)
	program := p.ParseProgram()

	if len(p.Errors) > 0 {
		fmt.Println("Syntax Errors:")
		for _, msg := range p.Errors {
			fmt.Println("\t" + msg)
		}
		return
	}

	if useVM {
		comp := compiler.New()
		err := comp.Compile(program)
		if err != nil {
			fmt.Printf("Compiler error: %s\n", err)
			return
		}

		machine := vm.New(comp.Bytecode())
		err = machine.Run()
		if err != nil {
			fmt.Printf("VM error: %s\n", err)
			return
		}
		return
	}

	env := object.NewEnvironment()
	evaluator.InitEnv(env)
	eval := evaluator.Eval(program, env)
	if eval != nil && eval.Type() == object.ERROR_OBJ {
		fmt.Printf("Traceback in %s:\n", scriptName)
		fmt.Println(eval.Inspect())
	}
}
