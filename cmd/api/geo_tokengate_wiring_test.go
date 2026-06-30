package main

import (
	"os"
	"strings"
	"testing"
)

func TestMainDoesNotWireNativePerplexityEnvAsPrimaryGEOProvider(t *testing.T) {
	data, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if strings.Contains(source, "geo.NewPerplexityProvider(env.PerplexityAPIKey") {
		t.Fatal("GEO provider must be admin TokenGate-backed; native PERPLEXITY_API_KEY env can only be a legacy fallback outside main wiring")
	}
}
