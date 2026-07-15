package opportunityfinding

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/crawl"
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

type candidateFunnelStore interface {
	FunnelStore
	CreateGrowthRadarItem(context.Context, db.CreateGrowthRadarItemParams) (db.GrowthRadarItem, error)
	UpdateGrowthRadarRun(context.Context, db.UpdateGrowthRadarRunParams) (db.GrowthRadarRun, error)
	UpsertGrowthRadarWatchlistItem(context.Context, db.UpsertGrowthRadarWatchlistItemParams) (db.GrowthRadarWatchlist, error)
	ResolveGrowthRadarWatchlistItem(context.Context, db.ResolveGrowthRadarWatchlistItemParams) error
	ExpireGrowthRadarWatchlist(context.Context, db.ExpireGrowthRadarWatchlistParams) ([]db.GrowthRadarWatchlist, error)
}

type AIDiscoveryService interface {
	GeneratePromptSet(context.Context, uuid.UUID, geo.GeneratePromptSetRequest) (geo.GeneratePromptSetResult, error)
	RunCrawlerAudit(context.Context, uuid.UUID, geo.CrawlerAuditRequest) (geo.CrawlerAuditResult, error)
	EnrichCompetitiveSeedURL(context.Context, string) (crawl.SeedURLEnrichment, error)
	ObserveAnswerProvider(context.Context, uuid.UUID, geo.ObserveAnswerProviderRequest) (geo.ObserveAnswerProviderResult, error)
	MonitorExternalSurfaces(context.Context, uuid.UUID, geo.MonitorExternalSurfacesRequest) (geo.MonitorExternalSurfacesResult, error)
	AnalyzeObservations(context.Context, uuid.UUID, geo.AnalyzeObservationsRequest) (geo.AnalyzeObservationsResult, error)
}

type AIDiscoveryOptions struct {
	ObserveRequest    geo.ObserveAnswerProviderRequest
	SearchCollector   *growthradar.SearchCollector
	GrowthRadarMode   GrowthRadarMode
	FreshEvidenceKey  string
	Planner           ManualDiscoveryPlanner
	Stage             string
	WorkflowID        uuid.UUID
	RepairReasons     []string
	DiscoveryEvidence growthradar.EvidenceIndex
	SeedURLs          []string
}

type AIDiscoveryResult struct {
	PromptSetGenerated            bool                      `json:"prompt_set_generated"`
	ActivePromptCount             int                       `json:"active_prompt_count"`
	ObservationCount              int                       `json:"observation_count"`
	ObservationCostUSD            float64                   `json:"observation_cost_usd"`
	OpportunityCount              int                       `json:"opportunity_count"`
	AssetBriefCount               int                       `json:"asset_brief_count"`
	SearchEvidenceCount           int                       `json:"search_evidence_count"`
	CompetitiveSeedURLCount       int                       `json:"competitive_seed_url_count"`
	CompetitiveSeedPageCount      int                       `json:"competitive_seed_page_count"`
	CompetitiveSeedArchetypeCount int                       `json:"competitive_seed_archetype_count"`
	CompetitiveSeedReports        []crawl.SeedURLEnrichment `json:"competitive_seed_reports,omitempty"`
	PlannerProposed               int                       `json:"planner_proposed"`
	PlannerAccepted               int                       `json:"planner_accepted"`
	PlannerTokens                 int                       `json:"planner_tokens"`
	PlannerProviderCall           bool                      `json:"planner_provider_called"`
	RepairAttempted               bool                      `json:"repair_attempted"`
	Funnel                        growthradar.Funnel        `json:"funnel"`
	Steps                         []AIDiscoveryStep         `json:"steps"`
	Errors                        map[string]string         `json:"errors,omitempty"`
}

type AIDiscoveryStep struct {
	Name    string  `json:"name"`
	Status  string  `json:"status"`
	Count   int     `json:"count,omitempty"`
	CostUSD float64 `json:"cost_usd,omitempty"`
	Error   string  `json:"error,omitempty"`
}

type AIDiscoveryHypothesisOptions struct {
	CompetitiveSeedReports []crawl.SeedURLEnrichment
}

