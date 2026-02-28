package vm

import (
	"encoding/binary"
	"exon/builtins"
	"exon/code"
	"exon/compiler"
	"exon/lexer"
	"exon/object"
	"exon/parser"
	"fmt"
	"io/ioutil"
	"math"
	"strings"
	"sync"
)

const (
	StackSize   = 2048
	GlobalsSize = 65536
	MaxFrames   = 1024
)

type Frame struct {
	cl          *object.Closure
	ip          int
	basePointer int
}

func NewFrame(cl *object.Closure, basePointer int) *Frame {
	return &Frame{
		cl:          cl,
		ip:          -1,
		basePointer: basePointer,
	}
}

func (f *Frame) Instructions() code.Instructions {
	return f.cl.Fn.Instructions
}

type VM struct {
	constants []object.Object

	stack     []object.Object
	sp        int
	globals   []object.Object
	globalsMu *sync.RWMutex

	frames        []*Frame
	frameIndex    int
	modules       map[string]*object.Hash
	catchHandlers []int
}

func New(bytecode *compiler.Bytecode) *VM {
	mainFn := &object.CompiledFunction{Instructions: bytecode.Instructions}
	mainClosure := &object.Closure{Fn: mainFn}
	mainFrame := NewFrame(mainClosure, 0)

	frames := make([]*Frame, MaxFrames)
	frames[0] = mainFrame

	return &VM{
		constants:      bytecode.Constants,
		stack:          make([]object.Object, StackSize),
		sp:             0,
		globals:        make([]object.Object, GlobalsSize),
		globalsMu:       &sync.RWMutex{},
		frames:         frames,
		frameIndex:     1,
		modules:        make(map[string]*object.Hash),
		catchHandlers:  make([]int, 0, 8),
	}
}

func NewWithGlobalsState(bytecode *compiler.Bytecode, globals []object.Object, mu *sync.RWMutex) *VM {
	vm := New(bytecode)
	vm.globals = globals
	vm.globalsMu = mu
	return vm
}

func (vm *VM) currentFrame() *Frame {
	if vm.frameIndex <= 0 {
		return nil
	}
	return vm.frames[vm.frameIndex-1]
}

// getConstants returns the constants for the current frame.
// If the frame's closure has its own constants (imported module), use those.
// Otherwise, fall back to the VM's main constants.
func (vm *VM) getConstants() []object.Object {
	frame := vm.currentFrame()
	if frame != nil && frame.cl.Fn.Constants != nil {
		return frame.cl.Fn.Constants
	}
	return vm.constants
}

func (vm *VM) pushFrame(f *Frame) {
	vm.frames[vm.frameIndex] = f
	vm.frameIndex++
}

func (vm *VM) popFrame() *Frame {
	vm.frameIndex--
	return vm.frames[vm.frameIndex]
}

func (vm *VM) StackTop() object.Object {
	if vm.sp == 0 {
		return nil
	}
	return vm.stack[vm.sp-1]
}

