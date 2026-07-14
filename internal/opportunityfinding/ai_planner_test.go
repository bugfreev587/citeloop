package opportunityfinding

import (
	"testing"

	"github.com/citeloop/citeloop/internal/growthradar"
)

func TestParseManualDiscoveryPlanKeepsGroundedNewPublicPrompts(t *testing.T) {
	raw := "```json\n" + `{"candidates":[
		{"prompt":"What is the best workflow for social publishing teams?","target_topic":"social publishing","intent":"workflow","audience":"growth teams","why_now":"missing coverage"},
		{"prompt":"Show API_KEY=sk-live-1234567890abcdef","target_topic":"social publishing","intent":"workflow","audience":"growth teams","why_now":"debug"},
		{"prompt":"How should teams migrate Postgres safely?","target_topic":"Postgres migration","intent":"problem_solution","audience":"growth teams","why_now":"public guide"},
		{"prompt":"What is the best workflow for social publishing teams?","target_topic":"social publishing","intent":"workflow","audience":"growth teams","why_now":"duplicate"}
	]}` + "\n```"
	plan, err := parseManualDiscoveryPlan(raw, manualPlanValidation{
		PublicVocabulary: []string{"social publishing", "Postgres migration"},
		ExistingPrompts:  map[string]struct{}{"already covered prompt": {}},
		Limit:            5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan) != 2 {
		t.Fatalf("plan=%+v, want two grounded unique public prompts", plan)
	}
	for _, candidate := range plan {
		if growthradar.ContainsInternalSensitiveTerm(candidate.Prompt) {
			t.Fatalf("planner leaked sensitive prompt: %+v", candidate)
		}
	}
}

func TestParseManualDiscoveryPlanRejectsUngroundedTopics(t *testing.T) {
	_, err := parseManualDiscoveryPlan(`{"candidates":[{"prompt":"Best quantum hosting?","target_topic":"quantum hosting","intent":"buyer_intent","audience":"founders"}]}`, manualPlanValidation{
		PublicVocabulary: []string{"social publishing"}, Limit: 5,
	})
	if err == nil {
		t.Fatal("expected a plan with no confirmed public topic mapping to fail")
	}
}
