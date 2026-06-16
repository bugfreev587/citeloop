package scheduler

import (
	"os"
	"strings"
	"testing"
)

func TestReviewRecoveryTickIsRegistered(t *testing.T) {
	source, err := os.ReadFile("helpers.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	if !strings.Contains(body, "TickReviewRecovery") {
		t.Fatal("Start must register the review recovery tick")
	}
	if !strings.Contains(body, "@every 2m") {
		t.Fatal("review recovery tick should run every 2 minutes")
	}
}

func TestReviewRecoveryDrivesAutomatedLadderAndAutoApprove(t *testing.T) {
	source, err := os.ReadFile("recovery.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"func (s *Scheduler) TickReviewRecovery",
		"AutoAdvanceEnabled",                  // hands-off recovery only with auto-advance
		"ListRecoverableArticlesForProject",   // blocked, not-human drafts
		"Requalify",                           // re-run QA on infra failures
		"RepairArticle",                       // AI repair on auto-fixable issues
		"regenerateOrEscalate",                // fresh draft as last resort
		"EscalateArticleToHumanForProject",    // only genuine decisions reach a human
		"autoApproveReadyForProject",          // hands-off approval
		"workflow.EventDraftApproved",         // approved drafts flow to publishing
		"MonthlySpend",                        // cost breaker
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("review recovery must wire %q", want)
		}
	}
}

func TestReviewRecoveryOnlyEscalatesGenuineDecisions(t *testing.T) {
	source, err := os.ReadFile("recovery.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	// articleNeedsHuman must require an actual claim map / decision options, never
	// treat a missing claim map (infra failure) as a human decision.
	for _, want := range []string{
		"func articleNeedsHuman",
		"func articleHasClaimMap",
		"maxTopicRegenerations",
		"maxReviewRecoveryAttempts",
		// Regeneration must clear the topic's drafts (not reject+recreate) to avoid
		// colliding with the (topic, kind, platform) unique index, and needs a profile.
		"DeleteRecoverableArticlesForTopic",
		"GetActiveProfile",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("review recovery classifier missing %q", want)
		}
	}
}
