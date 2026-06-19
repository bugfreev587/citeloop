package agents

import (
	"context"
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
		if req.Model != "claude-sonnet-4-6" {
			t.Fatalf("request %d model = %q, want Sonnet draft model", i, req.Model)
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
	if provider.reqs[0].Model != "claude-sonnet-4-6" {
		t.Fatalf("draft model = %q, want Sonnet", provider.reqs[0].Model)
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
