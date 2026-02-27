package vm

import (
	"artemis/code"
	"artemis/compiler"
	"artemis/object"
	"encoding/binary"
	"fmt"
)

const (
	StackSize   = 2048
	GlobalsSize = 65536
	MaxFrames   = 1024
)

type Frame struct {
	fn          *object.CompiledFunction
	ip          int
	basePointer int
}

func NewFrame(fn *object.CompiledFunction, basePointer int) *Frame {
	return &Frame{
		fn:          fn,
		ip:          -1,
		basePointer: basePointer,
	}
}

func (f *Frame) Instructions() code.Instructions {
	return f.fn.Instructions
}

type VM struct {
	constants []object.Object

	stack   []object.Object
	sp      int
	globals []object.Object

	frames     []*Frame
	frameIndex int
}

func New(bytecode *compiler.Bytecode) *VM {
	mainFn := &object.CompiledFunction{Instructions: bytecode.Instructions}
	mainFrame := NewFrame(mainFn, 0)

	frames := make([]*Frame, MaxFrames)
	frames[0] = mainFrame

	return &VM{
		constants: bytecode.Constants,
		stack:     make([]object.Object, StackSize),
		sp:        0,
		globals:   make([]object.Object, GlobalsSize),

		frames:     frames,
		frameIndex: 1,
	}
}

func (vm *VM) currentFrame() *Frame {
	return vm.frames[vm.frameIndex-1]
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

	for vm.currentFrame().ip < len(vm.currentFrame().Instructions())-1 {
		vm.currentFrame().ip++

		ip = vm.currentFrame().ip
		ins = vm.currentFrame().Instructions()
		op = code.Opcode(ins[ip])

		switch op {
		case code.OpConstant:
			constIndex := binary.BigEndian.Uint16(ins[ip+1:])
			vm.currentFrame().ip += 2

			err := vm.push(vm.constants[constIndex])
			if err != nil {
				return err
			}

		case code.OpString:
			constIndex := binary.BigEndian.Uint16(ins[ip+1:])
			vm.currentFrame().ip += 2

			err := vm.push(vm.constants[constIndex])
			if err != nil {
				return err
			}

		case code.OpAdd, code.OpSub, code.OpMul, code.OpDiv,
			code.OpGreaterThan, code.OpEqual, code.OpNotEqual:
			err := vm.executeBinaryOperation(op)
			if err != nil {
				return err
			}

		case code.OpTrue:
			err := vm.push(&object.Boolean{Value: true})
			if err != nil {
				return err
			}

		case code.OpFalse:
			err := vm.push(&object.Boolean{Value: false})
			if err != nil {
				return err
			}

		case code.OpOut:
			val := vm.pop()
			fmt.Println(val.Inspect())

		case code.OpGetGlobal:
			globalIndex := binary.BigEndian.Uint16(ins[ip+1:])
			vm.currentFrame().ip += 2

			err := vm.push(vm.globals[globalIndex])
			if err != nil {
				return err
			}

		case code.OpSetGlobal:
			globalIndex := binary.BigEndian.Uint16(ins[ip+1:])
			vm.currentFrame().ip += 2

			vm.globals[globalIndex] = vm.pop()

		case code.OpGetLocal:
			localIndex := int(ins[ip+1])
			vm.currentFrame().ip += 1

			frame := vm.currentFrame()
			err := vm.push(vm.stack[frame.basePointer+localIndex])
			if err != nil {
				return err
			}

		case code.OpSetLocal:
			localIndex := int(ins[ip+1])
			vm.currentFrame().ip += 1

			frame := vm.currentFrame()
			vm.stack[frame.basePointer+localIndex] = vm.pop()

		case code.OpJump:
			pos := int(binary.BigEndian.Uint16(ins[ip+1:]))
			vm.currentFrame().ip = pos - 1

		case code.OpJumpNotTruthy:
			pos := int(binary.BigEndian.Uint16(ins[ip+1:]))
			vm.currentFrame().ip += 2

			condition := vm.pop()
			if !isTruthy(condition) {
				vm.currentFrame().ip = pos - 1
			}

		case code.OpCall:
			numArgs := int(ins[ip+1])
			vm.currentFrame().ip += 1

			callee := vm.stack[vm.sp-1-numArgs]
			fn, ok := callee.(*object.CompiledFunction)
			if !ok {
				return fmt.Errorf("calling non-function")
			}

			if numArgs != fn.NumParameters {
				return fmt.Errorf("wrong number of arguments: want=%d, got=%d",
					fn.NumParameters, numArgs)
			}

			frame := NewFrame(fn, vm.sp-numArgs)
			vm.pushFrame(frame)
			vm.sp = frame.basePointer + fn.NumLocals

		case code.OpReturnValue:
			returnValue := vm.pop()

			frame := vm.popFrame()
			vm.sp = frame.basePointer - 1

			err := vm.push(returnValue)
			if err != nil {
				return err
			}

		case code.OpReturn:
			frame := vm.popFrame()
			vm.sp = frame.basePointer - 1

			err := vm.push(&object.Null{})
			if err != nil {
				return err
			}

		case code.OpPop:
			vm.pop()
		}
	}

	return nil
}

func (vm *VM) executeBinaryOperation(op code.Opcode) error {
	right := vm.pop()
	left := vm.pop()

	leftInt, ok1 := left.(*object.Integer)
	rightInt, ok2 := right.(*object.Integer)

	if ok1 && ok2 {
		switch op {
		case code.OpAdd:
			return vm.push(&object.Integer{Value: leftInt.Value + rightInt.Value})
		case code.OpSub:
			return vm.push(&object.Integer{Value: leftInt.Value - rightInt.Value})
		case code.OpMul:
			return vm.push(&object.Integer{Value: leftInt.Value * rightInt.Value})
		case code.OpDiv:
			return vm.push(&object.Integer{Value: leftInt.Value / rightInt.Value})
		case code.OpGreaterThan:
			return vm.push(nativeBoolToObj(leftInt.Value > rightInt.Value))
		case code.OpEqual:
			return vm.push(nativeBoolToObj(leftInt.Value == rightInt.Value))
		case code.OpNotEqual:
			return vm.push(nativeBoolToObj(leftInt.Value != rightInt.Value))
		}
	}

	// String concatenation
	leftStr, ok3 := left.(*object.String)
	rightStr, ok4 := right.(*object.String)
	if ok3 && ok4 && op == code.OpAdd {
		return vm.push(&object.String{Value: leftStr.Value + rightStr.Value})
	}

	return fmt.Errorf("unsupported types for binary operation: %s %s", left.Type(), right.Type())
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

func (vm *VM) pop() object.Object {
	obj := vm.stack[vm.sp-1]
	vm.sp--
	return obj
}

func (vm *VM) LastPoppedStackElem() object.Object {
	return vm.stack[vm.sp]
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
