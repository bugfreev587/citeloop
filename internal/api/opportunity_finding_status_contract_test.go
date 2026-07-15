package api

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestOpportunityFindingStatusUsesRunHistoryAndProjectConfig(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	for _, want := range []string{
		"type OpportunityFindingStatus struct",
		"GrowthSignalEnabled",
		"GrowthAIEnabled",
		"GrowthAIRunPolicy",
		"ManualMode",
		"LastRun",
		"NextFindingAt",
		"Summary",
		"Counts",
		"ListSEORuns",
		"SEOOpportunityCounts",
		"data_source_notes",
		"generated_anomalies",
		"latestOpportunityFindingRun",
		"ListOpportunityFindingStages",
		"StageProgress",
		"ProgressPercent",
	} {
		if !strings.Contains(source, want) {
			t.Fatalf("opportunity finding status contract missing %q", want)
		}
	}
}

func TestOpportunityFindingStageProgressExposesDurableCheckpoints(t *testing.T) {
	rows := []db.OpportunityFindingStageCheckpoint{
		{Stage: "evidence_refresh", StageOrder: 1, Status: "succeeded", AttemptNumber: 1, RequestFingerprint: "sha256:first", OutputSummary: []byte(`{"gsc":"completed"}`)},
		{Stage: "deterministic_signals", StageOrder: 2, Status: "running", AttemptNumber: 2, RequestFingerprint: "sha256:second", OutputSummary: []byte(`{}`)},
	}
	progress, percent, current := opportunityFindingStageProgress(rows)
	if len(progress) != 2 || percent != 16 || current != "deterministic_signals" {
		t.Fatalf("progress=%#v percent=%d current=%q", progress, percent, current)
	}
	if progress[0].Summary["gsc"] != "completed" || progress[1].AttemptNumber != 2 {
		t.Fatalf("stage progress lost checkpoint metadata: %#v", progress)
	}
}

func TestOpportunityFindingRunSurfacesTerminalPartialStageSummary(t *testing.T) {
	run := &OpportunityFindingRun{Status: "completed"}
	errText := "GA4 permission denied"
	attachOpportunityFindingStageProgress(run, []db.OpportunityFindingStageCheckpoint{
		{Stage: "evidence_refresh", StageOrder: 1, Status: "partial", AttemptNumber: 1, RequestFingerprint: "sha256:first", OutputSummary: []byte(`{"gsc":"completed"}`), Error: &errText},
		{Stage: "summary", StageOrder: 6, Status: "succeeded", AttemptNumber: 1, RequestFingerprint: "sha256:last", OutputSummary: []byte(`{"error_count":1}`)},
	})
	if run.Status != "partial" || run.ProgressPercent != 33 || len(run.StageProgress) != 2 {
		t.Fatalf("partial run = %#v", run)
	}
}

func TestOpportunityFindingRunSummarizesAIProgressAndZeroResult(t *testing.T) {
	run := &OpportunityFindingRun{Status: "completed"}
	attachOpportunityFindingStageProgress(run, []db.OpportunityFindingStageCheckpoint{
		{Stage: "evidence_refresh", StageOrder: 1, Status: "succeeded", OutputSummary: []byte(`{"ai_discovery":{"planner_provider_called":true,"planner_accepted":4}}`)},
		{Stage: "ai_hypotheses", StageOrder: 3, Status: "succeeded", OutputSummary: []byte(`{"ai_discovery":{"opportunity_count":0,"repair_attempted":true,"funnel":{"candidates":{"generated":18,"created":0,"filtered":10,"watchlist":5,"duplicates":0,"conflicts":0},"reasons":{"context.internal_sensitive":10,"demand.single_geo_provider":5}}}}`)},
		{Stage: "summary", StageOrder: 6, Status: "succeeded", OutputSummary: []byte(`{}`)},
	})
	if !run.AIProviderCalled || !run.RepairAttempted || run.NewOpportunityCount != 0 || run.ZeroResultReason != "already_handled_or_merged" {
		t.Fatalf("AI run summary = %#v", run)
	}
}

