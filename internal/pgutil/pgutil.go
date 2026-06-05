// Package pgutil holds small conversions between Go values and pgx/pgtype
// scalars used across the service.
package pgutil

import (
	"math/big"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// Numeric builds a pgtype.Numeric from a float (cost_usd, scores).
func Numeric(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	// represent as integer mantissa * 10^-6 for 6 decimal places
	scaled := int64(f*1e6 + 0.5)
	n.Int = big.NewInt(scaled)
	n.Exp = -6
	n.Valid = true
	return n
}

// NumericPtr is the nullable variant; nil -> invalid.
func NumericPtr(f *float64) pgtype.Numeric {
	if f == nil {
		return pgtype.Numeric{}
	}
	return Numeric(*f)
}

// Float reads a pgtype.Numeric back to float64 (0 if invalid).
func Float(n pgtype.Numeric) float64 {
	if !n.Valid || n.Int == nil {
		return 0
	}
	f := new(big.Float).SetInt(n.Int)
	f.Mul(f, big.NewFloat(pow10(int(n.Exp))))
	out, _ := f.Float64()
	return out
}

func pow10(e int) float64 {
	r := 1.0
	if e >= 0 {
		for i := 0; i < e; i++ {
			r *= 10
		}
		return r
	}
	for i := 0; i < -e; i++ {
		r /= 10
	}
	return r
}

// TS builds a valid pgtype.Timestamptz.
func TS(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}

// TSPtr is the nullable variant.
func TSPtr(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return TS(*t)
}
