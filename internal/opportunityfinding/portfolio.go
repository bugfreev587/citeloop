package opportunityfinding

import (
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxPortfolioPrompts      = 60
	maxClusterPrompts        = 6
	maxIntentAudiencePrompts = 2
	maxTargetedPromptsPerRun = 2
)

type PromptState struct {
	ID             uuid.UUID
	Priority       int32
	ClusterKey     string
	IntentType     string
	Audience       string
	TargetedReason string
	CreatedAt      time.Time
	LastObservedAt *time.Time
	NextObservedAt *time.Time
}

type Selection struct {
	Prompts []PromptState
	Reasons map[uuid.UUID]string
}

type PromptCandidate struct {
	ID         uuid.UUID
	ClusterKey string
	IntentType string
	Audience   string
	Priority   int
	PromptText string
}

type PortfolioDecision struct {
	Active   []PromptCandidate
	Archived []PromptCandidate
	Reasons  map[uuid.UUID]string
}

func SelectPrompts(now time.Time, prompts []PromptState, limit int) Selection {
	result := Selection{Prompts: []PromptState{}, Reasons: map[uuid.UUID]string{}}
	if limit <= 0 {
		return result
	}
	remaining := append([]PromptState(nil), prompts...)
	targeted := make([]PromptState, 0)
	exploration := make([]PromptState, 0)
	for _, prompt := range remaining {
		if strings.TrimSpace(prompt.TargetedReason) != "" {
			targeted = append(targeted, prompt)
		} else {
			exploration = append(exploration, prompt)
		}
	}
	sort.Slice(targeted, func(i, j int) bool {
		if targeted[i].Priority != targeted[j].Priority {
			return targeted[i].Priority > targeted[j].Priority
		}
		if !targeted[i].CreatedAt.Equal(targeted[j].CreatedAt) {
			return targeted[i].CreatedAt.Before(targeted[j].CreatedAt)
		}
		return targeted[i].ID.String() < targeted[j].ID.String()
	})
	for _, prompt := range targeted {
		if len(result.Prompts) >= limit || len(result.Prompts) >= maxTargetedPromptsPerRun {
			break
		}
		result.Prompts = append(result.Prompts, prompt)
		result.Reasons[prompt.ID] = "targeted"
	}
	sort.Slice(exploration, func(i, j int) bool {
		leftBand, rightBand := rotationBand(exploration[i], now), rotationBand(exploration[j], now)
		if leftBand != rightBand {
			return leftBand < rightBand
		}
		leftTime, rightTime := rotationTime(exploration[i]), rotationTime(exploration[j])
		if !leftTime.Equal(rightTime) {
			return leftTime.Before(rightTime)
		}
		return exploration[i].ID.String() < exploration[j].ID.String()
	})
	for _, prompt := range exploration {
		if len(result.Prompts) >= limit {
			break
		}
		result.Prompts = append(result.Prompts, prompt)
		switch rotationBand(prompt, now) {
		case 0:
			result.Reasons[prompt.ID] = "never_observed"
		case 1:
			result.Reasons[prompt.ID] = "overdue"
		default:
			result.Reasons[prompt.ID] = "lru"
		}
	}
	return result
}

func rotationBand(prompt PromptState, now time.Time) int {
	if prompt.LastObservedAt == nil {
		return 0
	}
	if prompt.NextObservedAt != nil && !prompt.NextObservedAt.After(now) {
		return 1
	}
	return 2
}

func rotationTime(prompt PromptState) time.Time {
	if prompt.LastObservedAt != nil {
		return *prompt.LastObservedAt
	}
	return prompt.CreatedAt
}

func RebuildPortfolio(candidates []PromptCandidate) PortfolioDecision {
	ordered := append([]PromptCandidate(nil), candidates...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Priority != ordered[j].Priority {
			return ordered[i].Priority > ordered[j].Priority
		}
		return ordered[i].ID.String() < ordered[j].ID.String()
	})
	decision := PortfolioDecision{Active: []PromptCandidate{}, Archived: []PromptCandidate{}, Reasons: map[uuid.UUID]string{}}
	clusters := map[string]int{}
	pairs := map[string]int{}
	for _, candidate := range ordered {
		cluster := strings.ToLower(strings.TrimSpace(candidate.ClusterKey))
		pair := strings.ToLower(strings.TrimSpace(candidate.IntentType)) + "\x00" + strings.ToLower(strings.TrimSpace(candidate.Audience))
		reason := ""
		switch {
		case len(decision.Active) >= maxPortfolioPrompts:
			reason = "project_cap"
		case clusters[cluster] >= maxClusterPrompts:
			reason = "cluster_cap"
		case pairs[pair] >= maxIntentAudiencePrompts:
			reason = "intent_audience_cap"
		}
		if reason != "" {
			decision.Archived = append(decision.Archived, candidate)
			decision.Reasons[candidate.ID] = reason
			continue
		}
		decision.Active = append(decision.Active, candidate)
		clusters[cluster]++
		pairs[pair]++
	}
	return decision
}
