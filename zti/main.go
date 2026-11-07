// ZTIS-Kern: Zustand, Transformation, Information, Syncpunkt.
//
// Ein einziges Zustands-Token kreist durch einen Ring aus vier Goroutines
// (Alpha → Bravo → Charlie → Delta → Alpha). Wer das Token hält, besitzt den
// Zustand – kein Mutex, keine geteilte Variable. Pro Zyklus ist genau ein Token
// unterwegs, damit ist die Reihenfolge streng kausal und deterministisch.
//
// Gate-Bit 0 → Charlie-Reset: Staging wird verworfen, Status CLEAN.
// Gate-Bit 1 → Delta-Commit:  Staging wird Live,      Status UPDATED.
package main

import (
	"fmt"
	"math/bits"
)

// Status entspricht direkt dem Gate-Bit des letzten Zyklus.
type Status uint8

const (
	CLEAN   Status = 0
	UPDATED Status = 1
)

func (s Status) String() string {
	if s == UPDATED {
		return "UPDATED"
	}
	return "CLEAN"
}

// Op ist eine Transformation auf dem Register.
type Op uint8

const (
	OpINC Op = iota
	OpXOR
	OpAND
	OpOR
	OpSHL
	OpROL
)

var opNames = [...]string{"INC", "XOR", "AND", "OR", "SHL", "ROL"}

func (o Op) String() string { return opNames[o] }

// Info ist eine Information, die von außen in den Ring eingespeist wird.
type Info struct {
	Op     Op
	Arg    uint64
	Expect uint64 // Invariante für Charlie: Staging muss exakt diesem Wert entsprechen
}

// Token ist der Zustand, der durch den Ring wandert.
type Token struct {
	Seq     uint64
	Live    uint64
	Staging uint64
	Width   uint64 // Ringmaske, z. B. 0xFFFF für U16
	Gate    uint8
	Status  Status
	Info    Info
}

// Trace ist die Telemetrie nach jedem Syncpunkt.
type Trace struct {
	Seq     uint64
	Op      Op
	Staging uint64
	Live    uint64
	Gate    uint8
	Status  Status
}

func bit(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// transform ist rein: gleiche Eingabe, gleiche Ausgabe.
func transform(v uint64, in Info, width uint64) uint64 {
	switch in.Op {
	case OpINC:
		v += in.Arg
	case OpXOR:
		v ^= in.Arg
	case OpAND:
		v &= in.Arg
	case OpOR:
		v |= in.Arg
	case OpSHL:
		v <<= in.Arg & 63
	case OpROL:
		// Rotation innerhalb der Registerbreite, nicht über 64 Bit.
		w := uint(bits.Len64(width))
		k := uint(in.Arg) % w
		v = (v<<k | v>>(w-k)) & width
	}
	return v & width
}

// alpha erfasst Information und koppelt sie an das zurückkehrende Token.
func alpha(in <-chan Info, back <-chan Token, next chan<- Token, final chan<- Token) {
	for info := range in {
		t := <-back
		t.Seq++
		t.Info = info
		t.Staging = t.Live
		next <- t
	}
	final <- <-back
	close(next)
}

// bravo transformiert in den Staging-Bereich. Live bleibt unberührt.
func bravo(prev <-chan Token, next chan<- Token) {
	for t := range prev {
		t.Staging = transform(t.Staging, t.Info, t.Width)
		next <- t
	}
	close(next)
}

// charlie prüft die Invariante und setzt das Gate-Bit.
// Gate 0 → Reset: Staging wird auf Live zurückgeworfen.
func charlie(prev <-chan Token, next chan<- Token) {
	for t := range prev {
		t.Gate = bit(t.Staging == t.Info.Expect)
		m := -uint64(t.Gate) // 0 → 0x00…0, 1 → 0xFF…F
		t.Staging = t.Staging&m | t.Live&^m
		next <- t
	}
	close(next)
}

// delta ist der Syncpunkt: verzweigungsfreier Commit, Telemetrie, Rückgabe des Tokens.
func delta(prev <-chan Token, back chan<- Token, out chan<- Trace) {
	for t := range prev {
		m := -uint64(t.Gate)
		t.Live = t.Staging&m | t.Live&^m
		t.Status = Status(t.Gate)
		out <- Trace{t.Seq, t.Info.Op, t.Staging, t.Live, t.Gate, t.Status}
		back <- t
	}
	close(out)
}

// Run startet den Ring mit einem Initialzustand und speist die Informationen ein.
func Run(init uint64, width uint64, infos []Info) (Token, []Trace) {
	in := make(chan Info)
	back := make(chan Token, 1) // genau ein Token im Umlauf
	ab := make(chan Token)
	bc := make(chan Token)
	cd := make(chan Token)
	out := make(chan Trace)
	final := make(chan Token, 1)

	back <- Token{Live: init & width, Width: width, Status: CLEAN}

	go alpha(in, back, ab, final)
	go bravo(ab, bc)
	go charlie(bc, cd)
	go delta(cd, back, out)

	go func() {
		for _, i := range infos {
			in <- i
		}
		close(in)
	}()

	var traces []Trace
	for tr := range out {
		traces = append(traces, tr)
	}
	return <-final, traces
}

func main() {
	const u16 = 0xFFFF

	scenarios := []struct {
		name  string
		init  uint64
		infos []Info
	}{
		{"Valider Zyklus", 100, []Info{{OpINC, 1, 101}}},
		{"U16-Wrap 65535+1", 65535, []Info{{OpINC, 1, 0}}},
		{"Gate-Bruch → Charlie-Reset", 42, []Info{{OpINC, 1, 99}}},
		{"Kette mit Bruch in der Mitte", 0x00F0, []Info{
			{OpXOR, 0x0F0F, 0x0FFF},
			{OpROL, 4, 0x0000}, // falsch erwartet → Reset, Live bleibt 0x0FFF
			{OpROL, 4, 0xFFF0},
			{OpAND, 0xFF00, 0xFF00},
		}},
	}

	for _, s := range scenarios {
		fmt.Printf("\n=== %s ===\n", s.name)
		final, traces := Run(s.init, u16, s.infos)
		for _, tr := range traces {
			fmt.Printf("  #%d %-3s staging=%016b gate=%d live=%016b %s\n",
				tr.Seq, tr.Op, tr.Staging, tr.Gate, tr.Live, tr.Status)
		}
		fmt.Printf("  → Live=%#04x Status=%s Zyklen=%d\n", final.Live, final.Status, final.Seq)
	}
}
