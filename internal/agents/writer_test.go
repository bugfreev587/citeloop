package agents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
)

type sequenceLLM struct {
	resps []string
	reqs  []llm.CompletionReq
}

func (s *sequenceLLM) Complete(_ context.Context, req llm.CompletionReq) (llm.CompletionResp, error) {
	s.reqs = append(s.reqs, req)
	text := ""
	if len(s.resps) > 0 {
		text = s.resps[0]
		s.resps = s.resps[1:]
	}
	return llm.CompletionResp{Text: text, Model: "sequence"}, nil
}

func TestWriterDraftFallsBackToMarkdownWhenStructuredJSONIsInvalid(t *testing.T) {
	provider := &sequenceLLM{resps: []string{
		`{content_md: bad}`,
		"# OAuth Flows Explained\n\nHosted OAuth keeps social account tokens out of your app.",
	}}
	writer := NewWriter(Deps{LLM: provider}, nil)
	topic := db.Topic{
		Title:         "OAuth Flows Explained: Securely Connecting User Social Accounts",
		TargetKeyword: ptr("OAuth social media account connection"),
		TargetPrompt:  ptr("Guide developers through hosted OAuth flows."),
		Angle:         ptr("Security-first implementation guide"),
		Format:        ptr("Step-by-step tutorial"),
		InternalLinks: []byte(`[]`),
	}

	out, _, err := writer.draft(context.Background(), topic, []byte(`{"features":["hosted OAuth"]}`), "", true)
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if out.ContentMD == "" {
		t.Fatal("fallback draft must include content")
	}
	if out.SEOMeta.Slug != "oauth-flows-explained-securely-connecting-user-social-accounts" {
		t.Fatalf("slug = %q", out.SEOMeta.Slug)
	}
	if len(provider.reqs) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(provider.reqs))
	}
	if provider.reqs[1].JSON {
		t.Fatal("fallback request must not ask for JSON")
	}
	if provider.reqs[1].MaxTokens != 8192 {
		t.Fatalf("fallback max tokens = %d, want 8192", provider.reqs[1].MaxTokens)
	}
	for i, req := range provider.reqs {
		if req.Purpose != llm.PurposeWriter {
			t.Fatalf("request %d purpose = %q, want writer", i, req.Purpose)
		}
	}
}

func TestWriterPromptTreatsBannedClaimsAsNegativeConstraints(t *testing.T) {
	provider := &sequenceLLM{resps: []string{
		`{"content_md":"# Draft\n\nSupported article.","seo_meta":{"title":"Draft","meta_description":"Desc","slug":"draft","h1":"Draft","target_keyword":"draft"}}`,
	}}
	writer := NewWriter(Deps{LLM: provider}, nil)
	topic := db.Topic{
		Title:         "Evidence-backed Content",
		TargetKeyword: ptr("evidence-backed content"),
	}
	profile := []byte(`{"features":["evidence library"],"banned_claims":["Guaranteed #1 rankings"],"content_rules":["Cite sources"]}`)

	if _, _, err := writer.draft(context.Background(), topic, profile, "", true); err != nil {
		t.Fatalf("draft: %v", err)
	}
	if len(provider.reqs) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.reqs))
	}
	prompt := provider.reqs[0].Prompt
	if !strings.Contains(prompt, "banned_claims") {
		t.Fatal("prompt must include banned_claims from the profile")
	}
	if !strings.Contains(prompt, "negative constraints") {
		t.Fatal("prompt must explain banned claims as negative constraints")
	}
	if !strings.Contains(prompt, "Do not repeat or imply banned_claims") {
		t.Fatal("prompt must forbid repeating banned claims")
	}
	if provider.reqs[0].Purpose != llm.PurposeWriter {
		t.Fatalf("draft purpose = %q, want writer", provider.reqs[0].Purpose)
	}
}

