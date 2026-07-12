package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/evidence"
	"github.com/google/uuid"
)

func (s Service) collectCrawlerAuditEvidence(ctx context.Context, projectID, geoRunID uuid.UUID, siteURL string, urls, omittedURLs, userAgents []string, now time.Time) ([]AuditResult, error) {
	if s.EvidenceStore == nil {
		rows := Auditor{HTTPClient: s.HTTPClient, Now: s.Now}.Audit(ctx, AuditRequest{SiteURL: siteURL, URLs: urls, TargetUserAgents: userAgents})
		for i := range rows {
			rows[i].ObservedAt = now
		}
		return rows, nil
	}
	day := now.UTC()
	all := make([]AuditResult, 0, len(urls)*len(userAgents))
	for _, userAgent := range userAgents {
		result, err := evidence.NewService(s.EvidenceStore).Collect(ctx, evidence.Request{
			ProjectID: projectID, Source: "crawl", NormalizedTarget: siteURL, TargetKind: "site",
			WindowStart: &day, WindowEnd: &day, RequestedBy: "shared_doctor_opportunities", Now: now,
			ConsumerType: "geo_run", ConsumerID: geoRunID,
			CollectionSpec: map[string]any{
				"user_agent": userAgent, "probe_user_agent": HonestProbeUserAgent,
				"http_method": "GET", "render_mode": "http", "urls": urls, "omitted_urls": omittedURLs,
				"sampling_policy":       "site_plus_requested_plus_published_v1",
				"normalization_version": "geo-crawler-url/v1",
			},
		}, func(ctx context.Context) ([]evidence.Observation, error) {
			rows := Auditor{HTTPClient: s.HTTPClient, Now: s.Now}.Audit(ctx, AuditRequest{SiteURL: siteURL, URLs: urls, TargetUserAgents: []string{userAgent}})
			for i := range rows {
				rows[i].ObservedAt = now
			}
			state, completeness, confidence, callStatus, failedURLs := crawlerEvidenceQuality(rows, len(omittedURLs))
			observations := []evidence.Observation{{
				Key: userAgent, State: state, Facts: map[string]any{"results": len(rows), "user_agent": userAgent, "failed_urls": failedURLs},
				RawSnapshot: rows, Confidence: confidence, Completeness: completeness,
				Provider: stringPointer(ProviderHonestProbe), CallStatus: stringPointer(callStatus),
				ObservedAt: now,
			}}
			if len(omittedURLs) > 0 {
				observations = append(observations, evidence.Observation{
					Key: userAgent + ":unchecked", State: evidence.StateMissing,
					Facts:       map[string]any{"coverage_gap": true, "reason": "url_cap", "unchecked_urls": omittedURLs},
					RawSnapshot: map[string]any{"unchecked_urls": omittedURLs}, Confidence: 0, Completeness: 0,
					Provider: stringPointer(ProviderHonestProbe), CallStatus: stringPointer("skipped"), ErrorCode: stringPointer("url_cap"), ObservedAt: now,
				})
			}
			return observations, nil
		})
		if err != nil {
			return all, err
		}
		if len(result.Observations) == 0 {
			return all, fmt.Errorf("crawler evidence run %s has no observations", result.Run.ID)
		}
		rows := []AuditResult{}
		var aggregate *db.EvidenceObservation
		for i := range result.Observations {
			if result.Observations[i].SourceObservationKey == userAgent {
				aggregate = &result.Observations[i]
				break
			}
		}
		if aggregate == nil {
			return all, fmt.Errorf("crawler evidence run %s is missing aggregate observation", result.Run.ID)
		}
		if err := json.Unmarshal(aggregate.RawSnapshot, &rows); err != nil {
			return all, err
		}
		all = append(all, rows...)
	}
	return all, nil
}

func crawlerEvidenceQuality(rows []AuditResult, omittedCount int) (string, float64, float64, string, []string) {
	if len(rows) == 0 {
		return evidence.StateMissing, 0, 0, "failed", []string{}
	}
	successful := 0
	failedURLs := []string{}
	for _, row := range rows {
		authoritativeRobots := row.EvidenceType == EvidenceRobotsStatic && (row.RobotsState == RobotsAllowed || row.RobotsState == RobotsDisallowed)
		if authoritativeRobots || (row.AccessState != AccessTimeout && row.AccessState != AccessError) {
			successful++
			continue
		}
		failedURLs = append(failedURLs, row.PageURL)
	}
	completeness := float64(successful) / float64(len(rows)+omittedCount)
	if successful == 0 {
		return evidence.StateMissing, 0, 0, "failed", failedURLs
	}
	if successful < len(rows)+omittedCount {
		return evidence.StateObserved, completeness, completeness, "partial", failedURLs
	}
	return evidence.StateObserved, 1, 1, "ok", failedURLs
}

func stringPointer(value string) *string { return &value }

type answerEvidenceSnapshot struct {
	Rows    []ProviderObservation `json:"rows"`
	CostUSD float64               `json:"cost_usd"`
}

