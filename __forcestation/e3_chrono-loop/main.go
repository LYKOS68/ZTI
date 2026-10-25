// Chrono-Loop: Taktungs-Motor für den 3+1-Agenten-Zyklus nach ABCD.yaml.
//
// Drei Worker übergeben in Kette, DELTA synthetisiert und entscheidet mit
// genau einem Bit (execution_loop). Jeder Worker liefert zusätzlich sein
// Übergangs-Bit; bei 0 folgt sofort der DROP ohne Fehlerfortpflanzung
// (ZTI.yaml, kette.regel). Zustand: shared_state.schema aus ABCD.yaml.
//
// Rollen nach KANONISCHES-LEXIKON.yaml (universelle Verben, innere Operation
// IDENT/GENERATE/TRANSFORM/VALIDATE) und zuordnung.anker:
//
//	ALPHA   Z  ERFASSEN:    nimmt die Eingabe auf; Bit 0 bei ungültigem UTF-8 (Wert korrupt)
//	BRAVO   T  UMFORMEN:    keine Eingabe → erzeugt "PING", sonst trimmt es Leerraum
//	CHARLIE I  ANWEISEN:    legt die Spezifikation fest, gegen die geprüft wird (das WIE):
//	                        nicht leer, nicht INVALID/CORRUPT, ≤ 4096 Zeichen
//	DELTA   S  ENTSCHEIDEN: prüft den Kandidaten gegen CHARLIEs Spezifikation; Bit 1 nur,
//	                        wenn alle drei Worker 1 lieferten, die Prüfung besteht und der
//	                        Zustand konsistent ist → APPLY (UPDATED, hash = sha256),
//	                        sonst RESET (CLEAN, ABCD.yaml step_5)
//
//	chronoloop demo
//	    fester Lauf wie loop.py __main__ (Intervall 0.5 s, Δt 0.3 s, Reihenfolge ABC)
//	chronoloop run [-order ABC|BCA|CBA] [-interval S] [-dt S]
//	    eine Eingabe pro stdin-Zeile, leere Zeile = keine Eingabe; simulierte Uhr
//	chronoloop step [-order ABC|BCA|CBA] <state.json> [eingabe]
//	    ein expliziter Takt (immer WAKE) auf einer Zustandsdatei, speichert atomar
//
// Exit-Codes: 0 OK, 2 Aufruf-/I/O-Fehler, 3 RESET fehlgeschlagen.
package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
// 1. ZUSTAND — shared_state.schema
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

// SharedState: Tick < 0 steht für "kein int ≥ 0", LatencyMs NaN für "keine Zahl".
type SharedState struct {
	Data      any     `json:"data"`
	Hash      *string `json:"hash"`
	LatencyMs float64 `json:"latency_ms"`
	History   []any   `json:"history"`
	Status    Status  `json:"status"`
	Tick      int64   `json:"tick"`

	badHash, badHistory bool // falscher Typ in der Datei
}

func (s SharedState) MarshalJSON() ([]byte, error) {
	type plain SharedState
	p := plain(s)
	if p.History == nil {
		p.History = []any{}
	}
	return json.Marshal(p)
}

func NewSharedState() SharedState { return SharedState{History: []any{}, Status: StatusSleep} }

func (s SharedState) tickOK() bool { return s.Tick >= 0 }

func (s SharedState) latencyOK() bool {
	return !math.IsNaN(s.LatencyMs) && !math.IsInf(s.LatencyMs, 0) && s.LatencyMs >= 0
}

// IsValid: tick ≥ 0, status gültig (ABCD-Invariante) plus Feldtypen.
func (s SharedState) IsValid() bool {
	return s.tickOK() && s.latencyOK() && !s.badHash && !s.badHistory && s.Status.Valid()
}

// ============================================================================
// 2. WORKER — ALPHA, BRAVO, CHARLIE: PROCESS(payload, state) → payload, Bit
// ============================================================================

// Bit ist int, nicht uint8: []uint8 würde encoding/json als Base64 schreiben.
type Bit int

type Layer string

const (
	Alpha   Layer = "ALPHA"
	Bravo   Layer = "BRAVO"
	Charlie Layer = "CHARLIE"
	Delta   Layer = "DELTA"
)

