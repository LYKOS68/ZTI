package main

import (
	"errors"
	"testing"
)

// TEST-VM-01: Valider 3+1 Transaktionszyklus
func TestVM_BasicCommit(t *testing.T) {
	bytecode := []BytecodeInstruction{
		{Op: OP_BYTE_READ, Dst: "counter", OpId: "U16", ImmVal: 100},
		{Op: OP_BYTE_XFORM, Src: "counter", Dst: "counter", OpId: STR_INKR},
		{Op: OP_BYTE_CHECK, Src: "counter", OpId: "==", ImmVal: 101},
		{Op: OP_BYTE_COMMIT, Dst: "counter"},
		{Op: OP_BYTE_HALT},
	}

	vm := NewVM(bytecode)
	if err := vm.Run(); err != nil {
		t.Fatalf("VM Run fehlgeschlagen: %v", err)
	}

	val, ok := vm.LiveState["counter"]
	if !ok || val.U16Val != 101 {
		t.Fatalf("Erwarteter Wert 101, erhalten: %+v", val)
	}
	if vm.Gate != 0 {
		t.Fatalf("Gate nicht auf 0000 zurückgesetzt, ist: %04b", vm.Gate)
	}
}

// TEST-VM-02: U16 Ring-Überlauf
func TestVM_U16_Overflow(t *testing.T) {
	bytecode := []BytecodeInstruction{
		{Op: OP_BYTE_READ, Dst: "reg", OpId: "U16", ImmVal: 65535},
		{Op: OP_BYTE_XFORM, Src: "reg", Dst: "reg", OpId: STR_INKR},
		{Op: OP_BYTE_CHECK, Src: "reg", OpId: "==", ImmVal: 0},
		{Op: OP_BYTE_COMMIT, Dst: "reg"},
		{Op: OP_BYTE_HALT},
	}

	vm := NewVM(bytecode)
	if err := vm.Run(); err != nil {
		t.Fatalf("U16 Overflow Run fehlgeschlagen: %v", err)
	}

	val, ok := vm.LiveState["reg"]
	if !ok || val.U16Val != 0 {
		t.Fatalf("Erwarteter Wrap-Around Wert 0, erhalten: %+v", val)
	}
}

// TEST-VM-03: Commit ohne Prüfung (Gate-Bruch & Rollback)
func TestVM_Commit_Rejected(t *testing.T) {
	bytecode := []BytecodeInstruction{
		{Op: OP_BYTE_READ, Dst: "dirty", OpId: "U16", ImmVal: 50},
		{Op: OP_BYTE_XFORM, Src: "dirty", Dst: "dirty", OpId: STR_INKR},
		// PRÜFE fehlt!
		{Op: OP_BYTE_COMMIT, Dst: "dirty"},
		{Op: OP_BYTE_HALT},
	}

	vm := NewVM(bytecode)
	err := vm.Run()
	if err == nil {
		t.Fatalf("VM hätte unberechtigten Commit abweisen müssen")
	}
	if !errors.Is(err, ErrGateClosed) {
		t.Errorf("Erwarteter ErrGateClosed, erhalten: %v", err)
	}

	// LiveState muss leer sein, Staging geleert durch Rollback
	if len(vm.LiveState) != 0 || len(vm.StagingState) != 0 {
		t.Errorf("Rollback unvollständig: Live=%+v, Staging=%+v", vm.LiveState, vm.StagingState)
	}
}

// TEST-VM-04: Echte Schleife über Bytecode-Jumps (SOLANGE i < 5)
func TestVM_Loop_Execution(t *testing.T) {
	// Algorithmus:
	// 0: READ i = 0
	// 1: READ limit = 5
	// 2: CHECK i < limit (Ergebnis in Gate I)
	// 3: XFORM i = i + 1
	// 4: CHECK i <= limit
	// 5: COMMIT i
	// 6: XFORM diff = limit - i
	// 7: JZ diff -> Sprung zu 10 (Ende)
	// 8: JUMP -> Sprung zu 3 (Schleifenkopf)
	// 9: HALT
	bytecode := []BytecodeInstruction{
		/* 0 */ {Op: OP_BYTE_READ, Dst: "i", OpId: "U16", ImmVal: 0},
		/* 1 */ {Op: OP_BYTE_READ, Dst: "limit", OpId: "U16", ImmVal: 5},
		/* 2 */ {Op: OP_BYTE_READ, Dst: "diff", OpId: "U16", ImmVal: 5},
		// --- Schleifenkopf ---
		/* 3 */ {Op: OP_BYTE_XFORM, Src: "i", Dst: "i", OpId: STR_INKR},
		/* 4 */ {Op: OP_BYTE_CHECK, Src: "i", OpId: "<=", ImmVal: 5},
		/* 5 */ {Op: OP_BYTE_COMMIT, Dst: "i"},
		/* 6 */ {Op: OP_BYTE_XFORM, Src: "diff", Dst: "diff", OpId: "-", ImmVal: 1},
		/* 7 */ {Op: OP_BYTE_JZ, Src: "diff", JumpAddr: 10}, // wenn diff == 0 -> Ende
		/* 8 */ {Op: OP_BYTE_JUMP, JumpAddr: 3},              // sonst Schleifenkopf
		/* 9 */ {Op: OP_BYTE_HALT},
	}

	vm := NewVM(bytecode)
	if err := vm.Run(); err != nil {
		t.Fatalf("Schleifenausführung fehlgeschlagen: %v", err)
	}

	finalI, ok := vm.LiveState["i"]
	if !ok || finalI.U16Val != 5 {
		t.Fatalf("Erwarteter Schleifen-Endwert 5, erhalten: %+v", finalI)
	}
	t.Logf("Schleife erfolgreich in %d Schritten ausgeführt! Finales i=%d", vm.Steps, finalI.U16Val)
}

// TEST-VM-05: Instruktionsbudget / Endlosschleifenschutz
func TestVM_InfiniteLoop_BudgetProtection(t *testing.T) {
	// Endlosschleife:
	// 0: READ x = 0
	// 1: JUMP -> 1
	bytecode := []BytecodeInstruction{
		/* 0 */ {Op: OP_BYTE_READ, Dst: "x", OpId: "U16", ImmVal: 0},
		/* 1 */ {Op: OP_BYTE_JUMP, JumpAddr: 1},
	}

	vm := NewVM(bytecode)
	vm.MaxSteps = 100 // Testlimit

	err := vm.Run()
	if err == nil {
		t.Fatalf("VM hätte bei Endlosschleife abbrechen müssen")
	}
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Errorf("Erwartete ErrBudgetExceeded, erhalten: %v", err)
	}
	t.Logf("Endlosschleifenschutz griff deterministisch bei Schritt %d", vm.Steps)
}
