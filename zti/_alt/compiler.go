package main

import (
	"fmt"
	"strconv"
)

type Compiler struct{}

func NewCompiler() *Compiler {
	return &Compiler{}
}

func (c *Compiler) Compile(prog *Program) ([]BytecodeInstruction, error) {
	var bc []BytecodeInstruction

	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ReadStmt:
			var imm uint16
			if s.From != "" {
				parsed, err := parseU16Literal(s.From)
				if err != nil {
					return nil, err
				}
				imm = parsed
			}
			bc = append(bc, BytecodeInstruction{
				Op:     OP_BYTE_READ,
				Dst:    s.Ident,
				OpId:   s.Type,
				ImmVal: imm,
			})
		case *TransformStmt:
			bc = append(bc, BytecodeInstruction{
				Op:   OP_BYTE_XFORM,
				Src:  s.Src,
				Dst:  s.Dst,
				OpId: s.Op,
			})
		case *CheckStmt:
			var imm uint16
			if s.Value != "" {
				parsed, err := parseU16Literal(s.Value)
				if err != nil {
					return nil, err
				}
				imm = parsed
			}
			// Normalize predicate names
			pred := s.Predicate
			if pred == STR_GLEICH {
				pred = "=="
			} else if pred == STR_RING_GLEICH {
				pred = "=="
			}
			bc = append(bc, BytecodeInstruction{
				Op:     OP_BYTE_CHECK,
				Src:    s.Ident,
				OpId:   pred,
				ImmVal: imm,
			})
		case *CommitStmt:
			bc = append(bc, BytecodeInstruction{
				Op:  OP_BYTE_COMMIT,
				Dst: s.Ident,
			})
		case *HaltStmt:
			bc = append(bc, BytecodeInstruction{
				Op:     OP_BYTE_HALT,
				ImmVal: uint16(s.Code),
			})
		default:
			return nil, fmt.Errorf("%w: unbekannter Statement-Typ", ErrSyntax)
		}
	}

	return bc, nil
}

func parseU16Literal(lit string) (uint16, error) {
	if len(lit) >= 2 && (lit[0:2] == "0x" || lit[0:2] == "0X") {
		v, err := strconv.ParseUint(lit[2:], 16, 16)
		return uint16(v), err
	}
	v, err := strconv.ParseUint(lit, 10, 16)
	return uint16(v), err
}
