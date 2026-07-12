package learning

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
)

type terminalStoreStub struct {
	params db.RecordGrowthTerminalOutcomeParams
	calls  int
}

func (stub *terminalStoreStub) RecordGrowthTerminalOutcome(_ context.Context, params db.RecordGrowthTerminalOutcomeParams) error {
	stub.params = params
	stub.calls++
	return nil
}

func TestRecordTerminalOutcomeSeparatesLearningFromMeasurementQuality(t *testing.T) {
	tests := []struct {
		label    string
		wantKind string
	}{
		{label: "positive", wantKind: "directional_learning"},
		{label: "negative", wantKind: "directional_learning"},
		{label: "mixed", wantKind: "directional_learning"},
		{label: "inconclusive", wantKind: "directional_learning"},
		{label: "insufficient_data", wantKind: "measurement_quality"},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			store := &terminalStoreStub{}
			action := db.ContentAction{ID: uuid.New(), ProjectID: uuid.New(), OpportunityID: uuid.New(), ActionType: "Improve page", Status: "completed", MeasurementPolicyVersion: "growth-measurement-v1", BaselineWindow: json.RawMessage(`{"source":"gsc"}`)}
			opportunity := db.SeoOpportunity{ID: action.OpportunityID, ProjectID: action.ProjectID, Type: "gsc_query_gap", GrowthSpec: json.RawMessage(`{"audience":["developers"],"primary_metric":"gsc_clicks"}`)}
			window := json.RawMessage(`{"checkpoints":[{"day":28,"role":"primary","status":"completed"}]}`)
			outcome := json.RawMessage(`{"outcome_label":"` + tt.label + `","outcome_reason":"measured result","data_quality_state":"complete"}`)
			if err := RecordTerminalOutcome(context.Background(), store, action, opportunity, window, outcome, "decision_outcome_reached"); err != nil {
				t.Fatal(err)
			}
			if store.calls != 1 || store.params.RecordKind != tt.wantKind || store.params.OutcomeLabel != tt.label {
				t.Fatalf("record separation failed: %+v", store.params)
			}
			if len(store.params.BaselineSnapshot) == 0 || len(store.params.CheckpointSnapshot) == 0 || len(store.params.OutcomeSnapshot) == 0 || len(store.params.Applicability) == 0 {
				t.Fatalf("terminal provenance missing: %+v", store.params)
			}
		})
	}
}

func TestRecordTerminalOutcomeRejectsNonterminalOrUnknownLabels(t *testing.T) {
	store := &terminalStoreStub{}
	action := db.ContentAction{ID: uuid.New(), ProjectID: uuid.New(), OpportunityID: uuid.New(), Status: "measuring"}
	opportunity := db.SeoOpportunity{ID: action.OpportunityID, ProjectID: action.ProjectID}
	if err := RecordTerminalOutcome(context.Background(), store, action, opportunity, json.RawMessage(`{}`), json.RawMessage(`{"outcome_label":"positive"}`), ""); err == nil {
		t.Fatal("measuring action must not create a terminal record")
	}
	action.Status = "completed"
	if err := RecordTerminalOutcome(context.Background(), store, action, opportunity, json.RawMessage(`{}`), json.RawMessage(`{"outcome_label":"waiting"}`), ""); err == nil {
		t.Fatal("unknown outcome must fail closed")
	}
}
