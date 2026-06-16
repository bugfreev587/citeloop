package agents

import "testing"

func TestIsGenuineHumanDecision(t *testing.T) {
	cases := []struct {
		name string
		qa   *QAOutput
		want bool
	}{
		{"nil", nil, false},
		{"not blocking", &QAOutput{QABlocking: false}, false},
		{"auto-fixable", &QAOutput{QABlocking: true, CanAutoFix: true, Claims: []Claim{{Mapped: false}}}, false},
		{"infra failure (no claims, no options)", &QAOutput{QABlocking: true, CanAutoFix: false}, false},
		{"unmapped claim, not auto-fixable", &QAOutput{QABlocking: true, CanAutoFix: false, Claims: []Claim{{Mapped: false}}}, true},
		{"model decision options", &QAOutput{QABlocking: true, CanAutoFix: false, HumanDecisionOptions: []HumanDecisionOption{{Label: "choose"}}}, true},
	}
	for _, tc := range cases {
		if got := isGenuineHumanDecision(tc.qa); got != tc.want {
			t.Fatalf("%s: isGenuineHumanDecision = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestRepairOutcomeDoesNotEscalateInfraFailures(t *testing.T) {
	// QA never produced output → exhausted but auto-recoverable, never human.
	if status, human := repairOutcome(nil, 0, maxDraftRepairAttempts); status != "exhausted" || human {
		t.Fatalf("nil qa: got (%q, %v), want (exhausted, false)", status, human)
	}

	// Blocking infra failure (no claims, not auto-fixable) after the budget is
	// spent stays off the human queue so the recovery tick can retry/regenerate.
	infra := &QAOutput{QABlocking: true, CanAutoFix: false}
	if status, human := repairOutcome(infra, maxDraftRepairAttempts, maxDraftRepairAttempts); status != "exhausted" || human {
		t.Fatalf("infra failure: got (%q, %v), want (exhausted, false)", status, human)
	}

	// A real unmapped claim that cannot be auto-fixed is a genuine human decision.
	genuine := &QAOutput{QABlocking: true, CanAutoFix: false, Claims: []Claim{{Mapped: false}}}
	if status, human := repairOutcome(genuine, maxDraftRepairAttempts, maxDraftRepairAttempts); status != "exhausted" || !human {
		t.Fatalf("genuine decision: got (%q, %v), want (exhausted, true)", status, human)
	}

	// Cleared QA after a repair is approvable.
	if status, human := repairOutcome(&QAOutput{QABlocking: false}, 1, maxDraftRepairAttempts); status != "repaired" || human {
		t.Fatalf("cleared: got (%q, %v), want (repaired, false)", status, human)
	}
}
