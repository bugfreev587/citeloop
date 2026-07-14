package growthradar

type SourceCounts struct {
	Scheduled int `json:"scheduled"`
	Succeeded int `json:"succeeded"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
}
type EvidenceCounts struct {
	Added   int `json:"added"`
	Changed int `json:"changed"`
	Reused  int `json:"reused"`
	Expired int `json:"expired"`
}
type TermCounts struct {
	Accepted int `json:"accepted"`
	Rejected int `json:"rejected"`
	Held     int `json:"held"`
}
type PromptCounts struct {
	Active   int `json:"active"`
	Selected int `json:"selected"`
	Rotated  int `json:"rotated"`
	Targeted int `json:"targeted"`
}
type CandidateCounts struct {
	Generated  int `json:"generated"`
	Duplicates int `json:"duplicates"`
	Conflicts  int `json:"conflicts"`
	Watchlist  int `json:"watchlist"`
	Filtered   int `json:"filtered"`
	Created    int `json:"created"`
}
type DemandCounts struct {
	SEOOnly  int `json:"seo_only"`
	GEOOnly  int `json:"geo_only"`
	Combined int `json:"combined"`
	None     int `json:"none"`
}

type Funnel struct {
	Stage      string          `json:"stage,omitempty"`
	Profile    string          `json:"stage_profile_version,omitempty"`
	Sources    SourceCounts    `json:"sources"`
	Evidence   EvidenceCounts  `json:"evidence"`
	Terms      TermCounts      `json:"terms"`
	Prompts    PromptCounts    `json:"prompts"`
	Candidates CandidateCounts `json:"candidates"`
	Demand     DemandCounts    `json:"demand"`
	ZeroReuse  int             `json:"zero_resolved_reuse_inputs"`
	CostUSD    float64         `json:"cost_usd"`
	Status     string          `json:"status"`
	Reasons    map[string]int  `json:"reasons"`
}

func NormalizeFunnel(funnel Funnel) Funnel {
	if funnel.Reasons == nil {
		funnel.Reasons = map[string]int{}
	}
	usable := funnel.Evidence.Added + funnel.Evidence.Changed + funnel.Evidence.Reused
	if funnel.Sources.Scheduled > 0 && usable == 0 {
		funnel.Status = "degraded"
		funnel.Reasons["no_usable_evidence"]++
	}
	if funnel.Status == "" {
		funnel.Status = "ok"
	}
	return funnel
}

func CombineFunnels(funnels ...Funnel) Funnel {
	result := Funnel{Status: "ok", Reasons: map[string]int{}}
	for _, value := range funnels {
		if result.Stage == "" {
			result.Stage, result.Profile = value.Stage, value.Profile
		}
		result.Sources.Scheduled += value.Sources.Scheduled
		result.Sources.Succeeded += value.Sources.Succeeded
		result.Sources.Skipped += value.Sources.Skipped
		result.Sources.Failed += value.Sources.Failed
		result.Evidence.Added += value.Evidence.Added
		result.Evidence.Changed += value.Evidence.Changed
		result.Evidence.Reused += value.Evidence.Reused
		result.Evidence.Expired += value.Evidence.Expired
		result.Terms.Accepted += value.Terms.Accepted
		result.Terms.Rejected += value.Terms.Rejected
		result.Terms.Held += value.Terms.Held
		result.Prompts.Active = max(result.Prompts.Active, value.Prompts.Active)
		result.Prompts.Selected += value.Prompts.Selected
		result.Prompts.Rotated += value.Prompts.Rotated
		result.Prompts.Targeted += value.Prompts.Targeted
		result.Candidates.Generated += value.Candidates.Generated
		result.Candidates.Duplicates += value.Candidates.Duplicates
		result.Candidates.Conflicts += value.Candidates.Conflicts
		result.Candidates.Watchlist += value.Candidates.Watchlist
		result.Candidates.Filtered += value.Candidates.Filtered
		result.Candidates.Created += value.Candidates.Created
		result.Demand.SEOOnly += value.Demand.SEOOnly
		result.Demand.GEOOnly += value.Demand.GEOOnly
		result.Demand.Combined += value.Demand.Combined
		result.Demand.None += value.Demand.None
		result.ZeroReuse += value.ZeroReuse
		result.CostUSD += value.CostUSD
		for reason, count := range value.Reasons {
			result.Reasons[reason] += count
		}
		if value.Status == "failed" {
			result.Status = "failed"
		} else if value.Status == "degraded" && result.Status != "failed" {
			result.Status = "degraded"
		}
	}
	return NormalizeFunnel(result)
}
