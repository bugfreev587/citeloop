package api

import (
	"os"
	"strings"
	"testing"
)

func TestCreateSEOContentActionSchedulesStructuredMeasurementWindow(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"measurementWindowForAction",
		"baseline",
		"checkpoints",
		"primary_metric",
		"secondary_metrics",
		"status",
		"scheduled",
		"metadata_rewrite",
		"internal_link_patch",
		"external_distribution",
		"GEO citation-ready asset",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("measurement window contract missing %q", want)
		}
	}
}
