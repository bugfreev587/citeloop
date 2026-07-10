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
		"admin.LoadRuntimeGEOCredentials",
		"admin.LoadCredentials",
		"tokenGateProviderFromGEOCredentials",
		"tokenGateProviderFromLLMCredentials",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("GEO runtime wiring missing %q", want)
		}
	}
	if strings.Contains(source, "admin.GEOProviderPerplexity") {
		t.Fatal("GEO runtime should not require Perplexity when OpenAI or Anthropic TokenGate credentials are configured")
	}
}
