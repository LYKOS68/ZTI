package main

import (
	"errors"
	"fmt"
)

var (
	ErrBudgetExceeded = errors.New("zti vm: instruktionsbudget ueberschritten (endlosschleifenschutz)")
	ErrDivideByZero   = errors.New("zti vm: division durch null")
	ErrInvalidOpcode  = errors.New("zti vm: ungueltiger opcode")
)

// Opcode-Definitionen (Bytecode ISA)
const (
	OP_BYTE_READ   byte = 0x01 // Z: ERFASSE
	OP_BYTE_XFORM  byte = 0x02 // T: FORME
	OP_BYTE_CHECK  byte = 0x03 // I: PRÜFE
	OP_BYTE_COMMIT byte = 0x04 // S: BIND
	OP_BYTE_JUMP   byte = 0x05 // Kontrollfluss: unbedingter Sprung
	OP_BYTE_JNZ    byte = 0x06 // Kontrollfluss: Sprung wenn wahr
	OP_BYTE_JZ     byte = 0x07 // Kontrollfluss: Sprung wenn falsch (WENN/SOLANGE)
	OP_BYTE_SEND   byte = 0x08 // Routing: Nachricht an Graph emittieren
	OP_BYTE_HALT   byte = 0xFF // STOP
)

// BytecodeInstruction ist die flache, maschinenlesbare Zwischenform.
type BytecodeInstruction struct {
	Op       byte
	Src      string
	Dst      string
	OpId     string
	ImmVal   uint16
	JumpAddr int
}

// SentMessage entspricht Ricks C/Win32 Message-Layout {type, val1, val2}
type SentMessage struct {
	Target string
	Type   uint8
	Val1   uint16
	Val2   uint16
}

// VM ist die Register- und Gatter-Maschine mit 3+1 Transaktionsbarriere.
type VM struct {
	Bytecode     []BytecodeInstruction
	PC           int
	LiveState    map[string]Value
	StagingState map[string]Value
	Gate         BitGate
	Halted       bool
	Steps        int
	MaxSteps     int
	OutMessages  []SentMessage
}

func NewVM(bytecode []BytecodeInstruction) *VM {
	return &VM{
		Bytecode:     bytecode,
		PC:           0,
		LiveState:    make(map[string]Value),
		StagingState: make(map[string]Value),
		Gate:         0,
		Halted:       false,
		Steps:        0,
		MaxSteps:     10000, // Schutz gegen Hänger
		OutMessages:  []SentMessage{},
	}
}

// Run führt die Bytecode-Schleife mit Instruktionsbudget und Fehler-Rollback aus.
func (vm *VM) Run() error {
	for vm.PC < len(vm.Bytecode) && !vm.Halted {
		if vm.Steps >= vm.MaxSteps {
			vm.Rollback()
			return fmt.Errorf("%w (%d Schritte)", ErrBudgetExceeded, vm.MaxSteps)
		}

		inst := vm.Bytecode[vm.PC]
		vm.Steps++

		if err := vm.Execute(inst); err != nil {
			vm.Rollback()
			return err
		}

		vm.PC++
	}
	return nil
}

// Execute dispatcht die jeweilige Instruktion deterministisch.
func (vm *VM) Execute(inst BytecodeInstruction) error {
	switch inst.Op {
	case OP_BYTE_READ:
		return vm.execRead(inst)
	case OP_BYTE_XFORM:
		return vm.execXform(inst)
	case OP_BYTE_CHECK:
		return vm.execCheck(inst)
	case OP_BYTE_COMMIT:
		return vm.execCommit(inst)
	case OP_BYTE_JUMP:
		// Unbedingter Sprung (PC wird vor Schleifenende dekrementiert wg. PC++)
		vm.PC = inst.JumpAddr - 1
		return nil
	case OP_BYTE_JZ:
		// Sprung wenn Bedingungsregister falsch / 0 ist
		condVal, ok := vm.resolveVal(inst.Src)
		if !ok || condVal.U16Val == 0 {
			vm.PC = inst.JumpAddr - 1
		}
		return nil
	case OP_BYTE_JNZ:
		// Sprung wenn Bedingungsregister wahr / != 0 ist
		condVal, ok := vm.resolveVal(inst.Src)
		if ok && condVal.U16Val != 0 {
			vm.PC = inst.JumpAddr - 1
		}
		return nil
	case OP_BYTE_SEND:
		return vm.execSend(inst)
	case OP_BYTE_HALT:
		vm.Halted = true
		return nil
	default:
		return fmt.Errorf("%w: 0x%02X an Adresse %d", ErrInvalidOpcode, inst.Op, vm.PC)
	}
}

