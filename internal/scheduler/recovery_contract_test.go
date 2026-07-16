package scheduler

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
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
		"ReviewAutoAdvanceEnabled",          // gates auto-approval only; recovery runs for all projects
		"ListRecoverableArticlesForProject", // blocked, not-human drafts
		"Requalify",                         // re-run QA on infra failures
		"RepairArticle",                     // AI repair on auto-fixable issues
		"regenerateOrEscalate",              // fresh draft as last resort
		"EscalateArticleToHumanForProject",  // only genuine decisions reach a human
		"autoApproveReadyForProject",        // hands-off approval
		"approveRecoveredArticle",           // QA recovery approval respects Review Auto
		"workflow.EventDraftApproved",       // approved drafts flow to publishing
		"MonthlySpend",                      // cost breaker
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

func TestArticleNeedsHumanKeepsUnsupportedClaimsAutomated(t *testing.T) {
	art := db.Article{
		QaBlocking: true,
		QaFeedback: mustTestJSON(t, map[string]any{
			"claims": []map[string]any{
				{"claim": "UniPost includes a native MCP server", "mapped": false},
			},
			"can_auto_fix": false,
			"human_decision_options": []map[string]string{
				{"label": "Add evidence", "description": "Update Context before approving."},
			},
		}),
	}

	if articleNeedsHuman(art) {
		t.Fatal("unsupported claims should stay in automated repair/regeneration instead of asking for Context edits")
	}
}

func TestArticleNeedsHumanKeepsMalformedContentAutomated(t *testing.T) {
	art := db.Article{
		QaBlocking: true,
		QaFeedback: mustTestJSON(t, map[string]any{
			"claims": []map[string]any{
				{"claim": "Hosted OAuth flows reduce platform integration work", "mapped": true, "evidence": "profile"},
			},
			"can_auto_fix": false,
			"fix_instructions": []string{
				"Complete the dangling section using supported evidence, or remove the empty heading so the draft ends cleanly.",
			},
			"blocking_reason": "Article is truncated mid-heading",
			"blocking_issues": []map[string]string{
				{"code": "malformed_content", "severity": "blocking", "message": "Article ends with a dangling empty heading."},
			},
			"human_decision_options": []map[string]string{
				{"label": "Article is truncated", "description": "Article body ends abruptly at a heading with no content under it."},
			},
		}),
	}

	if !articleCanAutoFix(art) {
		t.Fatal("malformed content with editor instructions should enter AI editor repair")
	}
	if articleNeedsHuman(art) {
		t.Fatal("malformed content with editor instructions should not be escalated to the user")
	}
}

func TestEditorRepairableHumanDecisionBypassesRecoveryBudget(t *testing.T) {
	art := db.Article{
		QaBlocking:            true,
		RequiresHumanDecision: true,
		RecoveryAttempts:      int32(maxReviewRecoveryAttempts),
		QaFeedback: mustTestJSON(t, map[string]any{
			"claims": []map[string]any{
				{"claim": "Hosted OAuth flows reduce platform integration work", "mapped": true, "evidence": "profile"},
			},
			"can_auto_fix":    false,
			"blocking_reason": "Article is truncated mid-heading",
			"fix_instructions": []string{
				"Complete the dangling section using supported evidence, or remove the empty heading.",
			},
			"human_decision_options": []map[string]string{
				{"label": "Article is truncated", "description": "Article body ends abruptly at a heading with no content under it."},
			},
		}),
	}

	if !articleShouldBypassRecoveryBudget(art) {
		t.Fatal("editor-repairable human-decision rows misrouted by the old cap should get one AI repair path")
	}

	positioning := db.Article{
		QaBlocking:            true,
		RequiresHumanDecision: true,
		RecoveryAttempts:      int32(maxReviewRecoveryAttempts),
		QaFeedback: mustTestJSON(t, map[string]any{
			"claims":       []map[string]any{{"claim": "UniPost helps small teams ship faster", "mapped": true}},
			"can_auto_fix": false,
			"human_decision_options": []map[string]string{
				{"label": "Choose positioning", "description": "Pick the approved positioning before publication."},
			},
		}),
	}

	if articleShouldBypassRecoveryBudget(positioning) {
		t.Fatal("true positioning decisions must not bypass the recovery budget")
	}
}

func TestArticleNeedsHumanStillAllowsTruePositioningDecisions(t *testing.T) {
	art := db.Article{
		QaBlocking: true,
		QaFeedback: mustTestJSON(t, map[string]any{
			"claims":       []map[string]any{{"claim": "UniPost helps small teams ship faster", "mapped": true}},
			"can_auto_fix": false,
			"human_decision_options": []map[string]string{
				{"label": "Choose positioning", "description": "Pick the approved positioning before publication."},
			},
		}),
	}

	if !articleNeedsHuman(art) {
		t.Fatal("non-editorial positioning choices can still reach a human")
	}
}

func TestReviewRecoverySkipsAlreadyEscalatedTrueHumanDecisions(t *testing.T) {
	source, err := os.ReadFile("recovery.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"if art.RequiresHumanDecision",
		"return nil",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("recoverArticle must no-op already escalated genuine human decisions; missing %q", want)
		}
	}
}

func TestEscalationOptionsFiltersContextEvidenceChoices(t *testing.T) {
	options := escalationOptions(mustTestJSON(t, map[string]any{
		"human_decision_options": []map[string]string{
			{"label": "Add evidence", "description": "Update Context before approving."},
			{"label": "Choose positioning", "description": "Pick the approved positioning."},
		},
	}))

	if len(options) != 1 || options[0].Label != "Choose positioning" {
		t.Fatalf("escalation options = %#v", options)
	}
}

func TestReviewRecoveryAutoApprovesClearedResultsOnlyWhenReviewAutoIsEnabled(t *testing.T) {
	source, err := os.ReadFile("recovery.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(source)
	for _, want := range []string{
		"func shouldAutoApproveRecoveryResult",
		"return art.Status == \"pending_review\" && !art.QaBlocking && !art.RequiresHumanDecision",
		"approveRecoveredArticle(ctx, q, projectID, updated, cfg)",
		"cfg.ReviewAutoAdvanceEnabled",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("review recovery must auto-approve cleared QA fixes only under Review Auto; missing %q", want)
		}
	}
}

func TestShouldAutoApproveRecoveryResult(t *testing.T) {
	if !shouldAutoApproveRecoveryResult(db.Article{Status: "pending_review", QaBlocking: false}) {
		t.Fatal("QA-cleared pending-review recovery results should be approved automatically")
	}
	if shouldAutoApproveRecoveryResult(db.Article{Status: "pending_review", QaBlocking: true}) {
		t.Fatal("still-blocking recovery results must not be approved")
	}
	if shouldAutoApproveRecoveryResult(db.Article{Status: "pending_review", RequiresHumanDecision: true}) {
		t.Fatal("genuine human decisions must not be auto-approved")
	}
	if shouldAutoApproveRecoveryResult(db.Article{Status: "approved", QaBlocking: false}) {
		t.Fatal("already-approved or non-review articles do not need recovery approval")
	}
}

func mustTestJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