func RunAIDiscovery(ctx context.Context, projectID uuid.UUID, store PromptStore, service AIDiscoveryService, opts AIDiscoveryOptions) (AIDiscoveryResult, error) {
	if opts.GrowthRadarMode == GrowthRadarOff {
		return skippedAIDiscoveryResult("growth_radar_off"), nil
	}
	evidenceResult, err := RefreshAIDiscoveryEvidence(ctx, projectID, store, service, opts)
	if err != nil {
		return evidenceResult, err
	}
	var hypothesisResult AIDiscoveryResult
	hypothesisOpts := AIDiscoveryHypothesisOptions{CompetitiveSeedReports: evidenceResult.CompetitiveSeedReports}
	if sink, ok := store.(FunnelStore); ok {
		hypothesisResult, err = materializeAIDiscoveryHypotheses(ctx, projectID, service, GrowthRadarCreate, hypothesisOpts, sink)
	} else {
		hypothesisResult, err = materializeAIDiscoveryHypotheses(ctx, projectID, service, GrowthRadarCreate, hypothesisOpts)
	}
	return mergeAIDiscoveryResults(evidenceResult, hypothesisResult), err
}

func skippedAIDiscoveryResult(reason string) AIDiscoveryResult {
	result := AIDiscoveryResult{Funnel: growthradar.NormalizeFunnel(growthradar.Funnel{Status: "skipped", Reasons: map[string]int{reason: 1}})}
	result.recordStep("discovery", "skipped", 0, 0, nil)
	return result
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
	if opts.Planner != nil {
		planned, planErr := opts.Planner.Plan(ctx, ManualDiscoveryPlanRequest{
			ProjectID: projectID, WorkflowID: opts.WorkflowID, Stage: opts.Stage,
			ExistingPrompts: prompts, RepairReasons: opts.RepairReasons, Evidence: opts.DiscoveryEvidence,
		})
		result.PlannerProposed = planned.Proposed
		result.PlannerAccepted = planned.Accepted
		result.PlannerTokens = planned.TotalTokens
		result.PlannerProviderCall = planned.ProviderCalled
		result.RepairAttempted = planned.Repair
		result.recordStep("plan_candidates", stepStatus("ok", planErr), planned.Accepted, planned.CostUSD, planErr)
		if planErr != nil {
			return result, planErr
		}
		prompts, err = store.ListActiveGEOPrompts(ctx, projectID)
		if err != nil {
			return result, err
		}
		result.ActivePromptCount = len(prompts)
	}

	observeReq := opts.ObserveRequest
	observeReq.FreshEvidenceKey = opts.FreshEvidenceKey
	if observeReq.Engine == "" {
		observeReq.Engine = "OpenAI"
	}
	if observeReq.MaxPrompts <= 0 {
		observeReq.MaxPrompts = 10
	}
	targetedLimit := maxTargetedPromptsPerRun
	if opts.Planner != nil {
		// Manual finding is an explicit request for fresh, stage-aware discovery.
		// Let one planner batch fill the observation budget instead of allowing the
		// scheduled-rotation quota to hide most newly planned prompts.
		targetedLimit = int(observeReq.MaxPrompts)
		if targetedLimit > 6 {
			targetedLimit = 6
		}
	}
	selection := selectPrompts(time.Now().UTC(), promptStates(prompts), int(observeReq.MaxPrompts), targetedLimit)
	observeReq.PromptIDs = make([]uuid.UUID, 0, len(selection.Prompts))
	for _, prompt := range selection.Prompts {
		observeReq.PromptIDs = append(observeReq.PromptIDs, prompt.ID)
	}
	observeReq.MaxPrompts = int32(len(observeReq.PromptIDs))
	var (
		audit        geo.CrawlerAuditResult
		auditErr     error
		searchCount  int
		searchErr    error
		autoSeedURLs []string
		seedReports  []crawl.SeedURLEnrichment
		seedErr      error
		observed     geo.ObserveAnswerProviderResult
		observeErr   error
		surfaces     geo.MonitorExternalSurfacesResult
		surfacesErr  error
		workers      sync.WaitGroup
	)
	workers.Add(3)
	go func() {
		defer workers.Done()
		audit, auditErr = service.RunCrawlerAudit(ctx, projectID, geo.CrawlerAuditRequest{})
	}()
	go func() {
		defer workers.Done()
		observed, observeErr = service.ObserveAnswerProvider(ctx, projectID, observeReq)
	}()
	go func() {
		defer workers.Done()
		surfaces, surfacesErr = service.MonitorExternalSurfaces(ctx, projectID, geo.MonitorExternalSurfacesRequest{Limit: 25})
	}()
	if opts.SearchCollector != nil {
		workers.Add(1)
		go func() {
			defer workers.Done()
			seenQueries := map[string]bool{}
			collectSearch := func(query string) bool {
				query = normalizedSearchQuery(query)
				if query == "" || seenQueries[query] {
					return true
				}
				seenQueries[query] = true
				set, err := opts.SearchCollector.Collect(ctx, growthradar.CollectSearchRequest{ProjectID: projectID, Query: query, Count: 10, Trigger: "daily"})
				if err != nil {
					searchErr = err
					return false
				}
				if set.UsableForScoring {
					searchCount += len(set.Results)
					autoSeedURLs = append(autoSeedURLs, competitiveSeedURLsFromSearch(set)...)
					autoSeedURLs = mergeSeedURLs(nil, autoSeedURLs)
					if len(autoSeedURLs) > maxAutoCompetitiveSeedURLs {
						autoSeedURLs = autoSeedURLs[:maxAutoCompetitiveSeedURLs]
					}
				}
				return true
			}
			for index, prompt := range selection.Prompts {
				if index >= 3 {
					break
				}
				if !collectSearch(promptText(prompts, prompt.ID)) {
					break
				}
			}
			for _, query := range competitiveRecallQueries(prompts, opts.DiscoveryEvidence) {
				if len(autoSeedURLs) >= maxAutoCompetitiveSeedURLs {
					break
				}
				if !collectSearch(query) {
					break
				}
			}
		}()
	}
	workers.Wait()
	seedURLs := mergeSeedURLs(opts.SeedURLs, autoSeedURLs)
	if len(seedURLs) > 0 {
		seedReports, seedErr = enrichCompetitiveSeedURLs(ctx, service, seedURLs)
	}

	result.recordStep("crawler_audit", audit.Run.Status, audit.CheckedURLs, 0, auditErr)
	if opts.SearchCollector != nil {
		result.SearchEvidenceCount = searchCount
		result.recordStep("search_evidence", "", searchCount, 0, searchErr)
	}
	if len(seedURLs) > 0 {
		result.CompetitiveSeedReports = seedReports
		result.CompetitiveSeedURLCount, result.CompetitiveSeedPageCount, result.CompetitiveSeedArchetypeCount = competitiveSeedCounts(seedReports)
		result.recordStep("competitive_seed_urls", stepStatus("ok", seedErr), result.CompetitiveSeedPageCount, 0, seedErr)
	}
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

	result.recordStep("external_surfaces", surfaces.Run.Status, surfaces.Checked, 0, surfacesErr)
	result.Funnel = evidenceFunnel(result, selection)
	if persistErr := persistAIDiscoveryFunnel(ctx, store, projectID, "evidence_refresh", result.Funnel); persistErr != nil {
		return result, persistErr
	}
	return result, nil
}

