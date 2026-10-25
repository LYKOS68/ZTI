package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stateJSON baut ein Zustandsobjekt mit data=null aus rohen JSON-Werten.
func stateJSON(tick, pulse, status string) string {
	return fmt.Sprintf(`{"data":null,"tick":%s,"last_pulse":%s,"status":%s}`, tick, pulse, status)
}

func TestIsValid(t *testing.T) {
	tests := []struct {
		name string
		s    SharedState
		want bool
	}{
		{"neu", NewSharedState(), true},
		{"updated", SharedState{Tick: 3, LastPulse: 1.5, Status: StatusUpdated, Data: "X"}, true},
		{"clean", SharedState{Status: StatusClean}, true},
		{"wake", SharedState{Status: StatusWake}, true},
		{"nullwert status leer", SharedState{}, false},
		{"tick negativ", SharedState{Tick: -1, Status: StatusSleep}, false},
		{"pulse NaN", SharedState{LastPulse: math.NaN(), Status: StatusSleep}, false},
		{"pulse Inf", SharedState{LastPulse: math.Inf(1), Status: StatusSleep}, false},
		{"status unbekannt", SharedState{Status: "BOOM"}, false},
		{"status kleingeschrieben", SharedState{Status: "sleep"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.IsValid(); got != tt.want {
				t.Fatalf("IsValid() = %v; want %v (%v)", got, tt.want, tt.s.Check())
			}
		})
	}
}

func TestCheckMeldetJedeVerletzung(t *testing.T) {
	err := SharedState{Tick: -1, LastPulse: math.NaN(), Status: "X"}.Check()
	if !errors.Is(err, ErrInkonsistent) || strings.Count(err.Error(), "inkonsistent") != 3 {
		t.Fatalf("Check() = %v; want 3 Verletzungen", err)
	}
}

const zuGross = `99999999999999999999` // > int64

func TestDecodeStateTypen(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		tick     int64
		pulseNaN bool
		status   Status
		valid    bool
	}{
		{"gültig", stateJSON(`2`, `1.5`, `"WAKE"`), 2, false, StatusWake, true},
		{"pulse als int", stateJSON(`0`, `7`, `"SLEEP"`), 0, false, StatusSleep, true},
		{"tick als float", stateJSON(`1.5`, `0`, `"SLEEP"`), -1, false, StatusSleep, false},
		{"tick als exponent", stateJSON(`1e3`, `0`, `"SLEEP"`), -1, false, StatusSleep, false},
		{"tick als string", stateJSON(`"3"`, `0`, `"SLEEP"`), -1, false, StatusSleep, false},
		{"tick als bool", stateJSON(`true`, `0`, `"SLEEP"`), -1, false, StatusSleep, false},
		{"tick negativ", stateJSON(`-4`, `0`, `"SLEEP"`), -4, false, StatusSleep, false},
		{"tick überlauf", stateJSON(zuGross, `0`, `"SLEEP"`), -1, false, StatusSleep, false},
		{"pulse als bool", stateJSON(`0`, `false`, `"SLEEP"`), 0, true, StatusSleep, false},
		{"pulse als string", stateJSON(`0`, `"0.0"`, `"SLEEP"`), 0, true, StatusSleep, false},
		{"pulse null", stateJSON(`0`, `null`, `"SLEEP"`), 0, true, StatusSleep, false},
		{"status als zahl", stateJSON(`0`, `0`, `1`), 0, false, "", false},
		{"felder fehlen", `{}`, -1, true, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := DecodeState([]byte(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			if s.Tick != tt.tick || math.IsNaN(s.LastPulse) != tt.pulseNaN || s.Status != tt.status {
				t.Fatalf("got %+v", s)
			}
			if s.IsValid() != tt.valid {
				t.Fatalf("IsValid() = %v; want %v", s.IsValid(), tt.valid)
			}
		})
	}
}

