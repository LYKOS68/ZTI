package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerarbeite(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"gross", "hallo welt", "HALLO WELT"},
		{"trim", "  \t abc \n\r\n", "ABC"},
		{"eszett", "straße", "STRASSE"},
		{"umlaute", "äöü", "ÄÖÜ"},
		{"nfc", "e\u0301", "\u00c9"},
		{"leer", "", ""},
		{"nur whitespace", " \n\t ", ""},
		{"innen bleibt", "a  b", "A  B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Verarbeite(tt.in)
			if got != tt.want {
				t.Fatalf("Verarbeite(%q) = %q; want %q", tt.in, got, tt.want)
			}
			if again := Verarbeite(got); again != got {
				t.Fatalf("nicht idempotent: %q → %q", got, again)
			}
		})
	}
}

func TestLiesText(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	tests := []struct {
		name    string
		pfad    string
		want    string
		wantErr string
	}{
		{"normal", write("a.txt", []byte("abc")), "abc", ""},
		{"bom", write("b.txt", []byte("\xEF\xBB\xBFabc")), "abc", ""},
		{"leer", write("c.txt", nil), "", ""},
		{"kein utf8", write("d.txt", []byte{0xff, 0xfe}), "", "UTF-8"},
		{"fehlt", filepath.Join(dir, "nope.txt"), "", "nicht gefunden"},
		{"ordner", dir, "", "nicht lesbar"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LiesText(tt.pfad)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v; want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("LiesText() = %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestSchreibeAtomar(t *testing.T) {
	dir := t.TempDir()
	pfad := filepath.Join(dir, "out.txt")

	for _, inhalt := range []string{"erst", "dann überschrieben"} {
		if err := SchreibeAtomar(pfad, []byte(inhalt)); err != nil {
			t.Fatal(err)
		}
		if got, _ := os.ReadFile(pfad); string(got) != inhalt {
			t.Fatalf("Inhalt = %q; want %q", got, inhalt)
		}
	}
	if m, _ := filepath.Glob(filepath.Join(dir, "*.tmp")); len(m) != 0 {
		t.Fatalf("tmp-Dateien übrig: %v", m)
	}

	// Rechte wie bei einem normalen Create (umask greift), nicht 0600 und nicht 0666.
	ref := filepath.Join(dir, "ref.txt")
	f, err := os.Create(ref)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	a, _ := os.Stat(pfad)
	b, _ := os.Stat(ref)
	if a.Mode().Perm() != b.Mode().Perm() {
		t.Fatalf("Rechte = %v; want %v", a.Mode().Perm(), b.Mode().Perm())
	}

	if err := SchreibeAtomar(filepath.Join(dir, "fehlt", "x.txt"), []byte("x")); err == nil {
		t.Fatal("Schreiben in fehlenden Ordner muss scheitern")
	}
}

func TestHashText(t *testing.T) {
	tests := map[string]string{
		"":    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"abc": "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	}
	for in, want := range tests {
		if got := HashText(in); got != want {
			t.Errorf("HashText(%q) = %s; want %s", in, got, want)
		}
	}
}

func TestEvalGateI(t *testing.T) {
	tests := []struct {
		z, t     uint16
		wantBit  uint8
		wantMask uint16
	}{
		{0x0100, 0x0005, 1, 0x0105},
		{0x0105, 0x0000, 0, 0x0105}, // T = 0 → Bit 0
		{0x0105, 0x0005, 0, 0x0100}, // T ganz in Z enthalten → Inversionsecho
		{0x0105, 0x0004, 0, 0x0101},
		{0x0105, 0x000F, 1, 0x010A}, // nur teilweise enthalten
		{0x0000, 0xFFFF, 1, 0xFFFF},
		{0xFFFF, 0xFFFF, 0, 0x0000},
	}
	for _, tt := range tests {
		bit, mask := EvalGateI(tt.z, tt.t)
		if bit != tt.wantBit || mask != tt.wantMask {
			t.Errorf("EvalGateI(%#x, %#x) = %d, %#x; want %d, %#x",
				tt.z, tt.t, bit, mask, tt.wantBit, tt.wantMask)
		}
	}
}

func TestTaktSchritt(t *testing.T) {
	var g GatterNetzwerk64
	g.Reset(0x0100)

	r1 := g.TaktSchritt(0x0005)
	if r1.Takt != 0 || r1.GateBit != 1 || r1.Z0 != "0x100" || r1.Z1 != "0x105" ||
		r1.Phi1 != 1 || r1.Status != "UPDATED" || r1.GateMask != "0x105" || !r1.InvarianteBestanden {
		t.Fatalf("Takt 1 = %+v", r1)
	}
	r2 := g.TaktSchritt(0x0000)
	if r2.Takt != 1 || r2.GateBit != 0 || r2.Z1 != "0x105" || r2.Phi1 != 2 ||
		r2.Status != "CLEAN" || !r2.InvarianteBestanden {
		t.Fatalf("Takt 2 = %+v", r2)
	}
	if g != (GatterNetzwerk64{RegZ: 0x0105, RegT: 0, RegI: 0x0105, RegPhi: 2}) {
		t.Fatalf("Register = %+v", g)
	}
}

func TestTaktPhiWrap(t *testing.T) {
	var g GatterNetzwerk64
	g.Reset(0)
	var r TaktErgebnis
	for range 64 {
		r = g.TaktSchritt(0)
	}
	if r.Takt != 63 || r.Phi1 != 0 || !r.InvarianteBestanden {
		t.Fatalf("Takt 64 = %+v", r)
	}
}

func TestTaktUeberlaufVerletztMonotonie(t *testing.T) {
	var g GatterNetzwerk64
	g.Reset(0x8000)
	r := g.TaktSchritt(0x8001) // 0x8000 + 0x8001 = 0x10001 → 0x0001
	if r.GateBit != 1 || r.Z1 != "0x1" || r.InvarianteBestanden {
		t.Fatalf("Überlauf = %+v; want Bit 1, Z1 0x1, Invariante false", r)
	}
}

func TestFindeRoot(t *testing.T) {
	base := t.TempDir()
	tief := filepath.Join(base, "a", "b", "c")
	if err := os.MkdirAll(tief, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := FindeRoot(tief); ok {
		t.Fatal("ohne Marker darf nichts gefunden werden")
	}

	for _, marker := range []string{"src", "01_spezifikationen"} {
		t.Run(marker, func(t *testing.T) {
			m := filepath.Join(base, "a", marker)
			if err := os.Mkdir(m, 0o755); err != nil {
				t.Fatal(err)
			}
			defer os.Remove(m)
			got, ok := FindeRoot(tief)
			if !ok || got != filepath.Join(base, "a") {
				t.Fatalf("FindeRoot() = %q, %v", got, ok)
			}
		})
	}
}

func TestBeweisPfadOhneRootNimmtArbeitsordner(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	got, err := BeweisPfad("")
	if err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if got != filepath.Join(cwd, beweisDatei) {
		t.Skipf("Root oberhalb des Testprogramms gefunden: %s", got)
	}
}

func TestBeweisPfadSuchtAbArbeitsordner(t *testing.T) {
	base := t.TempDir()
	tief := filepath.Join(base, "x", "y")
	if err := os.MkdirAll(filepath.Join(base, "01_spezifikationen"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tief, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tief)
	got, err := BeweisPfad("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, filepath.Join("src", "03_kern-engine", "BEWEIS", beweisDatei)) {
		t.Fatalf("BeweisPfad() = %q", got)
	}
}

func runCLI(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, strings.NewReader(stdin), &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestCLIRun(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.txt")
	outPfad := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(in, []byte("\xEF\xBB\xBF  straße \n"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out, _ := runCLI(t, "", "run", in, outPfad)
	want := "=== Atomic-Pipe gestartet ===\n" +
		"[1/4] Eingelesen: 10 Zeichen\n" +
		"[2/4] Verarbeitet: 7 Zeichen\n" +
		"[3/4] Atomar geschrieben + zurückgelesen: out.txt\n" +
		"[4/4] Hash der DATEI (sha256): " + HashText("STRASSE") + "\n" +
		"=== Fertig ===\n"
	if code != exitOK || out != want {
		t.Fatalf("code %d\n got: %q\nwant: %q", code, out, want)
	}
	if b, _ := os.ReadFile(outPfad); string(b) != "STRASSE" {
		t.Fatalf("Datei = %q", b)
	}

	// Gleicher Inhalt → gleiche Ausgabe (I0).
	if _, out2, _ := runCLI(t, "", "run", in, outPfad); out2 != out {
		t.Fatalf("nicht deterministisch:\n%q\n%q", out, out2)
	}
}

func TestCLIRunStdin(t *testing.T) {
	outPfad := filepath.Join(t.TempDir(), "out.txt")
	code, out, _ := runCLI(t, "hallo", "run", "-", outPfad)
	if code != exitOK || !strings.Contains(out, HashText("HALLO")) {
		t.Fatalf("code %d out %q", code, out)
	}

	code, out, _ = runCLI(t, "\xff", "run", "-", outPfad)
	if code != exitUsage || !strings.Contains(out, "stdin") {
		t.Fatalf("code %d out %q", code, out)
	}
}

func TestCLIRunFehler(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(in, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	nope, o := filepath.Join(dir, "nope"), filepath.Join(dir, "o")

	tests := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"input fehlt", []string{"run", nope, o}, exitUsage, "nicht gefunden"},
		{"output-ordner fehlt", []string{"run", in, filepath.Join(dir, "x", "o")}, exitIO, "I/O FEHLER"},
		{"zu wenige args", []string{"run", in}, exitUsage, "Aufruf"},
		{"unbekannt", []string{"rnu", in, in}, exitUsage, "Aufruf"},
		{"leer", nil, exitUsage, "Aufruf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := runCLI(t, "", tt.args...)
			if code != tt.code || !strings.Contains(out+errOut, tt.want) {
				t.Fatalf("code %d out %q err %q; want %d %q", code, out, errOut, tt.code, tt.want)
			}
		})
	}
}

func TestCLINachweis(t *testing.T) {
	root := t.TempDir()
	code, out, errOut := runCLI(t, "", "nachweis", "-root", root)
	if code != exitOK {
		t.Fatalf("code %d out %q err %q", code, out, errOut)
	}
	for _, w := range []string{
		"[TAKT 0 -> 1] Input T=0x0005 | Bit=1 | Z0=0x100 -> Z1=0x105 | Status=UPDATED",
		"[TAKT 1 -> 2] Input T=0x0000 | Bit=0 | Z0=0x105 -> Z1=0x105 | Status=CLEAN",
		"VOLLSTÄNDIG BESTANDEN",
	} {
		if !strings.Contains(out, w) {
			t.Fatalf("Ausgabe ohne %q:\n%s", w, out)
		}
	}

	pfad := filepath.Join(root, "src", "03_kern-engine", "BEWEIS", beweisDatei)
	raw, err := os.ReadFile(pfad)
	if err != nil {
		t.Fatal(err)
	}
	var b Beweis
	if err := json.Unmarshal(raw, &b); err != nil {
		t.Fatal(err)
	}
	if b.Ergebnis != "BESTANDEN" || b.GatterGesamt != 64 {
		t.Fatalf("Beweis = %+v", b)
	}
	if len(b.Protokoll) != 2 || len(b.Bloecke) != 4 {
		t.Fatalf("Beweis unvollständig: %+v", b)
	}
	if b.Protokoll[0].Fall != "VALIDE_TRANSFORMATION" || b.Protokoll[1].Ergebnis.Z1 != "0x105" {
		t.Fatalf("Protokoll = %+v", b.Protokoll)
	}
	if !strings.Contains(string(raw), "Phasen Zähler") {
		t.Fatal("Umlaute müssen unescaped bleiben")
	}
}

func TestCLINachweisFehler(t *testing.T) {
	if code, _, _ := runCLI(t, "", "nachweis", "extra"); code != exitUsage {
		t.Fatalf("code %d; want %d", code, exitUsage)
	}
	if code, _, _ := runCLI(t, "", "nachweis", "-falsch"); code != exitUsage {
		t.Fatalf("code %d; want %d", code, exitUsage)
	}
	// root ist eine Datei → MkdirAll scheitert
	f := filepath.Join(t.TempDir(), "datei")
	if err := os.WriteFile(f, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := runCLI(t, "", "nachweis", "-root", f); code != exitIO {
		t.Fatalf("code %d err %q; want %d", code, errOut, exitIO)
	}
}

func TestCLITakt(t *testing.T) {
	code, out, _ := runCLI(t, "", "takt", "0x100", "0x5", "0", "0b1111")
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if code != exitOK || len(lines) != 3 {
		t.Fatalf("code %d out %q", code, out)
	}
	var r TaktErgebnis
	if err := json.Unmarshal([]byte(lines[2]), &r); err != nil {
		t.Fatal(err)
	}
	// 0x105 & 0xF = 0x5 ≠ 0xF → Bit 1, 0x105 + 0xF = 0x114
	if r.Takt != 2 || r.GateBit != 1 || r.Z1 != "0x114" {
		t.Fatalf("Takt 3 = %+v", r)
	}
	if !strings.HasPrefix(lines[0], `{"takt":0,"eingabe_T":"0x5","z0":"0x100",`) {
		t.Fatalf("Feldreihenfolge: %s", lines[0])
	}

	if code, _, _ := runCLI(t, "", "takt", "0x8000", "0x8001"); code != exitMismatch {
		t.Fatalf("Überlauf: code %d; want %d", code, exitMismatch)
	}
	for _, args := range [][]string{
		{"takt", "0x100"},
		{"takt", "0x10000", "1"},
		{"takt", "1", "-1"},
		{"takt", "abc", "1"},
	} {
		if code, _, _ := runCLI(t, "", args...); code != exitUsage {
			t.Fatalf("%v: code %d; want %d", args, code, exitUsage)
		}
	}
}
