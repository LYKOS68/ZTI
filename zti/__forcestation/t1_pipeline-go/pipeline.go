package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

var LogSpalten = []string{"timestamp", "input_file", "output_file", "sha256", "status"}

// LiesText liest eine UTF-8 Datei ein und entfernt optionales BOM.
// Liefert ValueError-Äquivalent oder Inhalt.
func LiesText(pfad string) (string, error) {
	b, err := os.ReadFile(pfad)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("Datei nicht gefunden: %s", pfad)
		}
		return "", fmt.Errorf("Datei nicht lesbar: %s (%w)", pfad, err)
	}

	// UTF-8-SIG BOM (\xef\xbb\xbf) abtrennen falls vorhanden
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})

	if !utf8.Valid(b) {
		return "", fmt.Errorf("Datei nicht als UTF-8 dekodierbar: %s", pfad)
	}
	return string(b), nil
}

// Verarbeite normalisiert auf NFC, trimmt Whitespace und wandelt zu GROSSBUCHSTABEN
// unter Beachtung von voller Unicode-Casing-Parität (z. B. 'ß' -> 'SS').
func Verarbeite(text string) string {
	normalized := norm.NFC.String(text)
	trimmed := strings.TrimSpace(normalized)
	return cases.Upper(language.Und).String(trimmed)
}

// SchreibeAtomar schreibt erst in eine kollisionssichere os.CreateTemp-Datei im Zielverzeichnis,
// passt Dateirechte auf 0666 an (analog zu Pythons open(..., "w")), synct per fsync,
// schließt sie und benennt atomar auf das Ziel um.
func SchreibeAtomar(pfad, text string) error {
	dir := filepath.Dir(pfad)
	base := filepath.Base(pfad)

	tmpFile, err := os.CreateTemp(dir, base+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmpFile.Name()
	committed := false
	defer func() {
		if !committed {
			_ = tmpFile.Close()
			_ = os.Remove(tmpName)
		}
	}()

	// CreateTemp setzt standardmäßig 0600. Python open("w") nutzt 0666 &^ umask.
	_ = tmpFile.Chmod(0666)

	if _, err := tmpFile.WriteString(text); err != nil {
		return err
	}
	if err := tmpFile.Sync(); err != nil {
		return err
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	if err := os.Rename(tmpName, pfad); err != nil {
		return err
	}
	committed = true
	return nil
}

// HashText erzeugt 64 Hex-Zeichen sha256 über die UTF-8-Bytes.
func HashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// LetzterHash prüft das Log und liefert:
// "" (fehlt/leer), "CORRUPT" (Format ungültig), oder den letzten sha256 Hash.
func LetzterHash(logPfad string) (string, error) {
	if _, err := os.Stat(logPfad); errors.Is(err, os.ErrNotExist) {
		return "", nil
	}

	data, err := os.ReadFile(logPfad)
	if err != nil {
		return "CORRUPT", nil
	}

	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = ';'
	r.FieldsPerRecord = -1

	records, err := r.ReadAll()
	if err != nil || len(records) == 0 {
		if len(data) == 0 {
			return "", nil
		}
		return "CORRUPT", nil
	}

	header := records[0]
	headerMap := make(map[string]int)
	for idx, h := range header {
		headerMap[strings.TrimSpace(h)] = idx
	}

	for _, col := range LogSpalten {
		if _, ok := headerMap[col]; !ok {
			return "CORRUPT", nil
		}
	}

	if len(records) == 1 {
		// Nur Header vorhanden
		return "", nil
	}

	lastRow := records[len(records)-1]
	shaIdx := headerMap["sha256"]
	if shaIdx >= len(lastRow) {
		return "CORRUPT", nil
	}

	return strings.TrimSpace(lastRow[shaIdx]), nil
}

// LogAnhaengen hängt einen Datensatz an das Semikolon-getrennte CSV-Log an.
func LogAnhaengen(logPfad string, eintrag map[string]string) error {
	neu := false
	if _, err := os.Stat(logPfad); errors.Is(err, os.ErrNotExist) {
		neu = true
	}

	f, err := os.OpenFile(logPfad, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	w.Comma = ';'

	if neu {
		if err := w.Write(LogSpalten); err != nil {
			return err
		}
	}

	row := make([]string, len(LogSpalten))
	for i, col := range LogSpalten {
		row[i] = eintrag[col]
	}

	if err := w.Write(row); err != nil {
		return err
	}
	w.Flush()
	return w.Error()
}

// Run führt die Gesamtkette aus und gibt den Exit-Code zurück:
// 0: Erfolg
// 1: Fehler beim Schreiben / I/O Fehler
// 2: Fehler beim Einlesen des Inputs
// 3: Write-Verify Mismatch
func Run(inputPfad, outputPfad, logPfad string, out, errOut io.Writer) int {
	fmt.Fprintln(out, "=== Pipeline gestartet ===")

	text, err := LiesText(inputPfad)
	if err != nil {
		fmt.Fprintf(out, "[1/5] FEHLER: %v\n", err)
		return 2
	}
	fmt.Fprintf(out, "[1/5] Eingelesen: %d Zeichen\n", utf8.RuneCountInString(text))

	verarbeitet := Verarbeite(text)
	fmt.Fprintf(out, "[2/5] Verarbeitet: %d Zeichen\n", utf8.RuneCountInString(verarbeitet))

	if err := SchreibeAtomar(outputPfad, verarbeitet); err != nil {
		fmt.Fprintf(errOut, "[3/5] I/O FEHLER beim Schreiben: %v\n", err)
		return 1
	}

	geschrieben, err := LiesText(outputPfad)
	if err != nil || geschrieben != verarbeitet {
		fmt.Fprintln(out, "[3/5] FEHLER: geschriebene Datei != Ergebnis")
		return 3
	}
	fmt.Fprintf(out, "[3/5] Atomar geschrieben + zurueckgelesen: %s\n", filepath.Base(outputPfad))

	h := HashText(geschrieben)
	fmt.Fprintf(out, "[4/5] Hash der DATEI (sha256): %s\n", h)

	last, _ := LetzterHash(logPfad)
	if last == "CORRUPT" {
		backup := logPfad + ".bak"
		_ = os.Rename(logPfad, backup)
		fmt.Fprintf(out, "[5/5] WARNUNG: Log war kaputt -> gesichert als %s, starte neu\n", filepath.Base(backup))
		last = ""
	}

	if last == h {
		fmt.Fprintln(out, "[5/5] Skip: Hash bereits im Log, kein neuer Eintrag")
	} else {
		err := LogAnhaengen(logPfad, map[string]string{
			"timestamp":   time.Now().Format("2006-01-02 15:04:05"),
			"input_file":  filepath.Base(inputPfad),
			"output_file": filepath.Base(outputPfad),
			"sha256":      h,
			"status":      "SUCCESS",
		})
		if err != nil {
			fmt.Fprintf(errOut, "[5/5] Log-Schreibfehler: %v\n", err)
			return 1
		}
		fmt.Fprintf(out, "[5/5] Export: neuer Eintrag in %s\n", filepath.Base(logPfad))
	}

	fmt.Fprintln(out, "=== Fertig ===")
	return 0
}
