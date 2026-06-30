// Package agents implements the four pipeline agents from PRD §4/§5:
// Insight (cognition), Strategist (topics), Writer, and QA. Only Insight is
// genuinely agentic; Writer/QA are deterministic LLM steps.
package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
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
	case "urgent", "critical", "highest", "p0", "high", "p1":
		return 1, true
	case "p2":
		return 2, true
	case "p3":
		return 3, true
	case "medium", "moderate", "normal":
		return 5, true
	case "low":
		return 8, true
	case "lowest":
		return 10, true
	default:
		return 0, false
	}
}

func normalizeTopicPriorityNumber(value float64) int {
	if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
		return 0
	}
	if value > 10 {
		priority := int(math.Ceil((100 - value) / 10))
		if priority < 1 {
			return 1
		}
		if priority > 10 {
			return 10
		}
		return priority
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
	Title           string   `json:"title"`
	MetaDescription string   `json:"meta_description"`
	Slug            string   `json:"slug"`
	H1              string   `json:"h1"`
	TargetKeyword   string   `json:"target_keyword,omitempty"`
	CanonicalURL    string   `json:"canonical_url,omitempty"`
	AssetType       string   `json:"asset_type,omitempty"`
	SourceEvidence  []string `json:"source_evidence,omitempty"`
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

// stringList tolerates a JSON array whose elements are strings OR objects. QA
// models frequently return issues/fix_instructions as structured objects
// (e.g. {code,severity,message} or {priority,action,instruction}) instead of
// plain strings. Without this, the typed decode of the whole QA result fails and
// extractValidJSON falls through to an inner claim object — so a QA verdict that
// actually ran is misreported as "missing claims" and the draft dead-ends.
type stringList []string

func (sl *stringList) UnmarshalJSON(b []byte) error {
	trimmed := strings.TrimSpace(string(b))
	if trimmed == "" || trimmed == "null" {
		*sl = nil
		return nil
	}
	// Fast path: a plain []string.
	var ss []string
	if err := json.Unmarshal(b, &ss); err == nil {
		*sl = ss
		return nil
	}
	// Tolerant path: an array whose elements may be strings or objects.
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		// Not an array at all — coerce a lone string/object into one entry.
		if s := coerceJSONToString(b); s != "" {
			*sl = stringList{s}
			return nil
		}
		return err
	}
	out := make([]string, 0, len(raw))
	for _, el := range raw {
		if s := coerceJSONToString(el); s != "" {
			out = append(out, s)
		}
	}
	*sl = out
	return nil
}

// coerceJSONToString renders a JSON string or object as one readable line, so a
// richly-structured QA issue/fix entry collapses back into the []string the rest
// of the pipeline (writer feedback, UI) expects.
func coerceJSONToString(b []byte) string {
	var s string
	if json.Unmarshal(b, &s) == nil {
		return strings.TrimSpace(s)
	}
	var m map[string]any
	if json.Unmarshal(b, &m) == nil {
		// Prefer the most human-readable field if present.
		for _, k := range []string{"message", "instruction", "action", "text", "detail", "description", "summary", "reason", "issue", "claim", "label"} {
			if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		// Otherwise join whatever string values exist, in a stable order.
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		parts := make([]string, 0, len(keys))
		for _, k := range keys {
			if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
				parts = append(parts, strings.TrimSpace(v))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " — ")
		}
	}
	return strings.TrimSpace(string(b))
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
	Issues               stringList            `json:"issues"`
	BlockingIssues       []QAFeedbackIssue     `json:"blocking_issues"`
	FixInstructions      stringList            `json:"fix_instructions"`
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
	// A missing claims key means QA did not actually evaluate (a truncated or
	// empty response) — reject it so it retries/regenerates rather than passing
	// unchecked content. The real fix for truncation is the larger token budget
	// in qa.go, not loosening this gate. "claims": [] (ran, found none) is valid.
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
	normalizeUnsupportedClaimFeedback(&out)
	ensureActionableFixInstructions(&out)
	if out.QABlocking && !out.CanAutoFix && len(out.HumanDecisionOptions) == 0 && out.BlockingReason == "" && len(out.BlockingIssues) == 0 {
		out.CanAutoFix = true
	}
	return out
}

func ensureActionableFixInstructions(out *QAOutput) {
	if out == nil || !out.QABlocking || len(out.FixInstructions) > 0 {
		return
	}
	var parts []string
	if s := strings.TrimSpace(out.BlockingReason); s != "" {
		parts = append(parts, s)
	}
	for _, issue := range out.BlockingIssues {
		if s := strings.TrimSpace(issue.Message); s != "" {
			parts = append(parts, s)
		}
	}
	for _, issue := range out.Issues {
		if s := strings.TrimSpace(issue); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		parts = append(parts, "Resolve the blocking QA issue using only facts already supported by the project profile and draft evidence.")
	}
	out.FixInstructions = []string{"Revise the draft so it passes QA: " + strings.Join(parts, " ")}
}

func normalizeUnsupportedClaimFeedback(out *QAOutput) {
	if out == nil {
		return
	}
	hasUnmapped := false
	for _, claim := range out.Claims {
		if !claim.Mapped {
			hasUnmapped = true
			break
		}
	}
	out.HumanDecisionOptions = editorOnlyDecisionOptions(out.HumanDecisionOptions)
	if !hasUnmapped {
		return
	}
	out.QABlocking = true
	out.CanAutoFix = true
	if len(out.FixInstructions) == 0 {
		out.FixInstructions = []string{"Remove or rewrite unsupported product claims using only facts in the confirmed Product Context."}
	}
}

func editorOnlyDecisionOptions(options []HumanDecisionOption) []HumanDecisionOption {
	kept := make([]HumanDecisionOption, 0, len(options))
	for _, option := range options {
		if asksForContextOrEvidence(option.Label) || asksForContextOrEvidence(option.Description) {
			continue
		}
		kept = append(kept, option)
	}
	return kept
}

func asksForContextOrEvidence(s string) bool {
	normalized := strings.ToLower(strings.TrimSpace(s))
	if normalized == "" {
		return false
	}
	for _, token := range []string{"context", "evidence", "profile", "source"} {
		if strings.Contains(normalized, token) {
			return true
		}
	}
	return false
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