func TestOpportunityFindingRunSummarizesCompetitiveRecallEvidence(t *testing.T) {
	run := &OpportunityFindingRun{Status: "completed"}
	attachOpportunityFindingStageProgress(run, []db.OpportunityFindingStageCheckpoint{
		{Stage: "evidence_refresh", StageOrder: 1, Status: "succeeded", OutputSummary: []byte(`{"ai_discovery":{"competitive_recall_evidence":[
			{"query":"free social content workflow tools","url":"https://postsyncer.com/tools","host":"postsyncer.com","provider_order":1,"seed_candidate":true,"reason":"competitive_seed_candidate_url"},
			{"query":"free social content workflow tools","url":"https://example.com/blog/best-social-tools","host":"example.com","provider_order":2,"seed_candidate":false,"reason":"non_competitive_path"}
		]}}`)},
	})
	if run.CompetitiveRecallQueryCount != 1 || run.CompetitiveRecallResultCount != 2 || run.CompetitiveRecallSeedCandidateCount != 1 {
		t.Fatalf("competitive recall counters = queries:%d results:%d seeds:%d", run.CompetitiveRecallQueryCount, run.CompetitiveRecallResultCount, run.CompetitiveRecallSeedCandidateCount)
	}
	if run.CompetitiveRecallMissedReason != "" {
		t.Fatalf("missed reason = %q, want empty when seed candidate exists", run.CompetitiveRecallMissedReason)
	}

	cfg := config.Default()
	summary := opportunityFindingSummary(nil, run, cfg, OpportunityFindingCounts{})
	var recall *OpportunityFindingSummaryItem
	for index := range summary {
		if summary[index].Label == "Competitive recall" {
			recall = &summary[index]
			break
		}
	}
	if recall == nil || recall.Detail != "1 candidate page from 2 search results across 1 query" || recall.Tone != "green" {
		t.Fatalf("competitive recall summary = %+v", recall)
	}
}

func TestOpportunityFindingRunSummarizesCompetitiveRecallMisses(t *testing.T) {
	run := &OpportunityFindingRun{Status: "completed"}
	attachOpportunityFindingStageProgress(run, []db.OpportunityFindingStageCheckpoint{
		{Stage: "evidence_refresh", StageOrder: 1, Status: "succeeded", OutputSummary: []byte(`{"ai_discovery":{"competitive_recall_evidence":[
			{"query":"free social content workflow tools","url":"https://example.com/blog/best-social-tools","host":"example.com","provider_order":1,"seed_candidate":false,"reason":"non_competitive_path"},
			{"query":"social content workflow alternatives","url":"https://example.org/guide","host":"example.org","provider_order":1,"seed_candidate":false,"reason":"non_competitive_path"}
		]}}`)},
	})
	if run.CompetitiveRecallQueryCount != 2 || run.CompetitiveRecallResultCount != 2 || run.CompetitiveRecallSeedCandidateCount != 0 || run.CompetitiveRecallMissedReason != "non_competitive_path" {
		t.Fatalf("competitive recall miss summary = %#v", run)
	}

	summary := opportunityFindingSummary(nil, run, config.Default(), OpportunityFindingCounts{})
	var recall *OpportunityFindingSummaryItem
	for index := range summary {
		if summary[index].Label == "Competitive recall" {
			recall = &summary[index]
			break
		}
	}
	if recall == nil || recall.Detail != "0 candidate pages from 2 search results across 2 queries; top miss reason: non_competitive_path" || recall.Tone != "amber" {
		t.Fatalf("competitive recall miss item = %+v", recall)
	}
}

