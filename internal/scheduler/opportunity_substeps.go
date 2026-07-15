package scheduler

import (
	"strings"
	"time"

	"github.com/citeloop/citeloop/internal/opportunityfinding"
)

type opportunityFindingSubstep struct {
	Key        string  `json:"key"`
	Label      string  `json:"label"`
	Status     string  `json:"status"`
	Count      int     `json:"count,omitempty"`
	CostUSD    float64 `json:"cost_usd,omitempty"`
	DurationMs int64   `json:"duration_ms,omitempty"`
	Error      string  `json:"error,omitempty"`
}

func opportunityFindingDurationMs(started time.Time) int64 {
	ms := time.Since(started).Milliseconds()
	if ms <= 0 {
		return 1
	}
	return ms
}

func signalEvidenceSubstep(status string, durationMs int64, err error) opportunityFindingSubstep {
	if strings.TrimSpace(status) == "" {
		if err != nil {
			status = "error"
		} else {
			status = "ok"
		}
	}
	step := opportunityFindingSubstep{
		Key:        "signal_scan",
		Label:      "Search Console + page evidence",
		Status:     status,
		DurationMs: durationMs,
	}
	if err != nil {
		step.Error = err.Error()
	}
	return step
}

func skippedEvidenceSubstep(key, label, reason string) opportunityFindingSubstep {
	return opportunityFindingSubstep{Key: key, Label: label, Status: "skipped", Error: reason}
}

func aiDiscoveryEvidenceSubsteps(result opportunityfinding.AIDiscoveryResult) []opportunityFindingSubstep {
	steps := make([]opportunityFindingSubstep, 0, len(result.Steps))
	var audit *opportunityFindingSubstep
	for _, step := range result.Steps {
		substep, ok := aiDiscoverySubstep(step)
		if !ok {
			continue
		}
		if substep.Key == "site_surface_audit" {
			if audit == nil {
				copied := substep
				audit = &copied
			} else {
				mergeOpportunityFindingSubstep(audit, substep)
			}
			continue
		}
		steps = append(steps, substep)
	}
	if audit != nil {
		steps = append(steps, *audit)
	}
	return steps
}

func aiDiscoverySubstep(step opportunityfinding.AIDiscoveryStep) (opportunityFindingSubstep, bool) {
	key := step.Name
	label := ""
	switch step.Name {
	case "generate_prompt_set":
		label = "Generate discovery prompts"
	case "plan_candidates":
		label = "Plan Foundation probes"
	case "search_evidence":
		label = "Search + competitive recall"
	case "competitive_seed_urls":
		label = "Competitor page probes"
	case "observe_provider":
		label = "AI answer observations"
	case "crawler_audit", "external_surfaces":
		key = "site_surface_audit"
		label = "Site & surface audit"
	default:
		return opportunityFindingSubstep{}, false
	}
	return opportunityFindingSubstep{
		Key:        key,
		Label:      label,
		Status:     step.Status,
		Count:      step.Count,
		CostUSD:    step.CostUSD,
		DurationMs: step.DurationMs,
		Error:      step.Error,
	}, true
}

func mergeOpportunityFindingSubstep(target *opportunityFindingSubstep, next opportunityFindingSubstep) {
	target.Count += next.Count
	target.CostUSD += next.CostUSD
	target.DurationMs += next.DurationMs
	if target.Status == "" || target.Status == "ok" {
		target.Status = next.Status
	}
	if next.Status == "error" {
		target.Status = "error"
	}
	if target.Error == "" {
		target.Error = next.Error
	} else if next.Error != "" {
		target.Error += "; " + next.Error
	}
}
