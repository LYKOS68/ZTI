// Atomic-Pipe: Transformations- und Atom-Engine.
//
// Go-Portierung der Schritte 1–4 aus pipeline.py (lies_text, verarbeite,
// schreibe_atomar, hash_text) und des GatterNetzwerk64 aus gatter_nachweis.py.
// Das Logging (Schritt 5) gehört zu SecureLog und fehlt hier bewusst.
//
//	atomicpipe run <input|-> <output>
//	    lesen → NFC/trim/GROSS → atomar schreiben → zurücklesen → sha256
//	atomicpipe nachweis [-root DIR]
//	    Gatter-Nachweis Stufe 1, schreibt 11-gatter_nachweis_stufe1.json
//	atomicpipe takt <z_init> <t> [<t> ...]
//	    freie Taktfolge, eine JSON-Zeile pro Takt
//
// Input "-" liest von stdin. Exit-Codes: 0 OK, 1 Schreib-/I/O-Fehler,
// 2 Aufruf-/Eingabefehler, 3 Zurücklesen ungleich bzw. Invariante verletzt.
package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

const (
	exitOK       = 0
	exitIO       = 1
	exitUsage    = 2
	exitMismatch = 3
)

// ============================================================================
// 1. INPUT — lies_text
// ============================================================================

var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// DekodiereText entfernt ein optionales UTF-8-BOM und prüft auf gültiges UTF-8.
// Leere Eingabe ergibt "" (echter Zustand, kein Fehler).
func DekodiereText(name string, b []byte) (string, error) {
	b = bytes.TrimPrefix(b, utf8BOM)
	if !utf8.Valid(b) {
		return "", fmt.Errorf("nicht als UTF-8 dekodierbar: %s", name)
	}
	return string(b), nil
}

// LiesText liefert den vollständigen Text oder einen Fehler, nie "" bei fehlend/kaputt.
func LiesText(pfad string) (string, error) {
	b, err := os.ReadFile(pfad)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("Datei nicht gefunden: %s", pfad)
	}
	if err != nil {
		return "", fmt.Errorf("Datei nicht lesbar: %s (%w)", pfad, err)
	}
	return DekodiereText(pfad, b)
}

// ============================================================================
// 2. VERARBEITUNG — verarbeite
// ============================================================================

// Verarbeite ist rein und idempotent: NFC-normalisieren, trimmen, GROSS ('ß' → "SS").
func Verarbeite(text string) string {
	return cases.Upper(language.Und).String(strings.TrimSpace(norm.NFC.String(text)))
}

// ============================================================================
// 3. OUTPUT — schreibe_atomar
// ============================================================================

// SchreibeAtomar garantiert: nach nil-Return steht b vollständig unter pfad.
// Erst <pfad>.<zufall>.tmp schreiben + fsync, dann atomares rename.
// Die tmp-Datei entsteht mit 0666 &^ umask, wie ein normales Create.
func SchreibeAtomar(pfad string, b []byte) error {
	tmp, err := createTmp(pfad)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tmp.Close()
			_ = os.Remove(tmp.Name())
		}
	}()

	if _, err := tmp.Write(b); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), pfad); err != nil {
		return err
	}
	committed = true
	return nil
}

// createTmp legt eine kollisionsfreie tmp-Datei neben pfad an.
// Anders als os.CreateTemp (0600) greift hier die umask.
func createTmp(pfad string) (*os.File, error) {
	for range 16 {
		var r [6]byte
		if _, err := rand.Read(r[:]); err != nil {
			return nil, err
		}
		name := pfad + "." + hex.EncodeToString(r[:]) + ".tmp"
		f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if !errors.Is(err, os.ErrExist) {
			return f, err
		}
	}
	return nil, fmt.Errorf("keine freie tmp-Datei für %s", pfad)
}

// ============================================================================
// 4. HASH — hash_text
// ============================================================================

// HashText liefert 64 Hex-Zeichen sha256 über die UTF-8-Bytes.
func HashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// ============================================================================
// 5. GATTERNETZWERK64 — 4 Blöcke à 16 Gatter: Z | T | I | Φ
// ============================================================================

