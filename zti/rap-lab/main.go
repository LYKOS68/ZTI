package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ============================================================================
// 1. STATUSMODELL & INVARIANTEN
// ============================================================================

type ListingStatus string

const (
	StatusDraft  ListingStatus = "DRAFT"
	StatusActive ListingStatus = "ACTIVE"
	StatusSold   ListingStatus = "SOLD"
)

var allowedTransitions = map[ListingStatus][]ListingStatus{
	StatusDraft:  {StatusActive},
	StatusActive: {StatusDraft, StatusSold},
	StatusSold:   {}, // Terminaler Zustand: Ein SOLD-Listing kann nicht reaktiviert werden
}

func ValidateTransition(from, to ListingStatus) error {
	targets, ok := allowedTransitions[from]
	if !ok {
		return fmt.Errorf("ungültiger Ausgangsstatus: %s", from)
	}
	for _, target := range targets {
		if target == to {
			return nil
		}
	}
	return fmt.Errorf("unzulässiger Statusübergang von %s nach %s", from, to)
}

// ============================================================================
// 2. DOMÄNENTYPEN
// ============================================================================

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Status    string    `json:"status"` // "ACTIVE", "SUSPENDED"
	CreatedAt time.Time `json:"created_at"`
}

type Profile struct {
	UserID      string    `json:"user_id"`
	DisplayName string    `json:"display_name"`
	Instruments []string  `json:"instruments"`
	Genres      []string  `json:"genres"`
	Location    string    `json:"location"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Listing struct {
	ID          string        `json:"id"`
	OwnerID     string        `json:"owner_id"`
	Title       string        `json:"title"`
	Description string        `json:"description"`
	PriceCents  int64         `json:"price_cents"`
	Status      ListingStatus `json:"status"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// ============================================================================
// 3. EINGABEPRÜFUNG & VALIDIERUNG
// ============================================================================

func ValidateUser(u *User) error {
	if strings.TrimSpace(u.ID) == "" {
		return errors.New("user ID darf nicht leer sein")
	}
	if !strings.Contains(u.Email, "@") || len(u.Email) < 5 {
		return fmt.Errorf("ungültige E-Mail-Adresse: %s", u.Email)
	}
	return nil
}

func ValidateProfile(p *Profile) error {
	if strings.TrimSpace(p.UserID) == "" {
		return errors.New("profil benötigt eine gültige UserID")
	}
	if len(strings.TrimSpace(p.DisplayName)) < 2 {
		return errors.New("display_name muss mindestens 2 Zeichen lang sein")
	}
	return nil
}

func ValidateListing(l *Listing) error {
	if strings.TrimSpace(l.ID) == "" {
		return errors.New("listing ID darf nicht leer sein")
	}
	if strings.TrimSpace(l.OwnerID) == "" {
		return errors.New("listing muss einen berechtigten Eigentümer (OwnerID) besitzen")
	}
	title := strings.TrimSpace(l.Title)
	if len(title) < 3 || len(title) > 120 {
		return fmt.Errorf("titel muss zwischen 3 und 120 Zeichen lang sein (aktuell: %d)", len(title))
	}
	if l.PriceCents < 0 {
		return fmt.Errorf("preis darf nicht negativ sein (aktuell: %d Cents)", l.PriceCents)
	}
	switch l.Status {
	case StatusDraft, StatusActive, StatusSold:
		return nil
	default:
		return fmt.Errorf("unbekannter Listing-Status: %s", l.Status)
	}
}

// ============================================================================
// 4. ZTI-ERGEBNISMODELLE
// ============================================================================

type GateDecision string

const (
	DecisionCommit GateDecision = "COMMIT"
	DecisionDrop   GateDecision = "DROP"
)

type ZTIResult[T any] struct {
	Decision GateDecision `json:"decision"`
	From     T            `json:"from"`
	To       T            `json:"to"`
	Reason   string       `json:"reason,omitempty"`
	Error    error        `json:"-"`
}

// ============================================================================
// 5. TRANSAKTIONSABLAUF & ZTI-GATE (FÜR LISTINGS)
// ============================================================================

type UpdateListingCommand struct {
	CallerUserID string
	NewTitle     *string
	NewDesc      *string
	NewPrice     *int64
	NewStatus    *ListingStatus
}

// ZTIGateProcess führt die atomare Prüfung Z0 -> T -> I -> S/Gate -> Z1 aus.
func ZTIGateProcess(z0 Listing, cmd UpdateListingCommand) ZTIResult[Listing] {
	// 1. Eigentümerprüfung (Invariante)
	if cmd.CallerUserID != z0.OwnerID {
		return ZTIResult[Listing]{
			Decision: DecisionDrop,
			From:     z0,
			To:       z0,
			Reason:   "DROP: Aufrufer ist nicht der berechtigte Eigentümer des Listings",
			Error:    errors.New("unauthorized"),
		}
	}

	// 2. Transformation berechnen (T)
	z1 := z0
	z1.UpdatedAt = time.Now().UTC()

	if cmd.NewTitle != nil {
		z1.Title = *cmd.NewTitle
	}
	if cmd.NewDesc != nil {
		z1.Description = *cmd.NewDesc
	}
	if cmd.NewPrice != nil {
		z1.PriceCents = *cmd.NewPrice
	}

	// 3. Statusübergang prüfen
	if cmd.NewStatus != nil && *cmd.NewStatus != z0.Status {
		if err := ValidateTransition(z0.Status, *cmd.NewStatus); err != nil {
			return ZTIResult[Listing]{
				Decision: DecisionDrop,
				From:     z0,
				To:       z0,
				Reason:   fmt.Sprintf("DROP: %v", err),
				Error:    err,
			}
		}
		z1.Status = *cmd.NewStatus
	}

	// 4. Validierungsprüfung auf Zielzustand Z1
	if err := ValidateListing(&z1); err != nil {
		return ZTIResult[Listing]{
			Decision: DecisionDrop,
			From:     z0,
			To:       z0,
			Reason:   fmt.Sprintf("DROP: Ungültiger Zielzustand (%v)", err),
			Error:    err,
		}
	}

	// 5. Freigabe: COMMIT
	return ZTIResult[Listing]{
		Decision: DecisionCommit,
		From:     z0,
		To:       z1,
		Reason:   "COMMIT: Validierung und Rechteprüfung erfolgreich",
		Error:    nil,
	}
}

// ============================================================================
// 6. SCHNITTSTELLEN FÜR POSTGRESQL & SPEICHERUNG
// ============================================================================

type Store interface {
	GetUser(ctx context.Context, id string) (*User, error)
	GetListing(ctx context.Context, id string) (*Listing, error)
	CommitListing(ctx context.Context, res ZTIResult[Listing]) error
}

type AppConfig struct {
	HTTPPort    string
	DatabaseURL string
	Environment string
}

// ============================================================================
// 7. HTTP-ROUTEN & SERVERSTART
// ============================================================================

func NewRouter() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"UP","time":"%s"}`, time.Now().UTC().Format(time.RFC3339))
	})
	return mux
}

// ============================================================================
// 8. EINSTIEGSPUNKT & BEISPIELLAUF
// ============================================================================

func main() {
	fmt.Println("=== RAP-LAB Core Initialisierung ===")

	// 1. Initialer Zustand: Neues Listing als DRAFT
	user := User{
		ID:        "usr_producer_01",
		Email:     "producer@rap-lab.local",
		Status:    "ACTIVE",
		CreatedAt: time.Now().UTC(),
	}

	listing := Listing{
		ID:          "lst_beat_808",
		OwnerID:     user.ID,
		Title:       "Dark Trap Beat - 140 BPM",
		Description: "Exklusivlizenz, WAV Stems inklusive",
		PriceCents:  5000, // 50,00 €
		Status:      StatusDraft,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	fmt.Printf("[Z0] Listing initialisiert: ID=%s Status=%s Preis=%d Cents\n",
		listing.ID, listing.Status, listing.PriceCents)

	// 2. Gültiger Übergang: DRAFT -> ACTIVE
	pubStatus := StatusActive
	res1 := ZTIGateProcess(listing, UpdateListingCommand{
		CallerUserID: user.ID,
		NewStatus:    &pubStatus,
	})
	fmt.Printf("[ZTI-1] Aktion 'Veröffentlichen': Decision=%s -> Neuer Status=%s (%s)\n",
		res1.Decision, res1.To.Status, res1.Reason)

	// 3. Ungültiger Übergang: Fremder Nutzer versucht Änderung -> DROP
	res2 := ZTIGateProcess(res1.To, UpdateListingCommand{
		CallerUserID: "usr_attacker_99",
		NewPrice:     ptr(int64(1000)),
	})
	fmt.Printf("[ZTI-2] Fremder Zugriff: Decision=%s -> Status=%s (%s)\n",
		res2.Decision, res2.To.Status, res2.Reason)

	// 4. Gültiger Verkauf: ACTIVE -> SOLD
	soldStatus := StatusSold
	res3 := ZTIGateProcess(res1.To, UpdateListingCommand{
		CallerUserID: user.ID,
		NewStatus:    &soldStatus,
	})
	fmt.Printf("[ZTI-3] Aktion 'Verkaufen': Decision=%s -> Neuer Status=%s (%s)\n",
		res3.Decision, res3.To.Status, res3.Reason)

	// 5. Verbotener Reaktivierungsversuch: SOLD -> ACTIVE -> DROP
	res4 := ZTIGateProcess(res3.To, UpdateListingCommand{
		CallerUserID: user.ID,
		NewStatus:    &pubStatus,
	})
	fmt.Printf("[ZTI-4] Reaktivierung aus SOLD: Decision=%s -> Status=%s (%s)\n",
		res4.Decision, res4.To.Status, res4.Reason)

	fmt.Println("=== RAP-LAB Core erfolgreich verifiziert ===")
}

func ptr[T any](v T) *T {
	return &v
}
