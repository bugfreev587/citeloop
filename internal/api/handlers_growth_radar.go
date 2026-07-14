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
	watchlist, err := s.Q.ListActiveGrowthRadarWatchlist(r.Context(), db.ListActiveGrowthRadarWatchlistParams{ProjectID: projectID, NowAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	funnels := make([]growthradar.Funnel, 0, len(rows))
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		var funnel growthradar.Funnel
		if err := json.Unmarshal(row.Funnel, &funnel); err != nil {
			continue
		}
		funnels = append(funnels, funnel)
		items = append(items, map[string]any{"id": row.ID, "phase": row.Phase, "status": row.Status, "funnel": funnel, "cost_usd": pgutil.Float(row.CostUsd), "created_at": row.CreatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": growthradar.CombineFunnels(funnels...), "runs": items, "watchlist": watchlist})
}
