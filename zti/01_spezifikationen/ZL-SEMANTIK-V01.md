# ZL Semantik & Ausführungsvertrag v0.1

> **Status:** Verbindlich für Test-Suite und Runtime  
> **Modus:** Deterministische Zustandsmaschine (Zero Ad-hoc)  
> **Axiom:** `(Z, T, I, S)` — Jede Zustandsänderung ist ein geschlossenes 3+1-Gatter.

---

## 1. Arithmetik & Typen: U16 Modular-Vertrag
* `U16` ist strikt als unvorzeichenbehafteter Ganzzahlring modulo $2^{16}$ ($\mathbb{Z}/65536\mathbb{Z}$) definiert.
* Wertebereich: $[0, 65535]$.
* Operator `INKR(x)` berechnet:
  $$\text{INKR}(x) = (x + 1) \pmod{65536}$$
* **Spezifikationsinvariante:** $65535 + 1 = 0$ ist eine **gültige, legale Transformation**.
* Es gilt **keine** implizite numerische Monotonie ($Z_1 \ge Z_0$ ist kein Standard-Gate). Monotonie existiert nur, wenn sie explizit per Prädikat (`PRÜFE ... WENN MONOTON_STEIGEND`) verlangt wird.

---

## 2. Das S-Gate: Formale Regeln für BIT[ZTIS]

Der Gattervektor $\text{BIT} \in \{0, 1\}^4$ bildet den internen Verifikationszustand eines Zyklus ab:

$$\text{BIT} = \begin{pmatrix} \text{Bit}_Z \\ \text{Bit}_T \\ \text{Bit}_I \\ \text{Bit}_S \end{pmatrix}$$

### A. Setz-Bedingungen (Transitions)
1. **`Bit_Z` (Substrat aktiv):** Wird auf `1` gesetzt, wenn `ERFASSE` erfolgreich ein Register im Staging initialisiert oder bindet.
2. **`Bit_T` (Transformation vollzogen):** Wird auf `1` gesetzt, wenn `FORME` fehlerfrei ausgeführt wurde. **Voraussetzung:** `Bit_Z == 1`.
3. **`Bit_I` (Invariante erfüllt):** Wird auf `1` gesetzt, wenn `PRÜFE` das Prädikat erfolgreich als `true` validiert. **Voraussetzung:** `Bit_T == 1`.
4. **`Bit_S` (Commit-Freigabe):** Wird gesetzt, wenn `BIND` ausgeführt wird und $\text{Bit}_Z \land \text{Bit}_T \land \text{Bit}_I == 1$ erfüllt ist.

### B. Fehlverhalten von `PRÜFE`
* Evaluiert ein `PRÜFE`-Prädikat zu `false`, bleibt **`Bit_I = 0`** (Gatter bleibt geschlossen).
* Ein geschlossenes I-Gate löst *nicht* sofort einen harten System-Crash aus, sondern verweigert dem nachfolgenden `BIND` die Legitimation.
* Erreicht die Ausführung `BIND`, während $\text{Bit} \neq 1110$ (vor dem Commit), schlägt `BIND` mit **`ErrGateClosed`** fehl.

### C. Rücksetz-Bedingungen (Resets & Rollback)
1. **Nach erfolgreichem `BIND`:** Atomare Spiegelung `LiveState = StagingState`. Der Gattervektor wird für den nächsten Zyklus auf `0000` zurückgesetzt.
2. **Bei Trap / Laufzeitfehler:** Tritt ein Typfehler, Division durch Null, Zugriffsfehler auf ein nicht existentes Register oder ein unberechtigter `BIND` auf:
   * $\text{BIT} \to 0000$
   * `StagingState` wird komplett verworfen (Rollback auf unveränderten `LiveState`).
   * Ausführung terminiert mit definiertem Fehlercode.

---

## 3. Grammatik & Eindeutigkeit (Syntax)

Alle 10 Schlüsselwörter erhalten feste Positionsfunktionen:

```ebnf
Program       := { Statement NL }
Statement     := ReadStmt | TransformStmt | CheckStmt | CommitStmt | HaltStmt

ReadStmt      := "ERFASSE" Ident "ALS" Type [ "VON" Literal ]
TransformStmt := "FORME" "VON" Ident "NACH" Ident "MIT" Operator
CheckStmt     := "PRÜFE" Ident "WENN" Predicate [ Literal ]
CommitStmt    := "BIND" Ident
HaltStmt      := "STOP" [ IntLiteral ]
```

### Semantische Eindeutigkeit:
* `FORME VON reg_a NACH reg_b MIT INKR`:
  * **Quelle (Input):** `reg_a` (bleibt im Staging erhalten, sofern `reg_a != reg_b`).
  * **Ziel (Output):** `reg_b` nimmt das Ergebnis auf.
  * **In-place:** `FORME VON reg_a NACH reg_a MIT INKR` transformiert `reg_a` im Staging, lässt aber `LiveState[reg_a]` bis zum `BIND` unberührt.

---

## 4. Verbindliche Testmatrix (ZL v0.1)

| ID | Testfall | Spezifizierte Erwartung |
| :--- | :--- | :--- |
| **TEST-01** | **U16-Überlauf** | `0xFFFF` + 1 ergibt `0x0000`. Gültig, kein Monotoniefehler, erfolgreicher Commit. |
| **TEST-02** | **Commit ohne Prüfung** | `ERFASSE` $\to$ `FORME` $\to$ `BIND` ohne `PRÜFE`. Abweisung durch statischen Validator und durch Laufzeit-Gate (`ErrGateClosed`). `LiveState` unverändert. |
| **TEST-03** | **Transformation ohne Z** | `FORME` auf nicht erfasstem Register. Definierter Fehler `ErrUnboundState`. Kein Zustandswechsel. |
| **TEST-04** | **In-place Isolation** | `FORME VON x NACH x`. Staging ändert sich; `LiveState[x]` bleibt bis zum finalen `BIND` exakt auf Ursprungswert. |
| **TEST-05** | **Deterministische Replikation** | Gleicher Initialzustand + gleiches Skript führen über $N$ Ausführungen auf identischen Endzustand und identische Hash-Signatur. |
| **TEST-06** | **Strikte Typ- & Operatorprüfung** | Zuweisung eines ungültigen Literals oder Ausführung eines unbekannten Operators wirft definierten Typ-/Op-Fehler ohne implizite Konvertierung. |
