#!/usr/bin/env python3
"""
zti_runner.py — Dünne Verbindung zwischen loop.py und engine.py.
Erzwingt strikt: Z0 → T → I → S/Delta → Z1 mit JSON-Zustandsprotokollierung.

Die 3 geforderten Fälle:
  1. LEERLAUF   -> Bit 0, Byte-Gleichheit der Bibliothek (lib_hash_nach == lib_hash_vor)
  2. SCHREIBEN  -> Bit 1, monotones Wachstum der Bibliothek (lib_len_nach > lib_len_vor)
  3. STÖRFALL   -> Interferenz / Reset / Fehlernachweis, Zustand heilt zu CLEAN
"""

import sys
import os
import json
from pathlib import Path
from typing import Optional, Dict, Any, Callable

BASE_DIR = Path(__file__).resolve().parent
p = BASE_DIR
while p != p.parent:
    if (p / "src").exists() or (p / "01_spezifikationen").exists():
        PROJECT_ROOT = p
        break
    p = p.parent
else:
    PROJECT_ROOT = BASE_DIR.parent.parent

SRC_DIR = PROJECT_ROOT / "src"

sys.path.insert(0, str(SRC_DIR / "03_kern-engine"))
sys.path.insert(0, str(SRC_DIR / "04_chrono-loop"))
sys.path.insert(0, str(BASE_DIR))

from engine import kern_5, lies_text, sha256, stub_modell
from loop import ChronoLoop, SharedState


def capture_zustand(datei_pfad: str, loop: ChronoLoop) -> Dict[str, Any]:
    """Erfasst den messbaren Systemzustand (Z)."""
    try:
        inhalt = lies_text(datei_pfad)
        lib_hash = sha256(inhalt)
        lib_len = len([z for z in inhalt.splitlines() if z.strip()])
        lib_ok = True
    except Exception as e:
        inhalt = ""
        lib_hash = None
        lib_len = -1
        lib_ok = False

    return {
        "lib_hash": lib_hash,
        "lib_len": lib_len,
        "lib_ok": lib_ok,
        "loop_tick": loop.state.tick,
        "loop_status": loop.state.status,
        "loop_data": loop.state.data,
        "loop_konsistent": loop.state.konsistent()
    }


