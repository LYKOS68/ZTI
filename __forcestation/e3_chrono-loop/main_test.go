package main

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func val(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
}

func TestWorker(t *testing.T) {
	s := NewSharedState()
	zuLang := strings.Repeat("a", maxLen+1)
	tests := []struct {
		name    string
		w       Worker
		in      *string
		want    string
		wantBit Bit
	}{
		{"alpha nil → PING", alpha, nil, "PING", 1},
		{"alpha wert", alpha, ptr(" x "), " x ", 1},
		{"alpha leer bleibt leer", alpha, ptr(""), "", 1},
		{"alpha kaputtes utf8", alpha, ptr("\xff"), "\xff", 0},
		{"bravo trimmt", bravo, ptr("  x \n"), "x", 1},
		{"bravo nil läuft durch", bravo, nil, "<nil>", 1},
		{"charlie gültig", charlie, ptr(" VALID "), " VALID ", 1},
		{"charlie nil", charlie, nil, "<nil>", 0},
		{"charlie leer", charlie, ptr("  "), "  ", 0},
		{"charlie blockliste", charlie, ptr(" INVALID "), " INVALID ", 0},
		{"charlie blockliste case", charlie, ptr("invalid"), "invalid", 1},
		{"charlie max", charlie, ptr(strings.Repeat("ä", maxLen)), strings.Repeat("ä", maxLen), 1},
		{"charlie zu lang", charlie, ptr(zuLang), zuLang, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, bit := tt.w(tt.in, s)
			if val(got) != tt.want || bit != tt.wantBit {
				t.Fatalf("= %.20q, %d; want %.20q, %d", val(got), bit, tt.want, tt.wantBit)
			}
		})
	}
}

func TestSynthesize(t *testing.T) {
	ok := NewSharedState()
	tests := []struct {
		name string
		p    *string
		bits []Bit
		s    SharedState
		want Bit
	}{
		{"alles 1", ptr("x"), []Bit{1, 1, 1}, ok, 1},
		{"ein worker 0", ptr("x"), []Bit{1, 0, 1}, ok, 0},
		{"nicht alle gelaufen", ptr("x"), []Bit{1, 1}, ok, 0},
		{"kein kandidat", nil, []Bit{1, 1, 1}, ok, 0},
		{"zustand gedriftet", ptr("x"), []Bit{1, 1, 1}, SharedState{Status: "BOOM"}, 0},
	}
	for _, tt := range tests {
		if got := synthesize(tt.p, tt.bits, tt.s); got != tt.want {
			t.Errorf("%s: synthesize = %d; want %d", tt.name, got, tt.want)
		}
	}
}

func TestReihenfolgen(t *testing.T) {
	tests := []struct {
		order, input string // input "" = keine Eingabe
		want         string
		wantBits     []Bit
	}{
		{"ABC", "  x ", `EXECUTE (BIT=1) -> State: "x"`, []Bit{1, 1, 1, 1}},
		{"BCA", "  x ", `EXECUTE (BIT=1) -> State: "x"`, []Bit{1, 1, 1, 1}},
		{"CBA", "  x ", `EXECUTE (BIT=1) -> State: "x"`, []Bit{1, 1, 1, 1}},
		{"ABC", "", `EXECUTE (BIT=1) -> State: "PING"`, []Bit{1, 1, 1, 1}},
		// BRAVO lässt nil durch, CHARLIE verwirft:
		{"BCA", "", "RESET (BIT=0, DROP bei CHARLIE)", []Bit{1, 0, 0}},
		{"CBA", "", "RESET (BIT=0, DROP bei CHARLIE)", []Bit{0, 0}},
		{"ABC", "INVALID", "RESET (BIT=0, DROP bei CHARLIE)", []Bit{1, 1, 0, 0}},
		{"CBA", "\xff", "RESET (BIT=0, DROP bei ALPHA)", []Bit{1, 1, 0, 0}},
	}
	for _, tt := range tests {
		t.Run(tt.order+"/"+tt.input, func(t *testing.T) {
			l := NewChronoLoop(reihenfolgen[tt.order], 1, func() float64 { return 0 })
			var raw *string
			if tt.input != "" {
				raw = ptr(tt.input)
			}
			res, err := l.Zyklus(raw)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(res, tt.want) {
				t.Fatalf("Zyklus = %q; want …%q", res, tt.want)
			}
			v := l.State.History[len(l.State.History)-1].(Verlauf)
			if !reflect.DeepEqual(v.Bits, tt.wantBits) || v.Reihenfolge != tt.order+"D" {
				t.Fatalf("history = %+v; want bits %v", v, tt.wantBits)
			}
		})
	}
}

