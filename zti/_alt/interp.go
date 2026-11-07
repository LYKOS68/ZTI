package main

import "fmt"

type BitGate uint8

const (
	GateZ BitGate = 1 << 0 // 0001 (Zustand erfasst)
	GateT BitGate = 1 << 1 // 0010 (Transformation vollzogen)
	GateI BitGate = 1 << 2 // 0100 (Invariante geprüft & wahr)
	GateS BitGate = 1 << 3 // 1000 (Commit freigegeben)

	GateReadyForCommit = GateZ | GateT | GateI     // 0111
	GateFullCommit     = GateZ | GateT | GateI | GateS // 1111
)

type Value struct {
	Type   string
	U16Val uint16
}

type Interpreter struct {
	LiveState    map[string]Value
	StagingState map[string]Value
	Gate         BitGate
	Halted       bool
}

func NewInterpreter() *Interpreter {
	return &Interpreter{
		LiveState:    make(map[string]Value),
		StagingState: make(map[string]Value),
		Gate:         0,
		Halted:       false,
	}
}

func (in *Interpreter) Dispatch(inst Instruction) error {
	handler := handlerTable[inst.Op]
	if handler == nil {
		in.Rollback()
		return fmt.Errorf("zti: kein Handler fuer Opcode 0x%02X registriert", inst.Op)
	}
	return handler(in, inst)
}

func (in *Interpreter) EvalStatement(stmt Statement) error {
	var inst Instruction

	switch s := stmt.(type) {
	case *ReadStmt:
		inst = Instruction{
			Op:     OP_READ,
			Dst:    s.Ident,
			OpId:   s.Type,
			RawVal: s.From,
		}
	case *TransformStmt:
		inst = Instruction{
			Op:   OP_XFORM,
			Src:  s.Src,
			Dst:  s.Dst,
			OpId: s.Op,
		}
	case *CheckStmt:
		inst = Instruction{
			Op:     OP_CHECK,
			Src:    s.Ident,
			OpId:   s.Predicate,
			RawVal: s.Value,
		}
	case *CommitStmt:
		inst = Instruction{
			Op:  OP_COMMIT,
			Dst: s.Ident,
		}
	case *HaltStmt:
		inst = Instruction{
			Op:  OP_HALT,
			Val: uint16(s.Code),
		}
	default:
		return fmt.Errorf("%w: unbekannter Statement-Typ", ErrSyntax)
	}

	return in.Dispatch(inst)
}

func (in *Interpreter) Run(prog *Program) error {
	for _, stmt := range prog.Statements {
		if in.Halted {
			break
		}
		if err := in.EvalStatement(stmt); err != nil {
			return err
		}
	}
	return nil
}

func (in *Interpreter) Rollback() {
	in.StagingState = make(map[string]Value)
	in.Gate = 0
}

func (in *Interpreter) GetLive(ident string) (Value, bool) {
	val, ok := in.LiveState[ident]
	return val, ok
}

func (in *Interpreter) GetStaging(ident string) (Value, bool) {
	val, ok := in.StagingState[ident]
	return val, ok
}
