package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const zuGross = `99999999999999999999` // > int64

// telStart ist die Start-Telemetrie als JSON.
const telStart = `{"sektion":"S1","zoom":"MAKRO","active_operator":"ALPHA","last_bit":null}`

var tel = NewTelemetry()

// stateJSON baut ein Zustandsobjekt nach Schema; data=null, hash=null, history=[],
// telemetry=Startwert.
func stateJSON(tick, latency, status string) string {
	return fmt.Sprintf(`{"data":null,"hash":null,"latency_ms":%s,"history":[],"status":%s,"tick":%s,`+
		`"telemetry":%s}`, latency, status, tick, telStart)
}

// mit ersetzt ein Feld in einem stateJSON-Objekt.
func mit(body, feld, wert string) string {
	var m map[string]json.RawMessage
	_ = json.Unmarshal([]byte(body), &m)
	m[feld] = json.RawMessage(wert)
	b, _ := json.Marshal(m)
	return string(b)
}

func gleich(t *testing.T, got, want SharedState) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func ptr(s string) *string { return &s }

func TestIsValid(t *testing.T) {
	tests := []struct {
		name string
		s    SharedState
		want bool
	}{
		{"neu", NewSharedState(), true},
		{"updated", SharedState{Telemetry: tel, Data: "X", Hash: ptr("h"), Status: "UPDATED"}, true},
		{"clean", SharedState{Telemetry: tel, Status: StatusClean}, true},
		{"nullwert status leer", SharedState{}, false},
		{"tick negativ", SharedState{Telemetry: tel, Tick: -1, Status: StatusSleep}, false},
		{"latency negativ", SharedState{Telemetry: tel, LatencyMs: -0.1, Status: StatusSleep}, false},
		{"latency NaN", SharedState{Telemetry: tel, LatencyMs: math.NaN(), Status: StatusSleep}, false},
		{"latency Inf", SharedState{Telemetry: tel, LatencyMs: math.Inf(1), Status: StatusSleep}, false},
		{"hash falscher typ", SharedState{Telemetry: tel, badHash: true, Status: StatusSleep}, false},
		{"history kein array", SharedState{Telemetry: tel, badHistory: true, Status: "SLEEP"}, false},
		{"status unbekannt", SharedState{Telemetry: tel, Status: "BOOM"}, false},
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
	s := SharedState{Tick: -1, LatencyMs: math.NaN(), badHash: true, badHistory: true, Status: "X"}
	s.badTelemetry = true
	err := s.Check()
	if !errors.Is(err, ErrInkonsistent) || strings.Count(err.Error(), "inkonsistent") != 6 {
		t.Fatalf("Check() = %v; want 6 Verletzungen", err)
	}
}

// Prüfhelfer für TestDecodeStateTypen.
type pruef func(SharedState) bool

func tickIst(n int64) pruef      { return func(s SharedState) bool { return s.Tick == n } }
func latencyIst(f float64) pruef { return func(s SharedState) bool { return s.LatencyMs == f } }
func statusIst(st Status) pruef  { return func(s SharedState) bool { return s.Status == st } }
func historyLen(n int) pruef     { return func(s SharedState) bool { return len(s.History) == n } }

var (
	latencyNaN pruef = func(s SharedState) bool { return math.IsNaN(s.LatencyMs) }
	hashKaputt pruef = func(s SharedState) bool { return s.badHash }
	histKaputt pruef = func(s SharedState) bool { return s.badHistory }
	hashABC    pruef = func(s SharedState) bool { return s.Hash != nil && *s.Hash == "abc" }
)

func TestDecodeStateTypen(t *testing.T) {
	ok := stateJSON(`2`, `1.5`, `"WAKE"`)
	sleep := func(tick, latency string) string { return stateJSON(tick, latency, `"SLEEP"`) }
	tests := []struct {
		name  string
		body  string
		valid bool
		check pruef
	}{
		{"gültig", ok, true, tickIst(2)},
		{"latency gültig", ok, true, latencyIst(1.5)},
		{"latency als int", sleep(`0`, `7`), true, latencyIst(7)},
		{"hash string", mit(ok, "hash", `"abc"`), true, hashABC},
		{"history mit inhalt", mit(ok, "history", `[1,{"a":2}]`), true, historyLen(2)},
		{"tick als float", sleep(`1.5`, `0`), false, tickIst(-1)},
		{"tick als string", sleep(`"3"`, `0`), false, tickIst(-1)},
		{"tick als bool", sleep(`true`, `0`), false, tickIst(-1)},
		{"tick überlauf", sleep(zuGross, `0`), false, tickIst(-1)},
		{"latency als string", sleep(`0`, `"0"`), false, latencyNaN},
		{"latency negativ", sleep(`0`, `-1`), false, latencyIst(-1)},
		{"hash als zahl", mit(ok, "hash", `5`), false, hashKaputt},
		{"history als objekt", mit(ok, "history", `{}`), false, histKaputt},
		{"history null", mit(ok, "history", `null`), false, histKaputt},
		{"status als zahl", stateJSON(`0`, `0`, `1`), false, statusIst("")},
		{"felder fehlen: hash", `{}`, false, hashKaputt},
		{"felder fehlen: history", `{}`, false, histKaputt},
		{"felder fehlen: tick", `{}`, false, tickIst(-1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, err := DecodeState([]byte(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			if !tt.check(s) || s.IsValid() != tt.valid {
				t.Fatalf("got %+v valid=%v (%v)", s, s.IsValid(), s.Check())
			}
		})
	}
}

func TestDecodeStateData(t *testing.T) {
	const data = `{"n":12345678901234567890}`
	s, err := DecodeState([]byte(mit(stateJSON(`0`, `0`, `"SLEEP"`), "data", data)))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := json.Marshal(s.Data); string(b) != data {
		t.Fatalf("data verlustbehaftet: %s", b)
	}
}

func TestDecodeStateFehler(t *testing.T) {
	for name, body := range map[string]string{
		"altes feld last_pulse": mit(stateJSON(`0`, `0`, `"SLEEP"`), "last_pulse", `0`),
		"unbekanntes feld":      mit(stateJSON(`0`, `0`, `"SLEEP"`), "x", `1`),
		"array":                 `[1,2]`,
		"null":                  `null`,
		"müll danach":           `{"tick":0} {}`,
		"kein json":             `tick=0`,
		"leer":                  ``,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeState([]byte(body)); err == nil {
				t.Fatal("DecodeState() = nil; want Fehler")
			}
		})
	}
}

func TestHeal(t *testing.T) {
	hist := []any{"eintrag"}
	tests := []struct {
		name string
		in   SharedState
		want SharedState
	}{
		{
			name: "updated → clean, tick und history bleiben",
			in: SharedState{
				Data: "X", Hash: ptr("h"), LatencyMs: 0.6, History: hist, Status: StatusUpdated, Tick: 3,
			},
			want: SharedState{Telemetry: tel, LatencyMs: 0.6, History: hist, Status: StatusClean, Tick: 3},
		},
		{
			name: "alles kaputt → nullwerte",
			in: SharedState{
				Data: 1, badHash: true, LatencyMs: math.NaN(), badHistory: true, Status: "BOOM", Tick: -1,
			},
			want: SharedState{Telemetry: tel, History: []any{}, Status: StatusClean},
		},
		{
			name: "latency negativ → 0",
			in:   SharedState{Telemetry: tel, LatencyMs: -5, History: []any{}, Status: StatusWake, Tick: 5},
			want: SharedState{Telemetry: tel, History: []any{}, Status: StatusClean, Tick: 5},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.in
			alt, err := s.Heal()
			if err != nil {
				t.Fatal(err)
			}
			gleich(t, s, tt.want)
			if alt.Data != tt.in.Data || alt.Tick != tt.in.Tick || alt.Status != tt.in.Status {
				t.Fatalf("reset_von = %+v; want Vorzustand %+v", alt, tt.in)
			}
		})
	}
}