func (s Service) collectAnswerProviderEvidence(ctx context.Context, projectID, geoRunID uuid.UUID, prompts []db.GeoPrompt, req ObserveAnswerProviderRequest, providerName string, now time.Time) ([]ProviderObservation, float64, time.Time, error) {
	if s.EvidenceStore == nil {
		rows, cost, err := s.AnswerProvider.Observe(ctx, prompts)
		return rows, cost, now, err
	}
	promptSpec := make([]map[string]any, 0, len(prompts))
	promptIDs := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		promptIDs = append(promptIDs, prompt.ID.String())
		promptSpec = append(promptSpec, map[string]any{"id": prompt.ID, "text": prompt.PromptText, "locale": prompt.Locale, "intent": prompt.IntentType, "target_topic": prompt.TargetTopic})
	}
	target := "prompt-set:" + strings.Join(promptIDs, ",")
	identity := s.AnswerProvider.EvidenceIdentity()
	weekStart := startOfEvidenceWeek(now)
	weekEnd := weekStart.AddDate(0, 0, 6)
	result, collectErr := evidence.NewService(s.EvidenceStore).Collect(ctx, evidence.Request{
		ProjectID: projectID, Source: "ai_answer", NormalizedTarget: target, TargetKind: "prompt",
		WindowStart: &weekStart, WindowEnd: &weekEnd, RequestedBy: "opportunities", Now: now,
		ConsumerType: "geo_run", ConsumerID: geoRunID,
		CollectionSpec: map[string]any{
			"provider": providerName, "engine": req.Engine, "locale": req.Locale,
			"model": identity.Model, "provider_version": identity.ProviderVersion,
			"prompts": promptSpec, "max_prompts": req.MaxPrompts, "budget_usd": req.BudgetUSD,
			"sampling_policy": "priority_order_v1", "normalization_version": "geo-answer-observation/v1",
		},
	}, func(ctx context.Context) ([]evidence.Observation, error) {
		rows, costUSD, providerErr := s.AnswerProvider.Observe(ctx, prompts)
		if len(rows) == 0 && providerErr != nil {
			return nil, providerErr
		}
		status := "ok"
		completeness := 1.0
		if providerErr != nil || len(rows) < len(prompts) {
			status = "partial"
			completeness = float64(len(rows)) / float64(max(len(prompts), 1))
		}
		return []evidence.Observation{{
			Key: "aggregate", State: evidence.StateObserved,
			Facts:       map[string]any{"prompt_count": len(prompts), "observation_count": len(rows)},
			RawSnapshot: answerEvidenceSnapshot{Rows: rows, CostUSD: costUSD},
			Confidence:  1, Completeness: completeness, Provider: stringPointer(providerName),
			Model: stringPointer(identity.Model), ProviderVersion: stringPointer(identity.ProviderVersion), CallStatus: stringPointer(status), CostUSD: costUSD,
		}}, providerErr
	})
	if len(result.Observations) == 0 {
		return nil, 0, time.Time{}, collectErr
	}
	snapshot := answerEvidenceSnapshot{}
	if err := json.Unmarshal(result.Observations[0].RawSnapshot, &snapshot); err != nil {
		return nil, 0, time.Time{}, err
	}
	if collectErr == nil && result.Run.Status == "partial" && result.Run.ErrorSummary != nil {
		collectErr = fmt.Errorf("reused partial answer evidence: %s", *result.Run.ErrorSummary)
	}
	observedAt := result.Observations[0].ObservedAt.Time
	incrementalCost := snapshot.CostUSD
	if result.Reused {
		incrementalCost = 0
	}
	return snapshot.Rows, incrementalCost, observedAt, collectErr
}

func (s Service) recordAnswerProviderUnavailableEvidence(ctx context.Context, projectID, geoRunID uuid.UUID, prompts []db.GeoPrompt, req ObserveAnswerProviderRequest, now time.Time) (time.Time, error) {
	if s.EvidenceStore == nil {
		return now, nil
	}
	promptIDs := make([]string, 0, len(prompts))
	for _, prompt := range prompts {
		promptIDs = append(promptIDs, prompt.ID.String())
	}
	weekStart := startOfEvidenceWeek(now)
	weekEnd := weekStart.AddDate(0, 0, 6)
	result, err := evidence.NewService(s.EvidenceStore).Collect(ctx, evidence.Request{
		ProjectID: projectID, Source: "ai_answer", NormalizedTarget: "prompt-set:" + strings.Join(promptIDs, ","), TargetKind: "prompt",
		WindowStart: &weekStart, WindowEnd: &weekEnd, RequestedBy: "opportunities", Now: now,
		ConsumerType: "geo_run", ConsumerID: geoRunID,
		CollectionSpec: map[string]any{"provider": "provider_unavailable", "engine": req.Engine, "locale": req.Locale, "prompt_ids": promptIDs, "normalization_version": "geo-answer-observation/v1"},
	}, func(context.Context) ([]evidence.Observation, error) {
		return []evidence.Observation{{
			Key: "aggregate", State: evidence.StateProviderUnavailable,
			Facts:       map[string]any{"prompt_count": len(prompts), "coverage_gap": true},
			RawSnapshot: map[string]any{"provider_available": false}, Confidence: 0, Completeness: 0,
			Provider: stringPointer("provider_unavailable"), CallStatus: stringPointer("skipped"), ErrorCode: stringPointer("provider_unavailable"),
		}}, nil
	})
	if err != nil || len(result.Observations) == 0 {
		return time.Time{}, err
	}
	return result.Observations[0].ObservedAt.Time, nil
}

func startOfEvidenceWeek(value time.Time) time.Time {
	value = value.UTC()
	day := time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, time.UTC)
	offset := (int(day.Weekday()) + 6) % 7
	return day.AddDate(0, 0, -offset)
}
