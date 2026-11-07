package main

import (
	"fmt"
)

var (
	ErrProvenanceBroken  = fmt.Errorf("zti: provenancebruch - transformiertes substrat stammt nicht aus erfasstem ursprung")
	ErrChainNotCoherent  = fmt.Errorf("zti: inkohärentes zti-schema (pruefe/bind gehoeren nicht zum gleichen tripel)")
	ErrMissingPrecursor  = fmt.Errorf("zti: ohne z kein t - form/einsatz ohne vorheriges erfasse")
	ErrCheckWithoutForm  = fmt.Errorf("zti: pruefe ohne vorangehende forme im gleichen tripel")
	ErrCommitWithoutTrip = fmt.Errorf("zti: bind ohne vollstaendiges z-t-i-triplet")
)

type chainInfo struct {
	Origin   string            // ident aus erstem ERFASSE
	HasZ     bool
	HasT     bool
	HasI     bool
	Derived  map[string]string // dst -> src
	Members  map[string]bool   // alle register die zu diesem tripel gehoeren
	TransDst string            // das register aus FORME (ziel)
	Checked  string            // register aus PRÜFE
	Committed bool
}

type Validator struct{}

func NewValidator() *Validator {
	return &Validator{}
}

func (v *Validator) Validate(p *Program) error {
	chains := map[string]*chainInfo{}
	derived := map[string]string{} // global dst->src

	for _, stmt := range p.Statements {
		switch s := stmt.(type) {
		case *ReadStmt:
			// ERFASSE begruendet ein neues kohärentes Tripel
			c := &chainInfo{
				Origin:  s.Ident,
				HasZ:    true,
				HasT:    false,
				HasI:    false,
				Derived: map[string]string{},
				Members: map[string]bool{s.Ident: true},
			}
			chains[s.Ident] = c
			derived[s.Ident] = "" // wurzel

		case *TransformStmt:
			// FORME VON <Src> NACH <Dst> MIT <Op> - muss aus bekanntem substrat ableiten
			src := s.Src
			dst := s.Dst

			// Selbsttransformation (Inplace) ist erlaubt wenn src bekannt ist
			if src == dst {
				root := findRoot(src, derived)
				if root == "" {
					if _, ok := derived[src]; !ok {
						return fmt.Errorf("%w: '%s' nicht aus erfasse ableitbar", ErrMissingPrecursor, src)
					}
					// könnte direkt root sein
					if _, ok := chains[src]; ok {
						root = src
					}
				}
				if root == "" {
					return fmt.Errorf("%w: '%s' nicht aus erfasse ableitbar", ErrMissingPrecursor, src)
				}
				c, ok := chains[root]
				if !ok {
					return fmt.Errorf("%w: keine kohärente kette fuer '%s'", ErrProvenanceBroken, src)
				}
				if _, ok := derived[src]; !ok {
					derived[src] = root
				}
				c.Members[src] = true
				c.HasT = true
				if c.TransDst == "" {
					c.TransDst = dst
				}
				chains[src] = c
				break
			}

			// pruefe provenance: src muss bekannt sein
			if _, ok := derived[src]; !ok {
				return fmt.Errorf("%w: '%s' nicht aus erfasse ableitbar", ErrMissingPrecursor, src)
			}

			// finde chain des src
			var c *chainInfo
			var found bool
			root := findRoot(src, derived)
			if root != "" {
				if ch, ok := chains[root]; ok {
					c = ch
					found = true
				}
			}
			if !found {
				for _, ch := range chains {
					if ch.Members[src] || ch.Origin == src || src == ch.TransDst {
						c = ch
						found = true
						break
					}
				}
			}
			if !found || c == nil {
				return fmt.Errorf("%w: keine kohärente kette fuer '%s'", ErrProvenanceBroken, src)
			}

			// verkette
			derived[dst] = src
			c.Derived[dst] = src
			c.Members[dst] = true
			c.HasT = true
			c.TransDst = dst
			// dst gehört jetzt zum gleichen tripel
			chains[dst] = c // alias für lookup

		case *CheckStmt:
			ident := s.Ident
			root := findRoot(ident, derived)
			if root == "" {
				if _, ok := chains[ident]; ok {
					root = ident
				} else if _, ok := derived[ident]; !ok {
					return fmt.Errorf("%w: '%s' hat keinen erfasse-ursprung", ErrProvenanceBroken, ident)
				} else {
					root = ident
				}
			}
			c, ok := chains[root]
			if !ok {
				// fallback
				for _, ch := range chains {
					if ch.Members[ident] || ch.Origin == ident || ch.TransDst == ident {
						c = ch
						ok = true
						break
					}
				}
			}
			if !ok || c == nil {
				return fmt.Errorf("%w: unbekanntes tripel fuer '%s'", ErrChainNotCoherent, ident)
			}
			if !c.HasT {
				return fmt.Errorf("%w: '%s'", ErrCheckWithoutForm, ident)
			}
			c.HasI = true
			c.Checked = ident
			c.Members[ident] = true
			chains[ident] = c

		case *CommitStmt:
			ident := s.Ident
			root := findRoot(ident, derived)
			if root == "" {
				if _, ok := chains[ident]; ok {
					root = ident
				} else if _, ok := derived[ident]; !ok {
					return fmt.Errorf("%w: '%s' hat keinen erfasse-ursprung", ErrProvenanceBroken, ident)
				} else {
					root = ident
				}
			}
			c, ok := chains[root]
			if !ok {
				for _, ch := range chains {
					if ch.Members[ident] || ch.Origin == ident || ch.TransDst == ident || ch.Checked == ident {
						c = ch
						ok = true
						break
					}
				}
			}
			if !ok || c == nil {
				return fmt.Errorf("%w: unbekanntes tripel fuer '%s'", ErrChainNotCoherent, ident)
			}
			// II/DELTA verlangt vollstaendiges z-t-i
			if !(c.HasZ && c.HasT && c.HasI) {
				return fmt.Errorf("%w (gate: z=%v t=%v i=%v)", ErrCommitWithoutTrip, c.HasZ, c.HasT, c.HasI)
			}
			// kohärenz: bind muss auf register im gleichen tripel zeigen
			if !c.Members[ident] && c.Checked != ident && c.TransDst != ident && c.Origin != ident {
				return fmt.Errorf("%w: bind zielt auf fremdes register '%s' ausserhalb des geprüften tripels", ErrChainNotCoherent, ident)
			}
			c.Committed = true
		case *HaltStmt:
			// noop
		default:
			// ignore unknown for now
		}
	}

	// II-meta: jedes commit muss ein kohärentes tripel haben - grobe pruefung
	for _, c := range chains {
		if c.Committed {
			if !(c.HasZ && c.HasT && c.HasI) {
				return fmt.Errorf("%w: inkonsistentes tripel fuer ursprung '%s'", ErrCommitWithoutTrip, c.Origin)
			}
		}
	}

	return nil
}

func findRoot(ident string, derived map[string]string) string {
	visited := map[string]bool{}
	cur := ident
	for cur != "" {
		if visited[cur] {
			break
		}
		visited[cur] = true
		if p, ok := derived[cur]; ok {
			if p == "" {
				return cur
			}
			cur = p
			continue
		}
		// check if it's a root marker
		break
	}
	// if ident maps to empty string
	if p, ok := derived[ident]; ok && p == "" {
		return ident
	}
	return ""
}