func (vm *VM) Run() error {
	var ip int
	var ins code.Instructions
	var op code.Opcode

	for vm.frameIndex > 0 {
		frame := vm.currentFrame()
		if frame.ip >= len(frame.Instructions())-1 {
			break
		}
		frame.ip++

		ip = frame.ip
		ins = frame.Instructions()
		op = code.Opcode(ins[ip])

		switch op {
		case code.OpConstant:
			constIndex := binary.BigEndian.Uint16(ins[ip+1:])
			frame.ip += 2
			if err := vm.push(vm.getConstants()[constIndex]); err != nil {
				return err
			}

		case code.OpString:
			constIndex := binary.BigEndian.Uint16(ins[ip+1:])
			frame.ip += 2
			if err := vm.push(vm.getConstants()[constIndex]); err != nil {
				return err
			}

		case code.OpAdd, code.OpSub, code.OpMul, code.OpDiv,
			code.OpGreaterThan, code.OpEqual, code.OpNotEqual:
			if err := vm.executeBinaryOperation(op); err != nil {
				return err
			}

		case code.OpMinus:
			operand := vm.pop()
			switch obj := operand.(type) {
			case *object.Integer:
				vm.push(&object.Integer{Value: -obj.Value})
			case *object.Float:
				vm.push(&object.Float{Value: -obj.Value})
			default:
				return fmt.Errorf("unsupported type for negation: %s", obj.Type())
			}

		case code.OpBang:
			operand := vm.pop()
			if isTruthy(operand) {
				vm.push(&object.Boolean{Value: false})
			} else {
				vm.push(&object.Boolean{Value: true})
			}

		case code.OpBitNot:
			operand := vm.pop()
			obj, ok := operand.(*object.Integer)
			if !ok {
				return fmt.Errorf("bitwise NOT requires integer, got %s", operand.Type())
			}
			if err := vm.push(&object.Integer{Value: ^obj.Value}); err != nil {
				return err
			}

		case code.OpTrue:
			if err := vm.push(&object.Boolean{Value: true}); err != nil {
				return err
			}

		case code.OpFalse:
			if err := vm.push(&object.Boolean{Value: false}); err != nil {
				return err
			}

		case code.OpNull:
			if err := vm.push(&object.Null{}); err != nil {
				return err
			}

		case code.OpOut:
			val := vm.pop()
			fmt.Println(val.Inspect())

		case code.OpGetGlobal:
			globalIndex := binary.BigEndian.Uint16(ins[ip+1:])
			frame.ip += 2
			vm.globalsMu.RLock()
			val := vm.globals[globalIndex]
			vm.globalsMu.RUnlock()
			if err := vm.push(val); err != nil {
				return err
			}

		case code.OpSetGlobal:
			globalIndex := binary.BigEndian.Uint16(ins[ip+1:])
			frame.ip += 2
			val := vm.pop()
			vm.globalsMu.Lock()
			vm.globals[globalIndex] = val
			vm.globalsMu.Unlock()

		case code.OpGetLocal:
			localIndex := int(ins[ip+1])
			frame.ip += 1
			if err := vm.push(vm.stack[frame.basePointer+localIndex]); err != nil {
				return err
			}

		case code.OpSetLocal:
			localIndex := int(ins[ip+1])
			frame.ip += 1
			vm.stack[frame.basePointer+localIndex] = vm.pop()

		case code.OpGetBuiltin:
			builtinIndex := int(ins[ip+1])
			frame.ip += 1
			builtin := builtins.GetBuiltinByIndex(builtinIndex)
			if builtin == nil {
				return fmt.Errorf("builtin function not found at index %d", builtinIndex)
			}
			if err := vm.push(builtin); err != nil {
				return err
			}

		case code.OpArray:
			numElements := int(binary.BigEndian.Uint16(ins[ip+1:]))
			frame.ip += 2
			array := vm.buildArray(vm.sp-numElements, vm.sp)
			vm.sp = vm.sp - numElements
			if err := vm.push(array); err != nil {
				return err
			}

		case code.OpHash:
			numElements := int(binary.BigEndian.Uint16(ins[ip+1:]))
			frame.ip += 2
			hash, err := vm.buildHash(vm.sp-numElements, vm.sp)
			if err != nil {
				return err
			}
			vm.sp = vm.sp - numElements
			if err := vm.push(hash); err != nil {
				return err
			}

		case code.OpIndex:
			index := vm.pop()
			left := vm.pop()
			if err := vm.executeIndexExpression(left, index); err != nil {
				return err
			}

		case code.OpMember:
			constIndex := binary.BigEndian.Uint16(ins[ip+1:])
			frame.ip += 2
			memberName := vm.getConstants()[constIndex].(*object.String).Value
			obj := vm.pop()
			if err := vm.executeMemberExpression(obj, memberName); err != nil {
				return err
			}

		case code.OpJump:
			pos := int(binary.BigEndian.Uint16(ins[ip+1:]))
			vm.currentFrame().ip = pos - 1

		case code.OpJumpNotTruthy:
			pos := int(binary.BigEndian.Uint16(ins[ip+1:]))
			frame.ip += 2
			condition := vm.pop()
			if !isTruthy(condition) {
				vm.currentFrame().ip = pos - 1
			}

		case code.OpJumpTruthy:
			pos := int(binary.BigEndian.Uint16(ins[ip+1:]))
			frame.ip += 2
			condition := vm.StackTop()
			if isTruthy(condition) {
				vm.currentFrame().ip = pos - 1
			}

		case code.OpDup:
			if vm.sp == 0 {
				return fmt.Errorf("stack empty for OpDup")
			}
			vm.push(vm.stack[vm.sp-1])

		case code.OpCatch:
			pos := int(binary.BigEndian.Uint16(ins[ip+1:]))
			frame.ip += 2
			vm.catchHandlers = append(vm.catchHandlers, pos)

		case code.OpThrow:
			if vm.sp == 0 {
				return fmt.Errorf("throw with empty stack")
			}
			if len(vm.catchHandlers) == 0 {
				errObj := vm.pop()
				return fmt.Errorf("uncaught throw: %s", errObj.Inspect())
			}
			thrown := vm.pop()
			handlerPos := vm.catchHandlers[len(vm.catchHandlers)-1]
			vm.catchHandlers = vm.catchHandlers[:len(vm.catchHandlers)-1]
			vm.push(thrown)
			vm.currentFrame().ip = handlerPos - 1

		case code.OpEndCatch:
			if len(vm.catchHandlers) == 0 {
				return fmt.Errorf("OpEndCatch without OpCatch")
			}
			vm.catchHandlers = vm.catchHandlers[:len(vm.catchHandlers)-1]

		case code.OpCall:
			numArgs := int(ins[ip+1])
			frame.ip += 1
			callee := vm.stack[vm.sp-1-numArgs]

			switch cl := callee.(type) {
			case *object.Closure:
				if numArgs != cl.Fn.NumParameters {
					return fmt.Errorf("wrong number of arguments: want=%d, got=%d",
						cl.Fn.NumParameters, numArgs)
				}
				frame := NewFrame(cl, vm.sp-numArgs)
				vm.pushFrame(frame)
				vm.sp = frame.basePointer + cl.Fn.NumLocals

			case *object.Builtin:
				args := vm.stack[vm.sp-numArgs : vm.sp]
				result := cl.Fn(args...)
				vm.sp = vm.sp - numArgs - 1
				if result != nil {
					vm.push(result)
				} else {
					vm.push(&object.Null{})
				}

			default:
				return fmt.Errorf("calling non-function: %s", callee.Type())
			}

		case code.OpSpawn:
			numArgs := int(ins[ip+1])
			frame.ip += 1

			args := make([]object.Object, numArgs)
			for i := numArgs - 1; i >= 0; i-- {
				args[i] = vm.pop()
			}

			target := vm.pop()
			var cl *object.Closure

			switch t := target.(type) {
			case *object.Closure:
				cl = t
			case *object.CompiledFunction:
				cl = &object.Closure{Fn: t}
			default:
				return fmt.Errorf("spawn target must be a function, got %s", target.Type())
			}

			go func() {
				defer func() {
					if r := recover(); r != nil {
						fmt.Printf("Recovered in spawn goroutine: %v\n", r)
					}
				}()
				subVm := &VM{
					constants:  vm.constants,
					globals:    vm.globals,
					globalsMu:  vm.globalsMu,
					stack:      make([]object.Object, StackSize),
					sp:         0,
					frames:     make([]*Frame, MaxFrames),
					frameIndex: 1,
				}

				newFrame := NewFrame(cl, 0)
				subVm.frames[0] = newFrame

				for i, arg := range args {
					subVm.stack[i] = arg
				}
				subVm.sp = cl.Fn.NumLocals

				err := subVm.Run()
				if err != nil {
					fmt.Printf("Sub-VM error: %s\n", err)
				}
			}()

		case code.OpClosure:
			constIndex := binary.BigEndian.Uint16(ins[ip+1:])
			numFree := int(ins[ip+3])
			frame.ip += 3

			err := vm.pushClosure(int(constIndex), numFree)
			if err != nil {
				return err
			}

		case code.OpGetFree:
			freeIndex := int(ins[ip+1])
			frame.ip += 1

			err := vm.push(frame.cl.Free[freeIndex])
			if err != nil {
				return err
			}

		case code.OpSetFree:
			freeIndex := int(ins[ip+1])
			frame.ip += 1

			val := vm.pop()
			frame.cl.Free[freeIndex] = val

		case code.OpReturnValue:
			returnValue := vm.pop()
			frame := vm.popFrame()
			if vm.frameIndex == 0 {
				vm.sp = 0
				vm.push(returnValue)
				return nil
			}
			vm.sp = frame.basePointer - 1
			if err := vm.push(returnValue); err != nil {
				return err
			}

		case code.OpReturn:
			frame := vm.popFrame()
			if vm.frameIndex == 0 {
				vm.sp = 0
				vm.push(&object.Null{})
				return nil
			}
			vm.sp = frame.basePointer - 1
			if err := vm.push(&object.Null{}); err != nil {
				return err
			}

		case code.OpPop:
			vm.pop()

		case code.OpImport:
			pathObj := vm.pop()
			path, ok := pathObj.(*object.String)
			if !ok {
				return fmt.Errorf("import path must be string, got %s", pathObj.Type())
			}

			modulePath := path.Value
			if !strings.HasSuffix(modulePath, ".xn") {
				modulePath += ".xn"
			}

			if mod, ok := vm.modules[modulePath]; ok {
				if err := vm.push(mod); err != nil {
					return err
				}
				continue
			}

			// Load and compile
			content, err := ioutil.ReadFile(modulePath)
			if err != nil {
				return fmt.Errorf("could not read import file %s: %s", modulePath, err)
			}

			// Prepend standard library so modules have access to it
			stdSource, err := builtins.LoadStdLib()
			if err != nil {
				fmt.Printf("Warning: could not load stdlib for import: %v\n", err)
			}
			fullSource := stdSource + "\n" + string(content)

			l := lexer.New(fullSource)
			p := parser.New(l)
			program := p.ParseProgram()
			if len(p.Errors) != 0 {
				return fmt.Errorf("import parse error: %v", p.Errors)
			}

			c := compiler.New()
			err = c.Compile(program)
			if err != nil {
				return fmt.Errorf("import compile error: %s", err)
			}

			bytecode := c.Bytecode()
			// Run in sub-VM
			subVm := New(bytecode)
			subVm.modules = vm.modules

			err = subVm.Run()
			if err != nil {
				return fmt.Errorf("import runtime error: %s", err)
			}

			// Export all globals as a Hash
			exportHash := &object.Hash{Pairs: make(map[object.HashKey]object.HashPair)}
			moduleConstants := bytecode.Constants
			for _, sym := range bytecode.SymbolTable.Symbols() {
				if sym.Scope == compiler.GlobalScope {
					val := subVm.globals[sym.Index]
					if val != nil {
						// Attach module constants to closures so they work in the main VM
						vm.attachConstants(val, moduleConstants)
						key := &object.String{Value: sym.Name}
						exportHash.Pairs[key.HashKey()] = object.HashPair{Key: key, Value: val}
					}
				}
			}

			vm.modules[modulePath] = exportHash
			if err := vm.push(exportHash); err != nil {
				return err
			}
		}
	}

	return nil
}

