package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/growthradar"
	"github.com/go-chi/chi/v5"
)

func TestGrowthRadarRoutesAreRegistered(t *testing.T) {
	router := (&Server{}).Router()
	want := map[string]bool{"GET /api/projects/{projectID}/opportunities/radar": false, "GET /api/projects/{projectID}/seo/opportunity-finding/radar": false}
	if err := chi.Walk(router.(chi.Routes), func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if _, ok := want[method+" "+route]; ok {
			want[method+" "+route] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for route, found := range want {
		if !found {
			t.Errorf("missing route %s", route)
		}
	}
}

func TestLatestGrowthRadarCycleExcludesHistoricalFailures(t *testing.T) {
	rows := []db.GrowthRadarRun{
		{Phase: "candidate_analysis", Status: "ok", Funnel: json.RawMessage(`{"status":"ok","candidates":{"generated":8,"filtered":8}}`)},
		{Phase: "evidence_refresh", Status: "ok", Funnel: json.RawMessage(`{"status":"ok","evidence":{"added":38},"prompts":{"rotated":8},"sources":{"scheduled":4,"succeeded":4}}`)},
		{Phase: "candidate_analysis", Status: "ok", Funnel: json.RawMessage(`{"status":"ok"}`)},
		{Phase: "evidence_refresh", Status: "degraded", Funnel: json.RawMessage(`{"status":"degraded","sources":{"scheduled":1,"failed":1},"reasons":{"search_evidence":1}}`)},
	}
	cycle := latestGrowthRadarCycle(rows)
	if len(cycle) != 2 {
		t.Fatalf("cycle length = %d, want latest two phases", len(cycle))
	}
	var funnels []growthradar.Funnel
	for _, row := range cycle {
		var funnel growthradar.Funnel
		if err := json.Unmarshal(row.Funnel, &funnel); err != nil {
			t.Fatal(err)
		}
		funnels = append(funnels, funnel)
	}
	summary := growthradar.CombineFunnels(funnels...)
	if summary.Status != "ok" || summary.Evidence.Added != 38 || summary.Prompts.Rotated != 8 || summary.Candidates.Filtered != 8 {
		t.Fatalf("latest summary = %+v", summary)
	}
}
