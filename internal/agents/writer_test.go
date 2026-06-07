package agents

import (
	"context"
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
}
