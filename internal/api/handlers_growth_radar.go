package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/growthradar"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/jackc/pgx/v5/pgtype"
)

func (s *Server) getGrowthRadarDiagnostics(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	rows, err := s.Q.ListRecentGrowthRadarRuns(r.Context(), db.ListRecentGrowthRadarRunsParams{ProjectID: projectID, LimitRows: 10})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	latestCycle := latestGrowthRadarCycle(rows)
	watchlist, err := s.Q.ListActiveGrowthRadarWatchlist(r.Context(), db.ListActiveGrowthRadarWatchlistParams{ProjectID: projectID, NowAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	funnels := make([]growthradar.Funnel, 0, len(rows))
	items := make([]map[string]any, 0, len(rows))
	for index, row := range rows {
		var funnel growthradar.Funnel
		if err := json.Unmarshal(row.Funnel, &funnel); err != nil {
			continue
		}
		if index < len(latestCycle) {
			funnels = append(funnels, funnel)
		}
		items = append(items, map[string]any{"id": row.ID, "phase": row.Phase, "status": row.Status, "funnel": funnel, "cost_usd": pgutil.Float(row.CostUsd), "created_at": row.CreatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": growthradar.CombineFunnels(funnels...), "runs": items, "watchlist": watchlist})
}

// Growth Radar persists evidence_refresh and candidate_analysis as separate
// rows for one logical discovery cycle. Rows arrive newest-first. Summaries
// must stop at the first evidence phase so historical failures cannot make a
// later successful cycle look degraded.
func latestGrowthRadarCycle(rows []db.GrowthRadarRun) []db.GrowthRadarRun {
	for index, row := range rows {
		if row.Phase == "evidence_refresh" {
			return rows[:index+1]
		}
	}
	return rows
}
