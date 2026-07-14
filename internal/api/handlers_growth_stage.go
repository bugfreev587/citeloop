package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/growthradar"
	"github.com/citeloop/citeloop/internal/growthstage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type growthStageResponse struct {
	Stage                growthstage.Stage     `json:"stage"`
	StageProfileVersion  string                `json:"stage_profile_version"`
	SettingVersion       int64                 `json:"setting_version"`
	IsDefaultUnconfirmed bool                  `json:"is_default_unconfirmed"`
	Profiles             []growthstage.Profile `json:"profiles"`
	Rescore              *db.GrowthStageEvent  `json:"rescore,omitempty"`
}

type updateGrowthStageRequest struct {
	Stage           growthstage.Stage `json:"stage"`
	ExpectedVersion int64             `json:"expected_version"`
	Reason          string            `json:"reason"`
}

func virtualGrowthStageResponse() growthStageResponse {
	setting := growthstage.DefaultSetting()
	return growthStageResponse{
		Stage: setting.Stage, StageProfileVersion: setting.StageProfileVersion,
		SettingVersion: setting.SettingVersion, IsDefaultUnconfirmed: setting.IsDefaultUnconfirmed,
		Profiles: growthstage.AllProfiles(),
	}
}

func growthStageResponseFromRow(row db.GrowthStageSetting, event *db.GrowthStageEvent) growthStageResponse {
	return growthStageResponse{
		Stage: growthstage.Stage(row.Stage), StageProfileVersion: row.StageProfileVersion,
		SettingVersion: row.SettingVersion, IsDefaultUnconfirmed: row.IsDefaultUnconfirmed,
		Profiles: growthstage.AllProfiles(), Rescore: event,
	}
}

func (s *Server) getGrowthStage(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	row, err := s.Q.GetGrowthStageSetting(r.Context(), projectID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, virtualGrowthStageResponse())
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var event *db.GrowthStageEvent
	if latest, latestErr := s.Q.GetLatestGrowthStageEvent(r.Context(), projectID); latestErr == nil {
		event = &latest
	}
	writeJSON(w, http.StatusOK, growthStageResponseFromRow(row, event))
}

func (s *Server) updateGrowthStage(w http.ResponseWriter, r *http.Request) {
	projectID, err := s.projectID(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad project id")
		return
	}
	var input updateGrowthStageRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid growth stage request")
		return
	}
	profile, err := growthstage.ProfileFor(input.Stage)
	if err != nil || input.ExpectedVersion < 0 {
		writeErr(w, http.StatusBadRequest, "invalid growth stage")
		return
	}
	previous := virtualGrowthStageResponse()
	if current, loadErr := s.Q.GetGrowthStageSetting(r.Context(), projectID); loadErr == nil {
		previous = growthStageResponseFromRow(current, nil)
	} else if !errors.Is(loadErr, pgx.ErrNoRows) {
		writeErr(w, http.StatusInternalServerError, loadErr.Error())
		return
	}
	if input.ExpectedVersion != previous.SettingVersion {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "growth_stage_version_conflict", "current": previous})
		return
	}

	tx, err := s.Pool.Begin(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback(r.Context())
	q := db.New(tx)
	count, err := q.CountActiveGrowthRadarWatchlist(r.Context(), projectID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated, err := q.UpsertGrowthStageSetting(r.Context(), db.UpsertGrowthStageSettingParams{
		ProjectID: projectID, Stage: string(input.Stage), StageProfileVersion: profile.Version,
		SelectedBy: s.ownerID(r), ExpectedVersion: input.ExpectedVersion,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "growth_stage_version_conflict"})
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	event, err := q.CreateGrowthStageEvent(r.Context(), db.CreateGrowthStageEventParams{
		ProjectID: projectID, PreviousStage: string(previous.Stage), NewStage: string(input.Stage),
		PreviousProfileVersion: previous.StageProfileVersion, NewProfileVersion: profile.Version,
		ExpectedSettingVersion: input.ExpectedVersion, CommittedSettingVersion: updated.SettingVersion,
		Actor: s.ownerID(r), Reason: strings.TrimSpace(input.Reason), AffectedWatchlistCount: int32(count),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	rescore := s.rescoreGrowthStageWatchlist(r, projectID, updated, event)
	writeJSON(w, http.StatusOK, growthStageResponseFromRow(updated, &rescore))
}

func (s *Server) rescoreGrowthStageWatchlist(r *http.Request, projectID uuid.UUID, setting db.GrowthStageSetting, event db.GrowthStageEvent) db.GrowthStageEvent {
	ctx := r.Context()
	event, err := s.Q.UpdateGrowthStageEventStatus(ctx, db.UpdateGrowthStageEventStatusParams{
		ID: event.ID, ProjectID: projectID, RescoreStatus: "running", FailureCode: "", FailureDetail: "",
	})
	if err != nil {
		return event
	}
	rows, err := s.Q.ListActiveGrowthRadarWatchlist(ctx, db.ListActiveGrowthRadarWatchlistParams{
		ProjectID: projectID, NowAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true},
	})
	if err == nil {
		for _, row := range rows {
			var snapshot growthradar.Snapshot
			if decodeErr := json.Unmarshal(row.ScoringSnapshot, &snapshot); decodeErr != nil {
				err = decodeErr
				break
			}
			score, scoreErr := growthradar.ScoreCandidateForStage(snapshot, growthstage.Stage(setting.Stage))
			if scoreErr != nil {
				err = scoreErr
				break
			}
			scoreJSON, _ := json.Marshal(score)
			snapshotJSON, _ := json.Marshal(snapshot)
			reason := strings.Join(score.ReasonCodes, ",")
			if reason == "" {
				reason = score.Disposition
			}
			if _, updateErr := s.Q.UpdateGrowthRadarWatchlistStageScore(ctx, db.UpdateGrowthRadarWatchlistStageScoreParams{
				Score: scoreJSON, ScoringSnapshot: snapshotJSON, Reason: reason,
				ProjectID: projectID, CandidateIdentity: row.CandidateIdentity,
				ExpectedSettingVersion: setting.SettingVersion,
			}); updateErr != nil {
				err = updateErr
				break
			}
		}
	}
	status, code, detail := "complete", "", ""
	if err != nil {
		status, code, detail = "failed", "watchlist_rescore_failed", "CiteLoop could not rescore the active watchlist. Retry the stage selection."
	}
	updated, updateErr := s.Q.UpdateGrowthStageEventStatus(ctx, db.UpdateGrowthStageEventStatusParams{
		ID: event.ID, ProjectID: projectID, RescoreStatus: status, FailureCode: code, FailureDetail: detail,
	})
	if updateErr == nil {
		return updated
	}
	return event
}
