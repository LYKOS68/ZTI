package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	hashA  = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" // sha256("abc")
	hashB  = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" // sha256("")
	header = "timestamp;input_file;output_file;sha256;status\n"
)

var fest = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)

func zeile(h string) string { return "2026-10-25 12:00:00;in.txt;out.txt;" + h + ";SUCCESS\n" }

func schreibe(t *testing.T, pfad, inhalt string) {
	t.Helper()
	if err := os.WriteFile(pfad, []byte(inhalt), 0o644); err != nil {
		t.Fatal(err)
	}
}

func pruefe(t *testing.T, inhalt *string) LogInfo {
	t.Helper()
	pfad := filepath.Join(t.TempDir(), "log.csv")
	if inhalt != nil {
		schreibe(t, pfad, *inhalt)
	}
	info, err := PruefeLog(pfad)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

func TestPruefeLog(t *testing.T) {
	einsA := header + zeile(hashA)
	tests := []struct {
		name      string
		inhalt    *string // nil = Datei fehlt
		state     LogState
		last      string
		eintraege int
	}{
		{"fehlt", nil, LogMissing, "", 0},
		{"leer", ptr(""), LogEmpty, "", 0},
		{"nur header", ptr(header), LogEmpty, "", 0},
		{"eine zeile", ptr(einsA), LogValid, hashA, 1},
		{"zwei zeilen", ptr(einsA + zeile(hashB)), LogValid, hashB, 2},
		{"crlf", ptr(strings.ReplaceAll(einsA, "\n", "\r\n")), LogValid, hashA, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := pruefe(t, tt.inhalt)
			if info.State != tt.state || info.LastHash != tt.last || info.Eintraege != tt.eintraege {
				t.Fatalf("PruefeLog() = %+v; want %v %q %d", info, tt.state, tt.last, tt.eintraege)
			}
		})
	}
}

func TestPruefeLogCorrupt(t *testing.T) {
	tests := []struct {
		name, inhalt, grund string
	}{
		{"abgeschnitten", header + zeile(hashA)[:40], "abgeschnitten"},
		{"nur leerzeilen", "\n\n", "keine Header-Zeile"},
		{"header vertauscht", "sha256;timestamp;input_file;output_file;status\n", "Header"},
		{"header mit bom", "\xEF\xBB\xBF" + header, "Header"},
		{"fremde datei", "hallo welt\n", "Header"},
		{"zeile zu kurz", header + "a;b;c\n", "CSV"},
		{"hash kaputt", header + zeile("abc"), "Zeile 2"},
		{"hash in mitte kaputt", header + zeile("CORRUPT") + zeile(hashA), "Zeile 2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := pruefe(t, &tt.inhalt)
			if info.State != LogCorrupt || !strings.Contains(info.Grund, tt.grund) {
				t.Fatalf("PruefeLog() = %+v; want CORRUPT mit %q", info, tt.grund)
			}
		})
	}
}

func TestPruefeLogLesefehlerVerschiebtNichts(t *testing.T) {
	dir := t.TempDir() // ein Ordner ist existent, aber nicht als Datei lesbar
	if _, err := PruefeLog(dir); err == nil {
		t.Fatal("PruefeLog(Ordner) = nil; want Fehler")
	}
	_, _, _, err := Protokolliere(dir, Eintrag{Timestamp: fest, SHA256: hashA})
	if err == nil {
		t.Fatal("Protokolliere(Ordner) = nil; want Fehler")
	}
	if _, err := os.Stat(dir + ".bak"); !os.IsNotExist(err) {
		t.Fatal("bei Lesefehler darf nichts nach .bak verschoben werden")
	}
}

func TestRetteLogUeberschreibtNie(t *testing.T) {
	dir := t.TempDir()
	pfad := filepath.Join(dir, "log.csv")
	var baks []string
	for i, inhalt := range []string{"eins", "zwei", "drei"} {
		schreibe(t, pfad, inhalt)
		bak, err := RetteLog(pfad)
		if err != nil {
			t.Fatal(err)
		}
		baks = append(baks, bak)
		if b, _ := os.ReadFile(bak); string(b) != inhalt {
			t.Fatalf("bak %d = %q; want %q", i, b, inhalt)
		}
	}
	want := []string{pfad + ".bak", pfad + ".bak.1", pfad + ".bak.2"}
	for i := range want {
		if baks[i] != want[i] {
			t.Fatalf("baks = %v; want %v", baks, want)
		}
	}
	if _, err := os.Stat(pfad); !os.IsNotExist(err) {
		t.Fatal("Original muss weg sein")
	}
}

func TestHaengeAnHeader(t *testing.T) {
	dir := t.TempDir()
	e := Eintrag{
		Timestamp:  fest,
		InputFile:  "in.txt",
		OutputFile: "out.txt",
		SHA256:     hashA,
		Status:     "SUCCESS",
	}

	neu := filepath.Join(dir, "neu.csv")
	if err := HaengeAn(neu, e); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(neu); string(b) != header+zeile(hashA) {
		t.Fatalf("neu = %q", b)
	}

	leer := filepath.Join(dir, "leer.csv") // 0 Bytes: Header muss trotzdem kommen
	schreibe(t, leer, "")
	if err := HaengeAn(leer, e); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(leer); string(b) != header+zeile(hashA) {
		t.Fatalf("leer = %q", b)
	}

	e.SHA256 = hashB
	if err := HaengeAn(leer, e); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(leer); string(b) != header+zeile(hashA)+zeile(hashB) {
		t.Fatalf("zweiter Eintrag = %q", b)
	}
}

