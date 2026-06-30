package api

import (
	"os"
	"strings"
	"testing"
)

func TestGEORuntimeLoadsAdminTokenGateCredential(t *testing.T) {
	data, err := os.ReadFile("handlers_geo.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, want := range []string{
		"admin.LoadGEOCredentials",
		"admin.GEOProviderPerplexity",
		"tokenGateProviderFromGEOCredentials",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("GEO runtime wiring missing %q", want)
		}
	}
}