func (vm *VM) executeBinaryOperation(op code.Opcode) error {
	right := vm.pop()
	left := vm.pop()

	// Integer operations
	leftInt, ok1 := left.(*object.Integer)
	rightInt, ok2 := right.(*object.Integer)
	if ok1 && ok2 {
		return vm.executeIntegerBinaryOp(op, leftInt.Value, rightInt.Value)
	}

	// Float operations
	var leftF, rightF float64
	var isFloat bool
	if li, ok := left.(*object.Integer); ok {
		leftF = float64(li.Value)
		isFloat = true
	} else if lf, ok := left.(*object.Float); ok {
		leftF = lf.Value
		isFloat = true
	}
	if ri, ok := right.(*object.Integer); ok {
		rightF = float64(ri.Value)
	} else if rf, ok := right.(*object.Float); ok {
		rightF = rf.Value
	} else {
		isFloat = false
	}
	if isFloat {
		switch op {
		case code.OpAdd:
			return vm.push(&object.Float{Value: leftF + rightF})
		case code.OpSub:
			return vm.push(&object.Float{Value: leftF - rightF})
		case code.OpMul:
			return vm.push(&object.Float{Value: leftF * rightF})
		case code.OpDiv:
			return vm.push(&object.Float{Value: leftF / rightF})
		case code.OpMod:
			if rightF == 0 {
				return fmt.Errorf("modulo by zero")
			}
			return vm.push(&object.Float{Value: math.Mod(leftF, rightF)})
		case code.OpGreaterThan:
			return vm.push(nativeBoolToObj(leftF > rightF))
		case code.OpEqual:
			return vm.push(nativeBoolToObj(leftF == rightF))
		case code.OpNotEqual:
			return vm.push(nativeBoolToObj(leftF != rightF))
		}
	}

	// String concatenation
	leftStr, ok3 := left.(*object.String)
	rightStr, ok4 := right.(*object.String)
	if ok3 && ok4 && op == code.OpAdd {
		return vm.push(&object.String{Value: leftStr.Value + rightStr.Value})
	}

	// String + other -> auto convert
	if ok3 && op == code.OpAdd {
		return vm.push(&object.String{Value: leftStr.Value + right.Inspect()})
	}
	if ok4 && op == code.OpAdd {
		return vm.push(&object.String{Value: left.Inspect() + rightStr.Value})
	}

	// Boolean equality
	leftBool, ok5 := left.(*object.Boolean)
	rightBool, ok6 := right.(*object.Boolean)
	if ok5 && ok6 {
		switch op {
		case code.OpEqual:
			return vm.push(nativeBoolToObj(leftBool.Value == rightBool.Value))
		case code.OpNotEqual:
			return vm.push(nativeBoolToObj(leftBool.Value != rightBool.Value))
		}
	}

	return fmt.Errorf("unsupported types for binary operation: %s %s", left.Type(), right.Type())
}

