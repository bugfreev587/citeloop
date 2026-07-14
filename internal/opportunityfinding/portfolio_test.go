package opportunityfinding

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPromptRotationLimitsTargetedBandAndPrefersNeverObserved(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	states := []PromptState{}
	for i := 0; i < 4; i++ {
		states = append(states, PromptState{ID: stableUUID(i), Priority: 10, TargetedReason: "new-opportunity", CreatedAt: now.Add(time.Duration(i) * time.Minute)})
	}
	for i := 4; i < 12; i++ {
		states = append(states, PromptState{ID: stableUUID(i), Priority: 5, CreatedAt: now.Add(time.Duration(i) * time.Minute)})
	}
	selection := SelectPrompts(now, states, 10)
	if len(selection.Prompts) != 10 {
		t.Fatalf("selected = %d", len(selection.Prompts))
	}
	targeted := 0
	for _, prompt := range selection.Prompts {
		if selection.Reasons[prompt.ID] == "targeted" {
			targeted++
		}
	}
	if targeted != 2 {
		t.Fatalf("targeted = %d, want 2", targeted)
	}
	for _, prompt := range selection.Prompts[2:] {
		if selection.Reasons[prompt.ID] != "never_observed" {
			t.Fatalf("exploration reason = %q", selection.Reasons[prompt.ID])
		}
	}
}

func TestPromptRotationUsesOverdueThenLRUWithStableTiebreak(t *testing.T) {
	now := time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
	old := now.Add(-10 * 24 * time.Hour)
	states := []PromptState{
		{ID: stableUUID(3), LastObservedAt: &old, NextObservedAt: timePtr(now.Add(time.Hour))},
		{ID: stableUUID(2), LastObservedAt: &old, NextObservedAt: timePtr(now.Add(-time.Hour))},
		{ID: stableUUID(1), LastObservedAt: &old, NextObservedAt: timePtr(now.Add(-time.Hour))},
	}
	selection := SelectPrompts(now, states, 3)
	if selection.Prompts[0].ID != stableUUID(1) || selection.Reasons[stableUUID(1)] != "overdue" {
		t.Fatalf("order/reasons = %+v %+v", selection.Prompts, selection.Reasons)
	}
	if selection.Prompts[2].ID != stableUUID(3) || selection.Reasons[stableUUID(3)] != "lru" {
		t.Fatalf("last = %+v", selection.Prompts[2])
	}
}

func TestManualPromptRotationCanFillRunWithFreshTargetedPrompts(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	observed := now.Add(-time.Hour)
	states := []PromptState{
		{ID: stableUUID(1), Priority: 10, TargetedReason: "manual_foundation_discovery", CreatedAt: now.Add(-time.Hour), LastObservedAt: &observed},
	}
	for i := 2; i <= 7; i++ {
		states = append(states, PromptState{ID: stableUUID(i), Priority: 10, TargetedReason: "manual_foundation_discovery", CreatedAt: now.Add(time.Duration(i) * time.Minute)})
	}

	selection := selectPrompts(now, states, 6, 6)
	if len(selection.Prompts) != 6 {
		t.Fatalf("selected = %d, want 6", len(selection.Prompts))
	}
	for _, prompt := range selection.Prompts {
		if prompt.LastObservedAt != nil {
			t.Fatalf("selected previously observed targeted prompt %s while fresh prompts were available", prompt.ID)
		}
		if selection.Reasons[prompt.ID] != "targeted" {
			t.Fatalf("reason[%s] = %q, want targeted", prompt.ID, selection.Reasons[prompt.ID])
		}
	}
}

func TestPromptPortfolioCapsProjectClusterAndIntentAudience(t *testing.T) {
	candidates := make([]PromptCandidate, 0, 80)
	for i := 0; i < 80; i++ {
		candidates = append(candidates, PromptCandidate{ID: stableUUID(i), ClusterKey: fmt.Sprintf("cluster-%d", i/10), IntentType: "learn", Audience: fmt.Sprintf("audience-%d", i%4), Priority: 10 - i%10})
	}
	decision := RebuildPortfolio(candidates)
	if len(decision.Active) > 60 {
		t.Fatalf("active = %d", len(decision.Active))
	}
	clusters := map[string]int{}
	pairs := map[string]int{}
	for _, item := range decision.Active {
		clusters[item.ClusterKey]++
		pairs[item.IntentType+"\x00"+item.Audience]++
	}
	for key, count := range clusters {
		if count > 6 {
			t.Fatalf("cluster %s = %d", key, count)
		}
	}
	for key, count := range pairs {
		if count > 2 {
			t.Fatalf("pair %s = %d", key, count)
		}
	}
	if len(decision.Archived) == 0 {
		t.Fatal("overflow should be archived")
	}
}

func stableUUID(value int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", value+1))
}
func timePtr(value time.Time) *time.Time { return &value }
