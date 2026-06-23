package api

import (
	"os"
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
	combined := string(handler) + string(server) + string(query)
	for _, want := range []string{
		"MarkContentActionVerification",
		"/actions/{actionID}/verify",
		"verifySEOContentAction",
		"verification_failed",
		"recovery_required",
		"verified_at",
		"verification_snapshot",
		"VerificationSnapshot",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("verification contract missing %q", want)
		}
	}
}
