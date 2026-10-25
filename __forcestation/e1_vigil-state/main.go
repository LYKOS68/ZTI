// Vigil-State: Zustands- und Invarianten-Engine.
//
// Go-Portierung von SharedState + konsistent() (loop.py), dem validierten
// RESET mit Post-Heal-Check (CharlieGuard, loop.py), dem Pre-Wake-Check
// (ChronoLoop.step / zti_step) und der Loop-Seite von capture_zustand
// (zti_runner.py). Der Zustand liegt als JSON-Datei vor.
//
//	vigil init    <state.json>  legt SharedState() an (data=null, tick=0, last_pulse=0, SLEEP)
//	vigil check   <state.json>  prüft konsistent()
//	vigil capture <state.json>  misst Z: loop_tick, loop_status, loop_data, loop_konsistent
//	vigil heal    <state.json>  CharlieGuard-RESET → CLEAN, speichert atomar, gibt reset_von aus
//	vigil guard   <state.json>  Pre-Wake-Check: nur bei Inkonsistenz heal
//
// Exit-Codes: 0 OK, 1 inkonsistent, 2 Aufruf-/I/O-Fehler, 3 RESET fehlgeschlagen.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
)

const (
	exitOK           = 0
	exitInkonsistent = 1
	exitUsage        = 2
	exitResetFailed  = 3
)

var (
	ErrInkonsistent = errors.New("Zustand inkonsistent")
	ErrReset        = errors.New("RESET fehlgeschlagen, Zustand inkonsistent")
)

// ============================================================================
// 1. STATUS — GUELTIGE_STATUS
// ============================================================================

type Status string

const (
	StatusSleep   Status = "SLEEP"
	StatusWake    Status = "WAKE"
	StatusUpdated Status = "UPDATED"
	StatusClean   Status = "CLEAN"
)

func (s Status) Valid() bool {
	switch s {
	case StatusSleep, StatusWake, StatusUpdated, StatusClean:
		return true
	}
	return false
}

// ============================================================================
// 2. ZUSTAND — SharedState
// ============================================================================

// SharedState ist der gemeinsame Zustand des Loops.
// Ungültige Werte aus der Datei bleiben darstellbar, damit konsistent() sie
// erkennen und Heal() sie reparieren kann: Tick < 0 steht für "kein int ≥ 0",
// LastPulse NaN für "keine Zahl".
type SharedState struct {
	Data      any     `json:"data"`
	Tick      int64   `json:"tick"`
	LastPulse float64 `json:"last_pulse"`
	Status    Status  `json:"status"`
}

func NewSharedState() SharedState {
	return SharedState{Status: StatusSleep}
}

func (s SharedState) tickOK() bool { return s.Tick >= 0 }

func (s SharedState) pulseOK() bool {
	return !math.IsNaN(s.LastPulse) && !math.IsInf(s.LastPulse, 0)
}

// IsValid ist konsistent() aus loop.py: Feldtypen und -werte im erlaubten Bereich.
func (s SharedState) IsValid() bool { return s.Check() == nil }

// Check liefert jede verletzte Invariante einzeln.
func (s SharedState) Check() error {
	var errs []error
	if !s.tickOK() {
		errs = append(errs, fmt.Errorf("%w: tick muss int ≥ 0 sein", ErrInkonsistent))
	}
	if !s.pulseOK() {
		errs = append(errs, fmt.Errorf("%w: last_pulse muss eine Zahl sein", ErrInkonsistent))
	}
	if !s.Status.Valid() {
		const erlaubt = "{SLEEP, WAKE, UPDATED, CLEAN}"
		errs = append(errs, fmt.Errorf("%w: status %q nicht in %s", ErrInkonsistent, s.Status, erlaubt))
	}
	return errors.Join(errs...)
}

// ResetVon ist der Vorzustand, den CharlieGuard vor dem RESET festhält.
type ResetVon struct {
	Data   any    `json:"data"`
	Tick   int64  `json:"tick"`
	Status Status `json:"status"`
}

// Heal ist der validierte RESET (CharlieGuard.process): data leeren, ungültige
// Felder auf Nullwert, status CLEAN, danach Post-Heal-Validierung.
func (s *SharedState) Heal() (ResetVon, error) {
	alt := ResetVon{Data: s.Data, Tick: s.Tick, Status: s.Status}
	s.Data = nil
	if !s.tickOK() {
		s.Tick = 0
	}
	if !s.pulseOK() {
		s.LastPulse = 0
	}
	s.Status = StatusClean
	if !s.IsValid() || s.Data != nil {
		return alt, fmt.Errorf("%w (war: %+v)", ErrReset, alt)
	}
	return alt, nil
}

// Capture ist die Loop-Seite von capture_zustand (zti_runner.py).
type Capture struct {
	LoopTick       int64  `json:"loop_tick"`
	LoopStatus     Status `json:"loop_status"`
	LoopData       any    `json:"loop_data"`
	LoopKonsistent bool   `json:"loop_konsistent"`
}

func (s SharedState) Capture() Capture {
	return Capture{
		LoopTick:       s.Tick,
		LoopStatus:     s.Status,
		LoopData:       s.Data,
		LoopKonsistent: s.IsValid(),
	}
}

// ============================================================================
// 3. DEKODIERUNG — Typprüfung wie isinstance() in konsistent()
// ============================================================================

