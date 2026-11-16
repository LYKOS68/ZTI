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

---

## § 6 Layer Order (Deterministische Ausführungskaskade)

1. **Unverletzliche Reihenfolge:**  
   Jede Handlung im Repository durchläuft ausnahmslos die Kaskade:  
   $$\text{Policy} \longrightarrow \text{Role} \longrightarrow \text{Strategy} \longrightarrow \text{Task} \longrightarrow \text{Tool} \longrightarrow \text{Execution}$$
2. **Physische Bestimmung der Schichten:**  
   - **Policy:** UTF-8-Vertragsdatei (`ENTWICKLUNGSVERTRAG.md`) und Filterregeln (`.gitignore`), die Dateisystem- und Verhaltensgrenzen mit harten Exit-Codes deklarieren.
   - **Role:** System-Konfiguration und Prompt-Direktive, die den funktionalen Handlungsrahmen und erlaubte Arbeitsverzeichnisse setzt.
   - **Strategy:** Strukturierter Ausführungsplan als geordnete Sequenz konkreter Zieldateipfade und Komponenten.
   - **Task:** Diskrete, atomare Arbeitseinheit mit definierten Eingangs- und Ausgangsdaten für genau eine Zieldatei oder Komponente.
   - **Tool:** Minimal notwendiger Funktions- oder CLI-Aufruf (`write_file`, Shell-Kommando, Go-Compiler) ohne Kompetenzübertragung.
   - **Execution:** Laufender Betriebssystem-Prozess mit messbarem Exit-Code ($0$ oder $\neq 0$) und verifizierter Dateisystem-Mutation.
3. **Kaskaden-Invarianz:**  
   Eine nachgelagerte Schicht darf einer vorgelagerten Schicht niemals widersprechen. Bei Konflikten bricht der Prozess sofort ohne Mutation ab.

---

## § 7 Sprach- und Definitionsregeln (Physisches Bindungsgebot)

1. **Physische Lokalisierung:**  
   Jeder technische Begriff muss an eine konkrete physische Entität gebunden sein (Dateiformat, relativer Dateipfad, Go-Struct, Betriebssystem-Prozess). Abstrakte Worthülsen ohne physische Entsprechung sind unzulässig.
2. **Pflicht-Attribute jeder Begriffsdefinition:**  
   Jede Systemkomponente (z. B. Engine, Algorithmus, Parser) muss zwingend vier Attribute deklarieren:
   - *Speicherort / Dateiformat:* Konkretes Verzeichnis und Dateiendung.
   - *Akteur / Konsument:* Wer führt aus, wer liest (Mensch, CLI/KI, Tooling).
   - *Typisierte I/O:* Exakte Eingangsdatenstrukturen und Ausgangsdaten.
   - *Fehlerverhalten:* Numerischer Exit-Code und deterministischer Fehler-Typ.
3. **Konkretisierung Engine & Algorithmus:**  
   - *Engine:* Physisch ein Go-Paket in einem dedizierten Unterordner (`pkg/<name>/`) mit getypter Struct und einer exportierten Einstiegsfunktion `Execute(...)`.
   - *Leitender Algorithmus:* Die zentrale Kontrollflussfunktion in `engine.go`.
   - *Unteralgorithmen:* Unexportierte Hilfsfunktionen in separaten `.go`-Dateien desselben Ordners, die reine In-Memory-Transformationen berechnen.

---

## § 8 Empfänger- und Rollenmatrix für Dateiformate

1. **Verbindliche Format- und Konsumenten-Zuordnung:**
   - **`.md` — Mensch + Agentensteuerung:**  
     *Konsument:* Menschliche Entwickler und KI-Agenten.  
     *Zweck:* Architektur, Gesetze, Regeln, Entscheidungen, Anweisungen, lesbare Übergaben und Schnittstellendokumentation. Keine ausführbare Programmlogik.
   - **`.jsonl` — Maschinenprotokoll (Projektgedächtnis):**  
     *Konsument:* Nachfolgende CLI- und KI-Modelle zur Selbstreparatur, Wissensübergabe und Kontextrekonstruktion.  
     *Zweck:* Chronologische Ereignisse, Tool-Ergebnisse, Fehlerprotokolle, Abbrüche und Wiederherstellungsinformationen.  
     *Besonderheit:* Inkrementelle maschinelle Verarbeitung im Append-Only-Verfahren mit versioniertem Schema, Zeitstempeln und eindeutigen IDs.
   - **`.json` / `.yaml` — Strukturierte Konfiguration & Schnittstellenschemata:**  
     *Konsument:* Compiler, Linters, Parser und automatisierte Build-Tools.  
     *Zweck:* Feste Schemadefinitionen, statische Konfigurationen und deterministische Typen ohne Programmlogik.
   - **`.csv` — Numerische Messreihen & Benchmarks:**  
     *Konsument:* Analyse-Tools, Benchmarking-Pipelines und menschliche Auditierung.  
     *Zweck:* Tabellarische Leistungsdaten, Testmetriken und Profiling-Ergebnisse.
   - **`.go` / `*_test.go` — Ausführbare Programmlogik & Verifikation:**  
     *Konsument:* Go-Compiler, Runtime und Test-Runner (`go test`).  
     *Zweck:* Deterministische Transformationen ($\mathcal{T}$), Struct-Definitionen ($Z$) und automatisierte Invarianten-Prüfungen mit Exit-Code 0/1.
2. **Anti-Ballast-Gesetz:**  
   - Keine redundanten Dokumentationen oder doppelten Datenpfade.
   - Keine ungenutzten oder undeklarierten Dateiformate im Repository.
   - Jede Datei im Repository muss einen im Vertrag verankerten Konsumenten und eindeutigen operativen Zweck besitzen.

---

## § 9 Übergeordnetes Gesetz: Terminologische Einheitlichkeit & Inbound/Output-Schema (Terminological Consistency Law)

1. **Gesetz der terminologischen Einheitlichkeit (Terminological Consistency Law):**  
   - **Kanonischer Bezeichner:** Jedes Konzept, jeder Zustand ($Z$), jeder Datentyp, jedes Datenfeld und jede Transformation ($T$) besitzt im gesamten Repository (Code, Tests, Dokumente, Schemata) exakt einen einzigen, unveränderlichen kanonischen Namen.
   - **Absolutes Synonymverbot:** Die Verwendung von Pseudonymen, abweichenden Aliassen, semantischen Varianten oder lokalen Übersetzungen für dieselbe Entität ist über alle Schichten hinweg untersagt.
   - **Synchronisationspflicht:** Eine Umbenennung darf niemals isoliert in einer Datei stattfinden, sondern erfordert ein atomares Update über Vertrag, Typdefinitionen und Tests hinweg.
2. **Inbound- und Output-Schema (Deterministische Schnittstellengarantie):**  
   - **Striktes Inbound-Schema:** Jede Engine, Funktion und Pipeline akzeptiert ausschließlich typisierte, vorab deklarierte Eingabedaten. Ungetypte Wildcards (`any`, `interface{}` ohne Validierung) oder implizite Annahmen sind als Inbound verboten.
   - **Deterministisches Output-Schema:** Jede Operation liefert ausnahmslos entweder ein vollständig schema-konformes Datenobjekt oder einen typisierten Fehlercode zurück.
   - **Fail-Fast-Validierung:** Entspricht ein Inbound-Reiz nicht exakt dem definierten Schema, bricht die Kaskade vor der Weiterverarbeitung mit Exit-Code $\neq 0$ und strukturiertem Fehlereintrag im Maschinenprotokoll ab.
