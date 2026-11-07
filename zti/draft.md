# Projekt ZTI — Draft & Planning Log

> **Modus:** Append-Only  
> **Axiom:** $\mathcal{T}(Z, I) \longrightarrow Z'$ (Zustand — Transformation — Information)  
> **Start:** 2026-10-24

---

## Log & Planungsverlauf

### [2026-10-24] #001 — Initialisierung & Planungsstart
- **Status:** Kickoff
- **Prinzip:** Alle künftigen Planungsstände, Architekturbausteine, Spezifikationen und Entscheidungen werden chronologisch im Append-Only-Verfahren an dieses Dokument angehängt.
- **Kernfokus ZTI:**
  - **Z (Zustand):** State, Memory, Register, Identität, Invarianz.
  - **T (Transformation):** Operatoren, Pipelines, Dispatcher, Transitionen ($O(1)$ / deterministisch).
  - **I (Information):** Reize, Payloads, Interrupts, Signale.

### [2026-10-24] #002 — Zielsetzung: Code-Refactoring & Go-Portierung (RAP-LAB)
- **Ziel & Ausrichtung:**
  - Schrittweises Refactoring bestehender Codeblöcke und sukzessive Umwandlung in idiomatischen Go-Code (Golang).
  - **Ent-Generifizierung:** Entfernung unnötiger generischer Abstraktionsebenen und Wrapper zugunsten maximaler Klarheit und Direktheit.
  - **Spezifischer Fokus:** Direkte Ausrichtung und Spezialisierung auf den konkreten Anwendungsfall **RAP-LAB**.
- **ZTI-Mapping:**
  - **Z (Zustand):** Konkrete, getypte Go-Structs für RAP-LAB statt generischer State-Container.
  - **T (Transformation):** Flache, deterministische Go-Funktionen und Pipelines ohne überflüssige Manager-Hierarchien.
  - **I (Information):** Präzise definierte Signale/Events/Payloads der RAP-LAB-Domäne.

### [2026-10-24] #003 — Domänenmodell & Kernfeatures: Hybrid aus Kleinanzeigen + Skool (RAP-CONNECT / RAP-LAB)
- **Konzept:**
  - Spezialisierter Hybrid für Musiker, Produzenten, Songwriter und Visual Artists (Rap/Hip-Hop-Fokus).
  - Plattform verbindet Marktplatz (dauerhafte Kleinanzeigen statt flüchtige 24h-Stories) mit Community/Gruppenbildung (Skool-Modell).
  - Claim/Funktion: *„RAP-CONNECT bringt alle zusammen.“*

- **Tech-Stack:**
  - **Backend:** Go (Golang) — schlank, typisiert, deterministisch.
  - **Datenbank:** PostgreSQL — relationale Integrität für Profile, Listings, Gruppen und Transaktionen.
  - **Frontend:** Noch offen / entkoppelt.

- **Kern-Säulen:**
  1. **Kleinanzeigen / Marktplatz:**
     - Niederschwellige Dienstleistungs- und Asset-Listings ohne Label-Zwang oder Follower-Hürde.
     - Kernkategorien:
       - *Mix & Master* (z. B. Fixed Price pro Track)
       - *Beatmaker / Beats* (Exclusives, Leases, Custom Productions)
       - *Songwriting / Ghostwriting* (Hooks, Parts)
       - *Visuals / Media* (Musikvideos: Konzept, Dreh, Schnitt; Cover-Artwork)
  2. **Community / Skool-Komponente:**
     - Gruppen erstellen, suchen, beitreten.
     - Kollaborationsräume für Tracks, Feedback und Projekte.

- **ZTI-Architektur-Mapping (Golang + PostgreSQL):**
  - **Z (Zustand):**
    - PostgreSQL Tabellen & korrespondierende Go-Structs:
      - `User` (Artist-/Provider-Profil, Skills, Socials)
      - `Listing` (Kategorie, Pricing, Beschreibung, Status: Active/Archived)
      - `Group` & `Membership` (Community-Spaces, Rollen)
  - **T (Transformation):**
    - Go-Service-Pipelines:
      - Marktplatz-Operatoren (`PublishListing`, `FilterListings`, `ContactProvider`)
      - Community-Operatoren (`CreateGroup`, `JoinGroup`, `PostInGroup`)
  - **I (Information):**
    - HTTP/JSON-Payloads, Event-Signale (z. B. `ListingCreatedEvent`, `MemberJoinedEvent`, Suchfilter-Parameter).
