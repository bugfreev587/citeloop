package growthradar

import (
	"encoding/json"
	"testing"
)

func TestClassifyContextRejectsInternalTermsAndKeepsPublicVocabulary(t *testing.T) {
	profile := json.RawMessage(`{
		"positioning":"AI visibility workflow for SaaS teams",
		"features":["opportunity discovery","AES-256-GCM encrypted credentials","PostgreSQL deployment"],
		"icp":["SaaS growth teams"],
		"key_terms":["AI visibility","TOKEN_GATE_API_KEY"],
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
	for _, value := range []string{"AI visibility workflow for SaaS teams", "opportunity discovery", "SaaS growth teams", "AI visibility", "Outrank", "content gap analysis"} {
		if !accepted[value] {
			t.Errorf("expected accepted term %q", value)
		}
	}
	for _, value := range []string{"AES-256-GCM encrypted credentials", "PostgreSQL deployment", "TOKEN_GATE_API_KEY"} {
		if !rejected[value] {
			t.Errorf("expected rejected term %q", value)
		}
	}
	if !held["magic growth engine"] {
		t.Fatal("LLM-only suggestion must remain unknown")
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
