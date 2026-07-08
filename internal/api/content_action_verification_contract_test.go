package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContentActionVerificationEndpointPersistsSnapshot(t *testing.T) {
	handler, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	server, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	query, err := os.ReadFile("../db/queries/seo.sql")
	if err != nil {
		t.Fatalf("read seo.sql: %v", err)
	}
	migrations := ""
	migrationFiles, err := filepath.Glob("../migrations/*.sql")
	if err != nil {
		t.Fatalf("list migrations: %v", err)
	}
	for _, migrationFile := range migrationFiles {
		raw, err := os.ReadFile(migrationFile)
		if err != nil {
			t.Fatalf("read migration %s: %v", migrationFile, err)
		}
		migrations += string(raw)
	}
	combined := string(handler) + string(server) + string(query) + migrations
	for _, want := range []string{
		"MarkContentActionVerification",
		"/actions/{actionID}/verify",
		"/actions/{actionID}/dismiss",
		"verifySEOContentAction",
		"DismissSEOContentActionAndOpportunity",
		"dismissSEOContentActionAndOpportunity",
		"verification_failed",
		"recovery_required",
		"verified_at",
		"verification_snapshot",
		"VerificationSnapshot",
		"'verification_failed','recovery_required','dismissed'",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("verification contract missing %q", want)
		}
	}
}