func enrichCompetitiveSeedURLs(ctx context.Context, service AIDiscoveryService, seedURLs []string) ([]crawl.SeedURLEnrichment, error) {
	reports := make([]crawl.SeedURLEnrichment, 0, len(seedURLs))
	var firstErr error
	seen := map[string]bool{}
	for _, seedURL := range seedURLs {
		seedURL = strings.TrimSpace(seedURL)
		if seedURL == "" || seen[seedURL] {
			continue
		}
		seen[seedURL] = true
		report, err := service.EnrichCompetitiveSeedURL(ctx, seedURL)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			reports = append(reports, crawl.SeedURLEnrichment{URL: seedURL, FilterReasons: []string{"seed_enrichment_error"}})
			continue
		}
		reports = append(reports, report)
	}
	return reports, firstErr
}

func competitiveSeedCounts(reports []crawl.SeedURLEnrichment) (urls, pages, archetypes int) {
	urls = len(reports)
	for _, report := range reports {
		if report.StatusCode >= 200 && report.StatusCode < 400 && report.RobotsAllowed && report.Indexable {
			pages++
		}
		archetypes += len(report.Archetypes)
	}
	return urls, pages, archetypes
}

const (
	maxAutoCompetitiveSeedURLs  = 5
	maxCompetitiveRecallQueries = 6
)

