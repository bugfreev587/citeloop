package scheduler

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestMeasurementOutcomeSummaryExplainsInsufficientData(t *testing.T) {
	action := db.ContentAction{
		ID:        uuid.New(),
		ProjectID: uuid.New(),
		Status:    "measuring",
	}
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	window := json.RawMessage(`{"primary_metric":"clicks","checkpoints":[{"day":7,"status":"completed","outcome_label":"insufficient_data"}]}`)

	raw := measurementOutcomeSummary(action, "measuring", 1, 1, now, window)

	var summary map[string]any
	if err := json.Unmarshal(raw, &summary); err != nil {
		t.Fatal(err)
	}
	if summary["outcome_label"] != "insufficient_data" {
		t.Fatalf("outcome_label = %#v, want insufficient_data; summary=%s", summary["outcome_label"], string(raw))
	}
	if summary["outcome_reason"] == "" {
		t.Fatalf("outcome_reason should explain why attribution is unavailable: %s", string(raw))
	}
	if summary["attribution_confidence"] != "low" {
		t.Fatalf("attribution_confidence = %#v, want low", summary["attribution_confidence"])
	}
	confounders, ok := summary["confounders"].([]any)
	if !ok || len(confounders) == 0 {
		t.Fatalf("confounders = %#v, want at least one explanatory note", summary["confounders"])
	}
}

func TestDueCheckpointCarriesOutcomeLabelAndReason(t *testing.T) {
	publishedAt := pgtype.Timestamptz{Time: time.Date(2026, 6, 20, 12, 0, 0, 0, time.UTC), Valid: true}
	now := time.Date(2026, 6, 30, 12, 0, 0, 0, time.UTC)
	rawWindow := json.RawMessage(`{"primary_metric":"clicks","checkpoints":[{"day":7,"status":"scheduled"}]}`)

	window, completed, remaining := completeDueMeasurementCheckpoints(rawWindow, publishedAt, now)
	if completed != 1 || remaining != 0 {
		t.Fatalf("completed=%d remaining=%d, want 1/0", completed, remaining)
	}
	var data struct {
		Checkpoints []struct {
			Status        string `json:"status"`
			OutcomeLabel  string `json:"outcome_label"`
			OutcomeReason string `json:"outcome_reason"`
		} `json:"checkpoints"`
	}
	if err := json.Unmarshal(window, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Checkpoints) != 1 {
		t.Fatalf("checkpoints=%d, want 1", len(data.Checkpoints))
	}
	if data.Checkpoints[0].OutcomeLabel != "insufficient_data" {
		t.Fatalf("checkpoint outcome_label=%q, want insufficient_data; window=%s", data.Checkpoints[0].OutcomeLabel, string(window))
	}
	if data.Checkpoints[0].OutcomeReason == "" {
		t.Fatalf("checkpoint outcome_reason should be present; window=%s", string(window))
	}
}
