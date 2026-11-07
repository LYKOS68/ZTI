package main

import (
	"fmt"
	"os"
)

func runScript(name string, src string) {
	fmt.Printf("\n=== %s ===\n", name)
	fmt.Println("Quelltext:")
	fmt.Println(src)

	lexer := NewLexer(src)
	parser := NewParser(lexer)
	prog, err := parser.ParseProgram()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Parse-Fehler: %v\n", err)
		return
	}

	// II/DOPPEL_WAECHTER: Provenienz & Kohärenz vor Kompilierung
	val := NewValidator()
	if err := val.Validate(prog); err != nil {
		fmt.Printf("❌ SCHEMA-ABWEISUNG (II/ELENCHOS): %v\n", err)
		return
	}

	compiler := NewCompiler()
	bytecode, err := compiler.Compile(prog)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Compile-Fehler: %v\n", err)
		return
	}

	vm := NewVM(bytecode)
	if err := vm.Run(); err != nil {
		fmt.Printf("❌ AUSFÜHRUNG ABGEWIESEN: %v\n", err)
		fmt.Printf("   Gate-Zustand: %04b | LiveState: %v | StagingState: %v\n", vm.Gate, vm.LiveState, vm.StagingState)
		return
	}

	fmt.Printf("✔ ERFOLGREICH GEBUNDEN!\n")
	fmt.Printf("   LiveState: %+v\n", vm.LiveState)
	fmt.Printf("   Gate-Bits: %04b (sauber auf 0000 zurückgesetzt)\n", vm.Gate)
}

func main() {
	fmt.Println("==============================================================")
	fmt.Println("      ZTI LANGUAGE (ZL) — ETAPPE 1: PROTOTYP & RUNTIME       ")
	fmt.Println("==============================================================")

	// Szenario 1: Normaler ZTIS-Zyklus
	script1 := `ERFASSE counter ALS U16 VON 100
FORME VON counter NACH counter MIT INKR
PRÜFE counter WENN RING_GLEICH 101
BIND counter
STOP 0
`
	runScript("Szenario 1: Valider 3+1 Transaktionszyklus", script1)

	// Szenario 2: U16 Modular-Überlauf (65535 + 1 = 0)
	script2 := `ERFASSE reg_ring ALS U16 VON 65535
FORME VON reg_ring NACH reg_ring MIT INKR
PRÜFE reg_ring WENN RING_GLEICH 0
BIND reg_ring
STOP 0
`
	runScript("Szenario 2: Modulare Ring-Arithmetik (U16 Wrap-Around)", script2)

	// Szenario 3: Commit ohne Prüfung (Gate schließt, Rollback greift)
	script3 := `ERFASSE reg_unsecure ALS U16 VON 42
FORME VON reg_unsecure NACH reg_unsecure MIT INKR
BIND reg_unsecure
STOP 0
`
	runScript("Szenario 3: Unberechtigter Commit (Gate-Bruch)", script3)
}

// Validator wird zwischen Parser und Compiler geschaltet (II/DOPPEL_WAECHTER - Provenienz & Kohärenz)
