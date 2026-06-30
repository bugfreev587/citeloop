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
		"admin.LoadGEOCredentials",
		"admin.GEOProviderPerplexity",
		"geo.NewTokenGateAnswerProvider",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("scheduler GEO provider wiring missing %q", want)
		}
	}
}
