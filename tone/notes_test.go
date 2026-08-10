package tone

import "testing"

// Each note's frequency is checked against the value its own constant is
// documented with, so a period that is out by an octave cannot pass.
func TestNoteFrequencies(t *testing.T) {
	for _, c := range []struct {
		name string
		note Note
		hz   float64
	}{
		{"A0", A0, 27.5},
		{"A1", A1, 55},
		{"A3", A3, 220},
		{"A4", A4, 440},
		{"A5", A5, 880},
		{"A8", A8, 7040},
		{"C4", C4, 261.626},
		{"E4", E4, 329.628},
		{"G4", G4, 391.995},
	} {
		t.Run(c.name, func(t *testing.T) {
			period := c.note.Period()
			if period == 0 {
				t.Fatal("Period() returned 0")
			}
			got := 1e9 / float64(period)
			// A cent is a 0.058% frequency change, so 0.5% is comfortably
			// inside the smallest interval a listener would notice while
			// still allowing for the integer arithmetic.
			if tolerance := c.hz * 0.005; got < c.hz-tolerance || got > c.hz+tolerance {
				t.Errorf("%s = %.2fHz (period %dns), want %.2fHz", c.name, got, period, c.hz)
			}
		})
	}
}

func TestNoteZeroIsSilent(t *testing.T) {
	if got := Note(0).Period(); got != 0 {
		t.Errorf("Note(0).Period() = %d, want 0 so that a zero note is silent", got)
	}
}

// Octaves are exact powers of two apart, which pins the relationship the
// original base period got wrong.
func TestOctavesDouble(t *testing.T) {
	for _, c := range []struct {
		name  string
		lower Note
		upper Note
	}{
		{"A0/A1", A0, A1},
		{"A3/A4", A3, A4},
		{"A4/A5", A4, A5},
		{"C4/C5", C4, C5},
	} {
		t.Run(c.name, func(t *testing.T) {
			lower, upper := c.lower.Period(), c.upper.Period()
			if upper == 0 {
				t.Fatal("Period() returned 0")
			}
			ratio := float64(lower) / float64(upper)
			if ratio < 1.99 || ratio > 2.01 {
				t.Errorf("period ratio %.4f, want 2 (%d then %d)", ratio, lower, upper)
			}
		})
	}
}