func TestOpportunityFindingRunCountsPathProbeCandidatesWithoutInflatingSearchResults(t *testing.T) {
	run := &OpportunityFindingRun{Status: "completed"}
	attachOpportunityFindingStageProgress(run, []db.OpportunityFindingStageCheckpoint{
		{Stage: "evidence_refresh", StageOrder: 1, Status: "succeeded", OutputSummary: []byte(`{"ai_discovery":{"competitive_recall_evidence":[
			{"source":"search_result","query":"free social content workflow tools","url":"https://postsyncer.com/","host":"postsyncer.com","provider_order":1,"seed_candidate":false,"reason":"non_competitive_path"},
			{"source":"path_probe","query":"free social content workflow tools","url":"https://postsyncer.com/tools","host":"postsyncer.com","provider_order":1,"seed_candidate":true,"reason":"competitive_path_probe_url"}
		]}}`)},
	})
	if run.CompetitiveRecallQueryCount != 1 || run.CompetitiveRecallResultCount != 1 || run.CompetitiveRecallSeedCandidateCount != 1 {
		t.Fatalf("competitive recall counters = queries:%d results:%d seeds:%d", run.CompetitiveRecallQueryCount, run.CompetitiveRecallResultCount, run.CompetitiveRecallSeedCandidateCount)
	}

	summary := opportunityFindingSummary(nil, run, config.Default(), OpportunityFindingCounts{})
	var recall *OpportunityFindingSummaryItem
	for index := range summary {
		if summary[index].Label == "Competitive recall" {
			recall = &summary[index]
			break
		}
	}
	if recall == nil || recall.Detail != "1 candidate page from 1 search result across 1 query" || recall.Tone != "green" {
		t.Fatalf("path probe recall summary = %+v", recall)
	}
}

func TestOpportunityFindingRunHighlightsTopicPathProbeCandidates(t *testing.T) {
	run := &OpportunityFindingRun{Status: "completed"}
	attachOpportunityFindingStageProgress(run, []db.OpportunityFindingStageCheckpoint{
		{Stage: "evidence_refresh", StageOrder: 1, Status: "succeeded", OutputSummary: []byte(`{"ai_discovery":{"competitive_recall_evidence":[
			{"source":"search_result","query":"free social media caption generator tools","url":"https://postsyncer.com/","host":"postsyncer.com","provider_order":1,"seed_candidate":false,"reason":"non_competitive_path"},
			{"source":"path_probe","query":"free social media caption generator tools","url":"https://postsyncer.com/tools/social-media-caption-generator","host":"postsyncer.com","provider_order":1,"seed_candidate":true,"reason":"competitive_topic_path_probe_url"}
		]}}`)},
	})
	if run.CompetitiveRecallQueryCount != 1 || run.CompetitiveRecallResultCount != 1 || run.CompetitiveRecallSeedCandidateCount != 1 || run.CompetitiveRecallTopicProbeCount != 1 {
		t.Fatalf("competitive recall counters = queries:%d results:%d seeds:%d topic probes:%d", run.CompetitiveRecallQueryCount, run.CompetitiveRecallResultCount, run.CompetitiveRecallSeedCandidateCount, run.CompetitiveRecallTopicProbeCount)
	}

	summary := opportunityFindingSummary(nil, run, config.Default(), OpportunityFindingCounts{})
	var recall *OpportunityFindingSummaryItem
	for index := range summary {
		if summary[index].Label == "Competitive recall" {
			recall = &summary[index]
			break
		}
	}
	if recall == nil || recall.Detail != "1 candidate page from 1 search result across 1 query; 1 topic path probe" || recall.Tone != "green" {
		t.Fatalf("topic path probe recall summary = %+v", recall)
	}
}

func TestOpportunityFindingRunCountsSiteDiscoveryCandidatesWithoutInflatingSearchResults(t *testing.T) {
	run := &OpportunityFindingRun{Status: "completed"}
	attachOpportunityFindingStageProgress(run, []db.OpportunityFindingStageCheckpoint{
		{Stage: "evidence_refresh", StageOrder: 1, Status: "succeeded", OutputSummary: []byte(`{"ai_discovery":{"competitive_recall_evidence":[
			{"source":"search_result","query":"free social content workflow tools","url":"https://postsyncer.com/","host":"postsyncer.com","provider_order":1,"seed_candidate":false,"reason":"non_competitive_path"},
			{"source":"site_discovery","url":"https://postsyncer.com/tools/social-media-caption-generator","host":"postsyncer.com","seed_candidate":true,"reason":"competitive_site_discovery_url"}
		]}}`)},
	})
	if run.CompetitiveRecallQueryCount != 1 || run.CompetitiveRecallResultCount != 1 || run.CompetitiveRecallSeedCandidateCount != 1 {
		t.Fatalf("competitive recall counters = queries:%d results:%d seeds:%d", run.CompetitiveRecallQueryCount, run.CompetitiveRecallResultCount, run.CompetitiveRecallSeedCandidateCount)
	}
}