func TestApplySetztHashResetLeert(t *testing.T) {
	l := NewChronoLoop(reihenfolgen["ABC"], 1, func() float64 { return 0 })
	if _, err := l.Zyklus(ptr("abc")); err != nil {
		t.Fatal(err)
	}
	const sha = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	s := l.State
	if s.Status != StatusUpdated || s.Hash == nil || *s.Hash != sha || s.Data != "abc" {
		t.Fatalf("nach APPLY: %+v", l.State)
	}
	if _, err := l.Zyklus(ptr("CORRUPT")); err != nil {
		t.Fatal(err)
	}
	s = l.State
	if s.Status != StatusClean || s.Hash != nil || s.Data != nil || s.Tick != 2 {
		t.Fatalf("nach RESET: %+v", l.State)
	}
	if len(l.State.History) != 2 {
		t.Fatalf("history = %v; want 2 Einträge", l.State.History)
	}
}

func TestLatency(t *testing.T) {
	uhr := []float64{10.0, 10.25}
	i := 0
	l := NewChronoLoop(reihenfolgen["ABC"], 1, func() float64 { v := uhr[i]; i++; return v })
	if _, err := l.Zyklus(ptr("x")); err != nil {
		t.Fatal(err)
	}
	if l.State.LatencyMs != 250 {
		t.Fatalf("latency_ms = %v; want 250", l.State.LatencyMs)
	}
}

func TestTrigger(t *testing.T) {
	now := 0.0
	l := NewChronoLoop(reihenfolgen["ABC"], 0.5, func() float64 { return now })
	tests := []struct {
		now     float64
		hasData bool
		want    bool
	}{
		{0.0, false, false},
		{0.4, false, false},
		{0.5, false, true},
		{0.7, true, true},
		{0.8, false, false},
	}
	for _, tt := range tests {
		now = tt.now
		if got := l.Trigger(tt.hasData); got != tt.want {
			t.Fatalf("t=%.1f: Trigger = %v; want %v", tt.now, got, tt.want)
		}
	}
}

func TestPreWakeReset(t *testing.T) {
	l := NewChronoLoop(reihenfolgen["ABC"], 1, func() float64 { return 0 })
	l.State = SharedState{Data: "alt", Hash: ptr("h"), Tick: -1, badHistory: true, Status: "UPDATED"}
	res, err := l.Zyklus(ptr("X"))
	if err != nil {
		t.Fatal(err)
	}
	if res != "TICK 0: PRE-WAKE RESET (Zustand war inkonsistent)" {
		t.Fatalf("Zyklus = %q", res)
	}
	if !reflect.DeepEqual(l.State, SharedState{History: []any{}, Status: StatusClean}) {
		t.Fatalf("Zustand = %+v", l.State)
	}
}

