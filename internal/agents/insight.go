package agents

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/citeloop/citeloop/internal/config"
	"github.com/citeloop/citeloop/internal/crawl"
	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Insight is the cognition agent (PRD §5.1): crawl within bounds, then extract a
// versioned Product Profile and per-article Content Inventory with evidence.
type Insight struct {
	Deps
	Log                        *slog.Logger
	inventoryExtractionTimeout time.Duration
}

const inventoryWorkerLimit = 3
const maxInventoryEvidencePages = 20
const defaultInventoryExtractionTimeout = 20 * time.Second

type CrawlSummary struct {
	LandingURL      string   `json:"landing_url"`
	DiscoveredCount int      `json:"discovered_count"`
	FetchedCount    int      `json:"fetched_count"`
	InventoryCount  int      `json:"inventory_count"`
	Truncated       bool     `json:"truncated"`
	Errors          []string `json:"errors"`
	SampleURLs      []string `json:"sample_urls"`
}

func NewInsight(d Deps, log *slog.Logger) *Insight {
	if log == nil {
		log = slog.Default()
	}
	return &Insight{Deps: d, Log: log}
}

// Run crawls landingURL and writes a new active profile + inventory rows.
// A new profile version is created each run; old versions are retained and
// deactivated (one_active_profile_per_project, §5.1 acceptance).
func (a *Insight) Run(ctx context.Context, projectID uuid.UUID, landingURL string, crawlCfg config.CrawlConfig) (*Profile, int, CrawlSummary, error) {
	cr := crawl.New(crawlCfg, a.Log)
	res, err := cr.Run(ctx, landingURL)
	if err != nil {
		return nil, 0, CrawlSummary{}, fmt.Errorf("crawl: %w", err)
	}
	summary := summarizeCrawl(landingURL, res)

	// 1) Product Profile from landing + article corpus.
	profileStarted := time.Now()
	profile, resp, err := a.extractProfile(ctx, res)
	profileDurationMS := elapsedMS(profileStarted)
	recordRun(ctx, a.Q, projectID, agentInsight, map[string]any{"step": "profile", "landing": landingURL}, map[string]any{
		"profile":       profile,
		"crawl_summary": summary,
		"duration_ms":   profileDurationMS,
	}, resp, err)
	if err != nil {
		return nil, 0, summary, fmt.Errorf("profile extraction: %w", err)
	}

	// 2) Persist profile as a new active version.
	saved, err := a.saveProfile(ctx, projectID, profile, profileSourceURLs(landingURL, res, true))
	if err != nil {
		return nil, 0, summary, err
	}

	// 3) Per-article inventory with evidence snippets (skip failures, §5.1).
	count := a.persistInventory(ctx, projectID, res)
	summary.InventoryCount = count
	recordRun(ctx, a.Q, projectID, agentInsight, map[string]any{"step": "crawl_summary", "landing": landingURL}, map[string]any{
		"crawl_summary": summary,
	}, llm.CompletionResp{}, nil)
	a.Log.Info("insight complete", "version", saved.Version, "articles", count, "truncated", res.Truncated)
	return profile, count, summary, nil
}

// RunQuickProfile builds and persists a product profile from the landing page
// only, so onboarding can make the project usable before full crawl completes.
func (a *Insight) RunQuickProfile(ctx context.Context, projectID uuid.UUID, landingURL string, crawlCfg config.CrawlConfig) (*Profile, CrawlSummary, error) {
	recordRun(ctx, a.Q, projectID, agentInsight, map[string]any{"step": "profile", "landing": landingURL, "scope": "landing", "phase": "started"}, map[string]any{
		"landing_url":   landingURL,
		"profile_stage": "started",
	}, llm.CompletionResp{}, nil)

	cr := crawl.New(crawlCfg, a.Log)
	res, err := cr.FetchLanding(ctx, landingURL)
	if err != nil {
		runErr := fmt.Errorf("crawl landing: %w", err)
		recordRun(ctx, a.Q, projectID, agentInsight, map[string]any{"step": "profile", "landing": landingURL, "scope": "landing", "phase": "fetch_landing"}, map[string]any{
			"landing_url": landingURL,
		}, llm.CompletionResp{}, runErr)
		return nil, CrawlSummary{}, runErr
	}
	summary := summarizeCrawl(landingURL, res)

	profileStarted := time.Now()
	profile, resp, err := a.extractProfile(ctx, res)
	profileDurationMS := elapsedMS(profileStarted)
	recordRun(ctx, a.Q, projectID, agentInsight, map[string]any{"step": "profile", "landing": landingURL, "scope": "landing"}, map[string]any{
		"profile":        profile,
		"profile_source": "landing",
		"profile_stage":  "provisional",
		"landing_url":    summary.LandingURL,
		"duration_ms":    profileDurationMS,
	}, resp, err)
	if err != nil {
		return nil, summary, fmt.Errorf("profile extraction: %w", err)
	}

	saved, inserted, err := a.saveProfileIfMissing(ctx, projectID, profile, profileSourceURLs(landingURL, res, false))
	if err != nil {
		return nil, summary, err
	}
	if inserted {
		a.Log.Info("quick insight profile complete", "version", saved.Version, "landing", summary.LandingURL)
	} else {
		a.Log.Info("quick insight profile skipped active profile overwrite", "version", saved.Version, "landing", summary.LandingURL)
	}
	return profile, summary, nil
}

