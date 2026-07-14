package opportunityfinding

import (
	"context"
	"encoding/json"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/geo"
	"github.com/citeloop/citeloop/internal/growthradar"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type GrowthRadarMode = config.GrowthRadarMode

const (
	GrowthRadarOff     = config.GrowthRadarOff
	GrowthRadarObserve = config.GrowthRadarObserve
	GrowthRadarCreate  = config.GrowthRadarCreate
)

type PromptStore interface {
	ListActiveGEOPrompts(context.Context, uuid.UUID) ([]db.GeoPrompt, error)
}

type promptObservationStore interface {
	MarkGEOPromptsObserved(context.Context, db.MarkGEOPromptsObservedParams) ([]db.GeoPrompt, error)
}

type FunnelStore interface {
	CreateGrowthRadarRun(context.Context, db.CreateGrowthRadarRunParams) (db.GrowthRadarRun, error)
}

type AIDiscoveryService interface {
	GeneratePromptSet(context.Context, uuid.UUID, geo.GeneratePromptSetRequest) (geo.GeneratePromptSetResult, error)
	RunCrawlerAudit(context.Context, uuid.UUID, geo.CrawlerAuditRequest) (geo.CrawlerAuditResult, error)
	ObserveAnswerProvider(context.Context, uuid.UUID, geo.ObserveAnswerProviderRequest) (geo.ObserveAnswerProviderResult, error)
	MonitorExternalSurfaces(context.Context, uuid.UUID, geo.MonitorExternalSurfacesRequest) (geo.MonitorExternalSurfacesResult, error)
	AnalyzeObservations(context.Context, uuid.UUID, geo.AnalyzeObservationsRequest) (geo.AnalyzeObservationsResult, error)
}

type AIDiscoveryOptions struct {
	ObserveRequest  geo.ObserveAnswerProviderRequest
	SearchCollector *growthradar.SearchCollector
}

type AIDiscoveryResult struct {
	PromptSetGenerated  bool               `json:"prompt_set_generated"`
	ActivePromptCount   int                `json:"active_prompt_count"`
	ObservationCount    int                `json:"observation_count"`
	ObservationCostUSD  float64            `json:"observation_cost_usd"`
	OpportunityCount    int                `json:"opportunity_count"`
	AssetBriefCount     int                `json:"asset_brief_count"`
	SearchEvidenceCount int                `json:"search_evidence_count"`
	Funnel              growthradar.Funnel `json:"funnel"`
	Steps               []AIDiscoveryStep  `json:"steps"`
	Errors              map[string]string  `json:"errors,omitempty"`
}

type AIDiscoveryStep struct {
	Name    string  `json:"name"`
	Status  string  `json:"status"`
	Count   int     `json:"count,omitempty"`
	CostUSD float64 `json:"cost_usd,omitempty"`
	Error   string  `json:"error,omitempty"`
}

func RunAIDiscovery(ctx context.Context, projectID uuid.UUID, store PromptStore, service AIDiscoveryService, opts AIDiscoveryOptions) (AIDiscoveryResult, error) {
	evidenceResult, err := RefreshAIDiscoveryEvidence(ctx, projectID, store, service, opts)
	if err != nil {
		return evidenceResult, err
	}
	var hypothesisResult AIDiscoveryResult
	if sink, ok := store.(FunnelStore); ok {
		hypothesisResult, err = MaterializeAIDiscoveryHypotheses(ctx, projectID, service, sink)
	} else {
		hypothesisResult, err = MaterializeAIDiscoveryHypotheses(ctx, projectID, service)
	}
	return mergeAIDiscoveryResults(evidenceResult, hypothesisResult), err
}

func RefreshAIDiscoveryEvidence(ctx context.Context, projectID uuid.UUID, store PromptStore, service AIDiscoveryService, opts AIDiscoveryOptions) (AIDiscoveryResult, error) {
	result := AIDiscoveryResult{}
	prompts, err := store.ListActiveGEOPrompts(ctx, projectID)
	if err != nil {
		return result, err
	}
	result.ActivePromptCount = len(prompts)
	if len(prompts) == 0 {
		promptResult, err := service.GeneratePromptSet(ctx, projectID, geo.GeneratePromptSetRequest{Locale: geo.DefaultLocale, Status: geo.DefaultPromptSetStatus})
		if err != nil {
			result.recordStep("generate_prompt_set", "", 0, 0, err)
			return result, err
		}
		result.PromptSetGenerated = len(promptResult.Prompts) > 0
		result.ActivePromptCount = len(promptResult.Prompts)
		result.recordStep("generate_prompt_set", promptResult.Run.Status, len(promptResult.Prompts), 0, nil)
		prompts, err = store.ListActiveGEOPrompts(ctx, projectID)
		if err != nil {
			return result, err
		}
	}

	audit, err := service.RunCrawlerAudit(ctx, projectID, geo.CrawlerAuditRequest{})
	result.recordStep("crawler_audit", audit.Run.Status, audit.CheckedURLs, 0, err)

	observeReq := opts.ObserveRequest
	if observeReq.Engine == "" {
		observeReq.Engine = "OpenAI"
	}
	if observeReq.MaxPrompts <= 0 {
		observeReq.MaxPrompts = 10
	}
	selection := SelectPrompts(time.Now().UTC(), promptStates(prompts), int(observeReq.MaxPrompts))
	observeReq.PromptIDs = make([]uuid.UUID, 0, len(selection.Prompts))
	for _, prompt := range selection.Prompts {
		observeReq.PromptIDs = append(observeReq.PromptIDs, prompt.ID)
	}
	observeReq.MaxPrompts = int32(len(observeReq.PromptIDs))
	if opts.SearchCollector != nil {
		searchCount := 0
		var searchErr error
		for index, prompt := range selection.Prompts {
			if index >= 3 {
				break
			}
			set, err := opts.SearchCollector.Collect(ctx, growthradar.CollectSearchRequest{ProjectID: projectID, Query: promptText(prompts, prompt.ID), Count: 10, Trigger: "daily"})
			if err != nil {
				searchErr = err
				break
			}
			if set.UsableForScoring {
				searchCount += len(set.Results)
			}
		}
		result.SearchEvidenceCount = searchCount
		result.recordStep("search_evidence", "", searchCount, 0, searchErr)
	}
	observed, observeErr := service.ObserveAnswerProvider(ctx, projectID, observeReq)
	result.ObservationCount = len(observed.Observations)
	result.ObservationCostUSD = observed.CostUSD
	result.recordStep("observe_provider", observed.Run.Status, len(observed.Observations), observed.CostUSD, observeErr)
	if marker, ok := store.(promptObservationStore); ok && len(observed.Observations) > 0 {
		ids := make([]uuid.UUID, 0, len(observed.Observations))
		seen := map[uuid.UUID]struct{}{}
		for _, observation := range observed.Observations {
			if !observation.PromptID.Valid {
				continue
			}
			promptID := observation.PromptID.Bytes
			if _, exists := seen[promptID]; exists {
				continue
			}
			seen[promptID] = struct{}{}
			ids = append(ids, promptID)
		}
		if len(ids) > 0 {
			_, _ = marker.MarkGEOPromptsObserved(ctx, db.MarkGEOPromptsObservedParams{
				ObservedAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}, ProjectID: projectID, PromptIds: ids,
			})
		}
	}

	surfaces, err := service.MonitorExternalSurfaces(ctx, projectID, geo.MonitorExternalSurfacesRequest{Limit: 25})
	result.recordStep("external_surfaces", surfaces.Run.Status, surfaces.Checked, 0, err)
	result.Funnel = evidenceFunnel(result, selection)
	persistAIDiscoveryFunnel(ctx, store, projectID, "evidence_refresh", result.Funnel)
	return result, nil
}

