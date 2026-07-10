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
		"MarkSiteChangeApplicationAndContentActionVerified",
		"/actions/{actionID}/verify",
		"/actions/{actionID}/dismiss",
		"verifySEOContentAction",
		"DismissSEOContentActionAndOpportunity",
		"dismissSEOContentActionAndOpportunity",
		"verification_failed",
		"recovery_required",
		"verified_at",
		"verification_snapshot",
		"status = 'measuring'",
		"jsonb_build_object('publisher_result'",
		"VerificationSnapshot",
		"'verification_failed','recovery_required','dismissed'",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("verification contract missing %q", want)
		}
	}
}

func TestAutomaticSiteFixVerificationUsesOneAtomicStatement(t *testing.T) {
	queryRaw, err := os.ReadFile("../db/queries/seo.sql")
	if err != nil {
		t.Fatalf("read seo.sql: %v", err)
	}
	query := string(queryRaw)
	start := strings.Index(query, "-- name: MarkSiteChangeApplicationAndContentActionVerified :one")
	if start < 0 {
		t.Fatal("missing atomic Site Fix verification query")
	}
	end := strings.Index(query[start+1:], "-- name:")
	if end >= 0 {
		query = query[start : start+1+end]
	} else {
		query = query[start:]
	}
	for _, want := range []string{"with verified_application as", "update site_change_applications", "update content_actions", "from verified_application"} {
		if !strings.Contains(query, want) {
			t.Fatalf("atomic Site Fix verification query missing %q", want)
		}
	}

	schedulerRaw, err := os.ReadFile("../scheduler/sitefix_verify.go")
	if err != nil {
		t.Fatalf("read sitefix_verify.go: %v", err)
	}
	scheduler := string(schedulerRaw)
	start = strings.Index(scheduler, "func (s *Scheduler) markSiteChangeVerified")
	end = strings.Index(scheduler, "func siteFixVerifiedPublisherResult")
	if start < 0 || end <= start {
		t.Fatal("could not isolate markSiteChangeVerified")
	}
	body := scheduler[start:end]
	if !strings.Contains(body, "MarkSiteChangeApplicationAndContentActionVerified") {
		t.Fatal("automatic verification must use the atomic application/action query")
	}
	if strings.Contains(body, "MarkSiteChangeApplicationStatus") || strings.Contains(body, "MarkContentActionSiteFixVerified") {
		t.Fatal("automatic verification must not split application and content action updates")
	}
}
