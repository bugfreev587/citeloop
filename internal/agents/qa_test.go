package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/llm"
)

// A single transient unparseable QA response (the dominant QA failure mode)
// should not dead-end the check — the next attempt's valid response wins.
func TestCompleteQAWithRetryRecoversFromTransientBadResponse(t *testing.T) {
	valid := `{"claims":[],"qa_blocking":false,"geo_score":0.9,"seo_score":0.9,"issues":[]}`
	provider := &sequenceLLM{resps: []string{"sorry, I can't produce that", valid}}
	qa := NewQA(Deps{LLM: provider}, nil)

	out, _, err := qa.completeQAWithRetry(context.Background(), llm.CompletionReq{Prompt: "audit", JSON: true})
	if err != nil {
		t.Fatalf("retry should recover from a transient bad response: %v", err)
	}
	if out.QABlocking {
		t.Fatal("the valid response was non-blocking")
	}
	if len(provider.reqs) != 2 {
		t.Fatalf("expected exactly 2 attempts (1 retry), got %d", len(provider.reqs))
	}
}

// When every attempt is unparseable, the helper exhausts its budget and returns
// the last error so the caller surfaces a genuine infrastructure failure.
func TestCompleteQAWithRetryExhaustsAndReturnsError(t *testing.T) {
	provider := &sequenceLLM{resps: []string{"no", "json", "at all", "ever"}}
	qa := NewQA(Deps{LLM: provider}, nil)

	_, _, err := qa.completeQAWithRetry(context.Background(), llm.CompletionReq{Prompt: "audit", JSON: true})
	if err == nil {
		t.Fatal("expected an error after exhausting retries")
	}
	if len(provider.reqs) != qaParseRetries+1 {
		t.Fatalf("expected %d attempts, got %d", qaParseRetries+1, len(provider.reqs))
	}
}

func TestEnforceBannedClaimsBlocksLiteralMatch(t *testing.T) {
	out := &QAOutput{GeoScore: 0.9, SeoScore: 0.9}
	profile := json.RawMessage(`{"banned_claims":["SOC 2 certified","HIPAA compliant"]}`)

	enforceBannedClaims(out, profile, "UniPost is SOC 2 Certified and ships fast.")

	if !out.QABlocking {
		t.Fatal("a draft containing a banned claim must be blocked")
	}
	if !out.CanAutoFix {
		t.Fatal("banned-claim blocks should be auto-fixable so the editor loop can strip them")
	}
	if !strings.Contains(strings.ToLower(out.BlockingReason), "soc 2 certified") {
		t.Fatalf("blocking reason should name the banned claim, got %q", out.BlockingReason)
	}
	joined := strings.ToLower(strings.Join(out.Issues, " "))
	if !strings.Contains(joined, "banned claim present") {
		t.Fatalf("issues should record the banned claim, got %#v", out.Issues)
	}
}

func TestEnforceBannedClaimsAllowsCleanContent(t *testing.T) {
	out := &QAOutput{GeoScore: 0.9, SeoScore: 0.9}
	profile := json.RawMessage(`{"banned_claims":["SOC 2 certified"]}`)

	enforceBannedClaims(out, profile, "UniPost helps teams schedule posts across platforms.")

	if out.QABlocking {
		t.Fatal("clean content must not be blocked by the banned-claims gate")
	}
}

func TestEnforceBannedClaimsIgnoresEmptyAndMissing(t *testing.T) {
	out := &QAOutput{}
	// No banned_claims key at all.
	enforceBannedClaims(out, json.RawMessage(`{"positioning":"scheduler"}`), "anything")
	// Empty/whitespace entries are skipped.
	enforceBannedClaims(out, json.RawMessage(`{"banned_claims":["  ",""]}`), "anything")
	if out.QABlocking {
		t.Fatal("missing or empty banned_claims must not block")
	}
}
