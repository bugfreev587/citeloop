package geo

import (
	"testing"
	"time"

	"github.com/citeloop/citeloop/internal/evidence"
)

func TestCrawlerEvidenceQualityDoesNotTreatUncheckedAsHealthy(t *testing.T) {
	state, completeness, confidence, status, failed := crawlerEvidenceQuality([]AuditResult{{PageURL: "https://example.com/a", AccessState: AccessTimeout}}, 0)
	if state != evidence.StateMissing || completeness != 0 || confidence != 0 || status != "failed" || len(failed) != 1 {
		t.Fatalf("all-timeout quality = %q %.2f %.2f %q %#v", state, completeness, confidence, status, failed)
	}
	state, completeness, confidence, status, _ = crawlerEvidenceQuality([]AuditResult{
		{PageURL: "https://example.com/a", AccessState: AccessOK},
		{PageURL: "https://example.com/b", AccessState: AccessError},
	}, 1)
	if state != evidence.StateObserved || completeness != 1.0/3.0 || confidence != 1.0/3.0 || status != "partial" {
		t.Fatalf("partial quality = %q %.4f %.4f %q", state, completeness, confidence, status)
	}
}

func TestAnswerUsageFallsBackPerRowWhenProviderOmitsTotal(t *testing.T) {
	usage := answerUsageFromRows([]ProviderObservation{
		{PromptTokens: 3, CompletionTokens: 2, TotalTokens: 5},
		{PromptTokens: 7, CompletionTokens: 4},
	}, 0.03)
	if usage.PromptTokens != 10 || usage.CompletionTokens != 6 || usage.TotalTokens != 16 || usage.CostUSD != 0.03 {
		t.Fatalf("usage=%+v", usage)
	}
}

func TestAnswerEvidenceUsesWeeklyFreshnessBucket(t *testing.T) {
	got := startOfEvidenceWeek(time.Date(2026, 7, 12, 14, 30, 0, 0, time.UTC))
	want := time.Date(2026, 7, 6, 0, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("week start = %s, want %s", got, want)
	}
}

func TestAnswerProviderEvidenceIdentityIncludesModelAndEndpointVersion(t *testing.T) {
	first := NewPerplexityProvider("key", "https://api.example/v1", "sonar-a", nil).EvidenceIdentity()
	second := NewPerplexityProvider("key", "https://api.example/v2", "sonar-b", nil).EvidenceIdentity()
	if first.Model == second.Model || first.ProviderVersion == second.ProviderVersion {
		t.Fatalf("provider identities collapsed: first=%#v second=%#v", first, second)
	}
}
