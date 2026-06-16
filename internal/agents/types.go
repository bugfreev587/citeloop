// Package agents implements the four pipeline agents from PRD §4/§5:
// Insight (cognition), Strategist (topics), Writer, and QA. Only Insight is
// genuinely agentic; Writer/QA are deterministic LLM steps.
package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

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

func (t *TopicSpec) UnmarshalJSON(data []byte) error {
	var raw struct {
		Channel       string          `json:"channel"`
		Title         string          `json:"title"`
		TargetKeyword string          `json:"target_keyword"`
		TargetPrompt  string          `json:"target_prompt"`
		Angle         string          `json:"angle"`
		Format        string          `json:"format"`
		Priority      json.RawMessage `json:"priority"`
		PriorityScore json.RawMessage `json:"priority_score"`
		InternalLinks []string        `json:"internal_links"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*t = TopicSpec{
		Channel:       raw.Channel,
		Title:         raw.Title,
		TargetKeyword: raw.TargetKeyword,
		TargetPrompt:  raw.TargetPrompt,
		Angle:         raw.Angle,
		Format:        raw.Format,
		InternalLinks: raw.InternalLinks,
	}
	if priority, ok := parseTopicPriority(raw.Priority); ok {
		t.Priority = priority
	} else if priority, ok := parseTopicPriority(raw.PriorityScore); ok {
		t.Priority = priority
	}
	return nil
}

func parseTopicPriority(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		// A non-positive value (0, negative, NaN) is not a usable priority — report
		// it as unparsed so a positive alias (e.g. priority_score) or the index
		// fallback can take over instead of pinning the topic to 0.
		if p := normalizeTopicPriorityNumber(number); p > 0 {
			return p, true
		}
		return 0, false
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, false
	}
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return 0, false
	}
	if number, err := strconv.ParseFloat(text, 64); err == nil {
		if p := normalizeTopicPriorityNumber(number); p > 0 {
			return p, true
		}
		return 0, false
	}
	switch text {
	case "urgent", "critical", "highest", "p0":
		return 10, true
	case "high", "p1":
		return 8, true
	case "medium", "moderate", "normal", "p2":
		return 5, true
	case "low", "p3":
		return 3, true
	default:
		return 0, false
	}
}

func normalizeTopicPriorityNumber(value float64) int {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0
	}
	if value > 10 {
		value = value / 10
	}
	priority := int(math.Round(value))
	if priority < 1 {
		return 1
	}
	if priority > 10 {
		return 10
	}
	return priority
}

// SEOMeta is the on-page SEO block (PRD §5.3).
type SEOMeta struct {
	Title           string `json:"title"`
	MetaDescription string `json:"meta_description"`
	Slug            string `json:"slug"`
	H1              string `json:"h1"`
	TargetKeyword   string `json:"target_keyword,omitempty"`
	CanonicalURL    string `json:"canonical_url,omitempty"`
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

type QAFeedbackIssue struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Claim    string `json:"claim,omitempty"`
}

type HumanDecisionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// QAOutput is the QA agent result (PRD §5.3). qa_blocking is the real gate.
type QAOutput struct {
	Claims               []Claim               `json:"claims"`
	QABlocking           bool                  `json:"qa_blocking"`
	GeoScore             float64               `json:"geo_score"`
	SeoScore             float64               `json:"seo_score"`
	Issues               []string              `json:"issues"`
	BlockingIssues       []QAFeedbackIssue     `json:"blocking_issues"`
	FixInstructions      []string              `json:"fix_instructions"`
	HumanDecisionOptions []HumanDecisionOption `json:"human_decision_options"`
	BlockingReason       string                `json:"blocking_reason"`
	CanAutoFix           bool                  `json:"can_auto_fix"`
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
	var lastErr error
	for i, r := range s {
		if r != '{' {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(s[i:]))
		if err := dec.Decode(v); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return lastErr
	}
	return json.Unmarshal([]byte(s), v) // let it surface a real error
}

func extractValidJSON[T any](s string, validate func(T) error) (T, error) {
	var lastErr error
	for i, r := range s {
		if r != '{' {
			continue
		}
		var out T
		dec := json.NewDecoder(strings.NewReader(s[i:]))
		if err := dec.Decode(&out); err != nil {
			lastErr = err
			continue
		}
		if err := validate(out); err != nil {
			lastErr = err
			continue
		}
		return out, nil
	}
	var zero T
	if lastErr != nil {
		return zero, lastErr
	}
	return zero, json.Unmarshal([]byte(s), &zero)
}

func extractWriterOutput(s string) (WriterOutput, error) {
	return extractValidJSON(s, validateWriterOutput)
}

func extractQAOutput(s string) (QAOutput, error) {
	out, err := extractValidJSON(s, validateQAOutput)
	if err != nil {
		return QAOutput{}, err
	}
	return normalizeQAOutput(out), nil
}

func validateWriterOutput(out WriterOutput) error {
	if strings.TrimSpace(out.ContentMD) == "" {
		return fmt.Errorf("missing content_md")
	}
	if strings.Count(out.ContentMD, "```")%2 != 0 {
		return fmt.Errorf("unclosed markdown code fence")
	}
	if strings.TrimSpace(out.SEOMeta.Title) == "" {
		return fmt.Errorf("missing seo_meta.title")
	}
	if strings.TrimSpace(out.SEOMeta.MetaDescription) == "" {
		return fmt.Errorf("missing seo_meta.meta_description")
	}
	if strings.TrimSpace(out.SEOMeta.Slug) == "" {
		return fmt.Errorf("missing seo_meta.slug")
	}
	if strings.TrimSpace(out.SEOMeta.H1) == "" {
		return fmt.Errorf("missing seo_meta.h1")
	}
	return nil
}

func validateQAOutput(out QAOutput) error {
	if out.Claims == nil {
		return fmt.Errorf("missing claims")
	}
	if out.Issues == nil {
		return fmt.Errorf("missing issues")
	}
	if out.GeoScore < 0 || out.GeoScore > 1 {
		return fmt.Errorf("geo_score out of range")
	}
	if out.SeoScore < 0 || out.SeoScore > 1 {
		return fmt.Errorf("seo_score out of range")
	}
	return nil
}

func normalizeQAOutput(out QAOutput) QAOutput {
	if out.Claims == nil {
		out.Claims = []Claim{}
	}
	if out.Issues == nil {
		out.Issues = []string{}
	}
	if out.BlockingIssues == nil {
		out.BlockingIssues = []QAFeedbackIssue{}
	}
	if out.FixInstructions == nil {
		out.FixInstructions = []string{}
	}
	if out.HumanDecisionOptions == nil {
		out.HumanDecisionOptions = []HumanDecisionOption{}
	}
	if out.QABlocking && !out.CanAutoFix && len(out.HumanDecisionOptions) == 0 && out.BlockingReason == "" && len(out.BlockingIssues) == 0 {
		out.CanAutoFix = true
	}
	return out
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