// TestDemo hält den festen Lauf (wie loop.py __main__) exakt fest.
func TestDemo(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := run([]string{"demo"}, nil, nil, &out, &errOut); code != exitOK {
		t.Fatalf("code %d err %q", code, errOut.String())
	}
	want := `[Step 1] t=0.0 Status: SLEEP   | SLEEP
[Step 2] t=0.3 Status: SLEEP   | SLEEP
[Step 3] t=0.6 Status: UPDATED | TICK 1: EXECUTE (BIT=1) -> State: "VALID_DATA"
[Step 4] t=0.9 Status: SLEEP   | SLEEP
[Step 5] t=1.2 Status: CLEAN   | TICK 2: RESET (BIT=0, DROP bei CHARLIE)
[Step 6] t=1.5 Status: SLEEP   | SLEEP
`
	if out.String() != want {
		t.Fatalf("\n got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestRunStdin(t *testing.T) {
	var out, errOut bytes.Buffer
	args := []string{"run", "-order", "CBA", "-interval", "10", "-dt", "1"}
	stdin := strings.NewReader("hallo\n\n CORRUPT\n")
	if code := run(args, stdin, nil, &out, &errOut); code != exitOK {
		t.Fatalf("code %d err %q", code, errOut.String())
	}
	want := `[Step 1] t=0.0 Status: UPDATED | TICK 1: EXECUTE (BIT=1) -> State: "hallo"
[Step 2] t=1.0 Status: SLEEP   | SLEEP
[Step 3] t=2.0 Status: CLEAN   | TICK 2: RESET (BIT=0, DROP bei CHARLIE)
`
	if out.String() != want {
		t.Fatalf("\n got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func runCLI(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(""), func() float64 { return 0 }, &out, &errOut)
	return code, out.String() + errOut.String()
}

func TestStepDatei(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	steps := []struct {
		args []string
		want string
	}{
		{[]string{"step", path, " VALID_DATA "}, `TICK 1: EXECUTE (BIT=1) -> State: "VALID_DATA"`},
		// step ist immer WAKE, ohne Eingabe also PING:
		{[]string{"step", path}, `UPDATED | TICK 2: EXECUTE (BIT=1) -> State: "PING"`},
		{[]string{"step", "-order", "BCA", path}, "CLEAN   | TICK 3: RESET (BIT=0, DROP bei CHARLIE)"},
	}
	for _, st := range steps {
		code, out := runCLI(t, st.args...)
		if code != exitOK || !strings.Contains(out, st.want) {
			t.Fatalf("run(%v) = %d %q; want %q", st.args, code, out, st.want)
		}
	}

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil || len(m) != 6 {
		t.Fatalf("Datei hat nicht die 6 ABCD-Felder: %s", b)
	}
	var hist []Verlauf
	if err := json.Unmarshal(m["history"], &hist); err != nil || len(hist) != 3 {
		t.Fatalf("history = %s", m["history"])
	}
	if !reflect.DeepEqual(hist[2].Bits, []Bit{1, 0, 0}) || hist[2].Reihenfolge != "BCAD" {
		t.Fatalf("history[2] = %+v", hist[2])
	}
	var kompakt bytes.Buffer
	if err := json.Compact(&kompakt, m["history"]); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(kompakt.String(), `"bits":[1,0,0]`) {
		t.Fatalf("bits müssen als Zahlenliste erscheinen: %s", kompakt.String())
	}
}

func TestStepInkonsistenteDatei(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	body := `{"data":"alt","hash":1,"latency_ms":true,"history":"x","status":"UPDATED","tick":"7"}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out := runCLI(t, "step", path, "X")
	if code != exitOK || !strings.Contains(out, "CLEAN   | TICK 0: PRE-WAKE RESET") {
		t.Fatalf("code %d out %q", code, out)
	}
	s, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, SharedState{History: []any{}, Status: StatusClean}) {
		t.Fatalf("Datei = %+v", s)
	}
}

func TestSchemaWieABCD(t *testing.T) {
	b, _ := json.Marshal(NewSharedState())
	const want = `{"data":null,"hash":null,"latency_ms":0,"history":[],"status":"SLEEP","tick":0}`
	if string(b) != want {
		t.Fatalf("Schema = %s; want %s", b, want)
	}
}

func TestDecodeState(t *testing.T) {
	body := `{"data":null,"hash":2,"latency_ms":"0","history":{},"status":2,"tick":1.5}`
	s, err := DecodeState([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if s.Tick != -1 || !math.IsNaN(s.LatencyMs) || s.Status != "" || s.IsValid() {
		t.Fatalf("got %+v", s)
	}
	if !s.badHash || !s.badHistory {
		t.Fatalf("got %+v", s)
	}
	for _, body := range []string{``, `null`, `[]`, `{} {}`, `{"x":1}`, `{"last_pulse":0}`} {
		if _, err := DecodeState([]byte(body)); err == nil {
			t.Errorf("DecodeState(%q) = nil; want Fehler", body)
		}
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
		{"run", "-order", "ACB"}, // keine valid_permutation
		{"run", "-dt", "abc"},
		{"step"},
		{"step", "-order", "ABCD", filepath.Join(dir, "s.json")},
		{"step", "a", "b", "c"},
		{"step", kaputt},
		{"step", filepath.Join(dir, "fehlt", "s.json"), "X"},
	} {
		if code, out := runCLI(t, args...); code != exitUsage {
			t.Errorf("run(%v) = %d %q; want %d", args, code, out, exitUsage)
		}
	}
}