func competitiveRecallQueries(prompts []db.GeoPrompt, evidence growthradar.EvidenceIndex) []string {
	queries := make([]string, 0, maxCompetitiveRecallQueries)
	add := func(query string) {
		if len(queries) >= maxCompetitiveRecallQueries {
			return
		}
		query = normalizedSearchQuery(query)
		if query == "" {
			return
		}
		for _, existing := range queries {
			if existing == query {
				return
			}
		}
		queries = append(queries, query)
	}
	terms := competitiveRecallTerms(prompts, evidence)
	if len(terms) > 0 {
		primary := terms[0]
		add(competitiveToolsQuery(primary))
		add(primary + " alternatives")
		add(primary + " compare")
	}
	for _, term := range terms {
		add("best " + term)
		add(competitiveToolsQuery(term))
		add(term + " scheduler")
		add(term + " automation tools")
	}
	for _, prompt := range prompts {
		add(prompt.PromptText)
	}
	return queries
}

func competitiveRecallTerms(prompts []db.GeoPrompt, evidence growthradar.EvidenceIndex) []string {
	terms := make([]string, 0, 4)
	add := func(term string) {
		if len(terms) >= 4 {
			return
		}
		term = normalizedSearchQuery(term)
		if term == "" || growthradar.ContainsInternalSensitiveTerm(term) {
			return
		}
		for _, existing := range terms {
			if existing == term {
				return
			}
		}
		terms = append(terms, term)
	}
	for _, term := range evidence.PublicTerms {
		add(term)
	}
	for _, prompt := range prompts {
		if strings.TrimSpace(prompt.TargetTopic) != "" {
			add(prompt.TargetTopic)
		}
	}
	return terms
}

func competitiveToolsQuery(term string) string {
	term = normalizedSearchQuery(term)
	if term == "" {
		return ""
	}
	if strings.Contains(term, "tool") {
		if strings.HasPrefix(term, "free ") {
			return term
		}
		return "free " + term
	}
	return "free " + term + " tools"
}

func competitiveSeedURLsFromSearch(set growthradar.EvidenceSet) []string {
	urls := make([]string, 0, len(set.Results))
	for _, result := range set.Results {
		if !isCompetitiveSeedCandidateURL(result.URL) {
			continue
		}
		normalized, err := crawl.Normalize(result.URL)
		if err != nil {
			normalized = strings.TrimSpace(result.URL)
		}
		urls = append(urls, normalized)
	}
	return mergeSeedURLs(nil, urls)
}

func isCompetitiveSeedCandidateURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return false
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return false
	}
	path := strings.ToLower(parsed.EscapedPath())
	for _, marker := range []string{"/tools", "/alternatives", "/compare", "/comparison", "/scheduler"} {
		if strings.Contains(path, marker) {
			return true
		}
	}
	return false
}

func mergeSeedURLs(manual []string, auto []string) []string {
	merged := make([]string, 0, len(manual)+len(auto))
	seen := map[string]bool{}
	for _, seedURL := range append(append([]string{}, manual...), auto...) {
		seedURL = strings.TrimSpace(seedURL)
		if seedURL == "" {
			continue
		}
		if normalized, err := crawl.Normalize(seedURL); err == nil {
			seedURL = normalized
		}
		if seen[seedURL] {
			continue
		}
		seen[seedURL] = true
		merged = append(merged, seedURL)
	}
	return merged
}

func normalizedSearchQuery(query string) string {
	return strings.ToLower(strings.Join(strings.Fields(query), " "))
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
	return materializeAIDiscoveryHypotheses(ctx, projectID, service, GrowthRadarCreate, AIDiscoveryHypothesisOptions{}, stores...)
}