// RunInventoryFromCrawl performs the slower full crawl and content inventory
// pass without replacing the active landing-derived product profile.
func (a *Insight) RunInventoryFromCrawl(ctx context.Context, projectID uuid.UUID, landingURL string, crawlCfg config.CrawlConfig) (int, CrawlSummary, error) {
	recordRun(ctx, a.Q, projectID, agentInsight, map[string]any{"step": "crawl", "landing": landingURL, "scope": "background", "phase": "started"}, map[string]any{
		"landing_url":  landingURL,
		"target_pages": crawlCfg.MaxPages,
	}, llm.CompletionResp{}, nil)

	cr := crawl.New(crawlCfg, a.Log)
	res, err := cr.Run(ctx, landingURL)
	if err != nil {
		runErr := fmt.Errorf("crawl: %w", err)
		recordRun(ctx, a.Q, projectID, agentInsight, map[string]any{"step": "crawl", "landing": landingURL, "scope": "background"}, map[string]any{
			"landing_url": landingURL,
		}, llm.CompletionResp{}, runErr)
		return 0, CrawlSummary{LandingURL: landingURL}, runErr
	}
	summary := summarizeCrawl(landingURL, res)
	count := a.persistInventory(ctx, projectID, res)
	summary.InventoryCount = count
	recordRun(ctx, a.Q, projectID, agentInsight, map[string]any{"step": "crawl_summary", "landing": landingURL}, map[string]any{
		"crawl_summary": summary,
	}, llm.CompletionResp{}, nil)

	profileStarted := time.Now()
	profile, resp, err := a.extractProfile(ctx, res)
	profileDurationMS := elapsedMS(profileStarted)
	recordRun(ctx, a.Q, projectID, agentInsight, map[string]any{"step": "profile", "landing": landingURL, "scope": "full_crawl"}, map[string]any{
		"profile":        profile,
		"profile_source": "full_crawl",
		"profile_stage":  "full",
		"crawl_summary":  summary,
		"duration_ms":    profileDurationMS,
	}, resp, err)
	if err != nil {
		return count, summary, fmt.Errorf("profile upgrade: %w", err)
	}
	saved, err := a.saveProfile(ctx, projectID, profile, profileSourceURLs(landingURL, res, true))
	if err != nil {
		return count, summary, fmt.Errorf("profile save: %w", err)
	}
	a.Log.Info("insight inventory crawl complete", "articles", count, "truncated", res.Truncated)
	a.Log.Info("insight full profile upgraded", "version", saved.Version, "landing", summary.LandingURL)
	return count, summary, nil
}

func summarizeCrawl(landingURL string, res *crawl.Result) CrawlSummary {
	summary := CrawlSummary{LandingURL: landingURL}
	if res == nil {
		return summary
	}
	if res.Landing != nil && res.Landing.URL != "" {
		summary.LandingURL = res.Landing.URL
	}
	summary.DiscoveredCount = len(res.Discovered)
	summary.FetchedCount = len(res.Articles)
	summary.Truncated = res.Truncated
	summary.Errors = append([]string(nil), res.Errors...)
	for _, page := range res.Articles {
		if page == nil || page.URL == "" {
			continue
		}
		summary.SampleURLs = append(summary.SampleURLs, page.URL)
		if len(summary.SampleURLs) >= 10 {
			break
		}
	}
	return summary
}

func profileSourceURLs(landingURL string, res *crawl.Result, includeArticles bool) []string {
	srcURLs := []string{landingURL}
	if res != nil && res.Landing != nil && res.Landing.URL != "" {
		srcURLs[0] = res.Landing.URL
	}
	if !includeArticles || res == nil {
		return srcURLs
	}
	for _, p := range res.Articles {
		if p == nil || p.URL == "" {
			continue
		}
		srcURLs = append(srcURLs, p.URL)
	}
	return srcURLs
}

func (a *Insight) saveProfile(ctx context.Context, projectID uuid.UUID, profile *Profile, sourceURLs []string) (db.ProductProfile, error) {
	if err := a.Q.DeactivateProfiles(ctx, projectID); err != nil {
		return db.ProductProfile{}, err
	}
	ver, err := a.Q.NextProfileVersion(ctx, projectID)
	if err != nil {
		return db.ProductProfile{}, err
	}
	return a.Q.InsertProfile(ctx, db.InsertProfileParams{
		ProjectID:  projectID,
		SourceUrls: toJSON(sourceURLs),
		Profile:    toJSON(profile),
		Version:    int32(ver),
	})
}

func (a *Insight) saveProfileIfMissing(ctx context.Context, projectID uuid.UUID, profile *Profile, sourceURLs []string) (db.ProductProfile, bool, error) {
	existing, err := a.Q.GetActiveProfile(ctx, projectID)
	if err == nil {
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return db.ProductProfile{}, false, err
	}
	saved, err := a.saveProfile(ctx, projectID, profile, sourceURLs)
	return saved, true, err
}