func (vm *VM) execRead(inst BytecodeInstruction) error {
	vm.StagingState[inst.Dst] = Value{
		Type:   inst.OpId,
		U16Val: inst.ImmVal,
	}
	vm.Gate |= GateZ
	return nil
}

func (vm *VM) execXform(inst BytecodeInstruction) error {
	val, ok := vm.resolveVal(inst.Src)
	if !ok {
		return fmt.Errorf("%w: Register '%s' nicht initialisiert", ErrUnboundState, inst.Src)
	}

	// Sobald ein valides Substrat (Live oder Staging) existiert, ist Z aktiv
	vm.Gate |= GateZ

	var res uint16
	switch inst.OpId {
	case STR_INKR, "+":
		// Modulare U16 Ring-Arithmetik: (x + imm) mod 65536
		inc := inst.ImmVal
		if inc == 0 {
			inc = 1
		}
		res = uint16((uint32(val.U16Val) + uint32(inc)) % 65536)
	case "-":
		res = uint16((uint32(val.U16Val) - uint32(inst.ImmVal) + 65536) % 65536)
	case "*":
		res = uint16((uint32(val.U16Val) * uint32(inst.ImmVal)) % 65536)
	case "/":
		if inst.ImmVal == 0 {
			return ErrDivideByZero
		}
		res = val.U16Val / inst.ImmVal
	default:
		return fmt.Errorf("%w: '%s'", ErrUnknownOperator, inst.OpId)
	}

	vm.StagingState[inst.Dst] = Value{
		Type:   val.Type,
		U16Val: res,
	}
	vm.Gate |= GateT
	return nil
}

func (vm *VM) execCheck(inst BytecodeInstruction) error {
	val, ok := vm.resolveVal(inst.Src)
	if !ok {
		return fmt.Errorf("%w: Register '%s' vor PRÜFE nicht vorhanden", ErrUnboundState, inst.Src)
	}

	match := false
	switch inst.OpId {
	case STR_RING_GLEICH, STR_GLEICH, "==":
		match = (val.U16Val == inst.ImmVal)
	case "<=":
		match = (val.U16Val <= inst.ImmVal)
	case "<":
		match = (val.U16Val < inst.ImmVal)
	case ">=":
		match = (val.U16Val >= inst.ImmVal)
	case ">":
		match = (val.U16Val > inst.ImmVal)
	case "!=":
		match = (val.U16Val != inst.ImmVal)
	default:
		return fmt.Errorf("%w: '%s'", ErrUnknownPredicate, inst.OpId)
	}

	if match {
		vm.Gate |= GateI
	} else {
		vm.Gate &^= GateI
	}
	return nil
}

func (vm *VM) execCommit(inst BytecodeInstruction) error {
	// 3+1 Gate-Barriere: Z, T und I müssen aktiv sein
	if (vm.Gate & GateReadyForCommit) != GateReadyForCommit {
		return fmt.Errorf("%w: Gate-Bits sind %04b, benoetigt %04b", ErrGateClosed, vm.Gate, GateReadyForCommit)
	}

	staged, ok := vm.StagingState[inst.Dst]
	if !ok {
		return fmt.Errorf("%w: Register '%s' nicht im Staging vorhanden", ErrUnboundState, inst.Dst)
	}

	// Atomarer Commit (analog zu os.Rename / Pointer-Swap)
	vm.LiveState[inst.Dst] = staged
	delete(vm.StagingState, inst.Dst)

	// Gate sauber auf 0000 zurücksetzen
	vm.Gate = 0
	return nil
}

func (vm *VM) execSend(inst BytecodeInstruction) error {
	msg := SentMessage{
		Target: inst.Dst,
		Type:   uint8(inst.ImmVal >> 8),
		Val1:   inst.ImmVal & 0xFF,
		Val2:   0,
	}
	vm.OutMessages = append(vm.OutMessages, msg)
	return nil
}

// Rollback leert den Staging-Puffer und nullt das Gate (analog zu .bak-Reset in pipeline-go).
func (vm *VM) Rollback() {
	vm.StagingState = make(map[string]Value)
	vm.Gate = 0
}

func (vm *VM) resolveVal(ident string) (Value, bool) {
	if val, ok := vm.StagingState[ident]; ok {
		return val, true
	}
	val, ok := vm.LiveState[ident]
	return val, ok
}