// Spezifikation ist CHARLIEs Anweisung, wie ein gültiger Kandidat aussieht.
type Spezifikation struct {
	MaxLen     int
	Blockliste map[string]bool
}

// Payload ist der Arbeitskandidat, den die Worker einander übergeben.
type Payload struct {
	Wert *string        // nil = keine Eingabe
	Spez *Spezifikation // nil, bis CHARLIE angewiesen hat
}

// Worker lesen den Zustand nur (Kopie); schreiben darf allein DELTA.
type Worker func(p Payload, s SharedState) (Payload, Bit)

var workers = map[Layer]Worker{Alpha: alpha, Bravo: bravo, Charlie: charlie}

// valid_permutations aus ABCD.yaml; DELTA steht immer am Ende.
var reihenfolgen = map[string][3]Layer{
	"ABC": {Alpha, Bravo, Charlie},
	"BCA": {Bravo, Charlie, Alpha},
	"CBA": {Charlie, Bravo, Alpha},
}

// alpha erfasst: keine Eingabe ist ein gültiger Befund, korruptes UTF-8 nicht.
func alpha(p Payload, _ SharedState) (Payload, Bit) {
	if p.Wert != nil && !utf8.ValidString(*p.Wert) {
		return p, 0
	}
	return p, 1
}

// bravo formt um: erzeugt ohne Eingabe "PING", sonst trimmt es.
func bravo(p Payload, _ SharedState) (Payload, Bit) {
	if p.Wert == nil {
		p.Wert = ptr("PING")
		return p, 1
	}
	p.Wert = ptr(strings.TrimSpace(*p.Wert))
	return p, 1
}

// standardSpez ist die Anweisung, die CHARLIE erteilt.
var standardSpez = Spezifikation{
	MaxLen:     4096,
	Blockliste: map[string]bool{"INVALID": true, "CORRUPT": true},
}

// charlie weist an: legt fest, wie geprüft wird; prüft selbst nicht.
func charlie(p Payload, _ SharedState) (Payload, Bit) {
	p.Spez = &standardSpez
	return p, 1
}

// erfuellt prüft einen Wert gegen eine Spezifikation.
func (sp Spezifikation) erfuellt(wert string) bool {
	v := strings.TrimSpace(wert)
	return v != "" && !sp.Blockliste[v] && utf8.RuneCountInString(v) <= sp.MaxLen
}

// ============================================================================
// 3. SYNTHESIZER — DELTA: SYNTHESIZE → Bit, APPLY, RESET
// ============================================================================

// synthesize entscheidet: Bit 1 nur, wenn alle drei Worker gelaufen sind und
// 1 lieferten, ein Kandidat und CHARLIEs Spezifikation vorliegen, der Kandidat
// sie erfüllt und der Zustand konsistent ist (kein Drift).
func synthesize(p Payload, bits []Bit, s SharedState) Bit {
	if p.Wert == nil || p.Spez == nil || len(bits) != 3 || !s.IsValid() {
		return 0
	}
	for _, b := range bits {
		if b != 1 {
			return 0
		}
	}
	if !p.Spez.erfuellt(*p.Wert) {
		return 0
	}
	return 1
}

func apply(p string, s *SharedState) {
	sum := sha256.Sum256([]byte(p))
	h := hex.EncodeToString(sum[:])
	s.Data, s.Hash, s.Status = p, &h, StatusUpdated
}

// reset ist der validierte RESET mit Post-Heal-Check.
func reset(s *SharedState) error {
	s.Data, s.Hash, s.badHash = nil, nil, false
	if !s.tickOK() {
		s.Tick = 0
	}
	if !s.latencyOK() {
		s.LatencyMs = 0
	}
	if s.badHistory || s.History == nil {
		s.History, s.badHistory = []any{}, false
	}
	s.Status = StatusClean
	if !s.IsValid() || s.Data != nil {
		return ErrReset
	}
	return nil
}

// ============================================================================
// 4. LOOP — Trigger + Zyklus
// ============================================================================

