# ETAPPENPLAN ZL (ZTIS Language)

> **Formgesetz:** `Schema → Adressierung → Zuordnung → Validierung → Kompilierung`  
> **Architektur-Kaskade:** `Kernel → Runtime → Pipeline → Operator → Sprache → App` *(strikte Einweg-Bindung)*  
> **Kernzelle:** `PSID-001` (Primär: *Wahrnehmung → Entscheidung → Handlung → Rückkopplung*) `↔` `NSID-001` (Neben: *Prüfung → Korrektur → Freigabe → Prüfung*)  
> **Zustandsschranke:** `BIT[ZTIS] ∈ {0,1}⁴` | Vorwärts: `A→B→C→D` | Rückwärts: `C→B→A→D` | Gate: `1111` (Commit) sonst `0000` (Drop)

---

### ETAPPE 0: Spezifikation & Go-Fundament
* **Ebene:** `Schema` | **Ziel:** Formale Definition des Minimal-Kerns und Initialisierung des Go-Moduls.
* **Inhalt & Verträge:**
  * Modul-Init: `github.com/zti/zl` unter Go 1.24+ | Verzeichnisstruktur: `cmd/zl`, `pkg/{token,lexer,ast,parser,validator,opcode,compiler,vm}`.
  * Opcodes (ISA): `0x01: OP_READ` (ERFASSE / Z) `|` `0x02: OP_XFORM` (FORME / T) `|` `0x03: OP_CHECK` (PRÜFE / I) `|` `0x04: OP_COMMIT` (BIND / S) `|` `0xFF: OP_HALT` (STOP).
  * Schlüsselwörter: 5 Verben (`ERFASSE`, `FORME`, `PRÜFE`, `BIND`, `STOP`) `+` 5 Positionsanker (`VON`, `NACH`, `MIT`, `WENN`, `ALS`).
* **Exit-Kriterium:** Vollständige Tabellenverträge in `pkg/token` und `pkg/opcode` mit $O(1)$-Lookup-Tabellen.

---

### ETAPPE 1: Lexer & Positions-Parser
* **Ebene:** `Adressierung` | **Ziel:** Deterministische Zerlegung von ZL-Quelltext in typisierte AST-Knoten unter striktem Positionsgesetz.
* **Inhalt & Verträge:**
  * Lexer: Streaming-Scanner `io.Reader → []Token` mit exakter Positionsauflösung `(Zeile, Spalte)`.
  * Parser: Enforcing der Satzschablone `VERB [QUELLE] [NACH ZIEL] [MIT OPERATOR] [WENN BEDINGUNG]`.
  * Fraktale Adressierung: Jeder AST-Knoten trägt Koordinaten-Signatur `PID::SID::KID` (Muster::Schleife::Instanz).
* **Exit-Kriterium:** Parser akzeptiert syntaktisch korrekte ZL-Zeilen; ungültige Wortreihenfolgen werden deterministisch mit Zeilenfehler abgewiesen (`parser_test.go`).

---

### ETAPPE 2: Semantischer ZTI-Validator
* **Ebene:** `Zuordnung` | **Ziel:** Statische Integritätsprüfung vor der Kompilierung (Verhinderung unvollständiger ZTIS-Zyklen).
* **Inhalt & Verträge:**
  * Axiom-Prüfung: *Ohne Z kein T* (kein `FORME` ohne vorheriges `ERFASSE`) `|` *Ohne I kein S* (kein `BIND` ohne vorheriges `PRÜFE`).
  * Typ- und Registerprüfung: Validierung der Bindung von `ALS [TYP]` und Integrität von Registernamen.
  * Schichten-Isolierung: Prüfung gegen illegale Vorwärts-/Rückwärts-Sprünge außerhalb des definierten Kontrollflusses.
* **Exit-Kriterium:** Validator erkennt ZTI-Verletzungen statisch und blockiert Kompilierung bei unvollständigen Zyklen (`validator_test.go`).

---

### ETAPPE 3: Bytecode-Compiler
* **Ebene:** `Validierung` | **Ziel:** Übersetzung des validierten AST in binären Bytecode-Stream.
* **Inhalt & Verträge:**
  * Bytecode-Format: `[MAGIC: 4B][VERSION: 2B][INST_COUNT: 2B] + N * [OP: 1B][SRC_ID: 1B][DST_ID: 1B][AUX_ID: 2B]`.
  * String- und Literal-Pool: Flache Indizierung für Typen, Operatoren und Bedingungen.
  * Compiler-Pipeline: Lineare AST-Transformation ohne Seiteneffekte mit $O(N)$-Laufzeit.
* **Exit-Kriterium:** Generierung eines deterministischen Byte-Arrays aus ZL-Quelltext; Re-Disassembly stimmt 1:1 mit AST überein (`compiler_test.go`).

---

