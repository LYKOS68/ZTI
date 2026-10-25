// Chrono-Loop: Taktungs-Motor für den 4-Agenten-Zyklus.
//
// Go-Portierung von loop.py: SharedState, AlphaGenerator, BravoValidator,
// DeltaExecutor, CharlieGuard und ChronoLoop (trigger + step mit Pre-Wake-Check).
// Harte Kette: WAKE → Pre-Wake-Check → Z → T → I → S/Gate → neuer Zustand.
//
//	chronoloop demo
//	    fester Lauf wie loop.py __main__ (Intervall 0.5 s, Δt 0.3 s)
//	chronoloop run [-interval S] [-dt S]
//	    eine Eingabe pro stdin-Zeile, leere Zeile = keine Eingabe; simulierte Uhr
//	chronoloop step [-interval S] [-now S] <state.json> [eingabe]
//	    ein Schritt auf einer Zustandsdatei (fehlt sie: neuer Zustand), speichert atomar
//
// Die Zustandsdatei hat dasselbe Format wie bei Vigil-State (data, tick,
// last_pulse, status). Exit-Codes: 0 OK, 2 Aufruf-/I/O-Fehler, 3 RESET fehlgeschlagen.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	exitOK          = 0
	exitUsage       = 2
	exitResetFailed = 3
)

var ErrReset = errors.New("RESET fehlgeschlagen, Zustand inkonsistent")

// ============================================================================
// 1. ZUSTAND — SharedState
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

// SharedState: Tick < 0 steht für "kein int ≥ 0", LastPulse NaN für "keine Zahl".
type SharedState struct {
	Data      any     `json:"data"`
	Tick      int64   `json:"tick"`
	LastPulse float64 `json:"last_pulse"`
	Status    Status  `json:"status"`
}

func NewSharedState() SharedState { return SharedState{Status: StatusSleep} }

func (s SharedState) tickOK() bool { return s.Tick >= 0 }

func (s SharedState) pulseOK() bool {
	return !math.IsNaN(s.LastPulse) && !math.IsInf(s.LastPulse, 0)
}

// IsValid ist konsistent(): Feldtypen und -werte im erlaubten Bereich.
func (s SharedState) IsValid() bool { return s.tickOK() && s.pulseOK() && s.Status.Valid() }

// ============================================================================
// 2. TRANSFORMATION — AlphaGenerator
// ============================================================================

type AlphaGenerator struct{}

// Process: rohe Eingabe → Payload. Keine Eingabe (nil) wird zu "PING".
func (AlphaGenerator) Process(raw *string) string {
	if raw == nil {
		return "PING"
	}
	return *raw
}

// ============================================================================
// 3. INFORMATION — BravoValidator
// ============================================================================

type Bit uint8

const maxLen = 4096

var blockliste = map[string]bool{"INVALID": true, "CORRUPT": true}

type BravoValidator struct{}

// Process: Payload → genau ein Bit. 1 nur bei getrimmt nicht-leer,
// nicht in der Blockliste und höchstens maxLen Zeichen.
func (BravoValidator) Process(payload string) Bit {
	p := strings.TrimSpace(payload)
	if p == "" || blockliste[p] || utf8.RuneCountInString(p) > maxLen {
		return 0
	}
	return 1
}

// ============================================================================
// 4. SYNCPUNKT — DeltaExecutor (Bindung) und CharlieGuard (Rückkopplung)
// ============================================================================

type DeltaExecutor struct{}

func (DeltaExecutor) Process(payload string, s *SharedState) {
	s.Data = payload
	s.Status = StatusUpdated
}

type ResetVon struct {
	Data   any    `json:"data"`
	Tick   int64  `json:"tick"`
	Status Status `json:"status"`
}

type CharlieGuard struct{}