// Verlauf ist ein history-Eintrag pro Zyklus.
type Verlauf struct {
	Tick        int64  `json:"tick"`
	Reihenfolge string `json:"reihenfolge"`
	Bits        []Bit  `json:"bits"` // ALPHA/BRAVO/CHARLIE in Laufreihenfolge, dann DELTA
	Bit         Bit    `json:"bit"`
	Status      Status `json:"status"`
}

type ChronoLoop struct {
	State    SharedState
	Order    [3]Layer
	Interval float64        // Sekunden
	Now      func() float64 // injizierbare Uhr, Sekunden

	lastPulse float64 // Zeitpunkt des letzten WAKE; gehört dem Loop, nicht dem Zustand
}

func NewChronoLoop(order [3]Layer, interval float64, now func() float64) *ChronoLoop {
	return &ChronoLoop{State: NewSharedState(), Order: order, Interval: interval, Now: now}
}

// Trigger weckt bei abgelaufenem Intervall oder anliegender Eingabe.
func (l *ChronoLoop) Trigger(hasIncomingData bool) bool {
	now := l.Now()
	if now-l.lastPulse >= l.Interval || hasIncomingData {
		l.State.Status = StatusWake
		l.lastPulse = now
		return true
	}
	l.State.Status = StatusSleep
	return false
}

// Step = Trigger + Zyklus. raw == nil heißt: keine Eingabe.
func (l *ChronoLoop) Step(raw *string) (string, error) {
	if !l.Trigger(raw != nil) {
		return "SLEEP", nil
	}
	return l.Zyklus(raw)
}

// Zyklus läuft nach WAKE: Pre-Wake-Check → W1 → W2 → W3 → DELTA.
func (l *ChronoLoop) Zyklus(raw *string) (string, error) {
	start := l.Now()

	if !l.State.IsValid() {
		if err := reset(&l.State); err != nil {
			return "", err
		}
		return fmt.Sprintf("TICK %d: PRE-WAKE RESET (Zustand war inkonsistent)", l.State.Tick), nil
	}

	l.State.Tick++
	p, bits, dropBei := Payload{Wert: raw}, make([]Bit, 0, 4), Delta
	for _, w := range l.Order {
		var b Bit
		p, b = workers[w](p, l.State)
		bits = append(bits, b)
		if b == 0 {
			dropBei = w
			break
		}
	}
	bit := synthesize(p, bits, l.State)
	bits = append(bits, bit)

	var res string
	if bit == 1 {
		apply(*p.Wert, &l.State)
		res = fmt.Sprintf("TICK %d: EXECUTE (BIT=1) -> State: %q", l.State.Tick, *p.Wert)
	} else {
		if err := reset(&l.State); err != nil {
			return "", err
		}
		res = fmt.Sprintf("TICK %d: RESET (BIT=0, DROP bei %s)", l.State.Tick, dropBei)
	}

	l.State.LatencyMs = math.Max(0, (l.Now()-start)*1000)
	l.State.History = append(l.State.History, Verlauf{
		Tick:        l.State.Tick,
		Reihenfolge: l.orderName(),
		Bits:        bits,
		Bit:         bit,
		Status:      l.State.Status,
	})
	return res, nil
}

func (l *ChronoLoop) orderName() string {
	return string(l.Order[0][0]) + string(l.Order[1][0]) + string(l.Order[2][0]) + "D"
}

// ============================================================================
// 5. ZUSTANDSDATEI — lesen (typgeprüft), atomar schreiben
// ============================================================================

