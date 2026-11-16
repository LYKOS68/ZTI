# ENTWICKLUNGSVERTRAG — REPOSITORY ZTI

> **Geltungsbereich:** Dieses Dokument konstituiert das oberste, unverletzliche Grundgesetz für alle Entwicklungs-, Wartungs- und Refactoring-Prozesse innerhalb dieses Repositories. Es bindet alle Mitwirkenden, Automatisierungen, Werkzeuge und KI-Agenten gleichermaßen.

---

## § 1 Oberstes Gesetz: Absolute Isolation des Altbestands (`__historical-backup/`)

1. **Strikte Quarantäne:**  
   Das Verzeichnis `__historical-backup/` dient ausschließlich der historischen Sicherung des ursprünglichen Projektbestands vor dem Clean-up. Es ist kein aktiver Bestandteil der Codebase.
2. **Absolutes Zugriffsverbot:**  
   Jeglicher automatisierte, spekulative oder präventive Lese-, Schreib-, Such-, Indexierungs-, Import- oder Referenzierungszugriff auf `__historical-backup/` ist ausnahmslos verboten.
3. **Ausnahme ausschließlich bei expliziter Nutzeranweisung:**  
   Ein Zugriff auf Inhalte innerhalb von `__historical-backup/` ist **ausschließlich** dann gestattet, wenn der Benutzer im Prompt eine ausdrückliche, spezifische und unmissverständliche Aufforderung formuliert hat (z. B. *„Lies Datei X aus __historical-backup“*). Liegt keine solche explizite Aufforderung vor, ist das Verzeichnis wie eine nicht existierende Entität zu behandeln.
4. **Git-Ausschluss:**  
   `__historical-backup/` verbleibt rein lokal und darf unter keinen Umständen in das Versionskontrollsystem (Git) eingebracht oder dorthin übertragen werden.

---

## § 2 Oberstes Gesetz: Root-Minimalismus & Verzeichnisordnung

1. **Zulässiger Root-Bestand:**  
   Im Wurzelverzeichnis (Root) des Repositories sind permanent exakt drei Basiseinträge zulässig:
   - `__historical-backup/` *(lokaler isolierter Altbestand)*
   - `.gitignore` *(Ausschlussregeln)*
   - `ENTWICKLUNGSVERTRAG.md` *(vorliegendes Verfassungsdokument)*
2. **Strikte Strukturierung von Neuentwicklungen:**  
   Jeder künftige Code, alle Module, Bibliotheken, Tests und Dokumentationen müssen in zweckgebundenen, klar definierten Unterverzeichnissen strukturiert werden.
3. **Keine lose Ablage im Root:**  
   Das Ablegen temporärer Notizen, Entwürfe, fliegender Skripte, Logdateien oder Artefakte im Root-Verzeichnis ist untersagt.

---

## § 3 Architektur- und Implementierungsprinzipien (ZTI)

1. **ZTI-Grundformel:**  
   Das System folgt dem Axiom:  
   $$\mathcal{T}(Z, I) \longrightarrow Z'$$  
   - **Z (Zustand):** Explizit typisiert, invariant, kontrolliert und atomar verwaltet.
   - **T (Transformation):** Deterministische, flache und nachvollziehbare Operatoren ohne unnötige Abstraktionsschichten.
   - **I (Information):** Präzise definierte Signale, Ereignisse und Payloads.
2. **Direktheit vor Hyperabstraktion:**  
   Konkrete, funktionale und idiomatische Lösungen haben strikten Vorrang vor generischem Over-Engineering oder ungenutzten Wrapper-Hierarchien.
3. **Deterministisches Fehlerverhalten:**  
   Fehler sind explizit abzufangen und zu typisieren; implizite Ausnahmezustände oder unkontrollierte Nebeneffekte sind unzulässig.

---

## § 4 Revisionskontrolle & Commit-Hygiene

1. **Atomare Commits:**  
   Änderungen sind in inhaltlich geschlossenen, logischen Einheiten zu committen.
2. **Semantische Konventionen:**  
   Commit-Nachrichten folgen ausnahmslos der semantischen Konvention (`feat:`, `fix:`, `refactor:`, `docs:`, `test:`, `chore:`).
3. **Keine Artefakte im Index:**  
   Binärdateien, temporäre Caches, Testoutputs oder sensible Zugangsdaten dürfen niemals in den Git-Index gelangen.

---

## § 5 Identitäts- und Wahrheitsregel

1. **Projekt = Wahrheit:**  
   Dieses Repository und die darin deklarierten Verträge bilden die einzige Quelle der Wahrheit (Single Source of Truth).
2. **Vorrang:**  
   Bei Konflikten zwischen externen Annahmen, Modellen oder historischen Codefragmenten hat der Wortlaut dieses Entwicklungsvertrags absolute Priorität.
