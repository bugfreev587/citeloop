package scheduler

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestLegacyMeasuringPolicyBindsBeforeCheckpointEvaluation(t *testing.T) {
	raw, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	bind := strings.Index(source, "q.BindLegacyMeasuringContentActionPolicy")
	evaluate := strings.Index(source, "completeActionMeasurementCheckpoints(action, now)")
	if bind < 0 || evaluate < 0 || bind > evaluate {
		t.Fatal("legacy measuring action must bind its policy before checkpoint evaluation")
	}
}

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
	if completed != 1 || remaining != 4 {
		t.Fatalf("completed=%d remaining=%d, want 1/4 finite legacy schedule", completed, remaining)
	}
	var data struct {
		Checkpoints []struct {
			Role          string `json:"role"`
			Status        string `json:"status"`
			OutcomeLabel  string `json:"outcome_label"`
			OutcomeReason string `json:"outcome_reason"`
		} `json:"checkpoints"`
	}
	if err := json.Unmarshal(window, &data); err != nil {
		t.Fatal(err)
	}
	if len(data.Checkpoints) != 5 {
		t.Fatalf("checkpoints=%d, want 5", len(data.Checkpoints))
	}
	if data.Checkpoints[0].Role != "baseline" || data.Checkpoints[0].OutcomeLabel != "insufficient_data" {
		t.Fatalf("checkpoint outcome_label=%q, want insufficient_data; window=%s", data.Checkpoints[0].OutcomeLabel, string(window))
	}
	if data.Checkpoints[0].OutcomeReason == "" {
		t.Fatalf("checkpoint outcome_reason should be present; window=%s", string(window))
	}
}

func TestAbsoluteDeadlineTerminalizesRemainingCheckpoints(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	deadline := start.AddDate(0, 0, 91)
	action := db.ContentAction{
		ID: uuid.New(), ProjectID: uuid.New(), Status: "measuring",
		MeasurementPolicyVersion: "growth-measurement-v1",
		MeasurementPolicy: json.RawMessage(`{
          "policy_version":"growth-measurement-v1","early_signal_offset_days":28,
          "primary_checkpoint_offset_days":56,"follow_up_offsets_days":[70,84],
          "max_follow_up_attempts":2,"max_measuring_duration_days":84,
          "terminalization_grace_period_days":7
        }`),
		MeasuringStartedAt: pgtype.Timestamptz{Time: start, Valid: true},
		AbsoluteTerminalAt: pgtype.Timestamptz{Time: deadline, Valid: true},
	}
	window, _, remaining, terminalReason := completeActionMeasurementCheckpoints(action, deadline.Add(time.Minute))
	if remaining != 0 || terminalReason == nil || *terminalReason != "absolute_deadline_reached" {
		t.Fatalf("remaining=%d terminalReason=%v window=%s", remaining, terminalReason, window)
	}
	measurements := actionMeasurementsFromWindow(action, window, deadline.Add(time.Minute))
	if len(measurements) != 5 {
		t.Fatalf("measurements=%d, want all terminal checkpoints", len(measurements))
	}
	for _, item := range measurements {
		if item.MeasurementPolicyVersion != "growth-measurement-v1" || item.CheckpointRole == "" || item.DataQualityState != "insufficient" {
			t.Fatalf("measurement contract missing: %#v", item)
		}
	}
}
