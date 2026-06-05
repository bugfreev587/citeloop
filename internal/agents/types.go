// Package agents implements the four pipeline agents from PRD §4/§5:
// Insight (cognition), Strategist (topics), Writer, and QA. Only Insight is
// genuinely agentic; Writer/QA are deterministic LLM steps.
package agents

import (
	"context"
	"encoding/json"

	"github.com/citeloop/citeloop/internal/db"
	"github.com/citeloop/citeloop/internal/llm"
	"github.com/citeloop/citeloop/internal/pgutil"
	"github.com/citeloop/citeloop/internal/search"
	"github.com/google/uuid"
)

// Profile is the Product Profile schema (PRD §5.1).
type Profile struct {
	Positioning     string   `json:"positioning"`
	ValueProps      []string `json:"value_props"`
	Features        []string `json:"features"`
	ICP             []string `json:"icp"`
	Tone            string   `json:"tone"`
	KeyTerms        []string `json:"key_terms"`
	Competitors     []string `json:"competitors"`
	Differentiators []string `json:"differentiators"`
}

// InventoryItem is the per-article inventory schema (PRD §5.1).
type InventoryItem struct {
	URL              string   `json:"url"`
	Title            string   `json:"title"`
	TargetKeyword    string   `json:"target_keyword"`
	Topics           []string `json:"topics"`
	Summary          string   `json:"summary"`
	EvidenceSnippets []string `json:"evidence_snippets"`
}

// TopicSpec is a generated topic before persistence (PRD §5.2).
type TopicSpec struct {
	Channel       string   `json:"channel"`
	Title         string   `json:"title"`
	TargetKeyword string   `json:"target_keyword"`
	TargetPrompt  string   `json:"target_prompt"`
	Angle         string   `json:"angle"`
	Format        string   `json:"format"`
	Priority      int      `json:"priority"`
	InternalLinks []string `json:"internal_links"`
}

// SEOMeta is the on-page SEO block (PRD §5.3).
type SEOMeta struct {
	Title          string `json:"title"`
	MetaDescription string `json:"meta_description"`
	Slug           string `json:"slug"`
	H1             string `json:"h1"`
	CanonicalURL   string `json:"canonical_url,omitempty"`
}

// WriterOutput is the Writer agent's article payload (PRD §5.3).
type WriterOutput struct {
	ContentMD string  `json:"content_md"`
	SEOMeta   SEOMeta `json:"seo_meta"`
}

// Claim is one factual product claim and whether QA could map it to evidence.
type Claim struct {
	Claim    string `json:"claim"`
	Mapped   bool   `json:"mapped"`
	Evidence string `json:"evidence"`
}

// QAOutput is the QA agent result (PRD §5.3). qa_blocking is the real gate.
type QAOutput struct {
	Claims     []Claim  `json:"claims"`
	QABlocking bool     `json:"qa_blocking"`
	GeoScore   float64  `json:"geo_score"`
	SeoScore   float64  `json:"seo_score"`
	Issues     []string `json:"issues"`
}

// Deps bundles the collaborators every agent needs.
type Deps struct {
	Q      *db.Queries
	LLM    llm.Provider
	Search search.Provider
}

// agentName is the generation_runs.agent enum.
type agentName string

const (
	agentInsight    agentName = "insight"
	agentStrategist agentName = "strategist"
	agentWriter     agentName = "writer"
	agentQA         agentName = "qa"
)

// extractJSON pulls the first balanced JSON object out of an LLM response that
// may wrap it in prose or fences, then unmarshals into v.
func extractJSON(s string, v any) error {
	start := -1
	depth := 0
	for i, r := range s {
		switch r {
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			depth--
			if depth == 0 && start >= 0 {
				return json.Unmarshal([]byte(s[start:i+1]), v)
			}
		}
	}
	return json.Unmarshal([]byte(s), v) // let it surface a real error
}

func toJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// ptr is a helper for optional string columns.
func ptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// recordRun persists a generation_runs row for observability and the cost
// breaker (§5.4). It never fails the caller's flow.
func recordRun(ctx context.Context, q *db.Queries, projectID uuid.UUID, agent agentName,
	in, out any, resp llm.CompletionResp, runErr error) {
	status := "ok"
	var errStr *string
	if runErr != nil {
		status = "error"
		s := runErr.Error()
		errStr = &s
	}
	tok := int32(resp.Tokens)
	_, _ = q.InsertGenerationRun(ctx, db.InsertGenerationRunParams{
		ProjectID: projectID,
		Agent:     string(agent),
		Input:     toJSON(in),
		Output:    toJSON(out),
		Model:     ptr(resp.Model),
		Tokens:    &tok,
		CostUsd:   pgutil.Numeric(resp.CostUSD),
		Status:    status,
		Error:     errStr,
	})
}
