package geo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/aicalls"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/evidence"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	Rows               []ProviderObservation `json:"rows"`
	CostUSD            float64               `json:"cost_usd"`
	IncrementalCostUSD float64               `json:"incremental_cost_usd"`
	PromptTokens       int                   `json:"prompt_tokens"`
	CompletionTokens   int                   `json:"completion_tokens"`
	TotalTokens        int                   `json:"total_tokens"`
}

type answerCallUsage struct {
	PromptTokens, CompletionTokens, TotalTokens int
	CostUSD                                     float64
}

func (u answerCallUsage) add(other answerCallUsage) answerCallUsage {
	return answerCallUsage{
		PromptTokens: u.PromptTokens + other.PromptTokens, CompletionTokens: u.CompletionTokens + other.CompletionTokens,
		TotalTokens: u.TotalTokens + other.TotalTokens, CostUSD: u.CostUSD + other.CostUSD,
	}
}

func answerUsageFromRows(rows []ProviderObservation, costUSD float64) answerCallUsage {
	usage := answerCallUsage{CostUSD: costUSD}
	for _, row := range rows {
		usage.PromptTokens += row.PromptTokens
		usage.CompletionTokens += row.CompletionTokens
		total := row.TotalTokens
		if total == 0 {
			total = row.PromptTokens + row.CompletionTokens
		}
		usage.TotalTokens += total
	}
	return usage
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
		rows, previousUsage := previousAnswerEvidence(ctx)
		completed := make(map[uuid.UUID]struct{}, len(rows))
		for _, row := range rows {
			completed[row.PromptID] = struct{}{}
		}
		pending := make([]db.GeoPrompt, 0, len(prompts))
		for _, prompt := range prompts {
			if _, ok := completed[prompt.ID]; !ok {
				pending = append(pending, prompt)
			}
		}
		newRows, incrementalUsage, providerErr := s.observeAnswerPrompts(ctx, projectID, geoRunID, pending, req, providerName, identity)
		rows = append(rows, newRows...)
		usage := previousUsage.add(incrementalUsage)
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
			RawSnapshot: answerEvidenceSnapshot{Rows: rows, CostUSD: usage.CostUSD, IncrementalCostUSD: incrementalUsage.CostUSD, PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens, TotalTokens: usage.TotalTokens},
			Confidence:  1, Completeness: completeness, Provider: stringPointer(providerName),
			Model: stringPointer(identity.Model), ProviderVersion: stringPointer(identity.ProviderVersion), CallStatus: stringPointer(status), CostUSD: usage.CostUSD,
			PromptTokens: int64(usage.PromptTokens), CompletionTokens: int64(usage.CompletionTokens), TotalTokens: int64(usage.TotalTokens),
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
	incrementalCost := snapshot.IncrementalCostUSD
	if result.Run.AttemptNumber == 1 && incrementalCost == 0 {
		incrementalCost = snapshot.CostUSD
	}
	if result.Reused {
		incrementalCost = 0
	}
	return snapshot.Rows, incrementalCost, observedAt, collectErr
}

func previousAnswerEvidence(ctx context.Context) ([]ProviderObservation, answerCallUsage) {
	for _, observation := range evidence.PreviousObservations(ctx) {
		if observation.Source != "ai_answer" || observation.SourceObservationKey != "aggregate" {
			continue
		}
		var snapshot answerEvidenceSnapshot
		if json.Unmarshal(observation.RawSnapshot, &snapshot) == nil {
			usage := answerCallUsage{PromptTokens: snapshot.PromptTokens, CompletionTokens: snapshot.CompletionTokens, TotalTokens: snapshot.TotalTokens, CostUSD: snapshot.CostUSD}
			if usage.PromptTokens == 0 && usage.CompletionTokens == 0 && usage.TotalTokens == 0 {
				usage = answerUsageFromRows(snapshot.Rows, snapshot.CostUSD)
			}
			return append([]ProviderObservation(nil), snapshot.Rows...), usage
		}
	}
	return nil, answerCallUsage{}
}

