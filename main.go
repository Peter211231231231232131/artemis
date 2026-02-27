package main

import (
	"artemis/builtins"
	"artemis/compiler"
	"artemis/lexer"
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

	args := os.Args[1:]

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

	// Load standard library source
	stdSource := ""
	stdContent, err := builtins.LoadStdLib()
	if err == nil {
		stdSource = stdContent
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

	machine := vm.New(comp.Bytecode())
	err = machine.Run()
	if err != nil {
		fmt.Printf("VM error in %s: %s\n", scriptName, err)
		return
	}
}