type GatterNetzwerk64 struct {
	RegZ   uint16 // Gatter 0..15:  Zustand
	RegT   uint16 // Gatter 16..31: Transformation
	RegI   uint16 // Gatter 32..47: Informations-Gate
	RegPhi uint16 // Gatter 48..63: Phasen-Zähler
}

func (g *GatterNetzwerk64) Reset(zInit uint16) {
	*g = GatterNetzwerk64{RegZ: zInit}
}

// EvalGateI liefert Gate-Bit und Gate-Maske.
// Bit = 1 gdw. T ≠ 0 und T nicht vollständig in Z enthalten (kein Inversionsecho).
func EvalGateI(z, t uint16) (bit uint8, mask uint16) {
	if t != 0 && z&t != t {
		bit = 1
	}
	return bit, z ^ t
}

// TaktErgebnis ist das Protokoll eines Takts; Feldreihenfolge wie im Original.
type TaktErgebnis struct {
	Takt                uint16 `json:"takt"`
	EingabeT            string `json:"eingabe_T"`
	Z0                  string `json:"z0"`
	GateBit             uint8  `json:"gate_bit"`
	GateMask            string `json:"gate_mask"`
	Z1                  string `json:"z1"`
	Phi1                uint16 `json:"phi1"`
	Status              string `json:"status"`
	DauerNs             int64  `json:"dauer_ns"`
	InvarianteBestanden bool   `json:"invariante_bestanden"`
}

func hex16(v uint16) string { return fmt.Sprintf("0x%x", v) }

// TaktSchritt: Z0 | T | I | Φ0 → Z1 | Φ1. Z rechnet modulo 2^16, Φ modulo 64.
func (g *GatterNetzwerk64) TaktSchritt(t uint16) TaktErgebnis {
	start := time.Now()

	z0, phi0 := g.RegZ, g.RegPhi
	g.RegT = t
	bit, mask := EvalGateI(z0, t)
	g.RegI = mask

	z1, status := z0, "CLEAN"
	if bit == 1 {
		z1, status = z0+t, "UPDATED"
	}
	g.RegZ = z1

	phi1 := (phi0 + 1) % 64
	g.RegPhi = phi1

	// Ein 16-Bit-Überlauf (z0+t > 0xFFFF) verletzt die Monotonie und wird gemeldet.
	monoton := z1 == z0
	if bit == 1 {
		monoton = z1 >= z0
	}

	return TaktErgebnis{
		Takt:                phi0,
		EingabeT:            hex16(t),
		Z0:                  hex16(z0),
		GateBit:             bit,
		GateMask:            hex16(mask),
		Z1:                  hex16(z1),
		Phi1:                phi1,
		Status:              status,
		DauerNs:             time.Since(start).Nanoseconds(),
		InvarianteBestanden: monoton && phi1 == (phi0+1)%64,
	}
}

// ============================================================================
// 6. NACHWEIS — run_gatter_nachweis, Projekt-Root-Suche
// ============================================================================

const beweisDatei = "11-gatter_nachweis_stufe1.json"

type ProtokollEintrag struct {
	Schritt  int          `json:"schritt"`
	Fall     string       `json:"fall"`
	Ergebnis TaktErgebnis `json:"ergebnis"`
}

type Beweis struct {
	NachweisStufe string             `json:"nachweis_stufe"`
	GatterGesamt  int                `json:"gatter_gesamt"`
	Bloecke       map[string]string  `json:"bloecke"`
	Ergebnis      string             `json:"ergebnis"`
	Protokoll     []ProtokollEintrag `json:"protokoll"`
}

// FindeRoot sucht ab start aufwärts den ersten Ordner mit src/ oder
// 01_spezifikationen/. Das Dateisystem-Root selbst wird nicht geprüft.
func FindeRoot(start string) (string, bool) {
	for p := filepath.Clean(start); filepath.Dir(p) != p; p = filepath.Dir(p) {
		for _, marker := range []string{"src", "01_spezifikationen"} {
			if _, err := os.Stat(filepath.Join(p, marker)); err == nil {
				return p, true
			}
		}
	}
	return "", false
}

