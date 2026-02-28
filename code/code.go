package code

import (
	"bytes"
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
	OpGetBuiltin
	OpArray
	OpHash
	OpIndex
	OpMember
	OpNull
	OpMinus
	OpBang
	OpSpawn
	OpClosure
	OpGetFree
	OpSetFree
	OpImport
	OpBitAnd
	OpBitOr
	OpBitXor
	OpBitNot
	OpLshift
	OpRshift
	OpMod
	OpJumpTruthy
	OpDup
	OpCatch
	OpThrow
	OpEndCatch
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
	OpGetBuiltin:    {"OpGetBuiltin", []int{1}},
	OpArray:         {"OpArray", []int{2}},
	OpHash:          {"OpHash", []int{2}},
	OpIndex:         {"OpIndex", []int{}},
	OpMember:        {"OpMember", []int{2}},
	OpNull:          {"OpNull", []int{}},
	OpMinus:         {"OpMinus", []int{}},
	OpBang:          {"OpBang", []int{}},
	OpSpawn:         {"OpSpawn", []int{1}}, // 1 byte for number of arguments
	OpClosure:       {"OpClosure", []int{2, 1}},
	OpGetFree:       {"OpGetFree", []int{1}},
	OpSetFree:       {"OpSetFree", []int{1}},
	OpImport:        {"OpImport", []int{}},
	OpBitAnd:        {"OpBitAnd", []int{}},
	OpBitOr:         {"OpBitOr", []int{}},
	OpBitXor:        {"OpBitXor", []int{}},
	OpBitNot:        {"OpBitNot", []int{}},
	OpLshift:        {"OpLshift", []int{}},
	OpRshift:        {"OpRshift", []int{}},
	OpMod:           {"OpMod", []int{}},
	OpJumpTruthy:    {"OpJumpTruthy", []int{2}},
	OpDup:           {"OpDup", []int{}},
	OpCatch:         {"OpCatch", []int{2}},
	OpThrow:         {"OpThrow", []int{}},
	OpEndCatch:      {"OpEndCatch", []int{}},
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

func (ins Instructions) String() string {
	var out bytes.Buffer

	i := 0
	for i < len(ins) {
		def, err := Lookup(ins[i])
		if err != nil {
			fmt.Fprintf(&out, "ERROR: %s\n", err)
			continue
		}

		operands, read := ReadOperands(def, ins[i+1:])

		fmt.Fprintf(&out, "%04d %s\n", i, ins.fmtInstruction(def, operands))

		i += 1 + read
	}

	return out.String()
}

func (ins Instructions) fmtInstruction(def *Definition, operands []int) string {
	operandCount := len(def.OperandWidths)

	if len(operands) != operandCount {
		return fmt.Sprintf("ERROR: operand len %d does not match defined %d\n",
			len(operands), operandCount)
	}

	switch operandCount {
	case 0:
		return def.Name
	case 1:
		return fmt.Sprintf("%s %d", def.Name, operands[0])
	case 2:
		return fmt.Sprintf("%s %d %d", def.Name, operands[0], operands[1])
	}

	return fmt.Sprintf("ERROR: unhandled operandCount for %s\n", def.Name)
}

func ReadOperands(def *Definition, ins Instructions) ([]int, int) {
	operands := make([]int, len(def.OperandWidths))
	offset := 0

	for i, width := range def.OperandWidths {
		switch width {
		case 2:
			operands[i] = int(binary.BigEndian.Uint16(ins[offset:]))
		case 1:
			operands[i] = int(ins[offset])
		}

		offset += width
	}

	return operands, offset
}
