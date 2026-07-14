//go:build integration

package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestResultsSiteFixHTTPPostgres(t *testing.T) {
	dsn := os.Getenv("CITELOOP_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("CITELOOP_TEST_DATABASE_URL is not configured")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	now := time.Now().UTC().Truncate(time.Second)
	projectID, fixID := insertMeasurementTransactionFixture(t, ctx, pool, "verified", "verified", true, now.Add(-24*time.Hour))
	otherProjectID, _ := insertMeasurementTransactionFixture(t, ctx, pool, "verified", "verified", true, now.Add(-24*time.Hour))
	if _, err := pool.Exec(ctx, `update projects set owner_id=$2 where id in ($1,$3)`, projectID, localDevOwnerID, otherProjectID); err != nil {
		t.Fatal(err)
	}
	verificationOnlyFixID := uuid.New()
	if _, err := pool.Exec(ctx, `insert into site_fixes(id,project_id,doctor_finding_id,candidate_id,work_signature_id,supersedes_site_fix_id,status,finding_kind,target_urls,evidence_snapshot,proposed_fix,acceptance_tests,verified_at,measurement_policy) select $3,project_id,doctor_finding_id,candidate_id,work_signature_id,$2,'verified',finding_kind,target_urls,evidence_snapshot,proposed_fix,acceptance_tests,$4,'verification_only' from site_fixes where project_id=$1 and id=$2`, projectID, fixID, verificationOnlyFixID, now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	policy := json.RawMessage(`{"policy_version":"site-fix-growth-v1","early_signal_offset_days":7,"primary_checkpoint_offset_days":28,"follow_up_offsets_days":[42],"max_follow_up_attempts":1,"max_measuring_duration_days":56,"minimum_sample":{"minimum_after_periods":7,"minimum_after_sample":100},"metric_thresholds":{"direction":"increase","kind":"relative","value":0.05},"guardrails":[{"metric":"impressions","max_adverse_relative":0.15}],"required_data_sources":["gsc"],"terminalization_grace_period_days":2}`)
	measurement, err := q.CreateSiteFixMeasurement(ctx, db.CreateSiteFixMeasurementParams{
		ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, CreationIdempotencyKey: "http-results",
		TargetUrl: "https://example.com/pricing", NormalizedTargetUrl: "https://example.com/pricing",
		TargetIdentity: json.RawMessage(`{"provider_secret":"never-public"}`), FixType: "metadata_ctr_optimization",
		ImpactMode: "conversion_or_ctr", ClassifierVersion: "private-classifier", DecisionOrigin: "system_rule", DecisionConfidence: "high",
		GrowthHypothesis: "A clearer title improves CTR.", PrimaryMetric: "ctr", SecondaryMetrics: json.RawMessage(`["impressions"]`),
		MeasurementPolicyVersion: "site-fix-growth-v1", MeasurementPolicySnapshot: policy,
		BaselineWindow: json.RawMessage(`{"start":"2026-06-01","end":"2026-06-28"}`), BaselineSnapshot: json.RawMessage(`{"private_baseline":true}`),
		BaselineStatus: "ready", Status: "ready", AttributionConfidence: "high",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `update site_fix_measurements set updated_at=$3 where project_id=$1 and id=$2`, projectID, measurement.ID, now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	scopedStates, err := q.ListLatestSiteFixMeasurementStatesForFixes(ctx, db.ListLatestSiteFixMeasurementStatesForFixesParams{
		ProjectID: projectID, SiteFixIds: []uuid.UUID{verificationOnlyFixID},
	})
	if err != nil || len(scopedStates) != 0 {
		t.Fatalf("measurement batch query ignored requested Site Fix scope: rows=%+v err=%v", scopedStates, err)
	}
	scopedStates, err = q.ListLatestSiteFixMeasurementStatesForFixes(ctx, db.ListLatestSiteFixMeasurementStatesForFixesParams{
		ProjectID: projectID, SiteFixIds: []uuid.UUID{fixID},
	})
	if err != nil || len(scopedStates) != 1 || scopedStates[0].SiteFixID != fixID || scopedStates[0].Status != "ready" || scopedStates[0].HandoffStatus != "" {
		t.Fatalf("measurement batch query lost scoped status: rows=%+v err=%v", scopedStates, err)
	}

	opportunityID, actionID := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `insert into seo_opportunities(id,project_id,type,status,page_url,normalized_page_url,query,evidence) values($1,$2,'content_gap','accepted','https://example.com/content','https://example.com/content','query','{}')`, opportunityID, projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `insert into content_actions(id,project_id,opportunity_id,action_type,status,target_url,normalized_target_url,published_at,updated_at) values($1,$2,$3,'publish','completed','https://example.com/content','https://example.com/content',$4,$4)`, actionID, projectID, opportunityID, now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `insert into action_measurements(project_id,content_action_id,checkpoint_day,seo_metrics,outcome_label,outcome_reason,attribution_confidence) values($1,$2,7,'{"clicks":12}','positive','public action reason','high')`, projectID, actionID); err != nil {
		t.Fatal(err)
	}

	checkpoint, err := q.GetOrCreateSiteFixMeasurementCheckpoint(ctx, db.GetOrCreateSiteFixMeasurementCheckpointParams{
		ID: uuid.New(), ProjectID: projectID, MeasurementID: measurement.ID, CheckpointKey: "primary", CheckpointRole: "primary",
		ScheduledAt: pgutil.TS(now), WindowStart: pgutil.TS(now.Add(-24 * time.Hour)), WindowEnd: pgutil.TS(now), AttemptNumber: 1,
		RequiredDataSources: json.RawMessage(`["gsc"]`), DataAvailability: json.RawMessage(`{"private":true}`), MinimumSample: json.RawMessage(`{"private":true}`),
		SeoMetrics: json.RawMessage(`{"provider_secret":true}`), Ga4Metrics: json.RawMessage(`{}`), GeoMetrics: json.RawMessage(`{}`),
		ExecutionMetrics: json.RawMessage(`{"private":true}`), GuardrailResults: json.RawMessage(`{"private":true}`),
		AttributionConfidence: "high", RetryClassification: "not_applicable", NextAttemptAt: pgutil.TS(now),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.GetOrCreateSiteFixMeasurementTerminalOutcome(ctx, db.GetOrCreateSiteFixMeasurementTerminalOutcomeParams{
		ID: uuid.New(), ProjectID: projectID, MeasurementID: measurement.ID, OutcomeLabel: "positive", RecordKind: "directional_learning",
		TerminalReason: "public terminal reason", MeasurementPolicyVersion: measurement.MeasurementPolicyVersion,
		BaselineSnapshot: json.RawMessage(`{"private":true}`), CheckpointSnapshot: json.RawMessage(`{"private":true}`), OutcomeSnapshot: json.RawMessage(`{"private":true}`),
	}); err != nil {
		t.Fatal(err)
	}

	server := &Server{Pool: pool, Q: q, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	router := server.Router()
	listURL := "/api/projects/" + projectID.String() + "/results/actions?limit=1"
	firstPage := serveResultsHTTP(t, router, listURL)
	if firstPage.Code != http.StatusOK || firstPage.Header().Get("X-Next-Cursor") == "" {
		t.Fatalf("first page status=%d cursor=%q body=%s", firstPage.Code, firstPage.Header().Get("X-Next-Cursor"), firstPage.Body.String())
	}
	var contentItems []map[string]any
	if err := json.Unmarshal(firstPage.Body.Bytes(), &contentItems); err != nil || len(contentItems) != 1 {
		t.Fatalf("first page is not a bare array: items=%v err=%v body=%s", contentItems, err, firstPage.Body.String())
	}
	if contentItems[0]["source_type"] != "content_action" || contentItems[0]["action_type"] != "publish" || len(contentItems[0]["measurements"].([]any)) != 1 {
		t.Fatalf("content item lost discriminator, legacy fields, or measurements: %+v", contentItems[0])
	}

	secondPage := serveResultsHTTP(t, router, listURL+"&cursor="+firstPage.Header().Get("X-Next-Cursor"))
	var siteFixItems []map[string]any
	if err := json.Unmarshal(secondPage.Body.Bytes(), &siteFixItems); err != nil || len(siteFixItems) != 1 || siteFixItems[0]["source_type"] != "site_fix" {
		t.Fatalf("second page=%s err=%v", secondPage.Body.String(), err)
	}
	assertNoResultsSecrets(t, secondPage.Body.String())
	for _, forbidden := range []string{"action_type", "opportunity_id", "content_action_id"} {
		if _, exists := siteFixItems[0][forbidden]; exists {
			t.Fatalf("Site Fix summary contains ContentAction field %q: %s", forbidden, secondPage.Body.String())
		}
	}

	detailPath := "/api/projects/" + projectID.String() + "/results/site-fixes/" + measurement.ID.String()
	detail := serveResultsHTTP(t, router, detailPath)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), checkpoint.ID.String()) || !strings.Contains(detail.Body.String(), "public terminal reason") {
		t.Fatalf("detail status=%d body=%s", detail.Code, detail.Body.String())
	}
	assertNoResultsSecrets(t, detail.Body.String())
	if crossProject := serveResultsHTTP(t, router, "/api/projects/"+otherProjectID.String()+"/results/site-fixes/"+measurement.ID.String()); crossProject.Code != http.StatusNotFound {
		t.Fatalf("cross-project status=%d body=%s", crossProject.Code, crossProject.Body.String())
	}

	assertDoctorHandoffHTTP(t, router, projectID, fixID, "pending")
	assertDoctorListHandoffHTTP(t, router, projectID, fixID, measurement.ID, "pending")
	assertDoctorListVerificationOnlyHTTP(t, router, projectID, verificationOnlyFixID)
	handoff, err := q.EnqueueSiteFixMeasurementHandoff(ctx, db.EnqueueSiteFixMeasurementHandoffParams{
		ID: uuid.New(), ProjectID: projectID, SiteFixID: fixID, MeasurementGeneration: measurement.MeasurementGeneration,
		IdempotencyKey: "http-activate", MaxAttempts: 3, NextAttemptAt: pgutil.TS(now), OccurredAt: pgutil.TS(now),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `update site_fix_measurement_handoff_outbox set status='processing',lock_token=$2,locked_until=$3,updated_at=now() where id=$1`, handoff.ID, uuid.New(), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	assertDoctorHandoffHTTP(t, router, projectID, fixID, "pending")
	if _, err := q.ActivateSiteFixMeasurement(ctx, db.ActivateSiteFixMeasurementParams{
		StartedAt: pgutil.TS(now), ProjectID: projectID, SiteFixID: fixID, MeasurementGeneration: measurement.MeasurementGeneration,
	}); err != nil {
		t.Fatal(err)
	}
	assertDoctorHandoffHTTP(t, router, projectID, fixID, "started")
	assertDoctorListHandoffHTTP(t, router, projectID, fixID, measurement.ID, "started")
	if _, err := pool.Exec(ctx, `update site_fix_measurement_handoff_outbox set status='failed_terminal',lock_token=null,locked_until=null,last_error='private sql failure',updated_at=now() where id=$1`, handoff.ID); err != nil {
		t.Fatal(err)
	}
	assertDoctorHandoffHTTP(t, router, projectID, fixID, "failed")
	assertDoctorListHandoffHTTP(t, router, projectID, fixID, measurement.ID, "failed")

	brokenPool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	brokenServer := &Server{Q: db.New(brokenPool), Log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	brokenPool.Close()
	brokenDetail := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, detailPath, nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("projectID", projectID.String())
	routeContext.URLParams.Add("measurementID", measurement.ID.String())
	request = request.WithContext(context.WithValue(request.Context(), chi.RouteCtxKey, routeContext))
	brokenServer.getResultsSiteFixMeasurement(brokenDetail, request)
	if brokenDetail.Code < 500 || strings.Contains(strings.ToLower(brokenDetail.Body.String()), "pool is closed") || strings.Contains(strings.ToLower(brokenDetail.Body.String()), "select ") {
		t.Fatalf("operational error leaked internals: status=%d body=%s", brokenDetail.Code, brokenDetail.Body.String())
	}
	brokenList := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/projects/"+projectID.String()+"/results/actions", nil)
	listRouteContext := chi.NewRouteContext()
	listRouteContext.URLParams.Add("projectID", projectID.String())
	listRequest = listRequest.WithContext(context.WithValue(listRequest.Context(), chi.RouteCtxKey, listRouteContext))
	brokenServer.listResultsActions(brokenList, listRequest)
	if brokenList.Code < 500 || strings.Contains(strings.ToLower(brokenList.Body.String()), "pool is closed") || strings.Contains(strings.ToLower(brokenList.Body.String()), "select ") {
		t.Fatalf("list operational error leaked internals: status=%d body=%s", brokenList.Code, brokenList.Body.String())
	}
}

func serveResultsHTTP(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	return response
}

func assertNoResultsSecrets(t *testing.T, body string) {
	t.Helper()
	for _, forbidden := range []string{"provider_secret", "private_baseline", "private-classifier", "measurement_policy_snapshot", "seo_metrics", "execution_metrics", "guardrail_results", "last_error"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("Results response leaked %q: %s", forbidden, body)
		}
	}
}

func assertDoctorHandoffHTTP(t *testing.T, handler http.Handler, projectID, fixID uuid.UUID, want string) {
	t.Helper()
	response := serveResultsHTTP(t, handler, "/api/projects/"+projectID.String()+"/doctor/site-fixes/"+fixID.String())
	if response.Code != http.StatusOK {
		t.Fatalf("Doctor detail status=%d body=%s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["measurement_handoff_status"] != want {
		t.Fatalf("Doctor handoff=%v want=%s body=%s", body["measurement_handoff_status"], want, response.Body.String())
	}
}

func assertDoctorListHandoffHTTP(t *testing.T, handler http.Handler, projectID, fixID, measurementID uuid.UUID, want string) {
	t.Helper()
	response := serveResultsHTTP(t, handler, "/api/projects/"+projectID.String()+"/doctor/site-fixes")
	if response.Code != http.StatusOK {
		t.Fatalf("Doctor list status=%d body=%s", response.Code, response.Body.String())
	}
	var rows []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("Doctor list escaped requested project/fix scope: rows=%d body=%s", len(rows), response.Body.String())
	}
	for _, row := range rows {
		if row["id"] != fixID.String() {
			continue
		}
		summary, _ := row["measurement_summary"].(map[string]any)
		if row["measurement_handoff_status"] != want || summary["id"] != measurementID.String() {
			t.Fatalf("Doctor list handoff=%v summary=%v want=%s/%s body=%s", row["measurement_handoff_status"], summary, want, measurementID, response.Body.String())
		}
		return
	}
	t.Fatalf("Doctor list missing Site Fix %s: %s", fixID, response.Body.String())
}

func assertDoctorListVerificationOnlyHTTP(t *testing.T, handler http.Handler, projectID, fixID uuid.UUID) {
	t.Helper()
	response := serveResultsHTTP(t, handler, "/api/projects/"+projectID.String()+"/doctor/site-fixes")
	var rows []map[string]any
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &rows) != nil {
		t.Fatalf("Doctor verification-only list status=%d body=%s", response.Code, response.Body.String())
	}
	for _, row := range rows {
		if row["id"] == fixID.String() {
			if row["measurement_handoff_status"] != "not_applicable" || row["measurement_summary"] != nil {
				t.Fatalf("verification-only row synthesized measurement state: %s", response.Body.String())
			}
			return
		}
	}
	t.Fatalf("Doctor list missing verification-only Site Fix %s: %s", fixID, response.Body.String())
}
