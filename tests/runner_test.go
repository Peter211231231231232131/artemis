package tests

import (
	"bytes"
	"exon/builtins"
	"exon/compiler"
	"exon/lexer"
	"exon/object"
	"exon/parser"
	"exon/vm"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// runSource runs Exon source (stdlib will be prepended) and returns stdout and any error.
func runSource(source string) (stdout string, runErr error) {
	stdContent, err := builtins.LoadStdLib()
	if err != nil {
		return "", err
	}
	fullSource := stdContent + "\n" + source

	l := lexer.New(fullSource)
	p := parser.New(l)
	program := p.ParseProgram()
	if len(p.Errors) > 0 {
		return "", &parseError{errors: p.Errors}
	}

	comp := compiler.New()
	if err := comp.Compile(program); err != nil {
		return "", err
	}

	bytecode := comp.Bytecode()
	globals := make([]object.Object, vm.GlobalsSize)
	globalsMu := &sync.RWMutex{}
	builtins.SetVMContext(bytecode.Constants, globals, globalsMu)

	builtins.RunClosureCallback = func(cl *object.Closure, args []object.Object) object.Object {
		subVm := vm.NewWithGlobalsState(&compiler.Bytecode{
			Constants:    bytecode.Constants,
			Instructions: cl.Fn.Instructions,
		}, globals, globalsMu)
		frame := vm.NewFrame(cl, 0)
		subVm.SetFrame(0, frame)
		subVm.SetFrameIndex(1)
		for i, arg := range args {
			subVm.SetStack(i, arg)
		}
		subVm.SetStackPointer(cl.Fn.NumLocals)
		if err := subVm.Run(); err != nil {
			return &object.Error{Message: err.Error()}
		}
		return subVm.LastPoppedStackElem()
	}

	// Capture stdout
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	var outBuf bytes.Buffer
	done := make(chan struct{})
	go func() {
		io.Copy(&outBuf, r)
		close(done)
	}()

	machine := vm.NewWithGlobalsState(bytecode, globals, globalsMu)
	runErr = machine.Run()
	w.Close()
	<-done
	return outBuf.String(), runErr
}

type parseError struct{ errors []string }

func (e *parseError) Error() string { return strings.Join(e.errors, "; ") }

func TestFeatures(t *testing.T) {
	path := filepath.Join("tests", "features.xn")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = "features.xn"
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read test script: %v", err)
	}
	stdout, runErr := runSource(string(content))
	if runErr != nil {
		t.Fatalf("run failed: %v\nstdout:\n%s", runErr, stdout)
	}
	if strings.Contains(stdout, "FAIL") {
		t.Errorf("output contains FAIL:\n%s", stdout)
	}
	if !strings.Contains(stdout, "All feature tests completed") {
		t.Errorf("test script did not complete; output:\n%s", stdout)
	}
	// Count PASS lines as sanity check
	passCount := strings.Count(stdout, "PASS:")
	if passCount < 40 {
		t.Errorf("expected at least 40 PASS lines, got %d\noutput:\n%s", passCount, stdout)
	}
	t.Logf("feature tests passed (%d PASS lines)", passCount)
}