func MaterializeAIDiscoveryHypothesesWithMode(ctx context.Context, projectID uuid.UUID, service AIDiscoveryService, mode GrowthRadarMode, stores ...FunnelStore) (AIDiscoveryResult, error) {
	return materializeAIDiscoveryHypotheses(ctx, projectID, service, mode, AIDiscoveryHypothesisOptions{}, stores...)
}

func MaterializeAIDiscoveryHypothesesWithOptions(ctx context.Context, projectID uuid.UUID, service AIDiscoveryService, mode GrowthRadarMode, opts AIDiscoveryHypothesisOptions, stores ...FunnelStore) (AIDiscoveryResult, error) {
	return materializeAIDiscoveryHypotheses(ctx, projectID, service, mode, opts, stores...)
}

func materializeAIDiscoveryHypotheses(ctx context.Context, projectID uuid.UUID, service AIDiscoveryService, mode GrowthRadarMode, opts AIDiscoveryHypothesisOptions, stores ...FunnelStore) (AIDiscoveryResult, error) {
	result := AIDiscoveryResult{}
	if mode == GrowthRadarOff {
		result.recordStep("analyze", "skipped", 0, 0, nil)
		result.Funnel = growthradar.NormalizeFunnel(growthradar.Funnel{Status: "skipped", Reasons: map[string]int{"growth_radar_off": 1}})
		if len(stores) > 0 {
			if persistErr := persistAIDiscoveryFunnel(ctx, stores[0], projectID, "candidate_analysis", result.Funnel); persistErr != nil {
				return result, persistErr
			}
		}
		return result, nil
	}
	dryRun := mode != GrowthRadarCreate
	request := geo.AnalyzeObservationsRequest{Limit: 100, DryRun: dryRun, CompetitiveSeedReports: opts.CompetitiveSeedReports}
	var auditSink candidateFunnelStore
	var lifecycleSink candidateFunnelStore
	var auditRun db.GrowthRadarRun
	if len(stores) > 0 {
		lifecycleSink, _ = stores[0].(candidateFunnelStore)
		if lifecycleSink != nil {
			if _, expireErr := lifecycleSink.ExpireGrowthRadarWatchlist(ctx, db.ExpireGrowthRadarWatchlistParams{ProjectID: projectID, NowAt: pgtype.Timestamptz{Time: time.Now().UTC(), Valid: true}}); expireErr != nil {
				return result, fmt.Errorf("expire growth radar watchlist: %w", expireErr)
			}
		}
	}
	if mode == GrowthRadarCreate {
		if len(stores) == 0 {
			return result, fmt.Errorf("growth radar candidate audit store is required before opportunity creation")
		}
		auditSink = lifecycleSink
		if auditSink == nil {
			return result, fmt.Errorf("growth radar candidate audit store does not support replayable items")
		}
		pending := growthradar.NormalizeFunnel(growthradar.Funnel{Status: "ok", Reasons: map[string]int{"candidate_scoring_in_progress": 1}})
		encoded, _ := json.Marshal(pending)
		var createErr error
		auditRun, createErr = auditSink.CreateGrowthRadarRun(ctx, db.CreateGrowthRadarRunParams{ProjectID: projectID, Phase: "candidate_analysis", Status: "ok", Funnel: encoded, CostUsd: pgutil.Numeric(0)})
		if createErr != nil {
			return result, fmt.Errorf("create growth radar audit run: %w", createErr)
		}
		request.BeforeCreate = func(candidate geo.GrowthRadarCandidate) error {
			return persistGrowthRadarCandidate(ctx, auditSink, auditRun.ID, projectID, candidate)
		}
	}
	analyzed, err := service.AnalyzeObservations(ctx, projectID, request)
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
	candidateCounts := growthradar.CandidateCounts{Generated: generated, Created: result.OpportunityCount}
	demandCounts := growthradar.DemandCounts{}
	stage, profile, zeroReuse := "", "", 0
	for _, candidate := range analyzed.Candidates {
		if stage == "" {
			stage, profile = candidate.Score.Stage, candidate.Score.StageProfileVersion
		}
		seo := candidate.Snapshot.CurrentImpressions > 0 || candidate.Snapshot.PreviousImpressions > 0
		geoDemand := candidate.Snapshot.IndependentGEOProviders > 0
		switch {
		case seo && geoDemand:
			demandCounts.Combined++
		case seo:
			demandCounts.SEOOnly++
		case geoDemand:
			demandCounts.GEOOnly++
		default:
			demandCounts.None++
		}
		if candidate.Snapshot.CompatibleExternalTargets == 0 && candidate.Snapshot.AdditionalOutputTypes == 0 {
			zeroReuse++
		}
		for _, code := range candidate.Score.ReasonCodes {
			reasons[code]++
		}
		switch candidate.Disposition {
		case "watchlist", "hold":
			candidateCounts.Watchlist++
		case "merged", "near_duplicate":
			candidateCounts.Duplicates++
		case "arbitration":
			candidateCounts.Conflicts++
		case "filtered", "dismissed":
			candidateCounts.Filtered++
		}
	}
	result.Funnel = growthradar.NormalizeFunnel(growthradar.Funnel{
		Stage: stage, Profile: profile, Candidates: candidateCounts, Demand: demandCounts, ZeroReuse: zeroReuse,
		Status: stepStatus(analyzed.Run.Status, err), Reasons: reasons,
	})
	if auditSink != nil {
		encoded, _ := json.Marshal(result.Funnel)
		status := result.Funnel.Status
		if err != nil {
			status = "failed"
		}
		if _, persistErr := auditSink.UpdateGrowthRadarRun(ctx, db.UpdateGrowthRadarRunParams{Status: status, Funnel: encoded, CostUsd: pgutil.Numeric(result.Funnel.CostUSD), ID: auditRun.ID, ProjectID: projectID}); persistErr != nil {
			return result, fmt.Errorf("finalize growth radar audit run: %w", persistErr)
		}
	} else if len(stores) > 0 {
		if persistErr := persistAIDiscoveryFunnelWithCandidates(ctx, stores[0], projectID, "candidate_analysis", result.Funnel, analyzed.Candidates); persistErr != nil {
			return result, persistErr
		}
	}
	return result, err
}

