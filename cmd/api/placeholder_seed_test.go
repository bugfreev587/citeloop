package main

import (
	"testing"

	"github.com/citeloop/citeloop/internal/config"
)

func TestShouldSeedPlaceholderSkipsProductionAndClerkEnvironments(t *testing.T) {
	if shouldSeedPlaceholder(config.Env{Environment: "production"}) {
		t.Fatal("production must not auto-create the default placeholder project")
	}
	if shouldSeedPlaceholder(config.Env{ClerkSecretKey: "sk_live_clerk"}) {
		t.Fatal("Clerk-backed environments must not auto-create the default placeholder project")
	}
}

func TestShouldSeedPlaceholderAllowsLocalDevelopment(t *testing.T) {
	if !shouldSeedPlaceholder(config.Env{}) {
		t.Fatal("local development without Clerk should still get the placeholder project")
	}
}