func TestCapture(t *testing.T) {
	got := SharedState{Telemetry: tel, Data: "X", Tick: 2, Status: StatusUpdated}.Capture()
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
	want := SharedState{
		Data:      "VALID_DATA",
		Hash:      ptr("abc"),
		LatencyMs: 0.6,
		History:   []any{json.RawMessage(`{"z":1,"a":2}`)}, // bewusst nicht alphabetisch
		Status:    StatusUpdated,
		Tick:      4,
		Telemetry: Telemetry{Sektion: "S5", Zoom: "META", ActiveOperator: "DELTA", LastBit: eins},
	}
	if err := SaveState(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	if string(a) != string(b) {
		t.Fatalf("Roundtrip:\n got %s\nwant %s", a, b)
	}
	if !strings.Contains(string(a), `"history":[{"z":1,"a":2}]`) {
		t.Fatalf("Schlüsselreihenfolge in history verändert: %s", a)
	}
	if m, _ := filepath.Glob(path + ".*.tmp"); len(m) != 0 {
		t.Fatalf("tmp-Dateien übrig: %v", m)
	}
}

func TestSchemaWieZOOMSEK(t *testing.T) {
	b, err := json.Marshal(NewSharedState())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"data":null,"hash":null,"latency_ms":0,"history":[],"status":"SLEEP","tick":0,` +
		`"telemetry":` + telStart + `}`
	if string(b) != want {
		t.Fatalf("Schema = %s; want %s", b, want)
	}
	b, _ = json.Marshal(SharedState{Telemetry: tel, Status: StatusClean})
	if !strings.Contains(string(b), `"history":[]`) {
		t.Fatalf("nil-History muss als [] erscheinen: %s", b)
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
	b, _ := os.ReadFile(path)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil || len(m) != 7 {
		t.Fatalf("Datei hat nicht die 7 Schema-Felder: %s", b)
	}
}

func TestCLIGuardHeiltInkonsistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	body := `{"data":"VALID_DATA","hash":7,"latency_ms":true,` +
		`"history":"x","status":"BOOM","tick":"7"}`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out := runCLI(t, "check", path)
	if code != exitInkonsistent || strings.Count(out, "inkonsistent") != 6 {
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
	gleich(t, s, SharedState{Telemetry: tel, History: []any{}, Status: StatusClean})
}

var (
	null = (*int)(nil)
	eins = func() *int { i := 1; return &i }()
	zwei = func() *int { i := 2; return &i }()
)

func TestTelemetryI10(t *testing.T) {
	tests := []struct {
		name string
		t    Telemetry
		ok   bool
	}{
		{"start S1/MAKRO/ALPHA", NewTelemetry(), true},
		{"S1/META/CHARLIE", Telemetry{"S1", "META", "CHARLIE", null}, true},
		{"S3/MESO/BRAVO bit 1", Telemetry{"S3", "MESO", "BRAVO", eins}, true},
		{"S4/NANO/DELTA", Telemetry{"S4", "NANO", "DELTA", null}, true},
		{"S5/META/BRAVO", Telemetry{"S5", "META", "BRAVO", eins}, true},
		{"sektion unbekannt", Telemetry{"S6", "META", "ALPHA", null}, false},
		{"zoom passt nicht", Telemetry{"S1", "NANO", "ALPHA", null}, false},
		{"zoom reduziert META_NANO", Telemetry{"S4", "META_NANO", "DELTA", null}, false},
		{"operator passt nicht", Telemetry{"S3", "MESO", "DELTA", null}, false},
		{"operator kleingeschrieben", Telemetry{"S1", "MAKRO", "alpha", null}, false},
		{"last_bit 2", Telemetry{"S1", "MAKRO", "ALPHA", zwei}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.t.Check(); (err == nil) != tt.ok {
				t.Fatalf("Check() = %v; ok want %v", err, tt.ok)
			}
		})
	}
}

func TestDecodeTelemetry(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		bad  bool
		want Telemetry
	}{
		{"start", telStart, false, NewTelemetry()},
		{"last_bit 1", `{"sektion":"S5","zoom":"META","active_operator":"DELTA","last_bit":1}`, false,
			Telemetry{"S5", "META", "DELTA", eins}},
		{"fehlt", ``, true, Telemetry{}},
		{"null", `null`, true, Telemetry{}},
		{"liste", `[]`, true, Telemetry{}},
		{"feld fehlt", `{"sektion":"S1","zoom":"MAKRO","active_operator":"ALPHA"}`, true, Telemetry{}},
		{"feld zu viel", strings.Replace(telStart, "}", `,"x":1}`, 1),
			true, Telemetry{}},
		{"sektion zahl", strings.Replace(telStart, `"S1"`, `1`, 1), true, Telemetry{}},
		{"last_bit string", `{"sektion":"S1","zoom":"MAKRO","active_operator":"ALPHA","last_bit":"1"}`,
			true, Telemetry{}},
		{"last_bit float", `{"sektion":"S1","zoom":"MAKRO","active_operator":"ALPHA","last_bit":0.5}`,
			true, Telemetry{}},
		{"last_bit bool", `{"sektion":"S1","zoom":"MAKRO","active_operator":"ALPHA","last_bit":true}`,
			true, Telemetry{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, bad := decodeTelemetry(json.RawMessage(tt.raw))
			if bad != tt.bad || !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("decodeTelemetry = %+v, %v; want %+v, %v", got, bad, tt.want, tt.bad)
			}
		})
	}
}

func TestHealTelemetry(t *testing.T) {
	gueltig := Telemetry{"S4", "NANO", "DELTA", eins}
	s := SharedState{Telemetry: gueltig, History: []any{}, Status: StatusUpdated}
	if _, err := s.Heal(); err != nil || !reflect.DeepEqual(s.Telemetry, gueltig) {
		t.Fatalf("gültige telemetry muss bleiben: %+v, %v", s.Telemetry, err)
	}

	for _, kaputt := range []SharedState{
		{Telemetry: Telemetry{"S3", "MESO", "DELTA", null}, Status: StatusWake}, // I10 verletzt
		{badTelemetry: true, Status: StatusWake},                                // falscher Typ
	} {
		if _, err := kaputt.Heal(); err != nil || !reflect.DeepEqual(kaputt.Telemetry, NewTelemetry()) {
			t.Fatalf("ungültige telemetry muss auf Startwert: %+v, %v", kaputt.Telemetry, err)
		}
	}
}
