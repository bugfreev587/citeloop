package seo

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/citeloop/citeloop/internal/evidence"
	"github.com/citeloop/citeloop/internal/googledata"
	"github.com/google/uuid"
)

func (s Service) fetchSearchConsoleEvidence(ctx context.Context, projectID, seoRunID uuid.UUID, siteURL string, start, end time.Time) (googledata.SearchConsoleData, bool, error) {
	req := evidence.Request{
		ProjectID: projectID, Source: "gsc", NormalizedTarget: siteURL, TargetKind: "integration",
		WindowStart: &start, WindowEnd: &end, RequestedBy: "shared_doctor_opportunities", Now: s.now(),
		ConsumerType: "seo_run", ConsumerID: seoRunID,
		CollectionSpec: map[string]any{
			"provider": "google_search_console", "api": "searchanalytics.query/v1",
			"dimensions": []string{"date", "page", "query", "country", "device", "searchAppearance"},
			"filters":    map[string]any{"site_url": siteURL}, "row_limit": 25000,
			"normalization_version": "seo-url-normalization/v1", "stabilization_lag_days": 2,
		},
	}
	result, err := evidence.NewService(s.Q).Collect(ctx, req, func(ctx context.Context) ([]evidence.Observation, error) {
		data, fetchErr := s.GoogleData.FetchSearchConsole(ctx, googledata.SearchConsoleRequest{SiteURL: siteURL, StartDate: start, EndDate: end, RowLimit: 25000})
		availableRows := len(data.PageRows) + len(data.QueryRows) + len(data.AppearanceRows)
		if fetchErr != nil && availableRows == 0 {
			return nil, fetchErr
		}
		callStatus := "ok"
		completeness := 1.0
		if fetchErr != nil {
			callStatus, completeness = "partial", 0.5
		}
		return []evidence.Observation{{
			Key: "aggregate", State: evidence.StateObserved,
			Facts:       map[string]any{"page_rows": len(data.PageRows), "query_rows": len(data.QueryRows), "appearance_rows": len(data.AppearanceRows)},
			RawSnapshot: data, Confidence: 1, Completeness: completeness, CallStatus: &callStatus,
			PrivacyState: "project_aggregate", PermissionState: "authorized",
		}}, fetchErr
	})
	if err != nil && len(result.Observations) == 0 {
		return googledata.SearchConsoleData{}, result.Reused, err
	}
	if len(result.Observations) == 0 {
		return googledata.SearchConsoleData{}, result.Reused, fmt.Errorf("GSC evidence run %s has no observation", result.Run.ID)
	}
	var data googledata.SearchConsoleData
	if err := json.Unmarshal(result.Observations[0].RawSnapshot, &data); err != nil {
		return data, result.Reused, err
	}
	return data, result.Reused, err
}

func (s Service) fetchAnalyticsEvidence(ctx context.Context, projectID, seoRunID uuid.UUID, propertyID string, start, end time.Time) ([]googledata.AnalyticsPageRow, bool, error) {
	req := evidence.Request{
		ProjectID: projectID, Source: "ga4", NormalizedTarget: propertyID, TargetKind: "integration",
		WindowStart: &start, WindowEnd: &end, RequestedBy: "shared_doctor_opportunities", Now: s.now(),
		ConsumerType: "seo_run", ConsumerID: seoRunID,
		CollectionSpec: map[string]any{
			"provider": "google_analytics_4", "api": "properties.runReport/v1beta",
			"dimensions":  []string{"date", "pagePath"},
			"metrics":     []string{"sessions", "engagedSessions", "keyEvents"},
			"property_id": propertyID, "row_limit": 25000,
			"normalization_version": "seo-url-normalization/v1",
		},
	}
	result, err := evidence.NewService(s.Q).Collect(ctx, req, func(ctx context.Context) ([]evidence.Observation, error) {
		rows, fetchErr := s.GoogleData.FetchAnalytics(ctx, googledata.AnalyticsRequest{PropertyID: propertyID, StartDate: start, EndDate: end, RowLimit: 25000})
		if fetchErr != nil {
			return nil, fetchErr
		}
		return []evidence.Observation{{
			Key: "aggregate", State: evidence.StateObserved, Facts: map[string]any{"rows": len(rows)},
			RawSnapshot: rows, Confidence: 1, Completeness: 1,
			PrivacyState: "project_aggregate", PermissionState: "authorized",
		}}, nil
	})
	if err != nil {
		return nil, result.Reused, err
	}
	if len(result.Observations) == 0 {
		return nil, result.Reused, fmt.Errorf("GA4 evidence run %s has no observation", result.Run.ID)
	}
	rows := []googledata.AnalyticsPageRow{}
	if err := json.Unmarshal(result.Observations[0].RawSnapshot, &rows); err != nil {
		return nil, result.Reused, err
	}
	return rows, result.Reused, nil
}

func (s Service) recordGoogleCoverageGap(ctx context.Context, projectID, seoRunID uuid.UUID, source, target, state, reason, permissionState string) error {
	if target == "" {
		target = "project:" + projectID.String()
	}
	callStatus := "skipped"
	errorCode := reason
	_, err := evidence.NewService(s.Q).Collect(ctx, evidence.Request{
		ProjectID: projectID, Source: source, NormalizedTarget: target, TargetKind: "integration",
		RequestedBy: "shared_doctor_opportunities", Now: s.now(),
		ConsumerType: "seo_run", ConsumerID: seoRunID,
		CollectionSpec: map[string]any{"provider": source, "availability_state": state, "reason": reason, "normalization_version": "google-integration-coverage/v1"},
	}, func(context.Context) ([]evidence.Observation, error) {
		return []evidence.Observation{{
			Key: "coverage", State: state, Facts: map[string]any{"coverage_gap": true, "reason": reason},
			RawSnapshot: map[string]any{"available": false}, Confidence: 0, Completeness: 0,
			Provider: &source, CallStatus: &callStatus, ErrorCode: &errorCode,
			PrivacyState: "project_aggregate", PermissionState: permissionState,
		}}, nil
	})
	return err
}
