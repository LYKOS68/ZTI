package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAlpha(t *testing.T) {
	var a AlphaGenerator
	if got := a.Process(nil); got != "PING" {
		t.Errorf("Process(nil) = %q; want PING", got)
	}
	if got := a.Process(ptr("")); got != "" {
		t.Errorf("Process(\"\") = %q; leere Eingabe ist nicht nil", got)
	}
	if got := a.Process(ptr("X")); got != "X" {
		t.Errorf("Process(X) = %q", got)
	}
}

func TestBravo(t *testing.T) {
	tests := []struct {
		payload string
		want    Bit
	}{
		{"VALID_DATA", 1},
		{"PING", 1},
		{"  x  ", 1},
		{"", 0},
		{" \t\n", 0},
		{"INVALID", 0},
		{" CORRUPT ", 0},
		{"invalid", 1}, // Blockliste ist case-sensitiv
		{strings.Repeat("a", maxLen), 1},
		{strings.Repeat("a", maxLen+1), 0},
		{strings.Repeat("ä", maxLen), 1}, // Zeichen, nicht Bytes
	}
	var b BravoValidator
	for _, tt := range tests {
		if got := b.Process(tt.payload); got != tt.want {
			name := tt.payload
			if len(name) > 20 {
				name = name[:20] + "…"
			}
			t.Errorf("Process(%q) = %d; want %d", name, got, tt.want)
		}
	}
}

func TestCharlie(t *testing.T) {
	s := SharedState{Data: "X", Tick: -1, LastPulse: math.NaN(), Status: "BOOM"}
	alt, err := CharlieGuard{}.Process(&s)
	if err != nil {
		t.Fatal(err)
	}
	if s != (SharedState{Status: StatusClean}) {
		t.Fatalf("nach RESET: %+v", s)
	}
	if alt != (ResetVon{Data: "X", Tick: -1, Status: "BOOM"}) {
		t.Fatalf("reset_von = %+v", alt)
	}
}

func TestTrigger(t *testing.T) {
	now := 0.0
	l := NewChronoLoop(0.5, func() float64 { return now })

	tests := []struct {
		now     float64
		hasData bool
		want    bool
		pulse   float64
	}{
		{0.0, false, false, 0.0}, // Intervall nicht erreicht
		{0.4, false, false, 0.0},
		{0.5, false, true, 0.5}, // Intervall erreicht
		{0.7, true, true, 0.7},  // Eingabe weckt immer
		{0.8, false, false, 0.7},
	}
	for _, tt := range tests {
		now = tt.now
		got := l.Trigger(tt.hasData)
		wantStatus := StatusSleep
		if tt.want {
			wantStatus = StatusWake
		}
		if got != tt.want || l.State.Status != wantStatus || l.State.LastPulse != tt.pulse {
			t.Fatalf("t=%.1f: Trigger = %v, %+v; want %v, pulse %.1f",
				tt.now, got, l.State, tt.want, tt.pulse)
		}
	}
}

func TestStepPreWakeReset(t *testing.T) {
	l := NewChronoLoop(1, func() float64 { return 10 })
	l.State = SharedState{Data: "alt", Tick: -1, Status: StatusUpdated}

	res, err := l.Step(ptr("X"))
	if err != nil {
		t.Fatal(err)
	}
	if res != "TICK 0: PRE-WAKE RESET (Zustand war inkonsistent)" {
		t.Fatalf("Step = %q", res)
	}
	if l.State != (SharedState{Tick: 0, LastPulse: 10, Status: StatusClean}) {
		t.Fatalf("Zustand = %+v", l.State)
	}
}

