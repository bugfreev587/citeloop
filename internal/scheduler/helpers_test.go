package scheduler

import "testing"

func TestCeilDiv(t *testing.T) {
	cases := []struct {
		a, b, want int
	}{
		{3 * 5, 7, 3},  // cadence 3/wk, buffer 5d -> ceil(15/7)=3
		{3 * 7, 7, 3},  // exactly a week
		{3 * 14, 7, 6}, // two weeks
		{3 * 0, 7, 0},  // buffer 0 -> stock nothing (operator-driven)
		{0, 7, 0},
		{1, 7, 1},
	}
	for _, c := range cases {
		if got := ceilDiv(c.a, c.b); got != c.want {
			t.Errorf("ceilDiv(%d,%d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