var stateKeys = map[string]bool{
	"data": true, "hash": true, "latency_ms": true, "history": true, "status": true, "tick": true,
}

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

	s := SharedState{Tick: -1, LatencyMs: math.NaN()}
	if v, ok := raw["data"]; ok {
		if err := decodeAny(v, &s.Data); err != nil {
			return SharedState{}, fmt.Errorf("data: %w", err)
		}
	}
	switch t := bytes.TrimSpace(raw["hash"]); {
	case bytes.Equal(t, []byte("null")):
	case len(t) > 0 && t[0] == '"':
		var h string
		s.badHash = json.Unmarshal(t, &h) != nil
		s.Hash = &h
	default:
		s.badHash = true
	}
	if n, ok := number(raw["latency_ms"]); ok {
		if f, err := n.Float64(); err == nil {
			s.LatencyMs = f
		}
	}
	if t := bytes.TrimSpace(raw["history"]); len(t) > 0 && t[0] == '[' {
		// Einträge als Roh-JSON: so bleibt ihre Schlüsselreihenfolge beim Zurückschreiben erhalten.
		var eintraege []json.RawMessage
		if err := json.Unmarshal(t, &eintraege); err != nil {
			return SharedState{}, fmt.Errorf("history: %w", err)
		}
		s.History = make([]any, len(eintraege))
		for i, e := range eintraege {
			s.History[i] = e
		}
	} else {
		s.badHistory = true
	}
	if t := bytes.TrimSpace(raw["status"]); len(t) > 0 && t[0] == '"' {
		var st string
		if json.Unmarshal(t, &st) == nil {
			s.Status = Status(st)
		}
	}
	if n, ok := number(raw["tick"]); ok {
		if i, err := strconv.ParseInt(n.String(), 10, 64); err == nil {
			s.Tick = i
		}
	}
	return s, nil
}

func decodeAny(v json.RawMessage, ziel any) error {
	d := json.NewDecoder(bytes.NewReader(v))
	d.UseNumber()
	return d.Decode(ziel)
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
// 6. CLI
// ============================================================================

const usage = `Aufruf:
  chronoloop demo
  chronoloop run [-order ABC|BCA|CBA] [-interval S] [-dt S]
  chronoloop step [-order ABC|BCA|CBA] <state.json> [eingabe]`

func run(args []string, in io.Reader, now func() float64, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, usage)
		return exitUsage
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(errOut)
	orderFlag := fs.String("order", "ABC", "Worker-Reihenfolge aus valid_permutations")

	switch args[0] {
	case "demo":
		if len(args) != 1 {
			break
		}
		inputs := []*string{nil, nil, ptr("VALID_DATA"), nil, ptr("INVALID"), nil}
		return simulate(reihenfolgen["ABC"], 0.5, 0.3, inputs, out, errOut)

	case "run":
		interval := fs.Float64("interval", 1.0, "Takt-Intervall in Sekunden")
		dt := fs.Float64("dt", 1.0, "simulierter Zeitschritt pro Eingabe in Sekunden")
		if fs.Parse(args[1:]) != nil || fs.NArg() != 0 {
			break
		}
		order, ok := reihenfolgen[*orderFlag]
		if !ok {
			break
		}
		inputs, err := readInputs(in)
		if err != nil {
			fmt.Fprintf(errOut, "FEHLER: %v\n", err)
			return exitUsage
		}
		return simulate(order, *interval, *dt, inputs, out, errOut)

	case "step":
		if fs.Parse(args[1:]) != nil || fs.NArg() < 1 || fs.NArg() > 2 {
			break
		}
		order, ok := reihenfolgen[*orderFlag]
		if !ok {
			break
		}
		var raw *string
		if fs.NArg() == 2 {
			raw = ptr(fs.Arg(1))
		}
		return step(fs.Arg(0), order, now, raw, out, errOut)
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
func simulate(order [3]Layer, interval, dt float64, inputs []*string, out, errOut io.Writer) int {
	t := 0.0
	loop := NewChronoLoop(order, interval, func() float64 { return t })
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

func step(path string, order [3]Layer, now func() float64, raw *string,
	out, errOut io.Writer) int {
	s, err := LoadState(path)
	if err != nil {
		fmt.Fprintf(errOut, "FEHLER: %v\n", err)
		return exitUsage
	}
	loop := NewChronoLoop(order, 0, now)
	loop.State = s
	loop.State.Status = StatusWake // expliziter Takt

	res, err := loop.Zyklus(raw)
	if err != nil {
		fmt.Fprintf(errOut, "FEHLER: %v\n", err)
		return exitResetFailed
	}
	if err := SaveState(path, loop.State); err != nil {
		fmt.Fprintf(errOut, "FEHLER: %v\n", err)
		return exitUsage
	}
	fmt.Fprintf(out, "Status: %-7s | %s\n", loop.State.Status, res)
	return exitOK
}

func wallClock() float64 { return float64(time.Now().UnixNano()) / 1e9 }

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, wallClock, os.Stdout, os.Stderr))
}
