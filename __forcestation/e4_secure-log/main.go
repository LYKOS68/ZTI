// SecureLog: korruptionssicheres Protokoll mit .bak-Rettung.
//
// Go-Portierung von Schritt 5 aus pipeline.py (letzter_hash, log_anhaengen,
// LOG_SPALTEN). Invariante: Ein Eintrag wird nur angehängt, wenn sich der Hash
// gegenüber dem letzten Eintrag ändert. Ein kaputtes Log wird erkannt und
// gesichert, nie still erweitert.
//
//	securelog check  <log.csv>
//	    Zustand: MISSING, EMPTY, VALID (letzter Hash) oder CORRUPT (Grund)
//	securelog append [-input NAME] [-status S] <log.csv> <datei>
//	    sha256 der Datei; kaputtes Log → .bak, gleicher Hash → Skip, sonst Eintrag
//
// Strenger als das Original: abgeschnittene letzte Zeile, abweichender Header,
// ungleiche Feldzahl und ungültiger Hash gelten als CORRUPT; Lesefehler
// verschieben nichts; eine vorhandene .bak wird nie überschrieben.
// Exit-Codes: 0 OK, 1 Log CORRUPT (check), 2 Aufruf-/I/O-Fehler.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

const (
	exitOK      = 0
	exitCorrupt = 1
	exitUsage   = 2
)

var LogSpalten = []string{"timestamp", "input_file", "output_file", "sha256", "status"}

const shaSpalte = 3

// ============================================================================
// 1. ZUSTAND DES LOGS — letzter_hash
// ============================================================================

type LogState uint8

const (
	LogMissing LogState = iota // Datei fehlt
	LogEmpty                   // 0 Bytes oder nur Header
	LogValid                   // Header und alle Zeilen in Ordnung
	LogCorrupt                 // Format kaputt → .bak-Rettung
)

var logStateNames = [...]string{"MISSING", "EMPTY", "VALID", "CORRUPT"}

func (s LogState) String() string { return logStateNames[s] }

type LogInfo struct {
	State     LogState
	LastHash  string // nur bei LogValid
	Eintraege int
	Grund     string // nur bei LogCorrupt
}

