package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func runPythonPipeline(t *testing.T, pyScript, input, output, log string) (int, string, string) {
	t.Helper()
	cmd := exec.Command("python3", pyScript, input, output, log)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("Python execution error: %v", err)
		}
	}
	return exitCode, stdout.String(), stderr.String()
}

func runGoPipeline(t *testing.T, input, output, log string) (int, string, string) {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Run(input, output, log, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestParityWithPython(t *testing.T) {
	pyScript, err := filepath.Abs("../t1_pipeline-py/pipeline.py")
	if err != nil {
		t.Fatalf("Path error: %v", err)
	}

	testCases := []struct {
		name    string
		content []byte
	}{
		{
			name:    "StandardText",
			content: []byte("  rap-lab beats and recording 2026 \n  mix & master  "),
		},
		{
			// Testfall mit Umlauten UND 'ß':
			// cases.Upper(language.Und) expandiert 'ß' zu 'SS' und erzeugt
			// exakte Byte-Parität mit Pythons Unicode-Casing.
			name:    "UmlautsAndSharpS",
			content: []byte("  Künstler & Gesang: Größe und Qualität für Musiker  \n"),
		},
		{
			// Testfall: 'straße' -> 'STRASSE' kombiniert mit Windows CRLF (\r\n)
			name:    "StrasseWithCRLF",
			content: []byte("  abmischung in der berliner straße  \r\n"),
		},
		{
			// Testfall: Non-Breaking-Space (\u00A0) Trimming
			name:    "NonBreakingSpaceNBSP",
			content: []byte("\u00A0\u00A0rap track studio audio\u00A0\u00A0"),
		},
		{
			name:    "UTF8_BOM",
			content: append([]byte{0xEF, 0xBB, 0xBF}, []byte("  Track mit UTF-8 BOM  ")...),
		},
		{
			name:    "EmptyFile",
			content: []byte(""),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			// 1. Python Run
			pyIn := filepath.Join(tmpDir, "py_in.txt")
			pyOut := filepath.Join(tmpDir, "py_out.txt")
			pyLog := filepath.Join(tmpDir, "py_log.csv")
			if err := os.WriteFile(pyIn, tc.content, 0644); err != nil {
				t.Fatal(err)
			}
			pyCode, _, _ := runPythonPipeline(t, pyScript, pyIn, pyOut, pyLog)

			// 2. Go Run
			goIn := filepath.Join(tmpDir, "go_in.txt")
			goOut := filepath.Join(tmpDir, "go_out.txt")
			goLog := filepath.Join(tmpDir, "go_log.csv")
			if err := os.WriteFile(goIn, tc.content, 0644); err != nil {
				t.Fatal(err)
			}
			goCode, _, _ := runGoPipeline(t, goIn, goOut, goLog)

			// Verify Exit Codes
			if pyCode != goCode {
				t.Errorf("Exit code mismatch: Python=%d, Go=%d", pyCode, goCode)
			}

			// Verify Byte Parity of Output Files
			pyOutBytes, err := os.ReadFile(pyOut)
			if err != nil {
				t.Fatal(err)
			}
			goOutBytes, err := os.ReadFile(goOut)
			if err != nil {
				t.Fatal(err)
			}

			if !bytes.Equal(pyOutBytes, goOutBytes) {
				t.Errorf("Output mismatch:\nPython: %q\nGo:     %q", string(pyOutBytes), string(goOutBytes))
			}

			// Verify SHA-256 Hash Parity
			pyHash := sha256.Sum256(pyOutBytes)
			goHash := sha256.Sum256(goOutBytes)
			if pyHash != goHash {
				t.Errorf("Hash mismatch: Python=%x, Go=%x", pyHash, goHash)
			}

			// 3. Test Invariant I0: Repeated Run -> Skip
			pyCode2, pyOutMsg2, _ := runPythonPipeline(t, pyScript, pyIn, pyOut, pyLog)
			goCode2, goOutMsg2, _ := runGoPipeline(t, goIn, goOut, goLog)
			if pyCode2 != 0 || goCode2 != 0 {
				t.Errorf("Second run non-zero: Python=%d, Go=%d", pyCode2, goCode2)
			}
			if !strings.Contains(pyOutMsg2, "Skip:") || !strings.Contains(goOutMsg2, "Skip:") {
				t.Errorf("Expected skip message on identical re-run")
			}
		})
	}
}

func TestErrorPathsAndCorruptLog(t *testing.T) {
	pyScript, _ := filepath.Abs("../t1_pipeline-py/pipeline.py")
	tmpDir := t.TempDir()

	// 1. Missing File -> Exit Code 2 auf beiden Seiten
	pyCode, _, _ := runPythonPipeline(t, pyScript, filepath.Join(tmpDir, "missing.txt"), filepath.Join(tmpDir, "out_py.txt"), filepath.Join(tmpDir, "log_py.csv"))
	goCode, _, _ := runGoPipeline(t, filepath.Join(tmpDir, "missing.txt"), filepath.Join(tmpDir, "out_go.txt"), filepath.Join(tmpDir, "log_go.csv"))
	if pyCode != 2 || goCode != 2 {
		t.Errorf("Expected exit code 2 for missing file. Got Python=%d, Go=%d", pyCode, goCode)
	}

	// 2. Corrupt Log Recovery -> .bak file generated auf Python und Go
	corruptContent := []byte("invalid;header;without;required;columns\n1;2;3;4;5\n")
	testInput := []byte("rap-lab test audio export")

	// Python Run
	pyDir := t.TempDir()
	pyIn := filepath.Join(pyDir, "in.txt")
	pyOut := filepath.Join(pyDir, "out.txt")
	pyLog := filepath.Join(pyDir, "log.csv")
	_ = os.WriteFile(pyIn, testInput, 0644)
	_ = os.WriteFile(pyLog, corruptContent, 0644)
	pyExit, pyStdout, _ := runPythonPipeline(t, pyScript, pyIn, pyOut, pyLog)

	// Go Run
	goDir := t.TempDir()
	goIn := filepath.Join(goDir, "in.txt")
	goOut := filepath.Join(goDir, "out.txt")
	goLog := filepath.Join(goDir, "log.csv")
	_ = os.WriteFile(goIn, testInput, 0644)
	_ = os.WriteFile(goLog, corruptContent, 0644)
	goExit, goStdout, _ := runGoPipeline(t, goIn, goOut, goLog)

	if pyExit != 0 || goExit != 0 {
		t.Errorf("Expected exit code 0 on corrupt log recovery. Got Python=%d, Go=%d", pyExit, goExit)
	}

	if !strings.Contains(pyStdout, "WARNUNG: Log war kaputt -> gesichert als") {
		t.Errorf("Python expected corrupt log warning, got: %s", pyStdout)
	}
	if !strings.Contains(goStdout, "WARNUNG: Log war kaputt -> gesichert als") {
		t.Errorf("Go expected corrupt log warning, got: %s", goStdout)
	}

	// Beide müssen .bak erzeugt haben
	if _, err := os.Stat(pyLog + ".bak"); os.IsNotExist(err) {
		t.Errorf("Expected Python .bak backup file to exist")
	}
	if _, err := os.Stat(goLog + ".bak"); os.IsNotExist(err) {
		t.Errorf("Expected Go .bak backup file to exist")
	}

	// Hash-Parität im neu geschriebenen Log verifizieren
	goHash, err := LetzterHash(goLog)
	if err != nil || len(goHash) != 64 {
		t.Fatalf("Invalid Go hash in rewritten log: %v", goHash)
	}
	pyOutBytes, _ := os.ReadFile(pyOut)
	expectedPyHash := sha256.Sum256(pyOutBytes)
	if goHash != hex.EncodeToString(expectedPyHash[:]) {
		t.Errorf("Mismatch in recovered log hash: Go=%s, PythonExpected=%x", goHash, expectedPyHash)
	}
}

func TestSingleCharacterUnicodeInspection(t *testing.T) {
	input := "  hip-hop 808 straße  "
	got := Verarbeite(input)
	expected := "HIP-HOP 808 STRASSE"
	if got != expected {
		t.Errorf("Expected %q, got %q", expected, got)
	}
	h := HashText(got)
	sum := sha256.Sum256([]byte(expected))
	if h != hex.EncodeToString(sum[:]) {
		t.Errorf("HashText produced incorrect hash")
	}
}

func TestNonExistentOutputDirExit1(t *testing.T) {
	pyScript, _ := filepath.Abs("../t1_pipeline-py/pipeline.py")
	tmpDir := t.TempDir()

	inPath := filepath.Join(tmpDir, "in.txt")
	_ = os.WriteFile(inPath, []byte("rap-lab beat"), 0644)
	logPath := filepath.Join(tmpDir, "log.csv")

	// Ungültiger Ausgabepfad in nicht-existentem Unterordner
	invalidOutPath := filepath.Join(tmpDir, "non_existent_subdir", "out.txt")

	pyCode, _, _ := runPythonPipeline(t, pyScript, inPath, invalidOutPath, logPath)
	goCode, _, _ := runGoPipeline(t, inPath, invalidOutPath, logPath)

	if pyCode != 1 || goCode != 1 {
		t.Errorf("Expected exit code 1 on invalid output dir. Got Python=%d, Go=%d", pyCode, goCode)
	}
}

func TestMultiLineLogAndSkip(t *testing.T) {
	pyScript, _ := filepath.Abs("../t1_pipeline-py/pipeline.py")
	tmpDir := t.TempDir()

	pyLog := filepath.Join(tmpDir, "py_log.csv")
	goLog := filepath.Join(tmpDir, "go_log.csv")

	steps := []string{
		"Track 1 - Beat A",
		"Track 2 - Beat B",
		"Track 2 - Beat B", // Identisch zu vorherigem -> muss Skip auslösen
	}

	for idx, content := range steps {
		inPy := filepath.Join(tmpDir, fmt.Sprintf("in_py_%d.txt", idx))
		outPy := filepath.Join(tmpDir, fmt.Sprintf("out_py_%d.txt", idx))
		_ = os.WriteFile(inPy, []byte(content), 0644)

		inGo := filepath.Join(tmpDir, fmt.Sprintf("in_go_%d.txt", idx))
		outGo := filepath.Join(tmpDir, fmt.Sprintf("out_go_%d.txt", idx))
		_ = os.WriteFile(inGo, []byte(content), 0644)

		pyExit, pyOut, _ := runPythonPipeline(t, pyScript, inPy, outPy, pyLog)
		goExit, goOut, _ := runGoPipeline(t, inGo, outGo, goLog)

		if pyExit != 0 || goExit != 0 {
			t.Fatalf("Step %d failed: Python=%d, Go=%d", idx, pyExit, goExit)
		}

		if idx == 2 {
			// Der dritte Schritt muss Skip enthalten
			if !strings.Contains(pyOut, "Skip:") || !strings.Contains(goOut, "Skip:") {
				t.Errorf("Step 2 should trigger skip in both. py=%s, go=%s", pyOut, goOut)
			}
		}
	}

	// Logs vergleichen
	pyLogLines, _ := os.ReadFile(pyLog)
	goLogLines, _ := os.ReadFile(goLog)

	pyCount := strings.Count(string(pyLogLines), "\n")
	goCount := strings.Count(string(goLogLines), "\n")
	if pyCount != goCount {
		t.Errorf("Log line count mismatch: Python=%d, Go=%d", pyCount, goCount)
	}
}

func TestFilePermissionsAfterWrite(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "out_perm.txt")

	if err := SchreibeAtomar(outPath, "PERM TEST"); err != nil {
		t.Fatalf("SchreibeAtomar failed: %v", err)
	}

	info, err := os.Stat(outPath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	// Darf nicht auf 0600 beschränkt sein (CreateTemp default), sondern muss Gruppen-/Weltleserechte gemäß umask besitzen
	mode := info.Mode().Perm()
	if mode == 0600 {
		t.Errorf("File permissions not adjusted: got 0600, expected relaxed umask permissions (e.g. 0644/0664)")
	}
}
