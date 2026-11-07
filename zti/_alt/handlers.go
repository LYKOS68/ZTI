package main

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	OP_READ   byte = 0x01 // ERFASSE (Z)
	OP_XFORM  byte = 0x02 // FORME   (T)
	OP_CHECK  byte = 0x03 // PRÜFE   (I)
	OP_COMMIT byte = 0x04 // BIND    (S)
	OP_HALT   byte = 0xFF // STOP
)

const (
	STR_U16         = "U16"
	STR_INKR        = "INKR"
	STR_RING_GLEICH = "RING_GLEICH"
	STR_GLEICH      = "GLEICH"
)

type Instruction struct {
	Op     byte
	Src    string
	Dst    string
	OpId   string
	RawVal string
	Val    uint16
}

type Handler func(*Interpreter, Instruction) error

// O(1) Dispatcher-Tabelle (Rick-Style: handler_table analog zu Win32 WndProc / C-Kernels)
var handlerTable = [256]Handler{
	OP_READ:   onRead,
	OP_XFORM:  onXform,
	OP_CHECK:  onCheck,
	OP_COMMIT: onCommit,
	OP_HALT:   onHalt,
}

func onRead(in *Interpreter, inst Instruction) error {
	if inst.OpId != STR_U16 {
		in.Rollback()
		return fmt.Errorf("%w: '%s'", ErrInvalidType, inst.OpId)
	}

	var numVal uint16
	if inst.RawVal != "" {
		parsed, err := parseU16(inst.RawVal)
		if err != nil {
			in.Rollback()
			return fmt.Errorf("%w: ungueltiges Literal '%s'", ErrInvalidType, inst.RawVal)
		}
		numVal = parsed
	}

	in.StagingState[inst.Dst] = Value{
		Type:   inst.OpId,
		U16Val: numVal,
	}

	// Bit Z setzen
	in.Gate |= GateZ
	return nil
}

func onXform(in *Interpreter, inst Instruction) error {
	// Quelle ermitteln (Staging vor Live)
	val, ok := in.StagingState[inst.Src]
	if !ok {
		val, ok = in.LiveState[inst.Src]
		if !ok {
			in.Rollback()
			return fmt.Errorf("%w: Register '%s' nicht initialisiert", ErrUnboundState, inst.Src)
		}
		in.Gate |= GateZ
	}

	if (in.Gate & GateZ) == 0 {
		in.Rollback()
		return fmt.Errorf("%w: Vorbedingung Z nicht aktiv", ErrUnboundState)
	}

	var res uint16
	switch inst.OpId {
	case STR_INKR:
		// Strikte modulare U16 Ring-Arithmetik: (x + 1) mod 65536
		res = uint16((uint32(val.U16Val) + 1) % 65536)
	default:
		in.Rollback()
		return fmt.Errorf("%w: '%s'", ErrUnknownOperator, inst.OpId)
	}

	in.StagingState[inst.Dst] = Value{
		Type:   val.Type,
		U16Val: res,
	}

	// Bit T setzen
	in.Gate |= GateT
	return nil
}

func onCheck(in *Interpreter, inst Instruction) error {
	val, ok := in.StagingState[inst.Src]
	if !ok {
		val, ok = in.LiveState[inst.Src]
		if !ok {
			in.Rollback()
			return fmt.Errorf("%w: Register '%s' vor PRÜFE nicht vorhanden", ErrUnboundState, inst.Src)
		}
	}

	expected, err := parseU16(inst.RawVal)
	if err != nil {
		in.Rollback()
		return fmt.Errorf("%w: ungueltiger Pruefwert '%s'", ErrInvalidType, inst.RawVal)
	}

	match := false
	switch inst.OpId {
	case STR_RING_GLEICH, STR_GLEICH:
		match = (val.U16Val == expected)
	default:
		in.Rollback()
		return fmt.Errorf("%w: unbekanntes Praedikat '%s'", ErrUnknownPredicate, inst.OpId)
	}

	if match {
		in.Gate |= GateI
	} else {
		in.Gate &^= GateI
	}

	return nil
}

func onCommit(in *Interpreter, inst Instruction) error {
	// Gate-Schranke: Z, T und I muessen fuer atomaren Commit erfuellt sein
	if (in.Gate & GateReadyForCommit) != GateReadyForCommit {
		in.Rollback()
		return fmt.Errorf("%w: Gate-Bits sind %04b, benoetigt %04b", ErrGateClosed, in.Gate, GateReadyForCommit)
	}

	staged, ok := in.StagingState[inst.Dst]
	if !ok {
		in.Rollback()
		return fmt.Errorf("%w: Register '%s' nicht im Staging vorhanden", ErrUnboundState, inst.Dst)
	}

	// Atomarer Commit ins Live-Register
	in.LiveState[inst.Dst] = staged
	delete(in.StagingState, inst.Dst)

	// Gate nach erfolgreicher Bindung fuer naechsten Zyklus resetten
	in.Gate = 0
	return nil
}

func onHalt(in *Interpreter, inst Instruction) error {
	in.Halted = true
	return nil
}

func parseU16(lit string) (uint16, error) {
	lit = strings.TrimSpace(lit)
	if strings.HasPrefix(lit, "0x") || strings.HasPrefix(lit, "0X") {
		v, err := strconv.ParseUint(lit[2:], 16, 16)
		return uint16(v), err
	}
	v, err := strconv.ParseUint(lit, 10, 16)
	return uint16(v), err
}