func (a *Insight) persistInventory(ctx context.Context, projectID uuid.UUID, res *crawl.Result) int {
	if res == nil {
		return 0
	}
	pages := make([]*crawl.Page, 0, len(res.Articles))
	for _, page := range res.Articles {
		if page != nil {
			pages = append(pages, page)
		}
	}
	if len(pages) > maxInventoryEvidencePages {
		pages = pages[:maxInventoryEvidencePages]
	}
	if len(pages) == 0 {
		return 0
	}

	workers := inventoryWorkerLimit
	if len(pages) < workers {
		workers = len(pages)
	}
	jobs := make(chan *crawl.Page)
	results := make(chan inventoryExtractionResult, len(pages))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for page := range jobs {
				started := time.Now()
				pageCtx, cancel := a.inventoryExtractionContext(ctx)
				item, resp, ierr := a.extractInventory(pageCtx, page)
				cancel()
				results <- inventoryExtractionResult{
					page:       page,
					item:       item,
					resp:       resp,
					err:        ierr,
					durationMS: elapsedMS(started),
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, page := range pages {
			select {
			case <-ctx.Done():
				return
			case jobs <- page:
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	count := 0
	for result := range results {
		page := result.page
		item := result.item
		iresp := result.resp
		ierr := result.err
		runOutput := map[string]any{"item": item, "duration_ms": result.durationMS}
		recordRun(ctx, a.Q, projectID, agentInsight, map[string]any{"step": "inventory", "url": page.URL}, runOutput, iresp, ierr)
		if ierr != nil {
			a.Log.Warn("inventory extraction failed", "url", page.URL, "err", ierr)
			continue
		}
		item.URL = page.URL
		if _, err := a.Q.UpsertInventory(ctx, db.UpsertInventoryParams{
			ProjectID:        projectID,
			Url:              page.URL,
			Title:            ptr(item.Title),
			TargetKeyword:    ptr(item.TargetKeyword),
			Topics:           toJSON(item.Topics),
			Summary:          ptr(item.Summary),
			EvidenceSnippets: toJSON(item.EvidenceSnippets),
			Source:           "existing",
		}); err != nil {
			a.Log.Warn("inventory upsert failed", "url", page.URL, "err", err)
			continue
		}
		count++
	}
	return count
}

func (a *Insight) inventoryExtractionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := a.inventoryExtractionTimeout
	if timeout <= 0 {
		timeout = defaultInventoryExtractionTimeout
	}
	return context.WithTimeout(ctx, timeout)
}

type inventoryExtractionResult struct {
	page       *crawl.Page
	item       *InventoryItem
	resp       llm.CompletionResp
	err        error
	durationMS int64
}

func elapsedMS(started time.Time) int64 {
	return time.Since(started).Milliseconds()
}

func (a *Insight) extractProfile(ctx context.Context, res *crawl.Result) (*Profile, llm.CompletionResp, error) {
	corpus := ""
	if res.Landing != nil {
		corpus = clip(res.Landing.Title+"\n"+res.Landing.Text, 6000)
	}
	for i, p := range res.Articles {
		if i >= 8 {
			break
		}
		corpus += "\n\n---\n" + clip(p.Title+"\n"+p.Text, 1500)
	}
	prompt := fmt.Sprintf(`[[INSIGHT_PROFILE]] Extract a structured product profile from this site content.
Return JSON: {positioning, value_props[], features[], icp[], tone, key_terms[], competitors[], differentiators[]}.
Only use facts present in the content.

CONTENT:
%s`, corpus)
	resp, err := a.LLM.Complete(ctx, llm.CompletionReq{
		System: "You are a product analyst extracting verifiable product facts.",
		Prompt: prompt, JSON: true, MaxTokens: 2000,
	})
	if err != nil {
		return nil, resp, err
	}
	var p Profile
	if err := extractJSON(resp.Text, &p); err != nil {
		return nil, resp, fmt.Errorf("parse profile: %w", err)
	}
	return &p, resp, nil
}

func (a *Insight) extractInventory(ctx context.Context, page *crawl.Page) (*InventoryItem, llm.CompletionResp, error) {
	prompt := fmt.Sprintf(`[[INSIGHT_INVENTORY]] Summarize this article for a content inventory.
Return JSON: {title, target_keyword, topics[], summary, evidence_snippets[]}.
evidence_snippets must be verbatim factual sentences about the product, usable as QA evidence.

ARTICLE (%s):
%s`, page.URL, clip(page.Title+"\n"+page.Text, 6000))
	resp, err := a.LLM.Complete(ctx, llm.CompletionReq{
		System: "You extract structured content inventory with verbatim evidence.",
		Prompt: prompt, JSON: true, MaxTokens: 1500,
	})
	if err != nil {
		return nil, resp, err
	}
	var item InventoryItem
	if err := extractJSON(resp.Text, &item); err != nil {
		return nil, resp, fmt.Errorf("parse inventory: %w", err)
	}
	return &item, resp, nil
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