func TestOpportunityFindingStagesOnlyAttachToTheirWorkflowRun(t *testing.T) {
	workflow := &db.WorkflowEvent{ID: uuid.New()}
	if opportunityFindingWorkflowOwnsRun(&OpportunityFindingRun{ID: uuid.New()}, workflow) {
		t.Fatal("stale workflow checkpoints attached to a newer standalone analyzer run")
	}
	if !opportunityFindingWorkflowOwnsRun(&OpportunityFindingRun{ID: workflow.ID}, workflow) {
		t.Fatal("workflow checkpoints were not attached to their own run")
	}
}

func TestOpportunityFindingStatusUsesCapabilityAuthority(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	for _, marker := range []string{"func opportunityFindingAISummary", "func opportunityFindingManualMode", "func nextOpportunityFindingAt"} {
		body := functionBody(t, source, marker)
		if strings.Contains(body, "OpportunityFindingSourceMix") || strings.Contains(body, "AIDiscoveryAutomation") {
			t.Fatalf("%s still uses retired product-mode authority", marker)
		}
		if !strings.Contains(body, "GrowthAI") && marker != "func nextOpportunityFindingAt" {
			t.Fatalf("%s does not consume Growth AI capability policy", marker)
		}
	}
}

func TestOpportunityFindingStatusDescribesScheduledOnlyAsManualCapable(t *testing.T) {
	cfg := config.Default()
	cfg.GrowthAIEnabled = true
	cfg.GrowthAIRunPolicy = config.GrowthAIRunPolicyScheduledOnly

	got := opportunityFindingAISummary(cfg)
	if got.Detail != "Scheduled + manual runs" {
		t.Fatalf("scheduled_only AI summary = %q, want manual capability to be explicit", got.Detail)
	}
}

func TestOpportunityFindingStatusDoesNotExposeRetiredSourceModes(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.ToLower(string(raw))
	for _, retired := range []string{"opportunityfindingsourcemix", "aidiscoveryautomation", `json:"source_mix"`, `json:"ai_discovery_automation"`} {
		if strings.Contains(source, retired) {
			t.Fatalf("Opportunity status still exposes retired mode %q", retired)
		}
	}
}

func TestOpportunityFindingRoutesAreMounted(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	routes := string(raw)
	for _, want := range []string{
		`r.Get("/opportunity-finding/status", s.getOpportunityFindingStatus)`,
		`r.Post("/opportunity-finding/run", s.runOpportunityFinding)`,
	} {
		if !strings.Contains(routes, want) {
			t.Fatalf("server routes missing %q", want)
		}
	}
}

func TestRunOpportunityFindingQueuesAIDiscoveryStage(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	source := string(raw)
	body := functionBody(t, source, "func (s *Server) enqueueOpportunityFindingWorkflowEvent")
	for _, want := range []string{
		"EventOpportunityFindingRequested",
		"GrowthAITriggerManual",
		"EnqueueWorkflowEvent",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("runOpportunityFinding must queue the durable AI Discovery stage; missing %q", want)
		}
	}
}