func persistAIDiscoveryFunnelWithCandidates(ctx context.Context, store FunnelStore, projectID uuid.UUID, phase string, funnel growthradar.Funnel, candidates []geo.GrowthRadarCandidate) error {
	encoded, _ := json.Marshal(funnel)
	run, err := store.CreateGrowthRadarRun(ctx, db.CreateGrowthRadarRunParams{ProjectID: projectID, Phase: phase, Status: funnel.Status, Funnel: encoded, CostUsd: pgutil.Numeric(funnel.CostUSD)})
	if err != nil {
		return fmt.Errorf("create growth radar run: %w", err)
	}
	sink, ok := store.(candidateFunnelStore)
	if !ok {
		return nil
	}
	for _, candidate := range candidates {
		if err := persistGrowthRadarCandidate(ctx, sink, run.ID, projectID, candidate); err != nil {
			return err
		}
	}
	return nil
}

func persistGrowthRadarCandidate(ctx context.Context, sink candidateFunnelStore, runID, projectID uuid.UUID, candidate geo.GrowthRadarCandidate) error {
	score, err := json.Marshal(candidate.Score)
	if err != nil {
		return fmt.Errorf("encode growth radar score: %w", err)
	}
	snapshot, err := json.Marshal(candidate.Snapshot)
	if err != nil {
		return fmt.Errorf("encode growth radar scoring snapshot: %w", err)
	}
	evidence := candidate.Evidence
	if len(evidence) == 0 {
		evidence = json.RawMessage(`{}`)
	}
	if _, err := sink.CreateGrowthRadarItem(ctx, db.CreateGrowthRadarItemParams{
		RunID: runID, ProjectID: projectID, CandidateIdentity: candidate.Identity,
		Disposition: candidate.Disposition, Reason: candidate.Reason, Score: score, ScoringSnapshot: snapshot, Evidence: evidence,
	}); err != nil {
		return fmt.Errorf("create growth radar audit item: %w", err)
	}
	lastRunID := pgtype.UUID{Bytes: runID, Valid: runID != uuid.Nil}
	if candidate.Disposition == "watchlist" || candidate.Disposition == "hold" {
		fingerprintSnapshot := candidate.Snapshot
		// Evidence aging is derived from the clock and must not keep an otherwise
		// unchanged watchlist item alive forever.
		fingerprintSnapshot.NewestEvidenceAgeDays = nil
		fingerprintInput, _ := json.Marshal(struct {
			Snapshot growthradar.Snapshot `json:"scoring_snapshot"`
			Evidence json.RawMessage      `json:"evidence"`
		}{Snapshot: fingerprintSnapshot, Evidence: evidence})
		sum := sha256.Sum256(fingerprintInput)
		fingerprint := hex.EncodeToString(sum[:])
		if _, err := sink.UpsertGrowthRadarWatchlistItem(ctx, db.UpsertGrowthRadarWatchlistItemParams{
			ProjectID: projectID, CandidateIdentity: candidate.Identity, Reason: candidate.Reason,
			Score: score, ScoringSnapshot: snapshot, Evidence: evidence, EvidenceFingerprint: fingerprint,
			ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(90 * 24 * time.Hour), Valid: true}, LastRunID: lastRunID,
		}); err != nil {
			return fmt.Errorf("upsert durable growth radar watchlist item: %w", err)
		}
		return nil
	}
	if err := sink.ResolveGrowthRadarWatchlistItem(ctx, db.ResolveGrowthRadarWatchlistItemParams{
		Reason: candidate.Disposition, LastRunID: lastRunID, ProjectID: projectID, CandidateIdentity: candidate.Identity,
	}); err != nil {
		return fmt.Errorf("resolve durable growth radar watchlist item: %w", err)
	}
	return nil
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
		merged.CompetitiveSeedURLCount += result.CompetitiveSeedURLCount
		merged.CompetitiveSeedPageCount += result.CompetitiveSeedPageCount
		merged.CompetitiveSeedArchetypeCount += result.CompetitiveSeedArchetypeCount
		merged.CompetitiveSeedReports = append(merged.CompetitiveSeedReports, result.CompetitiveSeedReports...)
		merged.PlannerProposed += result.PlannerProposed
		merged.PlannerAccepted += result.PlannerAccepted
		merged.PlannerTokens += result.PlannerTokens
		merged.PlannerProviderCall = merged.PlannerProviderCall || result.PlannerProviderCall
		merged.RepairAttempted = merged.RepairAttempted || result.RepairAttempted
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

