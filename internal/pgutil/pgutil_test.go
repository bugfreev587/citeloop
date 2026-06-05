package pgutil

import (
	"math"
	"testing"
)

func TestNumericRoundTrip(t *testing.T) {
	for _, f := range []float64{0, 0.001, 0.82, 15.5, 49.999999} {
		got := Float(Numeric(f))
		if math.Abs(got-f) > 1e-6 {
			t.Errorf("Float(Numeric(%v)) = %v", f, got)
		}
	}
}
