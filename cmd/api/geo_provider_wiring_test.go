package main

import (
	"os"
	"strings"
	"testing"
)

func TestMainWiresGEOProviderConfigIntoScheduler(t *testing.T) {
	raw, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	body := string(raw)
	for _, want := range []string{
		"sched.GEOProviderRunBudgetUSD = env.GEOProviderRunBudgetUSD",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("main GEO provider wiring missing %q", want)
		}
	}
}