func TestHaengeAnQuoting(t *testing.T) {
	pfad := filepath.Join(t.TempDir(), "log.csv")
	e := Eintrag{Timestamp: fest, InputFile: "a;b\"c.txt", SHA256: hashA, Status: "SUCCESS"}
	if err := HaengeAn(pfad, e); err != nil {
		t.Fatal(err)
	}
	info, err := PruefeLog(pfad)
	if err != nil || info.State != LogValid || info.LastHash != hashA {
		t.Fatalf("PruefeLog() = %+v, %v", info, err)
	}
}

func TestProtokolliere(t *testing.T) {
	pfad := filepath.Join(t.TempDir(), "log.csv")
	e := func(h string) Eintrag { return Eintrag{Timestamp: fest, SHA256: h, Status: "SUCCESS"} }

	steps := []struct {
		hash    string
		want    Ergebnis
		wantBak bool
		rows    int
	}{
		{hashA, Angehaengt, false, 1},
		{hashA, Uebersprungen, false, 1}, // gleicher Hash → Skip
		{hashB, Angehaengt, false, 2},
		{hashA, Angehaengt, false, 3}, // nur der LETZTE Hash zählt
	}
	for i, st := range steps {
		res, bak, _, err := Protokolliere(pfad, e(st.hash))
		if err != nil || res != st.want || (bak != "") != st.wantBak {
			t.Fatalf("Schritt %d: %v %q %v; want %v", i, res, bak, err, st.want)
		}
		if info, _ := PruefeLog(pfad); info.Eintraege != st.rows {
			t.Fatalf("Schritt %d: %d Einträge; want %d", i, info.Eintraege, st.rows)
		}
	}

	// Kaputt machen: abgeschnittene Zeile → Rettung, Neustart mit Header.
	b, _ := os.ReadFile(pfad)
	kaputt := string(b) + "2026-10-25 12:00:00;x"
	schreibe(t, pfad, kaputt)
	res, bak, info, err := Protokolliere(pfad, e(hashB))
	if err != nil || res != Angehaengt || bak != pfad+".bak" || info.State != LogCorrupt {
		t.Fatalf("Rettung: %v %q %+v %v", res, bak, info, err)
	}
	if got, _ := os.ReadFile(bak); string(got) != kaputt {
		t.Fatal(".bak muss den kaputten Stand unverändert enthalten")
	}
	neuesLog := header + "2026-10-25 12:00:00;;;" + hashB + ";SUCCESS\n"
	if got, _ := os.ReadFile(pfad); string(got) != neuesLog {
		t.Fatalf("neues Log = %q", got)
	}
}

func runCLI(t *testing.T, args ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(args, func() time.Time { return fest }, &out, &errOut)
	return code, out.String() + errOut.String()
}

func TestCLI(t *testing.T) {
	dir := t.TempDir()
	logPfad := filepath.Join(dir, "log.csv")
	datei := filepath.Join(dir, "out.txt")
	schreibe(t, datei, "abc")

	steps := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"check", logPfad}, exitOK, "MISSING  log.csv"},
		{[]string{"append", "-input", "in.txt", logPfad, datei}, exitOK, "neuer Eintrag in log.csv"},
		{[]string{"append", logPfad, datei}, exitOK, "Skip: Hash bereits im Log"},
		{[]string{"check", logPfad}, exitOK, "VALID    log.csv eintraege=1 letzter=" + hashA},
	}
	for _, st := range steps {
		code, out := runCLI(t, st.args...)
		if code != st.code || !strings.Contains(out, st.want) {
			t.Fatalf("run(%v) = %d %q; want %d %q", st.args, code, out, st.code, st.want)
		}
	}
	if b, _ := os.ReadFile(logPfad); string(b) != header+zeile(hashA) {
		t.Fatalf("Log = %q", b)
	}

	schreibe(t, logPfad, "kaputt")
	code, out := runCLI(t, "check", logPfad)
	if code != exitCorrupt || !strings.Contains(out, "CORRUPT") {
		t.Fatalf("check = %d %q", code, out)
	}
	code, out = runCLI(t, "append", logPfad, datei)
	if code != exitOK || !strings.Contains(out, "gesichert als log.csv.bak") {
		t.Fatalf("append ohne Rettung = %d %q", code, out)
	}
	if !strings.Contains(out, "Export") {
		t.Fatalf("append ohne Eintrag = %q", out)
	}
}

func TestCLIFehler(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		nil,
		{"nope"},
		{"check"},
		{"check", "a", "b"},
		{"append", filepath.Join(dir, "log.csv")},
		{"append", "-x", "a", "b"},
		{"append", filepath.Join(dir, "log.csv"), filepath.Join(dir, "fehlt.txt")},
		{"check", dir},
	} {
		if code, out := runCLI(t, args...); code != exitUsage {
			t.Errorf("run(%v) = %d %q; want %d", args, code, out, exitUsage)
		}
	}
}

func ptr(s string) *string { return &s }