var stateKeys = map[string]bool{"data": true, "tick": true, "last_pulse": true, "status": true}

// DecodeState liest ein JSON-Objekt. Falsche Feldtypen sind kein Fehler, sondern
// werden zu ungültigen Werten, die Check() meldet und Heal() repariert.
// Fehler gibt es nur für kaputtes JSON, Nicht-Objekte und unbekannte Felder.
func DecodeState(b []byte) (SharedState, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return SharedState{}, fmt.Errorf("kein JSON-Objekt: %w", err)
	}
	if raw == nil {
		return SharedState{}, errors.New("kein JSON-Objekt: null")
	}
	if dec.More() {
		return SharedState{}, errors.New("Daten nach JSON-Objekt")
	}
	for k := range raw {
		if !stateKeys[k] {
			return SharedState{}, fmt.Errorf("unbekanntes Feld %q", k)
		}
	}

	s := SharedState{Tick: -1, LastPulse: math.NaN()}

	if v, ok := raw["data"]; ok {
		d := json.NewDecoder(bytes.NewReader(v))
		d.UseNumber()
		if err := d.Decode(&s.Data); err != nil {
			return SharedState{}, fmt.Errorf("data: %w", err)
		}
	}
	if n, ok := number(raw["tick"]); ok {
		if i, err := strconv.ParseInt(n.String(), 10, 64); err == nil {
			s.Tick = i
		}
	}
	if n, ok := number(raw["last_pulse"]); ok {
		if f, err := n.Float64(); err == nil {
			s.LastPulse = f
		}
	}
	var st string
	if err := json.Unmarshal(raw["status"], &st); err == nil {
		s.Status = Status(st)
	}
	return s, nil
}

// number akzeptiert nur JSON-Zahlen: kein bool, kein String, kein null.
func number(v json.RawMessage) (json.Number, bool) {
	t := bytes.TrimSpace(v)
	if len(t) == 0 || !(t[0] == '-' || (t[0] >= '0' && t[0] <= '9')) {
		return "", false
	}
	var n json.Number
	if err := json.Unmarshal(t, &n); err != nil {
		return "", false
	}
	return n, true
}

// ============================================================================
// 4. I/O — lesen, atomar schreiben
// ============================================================================

func LoadState(path string) (SharedState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return SharedState{}, fmt.Errorf("Zustand nicht lesbar: %w", err)
	}
	s, err := DecodeState(b)
	if err != nil {
		return SharedState{}, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// SaveState schreibt nur konsistente Zustände: .tmp → fsync → rename.
func SaveState(path string, s SharedState) error {
	if err := s.Check(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
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
	if err := os.Rename(tmp.Name(), path); err != nil {
		return err
	}
	committed = true
	return nil
}

// ============================================================================
// 5. CLI
// ============================================================================

const usage = `Aufruf: vigil <init|check|capture|heal|guard> <state.json>`

var commands = map[string]bool{"check": true, "capture": true, "heal": true, "guard": true}

func run(args []string, out, errOut io.Writer) int {
	if len(args) != 2 {
		fmt.Fprintln(errOut, usage)
		return exitUsage
	}
	cmd, path := args[0], args[1]
	name := filepath.Base(path)

	if cmd == "init" {
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(errOut, "FEHLER: %s existiert bereits\n", path)
			return exitUsage
		}
		if err := SaveState(path, NewSharedState()); err != nil {
			fmt.Fprintf(errOut, "FEHLER: %v\n", err)
			return exitUsage
		}
		fmt.Fprintf(out, "INIT     %s tick=0 status=SLEEP\n", name)
		return exitOK
	}

	if !commands[cmd] {
		fmt.Fprintln(errOut, usage)
		return exitUsage
	}
	s, err := LoadState(path)
	if err != nil {
		fmt.Fprintf(errOut, "FEHLER: %v\n", err)
		return exitUsage
	}

	switch cmd {
	case "check":
		if err := s.Check(); err != nil {
			fmt.Fprintf(out, "FAIL     %s\n%v\n", name, err)
			return exitInkonsistent
		}
		fmt.Fprintf(out, "OK       %s tick=%d status=%s\n", name, s.Tick, s.Status)
		return exitOK

	case "capture":
		b, _ := json.Marshal(s.Capture())
		fmt.Fprintf(out, "%s\n", b)
		if !s.IsValid() {
			return exitInkonsistent
		}
		return exitOK

	case "guard":
		if s.IsValid() {
			fmt.Fprintf(out, "OK       %s konsistent, kein RESET\n", name)
			return exitOK
		}
		fmt.Fprintf(out, "PRE-WAKE %s inkonsistent → RESET\n", name)
	}
	return heal(path, s, out, errOut)
}

// heal führt den CharlieGuard-RESET aus und speichert das Ergebnis.
func heal(path string, s SharedState, out, errOut io.Writer) int {
	alt, err := s.Heal()
	if err != nil {
		fmt.Fprintf(errOut, "FEHLER: %v\n", err)
		return exitResetFailed
	}
	if err := SaveState(path, s); err != nil {
		fmt.Fprintf(errOut, "FEHLER: %v\n", err)
		return exitUsage
	}
	b, _ := json.Marshal(map[string]ResetVon{"reset_von": alt})
	fmt.Fprintf(out, "RESET    %s → CLEAN tick=%d %s\n", filepath.Base(path), s.Tick, b)
	return exitOK
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