func (s Service) observeAnswerPrompts(ctx context.Context, projectID, geoRunID uuid.UUID, prompts []db.GeoPrompt, req ObserveAnswerProviderRequest, providerName string, identity AnswerProviderEvidenceIdentity) ([]ProviderObservation, answerCallUsage, error) {
	promptProvider, supportsPromptCalls := s.AnswerProvider.(PromptAnswerProvider)
	if s.AICallStore == nil || !supportsPromptCalls {
		rows, costUSD, err := s.AnswerProvider.Observe(ctx, prompts)
		return rows, answerUsageFromRows(rows, costUSD), err
	}
	recorder := aicalls.New(s.AICallStore)
	rows := make([]ProviderObservation, 0, len(prompts))
	usage := answerCallUsage{}
	for _, prompt := range prompts {
		fingerprint := aicalls.Fingerprint(llm.CompletionReq{
			Prompt: prompt.PromptText, Model: identity.Model, MaxTokens: 1024, Temperature: 0.2,
		})
		spec := aicalls.Spec{
			ProjectID: projectID, RunID: geoRunID, Stage: "evidence", LinkedObjectType: "geo_prompt", LinkedObjectID: prompt.ID,
			Provider: providerName, Model: identity.Model, PromptVersion: "geo-answer-observation-v2", RequestFingerprint: fingerprint,
		}
		if evidence.IsRetry(ctx) {
			latest, latestErr := recorder.Latest(ctx, spec)
			if latestErr == nil {
				spec.ParentCallID = latest.ID
			} else if !errors.Is(latestErr, pgx.ErrNoRows) {
				return rows, usage, latestErr
			}
		}
		call, err := recorder.Start(ctx, spec)
		if err != nil {
			return rows, usage, err
		}
		row, costUSD, providerErr := promptProvider.ObservePrompt(ctx, prompt)
		tokens := row.TotalTokens
		if tokens == 0 {
			tokens = row.PromptTokens + row.CompletionTokens
		}
		usage = usage.add(answerCallUsage{PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, TotalTokens: tokens, CostUSD: costUSD})
		status, errorCode := "ok", ""
		if providerErr != nil {
			status, errorCode = "failed", "provider_failure"
			if errors.Is(providerErr, ErrInvalidAnswerProviderResponse) {
				errorCode = "invalid_response"
			}
		}
		finishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		_, finishErr := recorder.Finish(finishCtx, call.ID, projectID, aicalls.Finish{
			Status: status, ErrorCode: errorCode, ResolvedProvider: providerName, ResolvedModel: identity.Model,
			PromptTokens: row.PromptTokens, CompletionTokens: row.CompletionTokens, TotalTokens: tokens, CostUSD: costUSD,
		})
		cancel()
		if finishErr != nil {
			return rows, usage, errors.Join(providerErr, finishErr)
		}
		if providerErr != nil {
			return rows, usage, providerErr
		}
		rows = append(rows, row)
	}
	return rows, usage, nil
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
	}, func(collectorCtx context.Context) ([]evidence.Observation, error) {
		if s.AICallStore != nil {
			recorder := aicalls.New(s.AICallStore)
			for _, prompt := range prompts {
				spec := aicalls.Spec{
					ProjectID: projectID, RunID: geoRunID, Stage: "evidence", LinkedObjectType: "geo_prompt", LinkedObjectID: prompt.ID,
					Provider: "provider_unavailable", Model: "none", PromptVersion: "geo-answer-observation-v2",
					RequestFingerprint: aicalls.Fingerprint(llm.CompletionReq{Prompt: prompt.PromptText, Model: "none"}),
				}
				if evidence.IsRetry(collectorCtx) {
					latest, latestErr := recorder.Latest(collectorCtx, spec)
					if latestErr == nil {
						spec.ParentCallID = latest.ID
					} else if !errors.Is(latestErr, pgx.ErrNoRows) {
						return nil, latestErr
					}
				}
				_, skipErr := recorder.Skip(collectorCtx, spec, "provider_unavailable")
				if skipErr != nil {
					return nil, skipErr
				}
			}
		}
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