// BeweisPfad: root/src/03_kern-engine/BEWEIS/<datei>. Ohne Root (root == ""):
// ab Programmordner, dann ab Arbeitsordner suchen; ohne Treffer in den Arbeitsordner.
func BeweisPfad(root string) (string, error) {
	if root == "" {
		var starts []string
		if exe, err := os.Executable(); err == nil {
			starts = append(starts, filepath.Dir(exe))
		}
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		starts = append(starts, cwd)
		for _, s := range starts {
			if r, ok := FindeRoot(s); ok {
				root = r
				break
			}
		}
		if root == "" {
			return filepath.Join(cwd, beweisDatei), nil
		}
	}
	return filepath.Join(root, "src", "03_kern-engine", "BEWEIS", beweisDatei), nil
}

const linie = "=============================================================="

func nachweis(root string, out, errOut io.Writer) int {
	fmt.Fprintln(out, linie)
	fmt.Fprintln(out, "      STUFE 1: FORMALER GATTER-NACHWEIS (Z | T | I | Φ → Z')  ")
	fmt.Fprintln(out, linie)

	var sim GatterNetzwerk64
	sim.Reset(0x0100)

	res1 := sim.TaktSchritt(0x0005)
	fmt.Fprintf(out, "\n[TAKT 0 -> 1] Input T=0x0005 | Bit=%d | Z0=%s -> Z1=%s | Status=%s\n",
		res1.GateBit, res1.Z0, res1.Z1, res1.Status)
	if res1.GateBit != 1 || !res1.InvarianteBestanden {
		fmt.Fprintln(out, "  ✘ TAKT 1 NICHT BESTANDEN")
		return exitMismatch
	}
	fmt.Fprintln(out, "  ✔ TAKT 1 BESTANDEN: Bit 1 gesetzt, Zustand Z1 exakt fortgeschrieben.")

	res2 := sim.TaktSchritt(0x0000)
	fmt.Fprintf(out, "\n[TAKT 1 -> 2] Input T=0x0000 | Bit=%d | Z0=%s -> Z1=%s | Status=%s\n",
		res2.GateBit, res2.Z0, res2.Z1, res2.Status)
	if res2.GateBit != 0 || res2.Z1 != res2.Z0 || !res2.InvarianteBestanden {
		fmt.Fprintln(out, "  ✘ TAKT 2 NICHT BESTANDEN")
		return exitMismatch
	}
	fmt.Fprintln(out, "  ✔ TAKT 2 BESTANDEN: Bit 0 gesetzt, Zustand Z1 == Z0 (Byte-Gleichheit).")

	b, err := json.MarshalIndent(Beweis{
		NachweisStufe: "STUFE_1_ARCHITEKTUR_FORMAL_PROVEN",
		GatterGesamt:  64,
		Bloecke: map[string]string{
			"Z_gatter_0_15":    "16-Bit Zustand Register",
			"T_gatter_16_31":   "16-Bit Transformation Register",
			"I_gatter_32_47":   "16-Bit Gate Logik",
			"PHI_gatter_48_63": "16-Bit Phasen Zähler",
		},
		Ergebnis: "BESTANDEN",
		Protokoll: []ProtokollEintrag{
			{Schritt: 1, Fall: "VALIDE_TRANSFORMATION", Ergebnis: res1},
			{Schritt: 2, Fall: "LEERLAUF_DUPLIKAT", Ergebnis: res2},
		},
	}, "", "  ")
	if err != nil {
		fmt.Fprintf(errOut, "FEHLER: %v\n", err)
		return exitIO
	}

	pfad, err := BeweisPfad(root)
	if err == nil {
		err = os.MkdirAll(filepath.Dir(pfad), 0o777)
	}
	if err == nil {
		err = SchreibeAtomar(pfad, append(b, '\n'))
	}
	if err != nil {
		fmt.Fprintf(errOut, "FEHLER: Protokoll nicht geschrieben: %v\n", err)
		return exitIO
	}

	fmt.Fprintf(out, "\n-> Maschinenlesbares Protokoll gespeichert in: %s\n", pfad)
	fmt.Fprintln(out, "\n"+linie)
	fmt.Fprintln(out, "   STUFE 1 DER STRIKTEN PRÜFKETTE VOLLSTÄNDIG BESTANDEN!     ")
	fmt.Fprintln(out, linie)
	return exitOK
}