func (vm *VM) executeIntegerBinaryOp(op code.Opcode, left, right int64) error {
	switch op {
	case code.OpAdd:
		return vm.push(&object.Integer{Value: left + right})
	case code.OpSub:
		return vm.push(&object.Integer{Value: left - right})
	case code.OpMul:
		return vm.push(&object.Integer{Value: left * right})
	case code.OpDiv:
		return vm.push(&object.Integer{Value: left / right})
	case code.OpMod:
		if right == 0 {
			return fmt.Errorf("modulo by zero")
		}
		return vm.push(&object.Integer{Value: left % right})
	case code.OpGreaterThan:
		return vm.push(nativeBoolToObj(left > right))
	case code.OpEqual:
		return vm.push(nativeBoolToObj(left == right))
	case code.OpNotEqual:
		return vm.push(nativeBoolToObj(left != right))
	case code.OpBitAnd:
		return vm.push(&object.Integer{Value: left & right})
	case code.OpBitOr:
		return vm.push(&object.Integer{Value: left | right})
	case code.OpBitXor:
		return vm.push(&object.Integer{Value: left ^ right})
	case code.OpLshift:
		return vm.push(&object.Integer{Value: left << uint(right&63)})
	case code.OpRshift:
		return vm.push(&object.Integer{Value: left >> uint(right&63)})
	default:
		return fmt.Errorf("unknown integer operator: %d", op)
	}
}

