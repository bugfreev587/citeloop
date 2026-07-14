package growthradar

import (
	"encoding/json"
	"testing"
)

func TestClassifyContextRejectsSecretsAndKeepsPublicTechnicalTopics(t *testing.T) {
	profile := json.RawMessage(`{
		"positioning":"AI visibility workflow for SaaS teams",
		"features":["opportunity discovery","AES-256-GCM encryption guide","PostgreSQL deployment guide","API key management","postgres://admin:hunter2@private-db.internal/app"],
		"icp":["SaaS growth teams"],
		"key_terms":["AI visibility","TOKEN_GATE_API_KEY=sk-live-1234567890abcdef","-----BEGIN PRIVATE KEY-----"],
		"competitors":["Outrank"]
	}`)
	classification := ClassifyContext(profile, EvidenceIndex{PublicTerms: []string{"content gap analysis"}, SuggestedTerms: []string{"magic growth engine"}})
	accepted := map[string]bool{}
	rejected := map[string]bool{}
	held := map[string]bool{}
	for _, term := range classification.Terms {
		if term.Accepted {
			accepted[term.Value] = true
		} else if term.Class == "unknown" {
			held[term.Value] = true
		} else {
			rejected[term.Value] = true
		}
	}
	for _, value := range []string{"AI visibility workflow for SaaS teams", "opportunity discovery", "AES-256-GCM encryption guide", "PostgreSQL deployment guide", "API key management", "SaaS growth teams", "AI visibility", "Outrank", "content gap analysis"} {
		if !accepted[value] {
			t.Errorf("expected accepted term %q", value)
		}
	}
	for _, value := range []string{"postgres://admin:hunter2@private-db.internal/app", "TOKEN_GATE_API_KEY=sk-live-1234567890abcdef", "-----BEGIN PRIVATE KEY-----"} {
		if !rejected[value] {
			t.Errorf("expected rejected term %q", value)
		}
	}
	if !held["magic growth engine"] {
		t.Fatal("LLM-only suggestion must remain unknown")
	}
}

func TestContainsInternalSensitiveTermUsesDisclosureRiskNotTechnicalNouns(t *testing.T) {
	for _, public := range []string{
		"API key management best practices",
		"Postgres migration checklist",
		"How AES-256 encryption works",
		"Docker deployment for SaaS teams",
	} {
		if ContainsInternalSensitiveTerm(public) {
			t.Errorf("public technical topic classified sensitive: %q", public)
		}
	}
	for _, private := range []string{
		"API_KEY=sk-live-1234567890abcdef",
		"postgres://admin:hunter2@private-db.internal/app",
		"-----BEGIN PRIVATE KEY-----",
		"internal diagnostic for token gate production",
	} {
		if !ContainsInternalSensitiveTerm(private) {
			t.Errorf("secret or internal detail was not classified sensitive: %q", private)
		}
	}
}

func TestClassifyContextOnlyAcceptsConfiguredCompetitors(t *testing.T) {
	profile := json.RawMessage(`{"competitors":["Semrush"]}`)
	classification := ClassifyContext(profile, EvidenceIndex{SuggestedCompetitors: []string{"SuperX", "Ahrefs"}})
	if len(classification.ConfirmedCompetitors) != 1 || classification.ConfirmedCompetitors[0] != "Semrush" {
		t.Fatalf("confirmed competitors = %#v", classification.ConfirmedCompetitors)
	}
	for _, term := range classification.Terms {
		if (term.Value == "SuperX" || term.Value == "Ahrefs") && term.Accepted {
			t.Fatalf("suggested competitor accepted: %+v", term)
		}
	}
}
