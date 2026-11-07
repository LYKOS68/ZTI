"""loop.py - kleinster 4-Agenten-Loop als ZTIS-Instanz.

Die kanonischen Rollen sind Alpha=Zustand, Bravo=Transformation,
Charlie=Information und Delta=Syncpunkt. Die historischen Klassennamen und
ihre bestehende Zuordnung bleiben eingefroren; dieser Ablauf dokumentiert
nur die kanonische Bedeutung des gemeinsamen Objekts.

  ZUSTAND        SharedState (data, tick, last_pulse, status) + konsistent()
  TRANSFORMATION AlphaGenerator - rohe Eingabe -> Payload (historische Benennung)
  INFORMATION    BravoValidator - Payload -> genau ein Bit 0/1, SCHEMA-geprueft
  SYNCPUNKT      DeltaExecutor/CharlieGuard - Zustand binden oder validiert zurücksetzen

Harte Kette: WAKE -> Pre-Wake-Check -> Z -> T -> I -> S/Gate -> neuer Zustand

Fixes ggue. Entwurf: Bravo = echtes Schema-Gate (nicht Always-Pass); Charlie = validierter
RESET mit Post-Heal-Check; Pre-Wake-Check in step(); Uhr injizierbar (now_fn) fuer Tests.
"""

import time
from typing import Any, Callable, Optional

GUELTIGE_STATUS = {"SLEEP", "WAKE", "UPDATED", "CLEAN"}


# --- 1. ZUSTAND ------------------------------------------------------
class SharedState:
    def __init__(self):
        self.data: Optional[Any] = None
        self.tick: int = 0
        self.last_pulse: float = 0.0
        self.status: str = "SLEEP"

    def konsistent(self) -> bool:
        """Zustands-Invariante: Feldtypen und -werte im erlaubten Bereich."""
        return (
            isinstance(self.tick, int) and self.tick >= 0
            and isinstance(self.last_pulse, (int, float)) and not isinstance(self.last_pulse, bool)
            and self.status in GUELTIGE_STATUS
        )


# --- 2. TRANSFORMATION ---------------------------------------------
class AlphaGenerator:
    def process(self, raw_input: Optional[Any]) -> Any:
        return raw_input if raw_input is not None else "PING"


# --- 3. INFORMATION (liefert die Gate-Information) ----------------
class BravoValidator:
    BLOCKLISTE = {"INVALID", "CORRUPT"}
    MAX_LEN = 4096

    def process(self, payload: Any) -> int:
        """Payload -> genau ein Bit. 1 nur bei: str, getrimmt nicht-leer,
        nicht in Blockliste, <= MAX_LEN. Alles andere -> 0."""
        if not isinstance(payload, str):
            return 0
        p = payload.strip()
        if not p or p in self.BLOCKLISTE or len(p) > self.MAX_LEN:
            return 0
        return 1


# --- 4. SYNCPUNKT: erfolgreiche Bindung ---------------------------
class DeltaExecutor:
    def process(self, payload: Any, state: SharedState) -> None:
        state.data = payload
        state.status = "UPDATED"


# --- 5. SYNCPUNKT: validierende Rückkopplung ----------------------
class CharlieGuard:
    def process(self, state: SharedState) -> dict:
        alt = {"data": state.data, "tick": state.tick, "status": state.status}
        state.data = None
        if not (isinstance(state.tick, int) and state.tick >= 0):
            state.tick = 0
        if not (isinstance(state.last_pulse, (int, float)) and not isinstance(state.last_pulse, bool)):
            state.last_pulse = 0.0
        state.status = "CLEAN"
        if not state.konsistent() or state.data is not None:      # Post-Heal-Validierung
            raise RuntimeError(f"RESET fehlgeschlagen, Zustand inkonsistent (war: {alt!r})")
        return {"reset_von": alt}


# --- Loop ------------------------------------------------------
class ChronoLoop:
    def __init__(self, interval_sec: float = 1.0, now_fn: Callable[[], float] = time.time):
        self.state = SharedState()
        self.alpha = AlphaGenerator()
        self.bravo = BravoValidator()
        self.charlie = CharlieGuard()
        self.delta = DeltaExecutor()
        self.interval = interval_sec
        self._now = now_fn

    def trigger(self, has_incoming_data: bool) -> bool:
        now = self._now()
        time_elapsed = (now - self.state.last_pulse) >= self.interval
        if time_elapsed or has_incoming_data:
            self.state.status = "WAKE"
            self.state.last_pulse = now
            return True
        self.state.status = "SLEEP"
        return False

    def step(self, raw_input: Optional[Any] = None) -> str:
        # ZUSTAND + Chrono-Trigger davor
        if not self.trigger(raw_input is not None):
            return "SLEEP"

        # Pre-Wake-Check: erst handeln, wenn der Zustand konsistent ist
        if not self.state.konsistent():
            self.charlie.process(self.state)
            return f"TICK {self.state.tick}: PRE-WAKE RESET (Zustand war inkonsistent)"

        self.state.tick += 1

        payload = self.alpha.process(raw_input)          # TRANSFORMATION
        bit = self.bravo.process(payload)                # INFORMATION -> genau ein Bit

        if bit == 0:                                     # S: validierende Rückkopplung
            self.charlie.process(self.state)
            return f"TICK {self.state.tick}: RESET (BIT=0)"

        self.delta.process(payload, self.state)          # S: erfolgreiche Bindung
        return f"TICK {self.state.tick}: EXECUTE (BIT=1) -> State: {self.state.data!r}"


if __name__ == "__main__":
    # deterministischer Lauf mit kontrollierter Uhr
    t = [0.0]
    loop = ChronoLoop(interval_sec=0.5, now_fn=lambda: t[0])
    for idx, inp in enumerate([None, None, "VALID_DATA", None, "INVALID", None]):
        res = loop.step(raw_input=inp)
        print(f"[Step {idx + 1}] t={t[0]:.1f} Status: {loop.state.status:<7} | {res}")
        t[0] += 0.3
