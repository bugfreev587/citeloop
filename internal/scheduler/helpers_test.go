package scheduler

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
)

func TestCeilDiv(t *testing.T) {
	cases := []struct {
		a, b, want int
	}{
		{3 * 5, 7, 3},  // cadence 3/wk, buffer 5d -> ceil(15/7)=3
		{3 * 7, 7, 3},  // exactly a week
		{3 * 14, 7, 6}, // two weeks
		{3 * 0, 7, 0},  // buffer 0 -> stock nothing (operator-driven)
		{0, 7, 0},
		{1, 7, 1},
	}
	for _, c := range cases {
		if got := ceilDiv(c.a, c.b); got != c.want {
			t.Errorf("ceilDiv(%d,%d) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestSchedulerExposesNotificationTick(t *testing.T) {
	var _ func(*Scheduler, context.Context) = (*Scheduler).TickNotifications
}

func TestSchedulerExposesWorkflowTick(t *testing.T) {
	var _ func(*Scheduler, context.Context) = (*Scheduler).TickWorkflow
}

func TestSchedulerExposesReviewOverdueTick(t *testing.T) {
	var _ func(*Scheduler, context.Context) = (*Scheduler).TickReviewOverdue
}

func TestSchedulerExposesSEOTick(t *testing.T) {
	var _ func(*Scheduler, context.Context) = (*Scheduler).TickSEO
}

func TestSchedulerExposesSEODoctorTick(t *testing.T) {
	var _ func(*Scheduler, context.Context) = (*Scheduler).TickSEODoctor
}

func TestSchedulerExposesGEOTick(t *testing.T) {
	var _ func(*Scheduler, context.Context) = (*Scheduler).TickGEO
}

func TestSchedulerExposesContextRefreshTick(t *testing.T) {
	var _ func(*Scheduler, context.Context) = (*Scheduler).TickContextRefresh
}

func TestStartRegistersNotificationTick(t *testing.T) {
	raw, err := os.ReadFile("helpers.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "TickNotifications") || !strings.Contains(source, "@every 10s") {
		t.Fatal("Start must register TickNotifications every 10 seconds")
	}
	if !strings.Contains(source, "TickWorkflow") || !strings.Contains(source, "@every 10s") {
		t.Fatal("Start must register TickWorkflow every 10 seconds")
	}
	if !strings.Contains(source, "TickMeasurements") || !strings.Contains(source, "@every 1h") {
		t.Fatal("Start must register TickMeasurements every hour")
	}
	if !strings.Contains(source, "TickReviewOverdue") || !strings.Contains(source, "@every 30m") {
		t.Fatal("Start must register TickReviewOverdue every 30 minutes")
	}
	if !strings.Contains(source, "TickSEO") || !strings.Contains(source, "0 3 * * *") {
		t.Fatal("Start must register TickSEO daily after generation")
	}
	if !strings.Contains(source, "TickSEODoctor") || !strings.Contains(source, "@weekly") {
		t.Fatal("Start must register TickSEODoctor weekly")
	}
	if !strings.Contains(source, "TickGEO") || !strings.Contains(source, "@weekly") {
		t.Fatal("Start must register TickGEO weekly")
	}
	if !strings.Contains(source, "TickContextRefresh") || !strings.Contains(source, "@weekly") {
		t.Fatal("Start must register TickContextRefresh weekly")
	}
}

func TestStartRegistersFrequentGenerationTick(t *testing.T) {
	raw, err := os.ReadFile("helpers.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "TickGenerate") || !strings.Contains(source, "@every 5m") {
		t.Fatal("Start must register TickGenerate every 5 minutes so automatic drafting does not wait for the daily cron")
	}
	if strings.Contains(source, "0 2 * * *") {
		t.Fatal("Start must not leave automatic drafting on the old daily 02:00 cron")
	}
}

func TestDailySEOTickRunsAutomaticAIDiscoveryWhenConfigured(t *testing.T) {
	raw, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := functionBody(t, string(raw), "func (s *Scheduler) executeOpportunityFindingStage")
	for _, want := range []string{
		"OpportunityFindingStages(scheduled)",
		"opportunityfinding.RefreshAIDiscoveryEvidence",
		"opportunityfinding.MaterializeAIDiscoveryHypotheses",
		"runner.Sync",
		"runner.Analyze",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("daily opportunity finding must include configured AI Discovery; missing %q", want)
		}
	}
}

func TestOpportunityFindingTriggerComesFromDurableEventPayload(t *testing.T) {
	event := db.WorkflowEvent{Payload: []byte(`{"trigger":"scheduled"}`)}
	trigger, scheduled, err := opportunityFindingTrigger(event)
	if err != nil {
		t.Fatal(err)
	}
	if trigger != config.GrowthAITriggerScheduled || !scheduled {
		t.Fatalf("trigger=%q scheduled=%v", trigger, scheduled)
	}
	event.Payload = []byte(`{"trigger":"manual"}`)
	trigger, scheduled, err = opportunityFindingTrigger(event)
	if err != nil || trigger != config.GrowthAITriggerManual || scheduled {
		t.Fatalf("manual trigger=%q scheduled=%v err=%v", trigger, scheduled, err)
	}
}

func TestOpportunityFindingStepErrorsAreDeterministicAndRetryable(t *testing.T) {
	err := opportunityFindingStepErrors(map[string]string{
		"observe_provider": "provider timeout",
		"crawler_audit":    "crawl unavailable",
	})
	if err == nil || err.Error() != "crawler_audit: crawl unavailable; observe_provider: provider timeout" {
		t.Fatalf("step error = %v", err)
	}
	if opportunityFindingStepErrors(nil) != nil {
		t.Fatal("empty AI Discovery step errors must remain successful")
	}
}

func TestGenerationFailureRequeuesTopic(t *testing.T) {
	raw, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, "resetTopicAfterGenerationFailure") {
		t.Fatal("generateCandidate must reset failed automatic drafts back to a retryable topic status")
	}
	if strings.Contains(source, "leave topic in generating") {
		t.Fatal("generation failure must not strand topics in generating")
	}
}

func TestGenerationReservesBeforeWritingOutsideAdvisoryTransaction(t *testing.T) {
	raw, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	generateBody := functionBody(t, source, "func (s *Scheduler) generateForProject")
	dueBody := functionBody(t, source, "func (s *Scheduler) generateDueScheduledForProject")

	if !strings.Contains(source, "pg_try_advisory_xact_lock") {
		t.Fatal("scheduler generation should use try-advisory locks so ticks do not queue behind long project work")
	}
	for name, body := range map[string]string{
		"generateForProject":             generateBody,
		"generateDueScheduledForProject": dueBody,
	} {
		if !strings.Contains(body, "reserveGenerationCandidates") {
			t.Fatalf("%s must reserve topics in a short transaction before writing", name)
		}
		if strings.Contains(body, "s.Pool.Begin") || strings.Contains(body, "pg_advisory_xact_lock") {
			t.Fatalf("%s must not hold a transaction/advisory lock while writer.Generate runs", name)
		}
		if !strings.Contains(body, "db.New(s.Pool)") || strings.Contains(body, "db.New(tx)") {
			t.Fatalf("%s must build the writer with a pool-backed query object, not the reservation tx query", name)
		}
	}
}

func TestGenerationClearsRejectedDraftRowsBeforeRegeneration(t *testing.T) {
	raw, err := os.ReadFile("scheduler.go")
	if err != nil {
		t.Fatal(err)
	}
	body := functionBody(t, string(raw), "func (s *Scheduler) generateReservedCandidate")
	deletePos := strings.Index(body, "DeleteRecoverableArticlesForTopic")
	generatePos := strings.Index(body, "writer.Generate")
	if deletePos == -1 {
		t.Fatal("reserved generation must clear rejected/generating stale articles before writing")
	}
	if generatePos == -1 {
		t.Fatal("reserved generation must call writer.Generate")
	}
	if deletePos > generatePos {
		t.Fatal("stale article cleanup must happen before writer.Generate to avoid the topic/kind/platform unique index collision")
	}
}

func TestOpportunityPriorityScoreUsesP1AsHighest(t *testing.T) {
	if got, want := priorityFromOpportunityScore(80), int32(2); got != want {
		t.Fatalf("priorityFromOpportunityScore(80) = %d, want %d", got, want)
	}
	if got, want := priorityFromOpportunityScore(0), int32(5); got != want {
		t.Fatalf("priorityFromOpportunityScore(0) = %d, want %d fallback", got, want)
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
