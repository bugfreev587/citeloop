package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/llm"
)

// QA models routinely return issues/fix_instructions as structured objects
// rather than plain strings. The typed decode must tolerate that instead of
// failing the whole verdict and misreporting it as "missing claims".
func TestExtractQAOutputToleratesStructuredIssueAndFixShapes(t *testing.T) {
	raw := "```json\n" + `{
	  "claims": [{"claim":"UniPost supports 9 platforms","mapped":true,"evidence":"profile"}],
	  "qa_blocking": true,
	  "geo_score": 0.6,
	  "seo_score": 0.7,
	  "issues": [{"code":"SEO_H1","severity":"medium","message":"Add an H1 before the first H2."}],
	  "fix_instructions": [{"priority":"high","action":"add_h1","instruction":"Insert an H1 heading."}],
	  "human_decision_options": [{"label":"Add evidence","description":"Provide a source."}],
	  "blocking_reason": "promotional bias",
	  "can_auto_fix": false
	}` + "\n```"

	out, err := extractQAOutput(raw)
	if err != nil {
		t.Fatalf("structured issue/fix shapes must parse, got: %v", err)
	}
	if len(out.Claims) != 1 {
		t.Fatalf("claims = %d, want 1", len(out.Claims))
	}
	if len(out.Issues) != 1 || !strings.Contains(out.Issues[0], "H1") {
		t.Fatalf("issues should coerce object->string, got %#v", out.Issues)
	}
	if len(out.FixInstructions) != 1 || !strings.Contains(out.FixInstructions[0], "Insert an H1") {
		t.Fatalf("fix_instructions should coerce object->string, got %#v", out.FixInstructions)
	}
	if len(out.HumanDecisionOptions) != 0 {
		t.Fatalf("Context/evidence decision options should be stripped, got %#v", out.HumanDecisionOptions)
	}
}

func TestExtractQAOutputTreatsUnsupportedClaimsAsAutoEditable(t *testing.T) {
	raw := `{
		"claims":[{"claim":"UniPost includes native MCP server support","mapped":false,"evidence":""}],
		"qa_blocking":true,
		"geo_score":0.7,
		"seo_score":0.8,
		"issues":["unsupported MCP claim"],
		"blocking_issues":[{"code":"unmapped_product_claim","severity":"blocking","message":"MCP claim lacks evidence","claim":"UniPost includes native MCP server support"}],
		"fix_instructions":[],
		"human_decision_options":[{"label":"Add evidence","description":"Update Context before approving."}],
		"blocking_reason":"MCP claim lacks evidence",
		"can_auto_fix":false
	}`

	out, err := extractQAOutput(raw)
	if err != nil {
		t.Fatalf("extractQAOutput: %v", err)
	}
	if !out.QABlocking {
		t.Fatal("unsupported product claims must still block publication")
	}
	if !out.CanAutoFix {
		t.Fatal("unsupported product claims should route to the AI editor, not Context edits")
	}
	if len(out.HumanDecisionOptions) != 0 {
		t.Fatalf("Context/evidence decision options should be stripped, got %#v", out.HumanDecisionOptions)
	}
	if len(out.FixInstructions) == 0 {
		t.Fatal("unsupported claims should carry an editor fix instruction")
	}
}

func TestExtractQAOutputAddsActionableFixInstructionForBlockingComments(t *testing.T) {
	raw := `{
		"claims":[{"claim":"UniPost is a scheduling platform","mapped":true,"evidence":"profile"}],
		"qa_blocking":true,
		"geo_score":0.7,
		"seo_score":0.8,
		"issues":["The introduction is too broad to pass QA."],
		"blocking_issues":[{"code":"required_seo_metadata","severity":"blocking","message":"Add a specific H1 and meta description."}],
		"fix_instructions":[],
		"human_decision_options":[],
		"blocking_reason":"Missing required SEO metadata",
		"can_auto_fix":true
	}`

	out, err := extractQAOutput(raw)
	if err != nil {
		t.Fatalf("extractQAOutput: %v", err)
	}
	if len(out.FixInstructions) == 0 {
		t.Fatal("blocking QA comments must be converted into actionable editor fix instructions")
	}
	if !strings.Contains(strings.ToLower(out.FixInstructions[0]), "missing required seo metadata") {
		t.Fatalf("fix instruction should explain how to pass QA, got %#v", out.FixInstructions)
	}
}

// Plain string arrays must still parse unchanged (regression guard).
func TestExtractQAOutputStillAcceptsPlainStringArrays(t *testing.T) {
	raw := `{"claims":[],"qa_blocking":false,"geo_score":0.9,"seo_score":0.9,"issues":["a","b"],"fix_instructions":["do x"]}`
	out, err := extractQAOutput(raw)
	if err != nil {
		t.Fatalf("plain string arrays must parse, got: %v", err)
	}
	if len(out.Issues) != 2 || out.Issues[1] != "b" {
		t.Fatalf("issues = %#v", out.Issues)
	}
}

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

func TestQACompactCheckUsesOpusModel(t *testing.T) {
	valid := `{"claims":[],"qa_blocking":false,"geo_score":0.9,"seo_score":0.9,"issues":[]}`
	provider := &sequenceLLM{resps: []string{valid}}
	qa := NewQA(Deps{LLM: provider}, nil)

	if _, _, err := qa.compactCheck(context.Background(), []byte(`{"features":["proof"]}`), "evidence", "# Draft"); err != nil {
		t.Fatalf("compactCheck: %v", err)
	}
	if len(provider.reqs) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.reqs))
	}
	if provider.reqs[0].Model != "claude-opus-4-8" {
		t.Fatalf("compact QA model = %q, want Opus", provider.reqs[0].Model)
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
