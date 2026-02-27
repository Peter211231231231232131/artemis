package main

import (
	"artemis/evaluator"
	"artemis/lexer"
	"artemis/object"
	"artemis/parser"
	"artemis/repl"
	"fmt"
	"io/ioutil"
	"os"
)

var EmbeddedScript string

func main() {
	var source string
	var scriptName string

	if EmbeddedScript != "" {
		source = EmbeddedScript
		scriptName = "embedded"
	} else if len(os.Args) < 2 {
		fmt.Println("Artemis Language REPL")
		fmt.Println("Type your code below. Press Ctrl+C to exit.")
		repl.Start(os.Stdin, os.Stdout)
		return
	} else {
		input, err := ioutil.ReadFile(os.Args[1])
		if err != nil {
			fmt.Println("Error reading file:", err)
			return
		}
		source = string(input)
		scriptName = os.Args[1]
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

	env := object.NewEnvironment()
	evaluator.InitEnv(env)
	eval := evaluator.Eval(program, env)
	if eval != nil && eval.Type() == object.ERROR_OBJ {
		fmt.Printf("Traceback in %s:\n", scriptName)
		fmt.Println(eval.Inspect())
	}
}
