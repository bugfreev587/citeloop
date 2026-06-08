package geo

import (
	"context"
	"net/http"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	AgentExternalSurfaceMonitor = "geo_external_surface_monitor"
	ExternalSurfaceMonitorUA    = "CiteLoop GEO external surface monitor"
)

type MonitorExternalSurfacesRequest struct {
	Limit int32 `json:"limit,omitempty"`
}

type MonitorExternalSurfacesResult struct {
	Run             db.GeoRun               `json:"run"`
	Surfaces        []db.GeoExternalSurface `json:"surfaces"`
	Checked         int                     `json:"checked"`
	Failed          int                     `json:"failed"`
	DataSourceNotes []string                `json:"data_source_notes"`
}

func (s Service) MonitorExternalSurfaces(ctx context.Context, projectID uuid.UUID, req MonitorExternalSurfacesRequest) (MonitorExternalSurfacesResult, error) {
	now := s.now()
	run, err := s.Q.StartGEORun(ctx, db.StartGEORunParams{
		ProjectID: projectID,
		Agent:     AgentExternalSurfaceMonitor,
		Provider:  ProviderHonestProbe,
		StartedAt: pgutil.TS(now),
		Input:     jsonBytes(req),
	})
	if err != nil {
		return MonitorExternalSurfacesResult{}, err
	}
	result := MonitorExternalSurfacesResult{
		Run:             run,
		DataSourceNotes: []string{"honest_http_probe", "no_login_or_private_account_scraping"},
	}
	finish := func(status string, output any, runErr error) (MonitorExternalSurfacesResult, error) {
		var errText *string
		if runErr != nil {
			message := runErr.Error()
			errText = &message
		}
		finished, finishErr := s.Q.FinishGEORun(ctx, db.FinishGEORunParams{
			ID:         run.ID,
			ProjectID:  projectID,
			Status:     status,
			FinishedAt: pgutil.TS(s.now()),
			Output:     jsonBytes(output),
			Error:      errText,
			CostUsd:    pgtype.Numeric{},
		})
		if finishErr == nil {
			result.Run = finished
		}
		if runErr != nil && result.Checked == 0 {
			return result, runErr
		}
		return result, finishErr
	}

	surfaces, err := s.Q.ListGEOExternalSurfaces(ctx, db.ListGEOExternalSurfacesParams{ProjectID: projectID})
	if err != nil {
		return finish("error", result, err)
	}
	surfaces = sampleSurfaces(surfaces, req.Limit)
	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	for _, surface := range surfaces {
		status := s.probeExternalSurface(ctx, client, surface.Url)
		if status == nil {
			result.Failed++
		}
		row, err := s.Q.UpsertGEOExternalSurface(ctx, db.UpsertGEOExternalSurfaceParams{
			ProjectID:          projectID,
			Url:                surface.Url,
			NormalizedUrl:      surface.NormalizedUrl,
			Platform:           surface.Platform,
			SurfaceType:        surface.SurfaceType,
			OwnerType:          surface.OwnerType,
			CanonicalTargetUrl: surface.CanonicalTargetUrl,
			BacklinkState:      surface.BacklinkState,
			LastHttpStatus:     status,
			LastCitedAt:        surface.LastCitedAt,
		})
		if err != nil {
			return finish("error", result, err)
		}
		result.Checked++
		result.Surfaces = append(result.Surfaces, row)
	}
	status := "ok"
	if result.Checked == 0 || result.Failed > 0 {
		status = "degraded"
	}
	return finish(status, result, nil)
}

func (s Service) probeExternalSurface(ctx context.Context, client *http.Client, rawURL string) *int32 {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", ExternalSurfaceMonitorUA)
	res, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer res.Body.Close()
	status := int32(res.StatusCode)
	return &status
}

func sampleSurfaces(surfaces []db.GeoExternalSurface, limit int32) []db.GeoExternalSurface {
	if limit <= 0 || int(limit) >= len(surfaces) {
		return surfaces
	}
	return surfaces[:limit]
}