func TestDecodeStateData(t *testing.T) {
	const data = `{"n":12345678901234567890}`
	body := strings.Replace(stateJSON(`0`, `0`, `"SLEEP"`), "null", data, 1)
	s, err := DecodeState([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(s.Data); string(b) != data {
		t.Fatalf("data verlustbehaftet: %s", b)
	}
}

func TestDecodeStateFehler(t *testing.T) {
	for name, body := range map[string]string{
		"unbekanntes feld": strings.Replace(stateJSON(`0`, `0`, `"SLEEP"`), "}", `,"x":1}`, 1),
		"array":            `[1,2]`,
		"null":             `null`,
		"müll danach":      `{"tick":0} {}`,
		"kein json":        `tick=0`,
		"leer":             ``,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeState([]byte(body)); err == nil {
				t.Fatal("DecodeState() = nil; want Fehler")
			}
		})
	}
}

func TestHeal(t *testing.T) {
	tests := []struct {
		name string
		in   SharedState
		want SharedState
	}{
		{
			name: "updated → clean, tick bleibt",
			in:   SharedState{Data: "VALID_DATA", Tick: 3, LastPulse: 0.6, Status: StatusUpdated},
			want: SharedState{Tick: 3, LastPulse: 0.6, Status: StatusClean},
		},
		{
			name: "tick kaputt → 0",
			in:   SharedState{Data: 1, Tick: -1, LastPulse: 2, Status: "BOOM"},
			want: SharedState{Tick: 0, LastPulse: 2, Status: StatusClean},
		},
		{
			name: "pulse kaputt → 0",
			in:   SharedState{Tick: 5, LastPulse: math.NaN(), Status: StatusWake},
			want: SharedState{Tick: 5, LastPulse: 0, Status: StatusClean},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.in
			alt, err := s.Heal()
			if err != nil {
				t.Fatal(err)
			}
			if s != tt.want {
				t.Fatalf("Heal() → %+v; want %+v", s, tt.want)
			}
			if alt.Data != tt.in.Data || alt.Tick != tt.in.Tick || alt.Status != tt.in.Status {
				t.Fatalf("reset_von = %+v; want Vorzustand %+v", alt, tt.in)
			}
		})
	}
}

func TestCapture(t *testing.T) {
	got := SharedState{Data: "X", Tick: 2, Status: StatusUpdated}.Capture()
	want := Capture{
		LoopTick:       int64(2),
		LoopStatus:     StatusUpdated,
		LoopData:       "X",
		LoopKonsistent: true,
	}
	if got != want {
		t.Fatalf("Capture() = %+v; want %+v", got, want)
	}
}

// TestRohwerte: falsch typisierte Dateiwerte erscheinen unverändert in
// reset_von und capture; nach Heal sind sie verschwunden.
func TestRohwerte(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		wantTick   string
		wantStatus string
	}{
		{"tick string", stateJSON(`"7"`, `0`, `"SLEEP"`), `"7"`, `"SLEEP"`},
		{"tick bool", stateJSON(`true`, `0`, `"SLEEP"`), `true`, `"SLEEP"`},
		{"tick float", stateJSON(`1.5`, `0`, `"SLEEP"`), `1.5`, `"SLEEP"`},
		{"tick überlauf", stateJSON(zuGross, `0`, `"SLEEP"`), zuGross, `"SLEEP"`},
		{"tick objekt", stateJSON(`{"a": 1}`, `0`, `"SLEEP"`), `{"a":1}`, `"SLEEP"`},
		{"tick negativ bleibt zahl", stateJSON(`-4`, `0`, `"SLEEP"`), `-4`, `"SLEEP"`},
		{"status zahl", stateJSON(`0`, `0`, `1`), `0`, `1`},
		{"status null", stateJSON(`0`, `0`, `null`), `0`, `null`},
		{"status unbekannt bleibt string", stateJSON(`0`, `0`, `"BOOM"`), `0`, `"BOOM"`},
		{"felder fehlen", `{}`, `null`, `null`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := DecodeState([]byte(tt.body))
			if err != nil {
				t.Fatal(err)
			}

			c, _ := json.Marshal(s.Capture())
			wantCapture := fmt.Sprintf(`"loop_tick":%s,"loop_status":%s,`, tt.wantTick, tt.wantStatus)
			if !strings.Contains(string(c), wantCapture) {
				t.Fatalf("capture = %s; want %s", c, wantCapture)
			}

			alt, err := s.Heal()
			if err != nil {
				t.Fatal(err)
			}
			r, _ := json.Marshal(alt)
			wantReset := fmt.Sprintf(`"tick":%s,"status":%s}`, tt.wantTick, tt.wantStatus)
			if !strings.HasSuffix(string(r), wantReset) {
				t.Fatalf("reset_von = %s; want …%s", r, wantReset)
			}

			if s.rawTick != "" || s.rawStatus != "" {
				t.Fatalf("nach Heal noch Rohwerte: %q, %q", s.rawTick, s.rawStatus)
			}
		})
	}
}

func TestSaveStateVerweigertInkonsistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := SaveState(path, SharedState{Status: "BOOM"}); !errors.Is(err, ErrInkonsistent) {
		t.Fatalf("SaveState() = %v; want ErrInkonsistent", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Datei darf nicht entstehen")
	}
}

func TestSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	want := SharedState{Data: "VALID_DATA", Tick: 4, LastPulse: 0.6, Status: StatusUpdated}
	if err := SaveState(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %+v; want %+v", got, want)
	}
	if m, _ := filepath.Glob(path + ".*.tmp"); len(m) != 0 {
		t.Fatalf("tmp-Dateien übrig: %v", m)
	}
}

func runCLI(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut)
	return code, out.String() + errOut.String()
}

func TestCLIAblauf(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	const (
		captureNeu = `{"loop_tick":0,"loop_status":"SLEEP","loop_data":null,"loop_konsistent":true}`
		resetNeu   = `{"reset_von":{"data":null,"tick":0,"status":"SLEEP"}}`
	)
	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"check", path}, exitUsage, "nicht lesbar"},
		{[]string{"init", path}, exitOK, "INIT"},
		{[]string{"init", path}, exitUsage, "existiert bereits"},
		{[]string{"check", path}, exitOK, "tick=0 status=SLEEP"},
		{[]string{"capture", path}, exitOK, captureNeu},
		{[]string{"guard", path}, exitOK, "kein RESET"},
		{[]string{"heal", path}, exitOK, resetNeu},
		{[]string{"check", path}, exitOK, "status=CLEAN"},
		{[]string{"eck", path}, exitUsage, "Aufruf"},
		{[]string{"check"}, exitUsage, "Aufruf"},
		{nil, exitUsage, "Aufruf"},
	}
	for _, st := range steps {
		code, out := runCLI(t, st.args...)
		if code != st.code || !strings.Contains(out, st.want) {
			t.Fatalf("run(%v) = %d %q; want %d %q", st.args, code, out, st.code, st.want)
		}
	}
}

func TestCLIGuardHeiltInkonsistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	body := `{"data":"VALID_DATA","tick":"7","last_pulse":true,"status":"BOOM"}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out := runCLI(t, "check", path)
	if code != exitInkonsistent || strings.Count(out, "inkonsistent") != 3 {
		t.Fatalf("check = %d %q", code, out)
	}
	code, out = runCLI(t, "capture", path)
	if code != exitInkonsistent || !strings.Contains(out, `"loop_tick":"7"`) {
		t.Fatalf("capture = %d %q", code, out)
	}
	code, out = runCLI(t, "guard", path)
	if code != exitOK || !strings.Contains(out, "PRE-WAKE") {
		t.Fatalf("guard = %d %q", code, out)
	}
	if !strings.Contains(out, `{"data":"VALID_DATA","tick":"7","status":"BOOM"}`) {
		t.Fatalf("guard reset_von ohne Vorzustand: %q", out)
	}

	s, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := (SharedState{Status: StatusClean}); s != want {
		t.Fatalf("nach guard: %+v; want %+v", s, want)
	}
}
