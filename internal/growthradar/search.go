package growthradar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/search"
	"github.com/google/uuid"
)

const searchRequestCostUSD = 0.005
const searchCacheTTL = 7 * 24 * time.Hour

var ErrSearchBudgetExhausted = errors.New("growth radar search budget exhausted")

type SearchBudget struct {
	DailyRequests         int     `json:"daily_requests"`
	WeeklyRebuildRequests int     `json:"weekly_rebuild_requests"`
	RollingRequests       int     `json:"rolling_requests"`
	RollingCostUSD        float64 `json:"rolling_cost_usd"`
	InstallationCostUSD   float64 `json:"installation_cost_usd"`
}

type EvidenceSet struct {
	ProjectID           uuid.UUID       `json:"project_id"`
	NormalizedQuery     string          `json:"normalized_query"`
	RequestHash         string          `json:"request_hash"`
	ResultSetHash       string          `json:"result_set_hash"`
	Provider            string          `json:"provider"`
	ProviderOrderIsRank bool            `json:"provider_order_is_rank"`
	Results             []search.Result `json:"results"`
	Synthetic           bool            `json:"synthetic"`
	UsableForScoring    bool            `json:"usable_for_scoring"`
	Status              string          `json:"status"`
	CostUSD             float64         `json:"cost_usd"`
	FetchedAt           time.Time       `json:"fetched_at"`
	ExpiresAt           time.Time       `json:"expires_at"`
	Reused              bool            `json:"reused"`
	Trigger             string          `json:"trigger"`
}

type CollectSearchRequest struct {
	ProjectID uuid.UUID
	Query     string
	Count     int
	Trigger   string
}

type SearchEvidenceStore interface {
	FindSearchEvidence(context.Context, uuid.UUID, string, time.Time) (*EvidenceSet, error)
	SearchUsage(context.Context, uuid.UUID, time.Time) (SearchBudget, error)
	SaveSearchEvidence(context.Context, EvidenceSet) error
}

type SearchCollector struct {
	Provider search.Provider
	Store    SearchEvidenceStore
	Now      func() time.Time
}

func (c SearchCollector) Collect(ctx context.Context, req CollectSearchRequest) (EvidenceSet, error) {
	now := time.Now().UTC()
	if c.Now != nil {
		now = c.Now().UTC()
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(req.Query), " "))
	if normalized == "" {
		return EvidenceSet{}, fmt.Errorf("search query is required")
	}
	requestHash := hashText(normalized)
	if c.Store != nil {
		cached, err := c.Store.FindSearchEvidence(ctx, req.ProjectID, requestHash, now)
		if err != nil {
			return EvidenceSet{}, err
		}
		if cached != nil && cached.ExpiresAt.After(now) {
			copy := *cached
			copy.Reused = true
			return copy, nil
		}
		usage, err := c.Store.SearchUsage(ctx, req.ProjectID, now)
		if err != nil {
			return EvidenceSet{}, err
		}
		if budgetBlocks(usage, req.Trigger) {
			return EvidenceSet{}, ErrSearchBudgetExhausted
		}
	}
	providerName := "unknown_search"
	synthetic := false
	if c.Provider == nil {
		return EvidenceSet{ProjectID: req.ProjectID, NormalizedQuery: normalized, RequestHash: requestHash, Status: "degraded"}, fmt.Errorf("search provider unavailable")
	}
	if provider, ok := c.Provider.(search.EvidenceProvider); ok {
		providerName, synthetic = provider.ProviderName(), provider.Synthetic()
	}
	set := EvidenceSet{
		ProjectID: req.ProjectID, NormalizedQuery: normalized, RequestHash: requestHash,
		Provider: providerName, ProviderOrderIsRank: false, Synthetic: synthetic,
		UsableForScoring: !synthetic, Status: "collected", CostUSD: searchRequestCostUSD,
		FetchedAt: now, ExpiresAt: now.Add(searchCacheTTL), Trigger: req.Trigger,
	}
	results, err := c.Provider.Search(ctx, search.Query{Text: normalized, Count: req.Count})
	if err != nil {
		set.Status = "degraded"
		set.UsableForScoring = false
		set.CostUSD = 0
		return set, err
	}
	for index := range results {
		results[index].ProviderOrder = index + 1
		if results[index].FetchedAt.IsZero() {
			results[index].FetchedAt = now
		}
		if results[index].Source == "" {
			results[index].Source = providerName
		}
	}
	set.Results = results
	encoded, _ := json.Marshal(results)
	set.ResultSetHash = hashText(string(encoded))
	if c.Store != nil {
		if err := c.Store.SaveSearchEvidence(ctx, set); err != nil {
			return EvidenceSet{}, err
		}
	}
	return set, nil
}

func budgetBlocks(budget SearchBudget, trigger string) bool {
	return budget.DailyRequests >= 30 ||
		(trigger == "weekly_rebuild" && budget.WeeklyRebuildRequests >= 60) ||
		budget.RollingRequests >= 600 || budget.RollingCostUSD+searchRequestCostUSD > 3 ||
		budget.InstallationCostUSD+searchRequestCostUSD > 25
}

func hashText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