func promptStates(prompts []db.GeoPrompt) []PromptState {
	states := make([]PromptState, 0, len(prompts))
	for _, prompt := range prompts {
		state := PromptState{
			ID: prompt.ID, Priority: prompt.Priority, ClusterKey: prompt.ClusterKey, IntentType: prompt.IntentType,
			Audience: prompt.TargetPersona, TargetedReason: prompt.TargetedReason,
		}
		if prompt.CreatedAt.Valid {
			state.CreatedAt = prompt.CreatedAt.Time
		}
		if prompt.LastObservedAt.Valid {
			value := prompt.LastObservedAt.Time
			state.LastObservedAt = &value
		}
		if prompt.NextObservedAt.Valid {
			value := prompt.NextObservedAt.Time
			state.NextObservedAt = &value
		}
		states = append(states, state)
	}
	return states
}

func MaterializeAIDiscoveryHypotheses(ctx context.Context, projectID uuid.UUID, service AIDiscoveryService, stores ...FunnelStore) (AIDiscoveryResult, error) {
	return materializeAIDiscoveryHypotheses(ctx, projectID, service, GrowthRadarCreate, stores...)
}

func MaterializeAIDiscoveryHypothesesWithMode(ctx context.Context, projectID uuid.UUID, service AIDiscoveryService, mode GrowthRadarMode, stores ...FunnelStore) (AIDiscoveryResult, error) {
	return materializeAIDiscoveryHypotheses(ctx, projectID, service, mode, stores...)
}

