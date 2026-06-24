package db

import (
	"os"
	"strings"
	"testing"
)

func TestMeasurementClosureQueriesExposeDueActionsAndOutcomeWriteback(t *testing.T) {
	raw, err := os.ReadFile("queries/seo.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, want := range []string{
		"-- name: ListDueMeasuringContentActions :many",
		"ca.status = 'measuring'",
		"measurement_window",
		"jsonb_array_elements",
		"for update",
		"skip locked",
		"-- name: UpdateContentActionOutcomeSummary :one",
		"outcome_summary =",
		"status = sqlc.arg(status)::text",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("measurement closure query contract missing %q", want)
		}
	}
}
