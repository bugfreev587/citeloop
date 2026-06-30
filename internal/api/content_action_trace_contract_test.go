package api

import (
	"encoding/json"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

func TestContentActionDefaultsTraceSnapshotsFromOpportunity(t *testing.T) {
	query := "source backed seo workflow"
	action := "Expand the existing page"
	impact := "Captures demand where Search Console shows relevance."
	opp := db.SeoOpportunity{
		ID:                uuid.New(),
		Type:              "gsc_query_gap",
		Query:             &query,
		Evidence:          json.RawMessage(`{"source":"gsc_search_analytics","scoring_version":"gsc_metric_v2"}`),
		RecommendedAction: &action,
		ExpectedImpact:    &impact,
	}

	evidence := contentActionEvidenceSnapshot(nil, opp)
	var evidencePayload map[string]any
	if err := json.Unmarshal(evidence, &evidencePayload); err != nil {
		t.Fatalf("unmarshal evidence snapshot: %v", err)
	}
	if evidencePayload["scoring_version"] != "gsc_metric_v2" {
		t.Fatalf("evidence snapshot = %#v, want opportunity evidence", evidencePayload)
	}

	input := contentActionInputSnapshot(nil, opp, action)
	var inputPayload map[string]any
	if err := json.Unmarshal(input, &inputPayload); err != nil {
		t.Fatalf("unmarshal input snapshot: %v", err)
	}
	if inputPayload["opportunity_id"] != opp.ID.String() {
		t.Fatalf("input opportunity_id = %v, want %s", inputPayload["opportunity_id"], opp.ID)
	}
	if inputPayload["opportunity_type"] != "gsc_query_gap" {
		t.Fatalf("input opportunity_type = %v, want gsc_query_gap", inputPayload["opportunity_type"])
	}
	if inputPayload["query"] != query {
		t.Fatalf("input query = %v, want %s", inputPayload["query"], query)
	}
	if inputPayload["action_type"] != action {
		t.Fatalf("input action_type = %v, want %s", inputPayload["action_type"], action)
	}
}

func TestContentActionTraceSnapshotsHonorExplicitInput(t *testing.T) {
	opp := db.SeoOpportunity{Evidence: json.RawMessage(`{"source":"opportunity"}`)}

	evidence := contentActionEvidenceSnapshot(json.RawMessage(`{"source":"client"}`), opp)
	if string(evidence) != `{"source":"client"}` {
		t.Fatalf("evidence snapshot = %s, want explicit client snapshot", evidence)
	}

	input := contentActionInputSnapshot(json.RawMessage(`{"source":"client_input"}`), opp, "Action")
	if string(input) != `{"source":"client_input"}` {
		t.Fatalf("input snapshot = %s, want explicit client snapshot", input)
	}
}
