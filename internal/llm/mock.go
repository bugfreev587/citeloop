package llm

import (
	"context"
	"strings"
)

// Mock is a deterministic Provider for tests and no-key local runs. It returns
// canned JSON keyed off markers embedded in the prompt by each agent, so the
// full pipeline runs end-to-end without an API key.
type Mock struct{}

func NewMock() *Mock { return &Mock{} }

func (m *Mock) Complete(_ context.Context, req CompletionReq) (CompletionResp, error) {
	var text string
	switch {
	case strings.Contains(req.Prompt, "[[INSIGHT_PROFILE]]"):
		text = mockProfile
	case strings.Contains(req.Prompt, "[[INSIGHT_INVENTORY]]"):
		text = mockInventory
	case strings.Contains(req.Prompt, "[[STRATEGIST]]"):
		text = mockTopics
	case strings.Contains(req.Prompt, "[[WRITER]]"):
		text = mockArticle
	case strings.Contains(req.Prompt, "[[QA]]"):
		text = mockQA
	default:
		text = `{"ok":true}`
	}
	return CompletionResp{Text: text, Provider: "mock", Model: "mock", PromptTokens: 60, CompletionTokens: 40, Tokens: 100, CostUSD: 0.001}, nil
}

const mockProfile = `{
  "positioning": "UniPost is an all-in-one social publishing tool.",
  "value_props": ["Cross-post everywhere", "Schedule in advance"],
  "features": ["multi-platform scheduling", "analytics dashboard", "AI captions"],
  "icp": ["solo creators", "small marketing teams"],
  "tone": "friendly, pragmatic",
  "key_terms": ["social scheduling", "cross-posting"],
  "competitors": ["Buffer", "Hootsuite"],
  "differentiators": ["single API for many platforms"]
}`

const mockInventory = `{
  "title": "How to schedule posts",
  "target_keyword": "social media scheduling",
  "topics": ["scheduling", "workflow"],
  "summary": "A guide to scheduling posts across platforms.",
  "evidence_snippets": ["UniPost supports multi-platform scheduling.", "Posts can be queued in advance."]
}`

const mockTopics = `{"topics":[
  {"channel":"blog","title":"The Complete Guide to Social Media Scheduling","target_keyword":"social media scheduling tools","angle":"how-to","format":"guide","priority":9,"internal_links":["/blog/how-to-schedule-posts"]},
  {"channel":"syndication","title":"Why cross-posting beats native posting","target_prompt":"best cross-posting tool 2026","angle":"opinion","format":"listicle","priority":7,"internal_links":[]},
  {"channel":"both","title":"Buffer vs UniPost: an honest comparison","target_keyword":"buffer alternative","angle":"comparison","format":"comparison","priority":8,"internal_links":[]}
]}`

const mockArticle = `{
  "content_md": "# The Complete Guide to Social Media Scheduling\n\nUniPost supports multi-platform scheduling and lets you queue posts in advance. Analytics are built in.\n\n## Why schedule\n\nScheduling saves time.",
  "seo_meta": {"title":"Social Media Scheduling Guide","meta_description":"Learn to schedule posts across platforms with UniPost.","slug":"social-media-scheduling-guide","h1":"The Complete Guide to Social Media Scheduling"}
}`

const mockQA = `{
  "claims": [
    {"claim":"UniPost supports multi-platform scheduling","mapped":true,"evidence":"features"},
    {"claim":"Analytics are built in","mapped":true,"evidence":"features"}
  ],
  "qa_blocking": false,
  "geo_score": 0.82,
  "seo_score": 0.78,
  "issues": []
}`
