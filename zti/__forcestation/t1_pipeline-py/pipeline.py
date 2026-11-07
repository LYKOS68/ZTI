"""pipeline.py - reproduzierbare Transform-und-Attestier-Kette.

Zweck: eine Datei deterministisch verarbeiten, das Ergebnis atomar schreiben und
ein Log fuehren, das nur waechst wenn sich der Inhalt wirklich aendert.

I0 (Invariante der Gesamtkette):
  Gleicher input-Inhalt  =>  gleicher output-Inhalt UND gleicher sha256.
  Ueber beliebig viele Laeufe: genau ein Log-Eintrag pro distinktem output-Hash.
  Kaputtes Log => erkannt und gesichert, nie stiller Anhang. (Zeitstempel ausgenommen.)

Aufruf:  python pipeline.py <input> <output> <log.csv>
"""

import csv
import hashlib
import io
import os
import sys
import unicodedata
from datetime import datetime
from pathlib import Path

LOG_SPALTEN = ["timestamp", "input_file", "output_file", "sha256", "status"]


# --- Schritt 1: INPUT -------------------------------------------------------
def lies_text(pfad):
    """B1. -> vollstaendiger Text ODER ValueError. Nie '' bei fehlend/kaputt.
    Leere Datei (0 Bytes) -> '' (echter Zustand)."""
    try:
        roh = Path(pfad).read_bytes()
    except FileNotFoundError:
        raise ValueError(f"Datei nicht gefunden: {pfad}")
    except OSError as e:
        raise ValueError(f"Datei nicht lesbar: {pfad} ({e})")
    try:
        return roh.decode("utf-8-sig")
    except UnicodeDecodeError as e:
        raise ValueError(f"Datei nicht als UTF-8 dekodierbar: {pfad} ({e})")


# --- Schritt 2: VERARBEITUNG ----------------------------------------------
def verarbeite(text):
    """B2. Rein + idempotent. NFC-normalisieren, trimmen, GROSS.
    Hinweis: .upper() haengt an der Unicode-DB-Version des Interpreters."""
    return unicodedata.normalize("NFC", text).strip().upper()


# --- Schritt 3: OUTPUT (atomar) -----------------------------------------
def schreibe_atomar(pfad, text):
    """B3. Nach Return steht text VOLLSTAENDIG unter pfad. Kein Teil-Write:
    erst <pfad>.tmp schreiben + fsync, dann atomarer os.replace."""
    pfad = Path(pfad)
    tmp = pfad.with_suffix(pfad.suffix + ".tmp")
    with open(tmp, "w", encoding="utf-8", newline="") as f:
        f.write(text)
        f.flush()
        os.fsync(f.fileno())
    os.replace(tmp, pfad)  # atomar auf derselben Partition (Win + POSIX)


# --- Schritt 4: HASH ---------------------------------------------------
def hash_text(text):
    """B4. Reine Funktion. 64 Hex-Zeichen sha256 ueber die UTF-8-Bytes."""
    return hashlib.sha256(text.encode("utf-8")).hexdigest()


# --- Schritt 5: LOGGING (bedingt, corruption-sicher) -----------------
def letzter_hash(log_pfad):
    """B5a. -> None (Log fehlt/leer) | str (letzter Hash) | 'CORRUPT' (Log kaputt).
    None und 'CORRUPT' NIE verwechseln (sonst Bug-Muster 2)."""
    log_pfad = Path(log_pfad)
    if not log_pfad.exists():
        return None
    try:
        text = log_pfad.read_text(encoding="utf-8")
        zeilen = list(csv.DictReader(io.StringIO(text), delimiter=";"))
    except (OSError, UnicodeDecodeError, csv.Error):
        return "CORRUPT"
    if not zeilen:
        return None
    letzte = zeilen[-1]
    if set(LOG_SPALTEN) - set(letzte.keys()):
        return "CORRUPT"  # Header passt nicht
    return letzte.get("sha256")


def log_anhaengen(log_pfad, eintrag):
    """B5b. Eine Zeile anhaengen; Header nur beim ersten Mal."""
    log_pfad = Path(log_pfad)
    neu = not log_pfad.exists()
    with open(log_pfad, "a", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=LOG_SPALTEN, delimiter=";")
        if neu:
            w.writeheader()
        w.writerow(eintrag)


# --- Integration ------------------------------------------------------
def main():
    if len(sys.argv) != 4:
        print("Aufruf: python pipeline.py <input> <output> <log.csv>")
        sys.exit(1)
    input_pfad, output_pfad, log_pfad = map(Path, sys.argv[1:4])

    print("=== Pipeline gestartet ===")
    try:
        text = lies_text(input_pfad)
    except ValueError as e:
        print(f"[1/5] FEHLER: {e}")
        sys.exit(2)
    print(f"[1/5] Eingelesen: {len(text)} Zeichen")

    verarbeitet = verarbeite(text)
    print(f"[2/5] Verarbeitet: {len(verarbeitet)} Zeichen")

    schreibe_atomar(output_pfad, verarbeitet)
    geschrieben = lies_text(output_pfad)  # I0: die DATEI pruefen, nicht den Speicher
    if geschrieben != verarbeitet:
        print("[3/5] FEHLER: geschriebene Datei != Ergebnis")
        sys.exit(3)
    print(f"[3/5] Atomar geschrieben + zurueckgelesen: {output_pfad.name}")

    h = hash_text(geschrieben)
    print(f"[4/5] Hash der DATEI (sha256): {h}")

    last = letzter_hash(log_pfad)
    if last == "CORRUPT":
        backup = log_pfad.with_suffix(log_pfad.suffix + ".bak")
        os.replace(log_pfad, backup)
        print(f"[5/5] WARNUNG: Log war kaputt -> gesichert als {backup.name}, starte neu")
        last = None
    if last == h:
        print("[5/5] Skip: Hash bereits im Log, kein neuer Eintrag")
    else:
        log_anhaengen(log_pfad, {
            "timestamp": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
            "input_file": input_pfad.name,
            "output_file": output_pfad.name,
            "sha256": h,
            "status": "SUCCESS",
        })
        print(f"[5/5] Export: neuer Eintrag in {log_pfad.name}")

    print("=== Fertig ===")


if __name__ == "__main__":
    main()