func (vm *VM) buildArray(startIndex, endIndex int) object.Object {
	elements := make([]object.Object, endIndex-startIndex)
	for i := startIndex; i < endIndex; i++ {
		elements[i-startIndex] = vm.stack[i]
	}
	return &object.Array{Elements: elements}
}

func (vm *VM) buildHash(startIndex, endIndex int) (object.Object, error) {
	pairs := make(map[object.HashKey]object.HashPair)
	for i := startIndex; i < endIndex; i += 2 {
		key := vm.stack[i]
		value := vm.stack[i+1]

		hashable, ok := key.(object.Hashable)
		if !ok {
			return nil, fmt.Errorf("unusable as hash key: %s", key.Type())
		}
		pairs[hashable.HashKey()] = object.HashPair{Key: key, Value: value}
	}
	return &object.Hash{Pairs: pairs}, nil
}

func (vm *VM) executeIndexExpression(left, index object.Object) error {
	switch {
	case left.Type() == object.ARRAY_OBJ && index.Type() == object.INTEGER_OBJ:
		return vm.executeArrayIndex(left, index)
	case left.Type() == object.HASH_OBJ:
		return vm.executeHashIndex(left, index)
	default:
		return fmt.Errorf("index operator not supported: %s", left.Type())
	}
}

func (vm *VM) executeArrayIndex(array, index object.Object) error {
	arr := array.(*object.Array)
	i := index.(*object.Integer).Value
	max := int64(len(arr.Elements) - 1)
	if i < 0 || i > max {
		return vm.push(&object.Null{})
	}
	return vm.push(arr.Elements[i])
}

