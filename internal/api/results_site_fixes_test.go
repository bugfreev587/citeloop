package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func TestResultsFeedCursorRoundTripAndLegacyCompatibility(t *testing.T) {
	want := resultsFeedCursor{ActivityAt: time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC), SourceType: "site_fix", ID: uuid.New()}
	req := httptest.NewRequest("GET", "/?cursor="+encodeResultsFeedCursor(want), nil)
	got, err := parseResultsFeedCursor(req)
	if err != nil || got.ActivityAt != want.ActivityAt || got.SourceType != want.SourceType || got.ID != want.ID {
		t.Fatalf("cursor=%+v err=%v", got, err)
	}
	legacy := "2026-07-14T11:00:00Z"
	got, err = parseResultsFeedCursor(httptest.NewRequest("GET", "/?cursor="+legacy, nil))
	if err != nil || got.LegacyAt.Format(time.RFC3339) != legacy {
		t.Fatalf("legacy cursor=%+v err=%v", got, err)
	}
}

func TestResultsUnionRowsAreDiscriminatedWithoutZeroContentAction(t *testing.T) {
	action := ResultsAction{ContentAction: db.ContentAction{ID: uuid.New(), ProjectID: uuid.New(), OpportunityID: uuid.New(), ActionType: "publish", Status: "completed"}, SourceType: "content_action", OpportunityType: "content_gap", Measurements: []ActionMeasurement{}}
	siteFix := ResultsSiteFixSummary{SourceType: "site_fix", ID: uuid.New(), ProjectID: uuid.New(), SiteFixID: uuid.New(), Status: "observing", SecondaryMetrics: json.RawMessage(`[]`)}
	actionJSON, _ := json.Marshal(action)
	siteFixJSON, _ := json.Marshal(siteFix)
	for _, want := range []string{`"source_type":"content_action"`, `"action_type":"publish"`, `"opportunity_id":"`} {
		if !strings.Contains(string(actionJSON), want) {
			t.Fatalf("legacy Content Action field missing %q: %s", want, actionJSON)
		}
	}
	for _, forbidden := range []string{`"action_type"`, `"opportunity_id"`, `"content_action_id"`} {
		if strings.Contains(string(siteFixJSON), forbidden) {
			t.Fatalf("Site Fix summary contains zero Content Action field %q: %s", forbidden, siteFixJSON)
		}
	}
}

func TestResultsSiteFixDetailDTOIsExplicitlyRedacted(t *testing.T) {
	detail := ResultsSiteFixMeasurementDetail{
		SourceType:               "site_fix",
		Measurement:              ResultsSiteFixSummary{SourceType: "site_fix", ID: uuid.New(), SecondaryMetrics: json.RawMessage(`[]`)},
		SiteFix:                  ResultsSiteFixPublic{ID: uuid.New(), TargetURLs: json.RawMessage(`[]`)},
		Checkpoints:              []ResultsSiteFixCheckpointPublic{{ID: uuid.New(), CheckpointKey: "primary", CheckpointRole: "primary"}},
		Terminal:                 &ResultsSiteFixTerminalPublic{ID: uuid.New(), OutcomeLabel: "positive", RecordKind: "directional_learning", TerminalReason: "public reason"},
		MeasurementSummary:       ResultsSiteFixSummary{SourceType: "site_fix", ID: uuid.New(), SecondaryMetrics: json.RawMessage(`[]`)},
		MeasurementHandoffStatus: "started",
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"target_identity", "classifier_version", "baseline_snapshot", "measurement_policy_snapshot", "seo_metrics", "ga4_metrics", "geo_metrics", "execution_metrics", "guardrail_results", "lock_token", "last_error"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("detail leaked %q: %s", forbidden, raw)
		}
	}
	if !strings.Contains(string(raw), `"terminal_reason":"public reason"`) || !strings.Contains(string(raw), `"checkpoint_key":"primary"`) {
		t.Fatalf("public terminal/checkpoint fields missing: %s", raw)
	}
}

func TestSiteFixMeasurementHandoffPublicStatesUsePersistedRows(t *testing.T) {
	for _, testCase := range []struct {
		status string
		err    error
		want   string
	}{
		{err: pgx.ErrNoRows, want: "not_started"},
		{status: "pending", want: "pending"},
		{status: "failed_retryable", want: "pending"},
		{status: "processing", want: "started"},
		{status: "completed", want: "started"},
		{status: "failed_terminal", want: "failed"},
	} {
		if got := siteFixMeasurementHandoffStatus(db.SiteFixMeasurementHandoffOutbox{Status: testCase.status}, testCase.err); got != testCase.want {
			t.Fatalf("status=%s err=%v got=%s want=%s", testCase.status, testCase.err, got, testCase.want)
		}
	}
}

func TestDoctorSiteFixMeasurementSummaryUsesActualRowsOnly(t *testing.T) {
	fix := db.SiteFix{ID: uuid.New(), ProjectID: uuid.New(), Status: "verified", MeasurementPolicy: "verification_only"}
	summary, status := doctorSiteFixMeasurementSummary(fix, db.SiteFixMeasurement{}, pgx.ErrNoRows, db.SiteFixMeasurementHandoffOutbox{}, pgx.ErrNoRows)
	if summary != nil || status != "not_applicable" || fix.Status != "verified" {
		t.Fatalf("verification-only summary=%+v handoff=%s fix=%s", summary, status, fix.Status)
	}
	fix.MeasurementPolicy = "measurement_required"
	summary, status = doctorSiteFixMeasurementSummary(fix, db.SiteFixMeasurement{}, pgx.ErrNoRows, db.SiteFixMeasurementHandoffOutbox{}, pgx.ErrNoRows)
	if summary != nil || status != "not_started" {
		t.Fatalf("missing required measurement summary=%+v handoff=%s", summary, status)
	}
	measurement := db.SiteFixMeasurement{ID: uuid.New(), ProjectID: fix.ProjectID, SiteFixID: fix.ID, MeasurementGeneration: 2, Status: "ready", BaselineStatus: "ready", AttributionConfidence: "high"}
	handoff := db.SiteFixMeasurementHandoffOutbox{Status: "failed_terminal"}
	summary, status = doctorSiteFixMeasurementSummary(fix, measurement, nil, handoff, nil)
	if summary == nil || summary.ID != measurement.ID || status != "failed" || summary.ResultsDeepLink == nil || *summary.ResultsDeepLink != siteFixResultsDeepLink(fix.ProjectID, measurement.ID) {
		t.Fatalf("actual summary=%+v handoff=%s", summary, status)
	}
}