def zti_step(
    datei_pfad: str,
    loop: ChronoLoop,
    neuer_fakt: Optional[str] = None,
    bedarf: str = "",
    relevanz: str = "",
    modell_fn: Callable[[str], str] = stub_modell
) -> Dict[str, Any]:
    """
    Führt exakt einen ZTIS-Zyklus aus:
      Z0 (Messen) → T (Loop-Step + Kern-Engine) → I (Information)
      → S/Delta (Gate, Zustandswirkung oder Rückkopplung) → Z1 (Messen).
    """
    # 1. Z0: Ausgangszustand erfassen
    z0 = capture_zustand(datei_pfad, loop)

    # 2. T: Transformation anstoßen
    has_input = (neuer_fakt is not None)
    trigger_ok = loop.trigger(has_input)

    if not trigger_ok:
        # Reiner SLEEP-Fall (Zeitintervall nicht erreicht, kein Input)
        z1 = capture_zustand(datei_pfad, loop)
        return {
            "fall": "SLEEP",
            "bit": 0,
            "z0": z0,
            "t": {"trigger": False},
            "i": {"gate": "SLEEP"},
            "delta": {"geschrieben": False},
            "z1": z1,
            "pass": (z1["lib_hash"] == z0["lib_hash"])
        }

    # Pre-Wake Check
    if not loop.state.konsistent():
        loop.charlie.process(loop.state)
        z1 = capture_zustand(datei_pfad, loop)
        return {
            "fall": "PRE_WAKE_RESET",
            "bit": 0,
            "z0": z0,
            "t": {"reset": True},
            "i": {"gate": "INKONSISTENT"},
            "delta": {"geschrieben": False},
            "z1": z1,
            "pass": (z1["loop_status"] == "CLEAN")
        }

    loop.state.tick += 1

    # Kern-5 Ausführung
    try:
        fakt_input = neuer_fakt if neuer_fakt else "TICK_BEAT"
        ang, kontext, block, herkunft, ergebnis, rueck = kern_5(
            datei_pfad=datei_pfad,
            neuer_fakt=fakt_input,
            bedarf=bedarf,
            relevanz=relevanz,
            modell_fn=modell_fn
        )

        # 3. I: Information für den Syncpunkt/Gate bestimmen
        # Bit = 1 gdw. rueckschreibung erfolgreich ein Delta geschrieben hat
        geschrieben = rueck.get("geschrieben", False)
        bit = 1 if geschrieben else 0

        # 4. S/Delta: Zustand binden oder validierend rückkoppeln
        # loop_bit = Bravo-Schema-Gate auf der Loop-Seite (unabhaengig vom Bibliotheks-Gate
        # in rueckschreibung()). Bibliothek ist bei bit==1 bereits geschrieben (kern_5/kern_1);
        # loop_bit entscheidet NUR, ob der ChronoLoop-Zustand das als UPDATED uebernimmt.
        loop_bit = None
        if bit == 1:
            loop_bit = loop.bravo.process(rueck.get("erkenntnis"))
            if loop_bit == 1:
                loop.delta.process(rueck.get("erkenntnis"), loop.state)
            else:
                loop.charlie.process(loop.state)
        else:
            # Bei Bit 0 (keine gültige Rückschreibung oder Duplikat) greift die
            # validierende Rückkopplung zu CLEAN
            # Zustand wird nicht beschädigt, bleibt konsistent
            if not neuer_fakt: # Reiner Leerlauf
                loop.state.status = "CLEAN"
            else:
                loop.state.status = "CLEAN"

        # 5. Z1: Endzustand erfassen
        z1 = capture_zustand(datei_pfad, loop)

        # Prüf-Bedingungen (präzisiert 2026-09-05, Freigabe Roman):
        # "bit" beschreibt NUR das Rueckschreibungs-Gate (geschrieben), nicht die gesamte
        # Bibliothek. kern_1 speichert einen NEUEN Rohfakt (ang) unabhaengig davon, ob
        # rueckschreibung() spaeter eine abgeleitete Erkenntnis akzeptiert (engine.py:
        # kern_2 -> kern_1, vor jedem Gate). Reale Bibliotheksveraenderung haengt daher von
        # BEIDEN Groessen ab, nicht nur von bit:
        lib_veraenderung_erwartet = ang or geschrieben
        if lib_veraenderung_erwartet:
            bestanden = (z1["lib_len"] > z0["lib_len"]) and (z1["lib_hash"] != z0["lib_hash"])
        else:
            bestanden = (z1["lib_hash"] == z0["lib_hash"])

        return {
            "fall": "ERFOLG" if bit == 1 else "LEERLAUF_ODER_SKIP",
            "bit": bit,
            "z0": z0,
            "t": {
                "neuer_fakt": fakt_input,
                "rohfakt_gespeichert": ang,
                "bedarf": bedarf,
                "ergebnis": ergebnis
            },
            "i": {
                "gate_bit": bit,
                "loop_bit": loop_bit,
                "rueckschreibung": rueck
            },
            "delta": {
                "geschrieben": geschrieben,
                "state_status": loop.state.status
            },
            "z1": z1,
            "pass": bestanden
        }

    except Exception as e:
        # STÖRFALL innerhalb von S: validierte Rückkopplung zu CLEAN
        loop.charlie.process(loop.state)
        z1 = capture_zustand(datei_pfad, loop)
        return {
            "fall": "STOERFALL",
            "bit": 0,
            "fehler": str(e),
            "z0": z0,
            "t": {"exception": type(e).__name__},
            "i": {"gate": "ERROR"},
            "delta": {"geschrieben": False, "reset": True},
            "z1": z1,
            "pass": (z1["loop_status"] == "CLEAN" and z1["loop_konsistent"])
        }