func TestWriterPromptUsesGEOAssetBriefContract(t *testing.T) {
	provider := &sequenceLLM{resps: []string{
		`{"content_md":"# Buffer vs UniPost\n\nA supported comparison draft.","seo_meta":{"title":"Buffer vs UniPost","meta_description":"Desc","slug":"buffer-vs-unipost","h1":"Buffer vs UniPost","target_keyword":"best social scheduling tools"}}`,
	}}
	writer := NewWriter(Deps{LLM: provider}, nil)
	topic := db.Topic{
		Title:        "best social scheduling tools",
		TargetPrompt: ptr("best social scheduling tools"),
		Angle:        ptr("comparison_page"),
		Format:       ptr("geo_asset_brief"),
		InternalLinks: json.RawMessage(`{
			"links":["/blog/social-scheduling"],
			"source_evidence":["first-party comparison proof","competitor citation evidence: Buffer"],
			"recommended_outline":["Decision criteria","Supported differentiators","Limitations"]
		}`),
	}

	out, _, err := writer.draft(context.Background(), topic, []byte(`{"features":["workspace scheduling"],"evidence":["calendar proof"]}`), "", true)
	if err != nil {
		t.Fatalf("draft: %v", err)
	}
	if len(provider.reqs) != 1 {
		t.Fatalf("provider calls = %d, want 1", len(provider.reqs))
	}
	prompt := provider.reqs[0].Prompt
	for _, want := range []string{
		"ASSET TYPE: comparison_page",
		"SOURCE EVIDENCE",
		"first-party comparison proof",
		"competitor citation evidence: Buffer",
		"decision criteria",
		"who each option is for",
		"supported differentiators",
		"limitations",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("writer prompt missing %q:\n%s", want, prompt)
		}
	}
	if out.SEOMeta.AssetType != "comparison_page" {
		t.Fatalf("seo_meta asset_type = %q, want comparison_page", out.SEOMeta.AssetType)
	}
	if len(out.SEOMeta.SourceEvidence) != 2 || out.SEOMeta.SourceEvidence[1] != "competitor citation evidence: Buffer" {
		t.Fatalf("seo_meta source_evidence = %#v, want brief evidence", out.SEOMeta.SourceEvidence)
	}
}

func TestSEOMetaFromTopicIncludesTargetKeyword(t *testing.T) {
	topic := db.Topic{
		Title:         "OAuth Flows Explained",
		TargetKeyword: ptr("oauth social account connection"),
	}

	meta := seoMetaFromTopic(topic, "", true)

	if meta.TargetKeyword != "oauth social account connection" {
		t.Fatalf("target keyword = %q", meta.TargetKeyword)
	}
}

func TestSEOMetaFromTopicInfersMissingTargetKeyword(t *testing.T) {
	topic := db.Topic{Title: "OAuth Flows Explained"}

	meta := seoMetaFromTopic(topic, "", true)

	if meta.TargetKeyword != "OAuth Flows Explained" {
		t.Fatalf("target keyword = %q", meta.TargetKeyword)
	}
}

func TestDraftNeedsRepairForQAAndSEOGaps(t *testing.T) {
	if !draftNeedsRepair(&WriterOutput{SEOMeta: SEOMeta{TargetKeyword: "oauth"}}, &QAOutput{QABlocking: true, CanAutoFix: true}, nil) {
		t.Fatal("qa blocking draft must be repaired before review")
	}
	if draftNeedsRepair(&WriterOutput{SEOMeta: SEOMeta{TargetKeyword: "oauth"}}, &QAOutput{QABlocking: true, CanAutoFix: false}, nil) {
		t.Fatal("qa blocking draft that requires human decision must not be repaired automatically")
	}
	if !draftNeedsRepair(&WriterOutput{SEOMeta: SEOMeta{TargetKeyword: "oauth"}}, nil, errors.New("parse qa: missing claims")) {
		t.Fatal("qa parser failure must be repaired before review")
	}
	if !draftNeedsRepair(&WriterOutput{}, &QAOutput{}, nil) {
		t.Fatal("missing target keyword must be repaired before review")
	}
	if draftNeedsRepair(&WriterOutput{SEOMeta: SEOMeta{TargetKeyword: "oauth"}}, &QAOutput{}, nil) {
		t.Fatal("clean draft should not be repaired")
	}
}

func TestShouldAttemptArticleRepairHonorsPersistentLoopState(t *testing.T) {
	if !shouldAttemptArticleRepair(db.Article{RepairAttempts: 1}, maxDraftRepairAttempts) {
		t.Fatal("article under repair cap should be eligible")
	}
	if shouldAttemptArticleRepair(db.Article{RepairAttempts: int32(maxDraftRepairAttempts)}, maxDraftRepairAttempts) {
		t.Fatal("article at repair cap must not be repaired again")
	}
	if shouldAttemptArticleRepair(db.Article{RequiresHumanDecision: true}, maxDraftRepairAttempts) {
		t.Fatal("article escalated to human decision must not be repaired again")
	}
}

