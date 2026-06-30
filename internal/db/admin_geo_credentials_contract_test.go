package db

import (
	"os"
	"strings"
	"testing"
)

func TestAdminGEOCredentialsMigrationStoresTokenGateProviderScopes(t *testing.T) {
	data, err := os.ReadFile("../migrations/0028_admin_geo_provider_credentials.sql")
	if err != nil {
		t.Fatalf("admin GEO credentials migration missing: %v", err)
	}
	migration := strings.ToLower(string(data))
	for _, want := range []string{
		"create table if not exists admin_geo_provider_credentials",
		"scope text not null",
		"provider text not null",
		"api_key text not null",
		"base_url text not null",
		"model text not null",
		"enabled boolean not null default true",
		"check (provider = 'tokengate')",
		"'perplexity'",
		"'openai'",
		"'anthropic'",
		"'gemini'",
	} {
		if !strings.Contains(migration, want) {
			t.Fatalf("admin GEO credentials migration missing %q", want)
		}
	}
}