func MergeAIDiscoveryResults(results ...AIDiscoveryResult) AIDiscoveryResult {
	return mergeAIDiscoveryResults(results...)
}

func evidenceFunnel(result AIDiscoveryResult, selection Selection) growthradar.Funnel {
	funnel := growthradar.Funnel{
		Sources:  growthradar.SourceCounts{Scheduled: len(result.Steps)},
		Evidence: growthradar.EvidenceCounts{Added: result.ObservationCount + result.SearchEvidenceCount + result.CompetitiveSeedPageCount},
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
	if result.CompetitiveSeedURLCount > 0 {
		funnel.Reasons["competitive_seed_url"] = result.CompetitiveSeedURLCount
	}
	for _, report := range result.CompetitiveSeedReports {
		for _, archetype := range report.Archetypes {
			if archetype.Archetype != "" {
				funnel.Reasons["competitive_archetype_"+archetype.Archetype]++
			}
		}
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

func persistAIDiscoveryFunnel(ctx context.Context, store any, projectID uuid.UUID, phase string, funnel growthradar.Funnel) error {
	sink, ok := store.(FunnelStore)
	if !ok {
		return nil
	}
	encoded, _ := json.Marshal(funnel)
	_, err := sink.CreateGrowthRadarRun(ctx, db.CreateGrowthRadarRunParams{
		ProjectID: projectID, Phase: phase, Status: funnel.Status, Funnel: encoded, CostUsd: pgutil.Numeric(funnel.CostUSD),
	})
	if err != nil {
		return fmt.Errorf("create growth radar funnel run: %w", err)
	}
	return nil
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