func materializeAIDiscoveryHypotheses(ctx context.Context, projectID uuid.UUID, service AIDiscoveryService, mode GrowthRadarMode, stores ...FunnelStore) (AIDiscoveryResult, error) {
	result := AIDiscoveryResult{}
	if mode == GrowthRadarOff {
		result.recordStep("analyze", "skipped", 0, 0, nil)
		result.Funnel = growthradar.NormalizeFunnel(growthradar.Funnel{Status: "skipped", Reasons: map[string]int{"growth_radar_off": 1}})
		if len(stores) > 0 {
			persistAIDiscoveryFunnel(ctx, stores[0], projectID, "candidate_analysis", result.Funnel)
		}
		return result, nil
	}
	dryRun := mode != GrowthRadarCreate
	analyzed, err := service.AnalyzeObservations(ctx, projectID, geo.AnalyzeObservationsRequest{Limit: 100, DryRun: dryRun})
	if !dryRun {
		result.OpportunityCount = len(analyzed.Opportunities)
		result.AssetBriefCount = len(analyzed.AssetBriefs)
	}
	result.recordStep("analyze", analyzed.Run.Status, len(analyzed.Opportunities), 0, err)
	generated := analyzed.CandidateCount
	if generated == 0 && !dryRun {
		generated = len(analyzed.Opportunities)
	}
	reasons := map[string]int{}
	if dryRun {
		reasons["observe_only"] = generated
	}
	result.Funnel = growthradar.NormalizeFunnel(growthradar.Funnel{
		Candidates: growthradar.CandidateCounts{Generated: generated, Created: result.OpportunityCount},
		Status:     stepStatus(analyzed.Run.Status, err), Reasons: reasons,
	})
	if len(stores) > 0 {
		persistAIDiscoveryFunnel(ctx, stores[0], projectID, "candidate_analysis", result.Funnel)
	}
	return result, nil
}

func mergeAIDiscoveryResults(results ...AIDiscoveryResult) AIDiscoveryResult {
	merged := AIDiscoveryResult{}
	for _, result := range results {
		merged.PromptSetGenerated = merged.PromptSetGenerated || result.PromptSetGenerated
		if result.ActivePromptCount > 0 {
			merged.ActivePromptCount = result.ActivePromptCount
		}
		merged.ObservationCount += result.ObservationCount
		merged.ObservationCostUSD += result.ObservationCostUSD
		merged.OpportunityCount += result.OpportunityCount
		merged.AssetBriefCount += result.AssetBriefCount
		merged.SearchEvidenceCount += result.SearchEvidenceCount
		merged.Funnel = growthradar.CombineFunnels(merged.Funnel, result.Funnel)
		merged.Steps = append(merged.Steps, result.Steps...)
		for name, message := range result.Errors {
			if merged.Errors == nil {
				merged.Errors = map[string]string{}
			}
			merged.Errors[name] = message
		}
	}
	return merged
}

func evidenceFunnel(result AIDiscoveryResult, selection Selection) growthradar.Funnel {
	funnel := growthradar.Funnel{
		Sources:  growthradar.SourceCounts{Scheduled: len(result.Steps)},
		Evidence: growthradar.EvidenceCounts{Added: result.ObservationCount + result.SearchEvidenceCount},
		Prompts:  growthradar.PromptCounts{Active: result.ActivePromptCount, Selected: len(selection.Prompts)},
		CostUSD:  result.ObservationCostUSD, Status: "ok", Reasons: map[string]int{},
	}
	for _, prompt := range selection.Prompts {
		if selection.Reasons[prompt.ID] == "targeted" {
			funnel.Prompts.Targeted++
		} else {
			funnel.Prompts.Rotated++
		}
	}
	for _, step := range result.Steps {
		switch step.Status {
		case "error", "failed":
			funnel.Sources.Failed++
		case "degraded", "skipped":
			funnel.Sources.Skipped++
		default:
			funnel.Sources.Succeeded++
		}
		if step.Error != "" {
			funnel.Reasons[step.Name]++
		}
	}
	if len(result.Errors) > 0 {
		funnel.Status = "degraded"
	}
	return growthradar.NormalizeFunnel(funnel)
}

func stepStatus(status string, err error) string {
	if err != nil || status == "error" || status == "failed" {
		return "failed"
	}
	if status == "degraded" {
		return "degraded"
	}
	return "ok"
}

func persistAIDiscoveryFunnel(ctx context.Context, store any, projectID uuid.UUID, phase string, funnel growthradar.Funnel) {
	sink, ok := store.(FunnelStore)
	if !ok {
		return
	}
	encoded, _ := json.Marshal(funnel)
	_, _ = sink.CreateGrowthRadarRun(ctx, db.CreateGrowthRadarRunParams{
		ProjectID: projectID, Phase: phase, Status: funnel.Status, Funnel: encoded, CostUsd: pgutil.Numeric(funnel.CostUSD),
	})
}

func promptText(prompts []db.GeoPrompt, id uuid.UUID) string {
	for _, prompt := range prompts {
		if prompt.ID == id {
			return prompt.PromptText
		}
	}
	return ""
}

func (r *AIDiscoveryResult) recordStep(name, status string, count int, costUSD float64, err error) {
	if status == "" {
		if err != nil {
			status = "error"
		} else {
			status = "ok"
		}
	}
	step := AIDiscoveryStep{Name: name, Status: status, Count: count, CostUSD: costUSD}
	if err != nil {
		step.Error = err.Error()
		if r.Errors == nil {
			r.Errors = map[string]string{}
		}
		r.Errors[name] = err.Error()
	}
	r.Steps = append(r.Steps, step)
}