### ETAPPE 4: Virtual Machine & 3+1 State-Gate Runtime
* **Ebene:** `Kompilierung` | **Ziel:** Ausführung der Opcodes mit getrenntem Staging/Live-Speicher und Bit-Gatter-Schranke.
* **Inhalt & Verträge:**
  * Speicherarchitektur: `Z_live` (persistenter Zustand) `|` `Z_staging` (Transaktionsraum) `|` `BIT[ZTIS]` (Gatter-Register).
  * Gate-Logik:
    * `OP_READ` setzt `BIT[Z]=1`.
    * `OP_XFORM` erfordert `BIT[Z]==1`, führt Transformation in Staging aus, setzt `BIT[T]=1`.
    * `OP_CHECK` erfordert `BIT[T]==1`, prüft Invariante, setzt `BIT[I]=1`.
    * `OP_COMMIT` erfordert `BIT[ZTIS]==1111` (`S-Syncpoint`), spiegelt Staging atomar in Live-State.
    * Jeder Fehler/Invariantenbruch setzt `BIT[ZTIS]=0000` und führt Rollback auf `Z_live` aus.
  * Built-in Basisoperatoren: `GROSS`, `KLEIN`, `TRIM`, `INKR`.
* **Exit-Kriterium:** Erfolgreicher Commit bei validem Ablauf; deterministischer Rollback bei fehlschlagender Prüfung (`vm_test.go`).

---

### ETAPPE 5: CLI, Rückwärts-Analyse & Isomorphie-Nachweis
* **Ebene:** `Gesamtsystem` | **Ziel:** CLI-Tooling und automatisierter Beweis beider Richtungen (Vorwärts `A→B→C→D` & Rückwärts `C→B→A→D`).
* **Inhalt & Verträge:**
  * CLI-Befehle (`cmd/zl`):
    * `zl run <file.zl>`: Kompiliert und führt direkt in VM aus.
    * `zl compile <file.zl> -o <out.zlb>`: Schreibt Roh-Bytecode.
    * `zl disasm <file.zlb>`: Dekompiliert Bytecode in lesbare Opcode-Sequenz.
    * `zl audit <file.zl>`: Führt Rückwärts-Analyse durch (`Ergebnis C → Transformation B → Zustand A → Gate D`).
  * End-to-End Suite: 100% Testabdeckung aller 5 Opcodes, 10 Schlüsselwörter und Fehlerpfade.
* **Exit-Kriterium:** Vollständig grüner Testlauf von Quelltext bis Zustandsänderung und Rückwärts-Audit ohne externe Abhängigkeiten.


---

## DRAFT: ZTIS-Matrix & O(1)-Dispatch-Architektur (Append-Only)

> **Prinzip:** Die Bedeutung jeder Zelle ergibt sich ausschließlich aus ihrer Koordinate `(X, Y)`. Der Zelleninhalt ist Wert, nicht Definition.  
> **Dimensionen:** `Y1..Y64` (64 Zustands-Slots) `×` `X1..X8` (Unveränderliche semantische Achsen).  
> **Laufzeit-Invariante:** Ablösung von $O(n)$-String-Suchen durch direkte $O(1)$-Array-Indizierung (`handler_table[MSG_COUNT]` / `dispatchTable[OP]`).

### 1. Semantische Achsen (Spalten X1..X8)
* **X1 (ZTIS-Säule):** `STATE` | `TRANSFORM` | `INFO` | `SYNC` *(2 Bit)*
* **X2 (Layer):** `ALPHA` | `BRAVO` | `CHARLIE` | `DELTA` *(2 Bit)*
* **X3 (Ebene):** `OBEN` | `MITTE` | `UNTEN` *(2 Bit)*
* **X4 (Operator):** `SETZEN` | `VERKNÜPFEN` | `PRÜFEN` | `FREIGEBEN` | `ABBRECHEN` | `ZURÜCKSETZEN` *(3 Bit)*
* **X5 (Validator):** `KONSISTENZ` | `INTEGRITÄT` | `BEREICH` | `GRENZE` | `SYNC` *(3 Bit)*
* **X6 (Zustands-ID):** Format `L[Layer].[Type].[4-stellige Nummer]` (z. B. `L1.S.0001`, `L2.O.0004`)
* **X7 (Übergangsregel):** `NEXT` | `JUMP` | `WAIT` | `GO` | `STOP` *(3 Bit)*
* **X8 (Bedeutung):** Freier Text / Semantischer Deskriptor (max. 200 Zeichen)

### 2. O(1) Dispatcher-Muster (Go-Portierung des C-Architekturvertrags)
* **Problem:** Lineare Stringsuche (`strcmp` auf Befehlsnamen) skaliert mit $O(n)$ und bricht bei komplexen Pipelines.
* **Lösung:** Numerische Opcode-Indizierung direkt im Funktionszeiger-Array ohne `if/else`-Kaskaden:
```go
type Handler func(*VM, Instruction) error

// O(1) Dispatcher-Tabelle (Rick-Style / C-Vorbild handler_table)
var dispatchTable = [64]Handler{
    OP_READ:   onRead,   // X1=STATE, X4=SETZEN,     X7=NEXT
    OP_XFORM:  onXform,  // X1=TRANSFORM, X4=VERKNÜPFEN, X7=NEXT
    OP_CHECK:  onCheck,  // X1=INFO,  X4=PRÜFEN,     X7=GO/WAIT
    OP_COMMIT: onCommit, // X1=SYNC,  X4=FREIGEBEN,  X7=NEXT
    OP_HALT:   onHalt,   // X7=STOP
}

func (vm *VM) Dispatch(inst Instruction) error {
    return dispatchTable[inst.Op](vm, inst) // O(1) Direktausführung
}
```
* **Kopplung:** Jede Zeile in `matrix.csv` definiert statisch die Vorbedingungen (`X1..X5`), die Übergangsregel (`X7`) und die Zieladresse für die $O(1)$-Ausführung in der VM.
