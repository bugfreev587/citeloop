package scheduler

import (
	"os"
	"strings"
	"testing"
)

func TestSchedulerGEOLoadsAdminTokenGateCredential(t *testing.T) {
	data, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	for _, want := range []string{
		"admin.LoadRuntimeGEOCredentials",
		"admin.LoadCredentials",
		"geo.NewTokenGateAnswerProvider",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("scheduler GEO provider wiring missing %q", want)
		}
	}
	if strings.Contains(source, "admin.GEOProviderPerplexity") {
		t.Fatal("scheduler GEO runtime should not require Perplexity when OpenAI or Anthropic TokenGate credentials are configured")
	}
}