func (vm *VM) executeHashIndex(hash, index object.Object) error {
	h := hash.(*object.Hash)
	key, ok := index.(object.Hashable)
	if !ok {
		return fmt.Errorf("unusable as hash key: %s", index.Type())
	}
	pair, ok := h.Pairs[key.HashKey()]
	if !ok {
		return vm.push(&object.Null{})
	}
	return vm.push(pair.Value)
}

func (vm *VM) executeMemberExpression(obj object.Object, member string) error {
	switch o := obj.(type) {
	case *object.Hash:
		key := &object.String{Value: member}
		pair, ok := o.Pairs[key.HashKey()]
		if !ok {
			return vm.push(&object.Null{})
		}
		return vm.push(pair.Value)

	case *object.Array:
		switch member {
		case "len":
			// Return a builtin-like function
			fn := &object.Builtin{Fn: func(args ...object.Object) object.Object {
				return &object.Integer{Value: int64(len(o.Elements))}
			}}
			return vm.push(fn)
		case "push":
			fn := &object.Builtin{Fn: func(args ...object.Object) object.Object {
				if len(args) != 1 {
					return &object.Error{Message: "wrong number of arguments"}
				}
				newElements := make([]object.Object, len(o.Elements)+1)
				copy(newElements, o.Elements)
				newElements[len(o.Elements)] = args[0]
				o.Elements = newElements
				return &object.Null{}
			}}
			return vm.push(fn)
		}
		return vm.push(&object.Null{})

	default:
		return fmt.Errorf("member access not supported on %s", obj.Type())
	}
}

func nativeBoolToObj(input bool) *object.Boolean {
	if input {
		return &object.Boolean{Value: true}
	}
	return &object.Boolean{Value: false}
}

func (vm *VM) push(obj object.Object) error {
	if vm.sp >= StackSize {
		return fmt.Errorf("stack overflow")
	}
	vm.stack[vm.sp] = obj
	vm.sp++
	return nil
}

func (vm *VM) pushClosure(constIndex int, numFree int) error {
	constant := vm.getConstants()[constIndex]
	compiledFn, ok := constant.(*object.CompiledFunction)
	if !ok {
		return fmt.Errorf("not a compiled function: %T", constant)
	}

	free := make([]object.Object, numFree)
	for i := 0; i < numFree; i++ {
		free[i] = vm.stack[vm.sp-numFree+i]
	}
	vm.sp -= numFree

	closure := &object.Closure{Fn: compiledFn, Free: free}
	return vm.push(closure)
}

// attachConstants recursively attaches module constants to closures
// so they can reference the correct constant pool when called from another VM.
func (vm *VM) attachConstants(val object.Object, constants []object.Object) {
	switch v := val.(type) {
	case *object.Closure:
		if v.Fn.Constants == nil {
			v.Fn.Constants = constants
		}
		// Also attach to free variables that might be closures
		for _, free := range v.Free {
			vm.attachConstants(free, constants)
		}
	case *object.Hash:
		for _, pair := range v.Pairs {
			vm.attachConstants(pair.Value, constants)
		}
	case *object.Array:
		for _, elem := range v.Elements {
			vm.attachConstants(elem, constants)
		}
	}
}

func (vm *VM) pop() object.Object {
	obj := vm.stack[vm.sp-1]
	vm.sp--
	return obj
}

func (vm *VM) LastPoppedStackElem() object.Object {
	return vm.stack[vm.sp]
}

func (vm *VM) SetFrame(i int, f *Frame) {
	vm.frames[i] = f
}

func (vm *VM) SetFrameIndex(i int) {
	vm.frameIndex = i
}

func (vm *VM) SetStack(i int, obj object.Object) {
	vm.stack[i] = obj
}

func (vm *VM) SetStackPointer(sp int) {
	vm.sp = sp
}

func isTruthy(obj object.Object) bool {
	switch obj := obj.(type) {
	case *object.Boolean:
		return obj.Value
	case *object.Null:
		return false
	default:
		return true
	}
}