// TestDemo hält den Lauf aus loop.py __main__ exakt fest.
func TestDemo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"demo"}, nil, &out, &errOut); code != exitOK {
		t.Fatalf("code %d err %q", code, errOut.String())
	}
	want := `[Step 1] t=0.0 Status: SLEEP   | SLEEP
[Step 2] t=0.3 Status: SLEEP   | SLEEP
[Step 3] t=0.6 Status: UPDATED | TICK 1: EXECUTE (BIT=1) -> State: "VALID_DATA"
[Step 4] t=0.9 Status: SLEEP   | SLEEP
[Step 5] t=1.2 Status: CLEAN   | TICK 2: RESET (BIT=0)
[Step 6] t=1.5 Status: SLEEP   | SLEEP
`
	if out.String() != want {
		t.Fatalf("\n got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestRunStdin(t *testing.T) {
	var out, errOut bytes.Buffer
	stdin := strings.NewReader("hallo\n\nCORRUPT\n")
	code := run([]string{"run", "-interval", "10", "-dt", "1"}, stdin, &out, &errOut)
	if code != exitOK {
		t.Fatalf("code %d err %q", code, errOut.String())
	}
	want := `[Step 1] t=0.0 Status: UPDATED | TICK 1: EXECUTE (BIT=1) -> State: "hallo"
[Step 2] t=1.0 Status: SLEEP   | SLEEP
[Step 3] t=2.0 Status: CLEAN   | TICK 2: RESET (BIT=0)
`
	if out.String() != want {
		t.Fatalf("\n got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestRunIntervallWecktOhneEingabe(t *testing.T) {
	var out, errOut bytes.Buffer
	args := []string{"run", "-interval", "1", "-dt", "1"}
	if code := run(args, strings.NewReader("\n\n"), &out, &errOut); code != exitOK {
		t.Fatalf("code %d", code)
	}
	// t=0: 0-0 ≥ 1? nein → SLEEP. t=1: ja → WAKE, keine Eingabe → PING → EXECUTE.
	const want = `[Step 2] t=1.0 Status: UPDATED | TICK 1: EXECUTE (BIT=1) -> State: "PING"`
	if !strings.Contains(out.String(), want) {
		t.Fatalf("out = %q", out.String())
	}
}

func runCLI(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(""), &out, &errOut)
	return code, out.String() + errOut.String()
}

func TestStepDatei(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	steps := []struct {
		args []string
		want string
	}{
		{[]string{"step", "-now", "100", path, "VALID_DATA"}, `TICK 1: EXECUTE (BIT=1)`},
		{[]string{"step", "-now", "100.5", path}, "SLEEP   | SLEEP"},
		{[]string{"step", "-now", "102", path}, `UPDATED | TICK 2: EXECUTE (BIT=1) -> State: "PING"`},
		{[]string{"step", "-now", "102", path, "INVALID"}, "CLEAN   | TICK 3: RESET (BIT=0)"},
	}
	for _, st := range steps {
		code, out := runCLI(t, st.args...)
		if code != exitOK || !strings.Contains(out, st.want) {
			t.Fatalf("run(%v) = %d %q; want %q", st.args, code, out, st.want)
		}
	}
	s, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if s != (SharedState{Tick: 3, LastPulse: 102, Status: StatusClean}) {
		t.Fatalf("Datei = %+v", s)
	}
}

func TestStepInkonsistenteDatei(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	body := []byte(`{"data":"alt","tick":"7","last_pulse":true,"status":"BOOM"}`)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}

	// Ohne Eingabe und mit NaN-Puls kein WAKE → Datei bleibt unverändert.
	code, out := runCLI(t, "step", "-now", "5", path)
	if code != exitOK || !strings.Contains(out, "Datei unverändert") {
		t.Fatalf("code %d out %q", code, out)
	}
	if b, _ := os.ReadFile(path); !bytes.Equal(b, body) {
		t.Fatalf("Datei verändert: %s", b)
	}

	// Mit Eingabe → WAKE → Pre-Wake-Reset → gespeichert.
	code, out = runCLI(t, "step", "-now", "5", path, "X")
	if code != exitOK || !strings.Contains(out, "CLEAN   | TICK 0: PRE-WAKE RESET") {
		t.Fatalf("code %d out %q", code, out)
	}
	s, _ := LoadState(path)
	if s != (SharedState{Tick: 0, LastPulse: 5, Status: StatusClean}) {
		t.Fatalf("Datei = %+v", s)
	}
}

func TestCLIFehler(t *testing.T) {
	dir := t.TempDir()
	kaputt := filepath.Join(dir, "kaputt.json")
	if err := os.WriteFile(kaputt, []byte(`{"tick":0,"x":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		nil,
		{"nope"},
		{"demo", "x"},
		{"run", "extra"},
		{"run", "-dt", "abc"},
		{"step"},
		{"step", "a", "b", "c"},
		{"step", kaputt},
		{"step", "-now", "1", filepath.Join(dir, "fehlt", "s.json"), "X"},
	} {
		if code, out := runCLI(t, args...); code != exitUsage {
			t.Errorf("run(%v) = %d %q; want %d", args, code, out, exitUsage)
		}
	}
}

func TestDecodeStateTypen(t *testing.T) {
	s, err := DecodeState([]byte(`{"data":null,"tick":1.5,"last_pulse":"0","status":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if s.Tick != -1 || !math.IsNaN(s.LastPulse) || s.Status != "" || s.IsValid() {
		t.Fatalf("got %+v", s)
	}
	for _, body := range []string{``, `null`, `[]`, `{} {}`, `{"x":1}`} {
		if _, err := DecodeState([]byte(body)); err == nil {
			t.Errorf("DecodeState(%q) = nil; want Fehler", body)
		}
	}
}