// ============================================================================
// 7. CLI
// ============================================================================

const usage = `Aufruf:
  atomicpipe run <input|-> <output>
  atomicpipe nachweis [-root DIR]
  atomicpipe takt <z_init> <t> [<t> ...]`

func run(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, usage)
		return exitUsage
	}
	switch args[0] {
	case "run":
		if len(args) != 3 {
			break
		}
		return pipe(args[1], args[2], in, out, errOut)
	case "nachweis":
		fs := flag.NewFlagSet("nachweis", flag.ContinueOnError)
		fs.SetOutput(errOut)
		root := fs.String("root", "", "Projekt-Root statt automatischer Suche")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
			break
		}
		return nachweis(*root, out, errOut)
	case "takt":
		if len(args) < 3 {
			break
		}
		return takt(args[1], args[2:], out, errOut)
	}
	fmt.Fprintln(errOut, usage)
	return exitUsage
}

func pipe(inputPfad, outputPfad string, in io.Reader, out, errOut io.Writer) int {
	fmt.Fprintln(out, "=== Atomic-Pipe gestartet ===")

	text, err := liesInput(inputPfad, in)
	if err != nil {
		fmt.Fprintf(out, "[1/4] FEHLER: %v\n", err)
		return exitUsage
	}
	fmt.Fprintf(out, "[1/4] Eingelesen: %d Zeichen\n", utf8.RuneCountInString(text))

	verarbeitet := Verarbeite(text)
	fmt.Fprintf(out, "[2/4] Verarbeitet: %d Zeichen\n", utf8.RuneCountInString(verarbeitet))

	if err := SchreibeAtomar(outputPfad, []byte(verarbeitet)); err != nil {
		fmt.Fprintf(errOut, "[3/4] I/O FEHLER beim Schreiben: %v\n", err)
		return exitIO
	}
	// Die DATEI prüfen, nicht den Speicher.
	geschrieben, err := LiesText(outputPfad)
	if err != nil || geschrieben != verarbeitet {
		fmt.Fprintln(out, "[3/4] FEHLER: geschriebene Datei != Ergebnis")
		return exitMismatch
	}
	fmt.Fprintf(out, "[3/4] Atomar geschrieben + zurückgelesen: %s\n", filepath.Base(outputPfad))

	fmt.Fprintf(out, "[4/4] Hash der DATEI (sha256): %s\n", HashText(geschrieben))
	fmt.Fprintln(out, "=== Fertig ===")
	return exitOK
}

func liesInput(pfad string, in io.Reader) (string, error) {
	if pfad != "-" {
		return LiesText(pfad)
	}
	b, err := io.ReadAll(in)
	if err != nil {
		return "", fmt.Errorf("stdin nicht lesbar: %w", err)
	}
	return DekodiereText("stdin", b)
}

func takt(zInit string, ts []string, out, errOut io.Writer) int {
	z, err := parse16(zInit)
	if err != nil {
		fmt.Fprintf(errOut, "FEHLER: z_init: %v\n", err)
		return exitUsage
	}
	werte := make([]uint16, len(ts))
	for i, s := range ts {
		if werte[i], err = parse16(s); err != nil {
			fmt.Fprintf(errOut, "FEHLER: t[%d]: %v\n", i, err)
			return exitUsage
		}
	}

	var sim GatterNetzwerk64
	sim.Reset(z)
	code := exitOK
	for _, t := range werte {
		res := sim.TaktSchritt(t)
		b, _ := json.Marshal(res)
		fmt.Fprintf(out, "%s\n", b)
		if !res.InvarianteBestanden {
			code = exitMismatch
		}
	}
	return code
}

// parse16 akzeptiert 0x.., 0b.., 0o.. und dezimal, nur 16 Bit.
func parse16(s string) (uint16, error) {
	v, err := strconv.ParseUint(s, 0, 16)
	if err != nil {
		return 0, fmt.Errorf("%q ist kein 16-Bit-Wert", s)
	}
	return uint16(v), nil
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
