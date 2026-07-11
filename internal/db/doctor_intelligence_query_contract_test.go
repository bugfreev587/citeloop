package db

import (
	"strings"
	"testing"
)

func TestDoctorPagePriorityInputsAreBoundedAndSourceExplicit(t *testing.T) {
	query := strings.ToLower(listDoctorPagePriorityInputs)
	for _, required := range []string{
		"from page_performance_daily",
		"date >= current_date - 28",
		"gsc_impressions_28d",
		"ga4_sessions_28d",
		"limit least(greatest",
		"50",
	} {
		if !strings.Contains(query, required) {
			t.Fatalf("Doctor priority query missing %q: %s", required, listDoctorPagePriorityInputs)
		}
	}
}