// PruefeLog liest das Log. Ein Formatfehler ist kein error, sondern LogCorrupt.
// error nur, wenn die Datei existiert, aber nicht lesbar ist: dann darf
// nichts verschoben werden.
func PruefeLog(pfad string) (LogInfo, error) {
	data, err := os.ReadFile(pfad)
	if errors.Is(err, os.ErrNotExist) {
		return LogInfo{State: LogMissing}, nil
	}
	if err != nil {
		return LogInfo{}, fmt.Errorf("Log nicht lesbar: %w", err)
	}
	if len(data) == 0 {
		return LogInfo{State: LogEmpty}, nil
	}
	corrupt := func(format string, args ...any) (LogInfo, error) {
		return LogInfo{State: LogCorrupt, Grund: fmt.Sprintf(format, args...)}, nil
	}

	if !bytes.HasSuffix(data, []byte("\n")) {
		return corrupt("letzte Zeile abgeschnitten (kein Zeilenende)")
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = ';'
	records, err := r.ReadAll() // FieldsPerRecord 0: alle Zeilen so breit wie der Header
	if err != nil {
		return corrupt("CSV: %v", err)
	}
	if len(records) == 0 {
		return corrupt("keine Header-Zeile")
	}
	if !slices.Equal(records[0], LogSpalten) {
		return corrupt("Header %q statt %q", records[0], LogSpalten)
	}
	for i, row := range records[1:] {
		if !gueltigerHash(row[shaSpalte]) {
			return corrupt("Zeile %d: sha256 %q ungültig", i+2, row[shaSpalte])
		}
	}

	rows := records[1:]
	if len(rows) == 0 {
		return LogInfo{State: LogEmpty}, nil
	}
	return LogInfo{State: LogValid, LastHash: rows[len(rows)-1][shaSpalte], Eintraege: len(rows)}, nil
}

func gueltigerHash(s string) bool {
	if len(s) != 2*sha256.Size {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

// ============================================================================
// 2. RETTUNG — kaputtes Log nach .bak, nie überschreiben
// ============================================================================

// RetteLog verschiebt pfad nach <pfad>.bak, bei Belegung nach <pfad>.bak.1, .2, …
func RetteLog(pfad string) (string, error) {
	for i := 0; i < 1000; i++ {
		bak := pfad + ".bak"
		if i > 0 {
			bak += "." + strconv.Itoa(i)
		}
		if _, err := os.Lstat(bak); !errors.Is(err, os.ErrNotExist) {
			continue
		}
		return bak, os.Rename(pfad, bak)
	}
	return "", fmt.Errorf("kein freier .bak-Name für %s", pfad)
}

// ============================================================================
// 3. ANHÄNGEN — log_anhaengen
// ============================================================================

type Eintrag struct {
	Timestamp  time.Time
	InputFile  string
	OutputFile string
	SHA256     string
	Status     string
}

func (e Eintrag) row() []string {
	ts := e.Timestamp.Format("2006-01-02 15:04:05")
	return []string{ts, e.InputFile, e.OutputFile, e.SHA256, e.Status}
}

// HaengeAn schreibt eine Zeile und fsynct. Der Header kommt dazu, wenn die
// Datei fehlt oder leer ist (das Original vergisst ihn bei 0 Bytes).
func HaengeAn(pfad string, e Eintrag) error {
	f, err := os.OpenFile(pfad, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o666)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Comma = ';'
	if st.Size() == 0 {
		_ = w.Write(LogSpalten)
	}
	_ = w.Write(e.row())
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}

	// Ein einziger Write: entweder die ganze Zeile oder (bei Absturz) eine
	// abgeschnittene, die PruefeLog als CORRUPT erkennt.
	if _, err := f.Write(buf.Bytes()); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return f.Close()
}

// ============================================================================
// 4. PROTOKOLLIEREN — Schritt 5 der Pipeline
// ============================================================================

type Ergebnis uint8

const (
	Angehaengt Ergebnis = iota
	Uebersprungen
)

// Protokolliere: kaputtes Log retten, bei gleichem letzten Hash überspringen,
// sonst anhängen. bak ist der Rettungspfad oder "".
func Protokolliere(logPfad string, e Eintrag) (res Ergebnis, bak string, info LogInfo, err error) {
	info, err = PruefeLog(logPfad)
	if err != nil {
		return 0, "", info, err
	}
	if info.State == LogCorrupt {
		if bak, err = RetteLog(logPfad); err != nil {
			return 0, "", info, err
		}
	}
	if info.State == LogValid && info.LastHash == e.SHA256 {
		return Uebersprungen, bak, info, nil
	}
	return Angehaengt, bak, info, HaengeAn(logPfad, e)
}

func HashDatei(pfad string) (string, error) {
	f, err := os.Open(pfad)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ============================================================================
// 5. CLI
// ============================================================================

const usage = `Aufruf:
  securelog check  <log.csv>
  securelog append [-input NAME] [-status S] <log.csv> <datei>`

func run(args []string, now func() time.Time, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, usage)
		return exitUsage
	}
	switch args[0] {
	case "check":
		if len(args) != 2 {
			break
		}
		return check(args[1], out, errOut)
	case "append":
		fs := flag.NewFlagSet("append", flag.ContinueOnError)
		fs.SetOutput(errOut)
		input := fs.String("input", "", "Wert für die Spalte input_file")
		status := fs.String("status", "SUCCESS", "Wert für die Spalte status")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 2 {
			break
		}
		return appendLog(fs.Arg(0), fs.Arg(1), *input, *status, now(), out, errOut)
	}
	fmt.Fprintln(errOut, usage)
	return exitUsage
}

func check(logPfad string, out, errOut io.Writer) int {
	info, err := PruefeLog(logPfad)
	if err != nil {
		fmt.Fprintf(errOut, "FEHLER: %v\n", err)
		return exitUsage
	}
	name := filepath.Base(logPfad)
	switch info.State {
	case LogValid:
		fmt.Fprintf(out, "VALID    %s eintraege=%d letzter=%s\n", name, info.Eintraege, info.LastHash)
	case LogCorrupt:
		fmt.Fprintf(out, "CORRUPT  %s: %s\n", name, info.Grund)
		return exitCorrupt
	default:
		fmt.Fprintf(out, "%-8s %s\n", info.State, name)
	}
	return exitOK
}

func appendLog(logPfad, datei, input, status string, now time.Time, out, errOut io.Writer) int {
	h, err := HashDatei(datei)
	if err != nil {
		fmt.Fprintf(errOut, "FEHLER: Datei nicht lesbar: %v\n", err)
		return exitUsage
	}
	res, bak, info, err := Protokolliere(logPfad, Eintrag{
		Timestamp:  now,
		InputFile:  input,
		OutputFile: filepath.Base(datei),
		SHA256:     h,
		Status:     status,
	})
	if bak != "" {
		fmt.Fprintf(out, "WARNUNG: Log war kaputt (%s) -> gesichert als %s, starte neu\n",
			info.Grund, filepath.Base(bak))
	}
	if err != nil {
		fmt.Fprintf(errOut, "FEHLER: %v\n", err)
		return exitUsage
	}
	if res == Uebersprungen {
		fmt.Fprintf(out, "Skip: Hash bereits im Log, kein neuer Eintrag (%s)\n", h)
		return exitOK
	}
	fmt.Fprintf(out, "Export: neuer Eintrag in %s (%s)\n", filepath.Base(logPfad), h)
	return exitOK
}

func main() {
	os.Exit(run(os.Args[1:], time.Now, os.Stdout, os.Stderr))
}
