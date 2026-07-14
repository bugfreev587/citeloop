package growthradar

import "testing"

func TestFunnelCombinesEveryDiscoveryDisposition(t *testing.T) {
	left := Funnel{
		Sources:  SourceCounts{Scheduled: 4, Succeeded: 2, Skipped: 1, Failed: 1},
		Evidence: EvidenceCounts{Added: 8, Reused: 2}, Terms: TermCounts{Accepted: 5, Rejected: 2, Held: 1},
		Prompts: PromptCounts{Active: 60, Selected: 10, Rotated: 8, Targeted: 2},
		Status:  "degraded", Reasons: map[string]int{"search_provider_unavailable": 1},
	}
	right := Funnel{Candidates: CandidateCounts{Generated: 9, Duplicates: 1, Conflicts: 1, Watchlist: 2, Filtered: 3, Created: 2}, CostUSD: .12, Status: "ok"}
	combined := CombineFunnels(left, right)
	if combined.Status != "degraded" || combined.Candidates.Created != 2 || combined.Sources.Failed != 1 || combined.CostUSD != .12 {
		t.Fatalf("combined = %+v", combined)
	}
}

func TestFunnelDegradedZeroNeverLooksHealthy(t *testing.T) {
	funnel := NormalizeFunnel(Funnel{Sources: SourceCounts{Scheduled: 3, Failed: 3}, Status: "ok"})
	if funnel.Status != "degraded" || funnel.Reasons["no_usable_evidence"] != 1 {
		t.Fatalf("funnel = %+v", funnel)
	}
	healthy := NormalizeFunnel(Funnel{Sources: SourceCounts{Scheduled: 2, Succeeded: 2}, Evidence: EvidenceCounts{Reused: 4}, Status: "ok"})
	if healthy.Status != "ok" {
		t.Fatalf("healthy = %+v", healthy)
	}
}