// Process ist der validierte RESET mit Post-Heal-Check.
func (CharlieGuard) Process(s *SharedState) (ResetVon, error) {
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

// ============================================================================
// 5. LOOP — ChronoLoop
// ============================================================================

type ChronoLoop struct {
	State    SharedState
	Interval float64        // Sekunden
	Now      func() float64 // injizierbare Uhr, Sekunden

	alpha   AlphaGenerator
	bravo   BravoValidator
	charlie CharlieGuard
	delta   DeltaExecutor
}

func NewChronoLoop(interval float64, now func() float64) *ChronoLoop {
	return &ChronoLoop{State: NewSharedState(), Interval: interval, Now: now}
}

// Trigger weckt bei abgelaufenem Intervall oder anliegender Eingabe.
func (l *ChronoLoop) Trigger(hasIncomingData bool) bool {
	now := l.Now()
	if now-l.State.LastPulse >= l.Interval || hasIncomingData {
		l.State.Status = StatusWake
		l.State.LastPulse = now
		return true
	}
	l.State.Status = StatusSleep
	return false
}

// Step führt einen Takt aus. raw == nil heißt: keine Eingabe.
func (l *ChronoLoop) Step(raw *string) (string, error) {
	if !l.Trigger(raw != nil) {
		return "SLEEP", nil
	}

	// Pre-Wake-Check: erst handeln, wenn der Zustand konsistent ist.
	if !l.State.IsValid() {
		if _, err := l.charlie.Process(&l.State); err != nil {
			return "", err
		}
		return fmt.Sprintf("TICK %d: PRE-WAKE RESET (Zustand war inkonsistent)", l.State.Tick), nil
	}

	l.State.Tick++

	payload := l.alpha.Process(raw) // TRANSFORMATION
	bit := l.bravo.Process(payload) // INFORMATION → genau ein Bit

	if bit == 0 { // S: validierende Rückkopplung
		if _, err := l.charlie.Process(&l.State); err != nil {
			return "", err
		}
		return fmt.Sprintf("TICK %d: RESET (BIT=0)", l.State.Tick), nil
	}

	l.delta.Process(payload, &l.State) // S: erfolgreiche Bindung
	return fmt.Sprintf("TICK %d: EXECUTE (BIT=1) -> State: %q", l.State.Tick, payload), nil
}

// ============================================================================
// 6. ZUSTANDSDATEI — lesen (typgeprüft), atomar schreiben
// ============================================================================

var stateKeys = map[string]bool{"data": true, "tick": true, "last_pulse": true, "status": true}

// DecodeState: falsche Feldtypen werden zu ungültigen Werten, die der
// Pre-Wake-Check erkennt. Fehler nur bei kaputtem JSON oder unbekannten Feldern.
func DecodeState(b []byte) (SharedState, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return SharedState{}, fmt.Errorf("kein JSON-Objekt: %w", err)
	}
	if raw == nil || dec.More() {
		return SharedState{}, errors.New("kein einzelnes JSON-Objekt")
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
	if t := bytes.TrimSpace(raw["status"]); len(t) > 0 && t[0] == '"' {
		var st string
		if json.Unmarshal(t, &st) == nil {
			s.Status = Status(st)
		}
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

// LoadState liefert für eine fehlende Datei einen neuen Zustand.
func LoadState(path string) (SharedState, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return NewSharedState(), nil
	}
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
	if !s.IsValid() {
		return errors.New("inkonsistenter Zustand wird nicht gespeichert")
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
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

	if _, err := tmp.Write(append(b, '\n')); err != nil {
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
// 7. CLI
// ============================================================================

const usage = `Aufruf:
  chronoloop demo
  chronoloop run [-interval S] [-dt S]
  chronoloop step [-interval S] [-now S] <state.json> [eingabe]`

func run(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, usage)
		return exitUsage
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(errOut)
	interval := fs.Float64("interval", 1.0, "Takt-Intervall in Sekunden")

	switch args[0] {
	case "demo":
		if len(args) != 1 {
			break
		}
		inputs := []*string{nil, nil, ptr("VALID_DATA"), nil, ptr("INVALID"), nil}
		return simulate(0.5, 0.3, inputs, out, errOut)

	case "run":
		dt := fs.Float64("dt", 1.0, "simulierter Zeitschritt pro Eingabe in Sekunden")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
			break
		}
		inputs, err := readInputs(in)
		if err != nil {
			fmt.Fprintf(errOut, "FEHLER: %v\n", err)
			return exitUsage
		}
		return simulate(*interval, *dt, inputs, out, errOut)

	case "step":
		now := fs.Float64("now", float64(time.Now().UnixNano())/1e9, "Uhrzeit in Sekunden")
		if fs.Parse(args[1:]) != nil || fs.NArg() < 1 || fs.NArg() > 2 {
			break
		}
		var raw *string
		if fs.NArg() == 2 {
			raw = ptr(fs.Arg(1))
		}
		return step(fs.Arg(0), *interval, *now, raw, out, errOut)
	}
	fmt.Fprintln(errOut, usage)
	return exitUsage
}

func ptr(s string) *string { return &s }

// readInputs: eine Eingabe pro Zeile, leere Zeile = keine Eingabe (nil).
func readInputs(in io.Reader) ([]*string, error) {
	var inputs []*string
	sc := bufio.NewScanner(in)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		if sc.Text() == "" {
			inputs = append(inputs, nil)
			continue
		}
		inputs = append(inputs, ptr(sc.Text()))
	}
	return inputs, sc.Err()
}

// simulate läuft mit kontrollierter Uhr: t beginnt bei 0 und wächst je Schritt um dt.
func simulate(interval, dt float64, inputs []*string, out, errOut io.Writer) int {
	t := 0.0
	loop := NewChronoLoop(interval, func() float64 { return t })
	for i, raw := range inputs {
		res, err := loop.Step(raw)
		if err != nil {
			fmt.Fprintf(errOut, "FEHLER: %v\n", err)
			return exitResetFailed
		}
		fmt.Fprintf(out, "[Step %d] t=%.1f Status: %-7s | %s\n", i+1, t, loop.State.Status, res)
		t += dt
	}
	return exitOK
}

func step(path string, interval, now float64, raw *string, out, errOut io.Writer) int {
	s, err := LoadState(path)
	if err != nil {
		fmt.Fprintf(errOut, "FEHLER: %v\n", err)
		return exitUsage
	}
	loop := NewChronoLoop(interval, func() float64 { return now })
	loop.State = s

	res, err := loop.Step(raw)
	if err != nil {
		fmt.Fprintf(errOut, "FEHLER: %v\n", err)
		return exitResetFailed
	}
	// Nur SLEEP lässt einen inkonsistenten Zustand stehen (kein Pre-Wake-Check
	// ohne WAKE). Dann bleibt die Datei unverändert bis zum nächsten WAKE.
	if !loop.State.IsValid() {
		fmt.Fprintf(out, "Status: %-7s | %s (Zustand inkonsistent, Datei unverändert)\n",
			loop.State.Status, res)
		return exitOK
	}
	if err := SaveState(path, loop.State); err != nil {
		fmt.Fprintf(errOut, "FEHLER: %v\n", err)
		return exitUsage
	}
	fmt.Fprintf(out, "Status: %-7s | %s\n", loop.State.Status, res)
	return exitOK
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