func TestOpportunityFindingWorkflowPayloadIncludesNormalizedSeedURLs(t *testing.T) {
	requestID := uuid.MustParse("018fa58c-ec45-72bb-9a3a-c3cf0c0bd4d0")
	payload, err := opportunityFindingWorkflowPayload(requestID, config.GrowthAITriggerManual, opportunityFindingRunRequest{
		SeedURLs: []string{" https://postsyncer.com/tools ", "", "https://postsyncer.com/tools", "https://postsyncer.com/blog"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		RequestID string   `json:"request_id"`
		Trigger   string   `json:"trigger"`
		SeedURLs  []string `json:"seed_urls"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.RequestID != requestID.String() || decoded.Trigger != string(config.GrowthAITriggerManual) {
		t.Fatalf("payload identity = %#v", decoded)
	}
	want := []string{"https://postsyncer.com/tools", "https://postsyncer.com/blog"}
	if !reflect.DeepEqual(decoded.SeedURLs, want) {
		t.Fatalf("seed_urls = %#v, want %#v", decoded.SeedURLs, want)
	}
}

func TestManualOpportunityFindingUsesDurableWorkflow(t *testing.T) {
	raw, err := os.ReadFile("handlers_seo.go")
	if err != nil {
		t.Fatalf("read handlers_seo.go: %v", err)
	}
	body := functionBody(t, string(raw), "func (s *Server) runOpportunityFinding")
	for _, want := range []string{
		"enqueueOpportunityFindingWorkflowEvent",
		"http.StatusAccepted",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("manual Opportunity Finding must enqueue durable work; missing %q", want)
		}
	}
	enqueueBody := functionBody(t, string(raw), "func (s *Server) enqueueOpportunityFindingWorkflowEvent")
	for _, want := range []string{"EventOpportunityFindingRequested", "pg_advisory_xact_lock", "ActiveOpportunityFindingWorkflowEvent", "EnqueueWorkflowEvent", "tx.Commit"} {
		if !strings.Contains(enqueueBody, want) {
			t.Fatalf("manual Opportunity Finding enqueue must atomically reuse active work; missing %q", want)
		}
	}
	for _, forbidden := range []string{"svc.Sync", "svc.Analyze", "RunAIDiscovery"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("manual request must not run long stage %q in the HTTP context", forbidden)
		}
	}

	statusBody := functionBody(t, string(raw), "func (s *Server) latestOpportunityFindingRun")
	if !strings.Contains(statusBody, "LatestOpportunityFindingWorkflowEvent") {
		t.Fatal("Opportunity Finding status must prefer the durable workflow event")
	}
}

func TestOpportunityFindingRunViewUsesWorkflowLifecycle(t *testing.T) {
	created := time.Date(2026, 7, 11, 10, 0, 0, 0, time.UTC)
	finished := created.Add(42 * time.Second)
	event := db.WorkflowEvent{
		ID: uuid.New(), Status: "pending",
		CreatedAt: pgtype.Timestamptz{Time: created, Valid: true},
	}
	view := opportunityFindingRunView(nil, &event)
	if view == nil || view.Status != "queued" || view.StartedAt == nil || !view.StartedAt.Equal(created) {
		t.Fatalf("queued workflow view = %#v", view)
	}

	analyzerStarted := created.Add(10 * time.Second)
	analyzerFinished := analyzerStarted.Add(5 * time.Second)
	analyzer := &db.SeoRun{
		ID: uuid.New(), Status: "ok",
		StartedAt:  pgtype.Timestamptz{Time: analyzerStarted, Valid: true},
		FinishedAt: pgtype.Timestamptz{Time: analyzerFinished, Valid: true},
	}
	event.Status = "running"
	event.LockedAt = pgtype.Timestamptz{Time: created.Add(time.Second), Valid: true}
	view = opportunityFindingRunView(analyzer, &event)
	if view.Status != "running" || view.ID != event.ID {
		t.Fatalf("active workflow was hidden by its newer Signal Scan analyzer row: %#v", view)
	}

	event.Status = "succeeded"
	event.ProcessedAt = pgtype.Timestamptz{Time: finished, Valid: true}
	event.UpdatedAt = pgtype.Timestamptz{Time: finished, Valid: true}
	view = opportunityFindingRunView(analyzer, &event)
	if view.Status != "completed" || view.FinishedAt == nil || view.DurationMs != 41_000 {
		t.Fatalf("completed workflow view = %#v", view)
	}

	errText := "provider timeout"
	event.Status = "dead"
	event.Error = &errText
	event.ProcessedAt = pgtype.Timestamptz{}
	event.UpdatedAt = pgtype.Timestamptz{Time: finished, Valid: true}
	view = opportunityFindingRunView(nil, &event)
	if view.Status != "failed" || view.Error == nil || *view.Error != errText || view.FinishedAt == nil {
		t.Fatalf("failed workflow view = %#v", view)
	}
}

func functionBody(t *testing.T, source, marker string) string {
	t.Helper()
	start := strings.Index(source, marker)
	if start == -1 {
		t.Fatalf("missing %s", marker)
	}
	open := strings.Index(source[start:], "{")
	if open == -1 {
		t.Fatalf("missing opening brace for %s", marker)
	}
	pos := start + open
	depth := 0
	for i := pos; i < len(source); i++ {
		switch source[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return source[pos+1 : i]
			}
		}
	}
	t.Fatalf("missing closing brace for %s", marker)
	return ""
}