func TestShouldAttemptArticleRepairReopensEditorRepairableHumanDecision(t *testing.T) {
	malformed := db.Article{
		QaBlocking:            true,
		RequiresHumanDecision: true,
		QaFeedback: toJSON(map[string]any{
			"qa_blocking":     true,
			"claims":          []map[string]any{{"claim": "Hosted OAuth reduces connection work", "mapped": true}},
			"can_auto_fix":    false,
			"blocking_reason": "Article is truncated mid-heading",
			"fix_instructions": []string{
				"Complete the dangling section using supported evidence, or remove the empty heading.",
			},
			"human_decision_options": []map[string]string{
				{"label": "Article is truncated", "description": "Article ends abruptly at an empty heading."},
			},
		}),
	}
	if !shouldAttemptArticleRepair(malformed, maxDraftRepairAttempts) {
		t.Fatal("editor-repairable human-decision rows should be reopened for AI repair")
	}

	positioning := db.Article{
		QaBlocking:            true,
		RequiresHumanDecision: true,
		QaFeedback: toJSON(map[string]any{
			"qa_blocking":  true,
			"claims":       []map[string]any{{"claim": "UniPost helps teams ship faster", "mapped": true}},
			"can_auto_fix": false,
			"human_decision_options": []map[string]string{
				{"label": "Choose positioning", "description": "Pick the approved positioning before publication."},
			},
		}),
	}
	if shouldAttemptArticleRepair(positioning, maxDraftRepairAttempts) {
		t.Fatal("genuine human positioning decisions must stay out of AI repair")
	}
}

func TestHumanDecisionOptionsExposeFixInstructionsForApplyFix(t *testing.T) {
	options := humanDecisionOptions(&QAOutput{
		QABlocking:      true,
		CanAutoFix:      true,
		FixInstructions: []string{"Remove the unsupported MCP claim and keep only profile-backed capabilities."},
	})

	if len(options) != 1 {
		t.Fatalf("fix instructions should become one-click editor options, got %#v", options)
	}
	if options[0].Label != "Apply QA fix" {
		t.Fatalf("label = %q, want Apply QA fix", options[0].Label)
	}
	if !strings.Contains(options[0].Description, "unsupported MCP claim") {
		t.Fatalf("description should carry the fix instruction, got %#v", options[0])
	}
}

func TestApprovedQAAfterAppliedFixClearsComments(t *testing.T) {
	previous := &QAOutput{
		Claims:               []Claim{{Claim: "UniPost includes native MCP support", Mapped: false}},
		QABlocking:           true,
		GeoScore:             0.61,
		SeoScore:             0.72,
		Issues:               []string{"unsupported MCP claim"},
		BlockingIssues:       []QAFeedbackIssue{{Code: "unmapped_product_claim", Severity: "blocking", Message: "MCP claim lacks evidence"}},
		FixInstructions:      []string{"Remove the MCP claim."},
		HumanDecisionOptions: []HumanDecisionOption{{Label: "Edit draft", Description: "Rewrite manually."}},
		BlockingReason:       "MCP claim lacks evidence",
		CanAutoFix:           true,
	}

	cleared := approvedQAAfterAppliedFix(previous, "Remove the unsupported MCP claim.")

	if cleared.QABlocking {
		t.Fatal("applying the requested AI editor fix must clear the QA gate")
	}
	if cleared.CanAutoFix {
		t.Fatal("cleared QA should not remain marked as auto-fixable")
	}
	if len(cleared.Claims) != 0 || len(cleared.Issues) != 0 || len(cleared.BlockingIssues) != 0 || len(cleared.FixInstructions) != 0 || len(cleared.HumanDecisionOptions) != 0 {
		t.Fatalf("cleared QA must not carry old or new comments: %#v", cleared)
	}
	if cleared.BlockingReason != "" {
		t.Fatalf("blocking reason = %q, want empty", cleared.BlockingReason)
	}
	if cleared.GeoScore != previous.GeoScore || cleared.SeoScore != previous.SeoScore {
		t.Fatalf("scores = %.2f/%.2f, want previous %.2f/%.2f", cleared.GeoScore, cleared.SeoScore, previous.GeoScore, previous.SeoScore)
	}
}
