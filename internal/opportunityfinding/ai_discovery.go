package opportunityfinding

import (
	"context"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/geo"
	"github.com/google/uuid"
)

type PromptStore interface {
	ListActiveGEOPrompts(context.Context, uuid.UUID) ([]db.GeoPrompt, error)
}

type AIDiscoveryService interface {
	GeneratePromptSet(context.Context, uuid.UUID, geo.GeneratePromptSetRequest) (geo.GeneratePromptSetResult, error)
	RunCrawlerAudit(context.Context, uuid.UUID, geo.CrawlerAuditRequest) (geo.CrawlerAuditResult, error)
	ObserveAnswerProvider(context.Context, uuid.UUID, geo.ObserveAnswerProviderRequest) (geo.ObserveAnswerProviderResult, error)
	MonitorExternalSurfaces(context.Context, uuid.UUID, geo.MonitorExternalSurfacesRequest) (geo.MonitorExternalSurfacesResult, error)
	AnalyzeObservations(context.Context, uuid.UUID, geo.AnalyzeObservationsRequest) (geo.AnalyzeObservationsResult, error)
}

type AIDiscoveryOptions struct {
	ObserveRequest geo.ObserveAnswerProviderRequest
}

type AIDiscoveryResult struct {
	PromptSetGenerated bool              `json:"prompt_set_generated"`
	ActivePromptCount  int               `json:"active_prompt_count"`
	ObservationCount   int               `json:"observation_count"`
	ObservationCostUSD float64           `json:"observation_cost_usd"`
	OpportunityCount   int               `json:"opportunity_count"`
	AssetBriefCount    int               `json:"asset_brief_count"`
	Steps              []AIDiscoveryStep `json:"steps"`
	Errors             map[string]string `json:"errors,omitempty"`
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
	hypothesisResult, err := MaterializeAIDiscoveryHypotheses(ctx, projectID, service)
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
	observed, observeErr := service.ObserveAnswerProvider(ctx, projectID, observeReq)
	result.ObservationCount = len(observed.Observations)
	result.ObservationCostUSD = observed.CostUSD
	result.recordStep("observe_provider", observed.Run.Status, len(observed.Observations), observed.CostUSD, observeErr)

	surfaces, err := service.MonitorExternalSurfaces(ctx, projectID, geo.MonitorExternalSurfacesRequest{Limit: 25})
	result.recordStep("external_surfaces", surfaces.Run.Status, surfaces.Checked, 0, err)
	return result, nil
}

func MaterializeAIDiscoveryHypotheses(ctx context.Context, projectID uuid.UUID, service AIDiscoveryService) (AIDiscoveryResult, error) {
	result := AIDiscoveryResult{}
	analyzed, err := service.AnalyzeObservations(ctx, projectID, geo.AnalyzeObservationsRequest{Limit: 100})
	result.OpportunityCount = len(analyzed.Opportunities)
	result.AssetBriefCount = len(analyzed.AssetBriefs)
	result.recordStep("analyze", analyzed.Run.Status, len(analyzed.Opportunities), 0, err)
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
