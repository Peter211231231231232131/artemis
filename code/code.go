package code

import (
	"encoding/binary"
	"fmt"
)

type Instructions []byte

type Opcode byte

const (
	OpConstant Opcode = iota
	OpAdd
	OpPop
	OpSub
	OpMul
	OpDiv
	OpTrue
	OpFalse
	OpString
	OpOut
	OpSetGlobal
	OpGetGlobal
	OpJump
	OpJumpNotTruthy
	OpGreaterThan
	OpEqual
	OpNotEqual
	OpCall
	OpReturnValue
	OpReturn
	OpGetLocal
	OpSetLocal
)

type Definition struct {
	Name          string
	OperandWidths []int
}

var definitions = map[Opcode]*Definition{
	OpConstant:      {"OpConstant", []int{2}}, // 2 bytes = 65535 possible constants
	OpAdd:           {"OpAdd", []int{}},
	OpPop:           {"OpPop", []int{}},
	OpSub:           {"OpSub", []int{}},
	OpMul:           {"OpMul", []int{}},
	OpDiv:           {"OpDiv", []int{}},
	OpTrue:          {"OpTrue", []int{}},
	OpFalse:         {"OpFalse", []int{}},
	OpString:        {"OpString", []int{2}},
	OpOut:           {"OpOut", []int{}},
	OpSetGlobal:     {"OpSetGlobal", []int{2}},
	OpGetGlobal:     {"OpGetGlobal", []int{2}},
	OpJump:          {"OpJump", []int{2}},
	OpJumpNotTruthy: {"OpJumpNotTruthy", []int{2}},
	OpGreaterThan:   {"OpGreaterThan", []int{}},
	OpEqual:         {"OpEqual", []int{}},
	OpNotEqual:      {"OpNotEqual", []int{}},
	OpCall:          {"OpCall", []int{1}}, // 1 byte for number of arguments
	OpReturnValue:   {"OpReturnValue", []int{}},
	OpReturn:        {"OpReturn", []int{}},
	OpGetLocal:      {"OpGetLocal", []int{1}}, // locals usually small, 1 byte is plenty
	OpSetLocal:      {"OpSetLocal", []int{1}},
}

func Lookup(op byte) (*Definition, error) {
	def, ok := definitions[Opcode(op)]
	if !ok {
		return nil, fmt.Errorf("opcode %d undefined", op)
	}
	return def, nil
}

func Make(op Opcode, operands ...int) []byte {
	def, ok := definitions[op]
	if !ok {
		return []byte{}
	}

	instructionLen := 1
	for _, w := range def.OperandWidths {
		instructionLen += w
	}

	instruction := make([]byte, instructionLen)
	instruction[0] = byte(op)

	offset := 1
	for i, o := range operands {
		width := def.OperandWidths[i]
		switch width {
		case 2:
			binary.BigEndian.PutUint16(instruction[offset:], uint16(o))
		case 1:
			instruction[offset] = byte(o)
		}
		offset += width
	}

	return instruction
}
